package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"tasktracker/internal/models"
)

const (
	schema = `
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  company_name TEXT NOT NULL DEFAULT '',
  date TEXT NOT NULL DEFAULT '',
  service1 TEXT NOT NULL DEFAULT '',
  service2 TEXT NOT NULL DEFAULT '',
  price1 REAL NOT NULL DEFAULT 0,
  price2 REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'Pending',
  completed_at TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  selected_price_ids TEXT NOT NULL DEFAULT '[]'
);
CREATE TABLE IF NOT EXISTS price_items (
  id TEXT PRIMARY KEY,
  service_name TEXT NOT NULL DEFAULT '',
  amount REAL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  note TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS app_user (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  username TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  session_secret TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS invoices (
  id TEXT PRIMARY KEY,
  invoice_no TEXT NOT NULL UNIQUE,
  task_id TEXT NOT NULL DEFAULT '',
  invoice_date TEXT NOT NULL DEFAULT '',
  terms TEXT NOT NULL DEFAULT '',
  due_date TEXT NOT NULL DEFAULT '',
  bill_to_name TEXT NOT NULL DEFAULT '',
  bill_to_addr TEXT NOT NULL DEFAULT '',
  ship_to_name TEXT NOT NULL DEFAULT '',
  ship_to_addr TEXT NOT NULL DEFAULT '',
  bill_to_email TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL DEFAULT 'USD',
  tax_rate REAL NOT NULL DEFAULT 0,
  items_json TEXT NOT NULL DEFAULT '[]',
  subtotal REAL NOT NULL DEFAULT 0,
  tax_amount REAL NOT NULL DEFAULT 0,
  total REAL NOT NULL DEFAULT 0,
  balance_due REAL NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'Draft',
  sent_at TEXT NOT NULL DEFAULT '',
  paid_amount REAL NOT NULL DEFAULT 0,
  paid_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
`
	seedUser = `INSERT OR IGNORE INTO app_user (id, username, password_hash, session_secret) VALUES (1, '', '', '');`
)

func resolveSQLitePath(dir string) string {
	current := filepath.Join(dir, "tasktracker.db")
	legacy := filepath.Join(dir, "biztracker.db")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}

// Open 打开 SQLite（DATA_DIR/tasktracker.db，若仅存在旧版 biztracker.db 则沿用），建表并从旧版 data.json / users.json 导入一次（若库为空）。
func Open(dir string) (*sql.DB, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	dsn := resolveSQLitePath(dir)
	db, err := sql.Open("sqlite", dsn+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(seedUser); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureInvoiceColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureAppSettings(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateLegacyJSON(dir, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureAppSettings(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS app_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  company_name TEXT NOT NULL DEFAULT '',
  logo_data_url TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  smtp_host TEXT NOT NULL DEFAULT '',
  smtp_port INTEGER NOT NULL DEFAULT 587,
  smtp_user TEXT NOT NULL DEFAULT '',
  smtp_pass TEXT NOT NULL DEFAULT '',
  smtp_from TEXT NOT NULL DEFAULT '',
  smtp_starttls INTEGER NOT NULL DEFAULT 1,
  smtp_tls INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO app_settings (id) VALUES (1);
`)
	return err
}

func ensureInvoiceColumns(db *sql.DB) error {
	// 对已有数据库做增量迁移（CREATE TABLE IF NOT EXISTS 不会补列）
	type colDef struct {
		Name string
		DDL  string
	}
	want := []colDef{
		{Name: "bill_to_email", DDL: "ALTER TABLE invoices ADD COLUMN bill_to_email TEXT NOT NULL DEFAULT ''"},
		{Name: "status", DDL: "ALTER TABLE invoices ADD COLUMN status TEXT NOT NULL DEFAULT 'Draft'"},
		{Name: "sent_at", DDL: "ALTER TABLE invoices ADD COLUMN sent_at TEXT NOT NULL DEFAULT ''"},
		{Name: "paid_amount", DDL: "ALTER TABLE invoices ADD COLUMN paid_amount REAL NOT NULL DEFAULT 0"},
		{Name: "paid_at", DDL: "ALTER TABLE invoices ADD COLUMN paid_at TEXT NOT NULL DEFAULT ''"},
	}
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(invoices)`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		existing[name] = true
	}
	for _, c := range want {
		if existing[c.Name] {
			continue
		}
		if _, err := db.Exec(c.DDL); err != nil {
			// 忽略并继续，避免中途阻塞启动
			continue
		}
	}
	return nil
}

type legacyData struct {
	Tasks      []models.Task      `json:"tasks"`
	PriceItems []models.PriceItem `json:"priceItems"`
}

func migrateLegacyJSON(dir string, db *sql.DB) error {
	var nTasks, nPrices int
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&nTasks)
	_ = db.QueryRow(`SELECT COUNT(*) FROM price_items`).Scan(&nPrices)
	if nTasks > 0 || nPrices > 0 {
		return nil
	}
	path := filepath.Join(dir, "data.json")
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		for _, p := range DefaultPriceList() {
			if err := insertPriceTx(tx, p); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		return tx.Commit()
	}
	var data legacyData
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, t := range data.Tasks {
		if err := insertTaskTx(tx, t); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	prices := data.PriceItems
	if len(prices) == 0 {
		prices = DefaultPriceList()
	}
	for _, p := range prices {
		if err := insertPriceTx(tx, p); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func insertTaskTx(tx *sql.Tx, t models.Task) error {
	if t.Status == "" {
		t.Status = models.StatusPending
	}
	sel, err := json.Marshal(t.SelectedPriceIDs)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO tasks (id, company_name, date, service1, service2, price1, price2, status, completed_at, note, selected_price_ids)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.CompanyName, t.Date, t.Service1, t.Service2, t.Price1, t.Price2, string(t.Status), t.CompletedAt, t.Note, string(sel))
	return err
}

func insertPriceTx(tx *sql.Tx, p models.PriceItem) error {
	var amt any
	if p.Amount != nil {
		amt = *p.Amount
	}
	_, err := tx.Exec(`INSERT INTO price_items (id, service_name, amount, currency, note) VALUES (?,?,?,?,?)`,
		p.ID, p.ServiceName, amt, string(p.Currency), p.Note)
	return err
}
