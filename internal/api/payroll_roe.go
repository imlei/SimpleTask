package api

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"simpletask/internal/auth"
	"simpletask/internal/models"
	"simpletask/internal/store"
)

// ─── Route handlers ───────────────────────────────────────────────────────────

// GET  /api/payroll/roe?company_id=PC0001
// POST /api/payroll/roe       { companyId, employeeId }
func (s *Server) handleROEs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.roeList(w, r)
	case http.MethodPost:
		s.roeGenerate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// /api/payroll/roe/{id}[/sub]
func (s *Server) handleROEByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/payroll/roe/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	if id == "" {
		http.Error(w, "roe id required", http.StatusBadRequest)
		return
	}

	switch sub {
	case "":
		if r.Method == http.MethodGet {
			s.roeGetDetail(w, r, id)
		} else if r.Method == http.MethodPatch {
			s.roeUpdate(w, r, id)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	case "validate":
		s.roeValidate(w, r, id)
	case "issue":
		s.roeIssue(w, r, id)
	case "regenerate":
		s.roeRegenerate(w, r, id)
	case "xml":
		s.roeXML(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

// ─── Business handlers ────────────────────────────────────────────────────────

func (s *Server) roeList(w http.ResponseWriter, r *http.Request) {
	companyID := r.URL.Query().Get("company_id")
	if companyID == "" {
		http.Error(w, "company_id required", http.StatusBadRequest)
		return
	}
	roes, err := s.Store.ListROEs(companyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if roes == nil {
		roes = []models.ROE{}
	}
	writeJSON(w, http.StatusOK, roes)
}

func (s *Server) roeGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CompanyID  string `json:"companyId"`
		EmployeeID string `json:"employeeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.CompanyID == "" || req.EmployeeID == "" {
		http.Error(w, "companyId and employeeId are required", http.StatusBadRequest)
		return
	}

	employee, err := s.Store.GetPayrollEmployee(req.EmployeeID)
	if err != nil {
		http.Error(w, "employee not found: "+err.Error(), http.StatusNotFound)
		return
	}
	if employee.CompanyID != req.CompanyID {
		http.Error(w, "employee does not belong to this company", http.StatusForbidden)
		return
	}

	username := auth.UsernameFromContext(r.Context())
	detail, err := s.Store.GenerateROEDraft(req.CompanyID, req.EmployeeID, username, employee)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (s *Server) roeGetDetail(w http.ResponseWriter, r *http.Request, id string) {
	detail, err := s.Store.GetROEDetail(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) roeUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var req store.ROEUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.Store.UpdateROEFields(id, req); err != nil {
		if strings.Contains(err.Error(), "issued") {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	detail, _ := s.Store.GetROEDetail(id)
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) roeValidate(w http.ResponseWriter, r *http.Request, id string) {
	errs := s.Store.ValidateROE(id)
	writeJSON(w, http.StatusOK, map[string]any{"validationErrors": errs})
}

func (s *Server) roeIssue(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// Pre-validate and return 422 with errors if invalid
	if errs := s.Store.ValidateROE(id); len(errs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"validationErrors": errs})
		return
	}
	username := auth.UsernameFromContext(r.Context())
	if err := s.Store.IssueROE(id, username); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	detail, _ := s.Store.GetROEDetail(id)
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) roeRegenerate(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	detail, err := s.Store.GetROEDetail(id)
	if err != nil {
		http.Error(w, "ROE not found", http.StatusNotFound)
		return
	}
	if detail.ROE.Status == models.ROEStatusIssued {
		http.Error(w, "issued ROE cannot be regenerated", http.StatusConflict)
		return
	}
	username := auth.UsernameFromContext(r.Context())
	newDetail, err := s.Store.GenerateROEDraft(detail.ROE.CompanyID, detail.ROE.EmployeeID, username, detail.Employee)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, newDetail)
}

// ─── ROE SAT XML export ───────────────────────────────────────────────────────

// roeXML renders Service Canada ROE SAT XML format for download.
func (s *Server) roeXML(w http.ResponseWriter, r *http.Request, id string) {
	detail, err := s.Store.GetROEDetail(id)
	if err != nil {
		http.Error(w, "ROE not found", http.StatusNotFound)
		return
	}

	xmlBytes, err := renderROESATXML(detail)
	if err != nil {
		http.Error(w, "XML generation error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("ROE_%s_%s.xml", detail.ROE.EmployeeID, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write(xmlBytes)
}

// ─── ROE SAT XML data types ───────────────────────────────────────────────────

type roeSATBatch struct {
	XMLName   xml.Name     `xml:"Batch"`
	ROERecord roeSATRecord `xml:"ROERecord"`
}

type roeSATRecord struct {
	// Block 10
	FirstDayWorked string `xml:"FirstDayWorked,omitempty"`
	// Block 11
	LastDayForWhichPaid string `xml:"LastDayForWhichPaid,omitempty"`
	// Block 12
	FinalPayPeriodEndingDate string `xml:"FinalPayPeriodEndingDate,omitempty"`
	// Block 13
	Occupation string `xml:"Occupation,omitempty"`
	// Block 14
	ExpectedDateOfRecall string `xml:"ExpectedDateOfRecall,omitempty"`
	// Block 15A
	TotalInsurableHours string `xml:"TotalInsurableHours,omitempty"`
	// Block 15B
	InsurableEarnings []roeSATEarningsBlock `xml:"InsurableEarnings>Period,omitempty"`
	// Block 15C
	TotalInsurableEarnings string `xml:"TotalInsurableEarnings,omitempty"`
	// Block 16
	ReasonForIssuingROE string `xml:"ReasonForIssuingROE,omitempty"`
	// Block 17A
	VacationPay string `xml:"VacationPay,omitempty"`
	// Block 18
	StatHolidayPay string `xml:"StatHolidayPay,omitempty"`
	// Block 19
	OtherMoneys []roeSATOtherMoney `xml:"OtherMoneys>OtherMoney,omitempty"`
	// Block 20
	Comments string `xml:"Comments,omitempty"`
	// Employee
	EmployeeSIN       string `xml:"EmployeeSIN,omitempty"`
	EmployeeFirstName string `xml:"EmployeeFirstName,omitempty"`
	EmployeeLastName  string `xml:"EmployeeLastName,omitempty"`
	// Employer
	EmployerPayrollAccountNumber string `xml:"EmployerPayrollAccountNumber,omitempty"`
	// Serial
	SerialNumber string `xml:"SerialNumber,omitempty"`
}

type roeSATEarningsBlock struct {
	PeriodNumber string `xml:"PeriodNumber,attr"`
	Amount       string `xml:"Amount"`
}

type roeSATOtherMoney struct {
	Type   string `xml:"Type"`
	Amount string `xml:"Amount"`
}

func renderROESATXML(detail models.ROEDetail) ([]byte, error) {
	roe := detail.ROE
	emp := detail.Employee
	company := detail.Company

	// Split employee name: last name, first name
	nameParts := strings.SplitN(strings.TrimSpace(emp.LegalName), " ", 2)
	firstName, lastName := "", ""
	if len(nameParts) == 2 {
		firstName = nameParts[0]
		lastName = nameParts[1]
	} else if len(nameParts) == 1 {
		lastName = nameParts[0]
	}

	// Build Block 15B periods
	var earningsBlocks []roeSATEarningsBlock
	for _, p := range roe.InsurableEarningsPeriods {
		earningsBlocks = append(earningsBlocks, roeSATEarningsBlock{
			PeriodNumber: fmt.Sprintf("%d", p.PeriodNo),
			Amount:       fmt.Sprintf("%.2f", p.InsurableEarnings),
		})
	}

	// Block 19 other moneys
	var otherMoneys []roeSATOtherMoney
	for _, om := range roe.OtherMoneys {
		otherMoneys = append(otherMoneys, roeSATOtherMoney{
			Type:   om.Type,
			Amount: fmt.Sprintf("%.2f", om.Amount),
		})
	}

	// Payroll account number: BN + RP0001
	pan := company.BusinessNumber
	if pan != "" && !strings.Contains(pan, "RP") {
		pan = pan + "RP0001"
	}

	// Expected recall date
	recallDate := ""
	if roe.RecallUnknown {
		recallDate = "Unknown"
	} else if roe.ExpectedRecallDate != "" {
		recallDate = roe.ExpectedRecallDate
	}

	record := roeSATRecord{
		FirstDayWorked:               roe.FirstDayWorked,
		LastDayForWhichPaid:          roe.LastDayPaid,
		FinalPayPeriodEndingDate:     roe.FinalPayPeriodEnd,
		Occupation:                   roe.Occupation,
		ExpectedDateOfRecall:         recallDate,
		TotalInsurableHours:          fmt.Sprintf("%.0f", roe.TotalInsurableHours),
		InsurableEarnings:            earningsBlocks,
		TotalInsurableEarnings:       fmt.Sprintf("%.2f", roe.TotalInsurableEarnings),
		ReasonForIssuingROE:          roe.ReasonCode,
		VacationPay:                  moneyStr(roe.VacationPay),
		StatHolidayPay:               moneyStr(roe.StatutoryHolidayPay),
		OtherMoneys:                  otherMoneys,
		Comments:                     roe.Comments,
		EmployeeSIN:                  emp.SINMasked,
		EmployeeFirstName:            firstName,
		EmployeeLastName:             lastName,
		EmployerPayrollAccountNumber: pan,
		SerialNumber:                 roe.SerialNumber,
	}

	batch := roeSATBatch{ROERecord: record}
	out, err := xml.MarshalIndent(batch, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func moneyStr(v float64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", v)
}
