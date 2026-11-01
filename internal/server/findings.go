package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jdtobe/galzu/internal/store"
)

// findings lists every claim with its evidence and current positions. Agreement
// is reported, never interpreted: the API has no verified field because the
// harness does not decide that.
func (v *server) findings(w http.ResponseWriter, r *http.Request) {
	findings, err := v.s.Findings(r.Context(), r.PathValue("id"), 0)
	if err != nil {
		failure(w, err)
		return
	}
	if findings == nil {
		findings = []store.Finding{}
	}
	respond(w, 200, map[string]any{"findings": findings})
}

// listFindings is the cross-session knowledge view. It returns one page, newest
// update first, with the session and author names already resolved.
func (v *server) listFindings(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 512 {
		http.Error(w, "query must be at most 512 UTF-8 bytes", 400)
		return
	}
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			http.Error(w, "invalid limit", 400)
			return
		}
		limit = parsed
	}
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			http.Error(w, "invalid offset", 400)
			return
		}
		offset = parsed
	}
	findings, err := v.s.AllFindings(r.Context(), query, limit+1, offset)
	if err != nil {
		failure(w, err)
		return
	}
	more := len(findings) > limit
	if more {
		findings = findings[:limit]
	}
	if findings == nil {
		findings = []store.SharedFinding{}
	}
	respond(w, 200, map[string]any{"findings": findings, "more": more})
}

// verifyFinding is the operator's mark on a claim. Agreement is not
// verification, and neither is this a guarantee; it is a human statement that
// the evidence was examined.
func (v *server) verifyFinding(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("finding"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "invalid finding id", 400)
		return
	}
	var input struct {
		Verified bool `json:"verified"`
	}
	if e := decode(w, r, &input); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if err := v.s.VerifyFinding(r.Context(), r.PathValue("id"), id, input.Verified, "human"); err != nil {
		if errors.Is(err, store.ErrFindingGone) {
			http.Error(w, "finding not found", 404)
			return
		}
		failure(w, err)
		return
	}
	v.notify(r.PathValue("id"))
	respond(w, 200, map[string]any{"verified": input.Verified})
}
