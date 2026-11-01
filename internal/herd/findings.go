package herd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

// handleFinding publishes a claim, or revises one and attaches evidence. A
// finding is not a vote: it stays open and agreement never becomes truth.
func (r *Runner) handleFinding(s store.Session, a store.Agent, reply ollama.Reply, last int64, memory *string) (bool, error) {
	claim := cleanClaim(reply.Text)
	evidence := strings.TrimSpace(reply.Evidence)
	if len(claim) > 2048 {
		return false, &ollama.ValidationError{Reason: "a finding claim must be at most 2048 bytes"}
	}
	if len(evidence) > 4096 {
		return false, &ollama.ValidationError{Reason: "finding evidence must be at most 4096 bytes"}
	}
	stance := strings.ToLower(strings.TrimSpace(reply.Stance))
	if stance == "" {
		stance = "support"
	}
	if stance != "support" && stance != "oppose" {
		return false, &ollama.ValidationError{Reason: `stance must be "support" or "oppose"`}
	}
	ctx := context.Background()
	var finding store.Finding
	var err error
	heading := ""
	if reply.Finding <= 0 {
		if claim == "" {
			return false, &ollama.ValidationError{Reason: "a new finding needs a claim in text"}
		}
		finding, err = r.store.CreateFinding(ctx, s.ID, a.ID, claim, evidence, stance)
		if err != nil {
			return false, err
		}
		heading = fmt.Sprintf("Finding #%d: %s", finding.ID, claim)
	} else {
		if claim == "" && evidence == "" {
			return false, &ollama.ValidationError{Reason: "a revision needs a claim or evidence"}
		}
		finding, err = r.store.ReviseFinding(ctx, s.ID, reply.Finding, a.ID, claim, evidence, stance)
		switch {
		case errors.Is(err, store.ErrFindingGone):
			return false, &ollama.ValidationError{Reason: fmt.Sprintf("finding #%d does not exist in this session", reply.Finding)}
		case err != nil:
			return false, err
		}
		if claim != "" {
			heading = fmt.Sprintf("Finding #%d revised: %s", finding.ID, claim)
		} else {
			heading = fmt.Sprintf("Finding #%d: evidence added.", finding.ID)
		}
	}
	if evidence != "" {
		heading += fmt.Sprintf("\nEvidence (%s): %s", stance, evidence)
	}
	if _, err := r.store.AppendAgentReplyWithMemory(ctx, s.ID, a.ID, "", "finding", heading, reply.Thinking, last, memory); err != nil {
		return false, err
	}
	r.notifyLocked(s.ID)
	return true, nil
}

// findingPrefix is the heading the harness itself adds to a finding message.
// Small models sometimes echo it into the claim, which then reads
// "Finding #4: Finding #4: ..." in the UI and the prompt.
var findingPrefix = regexp.MustCompile(`(?i)^\s*(?:publishing\s+)?finding\s*#?\d*\s*:\s*`)

func cleanClaim(text string) string {
	return strings.TrimSpace(findingPrefix.ReplaceAllString(strings.TrimSpace(text), ""))
}

// handleRetract withdraws a claim and says why in the conversation. Nothing is
// deleted: the claim, its evidence and its positions remain in the record.
func (r *Runner) handleRetract(s store.Session, a store.Agent, reply ollama.Reply, last int64, memory *string) (bool, error) {
	if reply.Finding <= 0 {
		return false, &ollama.ValidationError{Reason: "a retraction needs the finding id"}
	}
	reason := strings.TrimSpace(reply.Text)
	if len(reason) > 2048 {
		return false, &ollama.ValidationError{Reason: "a retraction reason must be at most 2048 bytes"}
	}
	finding, err := r.store.Finding(context.Background(), s.ID, reply.Finding)
	if errors.Is(err, store.ErrFindingGone) {
		return false, &ollama.ValidationError{Reason: fmt.Sprintf("finding #%d does not exist in this session", reply.Finding)}
	}
	if err != nil {
		return false, err
	}
	if finding.AuthorID != a.ID && a.Role != "leader" {
		return false, &ollama.ValidationError{Reason: "only the author or the leader may withdraw a finding"}
	}
	err = r.store.RetractFinding(context.Background(), s.ID, reply.Finding)
	switch {
	case errors.Is(err, store.ErrFindingGone):
		return false, &ollama.ValidationError{Reason: fmt.Sprintf("finding #%d does not exist in this session", reply.Finding)}
	case errors.Is(err, store.ErrFindingWithdrawn):
		return false, &ollama.ValidationError{Reason: fmt.Sprintf("finding #%d is already withdrawn", reply.Finding)}
	case err != nil:
		return false, err
	}
	body := fmt.Sprintf("Finding #%d withdrawn: %s", reply.Finding, reason)
	if _, err := r.store.AppendAgentReplyWithMemory(context.Background(), s.ID, a.ID, "", "retract", body, reply.Thinking, last, memory); err != nil {
		return false, err
	}
	r.notifyLocked(s.ID)
	return true, nil
}

// handleEndorse records agreement or disagreement. The latest position wins;
// earlier ones are not preserved, because a position is not evidence.
func (r *Runner) handleEndorse(s store.Session, a store.Agent, reply ollama.Reply, last int64, memory *string) (bool, error) {
	if reply.Finding <= 0 {
		return false, &ollama.ValidationError{Reason: "an endorsement needs the finding id"}
	}
	choice := strings.ToLower(strings.TrimSpace(reply.Choice))
	if choice != "agree" && choice != "disagree" {
		return false, &ollama.ValidationError{Reason: `choice must be "agree" or "disagree"`}
	}
	text := strings.TrimSpace(reply.Text)
	if len(text) > 2048 {
		return false, &ollama.ValidationError{Reason: "an endorsement reason must be at most 2048 bytes"}
	}
	if err := r.store.EndorseFinding(context.Background(), s.ID, reply.Finding, a.ID, choice, text); err != nil {
		if errors.Is(err, store.ErrFindingGone) {
			return false, &ollama.ValidationError{Reason: fmt.Sprintf("finding #%d does not exist in this session", reply.Finding)}
		}
		return false, err
	}
	body := fmt.Sprintf("Position on finding #%d: %s", reply.Finding, choice)
	if text != "" {
		body += " — " + text
	}
	if _, err := r.store.AppendAgentReplyWithMemory(context.Background(), s.ID, a.ID, "", "endorse", body, reply.Thinking, last, memory); err != nil {
		return false, err
	}
	r.notifyLocked(s.ID)
	return true, nil
}
