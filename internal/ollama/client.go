package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Reply struct {
	Action    string  `json:"action"`
	Text      string  `json:"text"`
	Thinking  string  `json:"thinking,omitempty"`
	Recipient string  `json:"recipient,omitempty"`
	Proposal  int64   `json:"proposal,omitempty"`
	Finding   int64   `json:"finding,omitempty"`
	Choice    string  `json:"choice,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	Evidence  string  `json:"evidence,omitempty"`
	Stance    string  `json:"stance,omitempty"`
	Memory    *string `json:"memory,omitempty"`
	// Server-side accounting. prompt_eval_count is the whole prompt, not just
	// cache misses, so the duration is what reveals a cache hit.
	PromptEvalCount    int   `json:"-"`
	PromptEvalDuration int64 `json:"-"` // nanoseconds
	EvalCount          int   `json:"-"`
}

// ValidationError describes a rejected model action. Excerpt is untrusted and
// may contain private data; it must not be written to ordinary logs.
type ValidationError struct{ Reason, Excerpt string }
type ContextLimitError struct{}

func (*ContextLimitError) Error() string { return "model context window exceeded" }
func (e *ValidationError) Error() string { return e.Reason }
func invalid(reason, content string) error {
	if len(content) > 1024 {
		content = content[:1024]
	}
	return &ValidationError{Reason: reason, Excerpt: strings.ToValidUTF8(content, "�")}
}

type Client struct {
	base string
	http *http.Client
}

func New(base string, c *http.Client) *Client {
	if c == nil {
		c = http.DefaultClient
	}
	return &Client{strings.TrimRight(base, "/"), c}
}
func (c *Client) Chat(ctx context.Context, model string, messages []Message) (Reply, error) {
	return c.ChatWithThinking(ctx, model, messages, false)
}
func (c *Client) ChatWithThinking(ctx context.Context, model string, messages []Message, think bool) (Reply, error) {
	var out Reply
	b, e := json.Marshal(map[string]any{"model": model, "messages": messages, "stream": false, "format": "json", "think": think})
	if e != nil {
		return out, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.base+"/api/chat", bytes.NewReader(b))
	if e != nil {
		return out, e
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := c.http.Do(req)
	if e != nil {
		return out, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
		text := strings.ToLower(string(body))
		if strings.Contains(text, "context") && (strings.Contains(text, "exceed") || strings.Contains(text, "too long") || strings.Contains(text, "maximum")) {
			return out, &ContextLimitError{}
		}
		return out, fmt.Errorf("ollama returned %s", res.Status)
	}
	var response struct {
		Message struct {
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"message"`
		PromptEvalCount    int   `json:"prompt_eval_count"`
		PromptEvalDuration int64 `json:"prompt_eval_duration"`
		EvalCount          int   `json:"eval_count"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); e != nil {
		return out, e
	}
	if e = json.Unmarshal([]byte(response.Message.Content), &out); e != nil {
		return out, invalid("response is not valid JSON", response.Message.Content)
	}
	out.PromptEvalCount = response.PromptEvalCount
	out.PromptEvalDuration = response.PromptEvalDuration
	out.EvalCount = response.EvalCount
	out.Thinking = ""
	if think {
		out.Thinking = response.Message.Thinking
	}
	if out.Action != "message" && out.Action != "rest" && out.Action != "direct" && out.Action != "help" &&
		out.Action != "propose" && out.Action != "vote" && out.Action != "plan" && out.Action != "available" &&
		out.Action != "finding" && out.Action != "endorse" && out.Action != "retract" && out.Action != "review" {
		return out, invalid("unknown action; use message, direct, help, available, propose, vote, plan, finding, endorse, retract, review, or rest", response.Message.Content)
	}
	if out.Action == "retract" && out.Finding <= 0 {
		return out, invalid("a retraction needs the finding id", response.Message.Content)
	}
	if out.Action == "review" {
		if out.Finding <= 0 {
			return out, invalid("a review request needs the finding id", response.Message.Content)
		}
		if strings.TrimSpace(out.Recipient) == "" {
			return out, invalid("a review request needs a recipient agent id", response.Message.Content)
		}
	}
	if out.Action == "endorse" {
		if out.Finding <= 0 {
			return out, invalid("endorsements need the finding id", response.Message.Content)
		}
		if out.Choice != "agree" && out.Choice != "disagree" {
			return out, invalid(`endorsements need a choice of "agree" or "disagree"`, response.Message.Content)
		}
	}
	if out.Action == "finding" && len(out.Evidence) > 4096 {
		return out, invalid("finding evidence exceeds 4096 bytes", response.Message.Content)
	}
	if out.Action == "vote" {
		if out.Proposal <= 0 {
			return out, invalid("votes need the proposal id", response.Message.Content)
		}
		if out.Choice == "" {
			return out, invalid(`votes need a choice of "yes" or "no"`, response.Message.Content)
		}
	}
	// A revision may carry evidence without repeating the claim; every other
	// action needs text.
	evidenceOnly := out.Action == "finding" && out.Finding > 0 && strings.TrimSpace(out.Evidence) != ""
	if out.Action != "rest" && !evidenceOnly && strings.TrimSpace(out.Text) == "" {
		return out, invalid("message text is empty", response.Message.Content)
	}
	if out.Action != "rest" && len(out.Text) > 4096 {
		return out, invalid("message text exceeds 4096 bytes", response.Message.Content)
	}
	return out, nil
}
