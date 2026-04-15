package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/models"
)

func (s *Store) ListPayrollCompanies(statusFilter string) []models.PayrollCompany {
	s.mu.Lock()
	defer s.mu.Unlock()

	q := `SELECT id, name, legal_name, business_number, email, phone, address, province, pay_frequency, status, created_at, updated_at
	      FROM payroll_companies`
	args := []any{}
	if statusFilter != "" && statusFilter != "all" {
		q += ` WHERE status = ?`
		args = append(args, statusFilter)
	}
	q += ` ORDER BY name ASC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []models.PayrollCompany{}
	}
	defer rows.Close()

	var list []models.PayrollCompany
	for rows.Next() {
		var c models.PayrollCompany
		if err := rows.Scan(&c.ID, &c.Name, &c.LegalName, &c.BusinessNumber,
			&c.Email, &c.Phone, &c.Address, &c.Province,
			&c.PayFrequency, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		list = append(list, c)
	}
	if list == nil {
		return []models.PayrollCompany{}
	}
	return list
}

func (s *Store) GetPayrollCompany(id string) (models.PayrollCompany, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var c models.PayrollCompany
	err := s.db.QueryRow(
		`SELECT id, name, legal_name, business_number, email, phone, address, province, pay_frequency, status, created_at, updated_at
		 FROM payroll_companies WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.LegalName, &c.BusinessNumber,
		&c.Email, &c.Phone, &c.Address, &c.Province,
		&c.PayFrequency, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, ErrNotFound
	}
	return c, nil
}

func (s *Store) CreatePayrollCompany(c models.PayrollCompany) models.PayrollCompany {
	s.mu.Lock()
	defer s.mu.Unlock()

	c.ID = s.nextPayrollCompanyID()
	now := time.Now().UTC().Format(time.RFC3339)
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = "active"
	}
	if c.PayFrequency == "" {
		c.PayFrequency = "biweekly"
	}

	_, _ = s.db.Exec(
		`INSERT INTO payroll_companies (id, name, legal_name, business_number, email, phone, address, province, pay_frequency, status, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.LegalName, c.BusinessNumber,
		c.Email, c.Phone, c.Address, c.Province,
		c.PayFrequency, c.Status, c.CreatedAt, c.UpdatedAt,
	)
	return c
}

func (s *Store) UpdatePayrollCompany(id string, patch models.PayrollCompany) (models.PayrollCompany, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var existing models.PayrollCompany
	err := s.db.QueryRow(
		`SELECT id, created_at FROM payroll_companies WHERE id = ?`, id,
	).Scan(&existing.ID, &existing.CreatedAt)
	if err != nil {
		return existing, ErrNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)
	patch.ID = id
	patch.CreatedAt = existing.CreatedAt
	patch.UpdatedAt = now
	if patch.Status == "" {
		patch.Status = "active"
	}
	if patch.PayFrequency == "" {
		patch.PayFrequency = "biweekly"
	}

	_, err = s.db.Exec(
		`UPDATE payroll_companies SET name=?, legal_name=?, business_number=?, email=?, phone=?, address=?, province=?, pay_frequency=?, status=?, updated_at=?
		 WHERE id=?`,
		patch.Name, patch.LegalName, patch.BusinessNumber,
		patch.Email, patch.Phone, patch.Address, patch.Province,
		patch.PayFrequency, patch.Status, now, id,
	)
	if err != nil {
		return patch, err
	}
	return patch, nil
}

func (s *Store) DeletePayrollCompany(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`DELETE FROM payroll_companies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) nextPayrollCompanyID() string {
	rows, err := s.db.Query(`SELECT id FROM payroll_companies WHERE id LIKE 'PC%'`)
	if err != nil {
		return "PC0001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "PC") {
			if v, err := strconv.Atoi(strings.TrimPrefix(id, "PC")); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("PC%04d", max+1)
}
