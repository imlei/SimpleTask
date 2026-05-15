package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"simpletask/internal/models"
	"simpletask/internal/store"
)

// GET  /api/payroll/periods?company_id=PC0001
// POST /api/payroll/periods
func (s *Server) handlePayrollPeriods(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		companyID := strings.TrimSpace(r.URL.Query().Get("company_id"))
		if companyID == "" {
			http.Error(w, "company_id is required", http.StatusBadRequest)
			return
		}
		if !s.checkCompanyAccess(w, r, companyID) {
			return
		}
		list := s.Store.ListPayrollPeriods(companyID)
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var p models.PayrollPeriod
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(p.CompanyID) == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}
		if !s.checkCompanyAccess(w, r, p.CompanyID) {
			return
		}
		if strings.TrimSpace(p.PayDate) == "" {
			http.Error(w, "payDate is required", http.StatusBadRequest)
			return
		}
		if p.PaysPerYear == 0 {
			p.PaysPerYear = 26
		}
		created, err := s.Store.CreatePayrollPeriod(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// GET /api/payroll/periods/{id}
// Subroutes:
//
//	GET    /api/payroll/periods/{id}/entries
//	POST   /api/payroll/periods/{id}/entries       — upsert one entry (gross pay input)
//	POST   /api/payroll/periods/{id}/calculate     — run CPP/EI/Tax for all entries
//	POST   /api/payroll/periods/{id}/finalize      — mark period as finalized
func (s *Server) handlePayrollPeriodByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/payroll/periods/")
	parts := strings.SplitN(rest, "/", 2)
	periodID := strings.TrimSpace(parts[0])
	if periodID == "" {
		http.NotFound(w, r)
		return
	}

	// Subroute dispatch
	if len(parts) == 2 {
		switch parts[1] {
		case "entries":
			s.handlePeriodEntries(w, r, periodID)
		case "calculate":
			s.handlePeriodCalculate(w, r, periodID)
		case "finalize":
			s.handlePeriodFinalize(w, r, periodID)
		case "recalculate":
			s.handlePeriodRecalculate(w, r, periodID)
		default:
			http.NotFound(w, r)
		}
		return
	}

	// Base: GET /api/payroll/periods/{id}
	if r.Method == http.MethodDelete {
		s.handleDeletePeriod(w, r, periodID)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	p, err := s.Store.GetPayrollPeriod(periodID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !s.checkCompanyAccess(w, r, p.CompanyID) {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// GET  /api/payroll/periods/{id}/entries   — list entries (with calculated values if calculated)
// POST /api/payroll/periods/{id}/entries   — upsert entry (gross pay / hours input)
func (s *Server) handlePeriodEntries(w http.ResponseWriter, r *http.Request, periodID string) {
	p, err := s.Store.GetPayrollPeriod(periodID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if !s.checkCompanyAccess(w, r, p.CompanyID) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		entries := s.Store.ListPayrollEntries(periodID)
		writeJSON(w, http.StatusOK, entries)

	case http.MethodPost:
		// Accept either a single entry or a bulk array
		body, err := readBody(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Try array first
		var bulk []models.PayrollEntry
		if json.Unmarshal(body, &bulk) == nil && len(bulk) > 0 {
			for _, e := range bulk {
				e.PeriodID = periodID
				if _, err := s.Store.UpsertPayrollEntry(e); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			writeJSON(w, http.StatusOK, s.Store.ListPayrollEntries(periodID))
			return
		}

		// Single entry
		var e models.PayrollEntry
		if err := json.Unmarshal(body, &e); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.PeriodID = periodID
		saved, err := s.Store.UpsertPayrollEntry(e)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, saved)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// POST /api/payroll/periods/{id}/calculate
func (s *Server) handlePeriodCalculate(w http.ResponseWriter, r *http.Request, periodID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
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
	if p.Status == "finalized" {
		http.Error(w, "period is already finalized", http.StatusConflict)
		return
	}

	// Use tax year matching pay_date year; load from DB (with hardcoded fallback)
	payYear := 2025
	if len(p.PayDate) >= 4 {
		if y, err2 := strconv.Atoi(p.PayDate[:4]); err2 == nil && y > 2000 {
			payYear = y
		}
	}
	rates := s.Store.GetPayrollRatesForYear(payYear)

	entries, err := s.Store.CalculatePeriod(periodID, rates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// POST /api/payroll/periods/{id}/finalize
func (s *Server) handlePeriodFinalize(w http.ResponseWriter, r *http.Request, periodID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
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
	if p.Status != "calculated" {
		http.Error(w, "period must be in 'calculated' status before finalizing", http.StatusConflict)
		return
	}
	if err := s.Store.UpdatePayrollPeriodStatus(periodID, "finalized"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.Status = "finalized"
	writeJSON(w, http.StatusOK, p)
}

// POST /api/payroll/periods/{id}/recalculate
// Recalculates a period that was already calculated. Resets entries to "approved" status and recalculates.
func (s *Server) handlePeriodRecalculate(w http.ResponseWriter, r *http.Request, periodID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
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

	// Use tax year matching pay_date year
	payYear := 2025
	if len(p.PayDate) >= 4 {
		if y, err2 := strconv.Atoi(p.PayDate[:4]); err2 == nil && y > 2000 {
			payYear = y
		}
	}
	rates := s.Store.GetPayrollRatesForYear(payYear)

	entries, err := s.Store.RecalculatePeriod(periodID, rates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// DELETE /api/payroll/periods/{id}
func (s *Server) handleDeletePeriod(w http.ResponseWriter, r *http.Request, periodID string) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
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

	// Check query param for force deletion of finalized periods
	force := strings.TrimSpace(r.URL.Query().Get("force")) == "true"

	if err := s.Store.DeletePayrollPeriod(periodID, force); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Period deleted"})
}

// ── helpers ────────────────────────────────────────────────────────────────────

// initEntriesForPeriod creates draft entries for all active employees in the company
// if no entries exist yet. Called when salaries page loads.
func (s *Server) handleInitPeriodEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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

	existing := s.Store.ListPayrollEntries(periodID)
	if len(existing) > 0 {
		writeJSON(w, http.StatusOK, existing)
		return
	}

	// Load all active employees for this company
	employees := s.Store.ListPayrollEmployees(p.CompanyID, "active")
	created := make([]models.PayrollEntry, 0, len(employees))
	for _, emp := range employees {
		// Skip contractors for entry init (they still get entries but $0 deductions)
		hours := emp.HoursPerWeek * 2 // default: bi-weekly hours
		if p.PaysPerYear == 24 {
			hours = (emp.HoursPerWeek * 52) / 24
		} else if p.PaysPerYear == 12 {
			hours = emp.HoursPerWeek * 52 / 12
		} else if p.PaysPerYear == 52 {
			hours = emp.HoursPerWeek
		}

		rate := emp.PayRate
		gross := 0.0
		if emp.SalaryType == 0 {
			// Salaried: annual rate / pays per year
			switch emp.PayRateUnit {
			case "Annually":
				gross = roundHalf(emp.PayRate / float64(p.PaysPerYear))
			case "Monthly":
				gross = roundHalf(emp.PayRate * 12 / float64(p.PaysPerYear))
			default:
				gross = roundHalf(emp.PayRate / float64(p.PaysPerYear))
			}
			hours = 0
		} else {
			// Time-based: hours * rate
			gross = roundHalf(hours * rate)
		}

		e := models.PayrollEntry{
			PeriodID:    periodID,
			EmployeeID:  emp.ID,
			CompanyID:   p.CompanyID,
			SalaryType:  emp.SalaryType,
			PayRateUnit: emp.PayRateUnit,
			Hours:       roundHalf(hours),
			PayRate:     rate,
			GrossPay:    gross,
			Status:      "draft",
		}
		saved, err := s.Store.UpsertPayrollEntry(e)
		if err == nil {
			saved.EmployeeName = emp.LegalName
			saved.SalaryType = emp.SalaryType
			saved.PayRateUnit = emp.PayRateUnit
			created = append(created, saved)
		}
	}
	writeJSON(w, http.StatusCreated, created)
}

func readBody(r *http.Request) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 512)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

func roundHalf(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
