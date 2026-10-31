package networktool_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jdtobe/garth/internal/networktool"
)

func TestHTTPRejectsOtherHostAndRedirect(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("out-of-scope host was contacted") }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	target := net.ParseIP(u.Hostname())
	executor, err := networktool.New(target, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	response, err := executor.HTTP(context.Background(), "GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.ReadAll(response.Body)
	if response.StatusCode != http.StatusFound {
		t.Fatalf("redirect followed: %d", response.StatusCode)
	}
	_, err = executor.HTTP(context.Background(), "GET", "http://192.0.2.1/", nil)
	if err == nil {
		t.Fatal("out-of-scope target accepted")
	}
}

func TestTCPScopeAndSharedRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	executor, err := networktool.New(net.ParseIP(u.Hostname()), 80*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	response, err := executor.HTTP(context.Background(), "GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	start := time.Now()
	conn, err := executor.TCP(context.Background(), u.Host)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("TCP bypassed HTTP rate limit")
	}
	if _, err = executor.TCP(context.Background(), "192.0.2.1:80"); err == nil {
		t.Fatal("out-of-scope TCP target accepted")
	}
}
