package herd

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

// A sole agent has no peer to wake it. After a response it continues on its own
// thread, and rest stops it.
func TestSoloAgentContinuesAfterItsOwnResponse(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "work", Status: "running", Concurrency: 1},
		[]store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendMessage(ctx, "s", "", "start"); err != nil {
		t.Fatal(err)
	}
	f := memoryInference{make(chan []ollama.Message, 4), make(chan ollama.Reply, 4)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.prompts:
	case <-time.After(time.Second):
		t.Fatal("no initial turn")
	}
	f.replies <- ollama.Reply{Action: "message", Text: "working"}
	select {
	case <-f.prompts:
	case <-time.After(2 * time.Second):
		t.Fatal("solo agent did not continue after its own response")
	}
	f.replies <- ollama.Reply{Action: "rest"}
	messages, err := s.Messages(ctx, "s", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Body != "working" {
		t.Fatalf("messages = %+v", messages)
	}
}
