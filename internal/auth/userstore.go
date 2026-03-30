package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// UserStore 单用户凭据，存于 SQLite 表 app_user
type UserStore struct {
	mu   sync.Mutex
	db   *sql.DB
	dir  string // 用于一次性从 users.json 迁移
	done bool   // 已尝试迁移
}

// LoadUserStore 从数据库加载（app_user 占位行由 store.Open 建表时插入）；可选从旧版 users.json 导入一次。
func LoadUserStore(db *sql.DB, dataDir string) (*UserStore, error) {
	u := &UserStore{db: db, dir: dataDir}
	if err := u.migrateFromJSONIfNeeded(); err != nil {
		return nil, err
	}
	return u, nil
}

type legacyUserFile struct {
	Username      string `json:"username"`
	PasswordHash  string `json:"passwordHash"`
	SessionSecret string `json:"sessionSecret"`
}

func (u *UserStore) migrateFromJSONIfNeeded() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.done {
		return nil
	}
	u.done = true
	var username string
	_ = u.db.QueryRow(`SELECT username FROM app_user WHERE id=1`).Scan(&username)
	if username != "" {
		return nil
	}
	path := filepath.Join(u.dir, "users.json")
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return nil
	}
	var leg legacyUserFile
	if json.Unmarshal(b, &leg) != nil || leg.Username == "" || leg.PasswordHash == "" {
		return nil
	}
	_, err = u.db.Exec(`UPDATE app_user SET username=?, password_hash=?, session_secret=? WHERE id=1`,
		leg.Username, leg.PasswordHash, leg.SessionSecret)
	return err
}

func (u *UserStore) HasUser() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	var name string
	_ = u.db.QueryRow(`SELECT username FROM app_user WHERE id=1`).Scan(&name)
	return name != ""
}

func (u *UserStore) loadLocked() (username, hash, secret string, err error) {
	err = u.db.QueryRow(`SELECT username, password_hash, session_secret FROM app_user WHERE id=1`).Scan(&username, &hash, &secret)
	return
}

// CreateFirstUser 仅在尚无用户时调用
func (u *UserStore) CreateFirstUser(username, password string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	var existing string
	_ = u.db.QueryRow(`SELECT username FROM app_user WHERE id=1`).Scan(&existing)
	if existing != "" {
		return errors.New("user already exists")
	}
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 6 {
		return errors.New("invalid username or password (min 6 chars)")
	}
	pwhash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	sec := make([]byte, 32)
	if _, err := rand.Read(sec); err != nil {
		return err
	}
	secret := hex.EncodeToString(sec)
	_, err = u.db.Exec(`UPDATE app_user SET username=?, password_hash=?, session_secret=? WHERE id=1`,
		username, string(pwhash), secret)
	return err
}

func (u *UserStore) VerifyPassword(username, password string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	name, hash, _, err := u.loadLocked()
	if err != nil || name == "" {
		return false
	}
	if username != name {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (u *UserStore) sessionKey() []byte {
	u.mu.Lock()
	defer u.mu.Unlock()
	_, _, secret, err := u.loadLocked()
	if err != nil || secret == "" {
		return nil
	}
	b, err := hex.DecodeString(secret)
	if err != nil || len(b) == 0 {
		return nil
	}
	return b
}

// Username 当前登录名（会话校验成功后与之一致）
func (u *UserStore) Username() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	var name string
	_ = u.db.QueryRow(`SELECT username FROM app_user WHERE id=1`).Scan(&name)
	return name
}

// ChangePassword 验证旧密码后更新密码并轮换 session_secret（使其它端会话失效）
func (u *UserStore) ChangePassword(oldPassword, newPassword string) error {
	if len(newPassword) < 6 {
		return errors.New("新密码至少 6 位")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	name, hash, _, err := u.loadLocked()
	if err != nil || name == "" {
		return errors.New("no user")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPassword)) != nil {
		return errors.New("当前密码错误")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	sec := make([]byte, 32)
	if _, err := rand.Read(sec); err != nil {
		return err
	}
	secret := hex.EncodeToString(sec)
	_, err = u.db.Exec(`UPDATE app_user SET password_hash=?, session_secret=? WHERE id=1`, string(newHash), secret)
	return err
}
