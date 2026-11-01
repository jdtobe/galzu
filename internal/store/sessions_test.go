package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
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
	entries, err := s.ListSessions(ctx, SessionQuery{Limit: 1})
	if err != nil || len(entries) != 1 || entries[0].ID != "new" || entries[0].Name != "New name" || entries[0].AgentCount != 1 || entries[0].LastActivity == "" {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	entries, err = s.ListSessions(ctx, SessionQuery{Limit: 1, Offset: 1})
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

func TestSessionSearchAndStatusFilters(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, v := range []Session{
		{ID: "s-literal", Name: "100%_Real", Goal: "A literal search", Status: "paused", Concurrency: 1},
		{ID: "s-upper", Name: "Imposter experiment", Goal: "Observations", Brief: "briefneedle context", Status: "paused", Concurrency: 1},
		{ID: "target-42", Name: "Other", Goal: "Evidence investigation", Status: "running", Concurrency: 1},
		{ID: "s-legacy", Goal: "Legacy evidence", Status: "paused", Concurrency: 1},
	} {
		if err := s.CreateSession(ctx, v, []Agent{{ID: v.ID + "-a", SessionID: v.ID, Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM session_names WHERE session_id='s-legacy'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendSocialMessage(ctx, "target-42", "", "target-42-a", "direct", "secretmessageonly", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFinding(ctx, "target-42", "target-42-a", "sharedclaimword about the evidence", "supporting evidenceword", ""); err != nil {
		t.Fatal(err)
	}
	memory := "secretmemoryonly"
	if err := s.CompleteRestWithMemory(ctx, "target-42", "target-42-a", 0, &memory); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		search, status string
		ids            []string
	}{
		{"IMPOSTER", "", []string{"s-upper"}},
		{"evidence", "running", []string{"target-42"}},
		{"evidence", "paused", []string{"s-legacy"}},
		{"GET-4", "", []string{"target-42"}},
		{"%", "", []string{"s-literal"}},
		{"_", "", []string{"s-literal"}},
		{"100%_", "", []string{"s-literal"}},
		{"", "paused", []string{"s-legacy", "s-literal", "s-upper"}},
		{"' OR 1=1 --", "", []string{}},
		{"briefneedle", "", []string{"s-upper"}},
		{"sharedclaimword", "", []string{"target-42"}},
		{"evidenceword", "", []string{"target-42"}},
		{"secretmessageonly", "", []string{}},
		{"secretmemoryonly", "", []string{}},
	} {
		t.Run(tc.search+"/"+tc.status, func(t *testing.T) {
			entries, err := s.ListSessions(ctx, SessionQuery{Search: tc.search, Status: tc.status, Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, entry := range entries {
				ids = append(ids, entry.ID)
			}
			sort.Strings(ids)
			if !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("got %v want %v", ids, tc.ids)
			}
		})
	}
}

func TestSessionFilteringPrecedesPagination(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "target", Name: "Needle", Goal: "Older run", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 55; i++ {
		if err := s.CreateSession(ctx, Session{ID: fmt.Sprintf("decoy-%02d", i), Name: "Archive", Goal: "Newer run", Status: "paused", Concurrency: 1}, nil); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ListSessions(ctx, SessionQuery{Limit: 50})
	if err != nil || len(first) != 50 {
		t.Fatalf("%d %v", len(first), err)
	}
	for _, entry := range first {
		if entry.ID == "target" {
			t.Fatal("target should be beyond the unfiltered first page")
		}
	}
	found, err := s.ListSessions(ctx, SessionQuery{Search: "needle", Status: "running", Limit: 50})
	if err != nil || len(found) != 1 || found[0].ID != "target" {
		t.Fatalf("%+v %v", found, err)
	}
	page, err := s.ListSessions(ctx, SessionQuery{Search: "archive", Status: "paused", Limit: 50, Offset: 50})
	if err != nil || len(page) != 5 {
		t.Fatalf("%d %v", len(page), err)
	}
}
