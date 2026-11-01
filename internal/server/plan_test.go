package server

import (
	"context"
	"encoding/json"
	"strings"
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

func TestPlanCompletionFlow(t *testing.T) {
	h, s := newScopeServer(t, "s")
	if _, err := s.ProposePlan(context.Background(), "s", "a", "v1"); err != nil {
		t.Fatal(err)
	}
	if w := do(t, h, "POST", "/api/sessions/s/plan/done", `{"version":1}`); w.Code != 409 {
		t.Fatalf("completed before approval: %d: %s", w.Code, w.Body.String())
	}
	if w := do(t, h, "POST", "/api/sessions/s/plan/decide", `{"version":1,"approve":true}`); w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if w := do(t, h, "POST", "/api/sessions/s/plan/done", `{"version":1}`); w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	current, ok, err := s.CurrentPlan(context.Background(), "s")
	if err != nil || !ok || current.Status != "done" {
		t.Fatalf("%+v %v %v", current, ok, err)
	}
	if w := do(t, h, "POST", "/api/sessions/s/plan/done", `{"version":1}`); w.Code != 409 {
		t.Fatalf("completed twice: %d: %s", w.Code, w.Body.String())
	}
	messages, err := s.Messages(context.Background(), "s", 0)
	if err != nil {
		t.Fatal(err)
	}
	last := messages[len(messages)-1]
	if last.AgentID != "" || last.Body != "Plan v1 marked implemented." {
		t.Fatalf("announcement = %+v", last)
	}
}

func TestExportSessionIsMarkdownWithTheRecord(t *testing.T) {
	ctx := context.Background()
	h, s := newScopeServer(t, "s")
	if _, err := s.AppendMessage(ctx, "s", "", "Ship the smallest thing."); err != nil {
		t.Fatal(err)
	}
	proposal, err := s.OpenProposal(ctx, "s", "a", "vote", "Is this the right increment?")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CastBallot(ctx, proposal, "a", "yes", "it is small"); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseProposal(ctx, proposal, "passed", "1-0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProposePlanFor(ctx, "s", "a", "1. ship it", proposal); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFinding(ctx, "s", "a", "Small increments ship.", "The last three did.", ""); err != nil {
		t.Fatal(err)
	}

	w := do(t, h, "GET", "/api/sessions/s/export", "")
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("content type %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "galzu-s.md") {
		t.Fatalf("disposition %q", cd)
	}
	body := w.Body.String()
	for _, want := range []string{
		"# ", "## Agents", "## Plans", "### Plan v1 · pending",
		"Decision: proposal #1 passed.", "## Questions and ballots",
		"### Question #1 · passed", "Ada: **yes** — it is small",
		"## Findings", "### Finding #1 · open", "**support** · Ada: The last three did.",
		"## Conversation", "**Human**", "Ship the smallest thing.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}
