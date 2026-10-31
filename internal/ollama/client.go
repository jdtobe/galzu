package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	Action string `json:"action"`
	Text   string `json:"text"`
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
	var out Reply
	b, e := json.Marshal(map[string]any{"model": model, "messages": messages, "stream": false, "format": "json"})
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
		return out, fmt.Errorf("ollama returned %s", res.Status)
	}
	var response struct {
		Message Message `json:"message"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); e != nil {
		return out, e
	}
	if e = json.Unmarshal([]byte(response.Message.Content), &out); e != nil {
		return out, fmt.Errorf("invalid social response: %w", e)
	}
	if out.Action != "message" && out.Action != "rest" {
		return out, errors.New("invalid action")
	}
	if out.Action == "message" && (strings.TrimSpace(out.Text) == "" || len(out.Text) > 4096) {
		return out, errors.New("invalid message length")
	}
	return out, nil
}
