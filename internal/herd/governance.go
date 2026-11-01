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

const (
	kindVote       = "vote"
	kindConfidence = "confidence"
	kindElection   = "election"
)

// handleGovernance applies a propose or vote action. The caller holds the
// runner lock. It reports whether a checkpoint was saved, so the caller can
// skip the rest path.
func (r *Runner) handleGovernance(s store.Session, a store.Agent, agents []store.Agent, reply ollama.Reply, last int64, memory *string) (bool, error) {
	ctx := context.Background()
	switch reply.Action {
	case "propose":
		kind := strings.ToLower(strings.TrimSpace(reply.Kind))
		if kind == "" {
			kind = kindVote
		}
		if kind != kindVote && kind != kindConfidence {
			return false, &ollama.ValidationError{Reason: `proposal kind must be "vote" or "confidence"`}
		}
		if kind == kindConfidence {
			// A leader moving no confidence in itself is a resignation, which is
			// a different motion with a different handoff. Refuse it here rather
			// than quietly treating one as the other.
			if leader, err := r.store.Leader(ctx, s.ID); err == nil && leader.ID == a.ID {
				return false, &ollama.ValidationError{Reason: "a leader cannot move no confidence in itself; state your intention in a message instead"}
			}
		}
		id, err := r.store.OpenProposal(ctx, s.ID, a.ID, kind, reply.Text)
		if err != nil {
			return false, err
		}
		label := ""
		if kind == kindConfidence {
			label = " (no confidence)"
		}
		body := fmt.Sprintf("Proposal #%d%s: %s", id, label, reply.Text)
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
// itself a result, and authority moves only here.
func (r *Runner) settleProposals(s store.Session, agents []store.Agent) {
	ctx := context.Background()
	ids := make([]string, 0, len(agents))
	leaderID := ""
	for _, a := range agents {
		ids = append(ids, a.ID)
		if a.Role == "leader" {
			leaderID = a.ID
		}
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

		switch proposal.Kind {
		case kindConfidence:
			// The subject does not vote on their own removal.
			verdict := governance.Decide(without(ids, leaderID), current, expired)
			if !verdict.Decided {
				continue
			}
			status := "failed"
			if verdict.Passed {
				status = "passed"
			}
			if r.store.CloseProposal(ctx, proposal.ID, status, verdictSummary(status, verdict)) != nil {
				continue
			}
			if !verdict.Passed {
				_, _ = r.store.AppendMessage(ctx, s.ID, "",
					fmt.Sprintf("Vote closed: %s — %s.", proposal.Body, verdictSummary(status, verdict)))
				continue
			}
			if err := r.store.SetAgentRole(ctx, leaderID, "helper"); err != nil {
				continue
			}
			electionID, err := r.store.OpenProposal(ctx, s.ID, proposal.AuthorID, kindElection,
				"Who should lead the herd? Vote for one candidate by agent id.")
			if err != nil {
				continue
			}
			_, _ = r.store.AppendMessage(ctx, s.ID, "",
				fmt.Sprintf("Vote closed: %s — %s. The leader has stepped down; election #%d is open.",
					proposal.Body, verdictSummary(status, verdict), electionID))

		case kindElection:
			election := governance.DecideElection(ids, ids, current, expired)
			if !election.Decided {
				continue
			}
			if election.Winner == "" {
				reason := "no majority"
				if election.Tie {
					reason = "tied"
				}
				if r.store.CloseProposal(ctx, proposal.ID, "failed", reason) == nil {
					_, _ = r.store.AppendMessage(ctx, s.ID, "",
						fmt.Sprintf("Election #%d closed: %s. No leader was chosen; propose another election when the herd has a majority.", proposal.ID, reason))
				}
				continue
			}
			// One leader at a time: demote anyone still holding the role, then
			// promote the winner.
			for _, a := range agents {
				if a.Role == "leader" && a.ID != election.Winner {
					_ = r.store.SetAgentRole(ctx, a.ID, "helper")
				}
			}
			if err := r.store.SetAgentRole(ctx, election.Winner, "leader"); err != nil {
				continue
			}
			if r.store.CloseProposal(ctx, proposal.ID, "passed", "elected "+election.Winner) == nil {
				_, _ = r.store.AppendMessage(ctx, s.ID, "",
					fmt.Sprintf("Election #%d closed: %s is the herd's leader.", proposal.ID, election.Winner))
			}

		default:
			verdict := governance.Decide(ids, current, expired)
			if !verdict.Decided {
				continue
			}
			status := "failed"
			if verdict.Passed {
				status = "passed"
			}
			if r.store.CloseProposal(ctx, proposal.ID, status, verdictSummary(status, verdict)) == nil {
				_, _ = r.store.AppendMessage(ctx, s.ID, "",
					fmt.Sprintf("Vote closed: %s — %s.", proposal.Body, verdictSummary(status, verdict)))
			}
		}
	}
}

func verdictSummary(status string, v governance.Verdict) string {
	return fmt.Sprintf("%s %d-%d, %d of %d voted", status, v.Yes, v.No, v.Cast, v.Eligible)
}

// without returns ids minus one, preserving order.
func without(ids []string, drop string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}
