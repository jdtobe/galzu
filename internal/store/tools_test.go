package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.CreateSession(context.Background(), Session{ID: "s", Goal: "g", Status: "paused", Concurrency: 1},
		[]Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	return s
}

func testEnvelope() Envelope {
	return Envelope{
		SessionID: "s", TotalBudget: 10, SafeBudget: 5, UnsafeBudget: 2, MaxBodyBytes: 1024,
		Entries: []EnvelopeEntry{{Name: "lab.local", Ports: []int{80}, Methods: []string{"GET"}, IPs: []string{"10.0.0.5"}}},
	}
}

// A new grant is a new version, and a new version never inherits arming.
func TestEnvelopeVersioningDisarms(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	first, err := s.SaveEnvelope(ctx, testEnvelope())
	if err != nil || first != 1 {
		t.Fatalf("version %d %v", first, err)
	}
	if err := s.SetArmed(ctx, "s", true); err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveEnvelope(ctx, testEnvelope())
	if err != nil || second != 2 {
		t.Fatalf("version %d %v", second, err)
	}
	envelope, err := s.Envelope(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Version != 2 || envelope.Armed {
		t.Fatalf("version %d armed %v, want 2 and disarmed", envelope.Version, envelope.Armed)
	}
	if len(envelope.Entries) != 1 || envelope.Entries[0].IPs[0] != "10.0.0.5" {
		t.Fatalf("entries did not round-trip: %+v", envelope.Entries)
	}
}

func TestEnvelopeMissingIsReported(t *testing.T) {
	if _, err := openTestStore(t).Envelope(context.Background(), "s"); !errors.Is(err, ErrNoEnvelope) {
		t.Fatalf("err = %v, want ErrNoEnvelope", err)
	}
}

// Deleting a session removes its scope but must not erase the record of what
// its agents attempted.
func TestAuditSurvivesSessionDeletion(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	if _, err := s.SaveEnvelope(ctx, testEnvelope()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArmed(ctx, "s", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordRequest(ctx, RequestRecord{
		SessionID: "s", AgentID: "a", Host: "lab.local", Port: 80, Method: "GET",
		Path: "/", Tier: "safe", Status: "allowed", PinnedIP: "10.0.0.5",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := s.Audit(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) < 3 {
		t.Fatalf("expected grant, arm and request in the audit, got %d", len(before))
	}

	if err := s.DeleteSession(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	after, err := s.Audit(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("audit changed across deletion: %d then %d", len(before), len(after))
	}
	// The scope itself is configuration and does not outlive the session.
	if _, err := s.Envelope(ctx, "s"); !errors.Is(err, ErrNoEnvelope) {
		t.Fatalf("envelope survived deletion: %v", err)
	}
}

// Only attempts that ran consume budget; refusals and pending approvals do not.
func TestBudgetCountsOnlyConsumedAttempts(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	for _, status := range []string{"refused", "needs_human", "allowed", "unknown"} {
		if _, err := s.RecordRequest(ctx, RequestRecord{SessionID: "s", Host: "lab.local", Port: 80, Method: "GET", Path: "/", Tier: "safe", Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	count, err := s.CountRequests(ctx, "s", "safe")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 (allowed and unknown only)", count)
	}
}

func TestHostApprovalIsRecordedOnce(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	if err := s.ApproveHost(ctx, "s", "lab.local", "operator"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApproveHost(ctx, "s", "lab.local", "operator"); err != nil {
		t.Fatal(err)
	}
	approved, err := s.HostApproved(ctx, "s", "lab.local")
	if err != nil || !approved {
		t.Fatalf("approved %v %v", approved, err)
	}
	if other, _ := s.HostApproved(ctx, "s", "nas.local"); other {
		t.Fatal("approval leaked to another host")
	}
}
