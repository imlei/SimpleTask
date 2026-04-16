package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/store"
)

// handleAdminPayrollSettings routes /api/admin/payroll-settings/*
func (s *Server) handleAdminPayrollSettings(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/payroll-settings/")

	switch {
	case rest == "rates" || rest == "rates/":
		s.handleAdminRates(w, r)
	case rest == "plans" || rest == "plans/":
		s.handleAdminPlans(w, r)
	default:
		http.NotFound(w, r)
	}
}

// ── Rates ──────────────────────────────────────────────────────────────────────

func (s *Server) handleAdminRates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		yearStr := r.URL.Query().Get("year")
		year := time.Now().Year()
		if y, err := strconv.Atoi(yearStr); err == nil && y > 2000 {
			year = y
		}
		// Try DB first, fall back to defaults
		existing, _ := s.Store.GetPayrollRateSetting(year)
		if existing != nil {
			writeJSON(w, http.StatusOK, existing)
			return
		}
		def := store.DefaultRateSetting(year)
		writeJSON(w, http.StatusOK, def)

	case http.MethodPut:
		var body store.PayrollRateSetting
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Year < 2020 || body.Year > 2100 {
			http.Error(w, "invalid year", http.StatusBadRequest)
			return
		}
		if err := s.Store.UpsertPayrollRateSetting(body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── Plans ──────────────────────────────────────────────────────────────────────

func (s *Server) handleAdminPlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.Store.ListPayrollPlans())

	case http.MethodPut:
		var body store.PayrollPlan
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.ID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		if err := s.Store.UpsertPayrollPlan(body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, s.Store.ListPayrollPlans())

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
