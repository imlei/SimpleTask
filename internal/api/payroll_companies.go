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

// GET  /api/payroll/companies?status=active|all
// POST /api/payroll/companies
func (s *Server) handlePayrollCompanies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
		if statusFilter == "" {
			statusFilter = "active"
		}
		list := s.Store.ListPayrollCompanies(statusFilter)
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		if auth.RoleFromContext(r.Context()) == "user1" && s.Store.CountPayrollCompanies() >= 1 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "试用账户最多只能创建 1 家公司"})
			return
		}
		var c models.PayrollCompany
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(c.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		created := s.Store.CreatePayrollCompany(c)
		writeJSON(w, http.StatusCreated, created)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// GET    /api/payroll/companies/{id}
// PUT    /api/payroll/companies/{id}
// DELETE /api/payroll/companies/{id}
func (s *Server) handlePayrollCompanyByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/payroll/companies/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		c, err := s.Store.GetPayrollCompany(id)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, c)

	case http.MethodPut:
		var patch models.PayrollCompany
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(patch.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		updated, err := s.Store.UpdatePayrollCompany(id, patch)
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
		if err := s.Store.DeletePayrollCompany(id); err != nil {
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
