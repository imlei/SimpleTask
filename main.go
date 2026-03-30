package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"tasktracker/internal/api"
	"tasktracker/internal/auth"
	"tasktracker/internal/mail"
	"tasktracker/internal/store"
)

//go:embed web/static
var staticFS embed.FS

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	db, err := store.Open(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	st := store.New(db)

	var mailer *mail.Mailer
	if os.Getenv("SMTP_HOST") != "" {
		if cfg, err := mail.FromEnv(); err == nil {
			mailer = mail.New(cfg)
		}
	}
	baseURL := os.Getenv("BASE_URL")

	srv := &api.Server{Store: st, Mail: mailer, BaseURL: baseURL}
	authCfg, err := auth.NewAuth(db, dataDir)
	if err != nil {
		log.Fatal(err)
	}

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
		addr = ":8088"
	}
	switch {
	case authCfg.Disabled:
		log.Printf("TaskTracker %s listening on %s (DATA_DIR=%s, auth disabled via AUTH_DISABLE)", Version, addr, dataDir)
	case authCfg.NeedsSetup():
		log.Printf("TaskTracker %s listening on %s (DATA_DIR=%s) — no user yet, open /setup.html to create admin", Version, addr, dataDir)
	default:
		log.Printf("TaskTracker %s listening on %s (DATA_DIR=%s, user=%s)", Version, addr, dataDir, authCfg.Store.Username())
	}
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
