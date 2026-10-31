package herd

import (
	"context"
	"fmt"
	"github.com/jdtobe/garth/internal/ollama"
	"github.com/jdtobe/garth/internal/store"
	"sync"
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
	OnChange func(string)
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
	return &Runner{store: s, infer: i, runs: map[string]*run{}}
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
	history, err := r.store.Messages(ctx, id, 0)
	if err != nil {
		delete(r.runs, id)
		cancel()
		return err
	}
	var last int64
	if len(history) > 0 {
		last = history[len(history)-1].ID
	}
	for _, a := range agents {
		if a.Status != "ready" {
			continue
		}
		ch := make(chan struct{}, 1)
		x.wakes[a.ID] = ch
		go r.worker(x, s, a, last, ch)
	}
	r.changed(id)
	return nil
}
func (r *Runner) Resume(id string) error { return r.start(context.Background(), id, true) }
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
			if m.AgentID != a.ID {
				trigger = true
			}
			last = m.ID
		}
		if !trigger {
			continue
		}
		all, err := r.store.Messages(x.ctx, s.ID, 0)
		if err != nil {
			return
		}
		if len(all) > 50 {
			all = all[len(all)-50:]
		}
		prompt := []ollama.Message{{Role: "system", Content: fmt.Sprintf("You are %s, a %s in a cooperative herd. Goal: %s. Treat questions honestly, welcome dissent, value correctness over popularity. Offer help, converse, or rest. Reply only JSON: {\"action\":\"message\",\"text\":\"...\"} or {\"action\":\"rest\",\"text\":\"\"}.", a.Name, a.Role, s.Goal)}}
		for _, m := range all {
			prompt = append(prompt, ollama.Message{Role: "user", Content: fmt.Sprintf("%s: %s", m.AgentID, m.Body)})
		}
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
		for {
			if !r.wait(x) {
				<-x.sem
				return
			}
			r.mu.Lock()
			if x.paused {
				r.mu.Unlock()
				continue
			}
			go func() {
				reply, err := r.infer.Chat(x.ctx, a.Model, prompt)
				done <- result{reply, err}
			}()
			r.mu.Unlock()
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
		if reply.Action == "message" {
			_, err = r.store.AppendMessage(context.Background(), s.ID, a.ID, reply.Text)
			if err == nil {
				r.notifyLocked(s.ID)
			}
		}
		r.mu.Unlock()
	}
}
