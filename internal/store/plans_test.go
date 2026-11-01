package store

import (
	"context"
	"errors"
	"testing"
)

func TestProposePlanVersionsAndSupersedes(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	first, err := s.ProposePlan(ctx, "s", "a", "steps: one")
	if err != nil || first.Version != 1 || first.Status != "pending" {
		t.Fatalf("%+v %v", first, err)
	}
	second, err := s.ProposePlan(ctx, "s", "b", "steps: two")
	if err != nil || second.Version != 2 {
		t.Fatalf("%+v %v", second, err)
	}
	versions, err := s.Plans(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Status != "superseded" || versions[1].Status != "pending" {
		t.Fatalf("versions = %+v", versions)
	}
}

// Approval survives a later proposal: the herd always has a plan or none.
func TestApprovedPlanSurvivesANewProposal(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	if _, err := s.ProposePlan(ctx, "s", "a", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.DecidePlan(ctx, "s", 1, true, "go", "human"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProposePlan(ctx, "s", "a", "v2"); err != nil {
		t.Fatal(err)
	}
	current, ok, err := s.CurrentPlan(ctx, "s")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if current.Version != 1 || current.Status != "approved" {
		t.Fatalf("current = %+v", current)
	}
}

func TestDecidePlanRefusesNonPending(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	if _, err := s.ProposePlan(ctx, "s", "a", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.DecidePlan(ctx, "s", 1, true, "", "human"); err != nil {
		t.Fatal(err)
	}
	if err := s.DecidePlan(ctx, "s", 1, true, "", "human"); !errors.Is(err, ErrPlanNotPending) {
		t.Fatalf("err = %v", err)
	}
	if err := s.DecidePlan(ctx, "s", 9, true, "", "human"); !errors.Is(err, ErrPlanNotPending) {
		t.Fatalf("err = %v", err)
	}
}

func TestCurrentPlanIsEmptyBeforeAnyProposal(t *testing.T) {
	if _, ok, err := openTestStore(t).CurrentPlan(context.Background(), "s"); ok || err != nil {
		t.Fatalf("ok %v err %v", ok, err)
	}
}

func TestCompletePlanClosesTheLoop(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	if _, err := s.ProposePlan(ctx, "s", "a", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.CompletePlan(ctx, "s", 1, "human"); !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("completed a pending plan: %v", err)
	}
	if err := s.DecidePlan(ctx, "s", 1, true, "go", "human"); err != nil {
		t.Fatal(err)
	}
	if err := s.CompletePlan(ctx, "s", 1, "human"); err != nil {
		t.Fatal(err)
	}
	current, ok, err := s.CurrentPlan(ctx, "s")
	if err != nil || !ok || current.Status != "done" || current.Version != 1 {
		t.Fatalf("current = %+v ok %v err %v", current, ok, err)
	}
	if err := s.CompletePlan(ctx, "s", 1, "human"); !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("completed twice: %v", err)
	}
	// A later proposal is current again; the done plan stays as history.
	second, err := s.ProposePlan(ctx, "s", "a", "v2")
	if err != nil {
		t.Fatal(err)
	}
	current, ok, err = s.CurrentPlan(ctx, "s")
	if err != nil || !ok || current.Version != second.Version || current.Status != "pending" {
		t.Fatalf("current = %+v ok %v err %v", current, ok, err)
	}
}
