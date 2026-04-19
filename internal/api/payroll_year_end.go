package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/auth"
	"simpletask/internal/models"
	"simpletask/internal/store"
)

// ── Route handlers ────────────────────────────────────────────────────────────

// GET  /api/payroll/year-end?company_id=PC0001
// POST /api/payroll/year-end   { companyId, payrollAccountNumber, calendarYear }
func (s *Server) handleYearEndReturns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.yearEndList(w, r)
	case http.MethodPost:
		s.yearEndGenerate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// /api/payroll/year-end/{id}[/sub]
func (s *Server) handleYearEndReturnByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/payroll/year-end/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	if id == "" {
		http.Error(w, "return id required", http.StatusBadRequest)
		return
	}

	ret, err := s.Store.GetYearEndReturn(id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !s.checkCompanyAccess(w, r, ret.CompanyID) {
		return
	}

	switch sub {
	case "":
		s.yearEndGetDetail(w, r, id)
	case "validate":
		s.yearEndValidate(w, r, id)
	case "finalize":
		s.yearEndFinalize(w, r, id)
	case "regenerate":
		s.yearEndRegenerate(w, r, id, ret.CompanyID)
	case "preview":
		s.yearEndHTMLPreview(w, r, id, false)
	case "pdf":
		s.yearEndHTMLPreview(w, r, id, true)
	case "contact":
		s.yearEndUpdateContact(w, r, id)
	default:
		if strings.HasPrefix(sub, "slips/") {
			slipID := strings.TrimPrefix(sub, "slips/")
			s.yearEndGetSlip(w, r, id, slipID)
			return
		}
		http.NotFound(w, r)
	}
}

// ── GET list ─────────────────────────────────────────────────────────────────

func (s *Server) yearEndList(w http.ResponseWriter, r *http.Request) {
	companyID := strings.TrimSpace(r.URL.Query().Get("company_id"))
	if companyID == "" {
		http.Error(w, "company_id is required", http.StatusBadRequest)
		return
	}
	if !s.checkCompanyAccess(w, r, companyID) {
		return
	}
	writeJSON(w, http.StatusOK, s.Store.ListYearEndReturns(companyID))
}

// ── POST generate draft ───────────────────────────────────────────────────────

func (s *Server) yearEndGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CompanyID            string `json:"companyId"`
		PayrollAccountNumber string `json:"payrollAccountNumber"`
		CalendarYear         int    `json:"calendarYear"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.CompanyID = strings.TrimSpace(req.CompanyID)
	req.PayrollAccountNumber = strings.TrimSpace(req.PayrollAccountNumber)

	if req.CompanyID == "" {
		http.Error(w, "companyId is required", http.StatusBadRequest)
		return
	}
	if !s.checkCompanyAccess(w, r, req.CompanyID) {
		return
	}
	if req.CalendarYear < 2000 || req.CalendarYear > time.Now().Year()+1 {
		http.Error(w, "calendarYear must be a valid year (2000 to present+1)", http.StatusBadRequest)
		return
	}

	// Derive default payroll account number if omitted: BN + "RP0001"
	if req.PayrollAccountNumber == "" {
		company, err := s.Store.GetPayrollCompany(req.CompanyID)
		if err != nil {
			http.Error(w, "company not found", http.StatusNotFound)
			return
		}
		bn := strings.ReplaceAll(strings.TrimSpace(company.BusinessNumber), " ", "")
		if bn == "" {
			http.Error(w, "company has no business number; set it in company settings before generating T4s", http.StatusUnprocessableEntity)
			return
		}
		req.PayrollAccountNumber = bn + "RP0001"
	}

	if !s.Store.HasFinalizedPeriodsForYear(req.CompanyID, req.CalendarYear) {
		http.Error(w, fmt.Sprintf(
			"no finalized payroll periods found for %d; finalize at least one period before generating T4s",
			req.CalendarYear,
		), http.StatusUnprocessableEntity)
		return
	}

	employees := s.loadEmployeeMap(req.CompanyID)
	username := auth.UsernameFromContext(r.Context())
	detail, err := s.Store.GenerateOrRegenerateDraft(
		req.CompanyID, req.PayrollAccountNumber, req.CalendarYear,
		username, employees,
	)
	if err != nil {
		if strings.Contains(err.Error(), "already finalized") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

// ── GET detail ────────────────────────────────────────────────────────────────

func (s *Server) yearEndGetDetail(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	detail, err := s.Store.GetYearEndReturnDetail(id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	detail.ValidationErrors = s.Store.ValidateYearEndReturn(id)
	writeJSON(w, http.StatusOK, detail)
}

// ── GET slip ──────────────────────────────────────────────────────────────────

func (s *Server) yearEndGetSlip(w http.ResponseWriter, r *http.Request, returnID, slipID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sl, err := s.Store.GetT4Slip(slipID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if sl.YearEndReturnID != returnID {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, sl)
}

// ── POST/GET validate ─────────────────────────────────────────────────────────

func (s *Server) yearEndValidate(w http.ResponseWriter, r *http.Request, id string) {
	errs := s.Store.ValidateYearEndReturn(id)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":  len(errs) == 0,
		"errors": errs,
	})
}

// ── POST finalize ─────────────────────────────────────────────────────────────

func (s *Server) yearEndFinalize(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	errs := s.Store.ValidateYearEndReturn(id)
	if len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
			"error":  "validation failed; correct errors before finalizing",
			"errors": errs,
		})
		return
	}
	username := auth.UsernameFromContext(r.Context())
	if err := s.Store.FinalizeYearEndReturn(id, username); err != nil {
		if strings.Contains(err.Error(), "already finalized") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	detail, _ := s.Store.GetYearEndReturnDetail(id)
	writeJSON(w, http.StatusOK, detail)
}

// ── POST regenerate ───────────────────────────────────────────────────────────

func (s *Server) yearEndRegenerate(w http.ResponseWriter, r *http.Request, id, companyID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ret, err := s.Store.GetYearEndReturn(id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if ret.Status == models.YearEndStatusFinalized {
		http.Error(w, "return is finalized; regeneration not allowed (Phase 2: use amend workflow)", http.StatusConflict)
		return
	}
	employees := s.loadEmployeeMap(companyID)
	username := auth.UsernameFromContext(r.Context())
	detail, err := s.Store.GenerateOrRegenerateDraft(
		companyID, ret.PayrollAccountNumber, ret.CalendarYear,
		username, employees,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// ── PATCH/PUT contact ─────────────────────────────────────────────────────────

func (s *Server) yearEndUpdateContact(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ContactName  string `json:"contactName"`
		ContactPhone string `json:"contactPhone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.Store.UpdateT4SummaryContact(id, req.ContactName, req.ContactPhone); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	detail, _ := s.Store.GetYearEndReturnDetail(id)
	writeJSON(w, http.StatusOK, detail.Summary)
}

// ── HTML preview / PDF export ─────────────────────────────────────────────────

func (s *Server) yearEndHTMLPreview(w http.ResponseWriter, r *http.Request, id string, asPDF bool) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	detail, err := s.Store.GetYearEndReturnDetail(id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	company, _ := s.Store.GetPayrollCompany(detail.Return.CompanyID)

	data := t4PreviewData{
		Company:     company,
		Return:      detail.Return,
		Summary:     detail.Summary,
		Slips:       detail.Slips,
		AuditLogs:   detail.AuditLogs,
		Year:        strconv.Itoa(detail.Return.CalendarYear),
		IsFinalized: detail.Return.Status == models.YearEndStatusFinalized,
	}

	var buf bytes.Buffer
	if err := t4PreviewTmpl.Execute(&buf, data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if asPDF {
		fname := fmt.Sprintf("T4-%d-%s.html", detail.Return.CalendarYear, detail.Return.CompanyID)
		w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`"`)
	}
	_, _ = w.Write(buf.Bytes())
}

// ── Template ──────────────────────────────────────────────────────────────────

type t4PreviewData struct {
	Company    models.PayrollCompany
	Return     models.YearEndReturn
	Summary    models.T4Summary
	Slips      []models.T4Slip
	AuditLogs  []models.YearEndAuditLog
	Year       string
	IsFinalized bool
}

var t4PreviewTmpl *template.Template

func init() {
	t4PreviewTmpl = template.Must(template.New("t4preview").Funcs(template.FuncMap{
		"money": func(v float64) string {
			if v == 0 {
				return "–"
			}
			return fmt.Sprintf("%.2f", v)
		},
		"dollars": func(v float64) string { return fmt.Sprintf("%.2f", v) },
		"add":     func(a, b int) int { return a + b },
		"atoi":    func(s string) int { n, _ := strconv.Atoi(s); return n },
	}).Parse(t4HTMLTemplate))
}

const t4HTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>T4 {{.Year}} – {{.Company.LegalName}}</title>
<style>
*, *::before, *::after { box-sizing: border-box; }
body { font-family: Arial, Helvetica, sans-serif; font-size: 10pt; margin: 0; color: #111; background: #fff; }
.toolbar { background: #f5f5f5; padding: 10px 20px; border-bottom: 2px solid #1d4ed8; display: flex; align-items: center; gap: 12px; }
.toolbar button { padding: 7px 16px; font-size: 10pt; cursor: pointer; background: #1d4ed8; color: #fff; border: none; border-radius: 4px; }
.toolbar button:hover { background: #1e40af; }
.pill { padding: 3px 10px; border-radius: 10px; font-size: 8.5pt; font-weight: bold; }
.pill-draft { background: #fef9c3; color: #713f12; }
.pill-finalized { background: #dcfce7; color: #166534; }
.page { max-width: 820px; margin: 0 auto; padding: 20px 28px; }
h1 { font-size: 15pt; margin: 0 0 6px; }
h2 { font-size: 12pt; margin: 20px 0 8px; border-bottom: 2px solid #1d4ed8; padding-bottom: 4px; color: #1d4ed8; }
.info-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px 20px; margin-bottom: 14px; font-size: 9pt; }
.info-grid .lbl { color: #666; font-size: 8pt; }
.notice { background: #fff7ed; border-left: 4px solid #f97316; padding: 8px 14px; font-size: 8.5pt; color: #7c2d12; margin-bottom: 14px; border-radius: 0 4px 4px 0; }
/* T4 slip */
.slip { border: 1.5px solid #374151; margin-bottom: 20px; page-break-inside: avoid; }
.slip-head { background: #1d4ed8; color: #fff; padding: 5px 10px; font-size: 9.5pt; font-weight: bold; display: flex; justify-content: space-between; }
.slip-emp { background: #eff6ff; border-bottom: 1px solid #bfdbfe; padding: 5px 10px; font-size: 9pt; }
.boxes { display: grid; grid-template-columns: repeat(4, 1fr); }
.bx { padding: 5px 8px; border-right: 1px solid #d1d5db; border-bottom: 1px solid #d1d5db; min-height: 42px; }
.bx:nth-child(4n) { border-right: none; }
.bx-num { font-size: 7pt; color: #6b7280; display: block; }
.bx-name { font-size: 7pt; color: #4b5563; display: block; margin-bottom: 2px; line-height: 1.2; }
.bx-val { font-size: 10.5pt; font-weight: bold; }
.bx-val-zero { color: #9ca3af; }
.bx-pending { background: #fff7ed; }
.bx-pending .bx-val { color: #9a3412; font-size: 7.5pt; font-weight: normal; font-style: italic; }
/* Summary table */
.sum-tbl { width: 100%; border-collapse: collapse; font-size: 9.5pt; }
.sum-tbl th { background: #1d4ed8; color: #fff; padding: 5px 10px; text-align: left; }
.sum-tbl th.r { text-align: right; }
.sum-tbl td { padding: 4px 10px; border-bottom: 1px solid #e5e7eb; }
.sum-tbl td.r { text-align: right; font-family: 'Courier New', monospace; }
.sum-tbl td.ln { color: #6b7280; font-size: 8pt; width: 48px; }
.sum-tbl tr.total { font-weight: bold; background: #f1f5f9; }
.audit-tbl { width: 100%; border-collapse: collapse; font-size: 8pt; margin-top: 6px; }
.audit-tbl td { padding: 3px 8px; border-bottom: 1px solid #f3f4f6; }
.footer { font-size: 8pt; color: #6b7280; margin-top: 20px; border-top: 1px solid #e5e7eb; padding-top: 8px; }
@media print {
  .toolbar { display: none !important; }
  .page { max-width: 100%; padding: 10px 14px; }
  .slip { page-break-inside: avoid; }
  h2 { color: #000; border-color: #000; }
}
</style>
</head>
<body>
<div class="toolbar">
  <button onclick="window.print()">Print / Save as PDF</button>
  <span class="pill {{if .IsFinalized}}pill-finalized{{else}}pill-draft{{end}}">{{.Return.Status}}</span>
  <span style="font-size:9pt;color:#374151;">Return {{.Return.ID}} &bull; Tax Year {{.Year}} &bull; {{.Return.PayrollAccountNumber}}</span>
</div>
<div class="page">

<h1>T4 Information Return &ndash; {{.Year}}</h1>
<div class="info-grid">
  <div><div class="lbl">Employer (Legal Name)</div>{{.Company.LegalName}}</div>
  <div><div class="lbl">Payroll Account Number (Box 54)</div>{{.Return.PayrollAccountNumber}}</div>
  <div><div class="lbl">Address</div>{{.Company.Address}}{{if .Company.City}}, {{.Company.City}}{{end}}{{if .Company.Province}} {{.Company.Province}}{{end}}{{if .Company.PostalCode}} {{.Company.PostalCode}}{{end}}</div>
  <div><div class="lbl">Status</div>{{.Return.Status}}{{if .Return.FinalizedAt}} &ndash; finalized {{.Return.FinalizedAt}}{{end}}</div>
</div>

<div class="notice">
  <strong>Phase 1 notice:</strong> The following boxes are not yet mapped from payroll data and will show &ldquo;pending&rdquo;:
  Box 20 (RPP Contributions), Box 44 (Union Dues), Box 45 (Dental Benefit Code), Box 46 (Charitable Donations), Box 52 (Pension Adjustment).
  Where detailed earnings line-items are unavailable, Box 24 (EI Insurable Earnings) and Box 26 (CPP Pensionable Earnings) are approximated from gross pay.
  These must be reviewed and corrected before CRA submission.
</div>

<h2>T4 Slips &mdash; {{len .Slips}} employee(s)</h2>

{{range .Slips}}
<div class="slip">
  <div class="slip-head">
    <span>{{.EmployeeLegalName}}</span>
    <span style="font-weight:normal;font-size:8.5pt;">Employee ID: {{.EmployeeID}}</span>
  </div>
  <div class="slip-emp">
    <strong>SIN (Box 12):</strong> {{.EmployeeSINMasked}} &nbsp;|&nbsp;
    <strong>Province of Employment (Box 10):</strong> {{if .EmployeeProvince}}{{.EmployeeProvince}}{{else}}<span style="color:#dc2626">MISSING</span>{{end}} &nbsp;|&nbsp;
    <strong>Address:</strong> {{.EmployeeAddress}}
  </div>
  <div class="boxes">
    <div class="bx"><span class="bx-num">Box 14</span><span class="bx-name">Employment Income</span><span class="bx-val">{{money .Box14EmploymentIncome}}</span></div>
    <div class="bx"><span class="bx-num">Box 16</span><span class="bx-name">Employee CPP Contributions</span><span class="bx-val">{{money .Box16CPPEmployee}}</span></div>
    <div class="bx"><span class="bx-num">Box 16A</span><span class="bx-name">Employee CPP2 Contributions</span><span class="bx-val">{{money .Box16ACpp2Employee}}</span></div>
    <div class="bx"><span class="bx-num">Box 17</span><span class="bx-name">Employee QPP Contributions</span><span class="bx-val">{{money .Box17QPPEmployee}}</span></div>
    <div class="bx"><span class="bx-num">Box 18</span><span class="bx-name">Employee EI Premiums</span><span class="bx-val">{{money .Box18EIEmployee}}</span></div>
    <div class="bx bx-pending"><span class="bx-num">Box 20</span><span class="bx-name">RPP Contributions</span><span class="bx-val">pending mapping</span></div>
    <div class="bx"><span class="bx-num">Box 22</span><span class="bx-name">Income Tax Deducted</span><span class="bx-val">{{money .Box22IncomeTaxDeducted}}</span></div>
    <div class="bx"><span class="bx-num">Box 24</span><span class="bx-name">EI Insurable Earnings</span><span class="bx-val">{{money .Box24EIInsurableEarnings}}</span></div>
    <div class="bx"><span class="bx-num">Box 26</span><span class="bx-name">CPP/QPP Pensionable Earnings</span><span class="bx-val">{{money .Box26CPPPensionableEarnings}}</span></div>
    <div class="bx bx-pending"><span class="bx-num">Box 44</span><span class="bx-name">Union Dues</span><span class="bx-val">pending mapping</span></div>
    <div class="bx bx-pending"><span class="bx-num">Box 45</span><span class="bx-name">Dental Benefit Code</span><span class="bx-val">pending mapping</span></div>
    <div class="bx bx-pending"><span class="bx-num">Box 46</span><span class="bx-name">Charitable Donations</span><span class="bx-val">pending mapping</span></div>
    <div class="bx bx-pending"><span class="bx-num">Box 52</span><span class="bx-name">Pension Adjustment</span><span class="bx-val">pending mapping</span></div>
    {{range .OtherInfo}}
    <div class="bx"><span class="bx-num">Code {{.Code}}</span><span class="bx-name">Other Information</span><span class="bx-val">{{money .Amount}}</span></div>
    {{end}}
  </div>
</div>
{{end}}

<h2>T4 Summary</h2>
<table class="sum-tbl">
  <thead><tr><th class="ln">Line</th><th>Description</th><th class="r">Amount ($)</th></tr></thead>
  <tbody>
    <tr><td class="ln">14</td><td>Total Employment Income</td><td class="r">{{dollars .Summary.TotalEmploymentIncome}}</td></tr>
    <tr><td class="ln">16</td><td>Total Employee CPP Contributions</td><td class="r">{{dollars .Summary.TotalCPPEmployee}}</td></tr>
    <tr><td class="ln">16A</td><td>Total Employee CPP2 Contributions</td><td class="r">{{dollars .Summary.TotalCPP2Employee}}</td></tr>
    <tr><td class="ln">17</td><td>Total Employee QPP Contributions</td><td class="r">{{dollars .Summary.TotalQPPEmployee}}</td></tr>
    <tr><td class="ln">18</td><td>Total Employee EI Premiums</td><td class="r">{{dollars .Summary.TotalEIEmployee}}</td></tr>
    <tr><td class="ln">19</td><td>Total Employer EI Premiums</td><td class="r">{{dollars .Summary.TotalEIEmployer}}</td></tr>
    <tr><td class="ln">22</td><td>Total Income Tax Deducted</td><td class="r">{{dollars .Summary.TotalIncomeTaxDeducted}}</td></tr>
    <tr><td class="ln">27</td><td>Total Employer CPP Contributions</td><td class="r">{{dollars .Summary.TotalCPPEmployer}}</td></tr>
    <tr><td class="ln">27A</td><td>Total Employer CPP2 Contributions</td><td class="r">{{dollars .Summary.TotalCPP2Employer}}</td></tr>
    <tr><td class="ln">52</td><td>Total Pension Adjustments</td><td class="r">{{dollars .Summary.TotalPensionAdjustments}}</td></tr>
    <tr class="total"><td class="ln">88</td><td>Total Number of T4 Slips</td><td class="r">{{.Summary.SlipCount}}</td></tr>
  </tbody>
</table>

{{if .Summary.ContactName}}
<p style="font-size:9pt;margin-top:10px;">
  <strong>Contact (Line 76/78):</strong> {{.Summary.ContactName}}{{if .Summary.ContactPhone}} &bull; {{.Summary.ContactPhone}}{{end}}
</p>
{{end}}

<h2>Audit Log</h2>
{{if .AuditLogs}}
<table class="audit-tbl">
  <tbody>
    {{range .AuditLogs}}
    <tr><td>{{.CreatedAt}}</td><td><strong>{{.Action}}</strong></td><td>{{.ActorUsername}}</td><td>{{.Note}}</td></tr>
    {{end}}
  </tbody>
</table>
{{end}}

<div class="footer">
  CRA filing deadline: on or before the last day of February {{add (atoi .Year) 1}} (for {{.Year}} taxation year). &bull;
  Employers with 50 or more T4 slips must file electronically (Internet File Transfer). &bull;
  Generated: {{.Return.GeneratedAt}} &bull;
  Source hash: <code style="font-size:7.5pt;">{{.Return.SourceHash}}</code>
</div>

</div>
</body>
</html>`

// loadEmployeeMap returns all employees for a company keyed by employee_id.
func (s *Server) loadEmployeeMap(companyID string) map[string]models.PayrollEmployee {
	list := s.Store.ListPayrollEmployees(companyID, "all")
	m := make(map[string]models.PayrollEmployee, len(list))
	for _, e := range list {
		m[e.ID] = e
	}
	return m
}
