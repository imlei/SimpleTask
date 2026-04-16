package auth

import (
	"context"
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

type ctxKey int

const (
	ctxKeyUsername ctxKey = iota
	ctxKeyRole
)

// UsernameFromContext returns the authenticated username stored in ctx.
func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyUsername).(string)
	return v
}

// RoleFromContext returns the authenticated role stored in ctx.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRole).(string)
	return v
}

// Config 认证配置
type Config struct {
	Disabled     bool
	Store        *UserStore
	SecureCookie bool
}

// NewAuth 初始化认证配置
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

// NeedsSetup reports whether no admin account exists yet.
func (c *Config) NeedsSetup() bool {
	if c.Disabled || c.Store == nil {
		return false
	}
	return !c.Store.HasUser()
}

// ValidSessionInfo validates the session cookie and returns (username, role, ok).
func (c *Config) ValidSessionInfo(r *http.Request) (username, role string, ok bool) {
	if c.Disabled {
		return "admin", "admin", true
	}
	if c.Store == nil || !c.Store.HasUser() {
		return "", "", false
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return "", "", false
	}
	return c.parseTokenInfo(cookie.Value)
}

// parseTokenInfo parses and verifies a token, returning (username, role, ok).
func (c *Config) parseTokenInfo(token string) (username, role string, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return
	}
	payload := string(payloadBytes)
	idx := strings.LastIndexByte(payload, '|')
	if idx <= 0 {
		return
	}
	uname := payload[:idx]
	expStr := payload[idx+1:]
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return
	}
	if time.Now().Unix() > exp {
		return
	}
	key := c.Store.SessionKeyFor(uname)
	if len(key) == 0 {
		return
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payloadBytes)
	if subtle.ConstantTimeCompare(mac.Sum(nil), sig) != 1 {
		return
	}
	roleStr := c.Store.GetRole(uname)
	return uname, roleStr, true
}

// ValidSession reports whether the request has a valid session.
func (c *Config) ValidSession(r *http.Request) bool {
	_, _, ok := c.ValidSessionInfo(r)
	return ok
}

// mintTokenFor mints a signed session token for the given username.
func (c *Config) mintTokenFor(username string) string {
	exp := time.Now().Add(time.Duration(maxAgeSec) * time.Second).Unix()
	payload := username + "|" + strconv.FormatInt(exp, 10)
	b := []byte(payload)
	key := c.Store.SessionKeyFor(username)
	mac := hmac.New(sha256.New, key)
	mac.Write(b)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func setSessionCookieFor(c *Config, w http.ResponseWriter, username string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    c.mintTokenFor(username),
		Path:     "/",
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.SecureCookie,
	})
}

// setSessionCookie mints a token for the admin (used by HandleSetup).
func setSessionCookie(c *Config, w http.ResponseWriter) {
	setSessionCookieFor(c, w, c.Store.Username())
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
	info, ok := c.Store.VerifyPassword(body.Username, body.Password)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	setSessionCookieFor(c, w, info.Username)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": info.Username, "role": info.Role})
}

// HandleSetup POST /api/setup — only available when no user exists
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
	username, role, ok := c.ValidSessionInfo(r)
	if ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"authEnabled":   true,
			"needsSetup":    false,
			"authenticated": true,
			"user":          username,
			"role":          role,
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
	username, _, ok := c.ValidSessionInfo(r)
	if !ok {
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
	if err := c.Store.ChangePasswordFor(username, body.OldPassword, body.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	setSessionCookieFor(c, w, username)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleUsers GET/POST/DELETE/PUT /api/users — admin only
func (c *Config) HandleUsers(w http.ResponseWriter, r *http.Request) {
	if c.Disabled || c.Store == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auth disabled"})
		return
	}
	_, role, ok := c.ValidSessionInfo(r)
	if !ok || role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin only"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := c.Store.ListUsers()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if users == nil {
			users = []UserInfo{}
		}
		writeJSON(w, http.StatusOK, users)

	case http.MethodPost:
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := c.Store.CreateUser(body.Username, body.Password, body.Role); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodDelete:
		var body struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := c.Store.DeleteUser(body.Username); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodPut:
		var body struct {
			Username    string `json:"username"`
			Role        string `json:"role,omitempty"`
			NewPassword string `json:"newPassword,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Role != "" {
			if err := c.Store.SetUserRole(body.Username, body.Role); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		if body.NewPassword != "" {
			if err := c.Store.AdminResetPassword(body.Username, body.NewPassword); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// HandleRegister POST /api/register — open registration, always creates a user1 account
func (c *Config) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if c.Disabled {
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
	if err := c.Store.CreateUser(body.Username, body.Password, "user1"); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	setSessionCookieFor(c, w, body.Username)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": body.Username, "role": "user1"})
}

// Register registers all auth-related routes.
func Register(mux *http.ServeMux, c *Config) {
	mux.HandleFunc("/api/login", c.HandleLogin)
	mux.HandleFunc("/api/setup", c.HandleSetup)
	mux.HandleFunc("/api/logout", c.HandleLogout)
	mux.HandleFunc("/api/me", c.HandleMe)
	mux.HandleFunc("/api/auth/password", c.HandleChangePassword)
	mux.HandleFunc("/api/users", c.HandleUsers)
	mux.HandleFunc("/api/register", c.HandleRegister)
}

func isPublicPath(c *Config, path string, r *http.Request) bool {
	if c.Disabled {
		return true
	}
	if path == "/api/version" && r.Method == http.MethodGet {
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
	// Admin login page is public
	switch path {
	case "/admin/login.html", "/admin/admin.js":
		return true
	}
	switch path {
	case "/login.html", "/login", "/login.js", "/style.css", "/invoice.html", "/invoice.js":
		return true
	}
	if path == "/api/login" && r.Method == http.MethodPost {
		return true
	}
	if path == "/api/register" && r.Method == http.MethodPost {
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

// isAllowedForRole reports whether the given role may access the given path.
func isAllowedForRole(role, path string) bool {
	switch role {
	case "admin":
		return true
	case "user2":
		// Full app access, but no user management and no admin module
		if path == "/api/users" || strings.HasPrefix(path, "/api/users/") {
			return false
		}
		if strings.HasPrefix(path, "/admin/") {
			return false
		}
		return true
	case "user1":
		// Payroll pages + payroll API only
		if strings.HasPrefix(path, "/payroll/") {
			return true
		}
		if strings.HasPrefix(path, "/api/payroll/") {
			return true
		}
		switch path {
		case "/api/auth/password", "/api/logout", "/api/me", "/api/version":
			return true
		}
		return false
	}
	return false
}

// csrfSafe validates Origin/Referer to block cross-origin state-changing requests.
func csrfSafe(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	host := r.Host
	if host == "" {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin != "" {
		if idx := strings.Index(origin, "://"); idx >= 0 {
			originHost := origin[idx+3:]
			return originHost == host
		}
		return false
	}
	referer := r.Header.Get("Referer")
	if referer != "" {
		if idx := strings.Index(referer, "://"); idx >= 0 {
			rest := referer[idx+3:]
			refHost := rest
			if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
				refHost = rest[:slashIdx]
			}
			return refHost == host
		}
		return false
	}
	return true
}

// Middleware protects all routes with authentication and role-based access control.
func Middleware(c *Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.Disabled {
			next.ServeHTTP(w, r)
			return
		}
		if !csrfSafe(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin request blocked"})
			return
		}
		path := r.URL.Path
		if isPublicPath(c, path, r) {
			next.ServeHTTP(w, r)
			return
		}
		username, role, ok := c.ValidSessionInfo(r)
		if ok {
			if !isAllowedForRole(role, path) {
				if strings.HasPrefix(path, "/api/") {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
				} else if strings.HasPrefix(path, "/admin/") {
					http.Redirect(w, r, "/admin/login.html", http.StatusFound)
				} else if role == "user1" {
					http.Redirect(w, r, "/payroll/dashboard.html", http.StatusFound)
				} else {
					http.Redirect(w, r, "/", http.StatusFound)
				}
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUsername, username)
			ctx = context.WithValue(ctx, ctxKeyRole, role)
			next.ServeHTTP(w, r.WithContext(ctx))
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
		// Unauthenticated access to admin module → admin login
		if strings.HasPrefix(path, "/admin/") {
			http.Redirect(w, r, "/admin/login.html", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/login.html", http.StatusFound)
	})
}
