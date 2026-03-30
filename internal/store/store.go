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

	"tasktracker/internal/models"
)

var ErrNotFound = errors.New("not found")

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

func (s *Store) nextTaskID(prefix string) string {
	s.taskSeq++
	if prefix == "" {
		prefix = "AC"
	}
	return prefix + fmt.Sprintf("%04d", s.taskSeq)
}

func scanTask(rows *sql.Rows) (models.Task, error) {
	return scanTaskScanner(rows)
}

func scanTaskScanner(sc interface {
	Scan(dest ...any) error
}) (models.Task, error) {
	var t models.Task
	var status string
	var sel string
	err := sc.Scan(
		&t.ID,
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

const taskCols = `id, company_name, date, service1, service2, price1, price2, status, completed_at, note, selected_price_ids`

func (s *Store) ListTasks() []models.Task {
	rows, err := s.db.Query(`SELECT ` + taskCols + ` FROM tasks ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]models.Task, 0)
	for rows.Next() {
		t, err := scanTask(rows)
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
		t.ID = s.nextTaskID("AC")
	}
	if t.Status == "" {
		t.Status = models.StatusPending
	}
	sel, _ := json.Marshal(t.SelectedPriceIDs)
	if sel == nil {
		sel = []byte("[]")
	}
	_, err := s.db.Exec(`INSERT INTO tasks (`+taskCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.CompanyName, t.Date, t.Service1, t.Service2, t.Price1, t.Price2, string(t.Status), t.CompletedAt, t.Note, string(sel))
	if err != nil {
		return t
	}
	return t
}

func (s *Store) UpdateTask(id string, t models.Task) (models.Task, error) {
	if t.Status == "" {
		t.Status = models.StatusPending
	}
	sel, _ := json.Marshal(t.SelectedPriceIDs)
	if sel == nil {
		sel = []byte("[]")
	}
	t.ID = id
	res, err := s.db.Exec(`UPDATE tasks SET company_name=?, date=?, service1=?, service2=?, price1=?, price2=?, status=?, completed_at=?, note=?, selected_price_ids=? WHERE id=?`,
		t.CompanyName, t.Date, t.Service1, t.Service2, t.Price1, t.Price2, string(t.Status), t.CompletedAt, t.Note, string(sel), id)
	if err != nil {
		return models.Task{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.Task{}, ErrNotFound
	}
	return t, nil
}

func (s *Store) DeleteTask(id string) error {
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

func (s *Store) nextInvoiceNo(now time.Time) string {
	prefix := fmt.Sprintf("INV-%04d%02d-", now.Year(), int(now.Month()))
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM invoices WHERE invoice_no LIKE ?`, prefix+"%").Scan(&n)
	return fmt.Sprintf("%s%04d", prefix, n+1)
}

func (s *Store) CreateInvoice(inv models.Invoice) (models.Invoice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	var subtotal float64
	for i := range inv.Items {
		if inv.Items[i].Amount == 0 {
			inv.Items[i].Amount = inv.Items[i].Qty * inv.Items[i].Rate
		}
		subtotal += inv.Items[i].Amount
	}
	inv.Subtotal = subtotal
	inv.TaxAmount = subtotal * (inv.TaxRate / 100.0)
	inv.Total = inv.Subtotal + inv.TaxAmount
	inv.BalanceDue = inv.Total
	itemsJSON, _ := json.Marshal(inv.Items)
	if inv.Status == "" {
		inv.Status = "Draft"
	}
	_, err := s.db.Exec(`INSERT INTO invoices (id, invoice_no, task_id, invoice_date, terms, due_date, bill_to_name, bill_to_addr, ship_to_name, ship_to_addr, bill_to_email, currency, tax_rate, items_json, subtotal, tax_amount, total, balance_due, status, sent_at, paid_amount, paid_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.InvoiceNo, inv.TaskID, inv.InvoiceDate, inv.Terms, inv.DueDate, inv.BillToName, inv.BillToAddr, inv.ShipToName, inv.ShipToAddr, inv.BillToEmail, inv.Currency, inv.TaxRate, string(itemsJSON), inv.Subtotal, inv.TaxAmount, inv.Total, inv.BalanceDue, inv.Status, inv.SentAt, inv.PaidAmount, inv.PaidAt, inv.CreatedAt)
	if err != nil {
		return models.Invoice{}, err
	}
	return inv, nil
}

func (s *Store) GetInvoice(id string) (models.Invoice, error) {
	var inv models.Invoice
	var itemsJSON string
	// 兼容旧库列缺失：列通过 ensureInvoiceColumns 逐步补齐
	err := s.db.QueryRow(`SELECT id, invoice_no, task_id, invoice_date, terms, due_date, bill_to_name, bill_to_addr, ship_to_name, ship_to_addr,
		COALESCE(bill_to_email,''), currency, tax_rate, items_json, subtotal, tax_amount, total, balance_due,
		COALESCE(status,'Draft'), COALESCE(sent_at,''), COALESCE(paid_amount,0), COALESCE(paid_at,''), created_at
		FROM invoices WHERE id=?`, id).
		Scan(&inv.ID, &inv.InvoiceNo, &inv.TaskID, &inv.InvoiceDate, &inv.Terms, &inv.DueDate, &inv.BillToName, &inv.BillToAddr, &inv.ShipToName, &inv.ShipToAddr,
			&inv.BillToEmail, &inv.Currency, &inv.TaxRate, &itemsJSON, &inv.Subtotal, &inv.TaxAmount, &inv.Total, &inv.BalanceDue,
			&inv.Status, &inv.SentAt, &inv.PaidAmount, &inv.PaidAt, &inv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Invoice{}, ErrNotFound
	}
	if err != nil {
		return models.Invoice{}, err
	}
	_ = json.Unmarshal([]byte(itemsJSON), &inv.Items)
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
	_, _ = s.db.Exec(`UPDATE tasks SET status=? WHERE id=? AND status<>?`, string(models.StatusSent), inv.TaskID, string(models.StatusPaid))
	return inv, nil
}

func (s *Store) AddInvoicePayment(id string, amount float64, paidAt string) (models.Invoice, error) {
	if amount <= 0 {
		return models.Invoice{}, errors.New("amount must be > 0")
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
	// 更新 balance_due / status
	bal := inv.Total - inv.PaidAmount
	if bal < 0 {
		bal = 0
	}
	newStatus := inv.Status
	if inv.PaidAmount >= inv.Total && inv.Total > 0 {
		newStatus = "Paid"
		_, _ = s.db.Exec(`UPDATE tasks SET status=? WHERE id=?`, string(models.StatusPaid), inv.TaskID)
	} else {
		newStatus = "Sent"
		_, _ = s.db.Exec(`UPDATE tasks SET status=? WHERE id=? AND status<>?`, string(models.StatusSent), inv.TaskID, string(models.StatusPaid))
	}
	_, _ = s.db.Exec(`UPDATE invoices SET balance_due=?, status=? WHERE id=?`, bal, newStatus, id)
	return s.GetInvoice(id)
}
