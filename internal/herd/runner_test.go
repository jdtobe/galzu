package herd

import (
	"context"
	"fmt"
	"github.com/jdtobe/garth/internal/ollama"
	"github.com/jdtobe/garth/internal/store"
	"path/filepath"
	"testing"
	"time"
)

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
