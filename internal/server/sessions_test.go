package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdtobe/garth/internal/herd"
	"github.com/jdtobe/garth/internal/store"
)

func TestSessionLibraryAPI(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Name: "Original", Goal: "Evidence", Status: "paused", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "s", "", "SECRET MESSAGE"); err != nil {
		t.Fatal(err)
	}
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/sessions?offset=-1", "", 400},
		{"GET", "/api/sessions?offset=oops", "", 400},
		{"POST", "/api/sessions/s/name", `{"name":"  Renamed  "}`, 200},
		{"POST", "/api/sessions/s/name", `{"name":" "}`, 400},
		{"POST", "/api/sessions/s/name", `{"name":"` + strings.Repeat("é", 81) + `"}`, 400},
		{"POST", "/api/sessions/missing/name", `{"name":"Missing"}`, 404},
		{"GET", "/api/sessions/s", "", 200},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions", nil))
	var result struct {
		Sessions []store.SessionEntry `json:"sessions"`
		More     bool                 `json:"more"`
	}
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Sessions) != 1 || result.Sessions[0].Name != "Renamed" || result.Sessions[0].Status != "paused" || result.More {
		t.Fatalf("%+v", result)
	}
	if strings.Contains(w.Body.String(), "SECRET MESSAGE") {
		t.Fatal("library leaked message content")
	}
	if len(r.Activity("s")) != 0 {
		t.Fatal("browsing started workers")
	}
}
