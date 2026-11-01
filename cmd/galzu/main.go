package main

import (
	"context"
	"flag"
	"github.com/jdtobe/galzu/internal/herd"
	"github.com/jdtobe/galzu/internal/ollama"
	"github.com/jdtobe/galzu/internal/server"
	"github.com/jdtobe/galzu/internal/store"
	"log"
	"net/http"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	db := flag.String("db", "galzu.db", "SQLite database path")
	endpoint := flag.String("ollama", "http://127.0.0.1:11434", "Ollama server URL")
	flag.Parse()
	s, err := store.Open(*db)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	if err = s.PauseAll(context.Background()); err != nil {
		log.Fatal(err)
	}
	r := herd.New(s, ollama.New(*endpoint, nil))
	defer r.Stop()
	log.Printf("Herd listening on http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, server.New(s, r)))
}
