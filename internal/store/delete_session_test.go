package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestDeleteSessionIsCompleteAndAtomic(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"target", "keep"} {
		if err := s.CreateSession(ctx, Session{ID: id, Name: id, Goal: "test", Status: "paused", Concurrency: 1, TargetIP: "192.0.2.1", RateIntervalMS: 1000, InferenceTimeoutMS: 1000, RetryDelaysMS: []int{10}}, []Agent{{ID: id + "-a", SessionID: id, Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
			t.Fatal(err)
		}
		memory := "private memory"
		if err := s.SetThinking(ctx, id, id+"-a", true); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AppendAgentReplyWithMemory(ctx, id, id+"-a", "", "help", "hello", "thinking", 0, &memory); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordFailure(ctx, id, id+"-a", "inference", "test failure", "excerpt", time.Now().Add(time.Hour), 1); err != nil {
			t.Fatal(err)
		}
		if err := s.BeginAttempt(ctx, id, id+"-a"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetPromptCut(ctx, id+"-a", 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`CREATE TRIGGER refuse_delete BEFORE DELETE ON sessions WHEN OLD.id='target' BEGIN SELECT RAISE(ABORT,'test refusal'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, "target"); err == nil {
		t.Fatal("expected rollback")
	}
	if messages, err := s.Messages(ctx, "target", 0); err != nil || len(messages) != 1 || messages[0].Thinking != "thinking" {
		t.Fatalf("rollback lost messages: %+v %v", messages, err)
	}
	if memory, _, err := s.AgentMemory(ctx, "target", "target-a"); err != nil || memory != "private memory" {
		t.Fatalf("rollback lost memory: %q %v", memory, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER refuse_delete`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, "target"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GetSession(ctx, "target"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("session retained: %v", err)
	}
	// Every session-owned table should now contain only the other run's data.
	for _, table := range []string{"sessions", "agents", "messages", "events", "network_policies", "inference_policies", "session_names", "message_routing", "message_thinking", "agent_settings", "agent_checkpoints", "delivery_baselines", "agent_memory", "agent_prompt_state", "agent_recovery", "agent_failures", "agent_inflight"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count == 0 {
			t.Fatalf("other run lost %s: %d %v", table, count, err)
		}
		// Removing the remaining run must clear each table, proving no orphaned target rows remain.
	}
	if err := s.DeleteSession(ctx, "keep"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"sessions", "agents", "messages", "events", "network_policies", "inference_policies", "session_names", "message_routing", "message_thinking", "agent_settings", "agent_checkpoints", "delivery_baselines", "agent_memory", "agent_prompt_state", "agent_recovery", "agent_failures", "agent_inflight"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphaned %s: %d %v", table, count, err)
		}
	}
}

func TestDeleteSessionRequiresPause(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, "s"); !errors.Is(err, ErrSessionNotPaused) {
		t.Fatalf("%v", err)
	}
	if err := s.DeleteSession(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("%v", err)
	}
}
