package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// databaseInfo exposes the database path and a warning when it lives somewhere
// a sync daemon may rewrite. The startup log has the same warning, but a terminal
// is easy to miss and this is the failure that recovery only hides until the next
// sync.
func (v *server) databaseInfo(w http.ResponseWriter, r *http.Request) {
	path := v.s.Path()
	respond(w, 200, map[string]string{"path": path, "warning": syncedPathWarning(path)})
}

func syncedPathWarning(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	for _, marker := range []string{"/documents/", "/desktop/", "/dropbox/", "/onedrive/", "/google drive/", "/library/mobile documents/"} {
		if strings.Contains(lower, marker) {
			return fmt.Sprintf("The database is at %s, inside a cloud-synced folder. SQLite is not reliable there; stop the app and restart with -db pointing to a local folder.", path)
		}
	}
	return ""
}
