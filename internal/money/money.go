package money

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrNegativeAmount    = errors.New("amount cannot be negative")
	ErrInvalidRate       = errors.New("invalid tax rate")
	ErrInsufficientFunds = errors.New("insufficient funds")
)

// Decimal 表示精确的金额，以基本货币单位（如分）存储
// 避免使用 float64 进行货币计算
type Decimal struct {
	value decimal.Decimal
}

// NewFromFloat 从 float64 创建 Decimal（不推荐，仅用于兼容）
func NewFromFloat(f float64) (Decimal, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Decimal{}, ErrInvalidAmount
	}
	return Decimal{value: decimal.NewFromFloat(f)}, nil
}

// NewFromString 从字符串创建 Decimal（推荐）
func NewFromString(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Decimal{}, ErrInvalidAmount
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Decimal{}, ErrInvalidAmount
	}
	if d.IsNegative() {
		return Decimal{}, ErrNegativeAmount
	}
	return Decimal{value: d}, nil
}

// NewFromInt 从整数创建 Decimal（以分为单位）
func NewFromInt(cents int64) Decimal {
	return Decimal{value: decimal.NewFromInt(cents).Div(decimal.NewFromInt(100))}
}

// Zero 返回零值
func Zero() Decimal {
	return Decimal{value: decimal.Zero}
}

// Add 加法
func (d Decimal) Add(other Decimal) Decimal {
	return Decimal{value: d.value.Add(other.value)}
}

// Sub 减法
func (d Decimal) Sub(other Decimal) Decimal {
	return Decimal{value: d.value.Sub(other.value)}
}

// Mul 乘法
func (d Decimal) Mul(other Decimal) Decimal {
	return Decimal{value: d.value.Mul(other.value)}
}

// MulFloat 乘以浮点数（用于税率等）
func (d Decimal) MulFloat(f float64) (Decimal, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Decimal{}, ErrInvalidRate
	}
	return Decimal{value: d.value.Mul(decimal.NewFromFloat(f))}, nil
}

// Div 除法
func (d Decimal) Div(other Decimal) (Decimal, error) {
	if other.value.IsZero() {
		return Decimal{}, errors.New("division by zero")
	}
	return Decimal{value: d.value.Div(other.value)}, nil
}

// Percent 计算百分比
func (d Decimal) Percent(percentage float64) (Decimal, error) {
	rate := decimal.NewFromFloat(percentage).Div(decimal.NewFromInt(100))
	return Decimal{value: d.value.Mul(rate)}, nil
}

// IsZero 是否为零
func (d Decimal) IsZero() bool {
	return d.value.IsZero()
}

// IsNegative 是否为负
func (d Decimal) IsNegative() bool {
	return d.value.IsNegative()
}

// IsPositive 是否为正
func (d Decimal) IsPositive() bool {
	return d.value.IsPositive()
}

// Compare 比较：-1 小于, 0 等于, 1 大于
func (d Decimal) Compare(other Decimal) int {
	return d.value.Cmp(other.value)
}

// GreaterThan 是否大于
func (d Decimal) GreaterThan(other Decimal) bool {
	return d.value.GreaterThan(other.value)
}

// GreaterThanOrEqual 是否大于等于
func (d Decimal) GreaterThanOrEqual(other Decimal) bool {
	return d.value.GreaterThanOrEqual(other.value)
}

// LessThan 是否小于
func (d Decimal) LessThan(other Decimal) bool {
	return d.value.LessThan(other.value)
}

// LessThanOrEqual 是否小于等于
func (d Decimal) LessThanOrEqual(other Decimal) bool {
	return d.value.LessThanOrEqual(other.value)
}

// Equal 是否相等
func (d Decimal) Equal(other Decimal) bool {
	return d.value.Equal(other.value)
}

// Round 四舍五入到指定小数位（财务计算通常保留2位）
func (d Decimal) Round(places int32) Decimal {
	return Decimal{value: d.value.Round(places)}
}

// RoundToCents 四舍五入到分（2位小数）
func (d Decimal) RoundToCents() Decimal {
	return d.Round(2)
}

// Float64 转换为 float64（仅用于显示，不建议用于计算）
func (d Decimal) Float64() float64 {
	f, _ := d.value.Float64()
	return f
}

// String 格式化为字符串
func (d Decimal) String() string {
	return d.value.String()
}

// StringFixed 格式化为固定小数位
func (d Decimal) StringFixed(places int32) string {
	return d.value.StringFixed(places)
}

// StringFixed2 格式化为2位小数（标准货币格式）
func (d Decimal) StringFixed2() string {
	return d.StringFixed(2)
}

// Format 格式化为货币字符串（如 "1,234.56"）
func (d Decimal) Format() string {
	s := d.StringFixed2()
	parts := strings.Split(s, ".")
	intPart := parts[0]
	decPart := ""
	if len(parts) > 1 {
		decPart = "." + parts[1]
	}

	// 添加千位分隔符
	var result []byte
	n := len(intPart)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, intPart[i])
	}
	return string(result) + decPart
}

// Max 返回最大值
func Max(a, b Decimal) Decimal {
	if a.GreaterThan(b) {
		return a
	}
	return b
}

// Min 返回最小值
func Min(a, b Decimal) Decimal {
	if a.LessThan(b) {
		return a
	}
	return b
}

// Sum 求和
func Sum(values []Decimal) Decimal {
	total := Zero()
	for _, v := range values {
		total = total.Add(v)
	}
	return total
}

// ValidateAmount 验证金额是否有效
func ValidateAmount(f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ErrInvalidAmount
	}
	if f < 0 {
		return ErrNegativeAmount
	}
	return nil
}

// MustFromString 从字符串创建 Decimal，如果失败则 panic
// 仅用于测试或已知有效的输入
func MustFromString(s string) Decimal {
	d, err := NewFromString(s)
	if err != nil {
		panic(fmt.Sprintf("invalid amount: %s", s))
	}
	return d
}

// MustFromFloat 从 float64 创建 Decimal，如果失败则 panic
// 仅用于测试或已知有效的输入
func MustFromFloat(f float64) Decimal {
	d, err := NewFromFloat(f)
	if err != nil {
		panic(fmt.Sprintf("invalid amount: %f", f))
	}
	return d
}
