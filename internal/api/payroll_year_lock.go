package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/auth"
)

// GET  /api/payroll/year-locks?company=&year=   → lock status for one year (or list all)
// POST /api/payroll/year-locks                  → lock a year  { companyId, year }
// DELETE /api/payroll/year-locks/{year}?company= → unlock a year
func (s *Server) handleYearLocks(w http.ResponseWriter, r *http.Request) {
	companyID := strings.TrimSpace(r.URL.Query().Get("company"))
	if companyID == "" {
		http.Error(w, "company is required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		yearStr := strings.TrimSpace(r.URL.Query().Get("year"))
		if yearStr != "" {
			year, err := strconv.Atoi(yearStr)
			if err != nil || year < 2000 {
				http.Error(w, "invalid year", http.StatusBadRequest)
				return
			}
			lock := s.Store.GetYearLock(companyID, year)
			if lock == nil {
				writeJSON(w, http.StatusOK, map[string]any{"locked": false, "year": year, "companyId": companyID})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"locked": true, "lock": lock})
			return
		}
		locks := s.Store.ListYearLocks(companyID)
		if locks == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		writeJSON(w, http.StatusOK, locks)

	case http.MethodPost:
		var body struct {
			CompanyID string `json:"companyId"`
			Year      int    `json:"year"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.CompanyID == "" {
			body.CompanyID = companyID
		}
		if body.CompanyID != companyID {
			http.Error(w, "companyId mismatch", http.StatusBadRequest)
			return
		}
		if body.Year < 2000 || body.Year > time.Now().Year() {
			http.Error(w, "invalid year", http.StatusBadRequest)
			return
		}
		lock, err := s.Store.LockYear(body.CompanyID, body.Year, currentUsername(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, lock)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleYearLockByYear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	companyID := strings.TrimSpace(r.URL.Query().Get("company"))
	if companyID == "" {
		http.Error(w, "company is required", http.StatusBadRequest)
		return
	}
	yearStr := strings.TrimPrefix(r.URL.Path, "/api/payroll/year-locks/")
	yearStr = strings.TrimSpace(yearStr)
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	if err := s.Store.UnlockYear(companyID, year); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// currentUsername reads the authenticated username from request context.
func currentUsername(r *http.Request) string {
	if u := auth.UsernameFromContext(r.Context()); u != "" {
		return u
	}
	return "system"
}
