package herd

import (
	"context"
	"fmt"
	"github.com/jdtobe/garth/internal/ollama"
	"github.com/jdtobe/garth/internal/store"
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
	OnChange func(string)
}
type Activity struct {
	State       string `json:"state"`
	QueueMS     int64  `json:"queue_ms"`
	InferenceMS int64  `json:"inference_ms"`
	PromptBytes int    `json:"prompt_bytes"`
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
	if r.activity[session] == nil {
		r.activity[session] = make(map[string]Activity)
	}
	r.activity[session][agent] = activity
	r.mu.Unlock()
	r.changed(session)
}

type run struct {
	ctx    context.Context
	cancel context.CancelFunc
	paused bool
	resume chan struct{}
	wakes  map[string]chan struct{}
	sem    chan struct{}
}

func New(s *store.Store, i Inference) *Runner {
	return &Runner{store: s, infer: i, runs: map[string]*run{}, activity: map[string]map[string]Activity{}}
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
	if resume {
		if err = r.store.SetSessionStatus(ctx, id, "running"); err != nil {
			return err
		}
		s.Status = "running"
	}
	c, cancel := context.WithCancel(context.Background())
	x := &run{ctx: c, cancel: cancel, paused: s.Status != "running", wakes: map[string]chan struct{}{}, sem: make(chan struct{}, s.Concurrency)}
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
		go r.worker(x, s, a, checkpoints[a.ID], ch)
		ch <- struct{}{}
	}
	r.changed(id)
	return nil
}
func (r *Runner) Resume(id string) error { return r.start(context.Background(), id, true) }

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
		if a.ID != agentID {
			continue
		}
		if a.Status != "error" {
			return fmt.Errorf("agent is not in error")
		}
		last, err := r.store.Checkpoint(context.Background(), sessionID, agentID)
		if err != nil {
			return err
		}
		if err = r.store.SetAgentStatus(context.Background(), agentID, "ready", ""); err != nil {
			return err
		}
		a.Status, a.Error = "ready", ""
		ch := make(chan struct{}, 1)
		x.wakes[agentID] = ch
		go r.worker(x, s, a, last, ch)
		ch <- struct{}{}
		r.changed(sessionID)
		return nil
	}
	return fmt.Errorf("agent not found")
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
	for {
		select {
		case <-x.ctx.Done():
			return
		case <-wake:
		}
		if !r.wait(x) {
			return
		}
		messages, err := r.store.Messages(x.ctx, s.ID, last)
		if err != nil {
			return
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
				return
			}
			continue
		}
		all, err := r.store.Messages(x.ctx, s.ID, 0)
		if err != nil {
			return
		}
		if len(all) > 50 {
			all = all[len(all)-50:]
		}
		_, agents, err := r.store.GetSession(x.ctx, s.ID)
		if err != nil {
			return
		}
		var roster strings.Builder
		for _, peer := range agents {
			fmt.Fprintf(&roster, "\n%s | %s | %s | %s", peer.ID, peer.Name, peer.Role, peer.Status)
		}
		prompt := []ollama.Message{{Role: "system", Content: fmt.Sprintf("You are %s (ID %s), a %s in a cooperative herd. Goal: %s. Treat questions honestly, welcome dissent, value correctness over popularity. Roster (ID | name | role | status):%s\nReply only JSON: {\"action\":\"message\",\"text\":\"...\"} for public chat; {\"action\":\"direct\",\"recipient\":\"agent ID\",\"text\":\"...\"} for a private message; {\"action\":\"help\",\"recipient\":\"agent ID or empty\",\"text\":\"...\"} to request help privately or publicly; or {\"action\":\"rest\",\"text\":\"\"}. Only use IDs from the roster. Private messages are visible to the human operator.", a.Name, a.ID, a.Role, s.Goal, roster.String())}}
		for _, m := range all {
			sender := "Human"
			for _, peer := range agents {
				if peer.ID == m.AgentID {
					sender = peer.Name + " (" + peer.ID + ")"
					break
				}
			}
			label := "public"
			if m.RecipientID != "" {
				label = "private to " + m.RecipientID
			}
			if m.Kind == "help" {
				label += " help request"
			}
			if m.RecipientID != "" && m.RecipientID != a.ID && m.AgentID != a.ID {
				prompt = append(prompt, ollama.Message{Role: "user", Content: fmt.Sprintf("Activity notice: %s sent a %s; content is private.", sender, label)})
			} else {
				prompt = append(prompt, ollama.Message{Role: "user", Content: fmt.Sprintf("%s [%s]: %s", sender, label, m.Body)})
			}
		}
		activity := Activity{State: "queued"}
		for _, m := range prompt {
			activity.PromptBytes += len(m.Content)
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
				if client, ok := r.infer.(interface {
					ChatWithThinking(context.Context, string, []ollama.Message, bool) (ollama.Reply, error)
				}); ok {
					reply, err = client.ChatWithThinking(x.ctx, a.Model, prompt, thinking)
				} else {
					reply, err = r.infer.Chat(x.ctx, a.Model, prompt)
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
		if err != nil {
			_ = r.store.SetAgentStatus(context.Background(), a.ID, "error", err.Error())
			r.notifyLocked(s.ID)
			r.mu.Unlock()
			return
		}
		checkpointSaved := false
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
				_, err = r.store.AppendAgentReply(context.Background(), s.ID, a.ID, recipient, kind, reply.Text, reply.Thinking, last)
				if err == nil {
					checkpointSaved = true
					r.notifyLocked(s.ID)
				}
			}
		}
		if err == nil && !checkpointSaved {
			err = r.store.SetCheckpoint(context.Background(), s.ID, a.ID, last)
		}
		r.mu.Unlock()
		if err != nil {
			return
		}
	}
}
