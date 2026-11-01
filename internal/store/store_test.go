package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDeliveryBaselineForOlderSessions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateSession(ctx, Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, []Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	first, err := s.AppendMessage(ctx, "s", "", "old")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`DROP TABLE delivery_baselines`); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.DeliveryProgress(ctx, "s")
	if err != nil || p["a"].Baseline != first.ID {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err = s.AppendMessage(ctx, "s", "", "new"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`DELETE FROM agent_checkpoints WHERE agent_id='a'`); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.Checkpoint(ctx, "s", "a")
	if err != nil || checkpoint != first.ID {
		t.Fatalf("checkpoint %d: %v", checkpoint, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err = s.DeliveryProgress(ctx, "s")
	if err != nil || p["a"].Baseline != first.ID {
		t.Fatalf("baseline changed: %+v %v", p, err)
	}
}

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

func TestMemorySavedAtomicallyWithReply(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, []Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}, {ID: "b", SessionID: "s", Name: "Bea", Role: "helper", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	memory := "private, uncertain claim"
	if _, err = s.AppendAgentReplyWithMemory(ctx, "s", "a", "missing", "direct", "hi", "", 1, &memory); err == nil {
		t.Fatal("invalid recipient accepted")
	}
	got, fresh, err := s.AgentMemory(ctx, "s", "a")
	if err != nil || got != "" || fresh {
		t.Fatalf("rollback: %q %v %v", got, fresh, err)
	}
	if _, err = s.AppendAgentReplyWithMemory(ctx, "s", "a", "b", "direct", "hi", "", 1, &memory); err != nil {
		t.Fatal(err)
	}
	got, fresh, err = s.AgentMemory(ctx, "s", "a")
	if err != nil || got != memory || !fresh {
		t.Fatalf("saved: %q %v %v", got, fresh, err)
	}
	got, fresh, err = s.AgentMemory(ctx, "s", "b")
	if err != nil || got != "" || fresh {
		t.Fatalf("other agent: %q %v %v", got, fresh, err)
	}
	if _, _, err = s.AgentMemory(ctx, "other", "a"); err == nil {
		t.Fatal("cross-session lookup accepted")
	}
}

func TestInterruptedAttemptIsRecordedWithoutFailure(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, []Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.BeginAttempt(ctx, "s", "a"); err != nil {
		t.Fatal(err)
	}
	if err = s.InterruptAttempts(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if err = s.InterruptAttempts(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(ctx, "s", 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Kind == "inference_interrupted" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("interrupt count %d", count)
	}
	recovery, err := s.Recovery(ctx, "a")
	if err != nil || recovery.Failures != 0 {
		t.Fatalf("recovery %+v %v", recovery, err)
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

func TestSessionBriefRoundTripsAndSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	brief := "Galzu is a platform for running a herd of local AI agents. A herd is a group of agents, not livestock."
	if err = s.CreateSession(ctx, Session{ID: "s", Goal: "pick a feature", Brief: brief, Status: "paused", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	saved, _, err := s.GetSession(ctx, "s")
	if err != nil || saved.Brief != brief {
		t.Fatalf("brief = %q, err %v", saved.Brief, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopening runs the ALTER again; a duplicate column must not be an error.
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	saved, _, err = s.GetSession(ctx, "s")
	if err != nil || saved.Brief != brief {
		t.Fatalf("after reopen brief = %q, err %v", saved.Brief, err)
	}
}
