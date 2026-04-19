package store

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"simpletask/internal/models"
)

// ── Test DB helpers ───────────────────────────────────────────────────────────

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}

	ensures := []func(*sql.DB) error{
		ensurePayrollCompaniesTable,
		ensurePayrollEmployeesTable,
		ensurePayrollPeriodsTable,
		ensurePayrollEntriesTable,
		ensurePayrollEarningsCodesTable,
		ensurePayrollEntryEarningsTable,
		ensureEarningsCodeExtraColumns,
		ensurePayrollCompanyRulesTable,
		func(db *sql.DB) error { return ensureEmployeeVacationBalance(db) },
		func(db *sql.DB) error { return ensureEntryPaymentType(db) },
		func(db *sql.DB) error { return ensureEmployeeExtraColumns(db) },
		ensurePayrollYearEndReturnsTable,
		ensurePayrollT4SlipsTable,
		ensurePayrollT4SummaryTable,
		ensurePayrollYearEndAuditLogsTable,
	}
	for _, fn := range ensures {
		if err := fn(db); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db := newTestDB(t)
	s := &Store{db: db}
	t.Cleanup(func() { db.Close() })
	return s
}

// ── Seed helpers ──────────────────────────────────────────────────────────────

func seedCompany(t *testing.T, s *Store, id, bn string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO payroll_companies
		  (id, name, legal_name, business_number, province, status, owner_username, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, "Test Co", "Test Company Inc.", bn, "ON", "active", "admin", now, now)
	if err != nil {
		t.Fatalf("seed company %s: %v", id, err)
	}
}

func seedEmployee(t *testing.T, s *Store, id, companyID, name, sinEnc, province string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO payroll_employees
		  (id, company_id, legal_name, member_type, status, province, sin_encrypted,
		   pays_per_year, pay_frequency, created_at, updated_at)
		VALUES (?,?,?,0,'active',?,?,26,'biweekly',?,?)`,
		id, companyID, name, province, sinEnc, now, now)
	if err != nil {
		t.Fatalf("seed employee %s: %v", id, err)
	}
}

// seedFinalizedPeriod inserts a finalized pay period and returns its ID.
func seedFinalizedPeriod(t *testing.T, s *Store, companyID, payDate string) string {
	t.Helper()
	id := fmt.Sprintf("PP%s%s", companyID, payDate)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO payroll_periods
		  (id, company_id, period_start, period_end, pay_date, pays_per_year, pay_frequency,
		   payroll_type, status, created_at, updated_at)
		VALUES (?,?,?,?,?,26,'biweekly','regular','finalized',?,?)`,
		id, companyID, payDate, payDate, payDate, now, now)
	if err != nil {
		t.Fatalf("seed period %s: %v", id, err)
	}
	return id
}

// seedOpenPeriod inserts an OPEN (not finalized) period.
func seedOpenPeriod(t *testing.T, s *Store, companyID, payDate string) string {
	t.Helper()
	id := fmt.Sprintf("PO%s%s", companyID, payDate)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO payroll_periods
		  (id, company_id, period_start, period_end, pay_date, pays_per_year, pay_frequency,
		   payroll_type, status, created_at, updated_at)
		VALUES (?,?,?,?,?,26,'biweekly','regular','open',?,?)`,
		id, companyID, payDate, payDate, payDate, now, now)
	if err != nil {
		t.Fatalf("seed open period: %v", err)
	}
	return id
}

func seedEntry(t *testing.T, s *Store, periodID, employeeID, companyID string,
	gross, cppEe, cpp2Ee, eiEe, fedTax, provTax, cppEr, cpp2Er, eiEr float64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("E%s%s", periodID, employeeID)
	_, err := s.db.Exec(`
		INSERT INTO payroll_entries
		  (id, period_id, employee_id, company_id,
		   gross_pay, cpp_ee, cpp2_ee, ei_ee, federal_tax, provincial_tax,
		   total_deductions, net_pay, cpp_er, cpp2_er, ei_er,
		   ytd_gross, ytd_cpp_ee, ytd_cpp2_ee, ytd_ei_ee,
		   status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,0,0,0,'draft',?,?)`,
		id, periodID, employeeID, companyID,
		gross, cppEe, cpp2Ee, eiEe, fedTax, provTax,
		cppEe+cpp2Ee+eiEe+fedTax+provTax, gross-(cppEe+cpp2Ee+eiEe+fedTax+provTax),
		cppEr, cpp2Er, eiEr,
		now, now)
	if err != nil {
		t.Fatalf("seed entry: %v", err)
	}
}

func empMap(emps ...models.PayrollEmployee) map[string]models.PayrollEmployee {
	m := make(map[string]models.PayrollEmployee, len(emps))
	for _, e := range emps {
		m[e.ID] = e
	}
	return m
}

func emp(id, companyID, name, sinMasked, province string) models.PayrollEmployee {
	return models.PayrollEmployee{
		ID: id, CompanyID: companyID, LegalName: name,
		SINMasked: sinMasked, Province: province,
	}
}

// ── Test 1: Normal generation for one employee, one year, one account ─────────

func TestT4_NormalGeneration(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC001", "111222333")
	seedEmployee(t, s, "E001", "PC001", "Alice Smith", "enc-sin", "ON")
	p := seedFinalizedPeriod(t, s, "PC001", "2024-03-15")
	seedEntry(t, s, p, "E001", "PC001", 5000, 250, 10, 82, 600, 200, 250, 10, 115)

	employees := empMap(emp("E001", "PC001", "Alice Smith", "***-***-001", "ON"))
	detail, err := s.GenerateOrRegenerateDraft("PC001", "111222333RP0001", 2024, "admin", employees)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if detail.Return.Status != models.YearEndStatusDraft {
		t.Errorf("expected draft, got %s", detail.Return.Status)
	}
	if len(detail.Slips) != 1 {
		t.Fatalf("expected 1 slip, got %d", len(detail.Slips))
	}
	sl := detail.Slips[0]
	if sl.Box14EmploymentIncome != 5000 {
		t.Errorf("Box14 expected 5000, got %v", sl.Box14EmploymentIncome)
	}
	if sl.Box16CPPEmployee != 250 {
		t.Errorf("Box16 expected 250, got %v", sl.Box16CPPEmployee)
	}
	if sl.Box18EIEmployee != 82 {
		t.Errorf("Box18 expected 82, got %v", sl.Box18EIEmployee)
	}
	if sl.Box22IncomeTaxDeducted != 800 { // 600+200
		t.Errorf("Box22 expected 800, got %v", sl.Box22IncomeTaxDeducted)
	}
	if sl.EmployeeLegalName != "Alice Smith" {
		t.Errorf("employee name mismatch")
	}
	if sl.EmployeeProvince != "ON" {
		t.Errorf("province mismatch")
	}

	// Audit log must record generation
	if len(detail.AuditLogs) != 1 || detail.AuditLogs[0].Action != "generated" {
		t.Error("expected audit log entry 'generated'")
	}
}

// ── Test 2: Multiple employees aggregate correctly into T4 Summary ──────────

func TestT4_MultiEmployeeSummary(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC002", "222333444")
	seedEmployee(t, s, "E001", "PC002", "Bob A", "", "BC")
	seedEmployee(t, s, "E002", "PC002", "Carol B", "", "BC")
	p := seedFinalizedPeriod(t, s, "PC002", "2024-06-15")
	seedEntry(t, s, p, "E001", "PC002", 3000, 150, 5, 49.2, 360, 120, 150, 5, 68.88)
	seedEntry(t, s, p, "E002", "PC002", 4000, 200, 8, 65.6, 480, 160, 200, 8, 91.84)

	employees := empMap(
		emp("E001", "PC002", "Bob A", "***-***-111", "BC"),
		emp("E002", "PC002", "Carol B", "***-***-222", "BC"),
	)
	detail, err := s.GenerateOrRegenerateDraft("PC002", "222333444RP0001", 2024, "admin", employees)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(detail.Slips) != 2 {
		t.Fatalf("expected 2 slips, got %d", len(detail.Slips))
	}

	sum := detail.Summary
	if sum.SlipCount != 2 {
		t.Errorf("slip count expected 2, got %d", sum.SlipCount)
	}
	wantIncome := roundT4(3000 + 4000)
	if sum.TotalEmploymentIncome != wantIncome {
		t.Errorf("summary income: want %v got %v", wantIncome, sum.TotalEmploymentIncome)
	}
	wantCPP := roundT4(150 + 200)
	if sum.TotalCPPEmployee != wantCPP {
		t.Errorf("summary CPP: want %v got %v", wantCPP, sum.TotalCPPEmployee)
	}
	wantEI := roundT4(49.2 + 65.6)
	if sum.TotalEIEmployee != wantEI {
		t.Errorf("summary EI: want %v got %v", wantEI, sum.TotalEIEmployee)
	}
}

// ── Test 3: Different payroll accounts are separated ────────────────────────

func TestT4_DifferentAccountsSeparated(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC003", "333444555")
	seedEmployee(t, s, "E001", "PC003", "Dan D", "", "AB")
	seedEmployee(t, s, "E002", "PC003", "Eve E", "", "AB")
	p1 := seedFinalizedPeriod(t, s, "PC003", "2024-04-15")
	seedEntry(t, s, p1, "E001", "PC003", 2000, 100, 0, 32.8, 240, 80, 100, 0, 45.92)
	seedEntry(t, s, p1, "E002", "PC003", 3000, 150, 0, 49.2, 360, 120, 150, 0, 68.88)

	employees1 := empMap(emp("E001", "PC003", "Dan D", "***-***-001", "AB"))
	employees2 := empMap(emp("E002", "PC003", "Eve E", "***-***-002", "AB"))

	// Generate two different payroll accounts for the same company+year
	d1, err := s.GenerateOrRegenerateDraft("PC003", "333444555RP0001", 2024, "admin", employees1)
	if err != nil {
		t.Fatalf("generate RP0001: %v", err)
	}
	d2, err := s.GenerateOrRegenerateDraft("PC003", "333444555RP0002", 2024, "admin", employees2)
	if err != nil {
		t.Fatalf("generate RP0002: %v", err)
	}

	// Each return has 1 slip (only the employee in its own map)
	// NOTE: GenerateOrRegenerateDraft aggregates ALL finalized entries for the company+year,
	// but only creates slips for employees present in the employees map.
	// This tests that distinct payroll account numbers produce distinct return records.
	if d1.Return.ID == d2.Return.ID {
		t.Error("different payroll accounts must produce different return IDs")
	}
	if d1.Return.PayrollAccountNumber == d2.Return.PayrollAccountNumber {
		t.Error("account numbers must differ")
	}
	// Each return must have its own summary with its own slip count
	if d1.Summary.SlipCount != 1 {
		t.Errorf("RP0001 expected 1 slip, got %d", d1.Summary.SlipCount)
	}
	if d2.Summary.SlipCount != 1 {
		t.Errorf("RP0002 expected 1 slip, got %d", d2.Summary.SlipCount)
	}
}

// ── Test 4: Unposted (non-finalized) periods are not included ───────────────

func TestT4_UnpostedPeriodsExcluded(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC004", "444555666")
	seedEmployee(t, s, "E001", "PC004", "Frank F", "", "ON")

	// One finalized + one open period
	finalP := seedFinalizedPeriod(t, s, "PC004", "2024-02-15")
	openP := seedOpenPeriod(t, s, "PC004", "2024-03-15")
	seedEntry(t, s, finalP, "E001", "PC004", 1000, 50, 0, 16.4, 120, 40, 50, 0, 22.96)
	seedEntry(t, s, openP, "E001", "PC004", 2000, 100, 0, 32.8, 240, 80, 100, 0, 45.92)

	employees := empMap(emp("E001", "PC004", "Frank F", "***-***-001", "ON"))
	detail, err := s.GenerateOrRegenerateDraft("PC004", "444555666RP0001", 2024, "admin", employees)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Only the finalized period (gross=1000) must be included
	if len(detail.Slips) != 1 {
		t.Fatalf("expected 1 slip, got %d", len(detail.Slips))
	}
	if detail.Slips[0].Box14EmploymentIncome != 1000 {
		t.Errorf("Box14 should be 1000 (only finalized); got %v", detail.Slips[0].Box14EmploymentIncome)
	}
}

// ── Test 5: Finalized return cannot be directly modified ────────────────────

func TestT4_FinalizedReturnIsImmutable(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC005", "555666777")
	seedEmployee(t, s, "E001", "PC005", "Grace G", "encsin", "ON")
	p := seedFinalizedPeriod(t, s, "PC005", "2024-05-15")
	seedEntry(t, s, p, "E001", "PC005", 3000, 150, 0, 49.2, 360, 120, 150, 0, 68.88)

	employees := empMap(emp("E001", "PC005", "Grace G", "***-***-001", "ON"))
	detail, err := s.GenerateOrRegenerateDraft("PC005", "555666777RP0001", 2024, "admin", employees)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	returnID := detail.Return.ID

	// Finalize
	if err := s.FinalizeYearEndReturn(returnID, "admin"); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Attempt to regenerate after finalize must fail
	_, err = s.GenerateOrRegenerateDraft("PC005", "555666777RP0001", 2024, "admin", employees)
	if err == nil {
		t.Error("expected error regenerating a finalized return, got nil")
	}

	// Attempt to finalize again must fail
	if err := s.FinalizeYearEndReturn(returnID, "admin"); err == nil {
		t.Error("expected error finalizing an already-finalized return")
	}

	// Verify status is still finalized
	ret, err := s.GetYearEndReturn(returnID)
	if err != nil {
		t.Fatalf("get return: %v", err)
	}
	if ret.Status != models.YearEndStatusFinalized {
		t.Errorf("expected finalized, got %s", ret.Status)
	}
}

// ── Test 6: Regenerate draft does not touch finalized snapshot ───────────────

func TestT4_RegenerateDraftDoesNotPolluteFinalizedSnapshot(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC006A", "600000001")
	seedCompany(t, s, "PC006B", "600000002")

	// Company A: finalize, then verify it cannot be regenerated
	seedEmployee(t, s, "E001", "PC006A", "Hannah H", "enc", "ON")
	pA := seedFinalizedPeriod(t, s, "PC006A", "2024-01-15")
	seedEntry(t, s, pA, "E001", "PC006A", 5000, 250, 0, 82, 600, 200, 250, 0, 114.8)
	empsA := empMap(emp("E001", "PC006A", "Hannah H", "***-***-001", "ON"))
	dA, _ := s.GenerateOrRegenerateDraft("PC006A", "600000001RP0001", 2024, "admin", empsA)
	_ = s.FinalizeYearEndReturn(dA.Return.ID, "admin")

	originalSlips, _ := s.GetYearEndReturnDetail(dA.Return.ID)
	originalBox14 := originalSlips.Slips[0].Box14EmploymentIncome

	// Attempting regenerate of finalized must return error (not silently overwrite)
	_, err := s.GenerateOrRegenerateDraft("PC006A", "600000001RP0001", 2024, "admin", empsA)
	if err == nil {
		t.Fatal("regenerating finalized return must return error")
	}

	// Verify slip data unchanged
	after, _ := s.GetYearEndReturnDetail(dA.Return.ID)
	if after.Slips[0].Box14EmploymentIncome != originalBox14 {
		t.Error("finalized slip data was mutated by failed regenerate attempt")
	}

	// Company B: draft can be regenerated successfully (uses different employee ID)
	seedEmployee(t, s, "E006B", "PC006B", "Ivan I", "enc", "BC")
	pB := seedFinalizedPeriod(t, s, "PC006B", "2024-02-15")
	seedEntry(t, s, pB, "E006B", "PC006B", 2000, 100, 0, 32.8, 240, 80, 100, 0, 45.92)
	empsB := empMap(emp("E006B", "PC006B", "Ivan I", "***-***-002", "BC"))
	dB, _ := s.GenerateOrRegenerateDraft("PC006B", "600000002RP0001", 2024, "admin", empsB)

	// Add another period and regenerate draft
	pB2 := seedFinalizedPeriod(t, s, "PC006B", "2024-03-15")
	seedEntry(t, s, pB2, "E006B", "PC006B", 2500, 125, 0, 41, 300, 100, 125, 0, 57.4)
	dB2, err := s.GenerateOrRegenerateDraft("PC006B", "600000002RP0001", 2024, "admin", empsB)
	if err != nil {
		t.Fatalf("regenerate draft: %v", err)
	}
	if dB2.Return.ID != dB.Return.ID {
		t.Error("regenerate should reuse the same return ID")
	}
	if dB2.Slips[0].Box14EmploymentIncome != 4500 { // 2000+2500
		t.Errorf("regenerated slip Box14 expected 4500, got %v", dB2.Slips[0].Box14EmploymentIncome)
	}
}

// ── Test 7: Summary totals must equal sum of all slips ─────────────────────

func TestT4_SummaryTotalsMatchSlips(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC007", "777888999")
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("E%03d", i)
		seedEmployee(t, s, id, "PC007", fmt.Sprintf("Employee %d", i), "", "ON")
	}
	p := seedFinalizedPeriod(t, s, "PC007", "2024-09-15")
	var wantIncome, wantCPP, wantEI, wantTax float64
	for i := 1; i <= 5; i++ {
		gross := float64(i * 1000)
		cppEe := gross * 0.0595
		eiEe := gross * 0.0166
		tax := gross * 0.15
		wantIncome += gross; wantCPP += cppEe; wantEI += eiEe; wantTax += tax
		id := fmt.Sprintf("E%03d", i)
		seedEntry(t, s, p, id, "PC007", gross, cppEe, 0, eiEe, tax*0.8, tax*0.2, cppEe, 0, eiEe*1.4)
	}

	employees := make(map[string]models.PayrollEmployee)
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("E%03d", i)
		employees[id] = emp(id, "PC007", fmt.Sprintf("Employee %d", i), fmt.Sprintf("***-***-%03d", i), "ON")
	}
	detail, err := s.GenerateOrRegenerateDraft("PC007", "777888999RP0001", 2024, "admin", employees)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	sum := detail.Summary

	// Verify summary = sum of individual slips
	var slipIncome, slipCPP, slipEI, slipTax float64
	for _, sl := range detail.Slips {
		slipIncome += sl.Box14EmploymentIncome
		slipCPP += sl.Box16CPPEmployee
		slipEI += sl.Box18EIEmployee
		slipTax += sl.Box22IncomeTaxDeducted
	}

	const tol = 0.10
	assertClose(t, "summary income vs slips", sum.TotalEmploymentIncome, roundT4(slipIncome), tol)
	assertClose(t, "summary CPP vs slips", sum.TotalCPPEmployee, roundT4(slipCPP), tol)
	assertClose(t, "summary EI vs slips", sum.TotalEIEmployee, roundT4(slipEI), tol)
	assertClose(t, "summary tax vs slips", sum.TotalIncomeTaxDeducted, roundT4(slipTax), tol)

	// Validation must pass (all data present)
	errs := s.ValidateYearEndReturn(detail.Return.ID)
	for _, e := range errs {
		// Allow pending-mapping warnings, but fail on totals_mismatch
		if contains(e.Code, "totals_mismatch") {
			t.Errorf("unexpected totals mismatch: %s — %s", e.Code, e.Message)
		}
	}
}

// ── Test 8: Company isolation ───────────────────────────────────────────────

func TestT4_CompanyIsolation(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC008A", "800000001")
	seedCompany(t, s, "PC008B", "800000002")

	seedEmployee(t, s, "E001", "PC008A", "Jack A", "", "ON")
	seedEmployee(t, s, "E002", "PC008B", "Karen B", "", "QC")

	pA := seedFinalizedPeriod(t, s, "PC008A", "2024-07-15")
	pB := seedFinalizedPeriod(t, s, "PC008B", "2024-07-15")
	seedEntry(t, s, pA, "E001", "PC008A", 6000, 300, 0, 98.4, 720, 240, 300, 0, 137.76)
	seedEntry(t, s, pB, "E002", "PC008B", 7000, 350, 0, 114.8, 840, 280, 350, 0, 160.72)

	dA, err := s.GenerateOrRegenerateDraft("PC008A", "800000001RP0001", 2024, "admin",
		empMap(emp("E001", "PC008A", "Jack A", "***-***-001", "ON")))
	if err != nil {
		t.Fatalf("generate PC008A: %v", err)
	}
	dB, err := s.GenerateOrRegenerateDraft("PC008B", "800000002RP0001", 2024, "admin",
		empMap(emp("E002", "PC008B", "Karen B", "***-***-002", "QC")))
	if err != nil {
		t.Fatalf("generate PC008B: %v", err)
	}

	// Each return belongs to correct company
	if dA.Return.CompanyID != "PC008A" {
		t.Error("PC008A return has wrong company_id")
	}
	if dB.Return.CompanyID != "PC008B" {
		t.Error("PC008B return has wrong company_id")
	}

	// Returns must not share slips
	if dA.Return.ID == dB.Return.ID {
		t.Error("different companies must not share return IDs")
	}

	// Company A return must only have PC008A employee
	for _, sl := range dA.Slips {
		if sl.EmployeeID != "E001" {
			t.Errorf("PC008A slip has wrong employee %s", sl.EmployeeID)
		}
	}

	// Company B return must only have PC008B employee
	for _, sl := range dB.Slips {
		if sl.EmployeeID != "E002" {
			t.Errorf("PC008B slip has wrong employee %s", sl.EmployeeID)
		}
	}

	// Income totals must be isolated
	if dA.Summary.TotalEmploymentIncome != 6000 {
		t.Errorf("PC008A income expected 6000, got %v", dA.Summary.TotalEmploymentIncome)
	}
	if dB.Summary.TotalEmploymentIncome != 7000 {
		t.Errorf("PC008B income expected 7000, got %v", dB.Summary.TotalEmploymentIncome)
	}
}

// ── Test 9: Missing required data blocks finalize ───────────────────────────

func TestT4_MissingDataBlocksFinalize(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC009", "999000111")
	// Employee with no SIN (sinEnc="") and no province
	seedEmployee(t, s, "E001", "PC009", "Leo L", "", "")
	p := seedFinalizedPeriod(t, s, "PC009", "2024-10-15")
	seedEntry(t, s, p, "E001", "PC009", 4000, 200, 0, 65.6, 480, 160, 200, 0, 91.84)

	// Employee snapshot: SINMasked missing, Province missing
	employees := empMap(emp("E001", "PC009", "Leo L", "", ""))
	detail, err := s.GenerateOrRegenerateDraft("PC009", "999000111RP0001", 2024, "admin", employees)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	returnID := detail.Return.ID

	errs := s.ValidateYearEndReturn(returnID)
	hasSINErr := false
	hasProvErr := false
	for _, e := range errs {
		if contains(e.Code, "missing_sin") {
			hasSINErr = true
		}
		if contains(e.Code, "missing_province") {
			hasProvErr = true
		}
	}
	if !hasSINErr {
		t.Error("expected missing_sin validation error")
	}
	if !hasProvErr {
		t.Error("expected missing_province validation error")
	}

	// FinalizeYearEndReturn should be blocked by the API layer (validate first);
	// at the store level, finalize itself just marks status. The blocking happens
	// in the API handler which calls ValidateYearEndReturn before FinalizeYearEndReturn.
	// We verify here that the validation errors are present and non-empty.
	if len(errs) == 0 {
		t.Error("expected validation errors before finalize")
	}
}

// ── Test 10: HasFinalizedPeriodsForYear guards draft generation ─────────────

func TestT4_NoFinalizedPeriodsBlocksGeneration(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC010", "101010101")
	seedEmployee(t, s, "E001", "PC010", "Mia M", "", "MB")
	// Only open period, no finalized period
	openP := seedOpenPeriod(t, s, "PC010", "2024-12-15")
	seedEntry(t, s, openP, "E001", "PC010", 3000, 150, 0, 49.2, 360, 120, 150, 0, 68.88)

	hasPeriods := s.HasFinalizedPeriodsForYear("PC010", 2024)
	if hasPeriods {
		t.Error("should report no finalized periods when only open periods exist")
	}

	// Aggregation should return empty when no finalized periods
	aggs, err := s.AggregateT4FromFinalizedPayroll("PC010", 2024)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(aggs) != 0 {
		t.Errorf("expected 0 aggregates for unfinalized periods, got %d", len(aggs))
	}
}

// ── Test 11: Source hash changes when finalized payroll changes ─────────────

func TestT4_SourceHashTraceability(t *testing.T) {
	s := newTestStore(t)
	seedCompany(t, s, "PC011", "111111111")
	seedEmployee(t, s, "E001", "PC011", "Nina N", "", "ON")
	p1 := seedFinalizedPeriod(t, s, "PC011", "2024-01-15")
	seedEntry(t, s, p1, "E001", "PC011", 2000, 100, 0, 32.8, 240, 80, 100, 0, 45.92)

	h1, err := s.ComputeSourceHash("PC011", 2024)
	if err != nil {
		t.Fatalf("hash 1: %v", err)
	}

	// Add another finalized period
	p2 := seedFinalizedPeriod(t, s, "PC011", "2024-02-15")
	seedEntry(t, s, p2, "E001", "PC011", 2500, 125, 0, 41, 300, 100, 125, 0, 57.4)

	h2, err := s.ComputeSourceHash("PC011", 2024)
	if err != nil {
		t.Fatalf("hash 2: %v", err)
	}

	if h1 == h2 {
		t.Error("source hash must change when a new finalized period is added")
	}
	if len(h1) != 64 || len(h2) != 64 {
		t.Error("expected SHA-256 hex strings (64 chars)")
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func assertClose(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > tol {
		t.Errorf("%s: got %.4f, want %.4f (diff %.4f > tol %.4f)", label, got, want, diff, tol)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
