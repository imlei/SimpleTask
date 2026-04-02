package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/models"
	"simpletask/internal/store"
)

func (s *Server) handleExpenseCodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		year := time.Now().Year()
		if y := strings.TrimSpace(r.URL.Query().Get("year")); y != "" {
			if n, err := strconv.Atoi(y); err == nil && n >= 1970 && n <= 2100 {
				year = n
			}
		}
		list := s.Store.ListExpenseCodeRows(year)
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var body models.ExpenseCodeRow
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !store.ValidExpenseCodeFormat(body.Code) {
			http.Error(w, "code must be 5XXX", http.StatusBadRequest)
			return
		}
		if err := s.Store.UpsertExpenseCode(body.Code, body.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"ok": "true"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExpenseCodesCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	list := s.Store.ListExpenseCatalogCodes()
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleExpenseCodeByCode(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/api/expense-codes/")
	if raw == "" {
		http.NotFound(w, r)
		return
	}
	code, err := url.PathUnescape(raw)
	if err != nil {
		code = raw
	}
	code = strings.TrimSpace(code)
	if !store.ValidExpenseCodeFormat(code) {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Store.UpsertExpenseCode(code, body.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
