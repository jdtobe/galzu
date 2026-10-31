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
			if !strings.Contains(p[len(p)-1].Content, want) {
				t.Fatalf("want %s, got %s", want, p[len(p)-1].Content)
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
		if !strings.Contains(prompt[len(prompt)-1].Content, "while offline") {
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
		if !strings.Contains(p[0].Content, "You are Bea") {
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
			for _, n := range []string{"Ada", "Bea", "Cy"} {
				if strings.Contains(p[0].Content, "You are "+n+" ") {
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
