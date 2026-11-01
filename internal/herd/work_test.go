package herd

import (
	"context"
	"strings"
	"testing"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

func TestAvailableOpensAWorkRequest(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 2)
	r := New(s, restsAtOnce{})
	defer r.Stop()
	helper := agents[1]

	saved, err := r.handleAvailable(store.Session{ID: "s"}, helper,
		ollamaReply("available", "I can take a task"), 0, nil)
	if err != nil || !saved {
		t.Fatalf("saved %v err %v", saved, err)
	}
	open, err := s.OpenWorkRequests(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].AgentID != "b" || open[0].Status != "open" {
		t.Fatalf("open = %+v", open)
	}
}

// A reply from anyone else answers the offer.
func TestAnsweredWorkIsResolved(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 2)
	r := New(s, restsAtOnce{})
	defer r.Stop()
	message, err := s.AppendAgentReply(ctx, "s", "b", "", "available", "free", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenWorkRequest(ctx, "s", "b", message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAgentReply(ctx, "s", "a", "", "", "take the docs task", "", 0); err != nil {
		t.Fatal(err)
	}
	r.settleWork(store.Session{ID: "s"}, agents)

	open, err := s.OpenWorkRequests(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("still open: %+v", open)
	}
}

// Nobody answers: the leader is asked directly, once.
func TestUnansweredWorkEscalatesToOneMessage(t *testing.T) {
	ctx := context.Background()
	agents := []store.Agent{
		{ID: "a", SessionID: "s", Name: "Ada", Role: "leader", Model: "m", Status: "ready"},
		{ID: "b", SessionID: "s", Name: "Beau", Role: "helper", Model: "m", Status: "ready"},
	}
	s := openHerd(t, agents, 2)
	r := New(s, restsAtOnce{})
	defer r.Stop()

	previous := workTimeout
	workTimeout = 0
	defer func() { workTimeout = previous }()

	message, err := s.AppendAgentReply(ctx, "s", "b", "", "available", "free", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenWorkRequest(ctx, "s", "b", message.ID); err != nil {
		t.Fatal(err)
	}
	r.settleWork(store.Session{ID: "s"}, agents)
	r.settleWork(store.Session{ID: "s"}, agents)

	messages, err := s.Messages(ctx, "s", 0)
	if err != nil {
		t.Fatal(err)
	}
	escalations := 0
	for _, m := range messages {
		if strings.Contains(m.Body, "allocate work or say why") {
			escalations++
			if !strings.Contains(m.Body, "Ada") || !strings.Contains(m.Body, "Beau") {
				t.Fatalf("escalation does not name both: %q", m.Body)
			}
		}
	}
	if escalations != 1 {
		t.Fatalf("escalated %d times, want 1", escalations)
	}
	open, _ := s.OpenWorkRequests(ctx, "s")
	if len(open) != 1 || open[0].Status != "escalated" {
		t.Fatalf("status = %+v", open)
	}
}

func ollamaReply(action, text string) ollama.Reply {
	return ollama.Reply{Action: action, Text: text}
}
