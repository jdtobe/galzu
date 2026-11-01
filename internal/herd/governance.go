package herd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jdtobe/galzu/internal/governance"
	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

// voteTimeout bounds a vote that nobody finishes. The harness closes it on
// expiry with whatever participation exists, because an agent that crashed is
// absent, not expelled.
const voteTimeout = 10 * time.Minute

// handleGovernance applies a propose or vote action. The caller holds the
// runner lock. It reports whether a checkpoint was saved, so the caller can
// skip the rest path.
func (r *Runner) handleGovernance(s store.Session, a store.Agent, agents []store.Agent, reply ollama.Reply, last int64, memory *string) (bool, error) {
	ctx := context.Background()
	switch reply.Action {
	case "propose":
		id, err := r.store.OpenProposal(ctx, s.ID, a.ID, reply.Text)
		if err != nil {
			return false, err
		}
		body := fmt.Sprintf("Proposal #%d: %s", id, reply.Text)
		if _, err := r.store.AppendAgentReplyWithMemory(ctx, s.ID, a.ID, "", "", body, reply.Thinking, last, memory); err != nil {
			return false, err
		}
	case "vote":
		choice, ok := governance.NormaliseChoice(reply.Choice)
		if !ok {
			return false, &ollama.ValidationError{Reason: `votes need a choice of "yes" or "no"`}
		}
		err := r.store.CastBallot(ctx, reply.Proposal, a.ID, choice, reply.Text)
		if err == store.ErrProposalClosed {
			return false, &ollama.ValidationError{Reason: fmt.Sprintf("proposal #%d is not open; vote on an open one from your turn packet", reply.Proposal)}
		}
		if err != nil {
			return false, err
		}
		body := fmt.Sprintf("Vote on #%d: %s — %s", reply.Proposal, choice, strings.TrimSpace(reply.Text))
		if _, err := r.store.AppendAgentReplyWithMemory(ctx, s.ID, a.ID, "", "", body, reply.Thinking, last, memory); err != nil {
			return false, err
		}
	default:
		return false, nil
	}
	r.settleProposals(s, agents)
	r.notifyLocked(s.ID)
	return true, nil
}

// settleProposals closes every proposal whose closing condition is met and
// announces the result. Closing is the harness's job: a model cannot award
// itself a result.
func (r *Runner) settleProposals(s store.Session, agents []store.Agent) {
	ctx := context.Background()
	eligible := make([]string, 0, len(agents))
	for _, a := range agents {
		eligible = append(eligible, a.ID)
	}
	open, err := r.store.OpenProposals(ctx, s.ID)
	if err != nil {
		return
	}
	for _, proposal := range open {
		ballots, err := r.store.Ballots(ctx, proposal.ID)
		if err != nil {
			continue
		}
		current := make([]governance.Ballot, 0, len(ballots))
		for _, b := range ballots {
			current = append(current, governance.Ballot{AgentID: b.AgentID, Choice: b.Choice, Text: b.Text})
		}
		expired := time.Since(proposal.CreatedAt) > voteTimeout
		verdict := governance.Decide(eligible, current, expired)
		if !verdict.Decided {
			continue
		}
		status := "failed"
		if verdict.Passed {
			status = "passed"
		}
		result := fmt.Sprintf("%s %d-%d, %d of %d voted", status, verdict.Yes, verdict.No, verdict.Cast, verdict.Eligible)
		if err := r.store.CloseProposal(ctx, proposal.ID, status, result); err != nil {
			continue
		}
		_, _ = r.store.AppendMessage(ctx, s.ID, "",
			fmt.Sprintf("Vote closed: %s — %s.", proposal.Body, result))
	}
}
