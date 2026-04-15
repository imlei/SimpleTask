package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"simpletask/internal/mail"
	"simpletask/internal/models"
	"simpletask/internal/store"
)

type Server struct {
	Store   *store.Store
	Mail    *mail.Mailer
	BaseURL string
}

type priceSaveResponse struct {
	models.PriceItem
	SyncedPending int `json:"syncedPendingTasks,omitempty"`
}

var (
	emailRx = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	phoneRx = regexp.MustCompile(`^\+?\d{10,15}$`)
)

func validateCustomerContact(c models.Customer) error {
	email := strings.TrimSpace(c.Email)
	if email != "" && !emailRx.MatchString(email) {
		return errors.New("invalid email format")
	}
	phone := strings.TrimSpace(c.Phone)
	if phone != "" && !phoneRx.MatchString(phone) {
		return errors.New("invalid phone format")
	}
	if disp := strings.TrimSpace(c.DisplayName); utf8.RuneCountInString(disp) > 20 {
		return errors.New("displayName must be at most 20 characters")
	}
	return nil
}

// parsePagination 解析可选的 limit/offset 查询参数；不传则返回 0,0（表示不分页）
func parsePagination(r *http.Request) (limit, offset int) {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}

// paginateSlice 对切片做分页；limit=0 表示不分页
func paginateSlice[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}
	items = items[offset:]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.Store.ListTasks()
		limit, offset := parsePagination(r)
		if limit > 0 {
			writeJSON(w, http.StatusOK, paginateSlice(list, limit, offset))
		} else {
			writeJSON(w, http.StatusOK, list)
		}
	case http.MethodPost:
		var t models.Task
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(t.CustomerID) == "" {
			http.Error(w, "customerId is required", http.StatusBadRequest)
			return
		}
		if err := s.Store.RequireCustomerActive(t.CustomerID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "customer not found", http.StatusBadRequest)
				return
			}
			if errors.Is(err, store.ErrCustomerInactive) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
	case http.MethodGet:
		t, err := s.Store.GetTask(id)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodPut:
		var t models.Task
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(t.CustomerID) == "" {
			http.Error(w, "customerId is required", http.StatusBadRequest)
			return
		}
		existing, err := s.Store.GetTask(id)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(t.CustomerID) != strings.TrimSpace(existing.CustomerID) {
			if err := s.Store.RequireCustomerActive(t.CustomerID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					http.Error(w, "customer not found", http.StatusBadRequest)
					return
				}
				if errors.Is(err, store.ErrCustomerInactive) {
					http.Error(w, err.Error(), http.StatusForbidden)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		invoiceEdit := r.URL.Query().Get("invoiceEdit") == "1"
		updated, err := s.Store.UpdateTask(id, t, invoiceEdit)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, store.ErrTaskLocked) || errors.Is(err, store.ErrTaskPaidLocked) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := s.Store.DeleteTask(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			if errors.Is(err, store.ErrTaskDeleteLocked) {
				http.Error(w, err.Error(), http.StatusForbidden)
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
		resp := priceSaveResponse{PriceItem: created}
		if r.URL.Query().Get("syncPendingTasks") == "1" {
			n, err := s.Store.SyncPendingTasksForPriceID(created.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp.SyncedPending = n
		}
		writeJSON(w, http.StatusCreated, resp)
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
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := priceSaveResponse{PriceItem: updated}
		if r.URL.Query().Get("syncPendingTasks") == "1" {
			n, err := s.Store.SyncPendingTasksForPriceID(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp.SyncedPending = n
		}
		writeJSON(w, http.StatusOK, resp)
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

func (s *Server) handleInvoices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		list, err := s.Store.ListInvoices(status)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		limit, offset := parsePagination(r)
		if limit > 0 {
			writeJSON(w, http.StatusOK, paginateSlice(list, limit, offset))
		} else {
			writeJSON(w, http.StatusOK, list)
		}
	case http.MethodPost:
		var inv models.Invoice
		if err := json.NewDecoder(r.Body).Decode(&inv); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		created, err := s.Store.CreateInvoice(inv)
		if err != nil {
			if errors.Is(err, store.ErrCustomerInactive) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleInvoiceByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/invoices/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	// subroutes: /api/invoices/{id}, /api/invoices/{id}/send, /api/invoices/{id}/payment
	rest := strings.TrimPrefix(r.URL.Path, "/api/invoices/")
	parts := strings.Split(rest, "/")
	invoiceID := parts[0]
	if invoiceID == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 || parts[1] == "" {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		inv, err := s.Store.GetInvoice(invoiceID)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, inv)
		return
	}
	switch parts[1] {
	case "send":
		s.handleInvoiceSend(w, r, invoiceID)
		return
	case "payment":
		s.handleInvoicePayment(w, r, invoiceID)
		return
	default:
		http.NotFound(w, r)
		return
	}
}

func (s *Server) handleInvoiceSend(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mailer := s.effectiveMailer()
	if mailer == nil {
		http.Error(w, "mail not configured", http.StatusBadRequest)
		return
	}
	var body struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	to := strings.TrimSpace(body.To)
	if to == "" {
		http.Error(w, "missing to", http.StatusBadRequest)
		return
	}
	inv, err := s.Store.GetInvoice(id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	link := s.publicBaseURL(r)
	url := link + "/invoice.html?id=" + inv.ID
	subject := "Invoice " + inv.InvoiceNo
	html := "<p>Invoice: <strong>" + inv.InvoiceNo + "</strong></p>" +
		"<p>Amount: <strong>" + inv.Currency + " " + fmtMoney(inv.Total) + "</strong></p>" +
		"<p>View/Print: <a href=\"" + url + "\">" + url + "</a></p>"
	if err := mailer.SendInvoice(to, subject, html); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	updated, err := s.Store.MarkInvoiceSent(inv.ID, to, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleInvoicePayment(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Amount float64 `json:"amount"`
		Date   string  `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated, err := s.Store.AddInvoicePayment(id, body.Amount, strings.TrimSpace(body.Date))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func fmtMoney(v float64) string {
	// 简单格式化，避免引入额外依赖
	s := strconv.FormatFloat(v, 'f', 2, 64)
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// completedReportRow 报表行：在 Task 上附加 CAD 支出、利润、利润率（Revenue=Price1+Price2，与 Trend Profit 一致）
type completedReportRow struct {
	models.Task
	ExpenseCAD float64  `json:"expenseCad"`
	Profit     float64  `json:"profit"`
	MarginPct  *float64 `json:"marginPct,omitempty"` // Revenue>0 时为 (Profit/Revenue)*100；否则省略
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
	ids := make([]string, len(out))
	for i, t := range out {
		ids[i] = t.ID
	}
	expByTask := s.Store.SumExpensesCADByTaskIDs(ids)
	rows := make([]completedReportRow, len(out))
	for i, t := range out {
		rev := t.Price1 + t.Price2
		exp := expByTask[t.ID]
		p := rev - exp
		var mp *float64
		if rev > 0 {
			v := (p / rev) * 100
			mp = &v
		}
		rows[i] = completedReportRow{Task: t, ExpenseCAD: exp, Profit: p, MarginPct: mp}
	}
	writeJSON(w, http.StatusOK, rows)
}

// completedAt 为 YYYY-MM-DD（或至少前 7 位为 YYYY-MM）
func completedAtInMonth(completedAt, month string) bool {
	if len(completedAt) < 7 {
		return completedAt == month
	}
	return completedAt[:7] == month
}

func dateInMonth(dateStr, month string) bool {
	if len(dateStr) < 7 {
		return dateStr == month
	}
	return dateStr[:7] == month
}

type trendMonthlyPoint struct {
	Month     string  `json:"month"`
	TasksNew  int     `json:"tasksNew"`
	AmountNew float64 `json:"amountNew"`
}

// trendProfitTask 按「完成日期」落在所选月的非 Pending 任务盈利行
type trendProfitTask struct {
	TaskID       string  `json:"taskId"`
	CompanyName  string  `json:"companyName"`
	Revenue      float64 `json:"revenue"`
	ExpenseTotal float64 `json:"expenseTotal"`
	Profit       float64 `json:"profit"`
	CompletedAt  string  `json:"completedAt"`
}

type trendReport struct {
	Month                     string              `json:"month"`
	MonthlyAmountTotal        float64             `json:"monthlyAmountTotal"`
	MonthlyAmountDone         float64             `json:"monthlyAmountDone"`
	PendingAmountTotal        float64             `json:"pendingAmountTotal"`
	PendingAmountNewThisMonth float64             `json:"pendingAmountNewThisMonth"`
	MonthlySeries             []trendMonthlyPoint `json:"monthlySeries"`
	MonthlyInvoicedAmount     float64             `json:"monthlyInvoicedAmount"`
	ProfitTotal               float64             `json:"profitTotal"`
	ProfitRevenue             float64             `json:"profitRevenue"`
	ProfitExpenses            float64             `json:"profitExpenses"`
	ProfitTasks               []trendProfitTask   `json:"profitTasks"`
}

// GET /api/reports/trend?month=YYYY-MM
func (s *Server) handleReportTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		http.Error(w, "invalid month, use YYYY-MM", http.StatusBadRequest)
		return
	}

	allTasks := s.Store.ListTasks()
	// 生成最近 12 个月列表（含当前 month）
	months := make([]string, 12)
	base, _ := time.Parse("2006-01", month)
	for i := 11; i >= 0; i-- {
		m := base.AddDate(0, -(11 - i), 0)
		months[i] = m.Format("2006-01")
	}
	monthIdx := make(map[string]int, len(months))
	for i, m := range months {
		monthIdx[m] = i
	}
	series := make([]trendMonthlyPoint, len(months))
	for i, m := range months {
		series[i].Month = m
	}

	var (
		monthlyAmountTotal        float64
		monthlyAmountDone         float64
		pendingAmountTotal        float64
		pendingAmountNewThisMonth float64
	)
	for _, t := range allTasks {
		m := ""
		if len(t.Date) >= 7 {
			m = t.Date[:7]
		}
		if idx, ok := monthIdx[m]; ok {
			series[idx].TasksNew++
			series[idx].AmountNew += t.Price1
		}
		if dateInMonth(t.Date, month) {
			monthlyAmountTotal += t.Price1
			if t.Status != models.StatusPending {
				monthlyAmountDone += t.Price1
			}
		}
		if t.Status == models.StatusPending {
			pendingAmountTotal += t.Price1
			if dateInMonth(t.Date, month) {
				pendingAmountNewThisMonth += t.Price1
			}
		}
	}

	var profitTaskIDs []string
	for _, t := range allTasks {
		if t.Status == models.StatusPending {
			continue
		}
		if t.CompletedAt == "" || !completedAtInMonth(t.CompletedAt, month) {
			continue
		}
		profitTaskIDs = append(profitTaskIDs, t.ID)
	}
	expByTask := s.Store.SumExpensesCADByTaskIDs(profitTaskIDs)
	var profitTotal, profitRev, profitExp float64
	var profitRows []trendProfitTask
	for _, t := range allTasks {
		if t.Status == models.StatusPending {
			continue
		}
		if t.CompletedAt == "" || !completedAtInMonth(t.CompletedAt, month) {
			continue
		}
		rev := t.Price1 + t.Price2
		exp := expByTask[t.ID]
		p := rev - exp
		profitTotal += p
		profitRev += rev
		profitExp += exp
		profitRows = append(profitRows, trendProfitTask{
			TaskID:       t.ID,
			CompanyName:  t.CompanyName,
			Revenue:      rev,
			ExpenseTotal: exp,
			Profit:       p,
			CompletedAt:  t.CompletedAt,
		})
	}
	sort.Slice(profitRows, func(i, j int) bool {
		if profitRows[i].CompletedAt != profitRows[j].CompletedAt {
			return profitRows[i].CompletedAt > profitRows[j].CompletedAt
		}
		return profitRows[i].TaskID < profitRows[j].TaskID
	})

	invs, err := s.Store.ListInvoices("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var monthlyInvoicedAmount float64
	for _, inv := range invs {
		if !dateInMonth(inv.InvoiceDate, month) {
			continue
		}
		if strings.EqualFold(inv.Currency, string(models.CAD)) {
			monthlyInvoicedAmount += inv.Total
		}
	}

	resp := trendReport{
		Month:                     month,
		MonthlyAmountTotal:        monthlyAmountTotal,
		MonthlyAmountDone:         monthlyAmountDone,
		PendingAmountTotal:        pendingAmountTotal,
		PendingAmountNewThisMonth: pendingAmountNewThisMonth,
		MonthlySeries:             series,
		MonthlyInvoicedAmount:     monthlyInvoicedAmount,
		ProfitTotal:               profitTotal,
		ProfitRevenue:             profitRev,
		ProfitExpenses:            profitExp,
		ProfitTasks:               profitRows,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCustomers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.Store.ListCustomers()
		writeJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var c models.Customer
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(c.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if err := validateCustomerContact(c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		created := s.Store.CreateCustomer(c)
		writeJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCustomerByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/customers/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		c, err := s.Store.GetCustomer(id)
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
		var patch models.Customer
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(patch.Name) == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if err := validateCustomerContact(patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, err := s.Store.UpdateCustomer(id, patch)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func Register(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("/api/settings/public", s.handleSettingsPublic)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/bank-accounts/default/cheque-next", s.handleBankAccountDefaultChequeNext)
	mux.HandleFunc("/api/bank-accounts/default", s.handleBankAccountDefault)
	mux.HandleFunc("/api/bank-accounts/", s.handleBankAccountByID)
	mux.HandleFunc("/api/bank-accounts", s.handleBankAccounts)
	mux.HandleFunc("/api/customers/", s.handleCustomerByID)
	mux.HandleFunc("/api/customers", s.handleCustomers)
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/tasks/", s.handleTaskByID)
	mux.HandleFunc("/api/prices", s.handlePrices)
	mux.HandleFunc("/api/prices/", s.handlePriceByID)
	mux.HandleFunc("/api/reports/completed", s.handleReportCompleted)
	mux.HandleFunc("/api/reports/trend", s.handleReportTrend)
	mux.HandleFunc("/api/invoices", s.handleInvoices)
	mux.HandleFunc("/api/invoices/", s.handleInvoiceByID)
	mux.HandleFunc("/api/expense-codes/catalog", s.handleExpenseCodesCatalog)
	mux.HandleFunc("/api/expense-codes/", s.handleExpenseCodeByCode)
	mux.HandleFunc("/api/expense-codes", s.handleExpenseCodes)
	mux.HandleFunc("/api/expenses/", s.handleExpenseByID)
	mux.HandleFunc("/api/expenses", s.handleExpenses)
	mux.HandleFunc("/api/expense-vendors", s.handleExpenseVendors)
	mux.HandleFunc("/api/exchange-rates/convert", s.handleExchangeRatesConvert)
	mux.HandleFunc("/api/exchange-rates/currencies", s.handleExchangeRatesCurrencies)
	mux.HandleFunc("/api/exchange-rates", s.handleExchangeRates)
	mux.HandleFunc("/api/exchange-rate-codes/", s.handleExchangeRateCodeByCode)
	mux.HandleFunc("/api/exchange-rate-codes", s.handleExchangeRateCodes)
	mux.HandleFunc("/api/payroll/companies/", s.handlePayrollCompanyByID)
	mux.HandleFunc("/api/payroll/companies", s.handlePayrollCompanies)
	mux.HandleFunc("/api/payroll/employees/", s.handlePayrollEmployeeByID)
	mux.HandleFunc("/api/payroll/employees", s.handlePayrollEmployees)
	mux.HandleFunc("/api/payroll/init-entries", s.handleInitPeriodEntries)
	mux.HandleFunc("/api/payroll/periods/", s.handlePayrollPeriodByID)
	mux.HandleFunc("/api/payroll/periods", s.handlePayrollPeriods)
	mux.HandleFunc("/api/payroll/report/period-summary", s.handleReportPeriodSummary)
	mux.HandleFunc("/api/payroll/report/remittance", s.handleReportRemittance)
	mux.HandleFunc("/api/payroll/export/csv", s.handleExportCSV)
	mux.HandleFunc("/api/payroll/earnings-codes/", s.handleEarningsCodeByID)
	mux.HandleFunc("/api/payroll/earnings-codes", s.handleEarningsCodes)
	mux.HandleFunc("/api/payroll/company-rules", s.handleCompanyRules)
	mux.HandleFunc("/api/payroll/entries/", s.handleEntryEarnings)
}
