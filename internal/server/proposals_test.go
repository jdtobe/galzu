package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jdtobe/galzu/internal/store"
)

func TestProposalsEndpointShowsBallots(t *testing.T) {
	ctx := context.Background()
	h, s := newScopeServer(t, "s")
	id, err := s.OpenProposal(ctx, "s", "a", "vote", "Should we probe port 22 first?")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CastBallot(ctx, id, "a", "yes", "the banner is old"); err != nil {
		t.Fatal(err)
	}
	w := do(t, h, "GET", "/api/sessions/s/proposals", "")
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Proposals []struct {
			store.Proposal
			Ballots []store.Ballot `json:"ballots"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Proposals) != 1 {
		t.Fatalf("got %d proposals", len(result.Proposals))
	}
	p := result.Proposals[0]
	if p.Status != "open" || p.Body != "Should we probe port 22 first?" {
		t.Fatalf("proposal = %+v", p.Proposal)
	}
	if len(p.Ballots) != 1 || p.Ballots[0].Choice != "yes" || p.Ballots[0].Text != "the banner is old" {
		t.Fatalf("ballots = %+v", p.Ballots)
	}
}
