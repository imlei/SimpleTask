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

func (s *Store) ListExpenseVendors() []models.ExpenseVendor {
	rows, err := s.db.Query(`
		SELECT id, name, currency, COALESCE(email,''), COALESCE(address,''), created_at
		FROM expense_vendors ORDER BY LOWER(name) ASC, id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []models.ExpenseVendor
	for rows.Next() {
		var v models.ExpenseVendor
		if err := rows.Scan(&v.ID, &v.Name, &v.Currency, &v.Email, &v.Address, &v.CreatedAt); err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (s *Store) GetExpenseVendor(id string) (models.ExpenseVendor, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return models.ExpenseVendor{}, ErrNotFound
	}
	row := s.db.QueryRow(`
		SELECT id, name, currency, COALESCE(email,''), COALESCE(address,''), created_at
		FROM expense_vendors WHERE id=?`, id)
	var v models.ExpenseVendor
	err := row.Scan(&v.ID, &v.Name, &v.Currency, &v.Email, &v.Address, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ExpenseVendor{}, ErrNotFound
	}
	if err != nil {
		return models.ExpenseVendor{}, err
	}
	return v, nil
}

func (s *Store) CreateExpenseVendor(v models.ExpenseVendor) (models.ExpenseVendor, error) {
	name := strings.TrimSpace(v.Name)
	if name == "" {
		return models.ExpenseVendor{}, errors.New("name is required")
	}
	cur := strings.ToUpper(strings.TrimSpace(v.Currency))
	if cur == "" {
		cur = "CAD"
	}
	if len(cur) > 8 {
		cur = cur[:8]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == "" {
		v.ID = s.nextExpenseVendorIDLocked()
	}
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO expense_vendors (id, name, currency, email, address, created_at) VALUES (?,?,?,?,?,?)`,
		v.ID, name, cur, strings.TrimSpace(v.Email), strings.TrimSpace(v.Address), now)
	if err != nil {
		return models.ExpenseVendor{}, err
	}
	v.Name = name
	v.Currency = cur
	v.Email = strings.TrimSpace(v.Email)
	v.Address = strings.TrimSpace(v.Address)
	v.CreatedAt = now
	return v, nil
}

func (s *Store) nextExpenseVendorIDLocked() string {
	rows, err := s.db.Query(`SELECT id FROM expense_vendors WHERE id LIKE 'V%'`)
	if err != nil {
		return "V0001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "V") && len(id) > 1 {
			if n, err := strconv.Atoi(strings.TrimPrefix(id, "V")); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("V%04d", max+1)
}
