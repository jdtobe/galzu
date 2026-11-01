package herd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/store"
)

func promptText(messages []ollama.Message) string {
	var b strings.Builder
	for _, m := range messages {
		b.WriteString(m.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func fixture() (store.Session, store.Agent, []store.Agent) {
	session := store.Session{ID: "s", Goal: "ship the release"}
	me := store.Agent{ID: "a1", Name: "Ada", Role: "leader", Status: "ready"}
	peers := []store.Agent{me, {ID: "a2", Name: "Bea", Role: "helper", Status: "ready"}}
	return session, me, peers
}

func TestPromptPrefixIsAppendOnly(t *testing.T) {
	session, me, peers := fixture()
	entries := []store.Message{
		{ID: 1, AgentID: "", Body: "start"},
		{ID: 2, AgentID: "a2", Body: "checking the tests"},
	}
	first := renderPrompt(promptInput{
		Session: session, Agent: me, Agents: peers,
		Activity: map[string]Activity{"a2": {State: "inference"}},
		Memory:   "first memory",
		Entries:  entries,
	})
	renamed := []store.Agent{me, {ID: "a2", Name: "Beatrice", Role: "helper", Status: "ready"}}
	second := renderPrompt(promptInput{
		Session: session, Agent: me, Agents: renamed,
		Activity: map[string]Activity{"a2": {State: "idle"}},
		Memory:   "second memory",
		Entries:  append(entries, store.Message{ID: 3, AgentID: "a1", Body: "taking notes"}),
	})
	if !bytes.HasPrefix(flatten(second.Fixed()), flatten(first.Fixed())) {
		t.Fatalf("prefix changed between turns:\nfirst=%q\nsecond=%q", flatten(first.Fixed()), flatten(second.Fixed()))
	}
	if bytes.Equal(flatten(first.Tail()), flatten(second.Tail())) {
		t.Fatal("volatile turn packet did not change")
	}
	if !strings.Contains(promptText(second.Messages()), "second memory") {
		t.Fatal("new memory missing from turn packet")
	}
}

func TestPromptPrefixIgnoresMemoryRosterAndActivity(t *testing.T) {
	session, me, peers := fixture()
	entries := []store.Message{{ID: 1, AgentID: "", Body: "hello"}}
	base := renderPrompt(promptInput{Session: session, Agent: me, Agents: peers, Entries: entries})
	changed := renderPrompt(promptInput{
		Session:  session,
		Agent:    me,
		Agents:   []store.Agent{me, {ID: "a2", Name: "Bea", Role: "helper", Status: "error"}},
		Activity: map[string]Activity{"a2": {State: "retrying"}},
		Memory:   "different",
		Entries:  entries,
	})
	if !bytes.Equal(flatten(base.Fixed()), flatten(changed.Fixed())) {
		t.Fatal("volatile state leaked into the cached prefix")
	}
}

func TestPromptLogUsesIDsAndHidesPrivateBodies(t *testing.T) {
	session, me, peers := fixture()
	entries := []store.Message{
		{ID: 1, AgentID: "a2", Body: "for you", RecipientID: "a1"},
		{ID: 2, AgentID: "a2", Body: "for someone else", RecipientID: "a3", Kind: "help"},
	}
	p := renderPrompt(promptInput{Session: session, Agent: me, Agents: peers, Entries: entries})
	fixed := promptText(p.Fixed())
	if strings.Contains(fixed, "Bea") {
		t.Fatalf("peer name leaked into the append-only prefix: %s", fixed)
	}
	if !strings.Contains(fixed, "1 a2 private-to:a1: for you") {
		t.Fatalf("recipient cannot read its own message: %s", fixed)
	}
	if strings.Contains(fixed, "for someone else") {
		t.Fatalf("private body leaked: %s", fixed)
	}
	if !strings.Contains(fixed, "2 a2 private-to:a3 help-request (content is private)") {
		t.Fatalf("missing content-free notice: %s", fixed)
	}
}

func TestPromptMarksOwnMessagesAsAssistant(t *testing.T) {
	session, me, peers := fixture()
	p := renderPrompt(promptInput{
		Session: session, Agent: me, Agents: peers,
		Entries: []store.Message{{ID: 4, AgentID: "a1", Body: "my own words"}},
	})
	last := p.Fixed()[len(p.Fixed())-1]
	if last.Role != "assistant" {
		t.Fatalf("own message role = %q", last.Role)
	}
	if p.Messages()[0].Role != "system" || p.Tail()[0].Role != "user" {
		t.Fatal("unexpected roles around the prefix boundary")
	}
}

func TestPromptReuseReportsSharedBytes(t *testing.T) {
	session, me, peers := fixture()
	entries := []store.Message{{ID: 1, AgentID: "", Body: "hello"}}
	first := renderPrompt(promptInput{Session: session, Agent: me, Agents: peers, Entries: entries})
	second := renderPrompt(promptInput{Session: session, Agent: me, Agents: peers, Memory: "new", Entries: append(entries, store.Message{ID: 2, AgentID: "a2", Body: "hi"})})
	reused := promptReuse(flatten(first.Messages()), flatten(second.Messages()))
	if reused < len(flatten(first.Fixed())) {
		t.Fatalf("reused %d bytes; expected at least the whole first prefix (%d)", reused, len(flatten(first.Fixed())))
	}
	if reused > len(flatten(first.Messages())) {
		t.Fatalf("reused %d bytes; longer than the previous prompt (%d)", reused, len(flatten(first.Messages())))
	}
}

func TestBriefIsCarriedInThePreamble(t *testing.T) {
	session, me, peers := fixture()
	session.Brief = "Galzu runs a herd of local AI agents; here \"herd\" means agents."
	text := promptText(renderPrompt(promptInput{Session: session, Agent: me, Agents: peers}).Fixed())
	if !strings.Contains(text, "Goal: ship the release\nBrief: Galzu runs a herd of local AI agents") {
		t.Fatalf("brief missing or misplaced:\n%s", text)
	}
}

func TestEmptyBriefLeavesNoStrayLine(t *testing.T) {
	session, me, peers := fixture()
	text := promptText(renderPrompt(promptInput{Session: session, Agent: me, Agents: peers}).Fixed())
	if strings.Contains(text, "Brief:") {
		t.Fatalf("unexpected brief line:\n%s", text)
	}
	if !strings.Contains(text, "Goal: ship the release\n\nYou are one member") {
		t.Fatalf("preamble layout changed:\n%s", text)
	}
}
