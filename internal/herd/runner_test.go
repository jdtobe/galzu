package herd

import (
	"context"
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
