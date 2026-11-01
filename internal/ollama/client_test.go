package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestThinkingDisabledIgnoresReturnedThinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Think bool `json:"think"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Think {
			t.Errorf("unexpected request: %+v %v", request, err)
		}
		_, _ = w.Write([]byte(`{"message":{"thinking":"private","content":"{\"action\":\"message\",\"text\":\"hello\",\"thinking\":\"injected\"}"}}`))
	}))
	defer srv.Close()
	reply, err := New(srv.URL, srv.Client()).Chat(context.Background(), "model", nil)
	if err != nil || reply.Thinking != "" {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestThinkingRequestedAndReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Think bool `json:"think"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !request.Think {
			t.Errorf("thinking not requested: %+v %v", request, err)
		}
		_, _ = w.Write([]byte(`{"message":{"thinking":"considering evidence","content":"{\"action\":\"message\",\"text\":\"hello\"}"}}`))
	}))
	defer srv.Close()
	reply, err := New(srv.URL, srv.Client()).ChatWithThinking(context.Background(), "model", nil, true)
	if err != nil || reply.Thinking != "considering evidence" || reply.Text != "hello" {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"{\"action\":\"message\",\"text\":\"hello\"}"}}`))
	}))
	defer srv.Close()
	reply, err := New(srv.URL, srv.Client()).Chat(context.Background(), "model", []Message{{Role: "user", Content: "hi"}})
	if err != nil || reply.Text != "hello" {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestWorkingMemoryReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"{\"action\":\"rest\",\"text\":\"\",\"memory\":\"uncertain claim\"}"}}`))
	}))
	defer srv.Close()
	reply, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil)
	if err != nil || reply.Memory == nil || *reply.Memory != "uncertain claim" {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestInvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"{\"action\":\"message\",\"text\":\"\"}"}}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestContextLimitResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"context window exceeded"}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil)
	if _, ok := err.(*ContextLimitError); !ok {
		t.Fatalf("expected context limit, got %v", err)
	}
}

func TestFindingEvidenceOnlyRevisionIsAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"{\"action\":\"finding\",\"finding\":3,\"evidence\":\"a counterexample\",\"stance\":\"oppose\"}"}}`))
	}))
	defer srv.Close()
	reply, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil)
	if err != nil || reply.Finding != 3 || reply.Evidence != "a counterexample" {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestEndorsementNeedsAFindingAndAChoice(t *testing.T) {
	for _, content := range []string{
		`{"action":"endorse","choice":"agree","text":"yes"}`,
		`{"action":"endorse","finding":2,"choice":"maybe","text":"hmm"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"message":{"content":` + "`" + content + "`" + `}}`))
		}))
		if _, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil); err == nil {
			t.Fatalf("accepted %s", content)
		}
		srv.Close()
	}
}

func TestStringAndFencedIDsAreAccepted(t *testing.T) {
	cases := []string{
		`{"action":"vote","proposal":"p2","choice":"yes","text":"ok"}`,
		"```json\n{\"action\":\"vote\",\"proposal\":\"2\",\"choice\":\"yes\",\"text\":\"ok\"}\n```",
		`Here is my answer: {"action":"endorse","finding":"#3","choice":"agree","text":"ok"} — done.`,
	}
	for _, content := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"message":{"content":` + strconv.Quote(content) + `}}`))
		}))
		reply, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil)
		srv.Close()
		if err != nil {
			t.Fatalf("%q: %v", content, err)
		}
		if reply.Action == "vote" && reply.Proposal != 2 {
			t.Fatalf("%q: proposal = %d", content, reply.Proposal)
		}
		if reply.Action == "endorse" && reply.Finding != 3 {
			t.Fatalf("%q: finding = %d", content, reply.Finding)
		}
	}
}

func TestMalformedReplyNamesTheProblem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"not json at all"}}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("err = %v", err)
	}
}
