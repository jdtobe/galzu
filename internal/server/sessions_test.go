package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdtobe/galzu/internal/herd"
	"github.com/jdtobe/galzu/internal/store"
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
		{"GET", "/api/sessions?status=unknown", "", 400},
		{"GET", "/api/sessions?q=" + strings.Repeat("a", 513), "", 400},
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

func TestSessionSearchAPI(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "older-target", Name: "Needle", Goal: "Find evidence", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendMessage(ctx, "older-target", "", "secretmessageonly"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 51; i++ {
		if err := s.CreateSession(ctx, store.Session{ID: fmt.Sprintf("decoy-%02d", i), Name: "Archive", Goal: "Newer", Status: "paused", Concurrency: 1}, nil); err != nil {
			t.Fatal(err)
		}
	}
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	for _, tc := range []struct {
		query string
		count int
		more  bool
		first string
	}{
		{"", 50, true, ""},
		{"?q=NEEDLE", 1, false, "older-target"},
		{"?q=+Needle+&status=running", 1, false, "older-target"},
		{"?q=needle&status=paused", 0, false, ""},
		{"?q=evidence", 1, false, "older-target"},
		{"?q=older-targ", 1, false, "older-target"},
		{"?q=secretmessageonly", 0, false, ""},
		{"?q=%25", 0, false, ""},
		{"?q=archive&status=paused", 50, true, ""},
		{"?q=archive&status=paused&offset=50", 1, false, ""},
	} {
		t.Run(tc.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions"+tc.query, nil))
			if w.Code != 200 {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			var result struct {
				Sessions []store.SessionEntry `json:"sessions"`
				More     bool                 `json:"more"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Sessions) != tc.count || result.More != tc.more {
				t.Fatalf("count %d more %v", len(result.Sessions), result.More)
			}
			if tc.first != "" && result.Sessions[0].ID != tc.first {
				t.Fatalf("first %+v", result.Sessions[0])
			}
		})
	}
}

func TestDeleteSessionAPIRequiresConfirmationAndPause(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "running", Concurrency: 1}, nil); err != nil {
		t.Fatal(err)
	}
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{}`, 400}, {`{"confirm_id":"wrong"}`, 400}, {`{"confirm_id":"s"}`, 409},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/sessions/s", strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	}
	if err := s.SetSessionStatus(ctx, "s", "paused"); err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{200, 404} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/sessions/s", strings.NewReader(`{"confirm_id":"s"}`)))
		if w.Code != code {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	}
}
