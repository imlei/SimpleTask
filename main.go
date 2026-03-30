package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"biztracker/internal/api"
	"biztracker/internal/auth"
	"biztracker/internal/store"
)

//go:embed web/static
var staticFS embed.FS

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	st, err := store.New(dataDir)
	if err != nil {
		log.Fatal(err)
	}

	srv := &api.Server{Store: st}
	authCfg := auth.ConfigFromEnv()

	mux := http.NewServeMux()
	api.Register(mux, srv)
	auth.Register(mux, authCfg)

	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	handler := auth.Middleware(authCfg, mux)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if authCfg.Enabled {
		log.Printf("biztracker listening on %s (DATA_DIR=%s, AUTH_USER=%s)", addr, dataDir, authCfg.Username)
	} else {
		log.Printf("biztracker listening on %s (DATA_DIR=%s, auth disabled — set AUTH_PASSWORD to enable)", addr, dataDir)
	}
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
