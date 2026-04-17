package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/models"
)

func (s *Store) ListPayrollPeriods(companyID string) []models.PayrollPeriod {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, company_id, period_start, period_end, pay_date,
		       pays_per_year, pay_frequency, payroll_type, status,
		       created_at, updated_at
		FROM payroll_periods
		WHERE company_id = ?
		ORDER BY pay_date DESC`, companyID)
	if err != nil {
		return []models.PayrollPeriod{}
	}
	defer rows.Close()

	var list []models.PayrollPeriod
	for rows.Next() {
		var p models.PayrollPeriod
		if err := rows.Scan(&p.ID, &p.CompanyID, &p.PeriodStart, &p.PeriodEnd, &p.PayDate,
			&p.PaysPerYear, &p.PayFrequency, &p.PayrollType, &p.Status,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		list = append(list, p)
	}
	if list == nil {
		return []models.PayrollPeriod{}
	}
	return list
}

func (s *Store) GetPayrollPeriod(id string) (models.PayrollPeriod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var p models.PayrollPeriod
	err := s.db.QueryRow(`
		SELECT id, company_id, period_start, period_end, pay_date,
		       pays_per_year, pay_frequency, payroll_type, status,
		       created_at, updated_at
		FROM payroll_periods WHERE id = ?`, id).
		Scan(&p.ID, &p.CompanyID, &p.PeriodStart, &p.PeriodEnd, &p.PayDate,
			&p.PaysPerYear, &p.PayFrequency, &p.PayrollType, &p.Status,
			&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, ErrNotFound
	}
	return p, nil
}

func (s *Store) CreatePayrollPeriod(p models.PayrollPeriod) models.PayrollPeriod {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Enforce at most one unfinished period per company:
	// delete any existing draft/open/calculated periods (and their entries) before creating a new one.
	rows, _ := s.db.Query(`
		SELECT id FROM payroll_periods
		WHERE company_id = ? AND status NOT IN ('finalized')`, p.CompanyID)
	var staleIDs []string
	if rows != nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				staleIDs = append(staleIDs, id)
			}
		}
		rows.Close()
	}
	for _, id := range staleIDs {
		_, _ = s.db.Exec(`DELETE FROM payroll_entries WHERE period_id = ?`, id)
		_, _ = s.db.Exec(`DELETE FROM payroll_periods WHERE id = ?`, id)
	}

	p.ID = s.nextPeriodID(p.CompanyID)
	now := time.Now().UTC().Format(time.RFC3339)
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = "open"
	}
	if p.PayrollType == "" {
		p.PayrollType = "regular"
	}

	_, _ = s.db.Exec(`
		INSERT INTO payroll_periods
		  (id, company_id, period_start, period_end, pay_date,
		   pays_per_year, pay_frequency, payroll_type, status,
		   created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.CompanyID, p.PeriodStart, p.PeriodEnd, p.PayDate,
		p.PaysPerYear, p.PayFrequency, p.PayrollType, p.Status,
		p.CreatedAt, p.UpdatedAt,
	)
	return p
}

func (s *Store) UpdatePayrollPeriodStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`UPDATE payroll_periods SET status=?, updated_at=? WHERE id=?`,
		status, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) nextPeriodID(companyID string) string {
	rows, err := s.db.Query(`SELECT id FROM payroll_periods WHERE company_id = ? AND id LIKE 'PP%'`, companyID)
	if err != nil {
		return "PP00001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "PP") {
			if v, err := strconv.Atoi(strings.TrimPrefix(id, "PP")); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("PP%05d", max+1)
}

// DeletePayrollPeriod deletes a period and all its entries.
// Only allows deletion of 'open' or 'calculated' periods.
// Finalized periods can only be deleted if force=true.
func (s *Store) DeletePayrollPeriod(id string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check period status
	var status string
	_ = s.db.QueryRow(`SELECT status FROM payroll_periods WHERE id=?`, id).Scan(&status)

	if status == "" {
		return ErrNotFound
	}
	if status == "finalized" && !force {
		return fmt.Errorf("cannot delete finalized period without force flag")
	}

	// Delete entries first (foreign key)
	_, _ = s.db.Exec(`DELETE FROM payroll_entries WHERE period_id = ?`, id)
	// Then delete period
	_, err := s.db.Exec(`DELETE FROM payroll_periods WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return nil
}
