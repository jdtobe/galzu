package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jdtobe/galzu/internal/store"
)

// currentPlan returns the approved plan, or the pending one when nothing has
// been approved yet, with the version history.
func (v *server) currentPlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok, err := v.s.CurrentPlan(r.Context(), id)
	if err != nil {
		failure(w, err)
		return
	}
	versions, err := v.s.Plans(r.Context(), id)
	if err != nil {
		failure(w, err)
		return
	}
	respond(w, 200, map[string]any{"plan": plan, "has_plan": ok, "versions": versions})
}

// decidePlan is the human's approval. The note reaches the herd as a message,
// because a rejection with no reason is just a wall.
func (v *server) decidePlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input struct {
		Version int    `json:"version"`
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if e := decode(w, r, &input); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if input.Version < 1 || len(input.Note) > 2048 {
		http.Error(w, "version and note are invalid", 400)
		return
	}
	if err := v.s.DecidePlan(r.Context(), id, input.Version, input.Approve, strings.TrimSpace(input.Note), "human"); err != nil {
		if errors.Is(err, store.ErrPlanNotPending) {
			http.Error(w, "that plan version is not pending review", http.StatusConflict)
			return
		}
		failure(w, err)
		return
	}
	outcome := "rejected"
	if input.Approve {
		outcome = "approved"
	}
	message := fmt.Sprintf("Plan v%d %s.", input.Version, outcome)
	if note := strings.TrimSpace(input.Note); note != "" {
		message = fmt.Sprintf("Plan v%d %s: %s", input.Version, outcome, note)
	}
	if _, err := v.s.AppendMessage(r.Context(), id, "", message); err != nil {
		failure(w, err)
		return
	}
	v.notify(id)
	respond(w, 200, map[string]any{"version": input.Version, "approved": input.Approve})
}

// completePlan is the human saying the approved work is implemented. The
// harness cannot observe that, so it records the claim rather than inferring it.
func (v *server) completePlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input struct {
		Version int `json:"version"`
	}
	if e := decode(w, r, &input); e != nil || input.Version < 1 {
		http.Error(w, "version is invalid", 400)
		return
	}
	if err := v.s.CompletePlan(r.Context(), id, input.Version, "human"); err != nil {
		if errors.Is(err, store.ErrPlanNotApproved) {
			http.Error(w, "that plan version is not approved", http.StatusConflict)
			return
		}
		failure(w, err)
		return
	}
	if _, err := v.s.AppendMessage(r.Context(), id, "", fmt.Sprintf("Plan v%d marked implemented.", input.Version)); err != nil {
		failure(w, err)
		return
	}
	v.notify(id)
	respond(w, 200, map[string]any{"version": input.Version, "status": "done"})
}
