package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"simpletask/internal/api"
	"simpletask/internal/auth"
	"simpletask/internal/mail"
	"simpletask/internal/store"
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

	certFile := strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("TLS_KEY_FILE"))
	if certFile != "" && keyFile != "" {
		if os.Getenv("AUTH_SECURE_COOKIE") == "" {
			_ = os.Setenv("AUTH_SECURE_COOKIE", "true")
		}
	} else if certFile != "" || keyFile != "" {
		log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must both be set for HTTPS, or leave both empty for HTTP")
	}

	srv := &api.Server{Store: st, Mail: mailer, BaseURL: baseURL}
	authCfg, err := auth.NewAuth(db, dataDir)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	api.Register(mux, srv)
	auth.Register(mux, authCfg)
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":"%s"}`, Version)
	})

	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		log.Fatal(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		fileServer.ServeHTTP(w, r)
	}))

	handler := auth.Middleware(authCfg, mux)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8088"
	}
	useTLS := certFile != "" && keyFile != ""
	tlsNote := ""
	if useTLS {
		tlsNote = ", HTTPS (TLS_CERT_FILE)"
	}
	switch {
	case authCfg.Disabled:
		log.Printf("SimpleTask %s listening on %s (DATA_DIR=%s, auth disabled via AUTH_DISABLE)%s", Version, addr, dataDir, tlsNote)
	case authCfg.NeedsSetup():
		log.Printf("SimpleTask %s listening on %s (DATA_DIR=%s) — no user yet, open /setup.html to create admin%s", Version, addr, dataDir, tlsNote)
	default:
		log.Printf("SimpleTask %s listening on %s (DATA_DIR=%s, user=%s)%s", Version, addr, dataDir, authCfg.Store.Username(), tlsNote)
	}
	var errListen error
	if useTLS {
		errListen = http.ListenAndServeTLS(addr, certFile, keyFile, handler)
	} else {
		errListen = http.ListenAndServe(addr, handler)
	}
	if errListen != nil {
		log.Fatal(errListen)
	}
}
