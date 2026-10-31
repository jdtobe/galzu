package server

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jdtobe/garth/internal/herd"
	"github.com/jdtobe/garth/internal/ollama"
	"github.com/jdtobe/garth/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

type fake struct{}

func (fake) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	return ollama.Reply{Action: "rest"}, nil
}
func TestSessionAPI(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"goal":"cooperate","concurrency":1,"agents":[{"name":"Ada","role":"leader","model":"m"}]}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
func TestRejectInvalid(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "db"))
	defer s.Close()
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"goal":"","concurrency":1}`)))
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestSessionPersistsOptionalNetworkScope(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	w := httptest.NewRecorder()
	body := `{"goal":"test","concurrency":1,"target_ip":"127.0.0.1","rate_interval_ms":500,"agents":[{"name":"Ada","role":"leader","model":"m"}]}`
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var created struct {
		Session store.Session `json:"session"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.GetSession(context.Background(), created.Session.ID)
	if err != nil || got.TargetIP != "127.0.0.1" || got.RateIntervalMS != 500 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestSessionRejectsIncompleteNetworkScope(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	for _, policy := range []string{`"target_ip":"example.com","rate_interval_ms":100`, `"target_ip":"127.0.0.1"`, `"rate_interval_ms":100`, `"target_ip":"0.0.0.0","rate_interval_ms":100`} {
		w := httptest.NewRecorder()
		body := `{"goal":"test","concurrency":1,` + policy + `,"agents":[{"name":"Ada","role":"leader","model":"m"}]}`
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("policy %s: %d", policy, w.Code)
		}
	}
}

type talker struct{}

func (talker) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	return ollama.Reply{Action: "message", Text: "I can help."}, nil
}

func TestHerdRespondsToInitialGoal(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := herd.New(s, talker{})
	defer r.Stop()
	h := New(s, r)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"goal":"work together","concurrency":1,"agents":[{"name":"Ada","role":"leader","model":"m"}]}`)))
	if w.Code != http.StatusCreated {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct {
		Session store.Session `json:"session"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for {
		messages, err := s.Messages(context.Background(), created.Session.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) == 2 && messages[1].Body == "I can help." {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("agent did not respond: %+v", messages)
		default:
			time.Sleep(time.Millisecond)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions/"+created.Session.ID+"/pause", nil))
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
}
