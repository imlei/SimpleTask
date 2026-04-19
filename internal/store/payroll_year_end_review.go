package store

import (
	"fmt"
	"strings"
	"time"

	"simpletask/internal/models"
)

// paysPerYearForFreq returns the standard number of pay periods per year.
func paysPerYearForFreq(freq string) int {
	switch strings.ToLower(freq) {
	case "weekly":
		return 52
	case "biweekly", "bi-weekly":
		return 26
	case "semi-monthly", "semimonthly":
		return 24
	case "monthly":
		return 12
	default:
		return 26
	}
}

// RunYearEndReview generates a comprehensive year-end compliance report for a company.
// It reads from finalized payroll periods only and cross-checks against CRA rate maxima.
func (s *Store) RunYearEndReview(companyID string, calendarYear int) models.YearEndReviewReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	yearStr := fmt.Sprintf("%d", calendarYear)
	now := time.Now().UTC().Format(time.RFC3339)

	report := models.YearEndReviewReport{
		CompanyID:    companyID,
		CalendarYear: calendarYear,
		GeneratedAt:  now,
	}

	// ── Company pay frequency → expected periods ──────────────────────────────
	var companyFreq string
	_ = s.db.QueryRow(`SELECT pay_frequency FROM payroll_companies WHERE id=?`, companyID).Scan(&companyFreq)
	report.ExpectedPeriods = paysPerYearForFreq(companyFreq)

	// ── CRA rate maxima for the year ──────────────────────────────────────────
	rates, _ := s.GetPayrollRateSetting(calendarYear)
	if rates != nil {
		report.CPPMaxEE = rates.CPPMaxEE
		report.CPP2MaxEE = rates.CPP2MaxEE
		report.EIMaxEE = rates.EIMaxEE
	} else {
		def := DefaultRateSetting(calendarYear)
		report.CPPMaxEE = def.CPPMaxEE
		report.CPP2MaxEE = def.CPP2MaxEE
		report.EIMaxEE = def.EIMaxEE
	}

	// ── Period counts ─────────────────────────────────────────────────────────
	_ = s.db.QueryRow(`
		SELECT COUNT(*) FROM payroll_periods
		WHERE company_id=? AND substr(pay_date,1,4)=? AND status='finalized'
	`, companyID, yearStr).Scan(&report.FinalizedPeriods)

	// Open / draft periods in the year
	openRows, err := s.db.Query(`
		SELECT id, period_start, period_end, pay_date, status
		FROM payroll_periods
		WHERE company_id=? AND substr(pay_date,1,4)=? AND status != 'finalized'
		ORDER BY pay_date
	`, companyID, yearStr)
	if err == nil {
		for openRows.Next() {
			var op models.ReviewOpenPeriod
			if openRows.Scan(&op.PeriodID, &op.PeriodStart, &op.PeriodEnd, &op.PayDate, &op.Status) == nil {
				report.OpenPeriodsList = append(report.OpenPeriodsList, op)
				report.OpenPeriods++
			}
		}
		openRows.Close()
	}

	// ── Per-employee YTD aggregates from finalized periods ────────────────────
	empRows, err := s.db.Query(`
		SELECT
			e.employee_id,
			emp.legal_name,
			emp.status,
			emp.province,
			emp.sin_encrypted,
			SUM(e.gross_pay),
			SUM(e.cpp_ee),
			SUM(e.cpp2_ee),
			SUM(e.ei_ee),
			SUM(e.federal_tax),
			SUM(e.provincial_tax),
			SUM(e.cpp_er),
			SUM(e.ei_er),
			COUNT(DISTINCT e.period_id)
		FROM payroll_entries e
		JOIN payroll_periods pp ON pp.id = e.period_id
		JOIN payroll_employees emp ON emp.id = e.employee_id
		WHERE pp.company_id=? AND substr(pp.pay_date,1,4)=? AND pp.status='finalized'
		GROUP BY e.employee_id
		ORDER BY emp.legal_name
	`, companyID, yearStr)
	if err != nil {
		report.Issues = append(report.Issues, models.ReviewIssue{
			Severity: models.ReviewError,
			Code:     "db_error",
			Message:  "Failed to aggregate payroll data: " + err.Error(),
		})
		return report
	}
	defer empRows.Close()

	for empRows.Next() {
		var row models.ReviewEmployeeRow
		var sinEnc string
		if err := empRows.Scan(
			&row.EmployeeID, &row.LegalName, &row.Status, &row.Province, &sinEnc,
			&row.GrossPay, &row.CPPEmployee, &row.CPP2Employee, &row.EIEmployee,
			&row.FederalTax, &row.ProvincialTax, &row.CPPEmployer, &row.EIEmployer,
			&row.PeriodCount,
		); err != nil {
			continue
		}
		row.SINMissing = strings.TrimSpace(sinEnc) == ""
		row.ProvinceMissing = strings.TrimSpace(row.Province) == ""

		report.TotalGrossPay += row.GrossPay
		report.TotalCPPEmployee += row.CPPEmployee
		report.TotalCPP2Employee += row.CPP2Employee
		report.TotalEIEmployee += row.EIEmployee
		report.TotalFederalTax += row.FederalTax
		report.TotalProvincialTax += row.ProvincialTax
		report.TotalCPPEmployer += row.CPPEmployer
		report.TotalEIEmployer += row.EIEmployer

		report.Employees = append(report.Employees, row)
	}
	report.TotalEmployees = len(report.Employees)

	// ── Issue detection ───────────────────────────────────────────────────────
	addIssue := func(sev models.ReviewSeverity, code, msg, empID, empName string) {
		report.Issues = append(report.Issues, models.ReviewIssue{
			Severity:     sev,
			Code:         code,
			Message:      msg,
			EmployeeID:   empID,
			EmployeeName: empName,
		})
	}

	// Error: no finalized periods at all
	if report.FinalizedPeriods == 0 {
		addIssue(models.ReviewError, "no_finalized_periods",
			fmt.Sprintf("No finalized payroll periods found for %d. At least one period must be finalized before generating T4s.", calendarYear),
			"", "")
	}

	// Warning: open/draft periods exist in the year
	if report.OpenPeriods > 0 {
		addIssue(models.ReviewWarning, "open_periods_in_year",
			fmt.Sprintf("%d pay period(s) in %d are not finalized. Finalize all periods before T4 generation to ensure complete income reporting.",
				report.OpenPeriods, calendarYear),
			"", "")
	}

	// Warning: fewer finalized periods than expected
	if report.FinalizedPeriods > 0 && report.FinalizedPeriods < report.ExpectedPeriods {
		addIssue(models.ReviewWarning, "fewer_periods_than_expected",
			fmt.Sprintf("Expected %d pay periods for %s frequency, but only %d are finalized. Verify no periods are missing.",
				report.ExpectedPeriods, companyFreq, report.FinalizedPeriods),
			"", "")
	}

	// Per-employee checks
	for _, row := range report.Employees {
		// Error: missing SIN
		if row.SINMissing {
			addIssue(models.ReviewError, "missing_sin",
				fmt.Sprintf("%s — SIN is missing. CRA requires a SIN for every T4 slip. Collect and enter the SIN before filing.", row.LegalName),
				row.EmployeeID, row.LegalName)
		}
		// Error: missing province
		if row.ProvinceMissing {
			addIssue(models.ReviewError, "missing_province",
				fmt.Sprintf("%s — Province of employment is missing. Required for correct provincial tax reporting on the T4.", row.LegalName),
				row.EmployeeID, row.LegalName)
		}
		// Warning: CPP over annual maximum (tolerance ±$2)
		if report.CPPMaxEE > 0 && row.CPPEmployee > report.CPPMaxEE+2.0 {
			addIssue(models.ReviewWarning, "cpp_over_maximum",
				fmt.Sprintf("%s — Employee CPP contributions ($%.2f) exceed the %d annual maximum ($%.2f). Review the payroll calculation for this employee.",
					row.LegalName, row.CPPEmployee, calendarYear, report.CPPMaxEE),
				row.EmployeeID, row.LegalName)
		}
		// Warning: EI over annual maximum (tolerance ±$2)
		if report.EIMaxEE > 0 && row.EIEmployee > report.EIMaxEE+2.0 {
			addIssue(models.ReviewWarning, "ei_over_maximum",
				fmt.Sprintf("%s — Employee EI premiums ($%.2f) exceed the %d annual maximum ($%.2f). Review the payroll calculation for this employee.",
					row.LegalName, row.EIEmployee, calendarYear, report.EIMaxEE),
				row.EmployeeID, row.LegalName)
		}
		// Warning: active employee with zero gross pay
		if row.Status == "active" && row.GrossPay == 0 {
			addIssue(models.ReviewWarning, "active_employee_no_pay",
				fmt.Sprintf("%s — This active employee has $0 gross pay for %d. Confirm whether this is correct (e.g., on leave) or if periods are missing.",
					row.LegalName, calendarYear),
				row.EmployeeID, row.LegalName)
		}
		// Info: negative income tax deducted
		if row.FederalTax < 0 || row.ProvincialTax < 0 {
			addIssue(models.ReviewInfo, "negative_tax",
				fmt.Sprintf("%s — Negative income tax deducted (federal: $%.2f, provincial: $%.2f). Verify this is intentional (e.g., tax correction entry).",
					row.LegalName, row.FederalTax, row.ProvincialTax),
				row.EmployeeID, row.LegalName)
		}
	}

	// Info: no employees paid at all
	if report.FinalizedPeriods > 0 && report.TotalEmployees == 0 {
		addIssue(models.ReviewInfo, "no_employees_paid",
			fmt.Sprintf("Finalized periods exist for %d but no employees appear in the payroll entries.", calendarYear),
			"", "")
	}

	// ── T4 status check ───────────────────────────────────────────────────────
	var t4Status string
	_ = s.db.QueryRow(`
		SELECT status FROM payroll_year_end_returns
		WHERE company_id=? AND calendar_year=?
		ORDER BY created_at DESC LIMIT 1
	`, companyID, calendarYear).Scan(&t4Status)
	report.HasT4Draft = t4Status == "draft"
	report.HasT4Final = t4Status == "finalized"

	// T4Ready = no error-severity issues
	report.T4Ready = true
	for _, iss := range report.Issues {
		if iss.Severity == models.ReviewError {
			report.T4Ready = false
			break
		}
	}

	// Round totals
	report.TotalGrossPay = roundT4(report.TotalGrossPay)
	report.TotalCPPEmployee = roundT4(report.TotalCPPEmployee)
	report.TotalCPP2Employee = roundT4(report.TotalCPP2Employee)
	report.TotalEIEmployee = roundT4(report.TotalEIEmployee)
	report.TotalFederalTax = roundT4(report.TotalFederalTax)
	report.TotalProvincialTax = roundT4(report.TotalProvincialTax)
	report.TotalCPPEmployer = roundT4(report.TotalCPPEmployer)
	report.TotalEIEmployer = roundT4(report.TotalEIEmployer)

	return report
}
