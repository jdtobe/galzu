package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/jdtobe/galzu/internal/herd"
	"github.com/jdtobe/galzu/internal/store"
)

// The bulk retry is one control for a session where several agents have given
// up, and the per-agent endpoint still works after it.
func TestRetryFailedAgentsEndpoint(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "g", Status: "running", Concurrency: 2},
		[]store.Agent{
			{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
			{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
		}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err = s.SetAgentStatus(ctx, id, "error", "boom"); err != nil {
			t.Fatal(err)
		}
	}
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)

	w := do(t, h, "POST", "/api/sessions/s/agents/retry", "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Retried int `json:"retried"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Retried != 2 {
		t.Fatalf("retried %d, want 2", result.Retried)
	}

	// Nothing is left failed, so pressing again is a no-op rather than an error.
	w = do(t, h, "POST", "/api/sessions/s/agents/retry", "")
	if w.Code != http.StatusOK {
		t.Fatalf("second call %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Retried != 0 {
		t.Fatalf("second call retried %d, want 0", result.Retried)
	}

	// The single-agent route still reaches an agent that failed afterwards.
	if err = s.SetAgentStatus(ctx, "a", "error", "boom"); err != nil {
		t.Fatal(err)
	}
	if w = do(t, h, "POST", "/api/sessions/s/agents/a/retry", ""); w.Code != http.StatusOK {
		t.Fatalf("per-agent retry %d: %s", w.Code, w.Body.String())
	}
}

func TestRetryFailedForUnknownSessionIsAConflict(t *testing.T) {
	h, _ := newScopeServer(t, "s")
	if w := do(t, h, "POST", "/api/sessions/missing/agents/retry", ""); w.Code != http.StatusConflict {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
}
