package toolgate

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/jdtobe/galzu/internal/store"
)

type fakeResolver struct {
	addrs map[string][]net.IP
	calls []string
}

func (f *fakeResolver) LookupIP(_ context.Context, host string) ([]net.IP, error) {
	f.calls = append(f.calls, host)
	if addrs, ok := f.addrs[host]; ok {
		return addrs, nil
	}
	return nil, fmt.Errorf("no such host %q", host)
}

func newGate(t *testing.T, envelope store.Envelope) (*Gate, *store.Store, *fakeResolver) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "g", Status: "paused", Concurrency: 1},
		[]store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	envelope.SessionID = "s"
	if _, err := s.SaveEnvelope(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	// A new envelope version always lands disarmed, so tests arm explicitly.
	if envelope.Armed {
		if err := s.SetArmed(ctx, "s", true); err != nil {
			t.Fatal(err)
		}
	}
	resolver := &fakeResolver{addrs: map[string][]net.IP{"lab.local": {net.ParseIP("10.0.0.5")}}}
	return &Gate{Store: s, Resolver: resolver}, s, resolver
}

func armed() store.Envelope {
	return store.Envelope{
		Armed:        true,
		TotalBudget:  10,
		SafeBudget:   5,
		UnsafeBudget: 2,
		IntervalMS:   0,
		MaxBodyBytes: 1024,
		Entries: []store.EnvelopeEntry{
			{Name: "lab.local", Ports: []int{80, 443}, IPs: []string{"10.0.0.5"}},
		},
	}
}

func request() Request {
	return Request{SessionID: "s", AgentID: "a", Host: "lab.local", Port: 80, Method: "GET", Path: "/status", Purpose: "check liveness"}
}

// 1. An unarmed session cannot send anything.
func TestUnarmedRefusesWithoutResolving(t *testing.T) {
	envelope := armed()
	envelope.Armed = false
	gate, _, resolver := newGate(t, envelope)

	decision, err := gate.Evaluate(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("unarmed session was allowed")
	}
	if decision.Reason != "session is not armed" {
		t.Fatalf("reason = %q", decision.Reason)
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("resolved %v for an unarmed session", resolver.calls)
	}
}

// 2. A host outside the envelope is refused before any lookup happens.
func TestHostOutsideEnvelopeIsNeverResolved(t *testing.T) {
	gate, _, resolver := newGate(t, armed())

	req := request()
	req.Host = "nas.local"
	decision, err := gate.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("out-of-envelope host was allowed")
	}
	if len(resolver.calls) != 0 {
		t.Fatalf("an unapproved name was resolved: %v", resolver.calls)
	}
}

// 3. A name that resolves outside its entry's set is refused before the request.
func TestResolutionOutsideEnvelopeIsRefused(t *testing.T) {
	envelope := armed()
	envelope.Entries[0].IPs = []string{"10.0.0.6"}
	gate, _, resolver := newGate(t, envelope)
	resolver.addrs["lab.local"] = []net.IP{net.ParseIP("192.168.1.10")}

	decision, err := gate.Evaluate(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.PinnedIP != "" {
		t.Fatalf("allowed a rebound name: %+v", decision)
	}
}

// 4. The first DELETE against a host needs a human; the second does not.
func TestFirstDeleteNeedsHumanThenIsAllowed(t *testing.T) {
	ctx := context.Background()
	gate, s, _ := newGate(t, armed())

	req := request()
	req.Method = "DELETE"
	req.Path = "/old"

	first, err := gate.Evaluate(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Allowed || !first.NeedsHuman {
		t.Fatalf("first DELETE = %+v, want needs-human", first)
	}

	if err := s.ApproveHost(ctx, "s", "lab.local", "operator"); err != nil {
		t.Fatal(err)
	}
	second, err := gate.Evaluate(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Allowed || second.NeedsHuman {
		t.Fatalf("second DELETE = %+v, want allowed", second)
	}
}

// 5. Exhausting a tier's budget refuses rather than escalating.
func TestBudgetExhaustionRefusesRatherThanEscalating(t *testing.T) {
	ctx := context.Background()
	envelope := armed()
	envelope.UnsafeBudget = 1
	gate, _, _ := newGate(t, envelope)

	req := request()
	req.Method = "POST"
	req.Path = "/jobs"

	if decision, err := gate.Evaluate(ctx, req); err != nil || !decision.Allowed {
		t.Fatalf("first POST = %+v %v", decision, err)
	}
	second, err := gate.Evaluate(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Allowed || second.NeedsHuman {
		t.Fatalf("second POST = %+v, want a plain refusal", second)
	}
	if second.Reason != "unsafe budget exhausted" {
		t.Fatalf("reason = %q", second.Reason)
	}
}

// Safe and unsafe verbs spend separate budgets, so reads cannot starve writes
// and writes cannot starve reads.
func TestTierBudgetsAreSeparate(t *testing.T) {
	ctx := context.Background()
	envelope := armed()
	envelope.UnsafeBudget = 1
	gate, _, _ := newGate(t, envelope)

	post := request()
	post.Method = "POST"
	if decision, err := gate.Evaluate(ctx, post); err != nil || !decision.Allowed {
		t.Fatalf("POST = %+v %v", decision, err)
	}
	for i := 0; i < 3; i++ {
		if decision, err := gate.Evaluate(ctx, request()); err != nil || !decision.Allowed {
			t.Fatalf("GET %d = %+v %v", i, decision, err)
		}
	}
}

// A body over the envelope limit is refused, not truncated.
func TestOversizedBodyIsRefused(t *testing.T) {
	envelope := armed()
	envelope.MaxBodyBytes = 8
	gate, _, _ := newGate(t, envelope)

	req := request()
	req.Method = "POST"
	req.Body = []byte("more than eight bytes")
	decision, err := gate.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("oversized body was allowed")
	}
}

// Refusals are recorded as evidence but never consume budget.
func TestRefusalsDoNotConsumeBudget(t *testing.T) {
	ctx := context.Background()
	gate, s, _ := newGate(t, armed())

	req := request()
	req.Host = "nas.local"
	for i := 0; i < 5; i++ {
		if _, err := gate.Evaluate(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	count, err := s.CountRequests(ctx, "s", TierSafe)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("refusals consumed %d of the safe budget", count)
	}
	trail, err := s.Audit(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(trail) < 5 {
		t.Fatalf("expected refusals in the audit, got %d records", len(trail))
	}
}

// The pinned address is the one the envelope permits, not the first resolved.
func TestPinsThePermittedAddress(t *testing.T) {
	envelope := armed()
	envelope.Entries[0].IPs = []string{"10.0.0.5", "10.0.0.7"}
	gate, _, resolver := newGate(t, envelope)
	resolver.addrs["lab.local"] = []net.IP{net.ParseIP("10.0.0.7"), net.ParseIP("10.0.0.5")}

	decision, err := gate.Evaluate(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.PinnedIP != "10.0.0.7" {
		t.Fatalf("pinned %q, want the first permitted address", decision.PinnedIP)
	}
}

// Path prefixes are enforced when an entry declares one.
func TestPathPrefixIsEnforced(t *testing.T) {
	envelope := armed()
	envelope.Entries[0].PathPrefix = "/api/"
	gate, _, _ := newGate(t, envelope)

	req := request()
	req.Path = "/admin/shutdown"
	if decision, err := gate.Evaluate(context.Background(), req); err != nil || decision.Allowed {
		t.Fatalf("allowed a path outside the prefix: %+v %v", decision, err)
	}
	req.Path = "/api/status"
	if decision, err := gate.Evaluate(context.Background(), req); err != nil || !decision.Allowed {
		t.Fatalf("refused a path inside the prefix: %+v %v", decision, err)
	}
}
