package api

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"simpletask/internal/store"
)

// GET /api/payroll/report/period-summary?period_id=PP00001
func (s *Server) handleReportPeriodSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	periodID := strings.TrimSpace(r.URL.Query().Get("period_id"))
	if periodID == "" {
		http.Error(w, "period_id is required", http.StatusBadRequest)
		return
	}
	p, err := s.Store.GetPayrollPeriod(periodID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if !s.checkCompanyAccess(w, r, p.CompanyID) {
		return
	}
	sm := s.Store.GetPeriodSummary(periodID)
	writeJSON(w, http.StatusOK, sm)
}

// GET /api/payroll/report/remittance?company_id=PC0001&month=2025-03
func (s *Server) handleReportRemittance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	companyID := strings.TrimSpace(r.URL.Query().Get("company_id"))
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if companyID == "" {
		http.Error(w, "company_id is required", http.StatusBadRequest)
		return
	}
	if !s.checkCompanyAccess(w, r, companyID) {
		return
	}
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		http.Error(w, "invalid month, use YYYY-MM", http.StatusBadRequest)
		return
	}
	list := s.Store.GetMonthlyRemittance(companyID, month)

	// Build totals across periods
	type response struct {
		Month   string                   `json:"month"`
		Periods []interface{}            `json:"periods"`
		Totals  map[string]float64       `json:"totals"`
	}

	var (
		totGross, totNet, totDed                         float64
		totCPPEe, totCPP2Ee, totEIEe                    float64
		totFed, totProv                                  float64
		totCPPEr, totCPP2Er, totEIEr, totRemit          float64
	)
	periods := make([]interface{}, len(list))
	for i, rp := range list {
		periods[i] = rp
		totGross    += rp.TotalGross
		totNet      += rp.TotalNet
		totDed      += rp.TotalDeductions
		totCPPEe    += rp.TotalCPPEe
		totCPP2Ee   += rp.TotalCPP2Ee
		totEIEe     += rp.TotalEIEe
		totFed      += rp.TotalFedTax
		totProv     += rp.TotalProvTax
		totCPPEr    += rp.TotalCPPEr
		totCPP2Er   += rp.TotalCPP2Er
		totEIEr     += rp.TotalEIEr
		totRemit    += rp.RemittanceTotal
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"month":   month,
		"periods": periods,
		"totals": map[string]float64{
			"totalGross":        totGross,
			"totalNet":          totNet,
			"totalDeductions":   totDed,
			"totalCppEmployee":  totCPPEe,
			"totalCpp2Employee": totCPP2Ee,
			"totalEiEmployee":   totEIEe,
			"totalFederalTax":   totFed,
			"totalProvincialTax": totProv,
			"totalCppEmployer":  totCPPEr,
			"totalCpp2Employer": totCPP2Er,
			"totalEiEmployer":   totEIEr,
			"remittanceTotal":   totRemit,
		},
	})
}

// GET /api/payroll/export/csv?period_id=PP00001
// Returns a CSV of all entries for a period (suitable for accountant handoff).
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	periodID := strings.TrimSpace(r.URL.Query().Get("period_id"))
	if periodID == "" {
		http.Error(w, "period_id is required", http.StatusBadRequest)
		return
	}

	period, pErr := s.Store.GetPayrollPeriod(periodID)
	if errors.Is(pErr, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if !s.checkCompanyAccess(w, r, period.CompanyID) {
		return
	}
	entries := s.Store.ListPayrollEntries(periodID)

	fname := fmt.Sprintf("payroll-%s-%s.csv", periodID, period.PayDate)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`"`)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"EmployeeID", "EmployeeName",
		"Hours", "PayRate", "GrossPay",
		"CPP_Employee", "CPP2_Employee", "EI_Employee",
		"FederalTax", "ProvincialTax",
		"TotalDeductions", "NetPay",
		"CPP_Employer", "CPP2_Employer", "EI_Employer",
		"YTD_Gross", "YTD_CPP_Ee", "YTD_CPP2_Ee", "YTD_EI_Ee",
		"Status",
	})
	for _, e := range entries {
		_ = cw.Write([]string{
			e.EmployeeID, e.EmployeeName,
			f2s(e.Hours), f2s(e.PayRate), f2s(e.GrossPay),
			f2s(e.CPPEmployee), f2s(e.CPP2Employee), f2s(e.EIEmployee),
			f2s(e.FederalTax), f2s(e.ProvincialTax),
			f2s(e.TotalDeductions), f2s(e.NetPay),
			f2s(e.CPPEmployer), f2s(e.CPP2Employer), f2s(e.EIEmployer),
			f2s(e.YTDGross), f2s(e.YTDCPPEe), f2s(e.YTDCPP2Ee), f2s(e.YTDEIEe),
			e.Status,
		})
	}
	cw.Flush()
}

func f2s(v float64) string {
	return fmt.Sprintf("%.2f", v)
}
