package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"simpletask/internal/auth"
	"simpletask/internal/models"
	"simpletask/internal/store"
)

// Province integer index → 2-letter code (matches onboard-personal.html select order)
var provinceIndexToCode = []string{
	"AB", "BC", "MB", "NB", "NL", "NS", "NT", "NU",
	"ON", "PE", "QC", "SK", "YT", "",
}

func resolveProvince(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 2 {
		return strings.ToUpper(raw)
	}
	// Legacy integer index from wizard
	if len(raw) <= 2 {
		idx := 0
		for i, r := range raw {
			_ = i
			if r >= '0' && r <= '9' {
				idx = idx*10 + int(r-'0')
			}
		}
		if idx >= 0 && idx < len(provinceIndexToCode) {
			return provinceIndexToCode[idx]
		}
	}
	return raw
}

// GET  /api/payroll/employees?company_id=PC0001&status=active
// POST /api/payroll/employees
func (s *Server) handlePayrollEmployees(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		companyID := strings.TrimSpace(r.URL.Query().Get("company_id"))
		if companyID == "" {
			http.Error(w, "company_id is required", http.StatusBadRequest)
			return
		}
		statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
		if statusFilter == "" {
			statusFilter = "active"
		}
		list := s.Store.ListPayrollEmployees(companyID, statusFilter)
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var e models.PayrollEmployee
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(e.LegalName) == "" {
			http.Error(w, "legalName is required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(e.CompanyID) == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}
		if auth.RoleFromContext(r.Context()) == "user1" && s.Store.CountPayrollEmployees(e.CompanyID) >= 3 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "试用账户每家公司最多只能添加 3 名员工"})
			return
		}
		e.Province = resolveProvince(e.Province)

		created, err := s.Store.CreatePayrollEmployee(e)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// GET    /api/payroll/employees/{id}
// PUT    /api/payroll/employees/{id}
// DELETE /api/payroll/employees/{id}  — soft-terminates
func (s *Server) handlePayrollEmployeeByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/payroll/employees/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		e, err := s.Store.GetPayrollEmployee(id)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, e)

	case http.MethodPut:
		var patch models.PayrollEmployee
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(patch.LegalName) == "" {
			http.Error(w, "legalName is required", http.StatusBadRequest)
			return
		}
		patch.Province = resolveProvince(patch.Province)

		updated, err := s.Store.UpdatePayrollEmployee(id, patch)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		// Soft-terminate: CRA requires 6-year record retention (T4001 §8)
		if err := s.Store.TerminatePayrollEmployee(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
