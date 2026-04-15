package models

// TaskStatus 任务状态（避免手写拼写错误，如 Penging）
type TaskStatus string

const (
	StatusPending TaskStatus = "Pending"
	StatusDone    TaskStatus = "Done"
	StatusSent    TaskStatus = "Sent"
	StatusPaid    TaskStatus = "Paid"
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

// PayrollCompany Payroll 模块中管理的公司（单用户可管理多家公司）
type PayrollCompany struct {
	ID             string `json:"id"`
	Name           string `json:"name"`           // 常用名 / 显示名
	LegalName      string `json:"legalName"`      // CRA 注册全名
	BusinessNumber string `json:"businessNumber"` // CRA BN（9位数字，如 123456789）
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Address        string `json:"address"`
	Province       string `json:"province"`      // 主要营业省份，如 BC / ON / AB
	PayFrequency   string `json:"payFrequency"`  // biweekly | semimonthly | monthly | weekly
	Status         string `json:"status"`        // active | inactive
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
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
	LegalName string `json:"legalName"`
	Nickname  string `json:"nickname"`
	Email     string `json:"email"`
	Mobile    string `json:"mobile"`
	Position  string `json:"position"`

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

	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// BankAccount 支票打印 / MICR 银行账户（支持多账户）
type BankAccount struct {
	ID                   string `json:"id"`
	Label                string `json:"label"`
	BankName             string `json:"bankName"`
	MICRCountry          string `json:"micrCountry"` // CA | US | EU
	BankInstitution       string `json:"bankInstitution"`
	BankTransit           string `json:"bankTransit"`
	BankRoutingABA        string `json:"bankRoutingAba"`
	BankAccount           string `json:"bankAccount"`
	BankIBAN             string `json:"bankIban"`
	BankSWIFT            string `json:"bankSwift"`
	BankChequeNumber      string `json:"bankChequeNumber"`
	MICRLineOverride      string `json:"micrLineOverride"`
	DefaultChequeCurrency string `json:"defaultChequeCurrency"`
}
