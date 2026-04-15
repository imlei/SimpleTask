package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"simpletask/internal/models"
	"simpletask/internal/store"
)

// ── Earnings Codes (GET/POST /api/payroll/earnings-codes) ───────────────────────

func (s *Server) handleEarningsCodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		companyID := strings.TrimSpace(r.URL.Query().Get("company_id"))
		if companyID == "" {
			http.Error(w, "company_id required", http.StatusBadRequest)
			return
		}
		// Auto-seed system codes if company has none yet
		s.Store.EnsureSystemCodes(companyID)
		list := s.Store.ListEarningsCodes(companyID)
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var c models.PayrollEarningsCode
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(c.CompanyID) == "" {
			http.Error(w, "companyId required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(c.Name) == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(c.Code) == "" {
			// Auto-derive code from name: uppercase first word
			words := strings.Fields(strings.ToUpper(c.Name))
			if len(words) > 0 {
				c.Code = words[0]
			}
		}
		created, err := s.Store.CreateEarningsCode(c)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ── Earnings Code by ID (PUT/DELETE /api/payroll/earnings-codes/{id}) ─────────

func (s *Server) handleEarningsCodeByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/payroll/earnings-codes/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		c, err := s.Store.GetEarningsCode(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, c)

	case http.MethodPut:
		var c models.PayrollEarningsCode
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(c.Name) == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		updated, err := s.Store.UpdateEarningsCode(id, c)
		if err != nil {
			if err == store.ErrNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := s.Store.DeleteEarningsCode(id); err != nil {
			if err == store.ErrNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ── Company Rules (GET/PUT /api/payroll/company-rules) ────────────────────────

func (s *Server) handleCompanyRules(w http.ResponseWriter, r *http.Request) {
	companyID := strings.TrimSpace(r.URL.Query().Get("company_id"))
	if companyID == "" {
		http.Error(w, "company_id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rules := s.Store.GetCompanyRules(companyID)
		writeJSON(w, http.StatusOK, rules)
	case http.MethodPut:
		var rules models.PayrollCompanyRules
		if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rules.CompanyID = companyID
		if rules.VacationRate < 0 || rules.VacationRate > 0.25 {
			http.Error(w, "vacationRate must be between 0 and 0.25", http.StatusBadRequest)
			return
		}
		saved, err := s.Store.UpsertCompanyRules(rules)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ── Entry Earnings lines (GET/PUT /api/payroll/entries/{id}/earnings) ─────────

func (s *Server) handleEntryEarnings(w http.ResponseWriter, r *http.Request) {
	// Path: /api/payroll/entries/{entryID}/earnings
	rest := strings.TrimPrefix(r.URL.Path, "/api/payroll/entries/")
	parts := strings.SplitN(rest, "/", 2)
	entryID := strings.TrimSpace(parts[0])
	if entryID == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		list := s.Store.ListEntryEarnings(entryID)
		writeJSON(w, http.StatusOK, list)

	case http.MethodPut:
		var lines []models.PayrollEntryEarning
		if err := json.NewDecoder(r.Body).Decode(&lines); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Validate amounts
		for i, l := range lines {
			if strings.TrimSpace(l.EarningsCodeID) == "" {
				http.Error(w, "earningsCodeId required on each line", http.StatusBadRequest)
				return
			}
			if l.Amount < 0 {
				http.Error(w, "amount must be ≥ 0", http.StatusBadRequest)
				return
			}
			lines[i].EntryID = entryID
		}
		saved, err := s.Store.ReplaceEntryEarnings(entryID, lines)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Also update the entry's gross_pay to reflect the new totals.
		// We need to re-read the base gross (stored as gross_pay before extras were added)
		// and recompute. To do this simply, we rely on the Calculate step to recompute.
		// Here we just return the saved lines so the UI can display them.
		writeJSON(w, http.StatusOK, saved)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
