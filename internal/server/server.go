package server

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jdtobe/garth/internal/herd"
	"github.com/jdtobe/garth/internal/store"
	"io"
	"io/fs"
	"net"
	"net/http"
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
	m.HandleFunc("GET /api/sessions/{id}", v.snapshot)
	m.HandleFunc("POST /api/sessions/{id}/messages", v.message)
	m.HandleFunc("POST /api/sessions/{id}/pause", v.pause)
	m.HandleFunc("POST /api/sessions/{id}/resume", v.resume)
	m.HandleFunc("POST /api/sessions/{id}/agents/{agent}/settings", v.agentSettings)
	m.HandleFunc("GET /api/sessions/{id}/events", v.events)
	files, _ := fs.Sub(web, "web")
	m.Handle("/", http.FileServer(http.FS(files)))
	return m
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
		Goal           string `json:"goal"`
		Concurrency    int    `json:"concurrency"`
		Thinking       bool   `json:"thinking"`
		TargetIP       string `json:"target_ip"`
		RateIntervalMS int    `json:"rate_interval_ms"`
		Agents         []struct {
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
	s := store.Session{ID: sid, Goal: input.Goal, Concurrency: input.Concurrency, Status: "running", TargetIP: input.TargetIP, RateIntervalMS: input.RateIntervalMS}
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
	respond(w, 200, map[string]any{"session": s, "agents": a, "messages": m, "activity": v.r.Activity(s.ID)})
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
			fmt.Fprint(w, "event: update\ndata: {}\n\n")
			f.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			f.Flush()
		}
	}
}
