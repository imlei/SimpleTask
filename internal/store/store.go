package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"simpletask/internal/models"
	"simpletask/internal/money"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrTaskLocked       = errors.New("task locked")
	ErrTaskPaidLocked   = errors.New("paid task cannot be modified")
	ErrTaskDeleteLocked = errors.New("only pending tasks can be deleted")
	ErrCustomerInactive = errors.New("customer is inactive")
)

// 任务列表等处 Customer 列：有简称用简称，否则全名
const sqlTaskCustomerDisplayName = `COALESCE(NULLIF(TRIM(COALESCE(c.display_name,'')), ''), COALESCE(c.name,''), '')`

const maxCustomerDisplayNameRunes = 20

func normalizeCustomerDisplayName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxCustomerDisplayNameRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxCustomerDisplayNameRunes])
}

func isActiveCustomerStatus(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "" || s == "active"
}

type Store struct {
	db      *sql.DB
	mu      sync.Mutex
	taskSeq int
}

func New(db *sql.DB) *Store {
	s := &Store{db: db}
	s.rebuildTaskSeq()
	return s
}

func (s *Store) rebuildTaskSeq() {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id FROM tasks`)
	if err != nil {
		return
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if n := parseNumericSuffix(id); n > max {
			max = n
		}
	}
	s.taskSeq = max
}

func parseNumericSuffix(id string) int {
	n := 0
	started := false
	for _, r := range id {
		if r >= '0' && r <= '9' {
			started = true
			n = n*10 + int(r-'0')
		} else if started {
			break
		}
	}
	return n
}

// nextTaskIDLocked 生成下一个任务ID，调用者必须持有 s.mu
func (s *Store) nextTaskIDLocked(prefix string) string {
	s.taskSeq++
	if prefix == "" {
		prefix = "AC"
	}
	return prefix + fmt.Sprintf("%04d", s.taskSeq)
}

func scanTaskCols(sc interface {
	Scan(dest ...any) error
}) (models.Task, error) {
	var t models.Task
	var status string
	var sel string
	err := sc.Scan(
		&t.ID,
		&t.CustomerID,
		&t.CompanyName,
		&t.Date,
		&t.Service1,
		&t.Service2,
		&t.Price1,
		&t.Price2,
		&status,
		&t.CompletedAt,
		&t.Note,
		&sel,
	)
	if err != nil {
		return models.Task{}, err
	}
	t.Status = models.TaskStatus(status)
	if sel != "" {
		_ = json.Unmarshal([]byte(sel), &t.SelectedPriceIDs)
	}
	return t, nil
}

func scanTaskJoin(sc interface {
	Scan(dest ...any) error
}) (models.Task, error) {
	var t models.Task
	var status string
	var sel string
	err := sc.Scan(
		&t.ID,
		&t.CustomerID,
		&t.CompanyName,
		&t.Date,
		&t.Service1,
		&t.Service2,
		&t.Price1,
		&t.Price2,
		&status,
		&t.CompletedAt,
		&t.Note,
		&sel,
		&t.CustomerName,
		&t.CustomerStatus,
	)
	if err != nil {
		return models.Task{}, err
	}
	t.Status = models.TaskStatus(status)
	if sel != "" {
		_ = json.Unmarshal([]byte(sel), &t.SelectedPriceIDs)
	}
	return t, nil
}

func sliceContainsStr(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

func currencyKey(c models.Currency) string {
	k := string(c)
	if k == "" {
		return "CNY"
	}
	return k
}

// recomputeTaskPricesFromSelection 与前端 applyTaskPriceSelection 一致：按勾选顺序汇总服务名与首币种金额
func recomputeTaskPricesFromSelection(ids []string, priceMap map[string]models.PriceItem) (service1 string, price1 float64) {
	var names []string
	byCur := map[string]float64{}
	var curOrder []string
	for _, id := range ids {
		p, ok := priceMap[id]
		if !ok {
			continue
		}
		names = append(names, p.ServiceName)
		if p.Amount == nil {
			continue
		}
		c := currencyKey(p.Currency)
		if _, ok := byCur[c]; !ok {
			curOrder = append(curOrder, c)
		}
		byCur[c] += *p.Amount
	}
	service1 = strings.Join(names, "；")
	if len(curOrder) == 0 {
		return service1, 0
	}
	price1 = byCur[curOrder[0]]
	return service1, price1
}

const taskCols = `id, customer_id, company_name, date, service1, service2, price1, price2, status, completed_at, note, selected_price_ids`

func (s *Store) GetTask(id string) (models.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT t.id, t.customer_id, t.company_name, t.date, t.service1, t.service2, t.price1, t.price2, t.status, t.completed_at, t.note, t.selected_price_ids, `+sqlTaskCustomerDisplayName+`, COALESCE(c.status,'active') FROM tasks t LEFT JOIN customers c ON c.id = t.customer_id WHERE t.id=?`, id)
	t, err := scanTaskJoin(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Task{}, ErrNotFound
		}
		return models.Task{}, err
	}
	return t, nil
}

func (s *Store) ListTasks() []models.Task {
	rows, err := s.db.Query(`SELECT t.id, t.customer_id, t.company_name, t.date, t.service1, t.service2, t.price1, t.price2, t.status, t.completed_at, t.note, t.selected_price_ids, ` + sqlTaskCustomerDisplayName + `, COALESCE(c.status,'active') FROM tasks t LEFT JOIN customers c ON c.id = t.customer_id ORDER BY t.id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]models.Task, 0)
	for rows.Next() {
		t, err := scanTaskJoin(rows)
		if err != nil {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (s *Store) CreateTask(t models.Task) models.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.ID == "" {
		t.ID = s.nextTaskIDLocked("AC")
	}
	if t.Status == "" {
		t.Status = models.StatusPending
	}
	sel, _ := json.Marshal(t.SelectedPriceIDs)
	if sel == nil {
		sel = []byte("[]")
	}
	_, err := s.db.Exec(`INSERT INTO tasks (`+taskCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.CustomerID, t.CompanyName, t.Date, t.Service1, t.Service2, t.Price1, t.Price2, string(t.Status), t.CompletedAt, t.Note, string(sel))
	if err != nil {
		return t
	}
	return t
}

func (s *Store) updateTaskCore(id string, t models.Task) (models.Task, error) {
	if t.Status == "" {
		t.Status = models.StatusPending
	}
	sel, _ := json.Marshal(t.SelectedPriceIDs)
	if sel == nil {
		sel = []byte("[]")
	}
	t.ID = id
	res, err := s.db.Exec(`UPDATE tasks SET customer_id=?, company_name=?, date=?, service1=?, service2=?, price1=?, price2=?, status=?, completed_at=?, note=?, selected_price_ids=? WHERE id=?`,
		t.CustomerID, t.CompanyName, t.Date, t.Service1, t.Service2, t.Price1, t.Price2, string(t.Status), t.CompletedAt, t.Note, string(sel), id)
	if err != nil {
		return models.Task{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.Task{}, ErrNotFound
	}
	return t, nil
}

// UpdateTask invoiceEdit 为 true 时允许修改 Done/Sent（供 Invoices 流程）；Paid 永远不可改。
func (s *Store) UpdateTask(id string, t models.Task, invoiceEdit bool) (models.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := scanTaskCols(s.db.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id=?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Task{}, ErrNotFound
		}
		return models.Task{}, err
	}
	if existing.Status == models.StatusPaid {
		return models.Task{}, ErrTaskPaidLocked
	}
	if (existing.Status == models.StatusDone || existing.Status == models.StatusSent) && !invoiceEdit {
		return models.Task{}, ErrTaskLocked
	}
	if invoiceEdit && (existing.Status == models.StatusDone || existing.Status == models.StatusSent) {
		t.Status = existing.Status
		t.CompletedAt = existing.CompletedAt
	}
	return s.updateTaskCore(id, t)
}

// SyncPendingTasksForPriceID 将价目变更同步到引用该价目 ID 的 Pending 任务（重写业务一/价格一）
func (s *Store) SyncPendingTasksForPriceID(priceID string) (int, error) {
	prices := s.ListPrices()
	priceMap := make(map[string]models.PriceItem, len(prices))
	for _, p := range prices {
		priceMap[p.ID] = p
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT ` + taskCols + ` FROM tasks`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		t, err := scanTaskCols(rows)
		if err != nil {
			continue
		}
		if t.Status != models.StatusPending {
			continue
		}
		if !sliceContainsStr(t.SelectedPriceIDs, priceID) {
			continue
		}
		if len(t.SelectedPriceIDs) == 0 {
			continue
		}
		s1, p1 := recomputeTaskPricesFromSelection(t.SelectedPriceIDs, priceMap)
		t.Service1 = s1
		t.Price1 = p1
		if _, err := s.updateTaskCore(t.ID, t); err != nil {
			continue
		}
		n++
	}
	return n, rows.Err()
}

func (s *Store) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := scanTaskCols(s.db.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id=?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if existing.Status != models.StatusPending {
		return ErrTaskDeleteLocked
	}
	res, err := s.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListCustomers() []models.Customer {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, name, COALESCE(display_name,''), COALESCE(email,''), COALESCE(phone,''), COALESCE(address,''), COALESCE(status,'active') FROM customers ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]models.Customer, 0)
	for rows.Next() {
		var c models.Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.DisplayName, &c.Email, &c.Phone, &c.Address, &c.Status); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// GetCustomer 按 id 查询客户
func (s *Store) GetCustomer(id string) (models.Customer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getCustomerUnlocked(id)
}

func (s *Store) getCustomerUnlocked(id string) (models.Customer, error) {
	var c models.Customer
	err := s.db.QueryRow(
		`SELECT id, name, COALESCE(display_name,''), COALESCE(email,''), COALESCE(phone,''), COALESCE(address,''), COALESCE(status,'active') FROM customers WHERE id=?`,
		strings.TrimSpace(id),
	).Scan(&c.ID, &c.Name, &c.DisplayName, &c.Email, &c.Phone, &c.Address, &c.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Customer{}, ErrNotFound
	}
	if err != nil {
		return models.Customer{}, err
	}
	return c, nil
}

// UpdateCustomer 更新客户资料（不修改客户 id）
func (s *Store) UpdateCustomer(id string, patch models.Customer) (models.Customer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	existing, err := s.getCustomerUnlocked(id)
	if err != nil {
		return models.Customer{}, err
	}
	name := strings.TrimSpace(patch.Name)
	if name == "" {
		name = strings.TrimSpace(existing.Name)
	}
	email := strings.TrimSpace(patch.Email)
	phone := strings.TrimSpace(patch.Phone)
	address := strings.TrimSpace(patch.Address)
	status := strings.ToLower(strings.TrimSpace(patch.Status))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(existing.Status))
	}
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "inactive" {
		status = strings.ToLower(strings.TrimSpace(existing.Status))
		if status == "" || (status != "active" && status != "inactive") {
			status = "active"
		}
	}
	displayName := normalizeCustomerDisplayName(patch.DisplayName)
	res, err := s.db.Exec(`UPDATE customers SET name=?, display_name=?, email=?, phone=?, address=?, status=? WHERE id=?`,
		name, displayName, email, phone, address, status, id)
	if err != nil {
		return models.Customer{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.Customer{}, ErrNotFound
	}
	return s.getCustomerUnlocked(id)
}

// RequireCustomerActive 新建任务或更换客户时：客户须存在且非 inactive
// 注意：调用此方法前必须先获取锁，否则会导致死锁
func (s *Store) RequireCustomerActive(customerID string) error {
	cid := strings.TrimSpace(customerID)
	if cid == "" {
		return ErrNotFound
	}
	c, err := s.getCustomerUnlocked(cid)
	if err != nil {
		return err
	}
	if !isActiveCustomerStatus(c.Status) {
		return ErrCustomerInactive
	}
	return nil
}

// RequireCustomerActiveLocked 线程安全版本，自动处理锁
func (s *Store) RequireCustomerActiveLocked(customerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.RequireCustomerActive(customerID)
}

func (s *Store) validateInvoiceTasksCustomersActiveLocked(inv *models.Invoice) error {
	normalizeInvoiceTaskIDs(inv)
	for _, tid := range invoiceLinkedTaskIDs(*inv) {
		if strings.TrimSpace(tid) == "" {
			continue
		}
		var cid string
		err := s.db.QueryRow(`SELECT customer_id FROM tasks WHERE id=?`, tid).Scan(&cid)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task %s not found", tid)
		}
		if err != nil {
			return err
		}
		var st string
		err = s.db.QueryRow(`SELECT COALESCE(status,'active') FROM customers WHERE id=?`, strings.TrimSpace(cid)).Scan(&st)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("customer for task %s not found", tid)
		}
		if err != nil {
			return err
		}
		if !isActiveCustomerStatus(st) {
			return ErrCustomerInactive
		}
	}
	return nil
}

// CreateCustomer 新建客户（id 空则自动生成 C0001 形式）
func (s *Store) CreateCustomer(c models.Customer) models.Customer {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return c
	}
	id := strings.TrimSpace(c.ID)
	if id == "" {
		id = s.nextCustomerIDLocked()
	}
	disp := normalizeCustomerDisplayName(c.DisplayName)
	_, err := s.db.Exec(`INSERT INTO customers (id, name, display_name, email, phone, address, status) VALUES (?,?,?,?,?,?,?)`,
		id, name, disp, strings.TrimSpace(c.Email), strings.TrimSpace(c.Phone), strings.TrimSpace(c.Address), "active")
	if err != nil {
		return c
	}
	c.ID = id
	c.Name = name
	c.DisplayName = disp
	c.Status = "active"
	return c
}

func (s *Store) nextCustomerIDLocked() string {
	rows, err := s.db.Query(`SELECT id FROM customers WHERE id LIKE 'C%'`)
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

func scanPrice(rows *sql.Rows) (models.PriceItem, error) {
	var p models.PriceItem
	var amt sql.NullFloat64
	err := rows.Scan(&p.ID, &p.ServiceName, &amt, &p.Currency, &p.Note)
	if err != nil {
		return models.PriceItem{}, err
	}
	if amt.Valid {
		v := amt.Float64
		p.Amount = &v
	}
	return p, nil
}

func (s *Store) ListPrices() []models.PriceItem {
	rows, err := s.db.Query(`SELECT id, service_name, amount, currency, note FROM price_items ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]models.PriceItem, 0)
	for rows.Next() {
		p, err := scanPrice(rows)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *Store) CreatePrice(p models.PriceItem) models.PriceItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = s.nextPriceID()
	}
	var amt any
	if p.Amount != nil {
		amt = *p.Amount
	}
	_, _ = s.db.Exec(`INSERT INTO price_items (id, service_name, amount, currency, note) VALUES (?,?,?,?,?)`,
		p.ID, p.ServiceName, amt, string(p.Currency), p.Note)
	return p
}

func (s *Store) nextPriceID() string {
	rows, err := s.db.Query(`SELECT id FROM price_items WHERE id LIKE 'P%'`)
	if err != nil {
		return "P0001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "P") && len(id) > 1 {
			if n, err := strconv.Atoi(strings.TrimPrefix(id, "P")); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("P%04d", max+1)
}

func (s *Store) UpdatePrice(id string, p models.PriceItem) (models.PriceItem, error) {
	p.ID = id
	var amt any
	if p.Amount != nil {
		amt = *p.Amount
	}
	res, err := s.db.Exec(`UPDATE price_items SET service_name=?, amount=?, currency=?, note=? WHERE id=?`,
		p.ServiceName, amt, string(p.Currency), p.Note, id)
	if err != nil {
		return models.PriceItem{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.PriceItem{}, ErrNotFound
	}
	return p, nil
}

func (s *Store) DeletePrice(id string) error {
	res, err := s.db.Exec(`DELETE FROM price_items WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func normalizeInvoiceTaskIDs(inv *models.Invoice) {
	if len(inv.TaskIDs) == 0 && inv.TaskID != "" {
		inv.TaskIDs = []string{inv.TaskID}
	}
	if len(inv.TaskIDs) > 0 {
		inv.TaskID = inv.TaskIDs[0]
	}
}

func invoiceLinkedTaskIDs(inv models.Invoice) []string {
	if len(inv.TaskIDs) > 0 {
		return inv.TaskIDs
	}
	if inv.TaskID != "" {
		return []string{inv.TaskID}
	}
	return nil
}

func (s *Store) nextInvoiceNo(now time.Time) string {
	prefix := fmt.Sprintf("INV-%04d%02d-", now.Year(), int(now.Month()))
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM invoices WHERE invoice_no LIKE ?`, prefix+"%").Scan(&n)
	return fmt.Sprintf("%s%04d", prefix, n+1)
}

func (s *Store) CreateInvoice(inv models.Invoice) (models.Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateInvoiceTasksCustomersActiveLocked(&inv); err != nil {
		return models.Invoice{}, err
	}
	now := time.Now()
	if inv.ID == "" {
		inv.ID = fmt.Sprintf("I%s", now.Format("20060102150405"))
	}
	if inv.InvoiceNo == "" {
		inv.InvoiceNo = s.nextInvoiceNo(now)
	}
	if inv.InvoiceDate == "" {
		inv.InvoiceDate = now.Format("2006-01-02")
	}
	if inv.CreatedAt == "" {
		inv.CreatedAt = now.Format(time.RFC3339)
	}
	// 使用精确的 Decimal 类型进行财务计算，避免浮点数精度问题
	var subtotal money.Decimal
	for i := range inv.Items {
		if inv.Items[i].Amount == 0 {
			inv.Items[i].Amount = inv.Items[i].Qty * inv.Items[i].Rate
		}
		amount, err := money.NewFromFloat(inv.Items[i].Amount)
		if err != nil {
			return models.Invoice{}, fmt.Errorf("invalid amount for item %d: %w", i, err)
		}
		subtotal = subtotal.Add(amount)
	}
	inv.Subtotal = subtotal.RoundToCents().Float64()

	// 计算税额（精确到分）
	taxAmount, err := subtotal.Percent(inv.TaxRate)
	if err != nil {
		return models.Invoice{}, fmt.Errorf("invalid tax rate: %w", err)
	}
	inv.TaxAmount = taxAmount.RoundToCents().Float64()

	// 计算总额
	total := subtotal.Add(taxAmount)
	inv.Total = total.RoundToCents().Float64()
	inv.BalanceDue = inv.Total
	itemsJSON, _ := json.Marshal(inv.Items)
	if inv.Status == "" {
		inv.Status = "Draft"
	}
	normalizeInvoiceTaskIDs(&inv)
	taskIDsJSON, _ := json.Marshal(inv.TaskIDs)
	_, err = s.db.Exec(`INSERT INTO invoices (id, invoice_no, task_id, invoice_date, terms, due_date, bill_to_name, bill_to_addr, ship_to_name, ship_to_addr, bill_to_email, currency, tax_rate, items_json, subtotal, tax_amount, total, balance_due, status, sent_at, paid_amount, paid_at, created_at, task_ids_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.InvoiceNo, inv.TaskID, inv.InvoiceDate, inv.Terms, inv.DueDate, inv.BillToName, inv.BillToAddr, inv.ShipToName, inv.ShipToAddr, inv.BillToEmail, inv.Currency, inv.TaxRate, string(itemsJSON), inv.Subtotal, inv.TaxAmount, inv.Total, inv.BalanceDue, inv.Status, inv.SentAt, inv.PaidAmount, inv.PaidAt, inv.CreatedAt, string(taskIDsJSON))
	if err != nil {
		return models.Invoice{}, err
	}
	return inv, nil
}

func (s *Store) GetInvoice(id string) (models.Invoice, error) {
	var inv models.Invoice
	var itemsJSON, taskIDsJSON string
	// 兼容旧库列缺失：列通过 ensureInvoiceColumns 逐步补齐
	err := s.db.QueryRow(`SELECT id, invoice_no, task_id, invoice_date, terms, due_date, bill_to_name, bill_to_addr, ship_to_name, ship_to_addr,
		COALESCE(bill_to_email,''), currency, tax_rate, items_json, subtotal, tax_amount, total, balance_due,
		COALESCE(status,'Draft'), COALESCE(sent_at,''), COALESCE(paid_amount,0), COALESCE(paid_at,''), created_at,
		COALESCE(task_ids_json,'')
		FROM invoices WHERE id=?`, id).
		Scan(&inv.ID, &inv.InvoiceNo, &inv.TaskID, &inv.InvoiceDate, &inv.Terms, &inv.DueDate, &inv.BillToName, &inv.BillToAddr, &inv.ShipToName, &inv.ShipToAddr,
			&inv.BillToEmail, &inv.Currency, &inv.TaxRate, &itemsJSON, &inv.Subtotal, &inv.TaxAmount, &inv.Total, &inv.BalanceDue,
			&inv.Status, &inv.SentAt, &inv.PaidAmount, &inv.PaidAt, &inv.CreatedAt, &taskIDsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Invoice{}, ErrNotFound
	}
	if err != nil {
		return models.Invoice{}, err
	}
	_ = json.Unmarshal([]byte(itemsJSON), &inv.Items)
	_ = json.Unmarshal([]byte(taskIDsJSON), &inv.TaskIDs)
	if len(inv.TaskIDs) == 0 && inv.TaskID != "" {
		inv.TaskIDs = []string{inv.TaskID}
	}
	return inv, nil
}

func (s *Store) ListInvoices(status string) ([]models.Invoice, error) {
	var rows *sql.Rows
	var err error
	base := `SELECT id, invoice_no, task_id, invoice_date, due_date, bill_to_name,
		currency, total, balance_due, COALESCE(status,'Draft'), COALESCE(sent_at,''), COALESCE(paid_amount,0), COALESCE(paid_at,'') FROM invoices`
	if status != "" {
		rows, err = s.db.Query(base+` WHERE status=? ORDER BY invoice_date DESC, invoice_no DESC`, status)
	} else {
		rows, err = s.db.Query(base + ` ORDER BY invoice_date DESC, invoice_no DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Invoice, 0)
	for rows.Next() {
		var inv models.Invoice
		if err := rows.Scan(&inv.ID, &inv.InvoiceNo, &inv.TaskID, &inv.InvoiceDate, &inv.DueDate, &inv.BillToName, &inv.Currency, &inv.Total, &inv.BalanceDue, &inv.Status, &inv.SentAt, &inv.PaidAmount, &inv.PaidAt); err != nil {
			continue
		}
		out = append(out, inv)
	}
	return out, nil
}

func (s *Store) MarkInvoiceSent(id, email, sentAt string) (models.Invoice, error) {
	if sentAt == "" {
		sentAt = time.Now().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`UPDATE invoices SET status='Sent', bill_to_email=?, sent_at=? WHERE id=?`, email, sentAt, id)
	if err != nil {
		return models.Invoice{}, err
	}
	inv, err := s.GetInvoice(id)
	if err != nil {
		return models.Invoice{}, err
	}
	// 同步 task 状态为 Sent（仅当未 Paid）
	for _, tid := range invoiceLinkedTaskIDs(inv) {
		if tid == "" {
			continue
		}
		_, _ = s.db.Exec(`UPDATE tasks SET status=? WHERE id=? AND status<>?`, string(models.StatusSent), tid, string(models.StatusPaid))
	}
	return inv, nil
}

func (s *Store) AddInvoicePayment(id string, amount float64, paidAt string) (models.Invoice, error) {
	if amount <= 0 {
		return models.Invoice{}, errors.New("amount must be > 0")
	}
	// 验证金额
	if err := money.ValidateAmount(amount); err != nil {
		return models.Invoice{}, fmt.Errorf("invalid payment amount: %w", err)
	}
	if paidAt == "" {
		paidAt = time.Now().Format("2006-01-02")
	}
	_, err := s.db.Exec(`UPDATE invoices SET paid_amount = paid_amount + ?, paid_at = CASE WHEN paid_at='' THEN ? ELSE paid_at END WHERE id=?`, amount, paidAt, id)
	if err != nil {
		return models.Invoice{}, err
	}
	inv, err := s.GetInvoice(id)
	if err != nil {
		return models.Invoice{}, err
	}
	// 使用精确的 Decimal 计算 balance_due
	total, _ := money.NewFromFloat(inv.Total)
	paid, _ := money.NewFromFloat(inv.PaidAmount)
	balanceDue := total.Sub(paid)
	// 确保余额不为负
	if balanceDue.IsNegative() {
		balanceDue = money.Zero()
	}
	bal := balanceDue.RoundToCents().Float64()

	newStatus := inv.Status
	tids := invoiceLinkedTaskIDs(inv)
	if inv.PaidAmount >= inv.Total && inv.Total > 0 {
		newStatus = "Paid"
		for _, tid := range tids {
			if tid == "" {
				continue
			}
			_, _ = s.db.Exec(`UPDATE tasks SET status=? WHERE id=?`, string(models.StatusPaid), tid)
		}
	} else {
		newStatus = "Sent"
		for _, tid := range tids {
			if tid == "" {
				continue
			}
			_, _ = s.db.Exec(`UPDATE tasks SET status=? WHERE id=? AND status<>?`, string(models.StatusSent), tid, string(models.StatusPaid))
		}
	}
	_, _ = s.db.Exec(`UPDATE invoices SET balance_due=?, status=? WHERE id=?`, bal, newStatus, id)
	return s.GetInvoice(id)
}
