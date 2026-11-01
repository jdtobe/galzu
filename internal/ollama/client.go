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
	Memory    *string `json:"memory,omitempty"`
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
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); e != nil {
		return out, e
	}
	if e = json.Unmarshal([]byte(response.Message.Content), &out); e != nil {
		return out, invalid("response is not valid JSON", response.Message.Content)
	}
	out.Thinking = ""
	if think {
		out.Thinking = response.Message.Thinking
	}
	if out.Action != "message" && out.Action != "rest" && out.Action != "direct" && out.Action != "help" {
		return out, invalid("unknown action; use message, direct, help, or rest", response.Message.Content)
	}
	if out.Action != "rest" && strings.TrimSpace(out.Text) == "" {
		return out, invalid("message text is empty", response.Message.Content)
	}
	if out.Action != "rest" && len(out.Text) > 4096 {
		return out, invalid("message text exceeds 4096 bytes", response.Message.Content)
	}
	return out, nil
}
