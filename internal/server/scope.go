package server

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/jdtobe/galzu/internal/store"
)

// The tool policy's HTTP surface. It grants scope, arms and disarms, clears a
// host for DELETE, and reports the trail. It never executes a request: the gate
// in internal/toolgate decides, and nothing calls it until the tool action
// exists.
//
// Spec: docs/superpowers/specs/2026-11-01-tool-policy.md

type scopeInput struct {
	DamageLine   string `json:"damage_line"`
	TotalBudget  int    `json:"total_budget"`
	SafeBudget   int    `json:"safe_budget"`
	UnsafeBudget int    `json:"unsafe_budget"`
	IntervalMS   int    `json:"rate_interval_ms"`
	MaxBodyBytes int    `json:"max_body_bytes"`
	Entries      []struct {
		Name       string   `json:"name"`
		Ports      []int    `json:"ports"`
		Methods    []string `json:"methods"`
		PathPrefix string   `json:"path_prefix"`
		IPs        []string `json:"ips"`
	} `json:"entries"`
}

func (v *server) getScope(w http.ResponseWriter, r *http.Request) {
	envelope, err := v.s.Envelope(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNoEnvelope) {
		respond(w, 200, map[string]any{"granted": false})
		return
	}
	if err != nil {
		failure(w, err)
		return
	}
	trail, err := v.s.Audit(r.Context(), r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"granted": true, "envelope": envelope, "audit": trail})
}

// grantScope records a new envelope version. It always lands disarmed: scope
// changes never silently re-arm a session.
func (v *server) grantScope(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input scopeInput
	if e := decode(w, r, &input); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if strings.TrimSpace(input.DamageLine) == "" || len(input.DamageLine) > 1024 {
		http.Error(w, "damage_line is required and must be at most 1024 UTF-8 bytes", 400)
		return
	}
	if input.TotalBudget < 1 || input.SafeBudget < 0 || input.UnsafeBudget < 0 ||
		input.SafeBudget+input.UnsafeBudget > input.TotalBudget || input.MaxBodyBytes < 0 {
		http.Error(w, "invalid budgets", 400)
		return
	}
	if len(input.Entries) == 0 || len(input.Entries) > 32 {
		http.Error(w, "an envelope needs between 1 and 32 entries", 400)
		return
	}
	envelope := store.Envelope{
		SessionID: id, DamageLine: strings.TrimSpace(input.DamageLine),
		TotalBudget: input.TotalBudget, SafeBudget: input.SafeBudget, UnsafeBudget: input.UnsafeBudget,
		IntervalMS: input.IntervalMS, MaxBodyBytes: input.MaxBodyBytes,
	}
	seen := map[string]bool{}
	for _, entry := range input.Entries {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entry.Name), "."))
		if name == "" || seen[name] || len(entry.Ports) == 0 || len(entry.IPs) == 0 {
			http.Error(w, "each entry needs a unique name, at least one port and at least one IP", 400)
			return
		}
		seen[name] = true
		for _, port := range entry.Ports {
			if port < 1 || port > 65535 {
				http.Error(w, "invalid port", 400)
				return
			}
		}
		ips := make([]string, 0, len(entry.IPs))
		for _, text := range entry.IPs {
			ip := net.ParseIP(strings.TrimSpace(text))
			if ip == nil {
				http.Error(w, "entries must list literal IPs", 400)
				return
			}
			ips = append(ips, ip.String())
		}
		methods := make([]string, 0, len(entry.Methods))
		for _, method := range entry.Methods {
			methods = append(methods, strings.ToUpper(strings.TrimSpace(method)))
		}
		if strings.TrimSpace(entry.PathPrefix) != "" && !strings.HasPrefix(entry.PathPrefix, "/") {
			http.Error(w, "path_prefix must start with /", 400)
			return
		}
		envelope.Entries = append(envelope.Entries, store.EnvelopeEntry{
			Name: name, Ports: entry.Ports, Methods: methods, PathPrefix: entry.PathPrefix, IPs: ips,
		})
	}
	version, err := v.s.SaveEnvelope(r.Context(), envelope)
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"version": version, "armed": false})
}

// armScope is the act the spec calls separate from creating a session. It needs
// a damage line, because the harness cannot evaluate "lasting damage" itself.
func (v *server) armScope(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	envelope, err := v.s.Envelope(r.Context(), id)
	if errors.Is(err, store.ErrNoEnvelope) {
		http.Error(w, "grant a scope before arming", http.StatusConflict)
		return
	}
	if err != nil {
		failure(w, err)
		return
	}
	if strings.TrimSpace(envelope.DamageLine) == "" {
		http.Error(w, "a written statement of lasting damage is required before arming", http.StatusConflict)
		return
	}
	if err := v.s.SetArmed(r.Context(), id, true); err != nil {
		failure(w, err)
		return
	}
	v.notify(id)
	respond(w, 200, map[string]any{"armed": true})
}

func (v *server) disarmScope(w http.ResponseWriter, r *http.Request) {
	if err := v.s.SetArmed(r.Context(), r.PathValue("id"), false); err != nil {
		failure(w, err)
		return
	}
	v.notify(r.PathValue("id"))
	respond(w, 200, map[string]any{"armed": false})
}

// approveHost clears DELETE against one host, once.
func (v *server) approveHost(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Host string `json:"host"`
	}
	if e := decode(w, r, &input); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(input.Host), "."))
	if host == "" {
		http.Error(w, "host is required", 400)
		return
	}
	if err := v.s.ApproveHost(r.Context(), r.PathValue("id"), host, "human"); err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"approved": host})
}

func (v *server) getAudit(w http.ResponseWriter, r *http.Request) {
	trail, err := v.s.Audit(r.Context(), r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"audit": trail})
}

// proposals lists every question in the session with the ballots behind it.
func (v *server) proposals(w http.ResponseWriter, r *http.Request) {
	questions, err := v.s.Proposals(r.Context(), r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	type entry struct {
		store.Proposal
		Ballots []store.Ballot `json:"ballots"`
	}
	out := make([]entry, 0, len(questions))
	for _, question := range questions {
		ballots, err := v.s.Ballots(r.Context(), question.ID)
		if err != nil {
			failure(w, err)
			return
		}
		out = append(out, entry{Proposal: question, Ballots: ballots})
	}
	respond(w, 200, map[string]any{"proposals": out})
}
