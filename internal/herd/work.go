package herd

import (
	"context"
	"fmt"
	"time"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

// workTimeout is how long an agent's offer of capacity waits for the fabric
// before the leader is asked directly.
var workTimeout = 2 * time.Minute

// handleAvailable records that an agent has capacity. Asking the herd comes
// first; the leader is the fallback, and settleWork decides when to use it.
func (r *Runner) handleAvailable(s store.Session, a store.Agent, reply ollama.Reply, last int64, memory *string) (bool, error) {
	ctx := context.Background()
	message, err := r.store.AppendAgentReplyWithMemory(ctx, s.ID, a.ID, "", "available", reply.Text, reply.Thinking, last, memory)
	if err != nil {
		return false, err
	}
	if err := r.store.OpenWorkRequest(ctx, s.ID, a.ID, message.ID); err != nil {
		return false, err
	}
	r.notifyLocked(s.ID)
	return true, nil
}

// settleWork resolves offers the fabric answered and escalates the ones it
// ignored. The caller holds the runner lock.
func (r *Runner) settleWork(s store.Session, agents []store.Agent) {
	ctx := context.Background()
	open, err := r.store.OpenWorkRequests(ctx, s.ID)
	if err != nil {
		return
	}
	names := make(map[string]string, len(agents))
	leader := ""
	for _, a := range agents {
		names[a.ID] = a.Name
		if a.Role == "leader" {
			leader = a.Name
		}
	}
	for _, request := range open {
		answered, err := r.store.AnsweredAfter(ctx, s.ID, request.MessageID, request.AgentID)
		if err != nil {
			continue
		}
		if answered {
			_ = r.store.ResolveWorkRequest(ctx, request.ID, "resolved")
			continue
		}
		if request.Status != "open" || time.Since(request.CreatedAt) < workTimeout {
			continue
		}
		if err := r.store.EscalateWorkRequest(ctx, request.ID); err != nil {
			continue
		}
		name := names[request.AgentID]
		if name == "" {
			name = request.AgentID
		}
		if leader == "" {
			_, _ = r.store.AppendMessage(ctx, s.ID, "",
				fmt.Sprintf("%s has offered capacity for %d minutes with no answer from the herd. There is no leader to ask.", name, int(workTimeout.Minutes())))
			continue
		}
		_, _ = r.store.AppendMessage(ctx, s.ID, "",
			fmt.Sprintf("%s has offered capacity for %d minutes with no answer from the herd. %s, allocate work or say why.", name, int(workTimeout.Minutes()), leader))
	}
}
