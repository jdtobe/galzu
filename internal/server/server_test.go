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

func TestInferenceRecoveryPolicy(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"goal":"test","concurrency":1,"inference_timeout_ms":5000,"retry_delays_ms":[],"agents":[{"name":"Ada","role":"leader","model":"m"}]}`, 201},
		{`{"goal":"test","concurrency":1,"inference_timeout_ms":20,"agents":[{"name":"Ada","role":"leader","model":"m"}]}`, 400},
		{`{"goal":"test","concurrency":1,"retry_delays_ms":[0],"agents":[{"name":"Ada","role":"leader","model":"m"}]}`, 400},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		if tc.status == 201 {
			var result struct {
				Session store.Session `json:"session"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			saved, _, err := s.GetSession(ctx, result.Session.ID)
			if err != nil || saved.InferenceTimeoutMS != 5000 || saved.RetryDelaysMS == nil || len(saved.RetryDelaysMS) != 0 {
				t.Fatalf("%+v %v", saved, err)
			}
		}
	}
}

func TestHumanDirectMessage(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	agents := []store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}
	if err := s.CreateSession(ctx, store.Session{ID: "s", Goal: "test", Status: "paused", Concurrency: 1}, agents); err != nil {
		t.Fatal(err)
	}
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"body":"private","recipient_id":"a"}`, 201},
		{`{"body":"wrong recipient","recipient_id":"other"}`, 400},
		{`{"body":"public"}`, 201},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions/s/messages", bytes.NewBufferString(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	messages, err := s.Messages(ctx, "s", 0)
	if err != nil || len(messages) != 2 {
		t.Fatalf("%+v %v", messages, err)
	}
	if messages[0].RecipientID != "a" || messages[0].Kind != "direct" || messages[0].AgentID != "" || messages[1].RecipientID != "" {
		t.Fatalf("%+v", messages)
	}
}

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

func TestAgentThinkingSetting(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"goal":"test","concurrency":1,"agents":[{"name":"Ada","role":"leader","model":"m"}]}`)))
	var created struct {
		Session store.Session `json:"session"`
		Agents  []store.Agent `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || len(created.Agents) != 1 {
		t.Fatalf("%s %v", w.Body.String(), err)
	}
	w = httptest.NewRecorder()
	url := "/api/sessions/" + created.Session.ID + "/agents/" + created.Agents[0].ID + "/settings"
	h.ServeHTTP(w, httptest.NewRequest("POST", url, bytes.NewBufferString(`{"thinking":true}`)))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	_, agents, err := s.GetSession(context.Background(), created.Session.ID)
	if err != nil || !agents[0].Thinking {
		t.Fatalf("%+v %v", agents, err)
	}
}

type thinkingObserver struct{ requested chan bool }

func (f thinkingObserver) Chat(context.Context, string, []ollama.Message) (ollama.Reply, error) {
	return ollama.Reply{Action: "rest"}, nil
}
func (f thinkingObserver) ChatWithThinking(_ context.Context, _ string, _ []ollama.Message, thinking bool) (ollama.Reply, error) {
	f.requested <- thinking
	return ollama.Reply{Action: "rest"}, nil
}

func TestSessionThinkingDefault(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		want        bool
	}{
		{"omitted", "", false}, {"enabled", `,"thinking":true`, true}, {"disabled", `,"thinking":false`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			observer := thinkingObserver{make(chan bool, 2)}
			r := herd.New(s, observer)
			defer r.Stop()
			h := New(s, r)
			body := `{"goal":"test","concurrency":2` + tc.field + `,"agents":[{"name":"Ada","role":"leader","model":"m"},{"name":"Bea","role":"helper","model":"m"}]}`
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body)))
			if w.Code != http.StatusCreated {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			var created struct {
				Session store.Session `json:"session"`
				Agents  []store.Agent `json:"agents"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			_, agents, err := s.GetSession(context.Background(), created.Session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(agents) != 2 || len(created.Agents) != 2 {
				t.Fatalf("agents: %+v", agents)
			}
			for i, agent := range agents {
				if agent.Thinking != tc.want || created.Agents[i].Thinking != tc.want {
					t.Fatalf("agent: %+v", agent)
				}
				select {
				case got := <-observer.requested:
					if got != tc.want {
						t.Fatalf("initial inference thinking = %v", got)
					}
				case <-time.After(time.Second):
					t.Fatal("no initial inference")
				}
			}
			if err := s.SetThinking(context.Background(), created.Session.ID, agents[0].ID, !tc.want); err != nil {
				t.Fatal(err)
			}
			_, agents, err = s.GetSession(context.Background(), created.Session.ID)
			if err != nil || agents[0].Thinking == tc.want || agents[1].Thinking != tc.want {
				t.Fatalf("override: %+v %v", agents, err)
			}
		})
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
