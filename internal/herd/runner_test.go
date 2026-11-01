package herd

import (
	"context"
	"fmt"
	"github.com/jdtobe/garth/internal/ollama"
	"github.com/jdtobe/garth/internal/store"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type capturePrompts struct{ prompts chan []ollama.Message }

type memoryInference struct {
	prompts chan []ollama.Message
	replies chan ollama.Reply
}

func (f memoryInference) Chat(ctx context.Context, _ string, p []ollama.Message) (ollama.Reply, error) {
	f.prompts <- p
	select {
	case reply := <-f.replies:
		return reply, nil
	case <-ctx.Done():
		return ollama.Reply{}, ctx.Err()
	}
}

func TestWorkingMemoryTravelsInTurnPacketWithoutRewritingHistory(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10; i++ {
		if _, err = s.AppendMessage(ctx, "s", "", fmt.Sprintf("old-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	f := memoryInference{make(chan []ollama.Message, 4), make(chan ollama.Reply, 4)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	var initial []ollama.Message
	select {
	case initial = <-f.prompts:
	case <-time.After(time.Second):
		t.Fatal("no initial turn")
	}
	memory := "Unverified claim from Ada; still needs evidence."
	f.replies <- ollama.Reply{Action: "rest", Memory: &memory}
	waitMemory := func(wantFresh bool) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for {
			got, fresh, err := s.AgentMemory(ctx, "s", "a")
			if err != nil {
				t.Fatal(err)
			}
			if got == memory && fresh == wantFresh {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("memory %q fresh %v", got, fresh)
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitMemory(true)
	if _, err = s.AppendMessage(ctx, "s", "", "new-one"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	var second []ollama.Message
	select {
	case second = <-f.prompts:
	case <-time.After(time.Second):
		t.Fatal("no second turn")
	}
	text := promptText(second)
	if !strings.Contains(text, memory) || !strings.Contains(text, "new-one") {
		t.Fatalf("memory or new message missing: %s", text)
	}
	// History stays in the prompt: memory lives in the turn packet, not in place
	// of the log, so the previous prefix is still a byte prefix of this one.
	for i := 0; i < len(initial)-1; i++ {
		if second[i] != initial[i] {
			t.Fatalf("prefix changed at message %d", i)
		}
	}
	f.replies <- ollama.Reply{Action: "rest"}
	waitMemory(false)
	if _, err = s.AppendMessage(ctx, "s", "", "new-two"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case third := <-f.prompts:
		text := promptText(third)
		if !strings.Contains(text, "old-01") || !strings.Contains(text, "new-two") || !strings.Contains(text, memory) {
			t.Fatalf("stale memory dropped history or memory: %s", text)
		}
		for i := 0; i < len(second)-1; i++ {
			if third[i] != second[i] {
				t.Fatalf("prefix changed at message %d after stale memory", i)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("no third turn")
	}
	f.replies <- ollama.Reply{Action: "rest"}
}

type timeoutInference struct{ called chan struct{} }

func (f timeoutInference) Chat(ctx context.Context, _ string, _ []ollama.Message) (ollama.Reply, error) {
	f.called <- struct{}{}
	<-ctx.Done()
	return ollama.Reply{}, ctx.Err()
}
func TestInferenceTimeoutIsRetryable(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1, InferenceTimeoutMS: 20, RetryDelaysMS: []int{10000}}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := timeoutInference{make(chan struct{}, 2)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendMessage(ctx, "s", "", "hello"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case <-f.called:
	case <-time.After(time.Second):
		t.Fatal("no inference")
	}
	deadline := time.Now().Add(time.Second)
	for {
		recovery, err := s.Recovery(ctx, "a")
		if err != nil {
			t.Fatal(err)
		}
		if recovery.Failures == 1 {
			if recovery.Category != "inference" || !strings.Contains(recovery.Reason, "deadline exceeded") {
				t.Fatal(recovery)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout not recorded")
		}
		time.Sleep(time.Millisecond)
	}
}

type contextThenRest struct {
	prompts chan []ollama.Message
	count   int
}

func (f *contextThenRest) Chat(_ context.Context, _ string, p []ollama.Message) (ollama.Reply, error) {
	f.count++
	f.prompts <- p
	if f.count == 1 {
		return ollama.Reply{}, &ollama.ContextLimitError{}
	}
	return ollama.Reply{Action: "rest"}, nil
}
func TestContextLimitRetriesSmallerPendingBatch(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 50; i++ {
		if _, err = s.AppendMessage(ctx, "s", "", fmt.Sprintf("item-%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	f := &contextThenRest{prompts: make(chan []ollama.Message, 4)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.prompts:
	case <-time.After(time.Second):
		t.Fatal("no attempt")
	}
	deadline := time.Now().Add(time.Second)
	for r.Activity("s")["a"].State != "retrying" {
		if time.Now().After(deadline) {
			t.Fatal("not retrying")
		}
		time.Sleep(time.Millisecond)
	}
	if err = r.RetryAgent("s", "a"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-f.prompts:
		var text strings.Builder
		for _, m := range p {
			text.WriteString(m.Content)
		}
		if !strings.Contains(text.String(), "item-025") || strings.Contains(text.String(), "item-026") || !strings.Contains(text.String(), "context window") {
			t.Fatalf("bad retry: %s", text.String())
		}
	case <-time.After(time.Second):
		t.Fatal("no retry")
	}
	select {
	case p := <-f.prompts:
		var text strings.Builder
		for _, m := range p {
			text.WriteString(m.Content)
		}
		if !strings.Contains(text.String(), "item-050") {
			t.Fatal("remaining backlog omitted")
		}
	case <-time.After(time.Second):
		t.Fatal("backlog stalled")
	}
}

func TestStorageFailureRequiresExplicitRecovery(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := capturePrompts{make(chan []ollama.Message, 2)}
	r := New(s, f)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	r.StorageFailure("s", fmt.Errorf("disk error"))
	if r.StorageError("s") == "" {
		t.Fatal("no storage error")
	}
	if err := r.Resume("s"); err == nil {
		t.Fatal("resumed despite storage error")
	}
	if _, err := s.AppendMessage(ctx, "s", "", "pending"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case <-f.prompts:
		t.Fatal("scheduled despite storage failure")
	case <-time.After(50 * time.Millisecond):
	}
	if err := r.RecoverStorage("s"); err != nil {
		t.Fatal(err)
	}
	if r.StorageError("s") != "" {
		t.Fatal("storage error not cleared")
	}
	if err := r.Resume("s"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.prompts:
	case <-time.After(time.Second):
		t.Fatal("pending message not processed after recovery")
	}
}

type invalidThenRest struct {
	prompts chan []ollama.Message
	count   int
}

func (f *invalidThenRest) Chat(_ context.Context, _ string, p []ollama.Message) (ollama.Reply, error) {
	f.count++
	f.prompts <- p
	if f.count == 1 {
		return ollama.Reply{}, &ollama.ValidationError{Reason: "message text is empty", Excerpt: `{"text":""}`}
	}
	return ollama.Reply{Action: "rest"}, nil
}
func TestInvalidActionRetrySeesFeedbackAndNewMessages(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := &invalidThenRest{prompts: make(chan []ollama.Message, 2)}
	r := New(s, f)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "first"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case <-f.prompts:
	case <-time.After(time.Second):
		t.Fatal("no initial attempt")
	}
	deadline := time.Now().Add(time.Second)
	for r.Activity("s")["a"].State != "retrying" {
		if time.Now().After(deadline) {
			t.Fatal("no retry")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "new information"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	if err := r.RetryAgent("s", "a"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-f.prompts:
		var text strings.Builder
		for _, m := range p {
			text.WriteString(m.Content)
		}
		if !strings.Contains(text.String(), "new information") || !strings.Contains(text.String(), "message text is empty") {
			t.Fatal(text.String())
		}
	case <-time.After(time.Second):
		t.Fatal("no retry attempt")
	}
	deadline = time.Now().Add(time.Second)
	for {
		v, err := s.Recovery(ctx, "a")
		if err != nil {
			t.Fatal(err)
		}
		if v.Failures == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovery not cleared")
		}
		time.Sleep(time.Millisecond)
	}
	for r.Activity("s")["a"].LastOutcome != "rested" {
		if time.Now().After(deadline) {
			t.Fatal("rest outcome not visible")
		}
		time.Sleep(time.Millisecond)
	}
	history, err := s.Failures(ctx, "s")
	if err != nil || len(history) != 1 || history[0].Category != "validation" {
		t.Fatalf("%+v %v", history, err)
	}
}

func TestPendingBacklogProcessedInOrder(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 110; i++ {
		if _, err := s.AppendMessage(ctx, "s", "", fmt.Sprintf("item-%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	f := capturePrompts{make(chan []ollama.Message, 4)}
	r := New(s, f)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"item-050", "item-100", "item-110"} {
		select {
		case p := <-f.prompts:
			if text := promptText(p); !strings.Contains(text, want) {
				t.Fatalf("want %s, got %s", want, text)
			}
		case <-time.After(time.Second):
			t.Fatal("backlog stalled")
		}
	}
}

type failingOnce struct {
	calls chan int
	count int
}

func TestAutomaticRetryAfterBackoff(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1, InferenceTimeoutMS: 1000, RetryDelaysMS: []int{10}}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := &failingOnce{calls: make(chan int, 2)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendMessage(ctx, "s", "", "question"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	for i := 1; i <= 2; i++ {
		select {
		case n := <-f.calls:
			if n != i {
				t.Fatalf("attempt %d, want %d", n, i)
			}
		case <-time.After(time.Second):
			t.Fatal("automatic retry stalled")
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		messages, err := s.Messages(ctx, "s", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reply not saved")
		}
		time.Sleep(time.Millisecond)
	}
	history, err := s.Failures(ctx, "s")
	if err != nil || len(history) != 1 {
		t.Fatalf("failures: %+v %v", history, err)
	}
}

func TestRecoveryContinuesAfterRestart(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendMessage(ctx, "s", "", "pending"); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordFailure(ctx, "s", "a", "validation", "invalid action", "bad output", time.Now().Add(-time.Second), 1); err != nil {
		t.Fatal(err)
	}
	f := capturePrompts{make(chan []ollama.Message, 1)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Resume("s"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-f.prompts:
		var text strings.Builder
		for _, m := range p {
			text.WriteString(m.Content)
		}
		if !strings.Contains(text.String(), "invalid action") || !strings.Contains(text.String(), "pending") {
			t.Fatal(text.String())
		}
	case <-time.After(time.Second):
		t.Fatal("persisted retry stalled")
	}
}

func (f *failingOnce) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	f.count++
	f.calls <- f.count
	if f.count == 1 {
		return ollama.Reply{}, fmt.Errorf("model unavailable")
	}
	return ollama.Reply{Action: "message", Text: "recovered"}, nil
}

func TestRetryFailedAgent(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := &failingOnce{calls: make(chan int, 2)}
	r := New(s, f)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "question"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case <-f.calls:
	case <-time.After(time.Second):
		t.Fatal("no inference")
	}
	deadline := time.Now().Add(time.Second)
	for {
		if r.Activity("s")["a"].State == "retrying" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no retrying status")
		}
		time.Sleep(time.Millisecond)
	}
	if err := r.RetryAgent("s", "a"); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-f.calls:
		if n != 2 {
			t.Fatal(n)
		}
	case <-time.After(time.Second):
		t.Fatal("retry did not start")
	}
	deadline = time.Now().Add(time.Second)
	for {
		messages, err := s.Messages(ctx, "s", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) == 2 {
			if messages[1].Body != "recovered" {
				t.Fatal(messages)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no recovered reply")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestResumeProcessesPendingMessagesOnce(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "while offline"); err != nil {
		t.Fatal(err)
	}
	f := capturePrompts{make(chan []ollama.Message, 2)}
	r := New(s, f)
	if err := r.Resume("s"); err != nil {
		t.Fatal(err)
	}
	select {
	case prompt := <-f.prompts:
		if !strings.Contains(promptText(prompt), "while offline") {
			t.Fatal(prompt)
		}
	case <-time.After(time.Second):
		t.Fatal("pending message not delivered")
	}
	deadline := time.Now().Add(time.Second)
	for {
		n, err := s.Checkpoint(ctx, "s", "a")
		if err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("checkpoint not saved")
		}
		time.Sleep(time.Millisecond)
	}
	r.Stop()
	r2 := New(s, f)
	defer r2.Stop()
	if err := r2.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-f.prompts:
		t.Fatalf("processed message replayed: %v", p)
	case <-time.After(100 * time.Millisecond):
	}
}

func (f capturePrompts) Chat(_ context.Context, _ string, prompt []ollama.Message) (ollama.Reply, error) {
	f.prompts <- prompt
	return ollama.Reply{Action: "rest"}, nil
}

func TestPrivateMessagesOnlyReachParticipants(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	agents := []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}, {ID: "b", SessionID: "s", Name: "Bea", Role: "helper", Model: "m", Status: "ready"}, {ID: "c", SessionID: "s", Name: "Cy", Role: "helper", Model: "m", Status: "ready"}}
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 3}, agents); err != nil {
		t.Fatal(err)
	}
	f := capturePrompts{make(chan []ollama.Message, 6)}
	r := New(s, f)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendSocialMessage(ctx, "s", "a", "b", "direct", "secret evidence", ""); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case p := <-f.prompts:
		if !strings.Contains(p[0].Content, "You are agent b.") {
			t.Fatalf("wrong recipient: %s", p[0].Content)
		}
	case <-time.After(time.Second):
		t.Fatal("recipient not woken")
	}
	if _, err := s.AppendMessage(ctx, "s", "", "public update"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case p := <-f.prompts:
			text := ""
			for _, m := range p {
				text += m.Content + "\n"
			}
			name := ""
			for id, n := range map[string]string{"a": "Ada", "b": "Bea", "c": "Cy"} {
				if strings.Contains(p[0].Content, "You are agent "+id+".") {
					name = n
				}
			}
			if name == "" {
				t.Fatalf("unknown agent: %s", p[0].Content)
			}
			if strings.Contains(text, "secret evidence") != (name != "Cy") {
				t.Fatalf("private content visibility for %s: %s", name, text)
			}
			if name == "Cy" && !strings.Contains(text, "content is private") {
				t.Fatalf("missing private activity notice: %s", text)
			}
			seen[name] = true
		case <-time.After(time.Second):
			t.Fatalf("missing prompts: %v", seen)
		}
	}
}

type fake struct {
	start   chan struct{}
	release chan struct{}
}
type thinkingFake struct{ requested chan bool }

func (f *thinkingFake) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	return ollama.Reply{}, fmt.Errorf("unexpected basic chat")
}
func (f *thinkingFake) ChatWithThinking(_ context.Context, _ string, _ []ollama.Message, think bool) (ollama.Reply, error) {
	f.requested <- think
	return ollama.Reply{Action: "message", Text: "answer", Thinking: "reason"}, nil
}

func TestThinkingSettingAppliesToNextTurn(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "a", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := &thinkingFake{requested: make(chan bool, 1)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Resume("s"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetThinking(ctx, "s", "a", true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendMessage(ctx, "s", "", "hi"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case enabled := <-f.requested:
		if !enabled {
			t.Fatal("thinking not requested")
		}
	case <-time.After(time.Second):
		t.Fatal("no inference")
	}
	deadline := time.After(time.Second)
	for {
		m, err := s.Messages(ctx, "s", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(m) > 1 {
			if m[1].Thinking != "reason" {
				t.Fatalf("%+v", m[1])
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("no response")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func (f *fake) Chat(ctx context.Context, _ string, _ []ollama.Message) (ollama.Reply, error) {
	select {
	case f.start <- struct{}{}:
	default:
	}
	select {
	case <-f.release:
		return ollama.Reply{Action: "message", Text: "hello"}, nil
	case <-ctx.Done():
		return ollama.Reply{}, ctx.Err()
	}
}
func TestPauseHoldsResult(t *testing.T) {
	ctx := context.Background()
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	e = s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "a", Role: "leader", Model: "m", Status: "ready"}})
	if e != nil {
		t.Fatal(e)
	}
	f := &fake{make(chan struct{}, 1), make(chan struct{})}
	r := New(s, f)
	defer r.Stop()
	if e = r.Resume("s"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AppendMessage(ctx, "s", "", "hi"); e != nil {
		t.Fatal(e)
	}
	r.Notify("s")
	select {
	case <-f.start:
	case <-time.After(time.Second):
		t.Fatal("no inference")
	}
	if e = r.Pause("s"); e != nil {
		t.Fatal(e)
	}
	close(f.release)
	time.Sleep(30 * time.Millisecond)
	m, _ := s.Messages(ctx, "s", 0)
	if len(m) != 1 {
		t.Fatalf("published while paused: %+v", m)
	}
	if e = r.Resume("s"); e != nil {
		t.Fatal(e)
	}
	deadline := time.After(time.Second)
	for {
		m, _ = s.Messages(ctx, "s", 0)
		if len(m) == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no result")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestPromptCompactsAtBudgetAndStaysAppendOnly(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	body := "deep-history-marker " + strings.Repeat("x", 1024)
	for i := 1; i <= 60; i++ {
		if _, err := s.AppendMessage(ctx, "s", "", fmt.Sprintf("%s %03d", body, i)); err != nil {
			t.Fatal(err)
		}
	}
	f := capturePrompts{make(chan []ollama.Message, 8)}
	r := New(s, f)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	var first []ollama.Message
	select {
	case first = <-f.prompts:
	case <-time.After(2 * time.Second):
		t.Fatal("no initial turn")
	}
	cut, err := s.PromptCut(ctx, "a")
	if err != nil || cut == 0 {
		t.Fatalf("prompt did not compact: cut=%d err=%v", cut, err)
	}
	text := promptText(first)
	if strings.Contains(text, "deep-history-marker x") && strings.Contains(text, " 001") {
		t.Fatal("old history was retained past the cut")
	}
	if len(text) > promptBudgetBytes+4096 {
		t.Fatalf("prompt still over budget: %d bytes", len(text))
	}
	if !strings.Contains(text, "Messages up to id") {
		t.Fatalf("compaction header missing: %s", text[:200])
	}
	if _, err := s.AppendMessage(ctx, "s", "", "after-compaction"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case second := <-f.prompts:
		for i := 0; i < len(first)-1; i++ {
			if second[i] != first[i] {
				t.Fatalf("prefix changed after compaction at message %d", i)
			}
		}
		if !strings.Contains(promptText(second), "after-compaction") {
			t.Fatal("new message missing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no second turn")
	}
}
