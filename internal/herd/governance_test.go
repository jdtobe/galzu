package herd

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

func openHerd(t *testing.T, agents []store.Agent, concurrency int) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: concurrency}, agents); err != nil {
		t.Fatal(err)
	}
	return s
}

// A full house closes the vote and the harness announces the result, not the
// model that called it.
func TestSettleProposalsClosesWhenEveryoneVoted(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 2)
	id, err := s.OpenProposal(ctx, "s", "a", "vote", "Should we probe port 22 first?")
	if err != nil {
		t.Fatal(err)
	}
	for _, vote := range []struct{ agent, choice string }{{"a", "yes"}, {"b", "no"}} {
		if err := s.CastBallot(ctx, id, vote.agent, vote.choice, "because"); err != nil {
			t.Fatal(err)
		}
	}
	r := New(s, restsAtOnce{})
	defer r.Stop()
	r.settleProposals(store.Session{ID: "s"}, agents)

	proposals, err := s.Proposals(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 1 || proposals[0].Status != "failed" {
		t.Fatalf("tie should fail: %+v", proposals)
	}
	messages, err := s.Messages(ctx, "s", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0].Body, "Vote closed") || !strings.Contains(messages[0].Body, "1-1") {
		t.Fatalf("announcement = %+v", messages)
	}
	if messages[0].AgentID != "" {
		t.Fatal("the announcement must come from the host, not an agent")
	}
}

func TestSettleProposalsPassesOnMajority(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
		{ID: "c", SessionID: "s", Name: "Cy", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 3)
	id, err := s.OpenProposal(ctx, "s", "a", "vote", "Do we need more evidence?")
	if err != nil {
		t.Fatal(err)
	}
	for _, vote := range []struct{ agent, choice string }{{"a", "yes"}, {"b", "yes"}, {"c", "no"}} {
		if err := s.CastBallot(ctx, id, vote.agent, vote.choice, "because"); err != nil {
			t.Fatal(err)
		}
	}
	r := New(s, restsAtOnce{})
	defer r.Stop()
	r.settleProposals(store.Session{ID: "s"}, agents)

	proposals, _ := s.Proposals(ctx, "s")
	if proposals[0].Status != "passed" {
		t.Fatalf("status = %q", proposals[0].Status)
	}
}

func TestSettleProposalsLeavesAnIncompleteVoteOpen(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 2)
	id, err := s.OpenProposal(ctx, "s", "a", "vote", "Question?")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CastBallot(ctx, id, "a", "yes", "because"); err != nil {
		t.Fatal(err)
	}
	r := New(s, restsAtOnce{})
	defer r.Stop()
	r.settleProposals(store.Session{ID: "s"}, agents)

	proposals, _ := s.Proposals(ctx, "s")
	if proposals[0].Status != "open" {
		t.Fatalf("closed early: %q", proposals[0].Status)
	}
}

type proposer struct {
	done chan struct{}
	once sync.Once
}

func (p *proposer) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	p.once.Do(func() { close(p.done) })
	return ollama.Reply{Action: "propose", Text: "Should we probe port 22 first?"}, nil
}

// The propose action reaches the store and the conversation.
func TestAgentCanPropose(t *testing.T) {
	ctx := context.Background()
	s := openHerd(t, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}, 1)
	model := &proposer{done: make(chan struct{})}
	r := New(s, model)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "begin"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case <-model.done:
	case <-time.After(2 * time.Second):
		t.Fatal("model was never called")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		proposals, err := s.Proposals(ctx, "s")
		if err == nil && len(proposals) == 1 {
			if proposals[0].Body != "Should we probe port 22 first?" || proposals[0].Status != "open" {
				t.Fatalf("proposal = %+v", proposals[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no proposal recorded")
		}
		time.Sleep(time.Millisecond)
	}
}

type voter struct {
	id   int64
	vote string
	done chan struct{}
	once sync.Once
}

func (v *voter) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	v.once.Do(func() { close(v.done) })
	return ollama.Reply{Action: "vote", Proposal: v.id, Choice: v.vote, Text: "the evidence is weak"}, nil
}

// A vote reaches the ballot table, carrying the reason as a message.
func TestAgentCanVote(t *testing.T) {
	ctx := context.Background()
	s := openHerd(t, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}, 1)
	id, err := s.OpenProposal(ctx, "s", "a", "vote", "Question?")
	if err != nil {
		t.Fatal(err)
	}
	model := &voter{id: id, vote: "yes", done: make(chan struct{})}
	r := New(s, model)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "begin"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case <-model.done:
	case <-time.After(2 * time.Second):
		t.Fatal("model was never called")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		ballots, err := s.Ballots(ctx, id)
		if err == nil && len(ballots) == 1 {
			if ballots[0].Choice != "yes" || ballots[0].Text != "the evidence is weak" {
				t.Fatalf("ballot = %+v", ballots[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no ballot recorded")
		}
		time.Sleep(time.Millisecond)
	}
	// Alone in the herd, the vote closes immediately and is announced.
	deadline = time.Now().Add(2 * time.Second)
	for {
		proposals, _ := s.Proposals(ctx, "s")
		if proposals[0].Status == "passed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("vote stayed %q", proposals[0].Status)
		}
		time.Sleep(time.Millisecond)
	}
}

func roles(t *testing.T, s *store.Store) map[string]string {
	t.Helper()
	_, agents, err := s.GetSession(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, a := range agents {
		out[a.ID] = a.Role
	}
	return out
}

// A passed no-confidence motion removes the leader, opens an election, and the
// subject does not get a ballot in its own removal.
func TestConfidenceRemovesTheLeaderAndOpensAnElection(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
		{ID: "c", SessionID: "s", Name: "Cy", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 3)
	id, err := s.OpenProposal(ctx, "s", "b", "confidence", "The leader is dismissing evidence")
	if err != nil {
		t.Fatal(err)
	}
	// The subject, a, does not vote. Two helpers is a full ballot of the eligible.
	for _, vote := range []string{"b", "c"} {
		if err := s.CastBallot(ctx, id, vote, "yes", "it keeps happening"); err != nil {
			t.Fatal(err)
		}
	}
	r := New(s, restsAtOnce{})
	defer r.Stop()
	r.settleProposals(store.Session{ID: "s"}, agents)

	roles := roles(t, s)
	if roles["a"] != "helper" {
		t.Fatalf("leader still leads: %+v", roles)
	}
	proposals, err := s.Proposals(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 2 || proposals[1].Kind != "election" || proposals[1].Status != "open" {
		t.Fatalf("expected an open election: %+v", proposals)
	}
	if proposals[0].Status != "passed" {
		t.Fatalf("motion status = %q", proposals[0].Status)
	}
}

// An election moves authority to the winner and leaves exactly one leader.
func TestElectionPromotesTheWinner(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
		{ID: "c", SessionID: "s", Name: "Cy", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 3)
	id, err := s.OpenProposal(ctx, "s", "b", "election", "Who leads?")
	if err != nil {
		t.Fatal(err)
	}
	for agent, candidate := range map[string]string{"a": "b", "b": "b", "c": "b"} {
		if err := s.CastBallot(ctx, id, agent, candidate, "steady"); err != nil {
			t.Fatal(err)
		}
	}
	r := New(s, restsAtOnce{})
	defer r.Stop()
	r.settleProposals(store.Session{ID: "s"}, agents)

	roles := roles(t, s)
	if roles["b"] != "leader" || roles["a"] != "helper" || roles["c"] != "helper" {
		t.Fatalf("roles = %+v", roles)
	}
	proposals, _ := s.Proposals(ctx, "s")
	if proposals[0].Status != "passed" {
		t.Fatalf("election status = %q", proposals[0].Status)
	}
}

// A tie leaves the herd without a leader rather than letting the harness choose.
func TestElectionTieLeavesNoLeader(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 2)
	id, err := s.OpenProposal(ctx, "s", "a", "election", "Who leads?")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CastBallot(ctx, id, "a", "a", "me"); err != nil {
		t.Fatal(err)
	}
	if err := s.CastBallot(ctx, id, "b", "b", "me"); err != nil {
		t.Fatal(err)
	}
	r := New(s, restsAtOnce{})
	defer r.Stop()
	r.settleProposals(store.Session{ID: "s"}, agents)

	if roles := roles(t, s); roles["a"] != "leader" {
		t.Fatalf("a tie installed a leader: %+v", roles)
	}
	proposals, _ := s.Proposals(ctx, "s")
	if proposals[0].Status != "failed" || proposals[0].Result != "tied" {
		t.Fatalf("tie result = %+v", proposals[0])
	}
}

// A leader may not move no confidence in itself; that is a resignation.
func TestLeaderCannotMoveNoConfidenceInItself(t *testing.T) {
	s := openHerd(t, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}, 1)
	r := New(s, restsAtOnce{})
	defer r.Stop()
	leader := store.Agent{ID: "a", SessionID: "s", Role: "leader", Status: "ready"}
	_, err := r.handleGovernance(store.Session{ID: "s"}, leader, []store.Agent{leader},
		ollama.Reply{Action: "propose", Kind: "confidence", Text: "remove me"}, 0, nil)
	var invalid *ollama.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want a validation error", err)
	}
}

type planner struct {
	done chan struct{}
	once sync.Once
}

func (p *planner) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	p.once.Do(func() { close(p.done) })
	return ollama.Reply{Action: "plan", Text: "1. Read the README\nDone when: summary written\nAcceptance: quotes the goal"}, nil
}

func TestAgentCanProposeAPlan(t *testing.T) {
	ctx := context.Background()
	s := openHerd(t, []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}, 1)
	model := &planner{done: make(chan struct{})}
	r := New(s, model)
	defer r.Stop()
	if err := r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "begin"); err != nil {
		t.Fatal(err)
	}
	r.Notify("s")
	select {
	case <-model.done:
	case <-time.After(2 * time.Second):
		t.Fatal("model was never called")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		plan, ok, err := s.CurrentPlan(ctx, "s")
		if err == nil && ok && plan.Status == "pending" {
			if plan.Version != 1 {
				t.Fatalf("version = %d", plan.Version)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no plan recorded")
		}
		time.Sleep(time.Millisecond)
	}
}
