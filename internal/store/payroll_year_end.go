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

// ── ID generation ────────────────────────────────────────────────────────────

func (s *Store) nextYearEndReturnID() string {
	var maxID string
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(id),'YR00000') FROM payroll_year_end_returns`).Scan(&maxID)
	num := 0
	if len(maxID) > 2 {
		num, _ = strconv.Atoi(maxID[2:])
	}
	return fmt.Sprintf("YR%05d", num+1)
}

func (s *Store) nextT4SlipID() string {
	var maxID string
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(id),'T400000') FROM payroll_t4_slips`).Scan(&maxID)
	num := 0
	if len(maxID) > 2 {
		num, _ = strconv.Atoi(maxID[2:])
	}
	return fmt.Sprintf("T4%05d", num+1)
}

func (s *Store) nextT4SummaryID() string {
	var maxID string
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(id),'TS00000') FROM payroll_t4_summary`).Scan(&maxID)
	num := 0
	if len(maxID) > 2 {
		num, _ = strconv.Atoi(maxID[2:])
	}
	return fmt.Sprintf("TS%05d", num+1)
}

// ── YearEndReturn CRUD ───────────────────────────────────────────────────────

func (s *Store) ListYearEndReturns(companyID string) []models.YearEndReturn {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT r.id, r.company_id, r.payroll_account_number, r.calendar_year,
		       r.status, r.generated_at, r.finalized_at, r.source_hash,
		       r.created_at, r.updated_at,
		       COUNT(sl.id)
		FROM payroll_year_end_returns r
		LEFT JOIN payroll_t4_slips sl ON sl.year_end_return_id = r.id
		WHERE r.company_id = ?
		GROUP BY r.id
		ORDER BY r.calendar_year DESC`, companyID)
	if err != nil {
		return []models.YearEndReturn{}
	}
	defer rows.Close()

	var list []models.YearEndReturn
	for rows.Next() {
		var r models.YearEndReturn
		var status string
		if err := rows.Scan(&r.ID, &r.CompanyID, &r.PayrollAccountNumber,
			&r.CalendarYear, &status, &r.GeneratedAt, &r.FinalizedAt,
			&r.SourceHash, &r.CreatedAt, &r.UpdatedAt, &r.SlipCount); err != nil {
			continue
		}
		r.Status = models.YearEndReturnStatus(status)
		list = append(list, r)
	}
	if list == nil {
		return []models.YearEndReturn{}
	}
	return list
}

func (s *Store) GetYearEndReturn(id string) (models.YearEndReturn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getYearEndReturnLocked(id)
}

func (s *Store) getYearEndReturnLocked(id string) (models.YearEndReturn, error) {
	var r models.YearEndReturn
	var status string
	err := s.db.QueryRow(`
		SELECT id, company_id, payroll_account_number, calendar_year,
		       status, generated_at, finalized_at, source_hash,
		       created_at, updated_at
		FROM payroll_year_end_returns WHERE id = ?`, id).
		Scan(&r.ID, &r.CompanyID, &r.PayrollAccountNumber, &r.CalendarYear,
			&status, &r.GeneratedAt, &r.FinalizedAt, &r.SourceHash,
			&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, ErrNotFound
	}
	r.Status = models.YearEndReturnStatus(status)
	return r, nil
}

func (s *Store) GetYearEndReturnDetail(id string) (models.YearEndReturnDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ret, err := s.getYearEndReturnLocked(id)
	if err != nil {
		return models.YearEndReturnDetail{}, err
	}

	sum := s.getT4SummaryLocked(id)
	slips := s.listT4SlipsLocked(id)
	logs := s.listAuditLogsLocked(id)

	return models.YearEndReturnDetail{
		Return:    ret,
		Summary:   sum,
		Slips:     slips,
		AuditLogs: logs,
	}, nil
}

// GetYearEndReturnByScope looks up an existing return for company+account+year.
func (s *Store) GetYearEndReturnByScope(companyID, payrollAccountNumber string, year int) (models.YearEndReturn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var r models.YearEndReturn
	var status string
	err := s.db.QueryRow(`
		SELECT id, company_id, payroll_account_number, calendar_year,
		       status, generated_at, finalized_at, source_hash, created_at, updated_at
		FROM payroll_year_end_returns
		WHERE company_id = ? AND payroll_account_number = ? AND calendar_year = ?`,
		companyID, payrollAccountNumber, year).
		Scan(&r.ID, &r.CompanyID, &r.PayrollAccountNumber, &r.CalendarYear,
			&status, &r.GeneratedAt, &r.FinalizedAt, &r.SourceHash,
			&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, ErrNotFound
	}
	r.Status = models.YearEndReturnStatus(status)
	return r, nil
}

// createYearEndReturn inserts a new return header; caller must hold mu.
func (s *Store) createYearEndReturnLocked(companyID, payrollAccountNumber string, year int, sourceHash string) (models.YearEndReturn, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	r := models.YearEndReturn{
		ID:                   s.nextYearEndReturnID(),
		CompanyID:            companyID,
		PayrollAccountNumber: payrollAccountNumber,
		CalendarYear:         year,
		Status:               models.YearEndStatusDraft,
		GeneratedAt:          now,
		SourceHash:           sourceHash,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	_, err := s.db.Exec(`
		INSERT INTO payroll_year_end_returns
		  (id, company_id, payroll_account_number, calendar_year,
		   status, generated_at, finalized_at, source_hash, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.CompanyID, r.PayrollAccountNumber, r.CalendarYear,
		string(r.Status), r.GeneratedAt, "", r.SourceHash,
		r.CreatedAt, r.UpdatedAt)
	return r, err
}

// FinalizeYearEndReturn locks the return. Returns error if already finalized.
func (s *Store) FinalizeYearEndReturn(id, actorUsername string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var status string
	err := s.db.QueryRow(`SELECT status FROM payroll_year_end_returns WHERE id = ?`, id).Scan(&status)
	if err != nil {
		return ErrNotFound
	}
	if status == string(models.YearEndStatusFinalized) {
		return fmt.Errorf("return %s is already finalized", id)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
		UPDATE payroll_year_end_returns
		SET status = 'finalized', finalized_at = ?, updated_at = ?
		WHERE id = ?`, now, now, id)
	if err != nil {
		return err
	}
	return s.insertAuditLogLocked(id, "finalized", actorUsername, "")
}

// deleteReturnDataLocked deletes slips + summary for a return; caller holds mu.
func (s *Store) deleteReturnDataLocked(returnID string) error {
	if _, err := s.db.Exec(`DELETE FROM payroll_t4_slips WHERE year_end_return_id = ?`, returnID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM payroll_t4_summary WHERE year_end_return_id = ?`, returnID)
	return err
}

// ── T4 Slip CRUD ─────────────────────────────────────────────────────────────

func (s *Store) listT4SlipsLocked(returnID string) []models.T4Slip {
	rows, err := s.db.Query(`
		SELECT id, year_end_return_id, employee_id, calendar_year,
		       employee_legal_name, employee_sin_masked, employee_address, employee_province,
		       box_14, box_16, box_16a, box_17, box_18, box_20,
		       box_22, box_24, box_26, box_44, box_45, box_46, box_52,
		       employer_cpp, employer_cpp2, employer_ei,
		       other_info_json, unsupported_fields_json,
		       version_no, created_at, updated_at
		FROM payroll_t4_slips
		WHERE year_end_return_id = ?
		ORDER BY employee_legal_name ASC`, returnID)
	if err != nil {
		return []models.T4Slip{}
	}
	defer rows.Close()

	var list []models.T4Slip
	for rows.Next() {
		sl, err := scanT4Slip(rows.Scan)
		if err == nil {
			list = append(list, sl)
		}
	}
	if list == nil {
		return []models.T4Slip{}
	}
	return list
}

func (s *Store) GetT4Slip(slipID string) (models.T4Slip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(`
		SELECT id, year_end_return_id, employee_id, calendar_year,
		       employee_legal_name, employee_sin_masked, employee_address, employee_province,
		       box_14, box_16, box_16a, box_17, box_18, box_20,
		       box_22, box_24, box_26, box_44, box_45, box_46, box_52,
		       employer_cpp, employer_cpp2, employer_ei,
		       other_info_json, unsupported_fields_json,
		       version_no, created_at, updated_at
		FROM payroll_t4_slips WHERE id = ?`, slipID)
	sl, err := scanT4Slip(row.Scan)
	if err != nil {
		return sl, ErrNotFound
	}
	return sl, nil
}

func scanT4Slip(scan func(dest ...any) error) (models.T4Slip, error) {
	var sl models.T4Slip
	var otherJSON, unsupportedJSON string
	err := scan(
		&sl.ID, &sl.YearEndReturnID, &sl.EmployeeID, &sl.CalendarYear,
		&sl.EmployeeLegalName, &sl.EmployeeSINMasked, &sl.EmployeeAddress, &sl.EmployeeProvince,
		&sl.Box14EmploymentIncome, &sl.Box16CPPEmployee, &sl.Box16ACpp2Employee,
		&sl.Box17QPPEmployee, &sl.Box18EIEmployee, &sl.Box20RPPContributions,
		&sl.Box22IncomeTaxDeducted, &sl.Box24EIInsurableEarnings, &sl.Box26CPPPensionableEarnings,
		&sl.Box44UnionDues, &sl.Box45DentalBenefitCode, &sl.Box46CharitableDonations, &sl.Box52PensionAdjustment,
		&sl.EmployerCPP, &sl.EmployerCPP2, &sl.EmployerEI,
		&otherJSON, &unsupportedJSON,
		&sl.VersionNo, &sl.CreatedAt, &sl.UpdatedAt,
	)
	if err != nil {
		return sl, err
	}
	_ = json.Unmarshal([]byte(otherJSON), &sl.OtherInfo)
	_ = json.Unmarshal([]byte(unsupportedJSON), &sl.UnsupportedFields)
	if sl.OtherInfo == nil {
		sl.OtherInfo = []models.T4OtherInfo{}
	}
	return sl, nil
}

func (s *Store) insertT4SlipLocked(sl models.T4Slip) error {
	now := time.Now().UTC().Format(time.RFC3339)
	sl.ID = s.nextT4SlipID()
	sl.VersionNo = 1
	sl.CreatedAt = now
	sl.UpdatedAt = now

	otherJSON, _ := json.Marshal(sl.OtherInfo)
	unsupportedJSON, _ := json.Marshal(sl.UnsupportedFields)

	_, err := s.db.Exec(`
		INSERT INTO payroll_t4_slips
		  (id, year_end_return_id, employee_id, calendar_year,
		   employee_legal_name, employee_sin_masked, employee_address, employee_province,
		   box_14, box_16, box_16a, box_17, box_18, box_20,
		   box_22, box_24, box_26, box_44, box_45, box_46, box_52,
		   employer_cpp, employer_cpp2, employer_ei,
		   other_info_json, unsupported_fields_json,
		   version_no, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sl.ID, sl.YearEndReturnID, sl.EmployeeID, sl.CalendarYear,
		sl.EmployeeLegalName, sl.EmployeeSINMasked, sl.EmployeeAddress, sl.EmployeeProvince,
		sl.Box14EmploymentIncome, sl.Box16CPPEmployee, sl.Box16ACpp2Employee,
		sl.Box17QPPEmployee, sl.Box18EIEmployee, sl.Box20RPPContributions,
		sl.Box22IncomeTaxDeducted, sl.Box24EIInsurableEarnings, sl.Box26CPPPensionableEarnings,
		sl.Box44UnionDues, sl.Box45DentalBenefitCode, sl.Box46CharitableDonations, sl.Box52PensionAdjustment,
		sl.EmployerCPP, sl.EmployerCPP2, sl.EmployerEI,
		string(otherJSON), string(unsupportedJSON),
		sl.VersionNo, sl.CreatedAt, sl.UpdatedAt,
	)
	return err
}

// ── T4 Summary CRUD ──────────────────────────────────────────────────────────

func (s *Store) GetT4Summary(returnID string) models.T4Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getT4SummaryLocked(returnID)
}

func (s *Store) getT4SummaryLocked(returnID string) models.T4Summary {
	var sm models.T4Summary
	_ = s.db.QueryRow(`
		SELECT id, year_end_return_id, slip_count,
		       total_employment_income,
		       total_cpp_employee, total_cpp2_employee, total_qpp_employee,
		       total_ei_employee, total_income_tax_deducted,
		       total_cpp_employer, total_cpp2_employer, total_ei_employer,
		       total_pension_adjustments,
		       contact_name, contact_phone,
		       created_at, updated_at
		FROM payroll_t4_summary
		WHERE year_end_return_id = ?`, returnID).
		Scan(&sm.ID, &sm.YearEndReturnID, &sm.SlipCount,
			&sm.TotalEmploymentIncome,
			&sm.TotalCPPEmployee, &sm.TotalCPP2Employee, &sm.TotalQPPEmployee,
			&sm.TotalEIEmployee, &sm.TotalIncomeTaxDeducted,
			&sm.TotalCPPEmployer, &sm.TotalCPP2Employer, &sm.TotalEIEmployer,
			&sm.TotalPensionAdjustments,
			&sm.ContactName, &sm.ContactPhone,
			&sm.CreatedAt, &sm.UpdatedAt)
	return sm
}

func (s *Store) upsertT4SummaryLocked(sm models.T4Summary) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if sm.ID == "" {
		sm.ID = s.nextT4SummaryID()
		sm.CreatedAt = now
	}
	sm.UpdatedAt = now
	_, err := s.db.Exec(`
		INSERT INTO payroll_t4_summary
		  (id, year_end_return_id, slip_count,
		   total_employment_income,
		   total_cpp_employee, total_cpp2_employee, total_qpp_employee,
		   total_ei_employee, total_income_tax_deducted,
		   total_cpp_employer, total_cpp2_employer, total_ei_employer,
		   total_pension_adjustments,
		   contact_name, contact_phone, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(year_end_return_id) DO UPDATE SET
		  slip_count=excluded.slip_count,
		  total_employment_income=excluded.total_employment_income,
		  total_cpp_employee=excluded.total_cpp_employee,
		  total_cpp2_employee=excluded.total_cpp2_employee,
		  total_qpp_employee=excluded.total_qpp_employee,
		  total_ei_employee=excluded.total_ei_employee,
		  total_income_tax_deducted=excluded.total_income_tax_deducted,
		  total_cpp_employer=excluded.total_cpp_employer,
		  total_cpp2_employer=excluded.total_cpp2_employer,
		  total_ei_employer=excluded.total_ei_employer,
		  total_pension_adjustments=excluded.total_pension_adjustments,
		  contact_name=excluded.contact_name,
		  contact_phone=excluded.contact_phone,
		  updated_at=excluded.updated_at`,
		sm.ID, sm.YearEndReturnID, sm.SlipCount,
		sm.TotalEmploymentIncome,
		sm.TotalCPPEmployee, sm.TotalCPP2Employee, sm.TotalQPPEmployee,
		sm.TotalEIEmployee, sm.TotalIncomeTaxDeducted,
		sm.TotalCPPEmployer, sm.TotalCPP2Employer, sm.TotalEIEmployer,
		sm.TotalPensionAdjustments,
		sm.ContactName, sm.ContactPhone, sm.CreatedAt, sm.UpdatedAt,
	)
	return err
}

// ── T4 Aggregation from finalized payroll ─────────────────────────────────────

// T4EmployeeAggregate holds the annual payroll totals for one employee,
// aggregated strictly from finalized payroll periods.
type T4EmployeeAggregate struct {
	EmployeeID string
	// Sums from payroll_entries
	GrossPay     float64
	CPPEmployee  float64
	CPP2Employee float64
	EIEmployee   float64
	FederalTax   float64
	ProvincialTax float64
	CPPEmployer  float64
	CPP2Employer float64
	EIEmployer   float64
	// Detailed earnings breakdowns (from payroll_entry_earnings + earnings codes)
	EIInsurableEarnings  float64 // SUM of amounts where ec.ei = 1
	CPPPensionableEarnings float64 // SUM of amounts where ec.cpp = 1
	HasDetailedEarnings  bool
}

// AggregateT4FromFinalizedPayroll computes annual payroll totals per employee
// using ONLY finalized periods for the given company + calendar year.
// This is the authoritative source for all T4 box amounts.
func (s *Store) AggregateT4FromFinalizedPayroll(companyID string, calendarYear int) ([]T4EmployeeAggregate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	yearStr := strconv.Itoa(calendarYear)

	rows, err := s.db.Query(`
		SELECT e.employee_id,
		       COALESCE(SUM(e.gross_pay), 0),
		       COALESCE(SUM(e.cpp_ee), 0),
		       COALESCE(SUM(e.cpp2_ee), 0),
		       COALESCE(SUM(e.ei_ee), 0),
		       COALESCE(SUM(e.federal_tax), 0),
		       COALESCE(SUM(e.provincial_tax), 0),
		       COALESCE(SUM(e.cpp_er), 0),
		       COALESCE(SUM(e.cpp2_er), 0),
		       COALESCE(SUM(e.ei_er), 0)
		FROM payroll_entries e
		JOIN payroll_periods p ON p.id = e.period_id
		WHERE p.company_id = ?
		  AND p.status = 'finalized'
		  AND substr(p.pay_date, 1, 4) = ?
		GROUP BY e.employee_id
		ORDER BY e.employee_id ASC`, companyID, yearStr)
	if err != nil {
		return nil, fmt.Errorf("aggregating T4 entries: %w", err)
	}
	defer rows.Close()

	var result []T4EmployeeAggregate
	for rows.Next() {
		var a T4EmployeeAggregate
		if err := rows.Scan(
			&a.EmployeeID,
			&a.GrossPay, &a.CPPEmployee, &a.CPP2Employee, &a.EIEmployee,
			&a.FederalTax, &a.ProvincialTax,
			&a.CPPEmployer, &a.CPP2Employer, &a.EIEmployer,
		); err != nil {
			continue
		}
		result = append(result, a)
	}
	rows.Close()

	// Enrich with detailed earnings breakdown where available.
	for i := range result {
		ei, cpp, hasDetail := s.getDetailedEarningsLocked(result[i].EmployeeID, companyID, yearStr)
		result[i].EIInsurableEarnings = ei
		result[i].CPPPensionableEarnings = cpp
		result[i].HasDetailedEarnings = hasDetail
	}

	return result, nil
}

// getDetailedEarningsLocked computes EI-insurable and CPP-pensionable earnings
// from the payroll_entry_earnings table for employees with detailed earning line items.
// Falls back to (0, 0, false) when no entry_earnings records exist.
func (s *Store) getDetailedEarningsLocked(employeeID, companyID, yearStr string) (eiInsurable, cppPensionable float64, hasDetail bool) {
	// Check if any entry_earnings records exist for this employee+year
	var count int
	_ = s.db.QueryRow(`
		SELECT COUNT(ee.id)
		FROM payroll_entry_earnings ee
		JOIN payroll_entries e ON e.id = ee.entry_id
		JOIN payroll_periods p ON p.id = e.period_id
		WHERE e.employee_id = ?
		  AND p.company_id = ?
		  AND p.status = 'finalized'
		  AND substr(p.pay_date, 1, 4) = ?`,
		employeeID, companyID, yearStr).Scan(&count)

	if count == 0 {
		return 0, 0, false
	}

	_ = s.db.QueryRow(`
		SELECT COALESCE(SUM(ee.amount), 0)
		FROM payroll_entry_earnings ee
		JOIN payroll_earnings_codes ec ON ec.id = ee.earnings_code_id
		JOIN payroll_entries e ON e.id = ee.entry_id
		JOIN payroll_periods p ON p.id = e.period_id
		WHERE e.employee_id = ?
		  AND p.company_id = ?
		  AND p.status = 'finalized'
		  AND substr(p.pay_date, 1, 4) = ?
		  AND ec.ei = 1`,
		employeeID, companyID, yearStr).Scan(&eiInsurable)

	_ = s.db.QueryRow(`
		SELECT COALESCE(SUM(ee.amount), 0)
		FROM payroll_entry_earnings ee
		JOIN payroll_earnings_codes ec ON ec.id = ee.earnings_code_id
		JOIN payroll_entries e ON e.id = ee.entry_id
		JOIN payroll_periods p ON p.id = e.period_id
		WHERE e.employee_id = ?
		  AND p.company_id = ?
		  AND p.status = 'finalized'
		  AND substr(p.pay_date, 1, 4) = ?
		  AND ec.cpp = 1`,
		employeeID, companyID, yearStr).Scan(&cppPensionable)

	return eiInsurable, cppPensionable, true
}

// ComputeSourceHash produces a SHA-256 fingerprint of all finalized period IDs
// and their pay_dates that contribute to a given company+year T4 generation.
// This hash is stored on the YearEndReturn to make the data source traceable.
func (s *Store) ComputeSourceHash(companyID string, calendarYear int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	yearStr := strconv.Itoa(calendarYear)
	rows, err := s.db.Query(`
		SELECT id, pay_date, updated_at
		FROM payroll_periods
		WHERE company_id = ?
		  AND status = 'finalized'
		  AND substr(pay_date, 1, 4) = ?
		ORDER BY id ASC`, companyID, yearStr)
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

// ── Draft generation ─────────────────────────────────────────────────────────

// GenerateOrRegenerateDraft creates or replaces the draft slips + summary for
// a year-end return. Returns error if the return is already finalized.
func (s *Store) GenerateOrRegenerateDraft(
	companyID, payrollAccountNumber string,
	calendarYear int,
	actorUsername string,
	employees map[string]models.PayrollEmployee, // keyed by employee_id
) (models.YearEndReturnDetail, error) {

	// Compute source hash outside the main lock (uses its own lock internally)
	sourceHash, err := s.ComputeSourceHash(companyID, calendarYear)
	if err != nil {
		return models.YearEndReturnDetail{}, fmt.Errorf("computing source hash: %w", err)
	}

	// Aggregate T4 data from finalized payroll
	aggregates, err := s.AggregateT4FromFinalizedPayroll(companyID, calendarYear)
	if err != nil {
		return models.YearEndReturnDetail{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for existing return
	var existingID, existingStatus string
	_ = s.db.QueryRow(`
		SELECT id, status FROM payroll_year_end_returns
		WHERE company_id = ? AND payroll_account_number = ? AND calendar_year = ?`,
		companyID, payrollAccountNumber, calendarYear).
		Scan(&existingID, &existingStatus)

	if existingStatus == string(models.YearEndStatusFinalized) {
		return models.YearEndReturnDetail{}, fmt.Errorf("return for %d is already finalized; cannot regenerate", calendarYear)
	}

	action := "generated"
	var returnID string

	if existingID == "" {
		// Create new return header
		r, err := s.createYearEndReturnLocked(companyID, payrollAccountNumber, calendarYear, sourceHash)
		if err != nil {
			return models.YearEndReturnDetail{}, fmt.Errorf("creating return header: %w", err)
		}
		returnID = r.ID
	} else {
		// Delete existing slips + summary, then regenerate
		if err := s.deleteReturnDataLocked(existingID); err != nil {
			return models.YearEndReturnDetail{}, fmt.Errorf("clearing existing data: %w", err)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, _ = s.db.Exec(`
			UPDATE payroll_year_end_returns
			SET source_hash = ?, generated_at = ?, updated_at = ?
			WHERE id = ?`, sourceHash, now, now, existingID)
		returnID = existingID
		action = "regenerated"
	}

	// Build and insert T4 slips
	var summary models.T4Summary
	summary.YearEndReturnID = returnID

	for _, agg := range aggregates {
		emp, ok := employees[agg.EmployeeID]
		if !ok {
			continue // employee not found in company scope; skip
		}

		// Compute insurable/pensionable earnings:
		// Use detailed breakdown if available; otherwise fall back to gross_pay.
		eiInsurable := agg.EIInsurableEarnings
		cppPensionable := agg.CPPPensionableEarnings
		var unsupported []string
		if !agg.HasDetailedEarnings {
			eiInsurable = agg.GrossPay
			cppPensionable = agg.GrossPay
			unsupported = append(unsupported,
				"box24_ei_insurable_earnings:approximated_from_gross_pay",
				"box26_cpp_pensionable_earnings:approximated_from_gross_pay",
			)
		}

		// Phase 1 unsupported boxes — mark explicitly, never silently zero
		unsupported = append(unsupported,
			"box20_rpp_contributions:pending_mapping",
			"box44_union_dues:pending_mapping",
			"box45_dental_benefit_code:pending_mapping",
			"box46_charitable_donations:pending_mapping",
			"box52_pension_adjustment:pending_mapping",
		)

		sl := models.T4Slip{
			YearEndReturnID:             returnID,
			EmployeeID:                  agg.EmployeeID,
			CalendarYear:                calendarYear,
			EmployeeLegalName:           emp.LegalName,
			EmployeeSINMasked:           emp.SINMasked,
			EmployeeAddress:             emp.Address,
			EmployeeProvince:            emp.Province,
			Box14EmploymentIncome:       roundT4(agg.GrossPay),
			Box16CPPEmployee:            roundT4(agg.CPPEmployee),
			Box16ACpp2Employee:          roundT4(agg.CPP2Employee),
			Box18EIEmployee:             roundT4(agg.EIEmployee),
			Box22IncomeTaxDeducted:      roundT4(agg.FederalTax + agg.ProvincialTax),
			Box24EIInsurableEarnings:    roundT4(eiInsurable),
			Box26CPPPensionableEarnings: roundT4(cppPensionable),
			EmployerCPP:                 roundT4(agg.CPPEmployer),
			EmployerCPP2:                roundT4(agg.CPP2Employer),
			EmployerEI:                  roundT4(agg.EIEmployer),
			OtherInfo:                   []models.T4OtherInfo{},
			UnsupportedFields:           unsupported,
		}

		if err := s.insertT4SlipLocked(sl); err != nil {
			return models.YearEndReturnDetail{}, fmt.Errorf("inserting slip for %s: %w", agg.EmployeeID, err)
		}

		// Accumulate summary totals from slips (never independently calculated)
		summary.SlipCount++
		summary.TotalEmploymentIncome += sl.Box14EmploymentIncome
		summary.TotalCPPEmployee += sl.Box16CPPEmployee
		summary.TotalCPP2Employee += sl.Box16ACpp2Employee
		summary.TotalQPPEmployee += sl.Box17QPPEmployee
		summary.TotalEIEmployee += sl.Box18EIEmployee
		summary.TotalIncomeTaxDeducted += sl.Box22IncomeTaxDeducted
		summary.TotalCPPEmployer += sl.EmployerCPP
		summary.TotalCPP2Employer += sl.EmployerCPP2
		summary.TotalEIEmployer += sl.EmployerEI
		summary.TotalPensionAdjustments += sl.Box52PensionAdjustment
	}

	// Round summary totals
	summary.TotalEmploymentIncome = roundT4(summary.TotalEmploymentIncome)
	summary.TotalCPPEmployee = roundT4(summary.TotalCPPEmployee)
	summary.TotalCPP2Employee = roundT4(summary.TotalCPP2Employee)
	summary.TotalQPPEmployee = roundT4(summary.TotalQPPEmployee)
	summary.TotalEIEmployee = roundT4(summary.TotalEIEmployee)
	summary.TotalIncomeTaxDeducted = roundT4(summary.TotalIncomeTaxDeducted)
	summary.TotalCPPEmployer = roundT4(summary.TotalCPPEmployer)
	summary.TotalCPP2Employer = roundT4(summary.TotalCPP2Employer)
	summary.TotalEIEmployer = roundT4(summary.TotalEIEmployer)

	if err := s.upsertT4SummaryLocked(summary); err != nil {
		return models.YearEndReturnDetail{}, fmt.Errorf("saving T4 summary: %w", err)
	}

	if err := s.insertAuditLogLocked(returnID, action, actorUsername, fmt.Sprintf("source_hash=%s slips=%d", sourceHash, summary.SlipCount)); err != nil {
		return models.YearEndReturnDetail{}, err
	}

	ret, _ := s.getYearEndReturnLocked(returnID)
	detail := models.YearEndReturnDetail{
		Return:    ret,
		Summary:   summary,
		Slips:     s.listT4SlipsLocked(returnID),
		AuditLogs: s.listAuditLogsLocked(returnID),
	}
	return detail, nil
}

// roundT4 rounds a dollar amount to 2 decimal places (CRA expects cents).
func roundT4(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// ── Validation ───────────────────────────────────────────────────────────────

// ValidateYearEndReturn checks all conditions required before finalizing.
// Returns a (possibly empty) list of errors; empty list means ready to finalize.
func (s *Store) ValidateYearEndReturn(id string) []models.T4ValidationError {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []models.T4ValidationError

	ret, err := s.getYearEndReturnLocked(id)
	if err != nil {
		return []models.T4ValidationError{{Scope: "return", Code: "not_found", Message: "return not found"}}
	}

	if ret.PayrollAccountNumber == "" {
		errs = append(errs, models.T4ValidationError{
			Scope: "summary", Code: "missing_payroll_account_number",
			Message: "payroll account number (Box 54) is required",
		})
	}

	summary := s.getT4SummaryLocked(id)
	if summary.SlipCount == 0 {
		errs = append(errs, models.T4ValidationError{
			Scope: "summary", Code: "no_slips",
			Message: "no T4 slips found; ensure at least one finalized payroll period exists for this year",
		})
	}

	slips := s.listT4SlipsLocked(id)

	// Verify summary totals match the sum of all slips
	var (
		sumIncome, sumCPP, sumCPP2, sumEI, sumTax float64
		sumErCPP, sumErCPP2, sumErEI              float64
	)
	for _, sl := range slips {
		scope := "slip:" + sl.EmployeeID

		if strings.TrimSpace(sl.EmployeeLegalName) == "" {
			errs = append(errs, models.T4ValidationError{
				Scope: scope, Code: "missing_employee_name",
				Message: fmt.Sprintf("employee %s: legal name is required (T4 Box 14 identification)", sl.EmployeeID),
			})
		}
		if strings.TrimSpace(sl.EmployeeSINMasked) == "" || sl.EmployeeSINMasked == "***-***-***" {
			errs = append(errs, models.T4ValidationError{
				Scope: scope, Code: "missing_sin",
				Message: fmt.Sprintf("employee %s (%s): SIN (Box 12) is required", sl.EmployeeID, sl.EmployeeLegalName),
			})
		}
		if strings.TrimSpace(sl.EmployeeProvince) == "" {
			errs = append(errs, models.T4ValidationError{
				Scope: scope, Code: "missing_province",
				Message: fmt.Sprintf("employee %s (%s): province of employment (Box 10) is required", sl.EmployeeID, sl.EmployeeLegalName),
			})
		}
		if sl.Box14EmploymentIncome < 0 {
			errs = append(errs, models.T4ValidationError{
				Scope: scope, Code: "negative_employment_income",
				Message: fmt.Sprintf("employee %s: Box 14 employment income cannot be negative", sl.EmployeeID),
			})
		}
		if sl.Box22IncomeTaxDeducted < 0 {
			errs = append(errs, models.T4ValidationError{
				Scope: scope, Code: "negative_income_tax",
				Message: fmt.Sprintf("employee %s: Box 22 income tax deducted cannot be negative", sl.EmployeeID),
			})
		}

		sumIncome += sl.Box14EmploymentIncome
		sumCPP += sl.Box16CPPEmployee
		sumCPP2 += sl.Box16ACpp2Employee
		sumEI += sl.Box18EIEmployee
		sumTax += sl.Box22IncomeTaxDeducted
		sumErCPP += sl.EmployerCPP
		sumErCPP2 += sl.EmployerCPP2
		sumErEI += sl.EmployerEI
	}

	// Summary totals must equal sum of slips (within rounding tolerance of $0.01 per slip)
	tolerance := 0.011 * float64(len(slips)+1)
	check := func(label string, sumSlips, summaryVal float64) {
		diff := sumSlips - summaryVal
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			errs = append(errs, models.T4ValidationError{
				Scope: "summary", Code: "totals_mismatch_" + label,
				Message: fmt.Sprintf("summary %s (%.2f) does not match sum of slips (%.2f)", label, summaryVal, sumSlips),
			})
		}
	}
	check("employment_income", roundT4(sumIncome), summary.TotalEmploymentIncome)
	check("cpp_employee", roundT4(sumCPP), summary.TotalCPPEmployee)
	check("cpp2_employee", roundT4(sumCPP2), summary.TotalCPP2Employee)
	check("ei_employee", roundT4(sumEI), summary.TotalEIEmployee)
	check("income_tax", roundT4(sumTax), summary.TotalIncomeTaxDeducted)
	check("cpp_employer", roundT4(sumErCPP), summary.TotalCPPEmployer)
	check("cpp2_employer", roundT4(sumErCPP2), summary.TotalCPP2Employer)
	check("ei_employer", roundT4(sumErEI), summary.TotalEIEmployer)

	if errs == nil {
		return []models.T4ValidationError{}
	}
	return errs
}

// ── Audit logs ───────────────────────────────────────────────────────────────

func (s *Store) insertAuditLogLocked(returnID, action, actorUsername, note string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO payroll_year_end_audit_logs
		  (year_end_return_id, action, actor_username, note, created_at)
		VALUES (?,?,?,?,?)`,
		returnID, action, actorUsername, note, now)
	return err
}

func (s *Store) listAuditLogsLocked(returnID string) []models.YearEndAuditLog {
	rows, err := s.db.Query(`
		SELECT id, year_end_return_id, action, actor_username, note, created_at
		FROM payroll_year_end_audit_logs
		WHERE year_end_return_id = ?
		ORDER BY id ASC`, returnID)
	if err != nil {
		return []models.YearEndAuditLog{}
	}
	defer rows.Close()

	var list []models.YearEndAuditLog
	for rows.Next() {
		var l models.YearEndAuditLog
		if rows.Scan(&l.ID, &l.YearEndReturnID, &l.Action, &l.ActorUsername, &l.Note, &l.CreatedAt) == nil {
			list = append(list, l)
		}
	}
	if list == nil {
		return []models.YearEndAuditLog{}
	}
	return list
}

// UpdateT4SummaryContact sets the contact name and phone on a draft summary.
func (s *Store) UpdateT4SummaryContact(returnID, contactName, contactPhone string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var status string
	if err := s.db.QueryRow(`SELECT status FROM payroll_year_end_returns WHERE id = ?`, returnID).Scan(&status); err != nil {
		return ErrNotFound
	}
	if status == string(models.YearEndStatusFinalized) {
		return fmt.Errorf("return is finalized; contact info cannot be changed")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE payroll_t4_summary
		SET contact_name = ?, contact_phone = ?, updated_at = ?
		WHERE year_end_return_id = ?`, contactName, contactPhone, now, returnID)
	return err
}

// HasFinalizedPeriodsForYear returns true if at least one finalized period
// with pay_date in the given year exists for the company.
func (s *Store) HasFinalizedPeriodsForYear(companyID string, calendarYear int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int
	_ = s.db.QueryRow(`
		SELECT COUNT(*) FROM payroll_periods
		WHERE company_id = ?
		  AND status = 'finalized'
		  AND substr(pay_date, 1, 4) = ?`,
		companyID, strconv.Itoa(calendarYear)).Scan(&count)
	return count > 0
}

// ── ensure* helpers (called from open.go) ────────────────────────────────────

func ensurePayrollYearEndReturnsTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_year_end_returns (
  id TEXT PRIMARY KEY,
  company_id TEXT NOT NULL DEFAULT '',
  payroll_account_number TEXT NOT NULL DEFAULT '',
  calendar_year INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'draft',
  generated_at TEXT NOT NULL DEFAULT '',
  finalized_at TEXT NOT NULL DEFAULT '',
  source_hash TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  UNIQUE(company_id, payroll_account_number, calendar_year)
);
CREATE INDEX IF NOT EXISTS idx_year_end_returns_company ON payroll_year_end_returns (company_id);
`)
	return err
}

func ensurePayrollT4SlipsTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_t4_slips (
  id TEXT PRIMARY KEY,
  year_end_return_id TEXT NOT NULL DEFAULT '',
  employee_id TEXT NOT NULL DEFAULT '',
  calendar_year INTEGER NOT NULL DEFAULT 0,
  employee_legal_name TEXT NOT NULL DEFAULT '',
  employee_sin_masked TEXT NOT NULL DEFAULT '',
  employee_address TEXT NOT NULL DEFAULT '',
  employee_province TEXT NOT NULL DEFAULT '',
  box_14 REAL NOT NULL DEFAULT 0,
  box_16 REAL NOT NULL DEFAULT 0,
  box_16a REAL NOT NULL DEFAULT 0,
  box_17 REAL NOT NULL DEFAULT 0,
  box_18 REAL NOT NULL DEFAULT 0,
  box_20 REAL NOT NULL DEFAULT 0,
  box_22 REAL NOT NULL DEFAULT 0,
  box_24 REAL NOT NULL DEFAULT 0,
  box_26 REAL NOT NULL DEFAULT 0,
  box_44 REAL NOT NULL DEFAULT 0,
  box_45 INTEGER NOT NULL DEFAULT 0,
  box_46 REAL NOT NULL DEFAULT 0,
  box_52 REAL NOT NULL DEFAULT 0,
  employer_cpp REAL NOT NULL DEFAULT 0,
  employer_cpp2 REAL NOT NULL DEFAULT 0,
  employer_ei REAL NOT NULL DEFAULT 0,
  other_info_json TEXT NOT NULL DEFAULT '[]',
  unsupported_fields_json TEXT NOT NULL DEFAULT '[]',
  version_no INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_t4_slips_return ON payroll_t4_slips (year_end_return_id);
CREATE INDEX IF NOT EXISTS idx_t4_slips_employee ON payroll_t4_slips (employee_id);
`)
	return err
}

func ensurePayrollT4SummaryTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_t4_summary (
  id TEXT PRIMARY KEY,
  year_end_return_id TEXT NOT NULL UNIQUE DEFAULT '',
  slip_count INTEGER NOT NULL DEFAULT 0,
  total_employment_income REAL NOT NULL DEFAULT 0,
  total_cpp_employee REAL NOT NULL DEFAULT 0,
  total_cpp2_employee REAL NOT NULL DEFAULT 0,
  total_qpp_employee REAL NOT NULL DEFAULT 0,
  total_ei_employee REAL NOT NULL DEFAULT 0,
  total_income_tax_deducted REAL NOT NULL DEFAULT 0,
  total_cpp_employer REAL NOT NULL DEFAULT 0,
  total_cpp2_employer REAL NOT NULL DEFAULT 0,
  total_ei_employer REAL NOT NULL DEFAULT 0,
  total_pension_adjustments REAL NOT NULL DEFAULT 0,
  contact_name TEXT NOT NULL DEFAULT '',
  contact_phone TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
`)
	return err
}

func ensurePayrollYearEndAuditLogsTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS payroll_year_end_audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  year_end_return_id TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL DEFAULT '',
  actor_username TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_year_end_audit_return ON payroll_year_end_audit_logs (year_end_return_id);
`)
	return err
}
