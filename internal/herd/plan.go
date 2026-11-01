package herd

import (
	"context"
	"fmt"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

// handlePlan records a proposed plan version. The human approves it; nothing in
// the harness reads the prose as authority.
func (r *Runner) handlePlan(s store.Session, a store.Agent, reply ollama.Reply, last int64, memory *string) (bool, error) {
	if len(reply.Text) > store.MaxPlanBytes {
		return false, &ollama.ValidationError{Reason: "plan exceeds the size limit; be concise"}
	}
	plan, err := r.store.ProposePlan(context.Background(), s.ID, a.ID, reply.Text)
	if err != nil {
		return false, err
	}
	body := fmt.Sprintf("Plan v%d proposed for review: %s", plan.Version, reply.Text)
	if _, err := r.store.AppendAgentReplyWithMemory(context.Background(), s.ID, a.ID, "", "", body, reply.Thinking, last, memory); err != nil {
		return false, err
	}
	r.notifyLocked(s.ID)
	return true, nil
}
