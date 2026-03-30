package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"biztracker/internal/models"
	"biztracker/internal/store"
)

type Server struct {
	Store *store.Store
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.Store.ListTasks()
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var t models.Task
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		created := s.Store.CreateTask(t)
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var t models.Task
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, err := s.Store.UpdateTask(id, t)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := s.Store.DeleteTask(id); err != nil {
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

func (s *Server) handlePrices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.Store.ListPrices()
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var p models.PriceItem
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		created := s.Store.CreatePrice(p)
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePriceByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/prices/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var p models.PriceItem
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, err := s.Store.UpdatePrice(id, p)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := s.Store.DeletePrice(id); err != nil {
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// GET /api/reports/completed?month=YYYY-MM
// 返回该月内完成（Done 且 completedAt 落在该月）的任务，按完成日期升序。
func (s *Server) handleReportCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if _, err := time.Parse("2006-01", month); err != nil {
		http.Error(w, "invalid month, use YYYY-MM", http.StatusBadRequest)
		return
	}
	all := s.Store.ListTasks()
	out := make([]models.Task, 0, len(all))
	for _, t := range all {
		if t.Status != models.StatusDone || t.CompletedAt == "" {
			continue
		}
		if !completedAtInMonth(t.CompletedAt, month) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CompletedAt != out[j].CompletedAt {
			return out[i].CompletedAt < out[j].CompletedAt
		}
		return out[i].ID < out[j].ID
	})
	writeJSON(w, http.StatusOK, out)
}

// completedAt 为 YYYY-MM-DD（或至少前 7 位为 YYYY-MM）
func completedAtInMonth(completedAt, month string) bool {
	if len(completedAt) < 7 {
		return completedAt == month
	}
	return completedAt[:7] == month
}

func Register(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/tasks/", s.handleTaskByID)
	mux.HandleFunc("/api/prices", s.handlePrices)
	mux.HandleFunc("/api/prices/", s.handlePriceByID)
	mux.HandleFunc("/api/reports/completed", s.handleReportCompleted)
}
