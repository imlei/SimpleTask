package models

// CompanyInfo holds the display info for a payroll company used in cheque rendering.
type CompanyInfo struct {
	Name     string
	Street   string
	City     string
	Province string
	Postal   string
}

// TaskStatus 任务状态（避免手写拼写错误，如 Penging）
type TaskStatus string

const (
	StatusPending  TaskStatus = "Pending"
	StatusDone     TaskStatus = "Done"
	StatusInvoiced TaskStatus = "Invoiced" // invoice created, not yet sent
	StatusSent     TaskStatus = "Sent"
	StatusPaid     TaskStatus = "Paid"
)

// Customer 客户（任务从客户中选择；公司名为任务上的具体名称）
type Customer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"` // 列表/下拉等处优先显示；空则用 Name；最多 20 字符（UTF-8 码点）
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Address     string `json:"address"`
	Status      string `json:"status"` // active | inactive；inactive 时不可新建任务/发票
}

// Task 对应任务表中的一行
type Task struct {
	ID               string     `json:"id"`
	CustomerID       string     `json:"customerId"`
	CustomerName     string     `json:"customerName,omitempty"` // 列表等处展示用：customers.display_name 非空则用之，否则 name
	CustomerStatus   string     `json:"customerStatus,omitempty"` // 来自 customers.status，用于前端过滤 inactive
	CompanyName      string     `json:"companyName"`            // 公司名（隶属于所选 Customer）
	Date             string     `json:"date"` // ISO 日期字符串，如 2026-03-30
	Service1         string     `json:"service1"`
	Service2         string     `json:"service2"`
	Price1           float64    `json:"price1"`
	Price2           float64    `json:"price2"`
	SelectedPriceIDs []string   `json:"selectedPriceIds,omitempty"` // 从价目表多选的服务项 ID
	Status           TaskStatus `json:"status"`
	CompletedAt      string     `json:"completedAt,omitempty"` // 标记为 Done 时的日期 YYYY-MM-DD
	Note             string     `json:"note"`
}

// Currency 价目表货币
type Currency string

const (
	CNY Currency = "CNY" // 元
	CAD Currency = "CAD" // 加币
	USD Currency = "USD" // 刀
)

// PriceItem 价目表条目
type PriceItem struct {
	ID          string   `json:"id"`
	ServiceName string   `json:"serviceName"`
	Amount      *float64 `json:"amount,omitempty"` // nil 表示未定价
	Currency    Currency `json:"currency"`
	Note        string   `json:"note"` // 如「起」、说明
}

// InvoiceItem 发票明细行
type InvoiceItem struct {
	Description string  `json:"description"`
	Detail      string  `json:"detail"`
	TaxLabel    string  `json:"taxLabel"` // 如 Zero-rated / GST @ 5%
	Qty         float64 `json:"qty"`
	Rate        float64 `json:"rate"`
	Amount      float64 `json:"amount"`
}

// Invoice 发票
type Invoice struct {
	ID          string        `json:"id"`
	InvoiceNo   string        `json:"invoiceNo"`
	TaskID      string        `json:"taskId"`
	// TaskIDs 与 taskId 一致：首项为主 task_id；合并开票时含全部任务 ID（持久化在 task_ids_json）
	TaskIDs     []string      `json:"taskIds,omitempty"`
	InvoiceDate string        `json:"invoiceDate"`
	Terms       string        `json:"terms"`
	DueDate     string        `json:"dueDate"`
	BillToName  string        `json:"billToName"`
	BillToAddr  string        `json:"billToAddr"`
	BillToEmail string        `json:"billToEmail"`
	ShipToName  string        `json:"shipToName"`
	ShipToAddr  string        `json:"shipToAddr"`
	Currency    string        `json:"currency"` // CNY/CAD/USD
	TaxRate     float64       `json:"taxRate"`  // 例如 0, 5
	Items       []InvoiceItem `json:"items"`
	Subtotal    float64       `json:"subtotal"`
	TaxAmount   float64       `json:"taxAmount"`
	Total       float64       `json:"total"`
	BalanceDue  float64       `json:"balanceDue"`
	Status      string        `json:"status"` // Draft/Sent/Paid
	SentAt      string        `json:"sentAt"`
	PaidAmount  float64       `json:"paidAmount"`
	PaidAt      string        `json:"paidAt"`
	CreatedAt   string        `json:"createdAt"`
}

// ExpenseVendor 支出供应商（在 New expense 中选择或新建）
type ExpenseVendor struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Currency  string `json:"currency"`
	Email     string `json:"email"`
	Address   string `json:"address"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// Expense 任务相关支出记录
type Expense struct {
	ID          string  `json:"id"`
	TaskID      string  `json:"taskId"`
	TaskName    string  `json:"taskName,omitempty"` // 列表/详情：来自 tasks.company_name
	VendorID    string  `json:"vendorId,omitempty"`
	VendorName  string  `json:"vendorName,omitempty"` // 列表/详情：来自 expense_vendors.name
	ExpenseDate string  `json:"expenseDate"`            // 业务日期 YYYY-MM-DD
	Description string  `json:"description"`            // 支出说明
	AccountCode string  `json:"accountCode"`            // 费用科目 5XXX（须在 Settings 目录中）
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	CreatedAt   string  `json:"createdAt,omitempty"`
}

// ExpenseCodeCatalogItem Expense 表单下拉：仅后台 expense_codes 表中的科目
type ExpenseCodeCatalogItem struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ExpenseCodeRow Settings 中费用科目列表（含年内累计支出）
type ExpenseCodeRow struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	BalanceYtd  float64 `json:"balanceYtd"`
	BalanceYear int     `json:"balanceYear"`
}

// PayrollCompany Payroll 模块中管理的公司（每家公司归属某个用户账号）
type PayrollCompany struct {
	ID             string `json:"id"`
	Name           string `json:"name"`           // 常用名 / 显示名
	LegalName      string `json:"legalName"`      // CRA 注册全名
	BusinessNumber string `json:"businessNumber"` // CRA BN（9位数字，如 123456789）
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Address        string `json:"address"`        // 街道地址
	City           string `json:"city"`
	Province       string `json:"province"`       // 主要营业省份，如 BC / ON / AB
	PostalCode     string `json:"postalCode"`
	Country        string `json:"country"`        // 默认 Canada
	PayFrequency   string `json:"payFrequency"`   // biweekly | semimonthly | monthly | weekly
	Status         string `json:"status"`         // active | inactive
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
	OwnerUsername  string `json:"-"` // 归属用户，不暴露给前端
}

// MemberType 员工/承包商类型
type MemberType int

const (
	MemberTypeEmployee             MemberType = 0 // T4 — full source deductions
	MemberTypeContractor           MemberType = 1 // T4A — no source deductions
	MemberTypeConstructionContractor MemberType = 2 // T5018
)

// SalaryType 薪资类型
type SalaryType int

const (
	SalaryTypeSalaried  SalaryType = 0
	SalaryTypeTimeBased SalaryType = 1
)

// PayrollEmployee Payroll 模块中的员工/承包商（隶属于某家公司）
// CRA T4001 §8：SIN 必须在入职 3 天内收集，加密存储
type PayrollEmployee struct {
	ID          string `json:"id"`
	CompanyID   string `json:"companyId"`

	// Personal / contact
	LegalName     string `json:"legalName"`
	Nickname      string `json:"nickname"`
	Email         string `json:"email"`
	Mobile        string `json:"mobile"`
	Position      string `json:"position"`
	Address       string `json:"address"`
	Gender        string `json:"gender"`        // Male | Female | Non-binary | Prefer not to say
	MaritalStatus string `json:"maritalStatus"` // Single | Married | Common-law | Other
	Notes         string `json:"notes"`

	// CRA required (T4001 §8)
	Province    string `json:"province"`    // 2-letter code: BC / ON / QC …
	SIN         string `json:"sin,omitempty"`    // write-only input, never returned after save
	SINMasked   string `json:"sinMasked,omitempty"` // ***-***-XXX, read-only
	DateOfBirth string `json:"dateOfBirth"` // YYYY-MM-DD
	HireDate    string `json:"hireDate"`    // YYYY-MM-DD

	// Employment classification
	MemberType int    `json:"memberType"` // 0=Employee 1=Contractor 2=Construction
	SalaryType int    `json:"salaryType"` // 0=Salaried 1=Time-Based
	Status     string `json:"status"`     // active | terminated

	// Payroll setup
	PayRate      float64 `json:"payRate"`
	PayRateUnit  string  `json:"payRateUnit"`  // Hourly | Annually | Monthly
	PaysPerYear  int     `json:"paysPerYear"`  // 52 | 26 | 24 | 12
	PayFrequency string  `json:"payFrequency"` // Weekly | Bi-weekly | Semi-Monthly | Monthly
	HoursPerWeek float64 `json:"hoursPerWeek"`

	// TD1 tax credits (T4001 §4.3)
	TD1Federal    float64 `json:"td1Federal"`    // default 16129 (2025 basic personal amount)
	TD1Provincial float64 `json:"td1Provincial"` // province-specific

	// Flags
	PaidYTDOtherPayroll bool `json:"paidYtdOtherPayroll"`
	AutoVacation        bool `json:"autoVacation"`

	// ROE / termination
	TerminationDate  string `json:"terminationDate,omitempty"`
	ROERecallDate    string `json:"roeRecallDate,omitempty"`
	ROERecallUnknown bool   `json:"roeRecallUnknown,omitempty"`

	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// PayrollPeriod 一次发薪周期（属于某家公司）
type PayrollPeriod struct {
	ID           string `json:"id"`
	CompanyID    string `json:"companyId"`
	PeriodStart  string `json:"periodStart"` // YYYY-MM-DD
	PeriodEnd    string `json:"periodEnd"`   // YYYY-MM-DD
	PayDate      string `json:"payDate"`     // YYYY-MM-DD (CRA: deductions based on pay date)
	PaysPerYear  int    `json:"paysPerYear"` // 52 | 26 | 24 | 12
	PayFrequency string `json:"payFrequency"`
	PayrollType  string `json:"payrollType"` // regular | special
	Status       string `json:"status"`      // open | calculated | finalized
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

// PayrollEntry 一名员工在一个发薪周期内的计算结果
type PayrollEntry struct {
	ID         string `json:"id"`
	PeriodID   string `json:"periodId"`
	EmployeeID string `json:"employeeId"`
	CompanyID  string `json:"companyId"`

	// Employee name (joined, not stored)
	EmployeeName string `json:"employeeName,omitempty"`
	SalaryType   int    `json:"salaryType"`            // joined from employee: 0=Salaried 1=Time-Based
	PayRateUnit  string `json:"payRateUnit,omitempty"` // joined from employee: Hourly | Annually | Monthly

	// Earnings input
	Hours   float64 `json:"hours"`
	PayRate float64 `json:"payRate"`
	GrossPay float64 `json:"grossPay"`

	// Employee deductions (CPP T4001 §2, EI §3, Income Tax §4)
	CPPEmployee   float64 `json:"cppEmployee"`
	CPP2Employee  float64 `json:"cpp2Employee"`
	EIEmployee    float64 `json:"eiEmployee"`
	FederalTax    float64 `json:"federalTax"`
	ProvincialTax float64 `json:"provincialTax"`
	TotalDeductions float64 `json:"totalDeductions"`
	NetPay          float64 `json:"netPay"`

	// Employer contributions (for PD7A remittance)
	CPPEmployer  float64 `json:"cppEmployer"`
	CPP2Employer float64 `json:"cpp2Employer"`
	EIEmployer   float64 `json:"eiEmployer"`

	// YTD snapshot at time of calculation
	YTDGross  float64 `json:"ytdGross"`
	YTDCPPEe  float64 `json:"ytdCppEmployee"`
	YTDCPP2Ee float64 `json:"ytdCpp2Employee"`
	YTDEIEe   float64 `json:"ytdEiEmployee"`

	// Calculation audit trail (JSON blob of rates+inputs used)
	CalcSnapshotJSON string `json:"calcSnapshotJson,omitempty"`

	PaymentType string `json:"paymentType,omitempty"` // cheque | deposit

	Status    string `json:"status"` // draft | approved
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// PayrollEarningsCode defines how an earnings type is treated for payroll deductions.
// Companies configure their own codes under Settings → Pay Rules.
type PayrollEarningsCode struct {
	ID           string  `json:"id"`
	CompanyID    string  `json:"companyId"`
	Code         string  `json:"code"`         // short identifier, e.g. "TIPS", "OT15"
	Name         string  `json:"name"`         // display name, e.g. "Tips", "Overtime Hours @ 1.5"
	Enabled      bool    `json:"enabled"`
	CPP          bool    `json:"cpp"`          // CPP-applicable
	EI           bool    `json:"ei"`           // EI-applicable (insurable earnings)
	TaxFed       bool    `json:"taxFed"`       // federal income tax applicable
	TaxProv      bool    `json:"taxProv"`      // provincial income tax applicable
	NonCash      bool    `json:"nonCash"`      // non-cash benefit (taxable but no cheque issued)
	Vacationable bool    `json:"vacationable"` // included in vacation-pay calculation base
	Multiplier   float64 `json:"multiplier"`   // rate multiplier: 1.0=regular, 1.5=OT, 2.0=DT
	IsSystem     bool    `json:"isSystem"`     // system-defined codes cannot be deleted
	T4Box        string  `json:"t4Box"`        // T4 slip box, e.g. "14", "40" (optional)
	SortOrder    int     `json:"sortOrder"`
	CreatedAt    string  `json:"createdAt,omitempty"`
	UpdatedAt    string  `json:"updatedAt,omitempty"`
}

// PayrollCompanyRules holds payroll-computation rules for one company.
type PayrollCompanyRules struct {
	CompanyID      string  `json:"companyId"`
	VacationRate   float64 `json:"vacationRate"`   // e.g. 0.04 = 4%
	VacationMethod string  `json:"vacationMethod"` // "per_period" | "accrued"
	UpdatedAt      string  `json:"updatedAt,omitempty"`
}

// PayrollEntryEarning is one line of additional earnings for a payroll entry.
// Regular base pay is stored directly on PayrollEntry; each extra earnings type
// (tips, overtime, commission, etc.) is stored as a separate line here.
type PayrollEntryEarning struct {
	ID             string  `json:"id"`
	EntryID        string  `json:"entryId"`
	EarningsCodeID string  `json:"earningsCodeId"`
	CodeName       string  `json:"codeName,omitempty"` // joined from payroll_earnings_codes
	Hours          float64 `json:"hours"`
	Rate           float64 `json:"rate"`
	Amount         float64 `json:"amount"`
	CreatedAt      string  `json:"createdAt,omitempty"`
	UpdatedAt      string  `json:"updatedAt,omitempty"`
}

// BankAccount 支票打印 / MICR 银行账户（支持多账户）
type BankAccount struct {
	ID                   string `json:"id"`
	Label                string `json:"label"`
	CompanyID            string `json:"companyId"`   // 关联 payroll company（可空）
	BankName             string `json:"bankName"`
	BankAddress          string `json:"bankAddress"`
	BankCity             string `json:"bankCity"`
	BankProvince         string `json:"bankProvince"`
	BankPostalCode       string `json:"bankPostalCode"`
	MICRCountry          string `json:"micrCountry"` // CA | US | EU
	BankInstitution      string `json:"bankInstitution"`
	BankTransit          string `json:"bankTransit"`
	BankRoutingABA       string `json:"bankRoutingAba"`
	BankAccount          string `json:"bankAccount"`
	BankIBAN             string `json:"bankIban"`
	BankSWIFT            string `json:"bankSwift"`
	BankChequeNumber     string `json:"bankChequeNumber"`
	MICRLineOverride     string `json:"micrLineOverride"`
	DefaultChequeCurrency string `json:"defaultChequeCurrency"`
}

// ── Year-End Information Returns (T4 / T4 Summary) ───────────────────────────

// YearEndReturnStatus tracks the lifecycle of a year-end return.
type YearEndReturnStatus string

const (
	YearEndStatusDraft     YearEndReturnStatus = "draft"
	YearEndStatusFinalized YearEndReturnStatus = "finalized"
)

// YearEndReturn is one annual filing for a single payroll account in a calendar year.
// Scope: company_id + payroll_account_number + calendar_year is UNIQUE.
type YearEndReturn struct {
	ID                   string              `json:"id"`
	CompanyID            string              `json:"companyId"`
	PayrollAccountNumber string              `json:"payrollAccountNumber"` // BN+RP, e.g. 123456789RP0001
	CalendarYear         int                 `json:"calendarYear"`
	Status               YearEndReturnStatus `json:"status"` // draft | finalized
	GeneratedAt          string              `json:"generatedAt"`
	FinalizedAt          string              `json:"finalizedAt,omitempty"`
	SourceHash           string              `json:"sourceHash"` // SHA-256 of contributing period IDs+dates
	SlipCount            int                 `json:"slipCount,omitempty"`
	CreatedAt            string              `json:"createdAt,omitempty"`
	UpdatedAt            string              `json:"updatedAt,omitempty"`
}

// T4OtherInfo represents one entry in the "Other Information" section of a T4 slip.
// Code is the two-digit CRA code (e.g. "30", "42"); Amount is the dollar value.
type T4OtherInfo struct {
	Code   string  `json:"code"`
	Amount float64 `json:"amount"`
}

// T4Slip is an immutable snapshot of one employee's T4 data for a calendar year.
// All monetary fields are populated from finalized payroll entries at draft-generation
// time. After finalize, this record must not be mutated.
type T4Slip struct {
	ID                string `json:"id"`
	YearEndReturnID   string `json:"yearEndReturnId"`
	EmployeeID        string `json:"employeeId"`
	CalendarYear      int    `json:"calendarYear"`

	// Employee snapshot (captured at generation time)
	EmployeeLegalName string `json:"employeeLegalName"`
	EmployeeSINMasked string `json:"employeeSinMasked"` // ***-***-XXX; full SIN never stored in snapshot
	EmployeeAddress   string `json:"employeeAddress"`
	EmployeeProvince  string `json:"employeeProvince"` // Box 10: province of employment

	// Box 14 – Employment income (gross pay from all finalized periods)
	Box14EmploymentIncome float64 `json:"box14EmploymentIncome"`

	// Box 16 – Employee's CPP contributions (province ≠ QC)
	Box16CPPEmployee float64 `json:"box16CppEmployee"`

	// Box 16A – Employee's CPP2 contributions (2024+, province ≠ QC)
	Box16ACpp2Employee float64 `json:"box16ACpp2Employee"`

	// Box 17 – Employee's QPP contributions (QC only) — reserved for future use
	Box17QPPEmployee float64 `json:"box17QppEmployee"`

	// Box 18 – Employee's EI premiums
	Box18EIEmployee float64 `json:"box18EiEmployee"`

	// Box 20 – RPP contributions (unsupported in Phase 1 — pending mapping)
	Box20RPPContributions float64 `json:"box20RppContributions"`

	// Box 22 – Income tax deducted (federal + provincial)
	Box22IncomeTaxDeducted float64 `json:"box22IncomeTaxDeducted"`

	// Box 24 – EI insurable earnings
	Box24EIInsurableEarnings float64 `json:"box24EiInsurableEarnings"`

	// Box 26 – CPP/QPP pensionable earnings
	Box26CPPPensionableEarnings float64 `json:"box26CppPensionableEarnings"`

	// Box 44 – Union dues (unsupported Phase 1 — pending mapping)
	Box44UnionDues float64 `json:"box44UnionDues"`

	// Box 45 – Dental benefit code (1–5); 0 = not set (required since 2023)
	Box45DentalBenefitCode int `json:"box45DentalBenefitCode"`

	// Box 46 – Charitable donations (unsupported Phase 1 — pending mapping)
	Box46CharitableDonations float64 `json:"box46CharitableDonations"`

	// Box 52 – Pension adjustment (unsupported Phase 1 — pending mapping)
	Box52PensionAdjustment float64 `json:"box52PensionAdjustment"`

	// Employer-side figures stored on the slip to support Summary aggregation
	EmployerCPP  float64 `json:"employerCpp"`
	EmployerCPP2 float64 `json:"employerCpp2"`
	EmployerEI   float64 `json:"employerEi"`

	// Other information codes (up to 6 per CRA spec)
	OtherInfo []T4OtherInfo `json:"otherInfo"`

	// Unsupported/pending-mapping fields are listed here so they are never silently dropped
	UnsupportedFields []string `json:"unsupportedFields,omitempty"`

	VersionNo int    `json:"versionNo"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// T4Summary is the aggregate totals for all T4 slips under one YearEndReturn.
// Totals are computed exclusively by summing T4Slip fields; they are never
// independently calculated.
type T4Summary struct {
	ID              string `json:"id"`
	YearEndReturnID string `json:"yearEndReturnId"`
	SlipCount       int    `json:"slipCount"`

	// Line 14 – Total employment income
	TotalEmploymentIncome float64 `json:"totalEmploymentIncome"`

	// Line 16 – Total employee CPP contributions
	TotalCPPEmployee float64 `json:"totalCppEmployee"`

	// Line 16A – Total employee CPP2 contributions
	TotalCPP2Employee float64 `json:"totalCpp2Employee"`

	// Line 17 – Total employee QPP contributions (Quebec)
	TotalQPPEmployee float64 `json:"totalQppEmployee"`

	// Line 18 – Total employee EI premiums
	TotalEIEmployee float64 `json:"totalEiEmployee"`

	// Line 22 – Total income tax deducted
	TotalIncomeTaxDeducted float64 `json:"totalIncomeTaxDeducted"`

	// Line 27 – Total employer CPP contributions
	TotalCPPEmployer float64 `json:"totalCppEmployer"`

	// Line 27A – Total employer CPP2 contributions
	TotalCPP2Employer float64 `json:"totalCpp2Employer"`

	// Line 19 – Total employer EI premiums
	TotalEIEmployer float64 `json:"totalEiEmployer"`

	// Line 52 – Total pension adjustments
	TotalPensionAdjustments float64 `json:"totalPensionAdjustments"`

	// Contact info (for CRA T4 Summary form)
	ContactName  string `json:"contactName"`
	ContactPhone string `json:"contactPhone"`

	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// YearEndAuditLog records every significant action on a YearEndReturn.
type YearEndAuditLog struct {
	ID              int    `json:"id"`
	YearEndReturnID string `json:"yearEndReturnId"`
	Action          string `json:"action"` // generated | regenerated | finalized | cancelled
	ActorUsername   string `json:"actorUsername"`
	Note            string `json:"note"`
	CreatedAt       string `json:"createdAt"`
}

// T4ValidationError is a single validation issue blocking finalization.
type T4ValidationError struct {
	Scope   string `json:"scope"`   // "summary" | "slip:{employeeID}"
	Code    string `json:"code"`    // machine-readable code
	Message string `json:"message"` // human-readable description
}

// YearEndReturnDetail is the full payload returned for a single return (slips + summary + audit).
type YearEndReturnDetail struct {
	Return    YearEndReturn    `json:"return"`
	Summary   T4Summary        `json:"summary"`
	Slips     []T4Slip         `json:"slips"`
	AuditLogs []YearEndAuditLog `json:"auditLogs"`
	ValidationErrors []T4ValidationError `json:"validationErrors,omitempty"`
}

// ─── ROE (Record of Employment) ───────────────────────────────────────────────

type ROEStatus string

const (
	ROEStatusDraft  ROEStatus = "draft"
	ROEStatusIssued ROEStatus = "issued"
)

// ROEPeriodEntry is one row of Block 15B — insurable earnings/hours for a single pay period.
type ROEPeriodEntry struct {
	PeriodNo          int     `json:"periodNo"`          // 1 = most recent
	PeriodID          string  `json:"periodId,omitempty"`
	PeriodEndDate     string  `json:"periodEndDate"`     // YYYY-MM-DD
	InsurableEarnings float64 `json:"insurableEarnings"` // capped to annual MEI
	InsurableHours    float64 `json:"insurableHours"`
	Approximated      bool    `json:"approximated,omitempty"` // true when no entry_earnings rows
}

// ROEOtherMoney is one row of Block 19.
type ROEOtherMoney struct {
	Type   string  `json:"type"`   // e.g. "StatutoryHoliday", "Bonus", "Commission"
	Amount float64 `json:"amount"`
}

// ROE is the main Record of Employment record.
type ROE struct {
	ID        string    `json:"id"`        // ROE00001
	CompanyID string    `json:"companyId"`
	EmployeeID string   `json:"employeeId"`
	SerialNumber string `json:"serialNumber"` // Service Canada serial
	Status    ROEStatus `json:"status"`       // draft | issued

	// Block 16 – reason for issuing
	ReasonCode      string `json:"reasonCode"`
	ReasonCodeOther string `json:"reasonCodeOther,omitempty"`

	// Block 10 – first day worked
	FirstDayWorked string `json:"firstDayWorked"` // YYYY-MM-DD

	// Block 11 – last day for which paid (extended by pay-in-lieu)
	LastDayPaid string `json:"lastDayPaid"` // YYYY-MM-DD

	// Block 12 – final pay period ending date
	FinalPayPeriodEnd string `json:"finalPayPeriodEnd"` // YYYY-MM-DD

	// Block 13 – occupation
	Occupation string `json:"occupation"`

	// Block 14 – expected date of recall
	ExpectedRecallDate string `json:"expectedRecallDate,omitempty"` // YYYY-MM-DD
	RecallUnknown      bool   `json:"recallUnknown"`

	// Block 15A – total insurable hours
	TotalInsurableHours float64 `json:"totalInsurableHours"`

	// Block 15B – per-period insurable earnings snapshot
	InsurableEarningsPeriods []ROEPeriodEntry `json:"insurableEarningsPeriods"`

	// Block 15C – total insurable earnings (sum of 15B)
	TotalInsurableEarnings float64 `json:"totalInsurableEarnings"`

	// Block 17A – vacation pay
	VacationPay     float64 `json:"vacationPay"`
	VacationPayType string  `json:"vacationPayType"` // "included" | "separate" | "deferred" | ""

	// Block 18 – statutory holiday pay
	StatutoryHolidayPay float64 `json:"statutoryHolidayPay"`

	// Block 19 – other moneys
	OtherMoneys []ROEOtherMoney `json:"otherMoneys,omitempty"`

	// Block 20 – comments
	Comments string `json:"comments"`

	// Metadata
	PayFrequency string `json:"payFrequency"`
	SourceHash   string `json:"sourceHash"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
	IssuedAt     string `json:"issuedAt,omitempty"`
}

// ROEValidationError is a single validation issue blocking issuance.
type ROEValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ROEAuditLog records every significant action on a ROE.
type ROEAuditLog struct {
	ID            int    `json:"id"`
	ROEID         string `json:"roeId"`
	Action        string `json:"action"` // generated | regenerated | issued
	ActorUsername string `json:"actorUsername"`
	Note          string `json:"note"`
	CreatedAt     string `json:"createdAt"`
}

// ROEDetail is the full payload for a single ROE.
type ROEDetail struct {
	ROE        ROE                  `json:"roe"`
	Employee   PayrollEmployee      `json:"employee"`
	Company    PayrollCompany       `json:"company"`
	AuditLogs  []ROEAuditLog        `json:"auditLogs"`
	Validation []ROEValidationError `json:"validationErrors,omitempty"`
}

// ─── Year-End Review ──────────────────────────────────────────────────────────

type ReviewSeverity string

const (
	ReviewError   ReviewSeverity = "error"   // blocks T4 finalize
	ReviewWarning ReviewSeverity = "warning" // should fix before filing
	ReviewInfo    ReviewSeverity = "info"    // informational
)

// ReviewIssue is a single finding from the year-end review.
type ReviewIssue struct {
	Severity     ReviewSeverity `json:"severity"`
	Code         string         `json:"code"`
	Message      string         `json:"message"`
	EmployeeID   string         `json:"employeeId,omitempty"`
	EmployeeName string         `json:"employeeName,omitempty"`
}

// ReviewOpenPeriod describes a pay period that was not finalized within the review year.
type ReviewOpenPeriod struct {
	PeriodID    string `json:"periodId"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	PayDate     string `json:"payDate"`
	Status      string `json:"status"`
}

// ReviewEmployeeRow holds per-employee YTD totals for the review year.
type ReviewEmployeeRow struct {
	EmployeeID      string  `json:"employeeId"`
	LegalName       string  `json:"legalName"`
	Status          string  `json:"status"`
	Province        string  `json:"province"`
	SINMissing      bool    `json:"sinMissing"`
	ProvinceMissing bool    `json:"provinceMissing"`
	PeriodCount     int     `json:"periodCount"`
	GrossPay        float64 `json:"grossPay"`
	CPPEmployee     float64 `json:"cppEmployee"`
	CPP2Employee    float64 `json:"cpp2Employee"`
	EIEmployee      float64 `json:"eiEmployee"`
	FederalTax      float64 `json:"federalTax"`
	ProvincialTax   float64 `json:"provincialTax"`
	CPPEmployer     float64 `json:"cppEmployer"`
	EIEmployer      float64 `json:"eiEmployer"`
}

// PayrollYearLock records that a calendar year has been locked for a company
// after the year-end task (T4 finalize + submit) is completed.
type PayrollYearLock struct {
	CompanyID string `json:"companyId"`
	Year      int    `json:"year"`
	LockedAt  string `json:"lockedAt"`
	LockedBy  string `json:"lockedBy"`
}

// YearEndReviewReport is the full response from the year-end review endpoint.
type YearEndReviewReport struct {
	CompanyID    string `json:"companyId"`
	CalendarYear int    `json:"calendarYear"`
	GeneratedAt  string `json:"generatedAt"`

	// Period stats
	FinalizedPeriods int `json:"finalizedPeriods"`
	OpenPeriods      int `json:"openPeriods"`
	OpenPeriodsList  []ReviewOpenPeriod `json:"openPeriodsList,omitempty"`

	// Expected periods (based on company pay frequency)
	ExpectedPeriods int `json:"expectedPeriods"`

	// Aggregate totals (finalized periods only)
	TotalEmployees    int     `json:"totalEmployees"`
	TotalGrossPay     float64 `json:"totalGrossPay"`
	TotalCPPEmployee  float64 `json:"totalCppEmployee"`
	TotalCPP2Employee float64 `json:"totalCpp2Employee"`
	TotalEIEmployee   float64 `json:"totalEiEmployee"`
	TotalFederalTax   float64 `json:"totalFederalTax"`
	TotalProvincialTax float64 `json:"totalProvincialTax"`
	TotalCPPEmployer  float64 `json:"totalCppEmployer"`
	TotalEIEmployer   float64 `json:"totalEiEmployer"`

	// CRA annual maxima (from rate settings)
	CPPMaxEE    float64 `json:"cppMaxEe"`
	CPP2MaxEE   float64 `json:"cpp2MaxEe"`
	EIMaxEE     float64 `json:"eiMaxEe"`

	// Per-employee breakdown
	Employees []ReviewEmployeeRow `json:"employees"`

	// Issues found
	Issues []ReviewIssue `json:"issues"`

	// Overall readiness
	T4Ready      bool `json:"t4Ready"`      // no error-severity issues
	HasT4Draft   bool `json:"hasT4Draft"`   // draft already generated
	HasT4Final   bool `json:"hasT4Final"`   // already finalized
}
