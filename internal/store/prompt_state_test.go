package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPromptCutDefaultsAdvancesAndPersists(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, []Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	if cut, err := s.PromptCut(ctx, "a"); err != nil || cut != 0 {
		t.Fatalf("default cut = %d, %v", cut, err)
	}
	if err := s.SetPromptCut(ctx, "a", 42); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPromptCut(ctx, "a", 43); err != nil {
		t.Fatal(err)
	}
	if cut, err := s.PromptCut(ctx, "a"); err != nil || cut != 43 {
		t.Fatalf("cut = %d, %v", cut, err)
	}
	if err := s.DeleteSession(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if cut, err := s.PromptCut(ctx, "a"); err != nil || cut != 0 {
		t.Fatalf("cut survived deletion: %d, %v", cut, err)
	}
}
