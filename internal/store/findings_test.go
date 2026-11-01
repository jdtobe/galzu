package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestFindingsKeepRevisionsEvidenceAndPositions(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "g", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}

	finding, err := s.CreateFinding(ctx, "s", "a", "The cache survives a rename.", "Renames appear only in the turn packet.", "")
	if err != nil {
		t.Fatal(err)
	}
	if finding.Status != "open" || finding.Claim == "" || len(finding.Evidence) != 1 || finding.Evidence[0].Stance != "support" {
		t.Fatalf("finding = %+v", finding)
	}
	if len(finding.Revisions) != 1 || finding.Revisions[0].Claim != "The cache survives a rename." {
		t.Fatalf("revisions = %+v", finding.Revisions)
	}

	revised, err := s.ReviseFinding(ctx, "s", finding.ID, "b", "The cache survives a rename, except at cut.", "I reproduced a miss only after compaction.", "oppose")
	if err != nil {
		t.Fatal(err)
	}
	if revised.Claim != "The cache survives a rename, except at cut." || len(revised.Revisions) != 2 || len(revised.Evidence) != 2 {
		t.Fatalf("revised = %+v", revised)
	}
	if revised.Revisions[0].Claim != "The cache survives a rename." {
		t.Fatalf("history was erased: %+v", revised.Revisions)
	}

	if err := s.EndorseFinding(ctx, "s", finding.ID, "a", "agree", "matches my reading"); err != nil {
		t.Fatal(err)
	}
	if err := s.EndorseFinding(ctx, "s", finding.ID, "a", "disagree", "changed my mind"); err != nil {
		t.Fatal(err)
	}
	if err := s.EndorseFinding(ctx, "s", finding.ID, "b", "agree", ""); err != nil {
		t.Fatal(err)
	}
	latest, err := s.Findings(ctx, "s", 0)
	if err != nil || len(latest) != 1 {
		t.Fatalf("findings = %+v err %v", latest, err)
	}
	if len(latest[0].Positions) != 2 {
		t.Fatalf("positions = %+v", latest[0].Positions)
	}
	for _, position := range latest[0].Positions {
		if position.AgentID == "a" && (position.Choice != "disagree" || position.Text != "changed my mind") {
			t.Fatalf("a later position did not replace the earlier one: %+v", position)
		}
	}
}

func TestFindingLimitKeepsTheNewest(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "g", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	for _, claim := range []string{"one", "two", "three"} {
		if _, err := s.CreateFinding(ctx, "s", "a", claim, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := s.Findings(ctx, "s", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].Claim != "two" || recent[1].Claim != "three" {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestFindingFromAnotherSessionIsNotReachable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []string{"s", "other"} {
		if err := s.CreateSession(ctx, Session{ID: id, Goal: "g", Status: "running", Concurrency: 1}, nil); err != nil {
			t.Fatal(err)
		}
	}
	finding, err := s.CreateFinding(ctx, "s", "a", "claim", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviseFinding(ctx, "other", finding.ID, "a", "new", "", ""); !errors.Is(err, ErrFindingGone) {
		t.Fatalf("revise across sessions: %v", err)
	}
	if err := s.EndorseFinding(ctx, "other", finding.ID, "a", "agree", ""); !errors.Is(err, ErrFindingGone) {
		t.Fatalf("endorse across sessions: %v", err)
	}
}

func TestDeletingASessionRemovesItsFindings(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "g", Status: "paused", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	finding, err := s.CreateFinding(ctx, "s", "a", "claim", "evidence", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EndorseFinding(ctx, "s", finding.ID, "b", "agree", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"findings", "finding_revisions", "finding_evidence", "finding_positions"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphaned %s: %d %v", table, count, err)
		}
	}
}
