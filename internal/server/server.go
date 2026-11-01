package server

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jdtobe/galzu/internal/herd"
	"github.com/jdtobe/galzu/internal/store"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var web embed.FS

type server struct {
	s       *store.Store
	r       *herd.Runner
	mu      sync.Mutex
	clients map[string]map[chan struct{}]bool
}

func New(s *store.Store, r *herd.Runner) http.Handler {
	v := &server{s: s, r: r, clients: map[string]map[chan struct{}]bool{}}
	r.OnChange = v.notify
	m := http.NewServeMux()
	m.HandleFunc("POST /api/sessions", v.create)
	m.HandleFunc("GET /api/sessions", v.listSessions)
	m.HandleFunc("POST /api/sessions/{id}/name", v.renameSession)
	m.HandleFunc("GET /api/sessions/{id}", v.snapshot)
	m.HandleFunc("DELETE /api/sessions/{id}", v.deleteSession)
	m.HandleFunc("POST /api/sessions/{id}/messages", v.message)
	m.HandleFunc("POST /api/sessions/{id}/pause", v.pause)
	m.HandleFunc("POST /api/sessions/{id}/resume", v.resume)
	m.HandleFunc("GET /api/storage", v.databaseInfo)
	m.HandleFunc("GET /api/sessions/{id}/storage", v.storageStatus)
	m.HandleFunc("POST /api/sessions/{id}/storage/recover", v.recoverStorage)
	m.HandleFunc("POST /api/sessions/{id}/agents/{agent}/settings", v.agentSettings)
	m.HandleFunc("POST /api/sessions/{id}/agents/{agent}/retry", v.retryAgent)
	m.HandleFunc("POST /api/sessions/{id}/agents/retry", v.retryFailedAgents)
	m.HandleFunc("GET /api/sessions/{id}/events", v.events)
	m.HandleFunc("GET /api/sessions/{id}/export", v.exportSession)
	m.HandleFunc("GET /api/sessions/{id}/scope", v.getScope)
	m.HandleFunc("POST /api/sessions/{id}/scope", v.grantScope)
	m.HandleFunc("POST /api/sessions/{id}/arm", v.armScope)
	m.HandleFunc("POST /api/sessions/{id}/disarm", v.disarmScope)
	m.HandleFunc("POST /api/sessions/{id}/approvals", v.approveHost)
	m.HandleFunc("GET /api/sessions/{id}/audit", v.getAudit)
	m.HandleFunc("GET /api/sessions/{id}/proposals", v.proposals)
	m.HandleFunc("GET /api/sessions/{id}/findings", v.findings)
	m.HandleFunc("GET /api/findings", v.listFindings)
	m.HandleFunc("GET /api/sessions/{id}/plan", v.currentPlan)
	m.HandleFunc("POST /api/sessions/{id}/plan/decide", v.decidePlan)
	m.HandleFunc("POST /api/sessions/{id}/plan/done", v.completePlan)
	files, _ := fs.Sub(web, "web")
	m.Handle("/", http.FileServer(http.FS(files)))
	return m
}
func (v *server) storageStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]string{"error": v.r.StorageError(r.PathValue("id"))})
}
func (v *server) recoverStorage(w http.ResponseWriter, r *http.Request) {
	if err := v.r.RecoverStorage(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	respond(w, 200, map[string]string{"status": "paused"})
}
func (v *server) retryAgent(w http.ResponseWriter, r *http.Request) {
	if err := v.r.RetryAgent(r.PathValue("id"), r.PathValue("agent")); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	respond(w, 200, map[string]string{"status": "ready"})
}

// retryFailedAgents restarts every agent left in error. Agents waiting on an
// automatic retry are not included; it reports how many it restarted.
func (v *server) retryFailedAgents(w http.ResponseWriter, r *http.Request) {
	retried, err := v.r.RetryFailed(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	respond(w, 200, map[string]int{"retried": retried})
}
func (v *server) agentSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Thinking *bool `json:"thinking"`
	}
	if err := decode(w, r, &input); err != nil || input.Thinking == nil {
		http.Error(w, "thinking must be a boolean", 400)
		return
	}
	if err := v.s.SetThinking(r.Context(), r.PathValue("id"), r.PathValue("agent"), *input.Thinking); err != nil {
		failure(w, err)
		return
	}
	v.notify(r.PathValue("id"))
	respond(w, 200, map[string]bool{"thinking": *input.Thinking})
}
func (v *server) notify(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for ch := range v.clients[id] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
func id() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func decode(w http.ResponseWriter, r *http.Request, x any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(x); e != nil {
		return e
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		return errors.New("invalid trailing input")
	}
	return nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, e error) {
	if errors.Is(e, sql.ErrNoRows) {
		http.Error(w, "session not found", 404)
		return
	}
	http.Error(w, "internal error", 500)
}
func (v *server) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name               string `json:"name"`
		Goal               string `json:"goal"`
		Brief              string `json:"brief"`
		Concurrency        int    `json:"concurrency"`
		Thinking           bool   `json:"thinking"`
		TargetIP           string `json:"target_ip"`
		RateIntervalMS     int    `json:"rate_interval_ms"`
		InferenceTimeoutMS int    `json:"inference_timeout_ms"`
		RetryDelaysMS      []int  `json:"retry_delays_ms"`
		IdleIntervalMS     int    `json:"idle_interval_ms"`
		Agents             []struct {
			Name  string `json:"name"`
			Role  string `json:"role"`
			Model string `json:"model"`
		} `json:"agents"`
	}
	if e := decode(w, r, &input); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if strings.TrimSpace(input.Goal) == "" || len(input.Goal) > 4096 || input.Concurrency < 1 || input.Concurrency > 32 || len(input.Agents) == 0 || len(input.Agents) > 200 {
		http.Error(w, "invalid goal, concurrency, or agent count", 400)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) > 160 {
		http.Error(w, "session name must be at most 160 UTF-8 bytes", 400)
		return
	}
	input.Brief = strings.TrimSpace(input.Brief)
	if len(input.Brief) > 16384 {
		http.Error(w, "session brief must be at most 16384 UTF-8 bytes", 400)
		return
	}
	if input.InferenceTimeoutMS == 0 {
		input.InferenceTimeoutMS = 600000
	}
	if input.RetryDelaysMS == nil {
		input.RetryDelaysMS = []int{2000, 10000, 30000}
	}
	if input.InferenceTimeoutMS < 1000 || input.InferenceTimeoutMS > 3600000 || len(input.RetryDelaysMS) > 10 {
		http.Error(w, "invalid inference recovery policy", 400)
		return
	}
	if input.IdleIntervalMS != 0 && (input.IdleIntervalMS < 30000 || input.IdleIntervalMS > 3600000) {
		http.Error(w, "idle_interval_ms must be 0 or between 30000 and 3600000", 400)
		return
	}
	for _, delay := range input.RetryDelaysMS {
		if delay < 1 || delay > 3600000 {
			http.Error(w, "invalid retry delay", 400)
			return
		}
	}
	if input.TargetIP != "" || input.RateIntervalMS != 0 {
		ip := net.ParseIP(input.TargetIP)
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || input.RateIntervalMS < 1 || input.RateIntervalMS > 3600000 {
			http.Error(w, "target_ip must be a unicast IP and rate_interval_ms must be between 1 and 3600000", 400)
			return
		}
	}
	leaders := 0
	for _, a := range input.Agents {
		if a.Role == "leader" {
			leaders++
		} else if a.Role != "helper" {
			http.Error(w, "invalid role", 400)
			return
		}
		if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Model) == "" || len(a.Name) > 128 || len(a.Model) > 256 {
			http.Error(w, "invalid agent", 400)
			return
		}
	}
	if leaders != 1 {
		http.Error(w, "exactly one leader required", 400)
		return
	}
	sid, e := id()
	if e != nil {
		failure(w, e)
		return
	}
	s := store.Session{ID: sid, Name: input.Name, Goal: input.Goal, Brief: input.Brief, Concurrency: input.Concurrency, Status: "running", TargetIP: input.TargetIP, RateIntervalMS: input.RateIntervalMS, InferenceTimeoutMS: input.InferenceTimeoutMS, RetryDelaysMS: input.RetryDelaysMS, IdleIntervalMS: input.IdleIntervalMS}
	agents := make([]store.Agent, 0, len(input.Agents))
	for _, a := range input.Agents {
		aid, e := id()
		if e != nil {
			failure(w, e)
			return
		}
		agents = append(agents, store.Agent{ID: aid, SessionID: sid, Name: a.Name, Role: a.Role, Model: a.Model, Status: "ready", Thinking: input.Thinking})
	}
	if e = v.s.CreateSession(r.Context(), s, agents); e != nil {
		failure(w, e)
		return
	}
	if e = v.r.Start(r.Context(), sid); e != nil {
		failure(w, e)
		return
	}
	if _, e = v.s.AppendMessage(r.Context(), sid, "", input.Goal); e != nil {
		failure(w, e)
		return
	}
	v.r.Notify(sid)
	respond(w, 201, map[string]any{"session": s, "agents": agents})
}
func (v *server) listSessions(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		var err error
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			http.Error(w, "invalid offset", 400)
			return
		}
	}
	query := store.SessionQuery{Search: strings.TrimSpace(r.URL.Query().Get("q")), Status: r.URL.Query().Get("status"), Limit: 51, Offset: offset}
	if len(query.Search) > 512 || (query.Status != "" && query.Status != "running" && query.Status != "paused") {
		http.Error(w, "search must be at most 512 UTF-8 bytes and status must be running or paused", 400)
		return
	}
	entries, err := v.s.ListSessions(r.Context(), query)
	if err != nil {
		failure(w, err)
		return
	}
	more := len(entries) > 50
	if more {
		entries = entries[:50]
	}
	respond(w, 200, map[string]any{"sessions": entries, "more": more})
}

func (v *server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input struct {
		ConfirmID string `json:"confirm_id"`
	}
	if err := decode(w, r, &input); err != nil || input.ConfirmID != id {
		http.Error(w, "confirm_id must match the session ID", 400)
		return
	}
	if err := v.r.DeleteSession(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrSessionNotPaused) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		failure(w, err)
		return
	}
	respond(w, 200, map[string]string{"deleted": id})
}

func (v *server) renameSession(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if err := decode(w, r, &input); err != nil {
		http.Error(w, "invalid name", 400)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 160 {
		http.Error(w, "name must contain 1–160 UTF-8 bytes", 400)
		return
	}
	if err := v.s.RenameSession(r.Context(), r.PathValue("id"), input.Name); err != nil {
		failure(w, err)
		return
	}
	v.notify(r.PathValue("id"))
	respond(w, 200, map[string]string{"name": input.Name})
}

func (v *server) snapshot(w http.ResponseWriter, r *http.Request) {
	s, a, e := v.s.GetSession(r.Context(), r.PathValue("id"))
	if e != nil {
		failure(w, e)
		return
	}
	m, e := v.s.Messages(r.Context(), s.ID, 0)
	if e != nil {
		failure(w, e)
		return
	}
	failures, e := v.s.Failures(r.Context(), s.ID)
	if e != nil {
		failure(w, e)
		return
	}
	recoveries := map[string]store.Recovery{}
	for _, agent := range a {
		recovery, err := v.s.Recovery(r.Context(), agent.ID)
		if err != nil {
			failure(w, err)
			return
		}
		if recovery.Failures > 0 {
			recovery.Excerpt = ""
			recoveries[agent.ID] = recovery
		}
	}
	progress, e := v.s.DeliveryProgress(r.Context(), s.ID)
	if e != nil {
		failure(w, e)
		return
	}
	respond(w, 200, map[string]any{"session": s, "agents": a, "messages": m, "activity": v.r.Activity(s.ID), "failures": failures, "recoveries": recoveries, "delivery": progress})
}
func (v *server) message(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	_, agents, e := v.s.GetSession(r.Context(), sid)
	if e != nil {
		failure(w, e)
		return
	}
	var x struct {
		Body        string `json:"body"`
		RecipientID string `json:"recipient_id"`
	}
	if e := decode(w, r, &x); e != nil || strings.TrimSpace(x.Body) == "" || len(x.Body) > 4096 {
		http.Error(w, "invalid message", 400)
		return
	}
	kind := ""
	if x.RecipientID != "" {
		found := false
		for _, agent := range agents {
			if agent.ID == x.RecipientID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "recipient not found in session", 400)
			return
		}
		kind = "direct"
	}
	m, e := v.s.AppendSocialMessage(r.Context(), sid, "", x.RecipientID, kind, x.Body, "")
	if e != nil {
		v.r.StorageFailure(sid, e)
		failure(w, e)
		return
	}
	v.r.Notify(sid)
	respond(w, 201, m)
}
func (v *server) pause(w http.ResponseWriter, r *http.Request) {
	if e := v.r.Pause(r.PathValue("id")); e != nil {
		failure(w, e)
		return
	}
	respond(w, 200, map[string]string{"status": "paused"})
}
func (v *server) resume(w http.ResponseWriter, r *http.Request) {
	if e := v.r.Resume(r.PathValue("id")); e != nil {
		failure(w, e)
		return
	}
	respond(w, 200, map[string]string{"status": "running"})
}
func (v *server) events(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("id")
	if _, _, e := v.s.GetSession(r.Context(), sid); e != nil {
		failure(w, e)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	ch := make(chan struct{}, 1)
	v.mu.Lock()
	if v.clients[sid] == nil {
		v.clients[sid] = map[chan struct{}]bool{}
	}
	v.clients[sid][ch] = true
	v.mu.Unlock()
	defer func() { v.mu.Lock(); delete(v.clients[sid], ch); v.mu.Unlock() }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, "event: update\ndata: {}\n\n")
	f.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if _, _, err := v.s.GetSession(r.Context(), sid); errors.Is(err, sql.ErrNoRows) {
				fmt.Fprint(w, "event: deleted\ndata: {}\n\n")
				f.Flush()
				return
			}
			fmt.Fprint(w, "event: update\ndata: {}\n\n")
			f.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			f.Flush()
		}
	}
}
