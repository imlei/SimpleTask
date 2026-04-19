package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/models"
)

// ─── Schema migrations ────────────────────────────────────────────────────────

func ensurePayrollROETable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_roe (
  id                         TEXT PRIMARY KEY,
  company_id                 TEXT NOT NULL DEFAULT '',
  employee_id                TEXT NOT NULL DEFAULT '',
  serial_number              TEXT NOT NULL DEFAULT '',
  status                     TEXT NOT NULL DEFAULT 'draft',
  reason_code                TEXT NOT NULL DEFAULT '',
  reason_code_other          TEXT NOT NULL DEFAULT '',
  first_day_worked           TEXT NOT NULL DEFAULT '',
  last_day_paid              TEXT NOT NULL DEFAULT '',
  final_pay_period_end       TEXT NOT NULL DEFAULT '',
  occupation                 TEXT NOT NULL DEFAULT '',
  expected_recall_date       TEXT NOT NULL DEFAULT '',
  recall_unknown             INTEGER NOT NULL DEFAULT 0,
  total_insurable_hours      REAL NOT NULL DEFAULT 0,
  insurable_earnings_json    TEXT NOT NULL DEFAULT '[]',
  total_insurable_earnings   REAL NOT NULL DEFAULT 0,
  vacation_pay               REAL NOT NULL DEFAULT 0,
  vacation_pay_type          TEXT NOT NULL DEFAULT '',
  statutory_holiday_pay      REAL NOT NULL DEFAULT 0,
  other_moneys_json          TEXT NOT NULL DEFAULT '[]',
  comments                   TEXT NOT NULL DEFAULT '',
  pay_frequency              TEXT NOT NULL DEFAULT '',
  source_hash                TEXT NOT NULL DEFAULT '',
  created_at                 TEXT NOT NULL DEFAULT '',
  updated_at                 TEXT NOT NULL DEFAULT '',
  issued_at                  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_payroll_roe_company   ON payroll_roe (company_id);
CREATE INDEX IF NOT EXISTS idx_payroll_roe_employee  ON payroll_roe (employee_id);
`)
	return err
}

func ensurePayrollROEAuditLogsTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_roe_audit_logs (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  roe_id         TEXT NOT NULL DEFAULT '',
  action         TEXT NOT NULL DEFAULT '',
  actor_username TEXT NOT NULL DEFAULT '',
  note           TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_roe_audit_roe ON payroll_roe_audit_logs (roe_id);
`)
	return err
}

// ensureEmployeeROEColumns adds termination/recall columns to payroll_employees.
func ensureEmployeeROEColumns(db *sql.DB) error {
	want := []struct {
		Name string
		DDL  string
	}{
		{Name: "termination_date",  DDL: "ALTER TABLE payroll_employees ADD COLUMN termination_date TEXT NOT NULL DEFAULT ''"},
		{Name: "roe_recall_date",   DDL: "ALTER TABLE payroll_employees ADD COLUMN roe_recall_date TEXT NOT NULL DEFAULT ''"},
		{Name: "roe_recall_unknown", DDL: "ALTER TABLE payroll_employees ADD COLUMN roe_recall_unknown INTEGER NOT NULL DEFAULT 0"},
	}
	rows, err := db.Query(`PRAGMA table_info(payroll_employees)`)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk) == nil {
			existing[name] = true
		}
	}
	rows.Close()
	for _, c := range want {
		if !existing[c.Name] {
			if _, err := db.Exec(c.DDL); err != nil {
				return err
			}
		}
	}
	return nil
}

// ─── ID generation ────────────────────────────────────────────────────────────

func (s *Store) nextROEID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var max int
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(CAST(SUBSTR(id,4) AS INTEGER)),0) FROM payroll_roe WHERE id LIKE 'ROE%'`).Scan(&max)
	return fmt.Sprintf("ROE%05d", max+1)
}

func (s *Store) nextROESerialNumber() string {
	var max int
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(CAST(serial_number AS INTEGER)),0) FROM payroll_roe WHERE serial_number GLOB '[0-9]*'`).Scan(&max)
	return fmt.Sprintf("%d", max+1)
}

// ─── Lookback window per pay frequency ───────────────────────────────────────

// lookbackCount returns the number of pay periods to collect for Block 15B.
// Source: CRA ROE instructions — W=53, B=27, S=25, M=13.
func lookbackCount(payFrequency string) int {
	switch strings.ToLower(payFrequency) {
	case "weekly":
		return 53
	case "biweekly", "bi-weekly":
		return 27
	case "semi-monthly", "semimonthly":
		return 25
	case "monthly":
		return 13
	default:
		return 27 // safe default (biweekly)
	}
}

// ─── Insurable earnings per period ───────────────────────────────────────────

type periodInsurable struct {
	periodID      string
	periodEnd     string
	payDate       string
	rawEarnings   float64 // EI-insurable gross before annual cap
	hours         float64
	approximated  bool
}

// computeInsurableForPeriod fetches EI-insurable earnings for one payroll entry.
// It joins entry_earnings + earnings_codes WHERE ei=1. Falls back to gross_pay.
func (s *Store) computeInsurableForPeriod(employeeID, periodID string) (earnings, hours float64, approx bool, err error) {
	// Sum EI-insurable earnings from detailed line items
	row := s.db.QueryRow(`
		SELECT COALESCE(SUM(ee.amount),0), COUNT(ee.id)
		FROM payroll_entry_earnings ee
		JOIN payroll_earnings_codes ec ON ec.id = ee.earnings_code_id
		JOIN payroll_entries e ON e.id = ee.entry_id
		WHERE e.employee_id = ? AND e.period_id = ? AND ec.ei = 1
	`, employeeID, periodID)
	var total float64
	var cnt int
	if err = row.Scan(&total, &cnt); err != nil {
		return
	}

	if cnt > 0 {
		// Also get hours from entry_earnings for EI codes
		var h float64
		_ = s.db.QueryRow(`
			SELECT COALESCE(SUM(ee.hours),0)
			FROM payroll_entry_earnings ee
			JOIN payroll_earnings_codes ec ON ec.id = ee.earnings_code_id
			JOIN payroll_entries e ON e.id = ee.entry_id
			WHERE e.employee_id = ? AND e.period_id = ? AND ec.ei = 1
		`, employeeID, periodID).Scan(&h)
		earnings = total
		hours = h
		return
	}

	// Fallback: use payroll_entries.gross_pay + hours
	var grossPay, entryHours float64
	if err = s.db.QueryRow(`
		SELECT COALESCE(gross_pay,0), COALESCE(hours,0)
		FROM payroll_entries WHERE employee_id = ? AND period_id = ?
	`, employeeID, periodID).Scan(&grossPay, &entryHours); err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return
	}
	earnings = grossPay
	hours = entryHours
	approx = true
	return
}

// ─── MEI cap allocation ───────────────────────────────────────────────────────

// maxInsurableForYear returns the annual maximum insurable earnings for a given year.
func (s *Store) maxInsurableForYear(year int) float64 {
	var mei float64
	_ = s.db.QueryRow(`SELECT COALESCE(max_insurable,0) FROM payroll_rate_settings WHERE year=?`, year).Scan(&mei)
	if mei <= 0 {
		// Sensible defaults if not configured
		switch {
		case year >= 2025:
			return 65700
		case year == 2024:
			return 63200
		default:
			return 61500
		}
	}
	return mei
}

// applyAnnualEICapAllocation applies per-year MEI caps to a chronologically-sorted
// slice of raw insurable earnings. It tracks how much cap was consumed before the
// lookback window by querying YTD earnings from prior finalized periods in the same year.
func (s *Store) applyAnnualEICapAllocation(employeeID, companyID string, periods []periodInsurable) []periodInsurable {
	// Process from oldest (last index) to newest (index 0) — periods are newest-first
	// We need to know, for each calendar year, how much insurable earnings were paid
	// in periods BEFORE the ones we are about to include.
	type yearState struct {
		cap     float64
		usedPre float64 // consumed in finalized periods outside our lookback window
	}
	ys := map[int]*yearState{}

	// Collect period IDs we are including (for exclusion in pre-window query)
	includedIDs := make([]string, len(periods))
	for i, p := range periods {
		includedIDs[i] = p.periodID
	}
	placeholders := strings.Repeat("?,", len(includedIDs))
	placeholders = strings.TrimRight(placeholders, ",")

	// For each year represented in our set, compute how much was already consumed
	// by earlier finalized periods not in our window.
	years := map[int]bool{}
	for _, p := range periods {
		if len(p.periodEnd) >= 4 {
			if y, e := strconv.Atoi(p.periodEnd[:4]); e == nil {
				years[y] = true
			}
		}
	}
	for yr := range years {
		mei := s.maxInsurableForYear(yr)
		ys[yr] = &yearState{cap: mei}
		// Sum insurable earnings from finalized periods in same year NOT in our window
		args := []any{employeeID, companyID, fmt.Sprintf("%d-%%", yr)}
		for _, id := range includedIDs {
			args = append(args, id)
		}
		var preUsed float64
		// Approximate from ei_ee premiums × (1/rate) or just gross_pay — we use gross_pay sum here
		// for simplicity, matching fallback behaviour. A future improvement could use entry_earnings.
		query := fmt.Sprintf(`
			SELECT COALESCE(SUM(e.gross_pay),0)
			FROM payroll_entries e
			JOIN payroll_periods pp ON pp.id = e.period_id
			WHERE e.employee_id = ? AND e.company_id = ?
			  AND pp.pay_date LIKE ? AND pp.status = 'finalized'
			  AND e.period_id NOT IN (%s)
		`, placeholders)
		_ = s.db.QueryRow(query, args...).Scan(&preUsed)
		ys[yr].usedPre = preUsed
	}

	// Walk periods oldest→newest (reverse slice), apply cap
	result := make([]periodInsurable, len(periods))
	copy(result, periods)
	// Periods are sorted newest-first; iterate from last element to first
	remaining := map[int]float64{}
	for yr, state := range ys {
		remaining[yr] = state.cap - state.usedPre
		if remaining[yr] < 0 {
			remaining[yr] = 0
		}
	}
	for i := len(result) - 1; i >= 0; i-- {
		p := &result[i]
		if len(p.periodEnd) < 4 {
			continue
		}
		yr, err := strconv.Atoi(p.periodEnd[:4])
		if err != nil {
			continue
		}
		rem, ok := remaining[yr]
		if !ok {
			rem = s.maxInsurableForYear(yr)
		}
		capped := p.rawEarnings
		if capped > rem {
			capped = rem
		}
		p.rawEarnings = capped
		remaining[yr] = rem - capped
	}
	return result
}

// ─── Build ROE periods (Block 15B) ───────────────────────────────────────────

// BuildROEPeriods collects the last N finalized pay periods for an employee,
// ending on or before finalPeriodEnd, and computes insurable earnings/hours.
func (s *Store) BuildROEPeriods(employeeID, companyID, finalPeriodEnd, payFrequency string) ([]models.ROEPeriodEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := lookbackCount(payFrequency)

	rows, err := s.db.Query(`
		SELECT pp.id, pp.period_end, pp.pay_date
		FROM payroll_periods pp
		JOIN payroll_entries pe ON pe.period_id = pp.id AND pe.employee_id = ?
		WHERE pp.company_id = ? AND pp.status = 'finalized'
		  AND pp.period_end <= ?
		ORDER BY pp.period_end DESC
		LIMIT ?
	`, employeeID, companyID, finalPeriodEnd, n)
	if err != nil {
		return nil, err
	}
	var raw []periodInsurable
	for rows.Next() {
		var p periodInsurable
		if err := rows.Scan(&p.periodID, &p.periodEnd, &p.payDate); err != nil {
			continue
		}
		raw = append(raw, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Compute insurable earnings for each period
	for i := range raw {
		earnings, hours, approx, err := s.computeInsurableForPeriod(employeeID, raw[i].periodID)
		if err != nil {
			return nil, err
		}
		raw[i].rawEarnings = earnings
		raw[i].hours = hours
		raw[i].approximated = approx
	}

	// Apply annual MEI cap
	capped := s.applyAnnualEICapAllocation(employeeID, companyID, raw)

	result := make([]models.ROEPeriodEntry, len(capped))
	for i, p := range capped {
		result[i] = models.ROEPeriodEntry{
			PeriodNo:          i + 1,
			PeriodID:          p.periodID,
			PeriodEndDate:     p.periodEnd,
			InsurableEarnings: roundROE(p.rawEarnings),
			InsurableHours:    roundROE(p.hours),
			Approximated:      p.approximated,
		}
	}
	return result, nil
}

// ─── ROE source hash ──────────────────────────────────────────────────────────

func (s *Store) computeROESourceHash(employeeID, companyID string) (string, error) {
	rows, err := s.db.Query(`
		SELECT pp.id, pp.pay_date, pp.updated_at
		FROM payroll_periods pp
		JOIN payroll_entries pe ON pe.period_id = pp.id AND pe.employee_id = ?
		WHERE pp.company_id = ? AND pp.status = 'finalized'
		ORDER BY pp.id
	`, employeeID, companyID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var id, payDate, updatedAt string
		if rows.Scan(&id, &payDate, &updatedAt) == nil {
			parts = append(parts, id+":"+payDate+":"+updatedAt)
		}
	}
	sort.Strings(parts)
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", h), nil
}

// ─── Generate / Regenerate ROE draft ─────────────────────────────────────────

// GenerateROEDraft creates (or re-creates) a draft ROE for the given employee.
// If an existing draft exists, it is replaced. Issued ROEs cannot be regenerated.
func (s *Store) GenerateROEDraft(
	companyID, employeeID, actorUsername string,
	employee models.PayrollEmployee,
) (models.ROEDetail, error) {

	// Determine final pay period end from most recent finalized period
	var finalPeriodEnd, lastPayDate string
	termDate := strings.TrimSpace(employee.TerminationDate)
	query := `
		SELECT pp.period_end, pp.pay_date
		FROM payroll_periods pp
		JOIN payroll_entries pe ON pe.period_id = pp.id AND pe.employee_id = ?
		WHERE pp.company_id = ? AND pp.status = 'finalized'`
	if termDate != "" {
		query += ` AND pp.period_end <= ?`
		row := s.db.QueryRow(query+` ORDER BY pp.period_end DESC LIMIT 1`, employeeID, companyID, termDate)
		_ = row.Scan(&finalPeriodEnd, &lastPayDate)
	} else {
		row := s.db.QueryRow(query+` ORDER BY pp.period_end DESC LIMIT 1`, employeeID, companyID)
		_ = row.Scan(&finalPeriodEnd, &lastPayDate)
	}

	if finalPeriodEnd == "" {
		return models.ROEDetail{}, fmt.Errorf("no finalized payroll periods found for employee %s", employeeID)
	}

	// Check for existing ROE — block if issued
	var existingID, existingStatus string
	_ = s.db.QueryRow(`SELECT id, status FROM payroll_roe WHERE company_id=? AND employee_id=? ORDER BY created_at DESC LIMIT 1`,
		companyID, employeeID).Scan(&existingID, &existingStatus)
	if existingStatus == string(models.ROEStatusIssued) {
		return models.ROEDetail{}, fmt.Errorf("ROE %s has already been issued and cannot be regenerated", existingID)
	}

	// Build Block 15B periods
	periods, err := s.BuildROEPeriods(employeeID, companyID, finalPeriodEnd, employee.PayFrequency)
	if err != nil {
		return models.ROEDetail{}, fmt.Errorf("building ROE periods: %w", err)
	}

	// Compute totals
	var totalHours, totalEarnings float64
	for _, p := range periods {
		totalHours += p.InsurableHours
		totalEarnings += p.InsurableEarnings
	}

	// Source hash
	sourceHash, _ := s.computeROESourceHash(employeeID, companyID)

	// Recall date
	recallDate := strings.TrimSpace(employee.ROERecallDate)
	recallUnknown := employee.ROERecallUnknown

	periodsJSON, _ := json.Marshal(periods)
	now := time.Now().UTC().Format(time.RFC3339)

	s.mu.Lock()
	defer s.mu.Unlock()

	var roeID string
	action := "generated"

	if existingID != "" {
		// Regenerate: delete old record and reuse path
		if _, err := s.db.Exec(`DELETE FROM payroll_roe WHERE id=?`, existingID); err != nil {
			return models.ROEDetail{}, err
		}
		roeID = existingID
		action = "regenerated"
	} else {
		roeID = s.nextROEIDLocked()
	}

	serialNum := s.nextROESerialNumber()

	_, err = s.db.Exec(`
		INSERT INTO payroll_roe (
			id, company_id, employee_id, serial_number, status,
			reason_code, reason_code_other,
			first_day_worked, last_day_paid, final_pay_period_end,
			occupation, expected_recall_date, recall_unknown,
			total_insurable_hours, insurable_earnings_json, total_insurable_earnings,
			vacation_pay, vacation_pay_type, statutory_holiday_pay, other_moneys_json,
			comments, pay_frequency, source_hash, created_at, updated_at, issued_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`,
		roeID, companyID, employeeID, serialNum, string(models.ROEStatusDraft),
		"", "",
		employee.HireDate, lastPayDate, finalPeriodEnd,
		employee.Position, recallDate, btoi(recallUnknown),
		roundROE(totalHours), string(periodsJSON), roundROE(totalEarnings),
		0, "", 0, "[]",
		"", employee.PayFrequency, sourceHash, now, now, "",
	)
	if err != nil {
		return models.ROEDetail{}, fmt.Errorf("inserting ROE: %w", err)
	}

	// Audit log
	_, _ = s.db.Exec(`
		INSERT INTO payroll_roe_audit_logs (roe_id, action, actor_username, note, created_at)
		VALUES (?,?,?,?,?)
	`, roeID, action, actorUsername, fmt.Sprintf("pay_frequency=%s periods=%d", employee.PayFrequency, len(periods)), now)

	return s.getROEDetailLocked(roeID)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) nextROEIDLocked() string {
	var max int
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(CAST(SUBSTR(id,4) AS INTEGER)),0) FROM payroll_roe WHERE id LIKE 'ROE%'`).Scan(&max)
	return fmt.Sprintf("ROE%05d", max+1)
}

// ─── Validate ROE ─────────────────────────────────────────────────────────────

func (s *Store) ValidateROE(roeID string) []models.ROEValidationError {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []models.ROEValidationError
	add := func(code, msg string) {
		errs = append(errs, models.ROEValidationError{Code: code, Message: msg})
	}

	var reasonCode, firstDay, lastDay, finalEnd, occupation, payFreq string
	var totalHours, totalEarnings float64
	err := s.db.QueryRow(`
		SELECT reason_code, first_day_worked, last_day_paid, final_pay_period_end,
		       occupation, pay_frequency, total_insurable_hours, total_insurable_earnings
		FROM payroll_roe WHERE id=?
	`, roeID).Scan(&reasonCode, &firstDay, &lastDay, &finalEnd, &occupation, &payFreq, &totalHours, &totalEarnings)
	if err != nil {
		add("roe_not_found", "ROE record not found")
		return errs
	}

	if strings.TrimSpace(reasonCode) == "" {
		add("missing_reason_code", "Block 16: reason code is required")
	}
	if strings.TrimSpace(firstDay) == "" {
		add("missing_first_day_worked", "Block 10: first day worked is required")
	}
	if strings.TrimSpace(lastDay) == "" {
		add("missing_last_day_paid", "Block 11: last day paid is required")
	}
	if strings.TrimSpace(finalEnd) == "" {
		add("missing_final_pay_period_end", "Block 12: final pay period ending date is required")
	}
	if strings.TrimSpace(occupation) == "" {
		add("missing_occupation", "Block 13: occupation is required")
	}
	if strings.TrimSpace(payFreq) == "" {
		add("missing_pay_frequency", "Pay frequency must be set to determine Block 15B lookback")
	}
	if totalEarnings <= 0 {
		add("no_insurable_earnings", "Block 15C: total insurable earnings must be greater than zero")
	}

	// Employee SIN check
	var sinMasked, province string
	_ = s.db.QueryRow(`
		SELECT e.sin_encrypted, e.province
		FROM payroll_roe r
		JOIN payroll_employees e ON e.id = r.employee_id
		WHERE r.id = ?
	`, roeID).Scan(&sinMasked, &province)
	if strings.TrimSpace(sinMasked) == "" {
		add("missing_sin", "Employee SIN is required for ROE submission")
	}
	if strings.TrimSpace(province) == "" {
		add("missing_province", "Employee province of employment is required")
	}

	return errs
}

// ─── Issue ROE ────────────────────────────────────────────────────────────────

// IssueROE locks a draft ROE. Returns an error if validation fails.
func (s *Store) IssueROE(roeID, actorUsername string) error {
	if errs := s.ValidateROE(roeID); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Message
		}
		return fmt.Errorf("validation failed: %s", strings.Join(msgs, "; "))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var status string
	if err := s.db.QueryRow(`SELECT status FROM payroll_roe WHERE id=?`, roeID).Scan(&status); err != nil {
		return fmt.Errorf("ROE not found: %w", err)
	}
	if status == string(models.ROEStatusIssued) {
		return nil // idempotent
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE payroll_roe SET status='issued', issued_at=?, updated_at=? WHERE id=?`, now, now, roeID); err != nil {
		return err
	}
	_, _ = s.db.Exec(`
		INSERT INTO payroll_roe_audit_logs (roe_id, action, actor_username, note, created_at)
		VALUES (?,?,?,?,?)
	`, roeID, "issued", actorUsername, "", now)
	return nil
}

// ─── Update ROE fields ────────────────────────────────────────────────────────

type ROEUpdateInput struct {
	ReasonCode          string
	ReasonCodeOther     string
	FirstDayWorked      string
	LastDayPaid         string
	FinalPayPeriodEnd   string
	Occupation          string
	ExpectedRecallDate  string
	RecallUnknown       bool
	VacationPay         float64
	VacationPayType     string
	StatutoryHolidayPay float64
	OtherMoneys         []models.ROEOtherMoney
	Comments            string
}

func (s *Store) UpdateROEFields(roeID string, in ROEUpdateInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var status string
	if err := s.db.QueryRow(`SELECT status FROM payroll_roe WHERE id=?`, roeID).Scan(&status); err != nil {
		return fmt.Errorf("ROE not found: %w", err)
	}
	if status == string(models.ROEStatusIssued) {
		return fmt.Errorf("cannot edit an issued ROE")
	}

	otherJSON, _ := json.Marshal(in.OtherMoneys)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE payroll_roe SET
			reason_code=?, reason_code_other=?,
			first_day_worked=?, last_day_paid=?, final_pay_period_end=?,
			occupation=?, expected_recall_date=?, recall_unknown=?,
			vacation_pay=?, vacation_pay_type=?, statutory_holiday_pay=?,
			other_moneys_json=?, comments=?, updated_at=?
		WHERE id=?
	`,
		in.ReasonCode, in.ReasonCodeOther,
		in.FirstDayWorked, in.LastDayPaid, in.FinalPayPeriodEnd,
		in.Occupation, in.ExpectedRecallDate, btoi(in.RecallUnknown),
		in.VacationPay, in.VacationPayType, in.StatutoryHolidayPay,
		string(otherJSON), in.Comments, now,
		roeID,
	)
	return err
}

// ─── List / Get ───────────────────────────────────────────────────────────────

func (s *Store) ListROEs(companyID string) ([]models.ROE, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, company_id, employee_id, serial_number, status,
		       reason_code, reason_code_other,
		       first_day_worked, last_day_paid, final_pay_period_end,
		       occupation, expected_recall_date, recall_unknown,
		       total_insurable_hours, total_insurable_earnings,
		       vacation_pay, vacation_pay_type, statutory_holiday_pay,
		       comments, pay_frequency, source_hash, created_at, updated_at, issued_at
		FROM payroll_roe WHERE company_id=? ORDER BY created_at DESC
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []models.ROE
	for rows.Next() {
		r, err := scanROERow(rows.Scan)
		if err == nil {
			result = append(result, r)
		}
	}
	return result, rows.Err()
}

func (s *Store) GetROEDetail(id string) (models.ROEDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getROEDetailLocked(id)
}

func (s *Store) getROEDetailLocked(id string) (models.ROEDetail, error) {
	row := s.db.QueryRow(`
		SELECT id, company_id, employee_id, serial_number, status,
		       reason_code, reason_code_other,
		       first_day_worked, last_day_paid, final_pay_period_end,
		       occupation, expected_recall_date, recall_unknown,
		       total_insurable_hours, total_insurable_earnings,
		       vacation_pay, vacation_pay_type, statutory_holiday_pay,
		       comments, pay_frequency, source_hash, created_at, updated_at, issued_at
		FROM payroll_roe WHERE id=?
	`, id)
	roe, err := scanROERow(row.Scan)
	if err != nil {
		return models.ROEDetail{}, fmt.Errorf("ROE not found: %w", err)
	}

	// Load insurable_earnings_json
	var periodsJSON string
	_ = s.db.QueryRow(`SELECT insurable_earnings_json FROM payroll_roe WHERE id=?`, id).Scan(&periodsJSON)
	if periodsJSON != "" && periodsJSON != "[]" {
		_ = json.Unmarshal([]byte(periodsJSON), &roe.InsurableEarningsPeriods)
	}

	// Load other_moneys_json
	var otherJSON string
	_ = s.db.QueryRow(`SELECT other_moneys_json FROM payroll_roe WHERE id=?`, id).Scan(&otherJSON)
	if otherJSON != "" && otherJSON != "[]" {
		_ = json.Unmarshal([]byte(otherJSON), &roe.OtherMoneys)
	}

	// Load employee
	var emp models.PayrollEmployee
	empRow := s.db.QueryRow(`
		SELECT id, company_id, legal_name, nickname, email, mobile, position,
		       province, sin_encrypted, date_of_birth, hire_date,
		       member_type, salary_type, status, pay_rate, pay_rate_unit,
		       pays_per_year, pay_frequency, hours_per_week,
		       td1_federal, td1_provincial, auto_vacation, created_at, updated_at,
		       COALESCE(termination_date,''), COALESCE(roe_recall_date,''), COALESCE(roe_recall_unknown,0)
		FROM payroll_employees WHERE id=?
	`, roe.EmployeeID)
	var sinEnc string
	var memberType, salaryType, autoVac, recallUnk int
	_ = empRow.Scan(
		&emp.ID, &emp.CompanyID, &emp.LegalName, &emp.Nickname, &emp.Email, &emp.Mobile, &emp.Position,
		&emp.Province, &sinEnc, &emp.DateOfBirth, &emp.HireDate,
		&memberType, &salaryType, &emp.Status, &emp.PayRate, &emp.PayRateUnit,
		&emp.PaysPerYear, &emp.PayFrequency, &emp.HoursPerWeek,
		&emp.TD1Federal, &emp.TD1Provincial, &autoVac, &emp.CreatedAt, &emp.UpdatedAt,
		&emp.TerminationDate, &emp.ROERecallDate, &recallUnk,
	)
	emp.MemberType = memberType
	emp.SalaryType = salaryType
	emp.AutoVacation = autoVac == 1
	emp.ROERecallUnknown = recallUnk == 1
	// Mask SIN
	if sinEnc != "" {
		emp.SINMasked = "***-***-" + sinEnc[max(0, len(sinEnc)-3):]
	}

	// Load company
	var company models.PayrollCompany
	_ = s.db.QueryRow(`
		SELECT id, name, legal_name, business_number, email, phone, address, province,
		       pay_frequency, status, created_at, updated_at
		FROM payroll_companies WHERE id=?
	`, roe.CompanyID).Scan(
		&company.ID, &company.Name, &company.LegalName, &company.BusinessNumber,
		&company.Email, &company.Phone, &company.Address, &company.Province,
		&company.PayFrequency, &company.Status, &company.CreatedAt, &company.UpdatedAt,
	)

	// Load audit logs
	logRows, _ := s.db.Query(`
		SELECT id, roe_id, action, actor_username, note, created_at
		FROM payroll_roe_audit_logs WHERE roe_id=? ORDER BY id ASC
	`, id)
	var logs []models.ROEAuditLog
	if logRows != nil {
		for logRows.Next() {
			var l models.ROEAuditLog
			if logRows.Scan(&l.ID, &l.ROEID, &l.Action, &l.ActorUsername, &l.Note, &l.CreatedAt) == nil {
				logs = append(logs, l)
			}
		}
		logRows.Close()
	}

	return models.ROEDetail{
		ROE:       roe,
		Employee:  emp,
		Company:   company,
		AuditLogs: logs,
	}, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// scanROERow scans a ROE row (without the JSON blob columns).
func scanROERow(scan func(dest ...any) error) (models.ROE, error) {
	var r models.ROE
	var status string
	var recallUnknown int
	err := scan(
		&r.ID, &r.CompanyID, &r.EmployeeID, &r.SerialNumber, &status,
		&r.ReasonCode, &r.ReasonCodeOther,
		&r.FirstDayWorked, &r.LastDayPaid, &r.FinalPayPeriodEnd,
		&r.Occupation, &r.ExpectedRecallDate, &recallUnknown,
		&r.TotalInsurableHours, &r.TotalInsurableEarnings,
		&r.VacationPay, &r.VacationPayType, &r.StatutoryHolidayPay,
		&r.Comments, &r.PayFrequency, &r.SourceHash,
		&r.CreatedAt, &r.UpdatedAt, &r.IssuedAt,
	)
	if err != nil {
		return r, err
	}
	r.Status = models.ROEStatus(status)
	r.RecallUnknown = recallUnknown == 1
	return r, nil
}

// roundROE rounds to 2 decimal places.
func roundROE(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
