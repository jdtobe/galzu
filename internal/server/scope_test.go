package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jdtobe/galzu/internal/herd"
	"github.com/jdtobe/galzu/internal/store"
)

func newScopeServer(t *testing.T, id string) (http.Handler, *store.Store) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.CreateSession(ctx, store.Session{ID: id, Goal: "g", Status: "paused", Concurrency: 1},
		[]store.Agent{{ID: "a", SessionID: id, Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	r := herd.New(s, fake{})
	t.Cleanup(r.Stop)
	return New(s, r), s
}

const scopeBody = `{"damage_line":"nothing that survives a reboot","total_budget":10,"safe_budget":6,"unsafe_budget":3,"max_body_bytes":1024,
"entries":[{"name":"lab.local","ports":[80,443],"methods":["GET","DELETE"],"ips":["10.0.0.5"]}]}`

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
	return w
}

func TestScopeStartsUngrantedAndDisarmed(t *testing.T) {
	h, _ := newScopeServer(t, "s")
	w := do(t, h, "GET", "/api/sessions/s/scope", "")
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var result struct {
		Granted bool `json:"granted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Granted {
		t.Fatal("a fresh session reported a granted scope")
	}
}

// Granting a scope never arms it, and arming without a damage line is refused.
func TestScopeGrantDoesNotArm(t *testing.T) {
	h, s := newScopeServer(t, "s")
	if w := do(t, h, "POST", "/api/sessions/s/scope", scopeBody); w.Code != 200 {
		t.Fatalf("grant %d: %s", w.Code, w.Body.String())
	}
	envelope, err := s.Envelope(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Armed {
		t.Fatal("granting a scope armed the session")
	}
	if envelope.Version != 1 || envelope.SafeBudget != 6 || envelope.Entries[0].Name != "lab.local" {
		t.Fatalf("envelope = %+v", envelope)
	}

	if w := do(t, h, "POST", "/api/sessions/s/arm", ""); w.Code != 200 {
		t.Fatalf("arm %d: %s", w.Code, w.Body.String())
	}
	envelope, err = s.Envelope(context.Background(), "s")
	if err != nil || !envelope.Armed {
		t.Fatalf("armed %v %v", envelope, err)
	}
}

func TestArmRequiresDamageLine(t *testing.T) {
	h, _ := newScopeServer(t, "s")
	body := `{"total_budget":5,"safe_budget":4,"unsafe_budget":1,"entries":[{"name":"lab.local","ports":[80],"ips":["10.0.0.5"]}]}`
	if w := do(t, h, "POST", "/api/sessions/s/scope", body); w.Code != 400 {
		t.Fatalf("grant without a damage line: %d %s", w.Code, w.Body.String())
	}
}

func TestArmWithoutScopeIsAConflict(t *testing.T) {
	h, _ := newScopeServer(t, "s")
	if w := do(t, h, "POST", "/api/sessions/s/arm", ""); w.Code != http.StatusConflict {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
}

func TestScopeRejectsUnusableEntries(t *testing.T) {
	h, _ := newScopeServer(t, "s")
	for _, body := range []string{
		`{"damage_line":"x","total_budget":5,"safe_budget":4,"unsafe_budget":1,"entries":[]}`,
		`{"damage_line":"x","total_budget":5,"safe_budget":4,"unsafe_budget":1,"entries":[{"name":"lab.local","ports":[80],"ips":["not-an-ip"]}]}`,
		`{"damage_line":"x","total_budget":5,"safe_budget":4,"unsafe_budget":1,"entries":[{"name":"lab.local","ports":[],"ips":["10.0.0.5"]}]}`,
		`{"damage_line":"x","total_budget":2,"safe_budget":2,"unsafe_budget":2,"entries":[{"name":"lab.local","ports":[80],"ips":["10.0.0.5"]}]}`,
	} {
		if w := do(t, h, "POST", "/api/sessions/s/scope", body); w.Code != 400 {
			t.Fatalf("accepted %s -> %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestEmbeddedNameInScopeIsNormalised(t *testing.T) {
	h, s := newScopeServer(t, "s")
	body := `{"damage_line":"x","total_budget":5,"safe_budget":4,"unsafe_budget":1,"entries":[{"name":"Lab.Local.","ports":[80],"ips":["10.0.0.5"]}]}`
	if w := do(t, h, "POST", "/api/sessions/s/scope", body); w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	envelope, err := s.Envelope(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Entries[0].Name != "lab.local" {
		t.Fatalf("name = %q", envelope.Entries[0].Name)
	}
}

func TestDisarmAndAuditTrail(t *testing.T) {
	h, s := newScopeServer(t, "s")
	if w := do(t, h, "POST", "/api/sessions/s/scope", scopeBody); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := do(t, h, "POST", "/api/sessions/s/arm", ""); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := do(t, h, "POST", "/api/sessions/s/disarm", ""); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	envelope, err := s.Envelope(context.Background(), "s")
	if err != nil || envelope.Armed {
		t.Fatalf("armed after disarm: %v %v", envelope, err)
	}
	w := do(t, h, "GET", "/api/sessions/s/audit", "")
	var result struct {
		Audit []store.AuditRecord `json:"audit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, record := range result.Audit {
		kinds[record.Kind] = true
	}
	for _, want := range []string{"envelope_granted", "armed", "disarmed"} {
		if !kinds[want] {
			t.Fatalf("audit missing %q: %+v", want, kinds)
		}
	}
}

func TestApproveHostIsRecorded(t *testing.T) {
	h, s := newScopeServer(t, "s")
	if w := do(t, h, "POST", "/api/sessions/s/approvals", `{"host":"Lab.Local."}`); w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	approved, err := s.HostApproved(context.Background(), "s", "lab.local")
	if err != nil || !approved {
		t.Fatalf("approved %v %v", approved, err)
	}
	if w := do(t, h, "POST", "/api/sessions/s/approvals", `{"host":""}`); w.Code != 400 {
		t.Fatalf("empty host accepted: %d", w.Code)
	}
}
