package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestInvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"{\"action\":\"message\",\"text\":\"\"}"}}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL, srv.Client()).Chat(context.Background(), "m", nil); err == nil {
		t.Fatal("expected error")
	}
}
