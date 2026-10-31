// Package networktool provides narrowly scoped network primitives. It is not
// yet exposed to agents or the session API.
package networktool

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Executor enforces a single literal IP target and a minimum interval between
// actions. HTTP redirects and environment-configured proxies are disabled.
type Executor struct {
	target   net.IP
	interval time.Duration
	mu       sync.Mutex
	next     time.Time
	client   *http.Client
}

func New(target net.IP, interval time.Duration) (*Executor, error) {
	if target == nil || target.IsUnspecified() || target.IsMulticast() || interval <= 0 {
		return nil, errors.New("a unicast target IP and positive interval are required")
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	return &Executor{target: append(net.IP(nil), target...), interval: interval, client: &http.Client{
		Transport: transport, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (e *Executor) wait(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if delay := time.Until(e.next); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	e.next = time.Now().Add(e.interval)
	return nil
}

// HTTP performs one request to the configured literal IP. The caller must
// close the response body; no redirect is followed.
func (e *Executor) HTTP(ctx context.Context, method, rawURL string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Hostname() == "" {
		return nil, errors.New("invalid HTTP URL")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.Equal(e.target) {
		return nil, errors.New("target is outside the configured scope")
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	if err = e.wait(ctx); err != nil {
		return nil, err
	}
	return e.client.Do(req)
}

// TCP opens one bounded connection to the configured literal IP. Callers must
// close it; the connection has an absolute deadline even if the caller forgets.
func (e *Executor) TCP(ctx context.Context, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return nil, errors.New("invalid TCP address")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.Equal(e.target) {
		return nil, errors.New("target is outside the configured scope")
	}
	if err = e.wait(ctx); err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	if err = conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}
