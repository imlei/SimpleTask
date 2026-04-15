package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/models"
)

// systemEarningsCodes is the standard set seeded for every new company (matches PAYEVO dropdown).
var systemEarningsCodes = []models.PayrollEarningsCode{
	{Code: "REG",  Name: "Regular Hours",          Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 1},
	{Code: "OT15", Name: "Overtime Hours @ 1.5",   Multiplier: 1.5,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 2},
	{Code: "DT2",  Name: "Doubletime Hours @ 2",   Multiplier: 2.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 3},
	{Code: "OHW",  Name: "Other Hours worked",     Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 4},
	{Code: "STAT", Name: "Statutory Holiday",      Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: false, Enabled: true, IsSystem: true, SortOrder: 5},
	{Code: "SICK", Name: "Sick day",               Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: false, Enabled: true, IsSystem: true, SortOrder: 6},
	{Code: "PERS", Name: "Personal day",           Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: false, Enabled: true, IsSystem: true, SortOrder: 7},
	{Code: "SHFT", Name: "Shift Premium",          Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 8},
	{Code: "OTHE", Name: "Other Earnings",         Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 9},
	{Code: "NTAX", Name: "Other Non-Taxable",      Multiplier: 1.0,  CPP: false, EI: false, TaxFed: false, TaxProv: false, Vacationable: false, Enabled: true, IsSystem: true, SortOrder: 10},
	{Code: "TIPS", Name: "Tips",                   Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 11},
	{Code: "VAC",  Name: "Vacation Pay",           Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: false, Enabled: true, IsSystem: true, SortOrder: 12},
	{Code: "COMM", Name: "Commission",             Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 13},
	{Code: "ADV",  Name: "Earnings Advances",      Multiplier: 1.0,  CPP: false, EI: false, TaxFed: false, TaxProv: false, Vacationable: false, Enabled: true, IsSystem: true, SortOrder: 14},
	{Code: "BONM", Name: "Bonus",                  Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: false, Enabled: true, IsSystem: true, SortOrder: 15},
	{Code: "RETR", Name: "Retroactive Pay",        Multiplier: 1.0,  CPP: true,  EI: true,  TaxFed: true,  TaxProv: true,  Vacationable: true,  Enabled: true, IsSystem: true, SortOrder: 16},
}

// ── Earnings Codes (company-level Pay Rules) ──────────────────────────────────

// EnsureSystemCodes seeds the standard earnings codes for a company if it has none yet.
func (s *Store) EnsureSystemCodes(companyID string) {
	s.mu.Lock()
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM payroll_earnings_codes WHERE company_id=?`, companyID).Scan(&count)
	s.mu.Unlock()
	if count > 0 {
		return
	}
	for _, tpl := range systemEarningsCodes {
		tpl.CompanyID = companyID
		_, _ = s.CreateEarningsCode(tpl)
	}
}

// ListEarningsCodes returns all codes for a company, ordered by sort_order.
func (s *Store) ListEarningsCodes(companyID string) []models.PayrollEarningsCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, company_id, code, name, enabled, cpp, ei, tax_fed, tax_prov,
		       non_cash, vacationable, COALESCE(multiplier,1.0), COALESCE(is_system,0),
		       t4_box, sort_order, created_at, updated_at
		FROM payroll_earnings_codes
		WHERE company_id = ?
		ORDER BY sort_order ASC, name ASC`, companyID)
	if err != nil {
		return []models.PayrollEarningsCode{}
	}
	defer rows.Close()

	var list []models.PayrollEarningsCode
	for rows.Next() {
		var c models.PayrollEarningsCode
		var enabled, cpp, ei, taxFed, taxProv, nonCash, vacationable, isSystem int
		if err := rows.Scan(
			&c.ID, &c.CompanyID, &c.Code, &c.Name,
			&enabled, &cpp, &ei, &taxFed, &taxProv, &nonCash, &vacationable,
			&c.Multiplier, &isSystem,
			&c.T4Box, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			continue
		}
		c.Enabled = enabled == 1
		c.CPP = cpp == 1
		c.EI = ei == 1
		c.TaxFed = taxFed == 1
		c.TaxProv = taxProv == 1
		c.NonCash = nonCash == 1
		c.Vacationable = vacationable == 1
		c.IsSystem = isSystem == 1
		list = append(list, c)
	}
	if list == nil {
		return []models.PayrollEarningsCode{}
	}
	return list
}

// GetEarningsCode retrieves one code by ID.
func (s *Store) GetEarningsCode(id string) (models.PayrollEarningsCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var c models.PayrollEarningsCode
	var enabled, cpp, ei, taxFed, taxProv, nonCash, vacationable, isSystem int
	err := s.db.QueryRow(`
		SELECT id, company_id, code, name, enabled, cpp, ei, tax_fed, tax_prov,
		       non_cash, vacationable, COALESCE(multiplier,1.0), COALESCE(is_system,0),
		       t4_box, sort_order, created_at, updated_at
		FROM payroll_earnings_codes WHERE id = ?`, id).Scan(
		&c.ID, &c.CompanyID, &c.Code, &c.Name,
		&enabled, &cpp, &ei, &taxFed, &taxProv, &nonCash, &vacationable,
		&c.Multiplier, &isSystem,
		&c.T4Box, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return c, ErrNotFound
	}
	c.Enabled = enabled == 1
	c.CPP = cpp == 1
	c.EI = ei == 1
	c.TaxFed = taxFed == 1
	c.TaxProv = taxProv == 1
	c.NonCash = nonCash == 1
	c.Vacationable = vacationable == 1
	c.IsSystem = isSystem == 1
	return c, nil
}

// GetVacationCodeID returns the ID of the "VAC" earnings code for a company (or "" if not found).
func (s *Store) GetVacationCodeID(companyID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var id string
	_ = s.db.QueryRow(`SELECT id FROM payroll_earnings_codes WHERE company_id=? AND code='VAC' AND enabled=1 LIMIT 1`, companyID).Scan(&id)
	return id
}

// CreateEarningsCode inserts a new earnings code.
func (s *Store) CreateEarningsCode(c models.PayrollEarningsCode) (models.PayrollEarningsCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	c.ID = s.nextEarningsCodeID()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Multiplier <= 0 {
		c.Multiplier = 1.0
	}

	_, err := s.db.Exec(`
		INSERT INTO payroll_earnings_codes
		  (id, company_id, code, name, enabled, cpp, ei, tax_fed, tax_prov,
		   non_cash, vacationable, multiplier, is_system, t4_box, sort_order, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.CompanyID, c.Code, c.Name,
		boolInt(c.Enabled), boolInt(c.CPP), boolInt(c.EI),
		boolInt(c.TaxFed), boolInt(c.TaxProv),
		boolInt(c.NonCash), boolInt(c.Vacationable),
		c.Multiplier, boolInt(c.IsSystem),
		c.T4Box, c.SortOrder, c.CreatedAt, c.UpdatedAt,
	)
	return c, err
}

// UpdateEarningsCode updates an existing code.
func (s *Store) UpdateEarningsCode(id string, c models.PayrollEarningsCode) (models.PayrollEarningsCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	if c.Multiplier <= 0 {
		c.Multiplier = 1.0
	}
	res, err := s.db.Exec(`
		UPDATE payroll_earnings_codes SET
		  code=?, name=?, enabled=?, cpp=?, ei=?, tax_fed=?, tax_prov=?,
		  non_cash=?, vacationable=?, multiplier=?, t4_box=?, sort_order=?, updated_at=?
		WHERE id=?`,
		c.Code, c.Name,
		boolInt(c.Enabled), boolInt(c.CPP), boolInt(c.EI),
		boolInt(c.TaxFed), boolInt(c.TaxProv),
		boolInt(c.NonCash), boolInt(c.Vacationable),
		c.Multiplier, c.T4Box, c.SortOrder, now, id,
	)
	if err != nil {
		return c, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return c, ErrNotFound
	}
	c.ID = id
	c.UpdatedAt = now
	return c, nil
}

// DeleteEarningsCode deletes a non-system earnings code.
func (s *Store) DeleteEarningsCode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var isSystem int
	_ = s.db.QueryRow(`SELECT COALESCE(is_system,0) FROM payroll_earnings_codes WHERE id=?`, id).Scan(&isSystem)
	if isSystem == 1 {
		return fmt.Errorf("system earnings codes cannot be deleted")
	}
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM payroll_entry_earnings WHERE earnings_code_id=?`, id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("earnings code is in use by %d entry lines; cannot delete", count)
	}
	res, err := s.db.Exec(`DELETE FROM payroll_earnings_codes WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) nextEarningsCodeID() string {
	rows, err := s.db.Query(`SELECT id FROM payroll_earnings_codes WHERE id LIKE 'EC%'`)
	if err != nil {
		return "EC00001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "EC") {
			if v, err := strconv.Atoi(strings.TrimPrefix(id, "EC")); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("EC%05d", max+1)
}

// ── Entry Earnings (additional earnings lines per payroll entry) ──────────────

// ListEntryEarnings returns all earnings lines for a payroll entry, joined with code details.
func (s *Store) ListEntryEarnings(entryID string) []models.PayrollEntryEarning {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT ee.id, ee.entry_id, ee.earnings_code_id,
		       COALESCE(ec.name, ''),
		       ee.hours, ee.rate, ee.amount,
		       ee.created_at, ee.updated_at
		FROM payroll_entry_earnings ee
		LEFT JOIN payroll_earnings_codes ec ON ec.id = ee.earnings_code_id
		WHERE ee.entry_id = ?
		ORDER BY ee.created_at ASC`, entryID)
	if err != nil {
		return []models.PayrollEntryEarning{}
	}
	defer rows.Close()

	var list []models.PayrollEntryEarning
	for rows.Next() {
		var e models.PayrollEntryEarning
		if err := rows.Scan(
			&e.ID, &e.EntryID, &e.EarningsCodeID, &e.CodeName,
			&e.Hours, &e.Rate, &e.Amount,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			continue
		}
		list = append(list, e)
	}
	if list == nil {
		return []models.PayrollEntryEarning{}
	}
	return list
}

// ReplaceEntryEarnings replaces all additional earnings lines for an entry atomically.
func (s *Store) ReplaceEntryEarnings(entryID string, lines []models.PayrollEntryEarning) ([]models.PayrollEntryEarning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM payroll_entry_earnings WHERE entry_id=?`, entryID); err != nil {
		return nil, err
	}

	result := make([]models.PayrollEntryEarning, 0, len(lines))
	seq := 1
	for _, l := range lines {
		l.ID = fmt.Sprintf("EE%s%03d", entryID, seq)
		seq++
		l.EntryID = entryID
		l.CreatedAt = now
		l.UpdatedAt = now
		if _, err := tx.Exec(`
			INSERT INTO payroll_entry_earnings (id, entry_id, earnings_code_id, hours, rate, amount, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			l.ID, l.EntryID, l.EarningsCodeID, l.Hours, l.Rate, l.Amount, l.CreatedAt, l.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, l)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// EarningsGross holds per-type gross sums for one payroll entry's additional earnings.
type EarningsGross struct {
	Total float64
	CPP   float64
	EI    float64
	// VacationableTotal is the portion of additional earnings included in vacation-pay base.
	VacationableTotal float64
}

// EarningsGrossForEntry returns gross breakdowns by CPP/EI/vacationable flags.
func (s *Store) EarningsGrossForEntry(entryID string) EarningsGross {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT ee.amount,
		       COALESCE(ec.cpp,1), COALESCE(ec.ei,1), COALESCE(ec.vacationable,0)
		FROM payroll_entry_earnings ee
		LEFT JOIN payroll_earnings_codes ec ON ec.id = ee.earnings_code_id
		WHERE ee.entry_id = ?`, entryID)
	if err != nil {
		return EarningsGross{}
	}
	defer rows.Close()

	var g EarningsGross
	for rows.Next() {
		var amount float64
		var cpp, ei, vacationable int
		if rows.Scan(&amount, &cpp, &ei, &vacationable) != nil {
			continue
		}
		g.Total += amount
		if cpp == 1 {
			g.CPP += amount
		}
		if ei == 1 {
			g.EI += amount
		}
		if vacationable == 1 {
			g.VacationableTotal += amount
		}
	}
	return g
}

// ── Company Rules ────────────────────────────────────────────────────────────

// GetCompanyRules retrieves (or returns defaults for) a company's payroll rules.
func (s *Store) GetCompanyRules(companyID string) models.PayrollCompanyRules {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := models.PayrollCompanyRules{
		CompanyID:      companyID,
		VacationRate:   0.04,
		VacationMethod: "per_period",
	}
	_ = s.db.QueryRow(`SELECT vacation_rate, vacation_method, updated_at
		FROM payroll_company_rules WHERE company_id=?`, companyID).
		Scan(&r.VacationRate, &r.VacationMethod, &r.UpdatedAt)
	return r
}

// UpsertCompanyRules saves company payroll rules (insert or replace).
func (s *Store) UpsertCompanyRules(r models.PayrollCompanyRules) (models.PayrollCompanyRules, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	r.UpdatedAt = now
	_, err := s.db.Exec(`
		INSERT INTO payroll_company_rules (company_id, vacation_rate, vacation_method, updated_at)
		VALUES (?,?,?,?)
		ON CONFLICT(company_id) DO UPDATE SET
		  vacation_rate=excluded.vacation_rate,
		  vacation_method=excluded.vacation_method,
		  updated_at=excluded.updated_at`,
		r.CompanyID, r.VacationRate, r.VacationMethod, r.UpdatedAt,
	)
	return r, err
}
