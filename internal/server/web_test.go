package server

import (
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
