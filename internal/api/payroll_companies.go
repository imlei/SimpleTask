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

// checkCompanyAccess verifies the current user may access companyID.
// Writes 404 or 403 and returns false on failure.
func (s *Server) checkCompanyAccess(w http.ResponseWriter, r *http.Request, companyID string) bool {
	username := auth.UsernameFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())
	_, err := s.Store.GetPayrollCompanyForUser(username, role, companyID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return false
	}
	if errors.Is(err, store.ErrForbidden) {
		http.Error(w, "access denied", http.StatusForbidden)
		return false
	}
	return err == nil
}

// GET  /api/payroll/companies?status=active|all
// POST /api/payroll/companies
func (s *Server) handlePayrollCompanies(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())

	switch r.Method {
	case http.MethodGet:
		statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
		if statusFilter == "" {
			statusFilter = "active"
		}
		list := s.Store.ListPayrollCompanies(username, role, statusFilter)
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		if role == "user1" && s.Store.CountPayrollCompanies(username, role) >= 1 {
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
		created := s.Store.CreatePayrollCompany(username, c)
		writeJSON(w, http.StatusCreated, created)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// GET    /api/payroll/companies/{id}
// GET    /api/payroll/companies/{id}/summary
// PUT    /api/payroll/companies/{id}
// DELETE /api/payroll/companies/{id}
func (s *Server) handlePayrollCompanyByID(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())

	rest := strings.TrimPrefix(r.URL.Path, "/api/payroll/companies/")
	parts := strings.SplitN(rest, "/", 2)
	id := strings.TrimSpace(parts[0])
	if id == "" {
		http.NotFound(w, r)
		return
	}

	// Sub-route: /summary
	if len(parts) == 2 && parts[1] == "summary" && r.Method == http.MethodGet {
		if !s.checkCompanyAccess(w, r, id) {
			return
		}
		sum, err := s.Store.GetCompanySummary(id)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, sum)
		return
	}

	switch r.Method {
	case http.MethodGet:
		c, err := s.Store.GetPayrollCompanyForUser(username, role, id)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, store.ErrForbidden) {
			http.Error(w, "access denied", http.StatusForbidden)
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
		updated, err := s.Store.UpdatePayrollCompany(username, role, id, patch)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, store.ErrForbidden) {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := s.Store.DeletePayrollCompany(username, role, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, store.ErrForbidden) {
				http.Error(w, "access denied", http.StatusForbidden)
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
