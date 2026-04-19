package store

import (
	"database/sql"
	"errors"
	"time"

	"simpletask/internal/models"
)

var ErrYearLocked = errors.New("payroll year is locked")

func ensurePayrollYearLocksTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_year_locks (
  company_id TEXT NOT NULL,
  year       INTEGER NOT NULL,
  locked_at  TEXT NOT NULL DEFAULT '',
  locked_by  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (company_id, year)
);`)
	return err
}

// IsYearLocked returns true if the given company+year combination is locked.
func (s *Store) IsYearLocked(companyID string, year int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM payroll_year_locks WHERE company_id=? AND year=?`,
		companyID, year,
	).Scan(&n)
	return n > 0
}

// GetYearLock returns the lock record, or nil if not locked.
func (s *Store) GetYearLock(companyID string, year int) *models.PayrollYearLock {
	s.mu.Lock()
	defer s.mu.Unlock()
	var l models.PayrollYearLock
	err := s.db.QueryRow(
		`SELECT company_id, year, locked_at, locked_by FROM payroll_year_locks WHERE company_id=? AND year=?`,
		companyID, year,
	).Scan(&l.CompanyID, &l.Year, &l.LockedAt, &l.LockedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return nil
	}
	return &l
}

// ListYearLocks returns all locks for a company, ordered by year desc.
func (s *Store) ListYearLocks(companyID string) []models.PayrollYearLock {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT company_id, year, locked_at, locked_by FROM payroll_year_locks WHERE company_id=? ORDER BY year DESC`,
		companyID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []models.PayrollYearLock
	for rows.Next() {
		var l models.PayrollYearLock
		if err := rows.Scan(&l.CompanyID, &l.Year, &l.LockedAt, &l.LockedBy); err != nil {
			continue
		}
		out = append(out, l)
	}
	return out
}

// LockYear locks the given company+year. Idempotent — locking an already-locked year is a no-op.
func (s *Store) LockYear(companyID string, year int, lockedBy string) (models.PayrollYearLock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO payroll_year_locks (company_id, year, locked_at, locked_by) VALUES (?,?,?,?)`,
		companyID, year, now, lockedBy,
	)
	if err != nil {
		return models.PayrollYearLock{}, err
	}
	var l models.PayrollYearLock
	_ = s.db.QueryRow(
		`SELECT company_id, year, locked_at, locked_by FROM payroll_year_locks WHERE company_id=? AND year=?`,
		companyID, year,
	).Scan(&l.CompanyID, &l.Year, &l.LockedAt, &l.LockedBy)
	return l, nil
}

// UnlockYear removes the lock for the given company+year.
func (s *Store) UnlockYear(companyID string, year int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`DELETE FROM payroll_year_locks WHERE company_id=? AND year=?`,
		companyID, year,
	)
	return err
}
