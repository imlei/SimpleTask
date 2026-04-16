package store

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/models"
	"simpletask/internal/payroll/calculator"
)

// YTDSnapshot holds year-to-date aggregates for one employee BEFORE a given period.
type YTDSnapshot struct {
	Gross  float64
	CPPEe  float64
	CPP2Ee float64
	EIEe   float64
}

// GetYTDBeforePeriod aggregates all finalized/calculated entries for an employee
// in the same calendar year, excluding the given period.
func (s *Store) GetYTDBeforePeriod(employeeID, periodID, payYear string) YTDSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	var snap YTDSnapshot
	// Join with payroll_periods to filter by calendar year of pay_date
	_ = s.db.QueryRow(`
		SELECT COALESCE(SUM(e.gross_pay),0),
		       COALESCE(SUM(e.cpp_ee),0),
		       COALESCE(SUM(e.cpp2_ee),0),
		       COALESCE(SUM(e.ei_ee),0)
		FROM payroll_entries e
		JOIN payroll_periods p ON p.id = e.period_id
		WHERE e.employee_id = ?
		  AND e.period_id <> ?
		  AND e.status IN ('draft','approved')
		  AND substr(p.pay_date, 1, 4) = ?`,
		employeeID, periodID, payYear,
	).Scan(&snap.Gross, &snap.CPPEe, &snap.CPP2Ee, &snap.EIEe)
	return snap
}

// UpsertPayrollEntry creates or updates an entry for (periodID, employeeID).
func (s *Store) UpsertPayrollEntry(e models.PayrollEntry) (models.PayrollEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)

	// Check if exists
	var existingID string
	_ = s.db.QueryRow(`SELECT id FROM payroll_entries WHERE period_id=? AND employee_id=?`,
		e.PeriodID, e.EmployeeID).Scan(&existingID)

	if existingID == "" {
		e.ID = s.nextEntryID()
		e.CreatedAt = now
		e.UpdatedAt = now
		if e.Status == "" {
			e.Status = "draft"
		}
		snap := "{}"
		if e.CalcSnapshotJSON != "" {
			snap = e.CalcSnapshotJSON
		}
		_, err := s.db.Exec(`
			INSERT INTO payroll_entries
			  (id, period_id, employee_id, company_id, hours, pay_rate, gross_pay,
			   cpp_ee, cpp2_ee, ei_ee, federal_tax, provincial_tax,
			   total_deductions, net_pay, cpp_er, cpp2_er, ei_er,
			   ytd_gross, ytd_cpp_ee, ytd_cpp2_ee, ytd_ei_ee,
			   calc_snapshot_json, status, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.ID, e.PeriodID, e.EmployeeID, e.CompanyID, e.Hours, e.PayRate, e.GrossPay,
			e.CPPEmployee, e.CPP2Employee, e.EIEmployee, e.FederalTax, e.ProvincialTax,
			e.TotalDeductions, e.NetPay, e.CPPEmployer, e.CPP2Employer, e.EIEmployer,
			e.YTDGross, e.YTDCPPEe, e.YTDCPP2Ee, e.YTDEIEe,
			snap, e.Status, e.CreatedAt, e.UpdatedAt,
		)
		return e, err
	}

	// Update
	e.ID = existingID
	e.UpdatedAt = now
	snap := "{}"
	if e.CalcSnapshotJSON != "" {
		snap = e.CalcSnapshotJSON
	}
	_, err := s.db.Exec(`
		UPDATE payroll_entries SET
		  hours=?, pay_rate=?, gross_pay=?,
		  cpp_ee=?, cpp2_ee=?, ei_ee=?, federal_tax=?, provincial_tax=?,
		  total_deductions=?, net_pay=?, cpp_er=?, cpp2_er=?, ei_er=?,
		  ytd_gross=?, ytd_cpp_ee=?, ytd_cpp2_ee=?, ytd_ei_ee=?,
		  calc_snapshot_json=?, status=?, updated_at=?
		WHERE id=?`,
		e.Hours, e.PayRate, e.GrossPay,
		e.CPPEmployee, e.CPP2Employee, e.EIEmployee, e.FederalTax, e.ProvincialTax,
		e.TotalDeductions, e.NetPay, e.CPPEmployer, e.CPP2Employer, e.EIEmployer,
		e.YTDGross, e.YTDCPPEe, e.YTDCPP2Ee, e.YTDEIEe,
		snap, e.Status, now, existingID,
	)
	return e, err
}

// ListPayrollEntries returns all entries for a period, joined with employee name.
// GetPayrollEntry returns a single entry by ID (used for ownership checks).
func (s *Store) GetPayrollEntry(id string) (models.PayrollEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var e models.PayrollEntry
	err := s.db.QueryRow(`SELECT id, period_id, employee_id, company_id FROM payroll_entries WHERE id=?`, id).
		Scan(&e.ID, &e.PeriodID, &e.EmployeeID, &e.CompanyID)
	if err != nil {
		return e, ErrNotFound
	}
	return e, nil
}

func (s *Store) ListPayrollEntries(periodID string) []models.PayrollEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT e.id, e.period_id, e.employee_id, e.company_id,
		       COALESCE(pe.legal_name,''),
		       e.hours, e.pay_rate, e.gross_pay,
		       e.cpp_ee, e.cpp2_ee, e.ei_ee, e.federal_tax, e.provincial_tax,
		       e.total_deductions, e.net_pay,
		       e.cpp_er, e.cpp2_er, e.ei_er,
		       e.ytd_gross, e.ytd_cpp_ee, e.ytd_cpp2_ee, e.ytd_ei_ee,
		       e.calc_snapshot_json, e.status,
		       e.created_at, e.updated_at
		FROM payroll_entries e
		LEFT JOIN payroll_employees pe ON pe.id = e.employee_id
		WHERE e.period_id = ?
		ORDER BY pe.legal_name ASC`, periodID)
	if err != nil {
		return []models.PayrollEntry{}
	}
	defer rows.Close()

	var list []models.PayrollEntry
	for rows.Next() {
		var e models.PayrollEntry
		if err := rows.Scan(
			&e.ID, &e.PeriodID, &e.EmployeeID, &e.CompanyID,
			&e.EmployeeName,
			&e.Hours, &e.PayRate, &e.GrossPay,
			&e.CPPEmployee, &e.CPP2Employee, &e.EIEmployee, &e.FederalTax, &e.ProvincialTax,
			&e.TotalDeductions, &e.NetPay,
			&e.CPPEmployer, &e.CPP2Employer, &e.EIEmployer,
			&e.YTDGross, &e.YTDCPPEe, &e.YTDCPP2Ee, &e.YTDEIEe,
			&e.CalcSnapshotJSON, &e.Status,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			continue
		}
		list = append(list, e)
	}
	if list == nil {
		return []models.PayrollEntry{}
	}
	return list
}

// CalculatePeriod runs CPP/EI/Tax for all entries in a period and updates them.
// Returns updated entries.
func (s *Store) CalculatePeriod(periodID string, rates calculator.TaxYear) ([]models.PayrollEntry, error) {
	period, err := s.GetPayrollPeriod(periodID)
	if err != nil {
		return nil, err
	}

	// Get all entries for this period
	entries := s.ListPayrollEntries(periodID)
	if len(entries) == 0 {
		return entries, nil
	}

	// Derive calendar year from pay_date
	payYear := "2025"
	if len(period.PayDate) >= 4 {
		payYear = period.PayDate[:4]
	}

	// Load company rules (vacation rate & method)
	companyRules := s.GetCompanyRules(period.CompanyID)
	vacationCodeID := s.GetVacationCodeID(period.CompanyID)

	// Load employee province/TD1 data
	type empInfo struct {
		Province     string
		TD1Federal   float64
		TD1Prov      float64
		MemberType   int
		AutoVacation bool
	}
	empCache := map[string]empInfo{}

	s.mu.Lock()
	rows, err := s.db.Query(
		`SELECT id, province, td1_federal, td1_provincial, member_type, auto_vacation
		 FROM payroll_employees WHERE company_id = ?`, period.CompanyID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var info empInfo
			var autoVac int
			if err := rows.Scan(&id, &info.Province, &info.TD1Federal, &info.TD1Prov, &info.MemberType, &autoVac); err == nil {
				info.AutoVacation = autoVac != 0
				empCache[id] = info
			}
		}
	}
	s.mu.Unlock()

	updated := make([]models.PayrollEntry, 0, len(entries))
	for _, e := range entries {
		// Only calculate entries that have been explicitly approved; skip drafts.
		if e.Status != "approved" {
			updated = append(updated, e)
			continue
		}

		info := empCache[e.EmployeeID]

		// Contractors: no CPP/EI/tax deductions
		if info.MemberType == 1 || info.MemberType == 2 {
			e.CPPEmployee = 0
			e.CPP2Employee = 0
			e.EIEmployee = 0
			e.FederalTax = 0
			e.ProvincialTax = 0
			e.TotalDeductions = 0
			e.NetPay = e.GrossPay
			e.CPPEmployer = 0
			e.CPP2Employer = 0
			e.EIEmployer = 0
			e.Status = "calculated"
		} else {
			// Get YTD before this period
			ytd := s.GetYTDBeforePeriod(e.EmployeeID, periodID, payYear)
			e.YTDGross = ytd.Gross
			e.YTDCPPEe = ytd.CPPEe
			e.YTDCPP2Ee = ytd.CPP2Ee
			e.YTDEIEe = ytd.EIEe

			// Fetch additional earnings lines and compute gross breakdowns.
			// Base gross (e.GrossPay) is fully CPP/EI/tax/vacationable applicable.
			// Additional earnings lines respect their per-code flags.
			addl := s.EarningsGrossForEntry(e.ID)

			// Vacation pay (per_period method): add vacation % of vacationable base.
			// Only applies when the employee has AutoVacation enabled.
			// vacationable base = base pay + vacationable additional earnings.
			var vacPay float64
			if info.AutoVacation && companyRules.VacationMethod == "per_period" && companyRules.VacationRate > 0 && vacationCodeID != "" {
				vacationableBase := e.GrossPay + addl.VacationableTotal
				raw := vacationableBase * companyRules.VacationRate
				vacPay = float64(int64(raw*100+0.5)) / 100
				if vacPay > 0 {
					// Add vacation pay as an entry earnings line (replaces existing VAC line if any).
					existingLines := s.ListEntryEarnings(e.ID)
					var newLines []models.PayrollEntryEarning
					for _, l := range existingLines {
						if l.EarningsCodeID != vacationCodeID {
							newLines = append(newLines, l)
						}
					}
					newLines = append(newLines, models.PayrollEntryEarning{
						EntryID:        e.ID,
						EarningsCodeID: vacationCodeID,
						CodeName:       "Vacation Pay",
						Amount:         vacPay,
					})
					_, _ = s.ReplaceEntryEarnings(e.ID, newLines)
					// Re-fetch after vacation line added
					addl = s.EarningsGrossForEntry(e.ID)
				}
			}

			totalGross := e.GrossPay + addl.Total
			cppGross := e.GrossPay + addl.CPP
			eiGross := e.GrossPay + addl.EI
			e.GrossPay = totalGross

			in := calculator.Input{
				Province:   info.Province,
				PayPeriods: period.PaysPerYear,
				GrossPay:   totalGross,
				CPPGross:   cppGross,
				EIGross:    eiGross,
				TD1Federal: info.TD1Federal,
				TD1Prov:    info.TD1Prov,
				YTDGross:   ytd.Gross,
				YTDCPPEe:   ytd.CPPEe,
				YTDCPP2Ee:  ytd.CPP2Ee,
				YTDEIEe:    ytd.EIEe,
			}
			result := calculator.Calculate(in, rates)

			e.CPPEmployee = result.CPPEmployee
			e.CPP2Employee = result.CPP2Employee
			e.EIEmployee = result.EIEmployee
			e.FederalTax = result.FederalTax
			e.ProvincialTax = result.ProvincialTax
			e.TotalDeductions = result.TotalDeductions
			e.NetPay = result.NetPay
			e.CPPEmployer = result.CPPEmployer
			e.CPP2Employer = result.CPP2Employer
			e.EIEmployer = result.EIEmployer
			e.Status = "calculated"

			// Store calc snapshot
			snap := map[string]any{
				"province":    info.Province,
				"payPeriods":  period.PaysPerYear,
				"grossPay":    e.GrossPay,
				"td1Federal":  info.TD1Federal,
				"td1Prov":     info.TD1Prov,
				"ytdGross":    ytd.Gross,
				"ytdCPPEe":    ytd.CPPEe,
				"ytdEIEe":     ytd.EIEe,
				"ratesYear":   rates.Year,
				"cppRate":     rates.CPPRate,
				"eiRate":      rates.EIRate,
			}
			b, _ := json.Marshal(snap)
			e.CalcSnapshotJSON = string(b)
		}

		saved, err := s.UpsertPayrollEntry(e)
		if err != nil {
			return nil, fmt.Errorf("save entry %s: %w", e.EmployeeID, err)
		}
		updated = append(updated, saved)
	}

	// Mark period as calculated
	_ = s.UpdatePayrollPeriodStatus(periodID, "calculated")

	return updated, nil
}

func (s *Store) nextEntryID() string {
	rows, err := s.db.Query(`SELECT id FROM payroll_entries WHERE id LIKE 'PE%'`)
	if err != nil {
		return "PE00001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "PE") {
			if v, err := strconv.Atoi(strings.TrimPrefix(id, "PE")); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("PE%05d", max+1)
}
