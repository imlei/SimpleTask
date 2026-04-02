package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	cookieName = "simpletask_auth"
	maxAgeSec  = 7 * 24 * 3600 // 7 天
)

// Config 认证配置：默认启用登录；无用户时需先 /setup；AUTH_DISABLE 可关闭（等同旧版无密码）
type Config struct {
	Disabled     bool
	Store        *UserStore
	SecureCookie bool
}

// NewAuth 使用 SQLite 中的 app_user；尚无用户时可用环境变量 AUTH_PASSWORD 自动创建首个用户（迁移）
func NewAuth(db *sql.DB, dataDir string) (*Config, error) {
	if strings.EqualFold(os.Getenv("AUTH_DISABLE"), "1") || strings.EqualFold(os.Getenv("AUTH_DISABLE"), "true") {
		return &Config{Disabled: true}, nil
	}
	st, err := LoadUserStore(db, dataDir)
	if err != nil {
		return nil, err
	}
	if !st.HasUser() {
		pass := strings.TrimSpace(os.Getenv("AUTH_PASSWORD"))
		if pass != "" && len(pass) >= 6 {
			user := strings.TrimSpace(os.Getenv("AUTH_USER"))
			if user == "" {
				user = "admin"
			}
			if err := st.CreateFirstUser(user, pass); err != nil {
				return nil, err
			}
		}
	}
	secure := strings.EqualFold(os.Getenv("AUTH_SECURE_COOKIE"), "1") ||
		strings.EqualFold(os.Getenv("AUTH_SECURE_COOKIE"), "true")
	return &Config{Store: st, SecureCookie: secure}, nil
}

// NeedsSetup 尚无账户，需访问创建账户页
func (c *Config) NeedsSetup() bool {
	if c.Disabled || c.Store == nil {
		return false
	}
	return !c.Store.HasUser()
}

func (c *Config) sessionKey() []byte {
	if c.Store == nil {
		return nil
	}
	return c.Store.sessionKey()
}

func (c *Config) ValidSession(r *http.Request) bool {
	if c.Disabled {
		return true
	}
	if c.Store == nil || !c.Store.HasUser() {
		return false
	}
	key := c.sessionKey()
	if len(key) == 0 {
		return false
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	return c.parseToken(cookie.Value, key)
}

func (c *Config) parseToken(token string, key []byte) bool {
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
	mac := hmac.New(sha256.New, key)
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
	return subtle.ConstantTimeCompare([]byte(user), []byte(c.Store.Username())) == 1
}

func (c *Config) mintToken() string {
	exp := time.Now().Add(time.Duration(maxAgeSec) * time.Second).Unix()
	payload := c.Store.Username() + "|" + strconv.FormatInt(exp, 10)
	b := []byte(payload)
	key := c.sessionKey()
	mac := hmac.New(sha256.New, key)
	mac.Write(b)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func setSessionCookie(c *Config, w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    c.mintToken(),
		Path:     "/",
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.SecureCookie,
	})
}

// HandleLogin POST /api/login
func (c *Config) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if c.Disabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auth disabled"})
		return
	}
	if c.NeedsSetup() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请先创建账户"})
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
	if !c.Store.VerifyPassword(body.Username, body.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	setSessionCookie(c, w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": c.Store.Username()})
}

// HandleSetup POST /api/setup 仅尚无用户时可用
func (c *Config) HandleSetup(w http.ResponseWriter, r *http.Request) {
	if c.Disabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auth disabled"})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !c.NeedsSetup() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "已有账户"})
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
	if err := c.Store.CreateFirstUser(body.Username, body.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	setSessionCookie(c, w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": c.Store.Username()})
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
	if c.Disabled {
		writeJSON(w, http.StatusOK, map[string]any{"authEnabled": false, "authenticated": true})
		return
	}
	if c.NeedsSetup() {
		writeJSON(w, http.StatusOK, map[string]any{
			"authEnabled":   true,
			"needsSetup":    true,
			"authenticated": false,
		})
		return
	}
	if c.ValidSession(r) {
		writeJSON(w, http.StatusOK, map[string]any{
			"authEnabled":   true,
			"needsSetup":    false,
			"authenticated": true,
			"user":          c.Store.Username(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authEnabled":   true,
		"needsSetup":    false,
		"authenticated": false,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Register 登录、注册、会话
// HandleChangePassword POST /api/auth/password
func (c *Config) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if c.Disabled || c.Store == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auth disabled"})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !c.ValidSession(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := c.Store.ChangePassword(body.OldPassword, body.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	setSessionCookie(c, w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func Register(mux *http.ServeMux, c *Config) {
	mux.HandleFunc("/api/login", c.HandleLogin)
	mux.HandleFunc("/api/setup", c.HandleSetup)
	mux.HandleFunc("/api/logout", c.HandleLogout)
	mux.HandleFunc("/api/me", c.HandleMe)
	mux.HandleFunc("/api/auth/password", c.HandleChangePassword)
}

func isPublicPath(c *Config, path string, r *http.Request) bool {
	if c.Disabled {
		return true
	}
	if c.NeedsSetup() {
		switch path {
		case "/setup.html", "/setup", "/setup.js", "/style.css":
			return true
		}
		if path == "/api/setup" && r.Method == http.MethodPost {
			return true
		}
		if path == "/api/me" && r.Method == http.MethodGet {
			return true
		}
		if path == "/api/settings/public" && r.Method == http.MethodGet {
			return true
		}
		return false
	}
	switch path {
	case "/login.html", "/login", "/login.js", "/style.css", "/invoice.html", "/invoice.js":
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
	if path == "/api/settings/public" && r.Method == http.MethodGet {
		return true
	}
	return false
}

// Middleware 保护页面与 API
func Middleware(c *Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.Disabled {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if isPublicPath(c, path, r) {
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
		if c.NeedsSetup() {
			http.Redirect(w, r, "/setup.html", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/login.html", http.StatusFound)
	})
}
