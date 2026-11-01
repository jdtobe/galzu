package herd

import (
	"context"
	"strings"
	"testing"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

func findingHerd(t *testing.T) (*store.Store, store.Session, []store.Agent) {
	t.Helper()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 2)
	return s, store.Session{ID: "s"}, agents
}

func TestFindingPublishesClaimAndEvidence(t *testing.T) {
	ctx := context.Background()
	s, session, agents := findingHerd(t)
	r := New(s, restsAtOnce{})
	defer r.Stop()

	saved, err := r.handleFinding(session, agents[0], ollama.Reply{
		Action: "finding", Text: "Renaming does not invalidate the prefix.", Evidence: "The name is in the turn packet.", Stance: "support",
	}, 0, nil)
	if err != nil || !saved {
		t.Fatalf("saved %v err %v", saved, err)
	}
	findings, err := s.Findings(ctx, "s", 0)
	if err != nil || len(findings) != 1 {
		t.Fatalf("findings = %+v err %v", findings, err)
	}
	if findings[0].Claim != "Renaming does not invalidate the prefix." || len(findings[0].Evidence) != 1 {
		t.Fatalf("finding = %+v", findings[0])
	}
	messages, err := s.Messages(ctx, "s", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Kind != "finding" || !strings.Contains(messages[0].Body, "Finding #1") {
		t.Fatalf("messages = %+v", messages)
	}
}

func TestFindingRevisionPreservesEarlierClaim(t *testing.T) {
	ctx := context.Background()
	s, session, agents := findingHerd(t)
	r := New(s, restsAtOnce{})
	defer r.Stop()

	first, err := r.handleFinding(session, agents[0], ollama.Reply{Action: "finding", Text: "Claim v1"}, 0, nil)
	if err != nil || !first {
		t.Fatalf("first %v %v", first, err)
	}
	findings, _ := s.Findings(ctx, "s", 0)
	revised, err := r.handleFinding(session, agents[1], ollama.Reply{
		Action: "finding", Finding: findings[0].ID, Text: "Claim v2", Evidence: "Counterexample.", Stance: "oppose",
	}, 0, nil)
	if err != nil || !revised {
		t.Fatalf("revised %v %v", revised, err)
	}
	findings, _ = s.Findings(ctx, "s", 0)
	if findings[0].Claim != "Claim v2" || len(findings[0].Revisions) != 2 || findings[0].Revisions[0].Claim != "Claim v1" {
		t.Fatalf("finding = %+v", findings[0])
	}
	if findings[0].Evidence[0].Stance != "oppose" {
		t.Fatalf("stance = %+v", findings[0].Evidence[0])
	}
}

func TestEndorseRecordsPositionAndRejectsUnknownFinding(t *testing.T) {
	ctx := context.Background()
	s, session, agents := findingHerd(t)
	r := New(s, restsAtOnce{})
	defer r.Stop()

	if _, err := r.handleFinding(session, agents[0], ollama.Reply{Action: "finding", Text: "A claim"}, 0, nil); err != nil {
		t.Fatal(err)
	}
	findings, _ := s.Findings(ctx, "s", 0)
	saved, err := r.handleEndorse(session, agents[1], ollama.Reply{
		Action: "endorse", Finding: findings[0].ID, Choice: "agree", Text: "I checked it.",
	}, 0, nil)
	if err != nil || !saved {
		t.Fatalf("saved %v err %v", saved, err)
	}
	findings, _ = s.Findings(ctx, "s", 0)
	if len(findings[0].Positions) != 1 || findings[0].Positions[0].Choice != "agree" {
		t.Fatalf("positions = %+v", findings[0].Positions)
	}
	if _, err := r.handleEndorse(session, agents[1], ollama.Reply{Action: "endorse", Finding: 999, Choice: "agree", Text: "x"}, 0, nil); err == nil {
		t.Fatal("endorsed a finding that does not exist")
	}
}

func TestEchoedFindingHeadingIsRemovedFromTheClaim(t *testing.T) {
	if got := cleanClaim("Finding #4: The cache survives a rename."); got != "The cache survives a rename." {
		t.Fatalf("got %q", got)
	}
	if got := cleanClaim("Publishing Finding #12: Claim text."); got != "Claim text." {
		t.Fatalf("got %q", got)
	}
	if got := cleanClaim("Candidate A: a claim"); got != "Candidate A: a claim" {
		t.Fatalf("removed a meaningful prefix: %q", got)
	}
}
