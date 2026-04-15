package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"simpletask/internal/models"
)

const (
	schema = `
CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  customer_id TEXT NOT NULL DEFAULT '',
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
	current := filepath.Join(dir, "SimpleTask.db")
	legacy := filepath.Join(dir, "biztracker.db")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}

// Open 打开 SQLite（DATA_DIR/SimpleTask.db，若仅存在旧版 biztracker.db 则沿用），建表并从旧版 data.json / users.json 导入一次（若库为空）。
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
	if err := ensureAppUserRoleColumn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureSubUsersTable(db); err != nil {
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
	if err := ensureAppSettingsMICRColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureBankAccounts(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateBankAccountsFromLegacySettings(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureCustomersTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureCustomerExtraColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureTaskCustomerIDColumn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateLegacyJSON(dir, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateCustomersBackfill(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExpensesTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExpenseAccountCodeColumn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExpenseCodesCatalogTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExpenseDateColumn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExpenseVendorsTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExpenseVendorIDColumn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureBaseCurrencyColumn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureCompanyContactColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExchangeRatesTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureExchangeRateWatchlistTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensurePayrollCompaniesTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensurePayrollEmployeesTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureCustomersTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT ''
);`)
	return err
}

func ensureCustomerExtraColumns(db *sql.DB) error {
	want := []struct {
		Name string
		DDL  string
	}{
		{Name: "email", DDL: "ALTER TABLE customers ADD COLUMN email TEXT NOT NULL DEFAULT ''"},
		{Name: "phone", DDL: "ALTER TABLE customers ADD COLUMN phone TEXT NOT NULL DEFAULT ''"},
		{Name: "address", DDL: "ALTER TABLE customers ADD COLUMN address TEXT NOT NULL DEFAULT ''"},
		{Name: "status", DDL: "ALTER TABLE customers ADD COLUMN status TEXT NOT NULL DEFAULT 'active'"},
		{Name: "display_name", DDL: "ALTER TABLE customers ADD COLUMN display_name TEXT NOT NULL DEFAULT ''"},
	}
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(customers)`)
	if err != nil {
		return err
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
			continue
		}
	}
	return nil
}

func ensureTaskCustomerIDColumn(db *sql.DB) error {
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		return err
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
	if !existing["customer_id"] {
		if _, err := db.Exec(`ALTER TABLE tasks ADD COLUMN customer_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

// migrateCustomersBackfill 为旧任务按 company_name 生成客户并关联（同一公司名共用一客户）
func migrateCustomersBackfill(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, company_name FROM tasks WHERE IFNULL(customer_id,'') = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type pair struct {
		id, cn string
	}
	var list []pair
	for rows.Next() {
		var id, cn string
		if err := rows.Scan(&id, &cn); err != nil {
			continue
		}
		list = append(list, pair{id: id, cn: strings.TrimSpace(cn)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	nameToID := map[string]string{}
	for _, p := range list {
		if p.cn == "" {
			continue
		}
		cid, ok := nameToID[p.cn]
		if !ok {
			cid = nextCustomerIDFromMax(db)
			if _, err := db.Exec(`INSERT INTO customers (id, name) VALUES (?,?)`, cid, p.cn); err != nil {
				return err
			}
			nameToID[p.cn] = cid
		}
		if _, err := db.Exec(`UPDATE tasks SET customer_id=? WHERE id=?`, cid, p.id); err != nil {
			return err
		}
	}
	return nil
}

func nextCustomerIDFromMax(db *sql.DB) string {
	rows, err := db.Query(`SELECT id FROM customers WHERE id LIKE 'C%'`)
	if err != nil {
		return "C0001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "C") && len(id) > 1 {
			if v, err := strconv.Atoi(strings.TrimPrefix(id, "C")); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("C%04d", max+1)
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
  smtp_tls INTEGER NOT NULL DEFAULT 0,
  micr_country TEXT NOT NULL DEFAULT 'CA',
  bank_institution TEXT NOT NULL DEFAULT '',
  bank_transit TEXT NOT NULL DEFAULT '',
  bank_routing_aba TEXT NOT NULL DEFAULT '',
  bank_account TEXT NOT NULL DEFAULT '',
  bank_cheque_number TEXT NOT NULL DEFAULT '000001',
  micr_line_override TEXT NOT NULL DEFAULT '',
  default_cheque_currency TEXT NOT NULL DEFAULT 'CAD',
  default_bank_account_id TEXT NOT NULL DEFAULT '',
  base_currency TEXT NOT NULL DEFAULT 'CAD'
);
INSERT OR IGNORE INTO app_settings (id) VALUES (1);
`)
	return err
}

func ensureBaseCurrencyColumn(db *sql.DB) error {
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(app_settings)`)
	if err != nil {
		return err
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
	if !existing["base_currency"] {
		if _, err := db.Exec(`ALTER TABLE app_settings ADD COLUMN base_currency TEXT NOT NULL DEFAULT 'CAD'`); err != nil {
			return err
		}
	}
	return nil
}

func ensureCompanyContactColumns(db *sql.DB) error {
	want := []struct {
		Name string
		DDL  string
	}{
		{Name: "company_phone", DDL: "ALTER TABLE app_settings ADD COLUMN company_phone TEXT NOT NULL DEFAULT ''"},
		{Name: "company_fax", DDL: "ALTER TABLE app_settings ADD COLUMN company_fax TEXT NOT NULL DEFAULT ''"},
		{Name: "company_address", DDL: "ALTER TABLE app_settings ADD COLUMN company_address TEXT NOT NULL DEFAULT ''"},
		{Name: "company_email", DDL: "ALTER TABLE app_settings ADD COLUMN company_email TEXT NOT NULL DEFAULT ''"},
	}
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(app_settings)`)
	if err != nil {
		return err
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
			return err
		}
	}
	return nil
}

func ensureExchangeRatesTable(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS exchange_rates (
  requested_date TEXT NOT NULL,
  base_code TEXT NOT NULL,
  quote_code TEXT NOT NULL,
  rate REAL NOT NULL,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (requested_date, base_code, quote_code)
);`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_exchange_rates_base_date ON exchange_rates (base_code, requested_date);`)
	return err
}

func ensureExchangeRateWatchlistTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS exchange_rate_watchlist (
  code TEXT PRIMARY KEY,
  sort_order INTEGER NOT NULL DEFAULT 0
);`)
	return err
}

func ensureAppSettingsMICRColumns(db *sql.DB) error {
	type colDef struct {
		Name string
		DDL  string
	}
	want := []colDef{
		{Name: "micr_country", DDL: "ALTER TABLE app_settings ADD COLUMN micr_country TEXT NOT NULL DEFAULT 'CA'"},
		{Name: "bank_institution", DDL: "ALTER TABLE app_settings ADD COLUMN bank_institution TEXT NOT NULL DEFAULT ''"},
		{Name: "bank_transit", DDL: "ALTER TABLE app_settings ADD COLUMN bank_transit TEXT NOT NULL DEFAULT ''"},
		{Name: "bank_routing_aba", DDL: "ALTER TABLE app_settings ADD COLUMN bank_routing_aba TEXT NOT NULL DEFAULT ''"},
		{Name: "bank_account", DDL: "ALTER TABLE app_settings ADD COLUMN bank_account TEXT NOT NULL DEFAULT ''"},
		{Name: "bank_cheque_number", DDL: "ALTER TABLE app_settings ADD COLUMN bank_cheque_number TEXT NOT NULL DEFAULT '000001'"},
		{Name: "micr_line_override", DDL: "ALTER TABLE app_settings ADD COLUMN micr_line_override TEXT NOT NULL DEFAULT ''"},
		{Name: "default_cheque_currency", DDL: "ALTER TABLE app_settings ADD COLUMN default_cheque_currency TEXT NOT NULL DEFAULT 'CAD'"},
		{Name: "default_bank_account_id", DDL: "ALTER TABLE app_settings ADD COLUMN default_bank_account_id TEXT NOT NULL DEFAULT ''"},
	}
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(app_settings)`)
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
			continue
		}
	}
	return nil
}

func ensureBankAccounts(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS bank_accounts (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL DEFAULT '',
  bank_name TEXT NOT NULL DEFAULT '',
  micr_country TEXT NOT NULL DEFAULT 'CA',
  bank_institution TEXT NOT NULL DEFAULT '',
  bank_transit TEXT NOT NULL DEFAULT '',
  bank_routing_aba TEXT NOT NULL DEFAULT '',
  bank_account TEXT NOT NULL DEFAULT '',
  bank_iban TEXT NOT NULL DEFAULT '',
  bank_swift TEXT NOT NULL DEFAULT '',
  bank_cheque_number TEXT NOT NULL DEFAULT '000001',
  micr_line_override TEXT NOT NULL DEFAULT '',
  default_cheque_currency TEXT NOT NULL DEFAULT 'CAD',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);`)
	if err != nil {
		return err
	}
	type colDef struct {
		Name string
		DDL  string
	}
	want := []colDef{
		{Name: "bank_name", DDL: "ALTER TABLE bank_accounts ADD COLUMN bank_name TEXT NOT NULL DEFAULT ''"},
		{Name: "bank_iban", DDL: "ALTER TABLE bank_accounts ADD COLUMN bank_iban TEXT NOT NULL DEFAULT ''"},
		{Name: "bank_swift", DDL: "ALTER TABLE bank_accounts ADD COLUMN bank_swift TEXT NOT NULL DEFAULT ''"},
	}
	existing := map[string]bool{}
	rows, e := db.Query(`PRAGMA table_info(bank_accounts)`)
	if e != nil {
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
			continue
		}
	}
	return nil
}

// migrateBankAccountsFromLegacySettings 将旧版 app_settings 中的单账户字段迁移到 bank_accounts（仅当 bank_accounts 为空时执行一次）。
func migrateBankAccountsFromLegacySettings(db *sql.DB) error {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM bank_accounts`).Scan(&n)
	if n > 0 {
		return nil
	}
	// 读取旧字段（若全空则不创建）
	var mc, inst, tr, rt, ac, chq, ovr, cur string
	_ = db.QueryRow(`SELECT COALESCE(micr_country,''), COALESCE(bank_institution,''), COALESCE(bank_transit,''), COALESCE(bank_routing_aba,''),
		COALESCE(bank_account,''), COALESCE(bank_cheque_number,''), COALESCE(micr_line_override,''), COALESCE(default_cheque_currency,'')
		FROM app_settings WHERE id=1`).Scan(&mc, &inst, &tr, &rt, &ac, &chq, &ovr, &cur)
	all := strings.TrimSpace(inst) + strings.TrimSpace(tr) + strings.TrimSpace(rt) + strings.TrimSpace(ac) + strings.TrimSpace(ovr)
	if strings.TrimSpace(all) == "" {
		return nil
	}
	if strings.TrimSpace(chq) == "" {
		chq = "000001"
	}
	if strings.TrimSpace(cur) == "" {
		cur = "CAD"
	}
	if strings.TrimSpace(mc) == "" {
		mc = "CA"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO bank_accounts (id, label, micr_country, bank_institution, bank_transit, bank_routing_aba, bank_account,
		bank_cheque_number, micr_line_override, default_cheque_currency, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"B0001", "Default", strings.ToUpper(strings.TrimSpace(mc)),
		strings.TrimSpace(inst), strings.TrimSpace(tr), strings.TrimSpace(rt), strings.TrimSpace(ac),
		strings.TrimSpace(chq), strings.TrimSpace(ovr), strings.ToUpper(strings.TrimSpace(cur)),
		now, now,
	)
	if err != nil {
		return err
	}
	_, _ = db.Exec(`UPDATE app_settings SET default_bank_account_id=? WHERE id=1`, "B0001")
	return nil
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
		{Name: "task_ids_json", DDL: "ALTER TABLE invoices ADD COLUMN task_ids_json TEXT NOT NULL DEFAULT '[]'"},
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
	_, err = tx.Exec(`INSERT INTO tasks (id, customer_id, company_name, date, service1, service2, price1, price2, status, completed_at, note, selected_price_ids)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.CustomerID, t.CompanyName, t.Date, t.Service1, t.Service2, t.Price1, t.Price2, string(t.Status), t.CompletedAt, t.Note, string(sel))
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

func ensureExpensesTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS expenses (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL DEFAULT '',
  expense_date TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  account_code TEXT NOT NULL DEFAULT '',
  amount REAL NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'CAD',
  created_at TEXT NOT NULL DEFAULT ''
);`)
	return err
}

func ensureExpenseAccountCodeColumn(db *sql.DB) error {
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(expenses)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		existing[name] = true
	}
	if !existing["account_code"] {
		_, err := db.Exec(`ALTER TABLE expenses ADD COLUMN account_code TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureExpenseCodesCatalogTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS expense_codes (
  code TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT ''
);`)
	return err
}

func ensureExpenseDateColumn(db *sql.DB) error {
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(expenses)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		existing[name] = true
	}
	if !existing["expense_date"] {
		if _, err := db.Exec(`ALTER TABLE expenses ADD COLUMN expense_date TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	_, _ = db.Exec(`UPDATE expenses SET expense_date = substr(created_at, 1, 10) WHERE IFNULL(expense_date,'') = '' AND length(created_at) >= 10`)
	return nil
}

func ensureExpenseVendorsTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS expense_vendors (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL DEFAULT 'CAD',
  email TEXT NOT NULL DEFAULT '',
  address TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);`)
	return err
}

func ensureExpenseVendorIDColumn(db *sql.DB) error {
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(expenses)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		existing[name] = true
	}
	if !existing["vendor_id"] {
		if _, err := db.Exec(`ALTER TABLE expenses ADD COLUMN vendor_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

func ensureAppUserRoleColumn(db *sql.DB) error {
	existing := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(app_user)`)
	if err != nil {
		return err
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
	if !existing["role"] {
		_, err := db.Exec(`ALTER TABLE app_user ADD COLUMN role TEXT NOT NULL DEFAULT 'admin'`)
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureSubUsersTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS app_sub_users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL DEFAULT '',
  session_secret TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT 'user2'
);`)
	return err
}

func ensurePayrollCompaniesTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_companies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  legal_name TEXT NOT NULL DEFAULT '',
  business_number TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  address TEXT NOT NULL DEFAULT '',
  province TEXT NOT NULL DEFAULT '',
  pay_frequency TEXT NOT NULL DEFAULT 'biweekly',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);`)
	return err
}

func ensurePayrollEmployeesTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_employees (
  id TEXT PRIMARY KEY,
  company_id TEXT NOT NULL DEFAULT '',
  legal_name TEXT NOT NULL DEFAULT '',
  nickname TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  mobile TEXT NOT NULL DEFAULT '',
  member_type INTEGER NOT NULL DEFAULT 0,
  position TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  province TEXT NOT NULL DEFAULT '',
  sin_encrypted TEXT NOT NULL DEFAULT '',
  date_of_birth TEXT NOT NULL DEFAULT '',
  hire_date TEXT NOT NULL DEFAULT '',
  salary_type INTEGER NOT NULL DEFAULT 1,
  pay_rate REAL NOT NULL DEFAULT 0,
  pay_rate_unit TEXT NOT NULL DEFAULT 'Hourly',
  pays_per_year INTEGER NOT NULL DEFAULT 26,
  pay_frequency TEXT NOT NULL DEFAULT 'biweekly',
  hours_per_week REAL NOT NULL DEFAULT 0,
  td1_federal REAL NOT NULL DEFAULT 16129,
  td1_provincial REAL NOT NULL DEFAULT 0,
  paid_ytd_other_payroll INTEGER NOT NULL DEFAULT 0,
  auto_vacation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_payroll_employees_company ON payroll_employees (company_id);
`)
	return err
}
