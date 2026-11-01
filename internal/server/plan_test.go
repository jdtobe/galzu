package server

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPlanApprovalFlow(t *testing.T) {
	ctx := context.Background()
	h, s := newScopeServer(t, "s")
	if _, err := s.ProposePlan(ctx, "s", "a", "1. probe\nDone when: banner read\nAcceptance: evidence quoted"); err != nil {
		t.Fatal(err)
	}

	w := do(t, h, "GET", "/api/sessions/s/plan", "")
	var current struct {
		HasPlan bool        `json:"has_plan"`
		Plan    interface{} `json:"plan"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if !current.HasPlan {
		t.Fatal("pending plan not reported")
	}

	if w := do(t, h, "POST", "/api/sessions/s/plan/decide", `{"version":1,"approve":true,"note":"one probe only"}`); w.Code != 200 {
		t.Fatalf("decide %d: %s", w.Code, w.Body.String())
	}
	approved, ok, err := s.CurrentPlan(ctx, "s")
	if err != nil || !ok || approved.Status != "approved" {
		t.Fatalf("%+v %v %v", approved, ok, err)
	}

	// The decision and its note reach the herd as a host message.
	messages, err := s.Messages(ctx, "s", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].AgentID != "" {
		t.Fatalf("messages = %+v", messages)
	}
	if messages[0].Body != "Plan v1 approved: one probe only" {
		t.Fatalf("announcement = %q", messages[0].Body)
	}
}

func TestPlanDecisionOnWrongVersionConflicts(t *testing.T) {
	h, s := newScopeServer(t, "s")
	if _, err := s.ProposePlan(context.Background(), "s", "a", "v1"); err != nil {
		t.Fatal(err)
	}
	if w := do(t, h, "POST", "/api/sessions/s/plan/decide", `{"version":7,"approve":true}`); w.Code != 409 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
}
