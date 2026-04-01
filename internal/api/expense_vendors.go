package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"tasktracker/internal/models"
)

func (s *Server) handleExpenseVendors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.Store.ListExpenseVendors()
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var v models.ExpenseVendor
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		v.Name = strings.TrimSpace(v.Name)
		if v.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		created, err := s.Store.CreateExpenseVendor(v)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
