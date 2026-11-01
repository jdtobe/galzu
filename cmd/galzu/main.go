package main

import (
	"context"
	"flag"
	"github.com/jdtobe/galzu/internal/herd"
	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/server"
	"github.com/jdtobe/galzu/internal/store"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// defaultDBPath keeps the database out of the working tree, and in particular
// out of iCloud-synced ~/Documents. SQLite cannot rely on file locking or
// in-place writes in a folder a sync daemon is also rewriting; the symptom is an
// intermittent "database is locked" or I/O error that recovery clears only
// until the next sync.
func defaultDBPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "Galzu", "galzu.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "galzu.db"
	}
	return filepath.Join(home, ".galzu", "galzu.db")
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	db := flag.String("db", defaultDBPath(), "SQLite database path")
	endpoint := flag.String("ollama", "http://127.0.0.1:11434", "Ollama server URL")
	flag.Parse()
	dbSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbSet = true
		}
	})
	path := *db
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Fatal(err)
	}
	if !dbSet {
		migrateLegacy(path)
	}
	warnSynced(path)
	s, err := store.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	if err = s.PauseAll(context.Background()); err != nil {
		log.Fatal(err)
	}
	r := herd.New(s, ollama.New(*endpoint, nil))
	defer r.Stop()
	log.Printf("Herd listening on http://%s (db %s)", *listen, path)
	log.Fatal(http.ListenAndServe(*listen, server.New(s, r)))
}

// migrateLegacy copies the old working-tree database once, when the new default
// is empty. It copies rather than moves so the old file stays as a backup, and
// it refuses if a sidecar journal exists: copying the main file alone could lose
// a transaction.
func migrateLegacy(path string) {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return
	}
	const legacy = "galzu.db"
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(legacy + suffix); err == nil {
			log.Printf("legacy %s%s exists; not copying it. Stop the old server and move the database to %s.", legacy, suffix, path)
			return
		}
	}
	if err := copyFile(legacy, path); err != nil {
		log.Printf("could not copy %s to %s: %v", legacy, path, err)
		return
	}
	log.Printf("copied legacy database %s to %s; the old file is unchanged", legacy, path)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	return out.Close()
}

// warnSynced is a warning, not a refusal: the operator may know better, but an
// intermittent storage failure in these folders is almost always the sync
// daemon, not the database.
func warnSynced(path string) {
	lower := strings.ToLower(path)
	for _, marker := range []string{"/documents/", "/desktop/", "/dropbox/", "/onedrive/", "/google drive/", "/library/mobile documents/"} {
		if strings.Contains(lower, marker) {
			log.Printf("warning: %s is in a cloud-synced folder. SQLite is not reliable there; use the default path %s or another local folder.", path, defaultDBPath())
			return
		}
	}
}
