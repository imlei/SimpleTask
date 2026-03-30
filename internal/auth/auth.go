package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	cookieName = "biztracker_auth"
	maxAgeSec  = 7 * 24 * 3600 // 7 天
)

// Config 从环境变量加载；未设置 AUTH_PASSWORD 时不启用登录（与旧行为兼容）
type Config struct {
	Enabled       bool
	Username      string
	Password      string
	Secret        []byte
	SecureCookie  bool
}

func ConfigFromEnv() *Config {
	pass := strings.TrimSpace(os.Getenv("AUTH_PASSWORD"))
	if pass == "" {
		return &Config{Enabled: false}
	}
	user := strings.TrimSpace(os.Getenv("AUTH_USER"))
	if user == "" {
		user = "admin"
	}
	secret := os.Getenv("AUTH_SECRET")
	if secret == "" {
		sum := sha256.Sum256([]byte("biztracker-session|" + pass))
		secret = string(sum[:])
	}
	secure := strings.EqualFold(os.Getenv("AUTH_SECURE_COOKIE"), "1") ||
		strings.EqualFold(os.Getenv("AUTH_SECURE_COOKIE"), "true")
	return &Config{
		Enabled:      true,
		Username:     user,
		Password:     pass,
		Secret:       []byte(secret),
		SecureCookie: secure,
	}
}

func (c *Config) ValidSession(r *http.Request) bool {
	if !c.Enabled {
		return true
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	return c.parseToken(cookie.Value)
}

func (c *Config) parseToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, c.Secret)
	mac.Write(payloadBytes)
	if subtle.ConstantTimeCompare(mac.Sum(nil), sig) != 1 {
		return false
	}
	payload := string(payloadBytes)
	idx := strings.LastIndexByte(payload, '|')
	if idx <= 0 {
		return false
	}
	expStr := payload[idx+1:]
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > exp {
		return false
	}
	user := payload[:idx]
	return subtle.ConstantTimeCompare([]byte(user), []byte(c.Username)) == 1
}

func (c *Config) mintToken() string {
	exp := time.Now().Add(time.Duration(maxAgeSec) * time.Second).Unix()
	payload := c.Username + "|" + strconv.FormatInt(exp, 10)
	b := []byte(payload)
	mac := hmac.New(sha256.New, c.Secret)
	mac.Write(b)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func ctEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// HandleLogin POST /api/login JSON {username,password}
func (c *Config) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if !c.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auth disabled"})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !ctEq(body.Username, c.Username) || !ctEq(body.Password, c.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    c.mintToken(),
		Path:     "/",
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.SecureCookie,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": c.Username})
}

// HandleLogout POST /api/logout
func (c *Config) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.SecureCookie,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleMe GET /api/me
func (c *Config) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !c.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{"authEnabled": false, "authenticated": true})
		return
	}
	if c.ValidSession(r) {
		writeJSON(w, http.StatusOK, map[string]any{"authEnabled": true, "authenticated": true, "user": c.Username})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authEnabled": true, "authenticated": false})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Register 注册登录相关路由（不受中间件拦截的路径由 Middleware 放行）
func Register(mux *http.ServeMux, c *Config) {
	mux.HandleFunc("/api/login", c.HandleLogin)
	mux.HandleFunc("/api/logout", c.HandleLogout)
	mux.HandleFunc("/api/me", c.HandleMe)
}

func isPublicPath(path string, r *http.Request) bool {
	if path == "/login.html" || path == "/login" {
		return true
	}
	if path == "/style.css" {
		return true
	}
	// 登录页使用独立脚本，主应用脚本需登录后加载
	if path == "/login.js" {
		return true
	}
	if path == "/api/login" && r.Method == http.MethodPost {
		return true
	}
	if path == "/api/logout" && r.Method == http.MethodPost {
		return true
	}
	if path == "/api/me" && r.Method == http.MethodGet {
		return true
	}
	return false
}

// Middleware 未启用时直通；启用时保护 / 与 /api/*（除公开路径）
func Middleware(c *Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if isPublicPath(path, r) {
			next.ServeHTTP(w, r)
			return
		}
		if c.ValidSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		http.Redirect(w, r, "/login.html", http.StatusFound)
	})
}
