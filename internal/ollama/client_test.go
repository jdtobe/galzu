package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
