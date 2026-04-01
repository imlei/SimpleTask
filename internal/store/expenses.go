package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tasktracker/internal/models"
)

// ErrExpenseTaskNotFound 支出关联的任务不存在（与 ErrNotFound 区分，便于 API 返回 400）
var ErrExpenseTaskNotFound = errors.New("task not found")

func (s *Store) ListExpenses() []models.Expense {
	rows, err := s.db.Query(`
		SELECT e.id, e.task_id, COALESCE(e.vendor_id,''), COALESCE(e.expense_date,''), e.description, COALESCE(e.account_code,''), e.amount, e.currency, e.created_at,
		       COALESCE(t.company_name,''), COALESCE(v.name,'')
		FROM expenses e
		LEFT JOIN tasks t ON t.id = e.task_id
		LEFT JOIN expense_vendors v ON v.id = e.vendor_id
		ORDER BY e.expense_date DESC, e.created_at DESC, e.id DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]models.Expense, 0)
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			continue
		}
		out = append(out, e)
	}
	return out
}

func scanExpense(rows interface{ Scan(dest ...any) error }) (models.Expense, error) {
	var e models.Expense
	var amt float64
	err := rows.Scan(&e.ID, &e.TaskID, &e.VendorID, &e.ExpenseDate, &e.Description, &e.AccountCode, &amt, &e.Currency, &e.CreatedAt, &e.TaskName, &e.VendorName)
	if err != nil {
		return models.Expense{}, err
	}
	e.Amount = amt
	return e, nil
}

func (s *Store) GetExpense(id string) (models.Expense, error) {
	row := s.db.QueryRow(`
		SELECT e.id, e.task_id, COALESCE(e.vendor_id,''), COALESCE(e.expense_date,''), e.description, COALESCE(e.account_code,''), e.amount, e.currency, e.created_at,
		       COALESCE(t.company_name,''), COALESCE(v.name,'')
		FROM expenses e
		LEFT JOIN tasks t ON t.id = e.task_id
		LEFT JOIN expense_vendors v ON v.id = e.vendor_id
		WHERE e.id=?`, id)
	e, err := scanExpense(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Expense{}, ErrNotFound
	}
	if err != nil {
		return models.Expense{}, err
	}
	return e, nil
}

func (s *Store) CreateExpense(e models.Expense) (models.Expense, error) {
	if strings.TrimSpace(e.TaskID) == "" {
		return models.Expense{}, errors.New("taskId is required")
	}
	if _, err := s.GetTask(e.TaskID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return models.Expense{}, ErrExpenseTaskNotFound
		}
		return models.Expense{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID == "" {
		e.ID = s.nextExpenseIDLocked()
	}
	if strings.TrimSpace(e.Currency) == "" {
		e.Currency = "CAD"
	}
	e.AccountCode = strings.TrimSpace(e.AccountCode)
	e.ExpenseDate = strings.TrimSpace(e.ExpenseDate)
	if e.ExpenseDate == "" {
		e.ExpenseDate = time.Now().Format("2006-01-02")
	}
	e.VendorID = strings.TrimSpace(e.VendorID)
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO expenses (id, task_id, vendor_id, expense_date, description, account_code, amount, currency, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		e.ID, e.TaskID, e.VendorID, e.ExpenseDate, strings.TrimSpace(e.Description), e.AccountCode, e.Amount, e.Currency, now)
	if err != nil {
		return models.Expense{}, err
	}
	e.CreatedAt = now
	_ = s.db.QueryRow(`SELECT company_name FROM tasks WHERE id=?`, e.TaskID).Scan(&e.TaskName)
	if e.VendorID != "" {
		_ = s.db.QueryRow(`SELECT name FROM expense_vendors WHERE id=?`, e.VendorID).Scan(&e.VendorName)
	}
	return e, nil
}

func (s *Store) UpdateExpense(id string, e models.Expense) (models.Expense, error) {
	if strings.TrimSpace(e.TaskID) == "" {
		return models.Expense{}, errors.New("taskId is required")
	}
	if _, err := s.GetTask(e.TaskID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return models.Expense{}, ErrExpenseTaskNotFound
		}
		return models.Expense{}, err
	}
	if strings.TrimSpace(e.Currency) == "" {
		e.Currency = "CAD"
	}
	e.AccountCode = strings.TrimSpace(e.AccountCode)
	e.ExpenseDate = strings.TrimSpace(e.ExpenseDate)
	if e.ExpenseDate == "" {
		e.ExpenseDate = time.Now().Format("2006-01-02")
	}
	e.VendorID = strings.TrimSpace(e.VendorID)
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE expenses SET task_id=?, vendor_id=?, expense_date=?, description=?, account_code=?, amount=?, currency=? WHERE id=?`,
		e.TaskID, e.VendorID, e.ExpenseDate, strings.TrimSpace(e.Description), e.AccountCode, e.Amount, e.Currency, id)
	if err != nil {
		return models.Expense{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.Expense{}, ErrNotFound
	}
	e.ID = id
	e.CreatedAt = ""
	_ = s.db.QueryRow(`SELECT created_at FROM expenses WHERE id=?`, id).Scan(&e.CreatedAt)
	_ = s.db.QueryRow(`SELECT company_name FROM tasks WHERE id=?`, e.TaskID).Scan(&e.TaskName)
	e.VendorName = ""
	if e.VendorID != "" {
		_ = s.db.QueryRow(`SELECT name FROM expense_vendors WHERE id=?`, e.VendorID).Scan(&e.VendorName)
	}
	return e, nil
}

func (s *Store) nextExpenseIDLocked() string {
	rows, err := s.db.Query(`SELECT id FROM expenses WHERE id LIKE 'E%'`)
	if err != nil {
		return "E0001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "E") && len(id) > 1 {
			if n, err := strconv.Atoi(strings.TrimPrefix(id, "E")); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("E%04d", max+1)
}
