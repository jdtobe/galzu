package herd

import (
	"context"
	"errors"
	"fmt"
	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
	"math/rand"
	"strings"
	"sync"
	"time"
)

type Inference interface {
	Chat(context.Context, string, []ollama.Message) (ollama.Reply, error)
}
type Runner struct {
	store    *store.Store
	infer    Inference
	mu       sync.Mutex
	runs     map[string]*run
	stopped  bool
	activity map[string]map[string]Activity
	prompts  map[string][]byte
	OnChange func(string)
}
type Activity struct {
	State           string `json:"state"`
	QueueMS         int64  `json:"queue_ms"`
	InferenceMS     int64  `json:"inference_ms"`
	PromptBytes     int    `json:"prompt_bytes"`
	FixedBytes      int    `json:"fixed_bytes,omitempty"`
	ReusedBytes     int    `json:"reused_bytes,omitempty"`
	ReusePercent    int    `json:"reuse_percent,omitempty"`
	PromptEvalCount int    `json:"prompt_eval_count,omitempty"`
	PromptEvalMS    int64  `json:"prompt_eval_ms,omitempty"`
	LastOutcome     string `json:"last_outcome,omitempty"`
	CompletedAt     string `json:"completed_at,omitempty"`
}

func (r *Runner) Activity(id string) map[string]Activity {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]Activity)
	for agent, activity := range r.activity[id] {
		out[agent] = activity
	}
	return out
}
func (r *Runner) setActivity(session, agent string, activity Activity) {
	r.mu.Lock()
	if r.runs[session] == nil {
		r.mu.Unlock()
		return
	}
	if r.activity[session] == nil {
		r.activity[session] = make(map[string]Activity)
	}
	r.activity[session][agent] = activity
	r.mu.Unlock()
	r.changed(session)
}

type run struct {
	ctx          context.Context
	cancel       context.CancelFunc
	paused       bool
	resume       chan struct{}
	wakes        map[string]chan struct{}
	retries      map[string]chan struct{}
	sem          chan struct{}
	storageError string
}

func New(s *store.Store, i Inference) *Runner {
	return &Runner{store: s, infer: i, runs: map[string]*run{}, activity: map[string]map[string]Activity{}, prompts: map[string][]byte{}}
}
func (r *Runner) changed(id string) {
	if r.OnChange != nil {
		r.OnChange(id)
	}
}
func (r *Runner) Start(ctx context.Context, id string) error { return r.start(ctx, id, false) }
func (r *Runner) start(ctx context.Context, id string, resume bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return fmt.Errorf("runner stopped")
	}
	if x := r.runs[id]; x != nil {
		if resume && x.paused {
			if x.storageError != "" {
				return fmt.Errorf("storage recovery required")
			}
			if err := r.store.SetSessionStatus(ctx, id, "running"); err != nil {
				return err
			}
			x.paused = false
			close(x.resume)
			r.changed(id)
		}
		return nil
	}
	s, agents, err := r.store.GetSession(ctx, id)
	if err != nil {
		return err
	}
	if err = r.store.InterruptAttempts(ctx, id); err != nil {
		return err
	}
	if resume {
		if err = r.store.SetSessionStatus(ctx, id, "running"); err != nil {
			return err
		}
		s.Status = "running"
	}
	c, cancel := context.WithCancel(context.Background())
	x := &run{ctx: c, cancel: cancel, paused: s.Status != "running", wakes: map[string]chan struct{}{}, retries: map[string]chan struct{}{}, sem: make(chan struct{}, s.Concurrency)}
	if x.paused {
		x.resume = make(chan struct{})
	}
	r.runs[id] = x
	checkpoints := make(map[string]int64, len(agents))
	for _, a := range agents {
		if a.Status != "ready" {
			continue
		}
		last, err := r.store.Checkpoint(ctx, id, a.ID)
		if err != nil {
			delete(r.runs, id)
			cancel()
			return err
		}
		checkpoints[a.ID] = last
	}
	for _, a := range agents {
		if a.Status != "ready" {
			continue
		}
		ch := make(chan struct{}, 1)
		x.wakes[a.ID] = ch
		x.retries[a.ID] = make(chan struct{}, 1)
		go r.worker(x, s, a, checkpoints[a.ID], ch)
		ch <- struct{}{}
	}
	r.changed(id)
	return nil
}

func (r *Runner) StorageError(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x := r.runs[id]; x != nil {
		return x.storageError
	}
	return ""
}
func (r *Runner) storageFailureLocked(id string, x *run, err error) {
	if x.ctx.Err() != nil {
		return
	}
	x.storageError = "Storage operation failed; session stopped. Check storage and retry recovery."
	if !x.paused {
		x.paused = true
		x.resume = make(chan struct{})
	}
	r.changed(id)
}
func (r *Runner) StorageFailure(id string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x := r.runs[id]; x != nil {
		r.storageFailureLocked(id, x, err)
	}
}
func (r *Runner) RecoverStorage(id string) error {
	r.mu.Lock()
	x := r.runs[id]
	if x == nil || x.storageError == "" {
		r.mu.Unlock()
		return fmt.Errorf("no storage failure")
	}
	if err := r.store.ProbeWrite(context.Background()); err != nil {
		r.mu.Unlock()
		return fmt.Errorf("storage write check failed")
	}
	if err := r.store.SetSessionStatus(context.Background(), id, "paused"); err != nil {
		r.mu.Unlock()
		return fmt.Errorf("storage write check failed")
	}
	x.cancel()
	delete(r.runs, id)
	r.mu.Unlock()
	return r.Start(context.Background(), id)
}
func (r *Runner) Resume(id string) error { return r.start(context.Background(), id, true) }

func (r *Runner) DeleteSession(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.runs[id]
	if x != nil && !x.paused {
		return store.ErrSessionNotPaused
	}
	// Keep workers intact if the transaction fails. The lock also excludes resume
	// and reply commits until deletion has either committed or rolled back.
	if err := r.store.DeleteSession(ctx, id); err != nil {
		return err
	}
	if x != nil {
		x.cancel()
	}
	delete(r.runs, id)
	delete(r.activity, id)
	for key := range r.prompts {
		if strings.HasPrefix(key, id+"\x00") {
			delete(r.prompts, key)
		}
	}
	r.changed(id)
	return nil
}

// RetryAgent restarts a failed worker from its last committed checkpoint.
func (r *Runner) RetryAgent(sessionID, agentID string) error {
	if err := r.Start(context.Background(), sessionID); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.runs[sessionID]
	if x == nil || r.stopped {
		return fmt.Errorf("session is not active")
	}
	s, agents, err := r.store.GetSession(context.Background(), sessionID)
	if err != nil {
		return err
	}
	for _, a := range agents {
		if a.ID == agentID {
			return r.retryLocked(sessionID, x, s, a)
		}
	}
	return fmt.Errorf("agent not found")
}

// RetryFailed restarts every agent left in error and reports how many it
// restarted. Agents already waiting on an automatic retry are left to that
// schedule; the per-agent control is what retries those immediately.
func (r *Runner) RetryFailed(sessionID string) (int, error) {
	if err := r.Start(context.Background(), sessionID); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.runs[sessionID]
	if x == nil || r.stopped {
		return 0, fmt.Errorf("session is not active")
	}
	s, agents, err := r.store.GetSession(context.Background(), sessionID)
	if err != nil {
		return 0, err
	}
	retried := 0
	for _, a := range agents {
		if a.Status != "error" {
			continue
		}
		if err := r.retryLocked(sessionID, x, s, a); err != nil {
			return retried, err
		}
		retried++
	}
	return retried, nil
}

// retryLocked does the work for one agent. The caller holds r.mu.
func (r *Runner) retryLocked(sessionID string, x *run, s store.Session, a store.Agent) error {
	if a.Status == "ready" {
		if r.activity[sessionID][a.ID].State != "retrying" {
			return fmt.Errorf("agent is not waiting to retry")
		}
		select {
		case x.retries[a.ID] <- struct{}{}:
		default:
		}
		return nil
	}
	if a.Status != "error" {
		return fmt.Errorf("agent is not in error")
	}
	last, err := r.store.Checkpoint(context.Background(), sessionID, a.ID)
	if err != nil {
		return err
	}
	if err = r.store.SetAgentStatus(context.Background(), a.ID, "ready", ""); err != nil {
		return err
	}
	if err = r.store.ClearRecovery(context.Background(), a.ID); err != nil {
		return err
	}
	a.Status, a.Error = "ready", ""
	ch := make(chan struct{}, 1)
	x.wakes[a.ID] = ch
	x.retries[a.ID] = make(chan struct{}, 1)
	go r.worker(x, s, a, last, ch)
	ch <- struct{}{}
	r.changed(sessionID)
	return nil
}

func (r *Runner) Pause(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.runs[id]
	if x == nil {
		return r.store.SetSessionStatus(context.Background(), id, "paused")
	}
	if x.paused {
		return nil
	}
	if err := r.store.SetSessionStatus(context.Background(), id, "paused"); err != nil {
		return err
	}
	x.paused = true
	x.resume = make(chan struct{})
	r.changed(id)
	return nil
}
func (r *Runner) Notify(id string) { r.mu.Lock(); defer r.mu.Unlock(); r.notifyLocked(id) }
func (r *Runner) notifyLocked(id string) {
	if x := r.runs[id]; x != nil {
		for _, ch := range x.wakes {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
	r.changed(id)
}
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	for _, x := range r.runs {
		x.cancel()
	}
}
func (r *Runner) wait(x *run) bool {
	for {
		r.mu.Lock()
		if !x.paused {
			r.mu.Unlock()
			return true
		}
		ch := x.resume
		r.mu.Unlock()
		select {
		case <-ch:
		case <-x.ctx.Done():
			return false
		}
	}
}
func (r *Runner) worker(x *run, s store.Session, a store.Agent, last int64, wake <-chan struct{}) {
	r.mu.Lock()
	retry := x.retries[a.ID]
	r.mu.Unlock()
	for {
		select {
		case <-x.ctx.Done():
			return
		case <-wake:
		}
		if !r.wait(x) {
			return
		}
		recovery, err := r.store.Recovery(x.ctx, a.ID)
		if err != nil {
			r.StorageFailure(s.ID, err)
			return
		}
		if recovery.Failures > 0 && !recovery.NextAt.IsZero() {
			r.setActivity(s.ID, a.ID, Activity{State: "retrying"})
			delay := time.Until(recovery.NextAt)
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-retry:
				case <-x.ctx.Done():
					timer.Stop()
					return
				}
				timer.Stop()
			}
			if !r.wait(x) {
				return
			}
		}
		base := last
		messages, err := r.store.Messages(x.ctx, s.ID, last)
		if err != nil {
			r.StorageFailure(s.ID, err)
			return
		}
		batchSize := 50
		if recovery.Category == "context" {
			for i := 0; i < recovery.Failures && batchSize > 1; i++ {
				batchSize /= 2
			}
		}
		more := len(messages) > batchSize
		if more {
			messages = messages[:batchSize]
		}
		var trigger bool
		for _, m := range messages {
			if m.AgentID != a.ID && (m.RecipientID == "" || m.RecipientID == a.ID) {
				trigger = true
			}
			last = m.ID
		}
		if !trigger {
			if err := r.store.SetCheckpoint(x.ctx, s.ID, a.ID, last); err != nil {
				r.StorageFailure(s.ID, err)
				return
			}
			if more {
				r.Notify(s.ID)
			}
			continue
		}
		all, err := r.store.Messages(x.ctx, s.ID, 0)
		if err != nil {
			r.StorageFailure(s.ID, err)
			return
		}
		// Do not include messages that arrived after this batch was selected.
		end := 0
		for end < len(all) && all[end].ID <= last {
			end++
		}
		all = all[:end]
		memory, _, err := r.store.AgentMemory(x.ctx, s.ID, a.ID)
		if err != nil {
			r.StorageFailure(s.ID, err)
			return
		}
		_, agents, err := r.store.GetSession(x.ctx, s.ID)
		if err != nil {
			r.StorageFailure(s.ID, err)
			return
		}
		cut, err := r.store.PromptCut(x.ctx, a.ID)
		if err != nil {
			r.StorageFailure(s.ID, err)
			return
		}
		entries := afterCut(all, cut)
		render := func() prompt {
			plan, hasPlan, _ := r.store.CurrentPlan(x.ctx, s.ID)
			var current *store.Plan
			if hasPlan {
				current = &plan
			}
			proposals, _ := r.store.OpenProposals(x.ctx, s.ID)
			ballots := make(map[int64][]store.Ballot, len(proposals))
			for _, proposal := range proposals {
				if cast, err := r.store.Ballots(x.ctx, proposal.ID); err == nil {
					ballots[proposal.ID] = cast
				}
			}
			return renderPrompt(promptInput{
				Session:   s,
				Agent:     a,
				Agents:    agents,
				Activity:  r.Activity(s.ID),
				Memory:    memory,
				Entries:   entries,
				Cut:       cut,
				Recovery:  recovery,
				Proposals: proposals,
				Ballots:   ballots,
				Plan:      current,
			})
		}
		rendered := render()
		if rendered.FixedBytes() > promptBudgetBytes || recovery.Category == "context" {
			target := promptTargetBytes
			if recovery.Category == "context" {
				// Each failed attempt keeps only half as much history, so a
				// mis-estimated budget converges instead of retrying the same
				// oversized prompt.
				shift := recovery.Failures
				if shift > 4 {
					shift = 4
				}
				target >>= shift
			}
			if next := chooseCut(entries, a.ID, target); next > cut {
				if err := r.store.SetPromptCut(x.ctx, a.ID, next); err != nil {
					r.StorageFailure(s.ID, err)
					return
				}
				cut = next
				entries = afterCut(entries, next)
				rendered = render()
			}
		}
		prompt := rendered.Messages()
		current := flatten(prompt)
		key := s.ID + "\x00" + a.ID
		r.mu.Lock()
		reused := promptReuse(r.prompts[key], current)
		r.prompts[key] = current
		r.mu.Unlock()
		activity := Activity{State: "queued", PromptBytes: rendered.Bytes(), FixedBytes: rendered.FixedBytes(), ReusedBytes: reused}
		if activity.PromptBytes > 0 {
			activity.ReusePercent = reused * 100 / activity.PromptBytes
		}
		queued := time.Now()
		r.setActivity(s.ID, a.ID, activity)
		select {
		case x.sem <- struct{}{}:
		case <-x.ctx.Done():
			return
		}
		// Serialize starting an inference with pause: once scheduled here, it is
		// an in-flight operation whose result must wait for resume.
		type result struct {
			reply ollama.Reply
			err   error
		}
		done := make(chan result, 1)
		var started time.Time
		for {
			if !r.wait(x) {
				<-x.sem
				return
			}
			_, currentAgents, err := r.store.GetSession(x.ctx, s.ID)
			if err != nil {
				<-x.sem
				r.StorageFailure(s.ID, err)
				return
			}
			thinking := false
			for _, current := range currentAgents {
				if current.ID == a.ID {
					thinking = current.Thinking
					break
				}
			}
			r.mu.Lock()
			if x.paused {
				r.mu.Unlock()
				continue
			}
			if err := r.store.BeginAttempt(x.ctx, s.ID, a.ID); err != nil {
				r.storageFailureLocked(s.ID, x, err)
				r.mu.Unlock()
				<-x.sem
				return
			}
			activity.State = "inference"
			activity.QueueMS = time.Since(queued).Milliseconds()
			started = time.Now()
			if r.activity[s.ID] == nil {
				r.activity[s.ID] = make(map[string]Activity)
			}
			r.activity[s.ID][a.ID] = activity
			go func() {
				var reply ollama.Reply
				var err error
				attemptCtx, cancel := context.WithTimeout(x.ctx, time.Duration(s.InferenceTimeoutMS)*time.Millisecond)
				defer cancel()
				if client, ok := r.infer.(interface {
					ChatWithThinking(context.Context, string, []ollama.Message, bool) (ollama.Reply, error)
				}); ok {
					reply, err = client.ChatWithThinking(attemptCtx, a.Model, prompt, thinking)
				} else {
					reply, err = r.infer.Chat(attemptCtx, a.Model, prompt)
				}
				if !thinking {
					reply.Thinking = ""
				}
				done <- result{reply, err}
			}()
			r.mu.Unlock()
			r.changed(s.ID)
			break
		}
		var outcome result
		select {
		case outcome = <-done:
		case <-x.ctx.Done():
			<-x.sem
			return
		}
		reply, err := outcome.reply, outcome.err
		<-x.sem
		activity.State = "idle"
		activity.PromptEvalCount = reply.PromptEvalCount
		activity.PromptEvalMS = reply.PromptEvalDuration / int64(time.Millisecond)
		activity.InferenceMS = time.Since(started).Milliseconds()
		r.setActivity(s.ID, a.ID, activity)
		if x.ctx.Err() != nil {
			return
		}
		if !r.wait(x) {
			return
		}
		r.mu.Lock()
		for x.paused {
			ch := x.resume
			r.mu.Unlock()
			select {
			case <-ch:
			case <-x.ctx.Done():
				return
			}
			r.mu.Lock()
		}
		if err == nil && (reply.Action == "direct" || reply.Action == "help") && reply.Recipient != "" {
			valid := false
			for _, peer := range agents {
				if peer.ID == reply.Recipient && peer.ID != a.ID && peer.Status == "ready" {
					valid = true
				}
			}
			if !valid {
				err = &ollama.ValidationError{Reason: "recipient is not an available peer in the roster"}
			}
		}
		if err == nil && reply.Action == "direct" && reply.Recipient == "" {
			err = &ollama.ValidationError{Reason: "direct messages require a recipient ID"}
		}
		if err != nil {
			category, reason, excerpt := "inference", err.Error(), ""
			var invalid *ollama.ValidationError
			if errors.As(err, &invalid) {
				category, reason, excerpt = "validation", invalid.Reason, invalid.Excerpt
			}
			var contextLimit *ollama.ContextLimitError
			if errors.As(err, &contextLimit) {
				category = "context"
			}
			failures := recovery.Failures + 1
			var next time.Time
			if failures <= len(s.RetryDelaysMS) {
				delay := time.Duration(s.RetryDelaysMS[failures-1]) * time.Millisecond
				next = time.Now().Add(delay + time.Duration(rand.Int63n(int64(delay/5)+1)))
			}
			if saveErr := r.store.RecordFailure(context.Background(), s.ID, a.ID, category, reason, excerpt, next, failures); saveErr != nil {
				r.storageFailureLocked(s.ID, x, saveErr)
				r.mu.Unlock()
				return
			}
			last = base
			if !next.IsZero() {
				r.activity[s.ID][a.ID] = Activity{State: "retrying"}
			}
			r.notifyLocked(s.ID)
			r.mu.Unlock()
			if next.IsZero() {
				return
			}
			continue
		}
		checkpointSaved := false
		updatedMemory := reply.Memory
		if updatedMemory != nil && (strings.TrimSpace(*updatedMemory) == "" || len(*updatedMemory) > store.MaxAgentMemoryBytes) {
			updatedMemory = nil
		}
		if reply.Action == "message" || reply.Action == "direct" || reply.Action == "help" {
			recipient, kind := "", ""
			if reply.Action == "direct" || reply.Action == "help" {
				recipient, kind = reply.Recipient, reply.Action
			}
			valid := recipient == "" && kind != "direct"
			for _, peer := range agents {
				if peer.ID == recipient && peer.ID != a.ID && peer.Status == "ready" {
					valid = true
				}
			}
			if valid {
				_, err = r.store.AppendAgentReplyWithMemory(context.Background(), s.ID, a.ID, recipient, kind, reply.Text, reply.Thinking, last, updatedMemory)
				if err == nil {
					checkpointSaved = true
					r.notifyLocked(s.ID)
				}
			}
		}
		if err == nil && (reply.Action == "propose" || reply.Action == "vote") {
			checkpointSaved, err = r.handleGovernance(s, a, agents, reply, last, updatedMemory)
		}
		if err == nil && reply.Action == "plan" {
			checkpointSaved, err = r.handlePlan(s, a, reply, last, updatedMemory)
		}
		if err == nil && !checkpointSaved {
			err = r.store.CompleteRestWithMemory(context.Background(), s.ID, a.ID, last, updatedMemory)
		}
		if err == nil {
			activity.LastOutcome = "rested"
			if checkpointSaved {
				activity.LastOutcome = "responded"
			}
			activity.CompletedAt = time.Now().UTC().Format(time.RFC3339)
			r.activity[s.ID][a.ID] = activity
			r.changed(s.ID)
		}
		if err == nil && more {
			r.notifyLocked(s.ID)
		}
		if err != nil {
			r.storageFailureLocked(s.ID, x, err)
		}
		r.mu.Unlock()
		if err != nil {
			return
		}
	}
}
