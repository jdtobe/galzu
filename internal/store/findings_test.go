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
	if _, err := s.RequestReview(ctx, "s", finding.ID, "a", "b", "check this"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"findings", "finding_revisions", "finding_evidence", "finding_positions", "finding_reviews"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphaned %s: %d %v", table, count, err)
		}
	}
}

func TestRetractionKeepsTheRecord(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "g", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	finding, err := s.CreateFinding(ctx, "s", "a", "a claim", "evidence", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EndorseFinding(ctx, "s", finding.ID, "b", "agree", "still think so"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetractFinding(ctx, "s", finding.ID); err != nil {
		t.Fatal(err)
	}
	findings, err := s.Findings(ctx, "s", 0)
	if err != nil || len(findings) != 1 {
		t.Fatalf("%+v %v", findings, err)
	}
	if findings[0].Status != "withdrawn" || len(findings[0].Evidence) != 1 || len(findings[0].Positions) != 1 {
		t.Fatalf("record was erased: %+v", findings[0])
	}
	if err := s.RetractFinding(ctx, "s", finding.ID); !errors.Is(err, ErrFindingWithdrawn) {
		t.Fatalf("second retraction: %v", err)
	}
	if err := s.RetractFinding(ctx, "other", finding.ID); !errors.Is(err, ErrFindingGone) {
		t.Fatalf("cross-session retraction: %v", err)
	}
}

func TestAllFindingsCarriesSessionAndAuthor(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, session := range []Session{
		{ID: "s1", Name: "First experiment", Goal: "g1", Status: "paused", Concurrency: 1},
		{ID: "s2", Name: "Second experiment", Goal: "g2", Status: "paused", Concurrency: 1},
	} {
		if err := s.CreateSession(ctx, session, []Agent{{ID: session.ID + "-a", SessionID: session.ID, Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.CreateFinding(ctx, "s1", "s1-a", "Old claim about caches", "first evidence", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFinding(ctx, "s2", "s2-a", "New claim about plans", "second evidence", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviseFinding(ctx, "s1", first.ID, "s1-a", "Old claim, revised", "", ""); err != nil {
		t.Fatal(err)
	}

	all, err := s.AllFindings(ctx, "", 10, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("%+v %v", all, err)
	}
	if all[0].Claim != "Old claim, revised" || all[0].SessionName != "First experiment" || all[0].AuthorName != "Ada" {
		t.Fatalf("newest first with names: %+v", all[0])
	}
	if len(all[0].Evidence) != 1 || all[0].Evidence[0].Body != "first evidence" {
		t.Fatalf("evidence: %+v", all[0])
	}
	found, err := s.AllFindings(ctx, "second evidence", 10, 0)
	if err != nil || len(found) != 1 || found[0].SessionName != "Second experiment" {
		t.Fatalf("evidence search: %+v %v", found, err)
	}
	found, err = s.AllFindings(ctx, "ada", 10, 0)
	if err != nil || len(found) != 2 {
		t.Fatalf("author search: %+v %v", found, err)
	}
	page, err := s.AllFindings(ctx, "", 1, 1)
	if err != nil || len(page) != 1 || page[0].Claim != "New claim about plans" {
		t.Fatalf("paging: %+v %v", page, err)
	}
}

func TestFindingReviewIsRequestedAnsweredAndNotSelf(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, Session{ID: "s", Goal: "g", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	finding, err := s.CreateFinding(ctx, "s", "a", "a claim with a weak spot", "some evidence", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestReview(ctx, "s", finding.ID, "a", "a", "self review"); err == nil {
		t.Fatal("requested a review from itself")
	}
	review, err := s.RequestReview(ctx, "s", finding.ID, "a", "b", "is the evidence sufficient?")
	if err != nil || review.Status != "open" {
		t.Fatalf("%+v %v", review, err)
	}
	open, err := s.OpenReviewsFor(ctx, "s", "b")
	if err != nil || len(open) != 1 || open[0].Request != "is the evidence sufficient?" {
		t.Fatalf("%+v %v", open, err)
	}
	if err := s.ResolveReviews(ctx, finding.ID, "b"); err != nil {
		t.Fatal(err)
	}
	open, err = s.OpenReviewsFor(ctx, "s", "b")
	if err != nil || len(open) != 0 {
		t.Fatalf("still open: %+v %v", open, err)
	}
	stored, err := s.Finding(ctx, "s", finding.ID)
	if err != nil || len(stored.Reviews) != 1 || stored.Reviews[0].Status != "answered" {
		t.Fatalf("finding = %+v %v", stored, err)
	}
	if _, err := s.RequestReview(ctx, "other", finding.ID, "a", "b", "x"); !errors.Is(err, ErrFindingGone) {
		t.Fatalf("cross-session review: %v", err)
	}
}
