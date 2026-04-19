package store

import (
	"fmt"
	"testing"
	"time"

	"simpletask/internal/models"
)

// ── Test DB helper with ROE tables ────────────────────────────────────────────

func newROETestStore(t *testing.T) *Store {
	t.Helper()
	db := newTestDB(t) // reuses newTestDB from payroll_year_end_test.go
	if err := ensureEmployeeROEColumns(db); err != nil {
		t.Fatalf("roe schema (employee cols): %v", err)
	}
	if err := ensurePayrollROETable(db); err != nil {
		t.Fatalf("roe schema (roe table): %v", err)
	}
	if err := ensurePayrollROEAuditLogsTable(db); err != nil {
		t.Fatalf("roe schema (audit table): %v", err)
	}
	s := &Store{db: db}
	t.Cleanup(func() { db.Close() })
	return s
}

// seedEmployeeROE is like seedEmployee but with hire_date, pay_frequency, and position.
func seedEmployeeROE(t *testing.T, s *Store, id, companyID, name, sinEnc, province, hireDate, payFreq, position string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO payroll_employees
		  (id, company_id, legal_name, member_type, status, province, sin_encrypted,
		   pays_per_year, pay_frequency, hire_date, position, created_at, updated_at)
		VALUES (?,?,?,0,'active',?,?,26,?,?,?,?,?)`,
		id, companyID, name, province, sinEnc, payFreq, hireDate, position, now, now)
	if err != nil {
		t.Fatalf("seed employee ROE %s: %v", id, err)
	}
}

// seedFinalizedPeriodWithEnd inserts a finalized period with explicit period_end.
func seedFinalizedPeriodWithEnd(t *testing.T, s *Store, companyID, periodEnd, payDate string) string {
	t.Helper()
	id := fmt.Sprintf("PP%s%s", companyID, periodEnd)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO payroll_periods
		  (id, company_id, period_start, period_end, pay_date,
		   pays_per_year, pay_frequency, payroll_type, status, created_at, updated_at)
		VALUES (?,?,?,?,?,26,'biweekly','regular','finalized',?,?)`,
		id, companyID, periodEnd, periodEnd, payDate, now, now)
	if err != nil {
		t.Fatalf("seed period (end=%s): %v", periodEnd, err)
	}
	return id
}

// seedEntryHours seeds an entry with hours field.
func seedEntryHours(t *testing.T, s *Store, periodID, employeeID, companyID string, gross, hours float64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("EN%s%s", periodID, employeeID)
	_, err := s.db.Exec(`
		INSERT INTO payroll_entries
		  (id, period_id, employee_id, company_id, hours, gross_pay,
		   cpp_ee, cpp2_ee, ei_ee, federal_tax, provincial_tax,
		   total_deductions, net_pay, cpp_er, cpp2_er, ei_er,
		   ytd_gross, ytd_cpp_ee, ytd_cpp2_ee, ytd_ei_ee,
		   status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,0,0,0,0,0,0,?,0,0,0,0,0,0,0,'draft',?,?)`,
		id, periodID, employeeID, companyID, hours, gross, gross, now, now)
	if err != nil {
		t.Fatalf("seed entry hours: %v", err)
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestROE_NormalGeneration verifies a basic draft ROE is created with correct totals.
func TestROE_NormalGeneration(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC101", "123456789")
	seedEmployeeROE(t, s, "EMP101", "PC101", "Alice Smith", "encABC", "ON",
		"2022-01-10", "biweekly", "Software Developer")

	// 3 finalized periods, 2 weeks apart
	for i, d := range []string{"2025-12-13", "2025-11-29", "2025-11-15"} {
		pid := seedFinalizedPeriodWithEnd(t, s, "PC101", d, d)
		seedEntryHours(t, s, pid, "EMP101", "PC101", float64(3000-i*10), 80)
	}

	emp := models.PayrollEmployee{
		ID: "EMP101", CompanyID: "PC101",
		LegalName: "Alice Smith", Province: "ON",
		HireDate: "2022-01-10", PayFrequency: "biweekly",
		Position: "Software Developer",
	}
	detail, err := s.GenerateROEDraft("PC101", "EMP101", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}
	roe := detail.ROE
	if roe.Status != models.ROEStatusDraft {
		t.Errorf("status = %q, want draft", roe.Status)
	}
	if len(roe.InsurableEarningsPeriods) != 3 {
		t.Errorf("periods = %d, want 3", len(roe.InsurableEarningsPeriods))
	}
	if roe.TotalInsurableEarnings <= 0 {
		t.Error("total insurable earnings should be > 0")
	}
	if roe.FinalPayPeriodEnd != "2025-12-13" {
		t.Errorf("finalPayPeriodEnd = %q, want 2025-12-13", roe.FinalPayPeriodEnd)
	}
	if roe.PayFrequency != "biweekly" {
		t.Errorf("payFrequency = %q, want biweekly", roe.PayFrequency)
	}
	if roe.SerialNumber == "" {
		t.Error("serial number should be set")
	}
}

// TestROE_LookbackBiweekly verifies biweekly cap is 27 periods.
func TestROE_LookbackBiweekly(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC102", "111222333")
	seedEmployeeROE(t, s, "EMP102", "PC102", "Bob Jones", "encBOB", "BC",
		"2020-03-01", "biweekly", "Analyst")

	// Seed 30 finalized periods — lookback should return max 27
	base := time.Date(2025, 12, 13, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 30; i++ {
		d := base.AddDate(0, 0, -14*i).Format("2006-01-02")
		pid := seedFinalizedPeriodWithEnd(t, s, "PC102", d, d)
		seedEntryHours(t, s, pid, "EMP102", "PC102", 2000, 80)
	}

	emp := models.PayrollEmployee{
		ID: "EMP102", CompanyID: "PC102", PayFrequency: "biweekly",
		HireDate: "2020-03-01",
	}
	detail, err := s.GenerateROEDraft("PC102", "EMP102", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}
	if len(detail.ROE.InsurableEarningsPeriods) != 27 {
		t.Errorf("biweekly lookback = %d, want 27", len(detail.ROE.InsurableEarningsPeriods))
	}
}

// TestROE_LookbackWeekly verifies weekly cap is 53 periods.
func TestROE_LookbackWeekly(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC103", "222333444")
	seedEmployeeROE(t, s, "EMP103", "PC103", "Carol Lee", "encCAR", "AB",
		"2020-01-01", "weekly", "Cashier")

	base := time.Date(2025, 12, 13, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		d := base.AddDate(0, 0, -7*i).Format("2006-01-02")
		pid := seedFinalizedPeriodWithEnd(t, s, "PC103", d, d)
		seedEntryHours(t, s, pid, "EMP103", "PC103", 800, 40)
	}

	emp := models.PayrollEmployee{
		ID: "EMP103", CompanyID: "PC103", PayFrequency: "weekly",
		HireDate: "2020-01-01",
	}
	detail, err := s.GenerateROEDraft("PC103", "EMP103", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}
	if len(detail.ROE.InsurableEarningsPeriods) != 53 {
		t.Errorf("weekly lookback = %d, want 53", len(detail.ROE.InsurableEarningsPeriods))
	}
}

// TestROE_OpenPeriodsExcluded verifies open (non-finalized) periods are not included.
func TestROE_OpenPeriodsExcluded(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC104", "333444555")
	seedEmployeeROE(t, s, "EMP104", "PC104", "Dave Wu", "encDAV", "ON",
		"2023-06-01", "biweekly", "Driver")

	// 2 finalized + 1 open
	pid1 := seedFinalizedPeriodWithEnd(t, s, "PC104", "2025-12-13", "2025-12-13")
	pid2 := seedFinalizedPeriodWithEnd(t, s, "PC104", "2025-11-29", "2025-11-29")
	openPid := seedOpenPeriod(t, s, "PC104", "2025-12-27")

	seedEntryHours(t, s, pid1, "EMP104", "PC104", 2500, 80)
	seedEntryHours(t, s, pid2, "EMP104", "PC104", 2500, 80)
	seedEntryHours(t, s, openPid, "EMP104", "PC104", 2500, 80)

	emp := models.PayrollEmployee{ID: "EMP104", CompanyID: "PC104", PayFrequency: "biweekly", HireDate: "2023-06-01"}
	detail, err := s.GenerateROEDraft("PC104", "EMP104", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}
	if len(detail.ROE.InsurableEarningsPeriods) != 2 {
		t.Errorf("expected 2 finalized periods, got %d", len(detail.ROE.InsurableEarningsPeriods))
	}
}

// TestROE_IssuedROEIsImmutable verifies issued ROE blocks regeneration.
func TestROE_IssuedROEIsImmutable(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC105", "444555666")
	seedEmployeeROE(t, s, "EMP105", "PC105", "Eve Turner", "encEVE", "MB",
		"2021-01-01", "biweekly", "Nurse")

	pid := seedFinalizedPeriodWithEnd(t, s, "PC105", "2025-11-30", "2025-11-30")
	seedEntryHours(t, s, pid, "EMP105", "PC105", 4000, 80)

	emp := models.PayrollEmployee{ID: "EMP105", CompanyID: "PC105", PayFrequency: "biweekly", HireDate: "2021-01-01"}
	detail, err := s.GenerateROEDraft("PC105", "EMP105", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}

	// Fill required fields and issue
	_ = s.UpdateROEFields(detail.ROE.ID, ROEUpdateInput{
		ReasonCode: "M", FirstDayWorked: "2021-01-01",
		LastDayPaid: "2025-11-30", FinalPayPeriodEnd: "2025-11-30",
		Occupation: "Nurse",
	})
	if err := s.IssueROE(detail.ROE.ID, "admin"); err != nil {
		t.Fatalf("IssueROE: %v", err)
	}

	// Verify status is issued
	d, _ := s.GetROEDetail(detail.ROE.ID)
	if d.ROE.Status != models.ROEStatusIssued {
		t.Errorf("status = %q, want issued", d.ROE.Status)
	}

	// Regenerate should fail
	_, err = s.GenerateROEDraft("PC105", "EMP105", "admin", emp)
	if err == nil {
		t.Error("expected error when regenerating issued ROE, got nil")
	}
}

// TestROE_ValidationBlocksIssue verifies missing fields prevent issuance.
func TestROE_ValidationBlocksIssue(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC106", "555666777")
	seedEmployeeROE(t, s, "EMP106", "PC106", "Frank Hall", "", "SK",
		"2022-05-01", "biweekly", "")

	pid := seedFinalizedPeriodWithEnd(t, s, "PC106", "2025-12-01", "2025-12-01")
	seedEntryHours(t, s, pid, "EMP106", "PC106", 3000, 80)

	emp := models.PayrollEmployee{ID: "EMP106", CompanyID: "PC106", PayFrequency: "biweekly", HireDate: "2022-05-01"}
	detail, err := s.GenerateROEDraft("PC106", "EMP106", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}

	// Don't fill required fields — validate should return errors
	errs := s.ValidateROE(detail.ROE.ID)
	if len(errs) == 0 {
		t.Error("expected validation errors, got none")
	}
	codes := map[string]bool{}
	for _, e := range errs {
		codes[e.Code] = true
	}
	if !codes["missing_reason_code"] {
		t.Error("expected missing_reason_code error")
	}
	if !codes["missing_occupation"] {
		t.Error("expected missing_occupation error")
	}
	if !codes["missing_sin"] {
		t.Error("expected missing_sin error")
	}

	// Issue should fail
	if err := s.IssueROE(detail.ROE.ID, "admin"); err == nil {
		t.Error("expected IssueROE to fail, got nil")
	}
}

// TestROE_CompanyIsolation verifies employees from different companies don't mix.
func TestROE_CompanyIsolation(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC107A", "111111111")
	seedCompany(t, s, "PC107B", "222222222")

	seedEmployeeROE(t, s, "EMP107A", "PC107A", "Grace Kim", "encGRA", "ON",
		"2020-01-01", "biweekly", "Manager")
	seedEmployeeROE(t, s, "EMP107B", "PC107B", "Henry Park", "encHEN", "ON",
		"2020-01-01", "biweekly", "Engineer")

	pidA := seedFinalizedPeriodWithEnd(t, s, "PC107A", "2025-12-01", "2025-12-01")
	pidB := seedFinalizedPeriodWithEnd(t, s, "PC107B", "2025-12-01", "2025-12-01")
	seedEntryHours(t, s, pidA, "EMP107A", "PC107A", 5000, 80)
	seedEntryHours(t, s, pidB, "EMP107B", "PC107B", 5000, 80)

	empA := models.PayrollEmployee{ID: "EMP107A", CompanyID: "PC107A", PayFrequency: "biweekly", HireDate: "2020-01-01"}
	empB := models.PayrollEmployee{ID: "EMP107B", CompanyID: "PC107B", PayFrequency: "biweekly", HireDate: "2020-01-01"}

	_, err := s.GenerateROEDraft("PC107A", "EMP107A", "admin", empA)
	if err != nil {
		t.Fatalf("GenerateROEDraft A: %v", err)
	}
	_, err = s.GenerateROEDraft("PC107B", "EMP107B", "admin", empB)
	if err != nil {
		t.Fatalf("GenerateROEDraft B: %v", err)
	}

	roeA, _ := s.ListROEs("PC107A")
	roeB, _ := s.ListROEs("PC107B")
	if len(roeA) != 1 {
		t.Errorf("PC107A ROE count = %d, want 1", len(roeA))
	}
	if len(roeB) != 1 {
		t.Errorf("PC107B ROE count = %d, want 1", len(roeB))
	}
	if roeA[0].ID == roeB[0].ID {
		t.Error("ROE IDs should be different across companies")
	}
	if roeA[0].EmployeeID != "EMP107A" {
		t.Errorf("Company A ROE has wrong employeeId: %s", roeA[0].EmployeeID)
	}
}

// TestROE_RegenerateDraftPreservesIssued verifies issued ROE is never overwritten.
func TestROE_RegenerateDraftPreservesIssued(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC108", "777888999")
	seedEmployeeROE(t, s, "EMP108", "PC108", "Iris Chen", "encIRI", "QC",
		"2019-07-01", "biweekly", "Accountant")

	pid := seedFinalizedPeriodWithEnd(t, s, "PC108", "2025-09-30", "2025-09-30")
	seedEntryHours(t, s, pid, "EMP108", "PC108", 3500, 80)

	emp := models.PayrollEmployee{ID: "EMP108", CompanyID: "PC108", PayFrequency: "biweekly", HireDate: "2019-07-01"}
	d1, _ := s.GenerateROEDraft("PC108", "EMP108", "admin", emp)

	// Issue it
	_ = s.UpdateROEFields(d1.ROE.ID, ROEUpdateInput{
		ReasonCode: "A", FirstDayWorked: "2019-07-01",
		LastDayPaid: "2025-09-30", FinalPayPeriodEnd: "2025-09-30",
		Occupation: "Accountant",
	})
	_ = s.IssueROE(d1.ROE.ID, "admin")

	// Attempt regenerate should fail
	_, err := s.GenerateROEDraft("PC108", "EMP108", "admin", emp)
	if err == nil {
		t.Error("expected error regenerating an issued ROE")
	}

	// Issued ROE should still exist with original data
	d2, _ := s.GetROEDetail(d1.ROE.ID)
	if d2.ROE.Status != models.ROEStatusIssued {
		t.Errorf("issued ROE status changed to %q", d2.ROE.Status)
	}
	if d2.ROE.TotalInsurableEarnings != d1.ROE.TotalInsurableEarnings {
		t.Error("issued ROE totals were modified")
	}
}

// TestROE_NoFinalizedPeriodsBlocksGeneration verifies open-only periods produce an error.
func TestROE_NoFinalizedPeriodsBlocksGeneration(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC109", "100200300")
	seedEmployeeROE(t, s, "EMP109", "PC109", "Jack Brown", "encJAC", "NS",
		"2024-01-01", "biweekly", "Cook")

	// Only open period
	openPid := seedOpenPeriod(t, s, "PC109", "2025-12-14")
	seedEntryHours(t, s, openPid, "EMP109", "PC109", 1800, 40)

	emp := models.PayrollEmployee{ID: "EMP109", CompanyID: "PC109", PayFrequency: "biweekly", HireDate: "2024-01-01"}
	_, err := s.GenerateROEDraft("PC109", "EMP109", "admin", emp)
	if err == nil {
		t.Error("expected error when no finalized periods exist")
	}
}

// TestROE_SourceHashChangesOnNewPeriod verifies hash is recomputed on regenerate.
func TestROE_SourceHashChangesOnNewPeriod(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC110", "400500600")
	seedEmployeeROE(t, s, "EMP110", "PC110", "Karen Fox", "encKAR", "PE",
		"2021-11-01", "biweekly", "Analyst")

	pid1 := seedFinalizedPeriodWithEnd(t, s, "PC110", "2025-11-29", "2025-11-29")
	seedEntryHours(t, s, pid1, "EMP110", "PC110", 2800, 80)

	emp := models.PayrollEmployee{ID: "EMP110", CompanyID: "PC110", PayFrequency: "biweekly", HireDate: "2021-11-01"}
	d1, err := s.GenerateROEDraft("PC110", "EMP110", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft 1: %v", err)
	}
	hash1 := d1.ROE.SourceHash

	// Add another finalized period
	pid2 := seedFinalizedPeriodWithEnd(t, s, "PC110", "2025-12-13", "2025-12-13")
	seedEntryHours(t, s, pid2, "EMP110", "PC110", 2800, 80)

	d2, err := s.GenerateROEDraft("PC110", "EMP110", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft 2: %v", err)
	}
	hash2 := d2.ROE.SourceHash

	if hash1 == hash2 {
		t.Error("source hash should change when a new finalized period is added")
	}
	if len(d2.ROE.InsurableEarningsPeriods) != 2 {
		t.Errorf("expected 2 periods after regenerate, got %d", len(d2.ROE.InsurableEarningsPeriods))
	}
}

// TestROE_AuditLogRecordsActions verifies audit trail.
func TestROE_AuditLogRecordsActions(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC111", "700800900")
	seedEmployeeROE(t, s, "EMP111", "PC111", "Leo Zhang", "encLEO", "YT",
		"2023-03-01", "biweekly", "Engineer")

	pid := seedFinalizedPeriodWithEnd(t, s, "PC111", "2025-11-15", "2025-11-15")
	seedEntryHours(t, s, pid, "EMP111", "PC111", 3200, 80)

	emp := models.PayrollEmployee{ID: "EMP111", CompanyID: "PC111", PayFrequency: "biweekly", HireDate: "2023-03-01"}
	d1, err := s.GenerateROEDraft("PC111", "EMP111", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}

	// Regenerate
	d2, err := s.GenerateROEDraft("PC111", "EMP111", "admin", emp)
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	_ = d1

	// Check audit log has regenerated entry
	detail, _ := s.GetROEDetail(d2.ROE.ID)
	actions := map[string]bool{}
	for _, l := range detail.AuditLogs {
		actions[l.Action] = true
	}
	if !actions["generated"] && !actions["regenerated"] {
		t.Errorf("audit log missing generated/regenerated entry: %v", detail.AuditLogs)
	}
}

// TestROE_UpdateFieldsAndRetrieve verifies PATCH fields are saved and returned.
func TestROE_UpdateFieldsAndRetrieve(t *testing.T) {
	s := newROETestStore(t)
	seedCompany(t, s, "PC112", "112233445")
	seedEmployeeROE(t, s, "EMP112", "PC112", "Maya Roy", "encMAY", "NB",
		"2022-08-01", "monthly", "Designer")

	pid := seedFinalizedPeriodWithEnd(t, s, "PC112", "2025-11-30", "2025-11-30")
	seedEntryHours(t, s, pid, "EMP112", "PC112", 4500, 160)

	emp := models.PayrollEmployee{ID: "EMP112", CompanyID: "PC112", PayFrequency: "monthly", HireDate: "2022-08-01"}
	detail, err := s.GenerateROEDraft("PC112", "EMP112", "admin", emp)
	if err != nil {
		t.Fatalf("GenerateROEDraft: %v", err)
	}

	update := ROEUpdateInput{
		ReasonCode:          "T",
		FirstDayWorked:      "2022-08-01",
		LastDayPaid:         "2025-11-30",
		FinalPayPeriodEnd:   "2025-11-30",
		Occupation:          "Senior Designer",
		ExpectedRecallDate:  "2026-03-01",
		VacationPay:         750.00,
		VacationPayType:     "separate",
		StatutoryHolidayPay: 200.00,
		Comments:            "Seasonal layoff",
	}
	if err := s.UpdateROEFields(detail.ROE.ID, update); err != nil {
		t.Fatalf("UpdateROEFields: %v", err)
	}

	d2, err := s.GetROEDetail(detail.ROE.ID)
	if err != nil {
		t.Fatalf("GetROEDetail: %v", err)
	}
	if d2.ROE.ReasonCode != "T" {
		t.Errorf("ReasonCode = %q, want T", d2.ROE.ReasonCode)
	}
	if d2.ROE.Occupation != "Senior Designer" {
		t.Errorf("Occupation = %q, want Senior Designer", d2.ROE.Occupation)
	}
	if d2.ROE.VacationPay != 750.00 {
		t.Errorf("VacationPay = %.2f, want 750.00", d2.ROE.VacationPay)
	}
	if d2.ROE.StatutoryHolidayPay != 200.00 {
		t.Errorf("StatutoryHolidayPay = %.2f, want 200.00", d2.ROE.StatutoryHolidayPay)
	}
	if d2.ROE.Comments != "Seasonal layoff" {
		t.Errorf("Comments = %q, want 'Seasonal layoff'", d2.ROE.Comments)
	}
}
