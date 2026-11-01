package herd

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

type lateDeletionReply struct{ started, release, returned chan struct{} }

func (f lateDeletionReply) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	close(f.started)
	<-f.release
	defer close(f.returned)
	return ollama.Reply{Action: "message", Text: "must not be saved"}, nil
}

func TestDeletePausedSessionDiscardsInFlightResult(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := lateDeletionReply{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	r := New(s, f)
	defer r.Stop()
	if _, err := s.AppendMessage(ctx, "s", "", "question"); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("inference not started")
	}
	if err := r.DeleteSession(ctx, "s"); !errors.Is(err, store.ErrSessionNotPaused) {
		t.Fatalf("deleted running session: %v", err)
	}
	if err := r.Pause("s"); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteSession(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	close(f.release)
	select {
	case <-f.returned:
	case <-time.After(time.Second):
		t.Fatal("late result not released")
	}
	r.setActivity("s", "a", Activity{State: "idle"})
	if len(r.Activity("s")) != 0 {
		t.Fatal("late activity resurrected deleted run")
	}
	if err := r.Resume("s"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted run resumed: %v", err)
	}
	if messages, err := s.Messages(ctx, "s", 0); err != nil || len(messages) != 0 {
		t.Fatalf("late reply retained: %+v %v", messages, err)
	}
}
