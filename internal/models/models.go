package models

// TaskStatus 任务状态（避免手写拼写错误，如 Penging）
type TaskStatus string

const (
	StatusPending TaskStatus = "Pending"
	StatusDone    TaskStatus = "Done"
)

// Task 对应业务追踪表中的一行
type Task struct {
	ID                string     `json:"id"`
	CompanyName       string     `json:"companyName"`
	Date              string     `json:"date"` // ISO 日期字符串，如 2026-03-30
	Service1          string     `json:"service1"`
	Service2          string     `json:"service2"`
	Price1            float64    `json:"price1"`
	Price2            float64    `json:"price2"`
	SelectedPriceIDs  []string   `json:"selectedPriceIds,omitempty"` // 从价目表多选的服务项 ID
	Status            TaskStatus `json:"status"`
	CompletedAt       string     `json:"completedAt,omitempty"` // 标记为 Done 时的日期 YYYY-MM-DD
	Note              string     `json:"note"`
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
