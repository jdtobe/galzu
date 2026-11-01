// Package toolgate decides whether a herd may make an external request.
//
// It sits in front of internal/networktool and is the only thing that may
// authorise one. The prompt states the policy as guidance; this is the
// structural refusal that does not care what a model argues.
//
// Spec: docs/superpowers/specs/2026-11-01-tool-policy.md
package toolgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/jdtobe/galzu/internal/store"
)

// Resolver turns an approved hostname into addresses. It is injected so tests
// never touch DNS, and so production can pin a resolver.
type Resolver interface {
	LookupIP(ctx context.Context, host string) ([]net.IP, error)
}

// Gate evaluates requests against a session's envelope and records what it did.
type Gate struct {
	Store    *store.Store
	Resolver Resolver
}

// Request is a single action, never a batch.
type Request struct {
	SessionID string
	AgentID   string
	Host      string
	Port      int
	Method    string
	Path      string
	Body      []byte
	Purpose   string
	Expect    string
}

// Decision is the verdict. NeedsHuman means the action is inside the envelope
// but requires a human once: the first DELETE against a host.
type Decision struct {
	Allowed    bool
	NeedsHuman bool
	PinnedIP   string
	Tier       string
	Reason     string
	RequestID  int64
}

const (
	TierSafe   = "safe"
	TierUnsafe = "unsafe"

	// StatusAllowed and friends are the values written to tool_requests.
	StatusAllowed    = "allowed"
	StatusRefused    = "refused"
	StatusNeedsHuman = "needs_human"
)

var safeMethods = map[string]bool{"GET": true, "HEAD": true, "OPTIONS": true}

// Tier classifies a verb. The harness cannot see consequence — POST /search and
// POST /restart are the same bytes to a policy engine — so the method is a risk
// signal, budgeted separately, never a safety property.
func Tier(method string) string {
	if safeMethods[strings.ToUpper(method)] {
		return TierSafe
	}
	return TierUnsafe
}

// Evaluate runs the ordered check from the spec and records the attempt. Any
// failure refuses; a refusal is evidence, so it is written down too.
func (g *Gate) Evaluate(ctx context.Context, req Request) (Decision, error) {
	req.Method = strings.ToUpper(req.Method)
	if req.Path == "" {
		req.Path = "/"
	}
	decision := Decision{Tier: Tier(req.Method)}

	refuse := func(reason string) (Decision, error) {
		decision.Allowed, decision.NeedsHuman, decision.Reason = false, false, reason
		id, err := g.record(ctx, req, decision, StatusRefused)
		if err != nil {
			return decision, err
		}
		decision.RequestID = id
		return decision, nil
	}

	envelope, err := g.Store.Envelope(ctx, req.SessionID)
	if err == store.ErrNoEnvelope {
		return refuse("session has no tool envelope")
	}
	if err != nil {
		return decision, err
	}
	if !envelope.Armed {
		return refuse("session is not armed")
	}

	// An unapproved name is refused before it is looked up, so a model cannot
	// leak a secret through the lookup itself.
	entry := findEntry(envelope, req.Host)
	if entry == nil {
		return refuse("host is not in the tool envelope")
	}
	if !containsInt(entry.Ports, req.Port) {
		return refuse(fmt.Sprintf("port %d is not allowed for %s", req.Port, entry.Name))
	}
	if len(entry.Methods) > 0 && !containsString(entry.Methods, req.Method) {
		return refuse(fmt.Sprintf("method %s is not allowed for %s", req.Method, entry.Name))
	}
	if entry.PathPrefix != "" && !strings.HasPrefix(req.Path, entry.PathPrefix) {
		return refuse(fmt.Sprintf("path is outside %s", entry.PathPrefix))
	}
	if envelope.MaxBodyBytes > 0 && len(req.Body) > envelope.MaxBodyBytes {
		return refuse("request body exceeds the envelope limit")
	}

	// Budgets refuse rather than escalate: approval under pressure is exactly
	// the operator error the interface is meant to prevent.
	total, err := g.Store.CountRequests(ctx, req.SessionID, "")
	if err != nil {
		return decision, err
	}
	if total >= envelope.TotalBudget {
		return refuse("envelope budget exhausted")
	}
	tierCount, err := g.Store.CountRequests(ctx, req.SessionID, decision.Tier)
	if err != nil {
		return decision, err
	}
	if tierCount >= tierBudget(envelope, decision.Tier) {
		return refuse(decision.Tier + " budget exhausted")
	}

	// Resolve once, validate the address, then dial that address. Go's default
	// transport re-resolves at dial time, which is how rebinding gets in.
	addresses, err := g.Resolver.LookupIP(ctx, entry.Name)
	if err != nil {
		return refuse("could not resolve " + entry.Name)
	}
	pinned := pin(addresses, entry.IPs)
	if pinned == "" {
		return refuse(entry.Name + " resolves outside the envelope")
	}
	decision.PinnedIP = pinned

	if req.Method == "DELETE" {
		approved, err := g.Store.HostApproved(ctx, req.SessionID, entry.Name)
		if err != nil {
			return decision, err
		}
		if !approved {
			decision.NeedsHuman = true
			decision.Reason = "first DELETE against " + entry.Name + " needs a human"
			id, err := g.record(ctx, req, decision, StatusNeedsHuman)
			if err != nil {
				return decision, err
			}
			decision.RequestID = id
			return decision, nil
		}
	}

	decision.Allowed = true
	id, err := g.record(ctx, req, decision, StatusAllowed)
	if err != nil {
		return decision, err
	}
	decision.RequestID = id
	return decision, nil
}

// record writes the attempt. The digest never carries the body itself, which
// may hold private content.
func (g *Gate) record(ctx context.Context, req Request, d Decision, status string) (int64, error) {
	sum := sha256.Sum256(req.Body)
	return g.Store.RecordRequest(ctx, store.RequestRecord{
		SessionID:  req.SessionID,
		AgentID:    req.AgentID,
		Host:       strings.ToLower(req.Host),
		Port:       req.Port,
		Method:     req.Method,
		Path:       req.Path,
		Purpose:    req.Purpose,
		Expect:     req.Expect,
		Tier:       d.Tier,
		Status:     status,
		PinnedIP:   d.PinnedIP,
		Reason:     d.Reason,
		BodyBytes:  len(req.Body),
		BodyDigest: hex.EncodeToString(sum[:]),
	})
}

func findEntry(e *store.Envelope, host string) *store.EnvelopeEntry {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for i := range e.Entries {
		if strings.ToLower(e.Entries[i].Name) == host {
			return &e.Entries[i]
		}
	}
	return nil
}

// pin returns the first resolved address that the envelope permits. An entry
// without an explicit set is not permitted anything, which makes the safe
// failure "nothing resolves".
func pin(addresses []net.IP, allowed []string) string {
	for _, ip := range addresses {
		for _, candidate := range allowed {
			if parsed := net.ParseIP(candidate); parsed != nil && parsed.Equal(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

func tierBudget(e *store.Envelope, tier string) int {
	if tier == TierSafe {
		return e.SafeBudget
	}
	return e.UnsafeBudget
}

func containsInt(values []int, want int) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// ParsePort is a small helper for callers that accept ports as text.
func ParsePort(text string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q", text)
	}
	return port, nil
}
