package store

// PeriodSummary aggregates payroll_entries for one period.
type PeriodSummary struct {
	PeriodID        string  `json:"periodId"`
	EmployeeCount   int     `json:"employeeCount"`
	TotalGross      float64 `json:"totalGross"`
	TotalNet        float64 `json:"totalNet"`
	TotalDeductions float64 `json:"totalDeductions"`
	TotalCPPEe      float64 `json:"totalCppEmployee"`
	TotalCPP2Ee     float64 `json:"totalCpp2Employee"`
	TotalEIEe       float64 `json:"totalEiEmployee"`
	TotalFedTax     float64 `json:"totalFederalTax"`
	TotalProvTax    float64 `json:"totalProvincialTax"`
	TotalCPPEr      float64 `json:"totalCppEmployer"`
	TotalCPP2Er     float64 `json:"totalCpp2Employer"`
	TotalEIEr       float64 `json:"totalEiEmployer"`
	RemittanceTotal float64 `json:"remittanceTotal"` // CPP(ee+er) + CPP2(ee+er) + EI(ee+er) + FedTax + ProvTax
}

// RemittancePeriod is one pay period's contribution to a monthly PD7A.
type RemittancePeriod struct {
	PeriodID    string `json:"periodId"`
	PayDate     string `json:"payDate"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	PeriodSummary
}

// GetPeriodSummary aggregates all entries for a single period.
func (s *Store) GetPeriodSummary(periodID string) PeriodSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sm PeriodSummary
	sm.PeriodID = periodID

	_ = s.db.QueryRow(`
		SELECT
		  COUNT(*),
		  COALESCE(SUM(gross_pay),0),
		  COALESCE(SUM(net_pay),0),
		  COALESCE(SUM(total_deductions),0),
		  COALESCE(SUM(cpp_ee),0),
		  COALESCE(SUM(cpp2_ee),0),
		  COALESCE(SUM(ei_ee),0),
		  COALESCE(SUM(federal_tax),0),
		  COALESCE(SUM(provincial_tax),0),
		  COALESCE(SUM(cpp_er),0),
		  COALESCE(SUM(cpp2_er),0),
		  COALESCE(SUM(ei_er),0)
		FROM payroll_entries
		WHERE period_id = ?`, periodID).Scan(
		&sm.EmployeeCount,
		&sm.TotalGross, &sm.TotalNet, &sm.TotalDeductions,
		&sm.TotalCPPEe, &sm.TotalCPP2Ee, &sm.TotalEIEe,
		&sm.TotalFedTax, &sm.TotalProvTax,
		&sm.TotalCPPEr, &sm.TotalCPP2Er, &sm.TotalEIEr,
	)
	sm.RemittanceTotal = sm.TotalCPPEe + sm.TotalCPP2Ee + sm.TotalEIEe +
		sm.TotalFedTax + sm.TotalProvTax +
		sm.TotalCPPEr + sm.TotalCPP2Er + sm.TotalEIEr
	return sm
}

// GetMonthlyRemittance returns one RemittancePeriod per pay period whose
// pay_date falls in the given YYYY-MM month, for the given company.
// Used to build the CRA PD7A remittance summary.
func (s *Store) GetMonthlyRemittance(companyID, month string) []RemittancePeriod {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT p.id, p.pay_date, p.period_start, p.period_end,
		  COUNT(e.id),
		  COALESCE(SUM(e.gross_pay),0),
		  COALESCE(SUM(e.net_pay),0),
		  COALESCE(SUM(e.total_deductions),0),
		  COALESCE(SUM(e.cpp_ee),0),
		  COALESCE(SUM(e.cpp2_ee),0),
		  COALESCE(SUM(e.ei_ee),0),
		  COALESCE(SUM(e.federal_tax),0),
		  COALESCE(SUM(e.provincial_tax),0),
		  COALESCE(SUM(e.cpp_er),0),
		  COALESCE(SUM(e.cpp2_er),0),
		  COALESCE(SUM(e.ei_er),0)
		FROM payroll_periods p
		LEFT JOIN payroll_entries e ON e.period_id = p.id
		WHERE p.company_id = ?
		  AND substr(p.pay_date, 1, 7) = ?
		  AND p.status IN ('calculated','finalized')
		GROUP BY p.id
		ORDER BY p.pay_date ASC`, companyID, month)
	if err != nil {
		return []RemittancePeriod{}
	}
	defer rows.Close()

	var list []RemittancePeriod
	for rows.Next() {
		var rp RemittancePeriod
		if err := rows.Scan(
			&rp.PeriodID, &rp.PayDate, &rp.PeriodStart, &rp.PeriodEnd,
			&rp.EmployeeCount,
			&rp.TotalGross, &rp.TotalNet, &rp.TotalDeductions,
			&rp.TotalCPPEe, &rp.TotalCPP2Ee, &rp.TotalEIEe,
			&rp.TotalFedTax, &rp.TotalProvTax,
			&rp.TotalCPPEr, &rp.TotalCPP2Er, &rp.TotalEIEr,
		); err != nil {
			continue
		}
		rp.PeriodID = rp.PeriodID // already set by Scan
		rp.RemittanceTotal = rp.TotalCPPEe + rp.TotalCPP2Ee + rp.TotalEIEe +
			rp.TotalFedTax + rp.TotalProvTax +
			rp.TotalCPPEr + rp.TotalCPP2Er + rp.TotalEIEr
		list = append(list, rp)
	}
	if list == nil {
		return []RemittancePeriod{}
	}
	return list
}
