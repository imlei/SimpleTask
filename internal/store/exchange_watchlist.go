package store

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var iso4217CodeRx = regexp.MustCompile(`^[A-Za-z]{3}$`)

// ExchangeRateWatchItem 主页汇率表仅展示此列表中的货币
type ExchangeRateWatchItem struct {
	Code      string `json:"code"`
	SortOrder int    `json:"sortOrder"`
}

// ListExchangeRateWatchlist 按 sort_order、code 排序
func (s *Store) ListExchangeRateWatchlist() ([]ExchangeRateWatchItem, error) {
	rows, err := s.db.Query(`SELECT code, sort_order FROM exchange_rate_watchlist ORDER BY sort_order ASC, code ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExchangeRateWatchItem
	for rows.Next() {
		var it ExchangeRateWatchItem
		if err := rows.Scan(&it.Code, &it.SortOrder); err != nil {
			return nil, err
		}
		it.Code = strings.ToUpper(strings.TrimSpace(it.Code))
		out = append(out, it)
	}
	return out, rows.Err()
}

// ListExchangeRateWatchlistCodes 仅代码列表（已大写、去重顺序保留）
func (s *Store) ListExchangeRateWatchlistCodes() ([]string, error) {
	list, err := s.ListExchangeRateWatchlist()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var codes []string
	for _, it := range list {
		if it.Code == "" || seen[it.Code] {
			continue
		}
		seen[it.Code] = true
		codes = append(codes, it.Code)
	}
	return codes, nil
}

// AddExchangeRateWatchlist 添加 ISO 4217 三位代码（已存在则返回原顺序，不报错）
func (s *Store) AddExchangeRateWatchlist(code string) (ExchangeRateWatchItem, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !iso4217CodeRx.MatchString(code) {
		return ExchangeRateWatchItem{}, fmt.Errorf("currency code must be 3 letters (ISO 4217)")
	}
	var so int
	err := s.db.QueryRow(`SELECT sort_order FROM exchange_rate_watchlist WHERE code=?`, code).Scan(&so)
	if err == nil {
		return ExchangeRateWatchItem{Code: code, SortOrder: so}, nil
	}
	if err != sql.ErrNoRows {
		return ExchangeRateWatchItem{}, err
	}
	var maxOrd sql.NullInt64
	_ = s.db.QueryRow(`SELECT MAX(sort_order) FROM exchange_rate_watchlist`).Scan(&maxOrd)
	ord := 0
	if maxOrd.Valid {
		ord = int(maxOrd.Int64) + 1
	}
	if _, err := s.db.Exec(`INSERT INTO exchange_rate_watchlist (code, sort_order) VALUES (?, ?)`, code, ord); err != nil {
		return ExchangeRateWatchItem{}, err
	}
	return ExchangeRateWatchItem{Code: code, SortOrder: ord}, nil
}

// DeleteExchangeRateWatchlist 从列表移除
func (s *Store) DeleteExchangeRateWatchlist(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return fmt.Errorf("code required")
	}
	res, err := s.db.Exec(`DELETE FROM exchange_rate_watchlist WHERE code=?`, code)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountCachedWatchlistQuotes 该日、基准下，watchlist 中有多少币种已有缓存行
func (s *Store) CountCachedWatchlistQuotes(requestedDate, base string, watchlist []string) (int, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	requestedDate = strings.TrimSpace(requestedDate)
	if len(watchlist) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(watchlist))
	args := make([]any, 0, len(watchlist)+2)
	args = append(args, requestedDate, base)
	for i, c := range watchlist {
		placeholders[i] = "?"
		args = append(args, strings.ToUpper(strings.TrimSpace(c)))
	}
	q := fmt.Sprintf(
		`SELECT COUNT(DISTINCT quote_code) FROM exchange_rates WHERE requested_date=? AND base_code=? AND quote_code IN (%s)`,
		strings.Join(placeholders, ","),
	)
	var n int
	err := s.db.QueryRow(q, args...).Scan(&n)
	return n, err
}
