package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestSessionLibraryPersistenceAndLegacyNames(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "new"} {
		if err := s.CreateSession(ctx, Session{ID: id, Name: "Experiment", Goal: "Test evidence", Status: "paused", Concurrency: 1}, []Agent{{ID: id + "-a", SessionID: id, Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a session created before the names table existed.
	if _, err := s.db.Exec(`DELETE FROM session_names WHERE session_id='old'`); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameSession(ctx, "new", "New name"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameSession(ctx, "missing", "Bad"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	entries, err := s.ListSessions(ctx, 1, 0)
	if err != nil || len(entries) != 1 || entries[0].ID != "new" || entries[0].Name != "New name" || entries[0].AgentCount != 1 || entries[0].LastActivity == "" {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	entries, err = s.ListSessions(ctx, 1, 1)
	if err != nil || len(entries) != 1 || entries[0].ID != "old" || entries[0].Name != "" {
		t.Fatalf("legacy: %+v %v", entries, err)
	}
	if err := s.RenameSession(ctx, "old", "Legacy renamed"); err != nil {
		t.Fatal(err)
	}
	session, _, err := s.GetSession(ctx, "old")
	if err != nil || session.Name != "Legacy renamed" || session.Status != "paused" {
		t.Fatalf("session: %+v %v", session, err)
	}
}
