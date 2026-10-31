package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAgentReplyAndCheckpointAtomic(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	input, err := s.AppendMessage(ctx, "s", "", "question")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAgentReply(ctx, "s", "missing", "", "", "bad", "", input.ID); err == nil {
		t.Fatal("missing checkpoint accepted")
	}
	messages, err := s.Messages(ctx, "s", 0)
	if err != nil || len(messages) != 1 {
		t.Fatalf("rollback: %+v %v", messages, err)
	}
	if _, err := s.AppendAgentReply(ctx, "s", "a", "", "", "answer", "", input.ID); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.Checkpoint(ctx, "s", "a")
	if err != nil || checkpoint != input.ID {
		t.Fatalf("checkpoint %d: %v", checkpoint, err)
	}
	messages, err = s.Messages(ctx, "s", 0)
	if err != nil || len(messages) != 2 || messages[1].Body != "answer" {
		t.Fatalf("commit: %+v %v", messages, err)
	}
}

func TestPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "herd.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	session := Session{ID: "s", Goal: "work together", Status: "paused", Concurrency: 2}
	agents := []Agent{{ID: "a", SessionID: "s", Name: "A", Role: "leader", Model: "test", Status: "ready"}}
	if err = s.CreateSession(ctx, session, agents); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendMessage(ctx, "s", "", "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendMessage(ctx, "s", "a", "help"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, aa, err := s.GetSession(ctx, "s")
	if err != nil || got.Goal != session.Goal || len(aa) != 1 {
		t.Fatalf("%+v %+v %v", got, aa, err)
	}
	m, err := s.Messages(ctx, "s", 0)
	if err != nil || len(m) != 2 || m[0].Body != "hello" || m[1].Body != "help" {
		t.Fatalf("%+v %v", m, err)
	}
	e, err := s.Events(ctx, "s", 0)
	if err != nil || len(e) < 2 || e[0].ID >= e[1].ID {
		t.Fatalf("%+v %v", e, err)
	}
}
