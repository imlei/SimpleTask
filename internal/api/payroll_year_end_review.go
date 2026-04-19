package api

import (
	"net/http"
	"strconv"
	"time"
)

// GET /api/payroll/year-end-review?company_id=PC0001&year=2025
func (s *Server) handleYearEndReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		http.Error(w, "company_id required", http.StatusBadRequest)
		return
	}
	yearStr := r.URL.Query().Get("year")
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 || year > 2100 {
		year = time.Now().Year() - 1 // default to prior year
	}
	report := s.Store.RunYearEndReview(companyID, year)
	writeJSON(w, http.StatusOK, report)
}
