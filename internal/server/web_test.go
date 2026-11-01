package server

import (
	"encoding/xml"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdtobe/garth/internal/herd"
	"github.com/jdtobe/garth/internal/store"
)

func TestConversationWorkspaceAssets(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := herd.New(s, fake{})
	defer r.Stop()
	h := New(s, r)
	for _, tc := range []struct {
		path    string
		content []string
	}{
		{"/", []string{`id="agents"`, `id="agent-detail"`, `id="send"`, `id="storage-recover"`, `id="rename-session"`, `id="session-query"`, `id="session-status"`, `id="clear-session-filters"`, `data-filter="direct"`, `src="/ui.js"`, `src="/app.js"`}},
		{"/style.css", []string{".session-workspace", ".inspector", "@media (max-width: 700px)"}},
		{"/ui.js", []string{"agentState", "visibleMessage", "deliveryState"}},
		{"/app.js", []string{"renderAgentDetail", "renderMessages", "refreshLibrary"}},
		{"/garth-mark.svg", []string{"<svg", "long-haired profile with glasses", `fill-rule="evenodd"`}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != 200 {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			for _, want := range tc.content {
				if !strings.Contains(w.Body.String(), want) {
					t.Errorf("missing %q", want)
				}
			}
		})
	}
}

func TestBrandArtworkIsSelfContained(t *testing.T) {
	data, err := web.ReadFile("web/garth-mark.svg")
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "script", "foreignObject", "image":
			t.Fatalf("unexpected active or external content: %s", start.Name.Local)
		}
		for _, attribute := range start.Attr {
			if strings.HasPrefix(strings.ToLower(attribute.Name.Local), "on") {
				t.Fatalf("unexpected event handler: %s", attribute.Name.Local)
			}
			if attribute.Name.Local == "href" && !strings.HasPrefix(attribute.Value, "#") {
				t.Fatalf("external resource: %s", attribute.Value)
			}
		}
	}
}

func TestBrandMarkUsesOneColorAndTransparentNegativeSpace(t *testing.T) {
	data, err := web.ReadFile("web/garth-mark.svg")
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	hasCutout := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attribute := range start.Attr {
			switch attribute.Name.Local {
			case "fill", "stroke":
				if attribute.Value != "none" && attribute.Value != "currentColor" {
					t.Fatalf("unexpected extra paint color: %s", attribute.Value)
				}
			case "fill-rule":
				if attribute.Value == "evenodd" {
					hasCutout = true
				}
			}
		}
	}
	if !hasCutout {
		t.Fatal("expected transparent face cutout")
	}
}
