package server

import (
	"net/http"

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
