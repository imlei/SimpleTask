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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	Status  string `json:"status"` // active | inactive；inactive 时不可新建任务/发票
}

// Task 对应任务表中的一行
type Task struct {
	ID               string     `json:"id"`
	CustomerID       string     `json:"customerId"`
	CustomerName     string     `json:"customerName,omitempty"` // 列表/详情展示，来自 customers 表
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

// BankAccount 支票打印 / MICR 银行账户（支持多账户）
type BankAccount struct {
	ID                   string `json:"id"`
	Label                string `json:"label"`
	MICRCountry           string `json:"micrCountry"` // CA | US
	BankInstitution       string `json:"bankInstitution"`
	BankTransit           string `json:"bankTransit"`
	BankRoutingABA        string `json:"bankRoutingAba"`
	BankAccount           string `json:"bankAccount"`
	BankChequeNumber      string `json:"bankChequeNumber"`
	MICRLineOverride      string `json:"micrLineOverride"`
	DefaultChequeCurrency string `json:"defaultChequeCurrency"`
}
