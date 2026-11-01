package herd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

// Idle reflection is off by default and, when on, wakes one agent with a prompt
// that says it is idle rather than pretending a peer spoke.
func TestIdleIntervalWakesAnAgentWithAnIdleNotice(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "work", Status: "running", Concurrency: 1, IdleIntervalMS: 20},
		[]store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := memoryInference{make(chan []ollama.Message, 4), make(chan ollama.Reply, 4)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	var prompt []ollama.Message
	select {
	case prompt = <-f.prompts:
	case <-time.After(2 * time.Second):
		t.Fatal("no idle wake")
	}
	found := false
	for _, message := range prompt {
		if strings.Contains(message.Content, "You have been idle") {
			found = true
		}
	}
	if !found {
		t.Fatalf("prompt does not say it is an idle turn: %+v", prompt)
	}
	f.replies <- ollama.Reply{Action: "rest"}
}

func TestIdleIntervalIsOffByDefault(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(ctx, store.Session{ID: "s", Goal: "work", Status: "running", Concurrency: 1},
		[]store.Agent{{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	f := memoryInference{make(chan []ollama.Message, 1), make(chan ollama.Reply, 1)}
	r := New(s, f)
	defer r.Stop()
	if err = r.Start(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	select {
	case prompt := <-f.prompts:
		t.Fatalf("idle prompt without an interval: %+v", prompt)
	case <-time.After(150 * time.Millisecond):
	}
}
