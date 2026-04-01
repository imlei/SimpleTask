package store

import (
	"fmt"
	"strings"
	"time"
)

// ReplaceExchangeRatesForDate 用 Frankfurter 一次响应覆盖该日、该基准币下的全部报价（cache-aside 写入）
func (s *Store) ReplaceExchangeRatesForDate(requestedDate, base string, quoteToRate map[string]float64, fetchedAt string) error {
	base = strings.ToUpper(strings.TrimSpace(base))
	requestedDate = strings.TrimSpace(requestedDate)
	if base == "" || requestedDate == "" {
		return fmt.Errorf("base and date required")
	}
	if fetchedAt == "" {
		fetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM exchange_rates WHERE requested_date=? AND base_code=?`, requestedDate, base); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO exchange_rates (requested_date, base_code, quote_code, rate, fetched_at) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for q, r := range quoteToRate {
		q = strings.ToUpper(strings.TrimSpace(q))
		if q == "" || q == base {
			continue
		}
		if _, err := stmt.Exec(requestedDate, base, q, r, fetchedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CountExchangeRatesForDate 本地是否已有该日该基准的缓存（>0 则视为命中，不再请求外网）
func (s *Store) CountExchangeRatesForDate(requestedDate, base string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM exchange_rates WHERE requested_date=? AND base_code=?`,
		strings.TrimSpace(requestedDate),
		strings.ToUpper(strings.TrimSpace(base)),
	).Scan(&n)
	return n, err
}

// GetExchangeRatesForDates 返回 date -> (quote -> rate)
func (s *Store) GetExchangeRatesForDates(base string, dates []string) (map[string]map[string]float64, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	out := make(map[string]map[string]float64)
	if len(dates) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(dates))
	args := make([]any, 0, len(dates)+1)
	args = append(args, base)
	for i, d := range dates {
		placeholders[i] = "?"
		args = append(args, strings.TrimSpace(d))
	}
	q := fmt.Sprintf(
		`SELECT requested_date, quote_code, rate FROM exchange_rates WHERE base_code=? AND requested_date IN (%s)`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dateStr, quote string
		var rate float64
		if err := rows.Scan(&dateStr, &quote, &rate); err != nil {
			return nil, err
		}
		if out[dateStr] == nil {
			out[dateStr] = make(map[string]float64)
		}
		out[dateStr][strings.ToUpper(quote)] = rate
	}
	return out, rows.Err()
}

// ReplaceExchangeQuotesForDate 仅替换指定报价币种在该日、该基准下的缓存（不影响其它币种行）
func (s *Store) ReplaceExchangeQuotesForDate(requestedDate, base string, quoteToRate map[string]float64, fetchedAt string) error {
	base = strings.ToUpper(strings.TrimSpace(base))
	requestedDate = strings.TrimSpace(requestedDate)
	if base == "" || requestedDate == "" {
		return fmt.Errorf("base and date required")
	}
	if len(quoteToRate) == 0 {
		return nil
	}
	if fetchedAt == "" {
		fetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keys := make([]string, 0, len(quoteToRate))
	for q := range quoteToRate {
		q = strings.ToUpper(strings.TrimSpace(q))
		if q == "" || q == base {
			continue
		}
		keys = append(keys, q)
	}
	if len(keys) == 0 {
		return tx.Commit()
	}
	ph := make([]string, len(keys))
	args := make([]any, 0, len(keys)+2)
	args = append(args, requestedDate, base)
	for i, k := range keys {
		ph[i] = "?"
		args = append(args, k)
	}
	delQ := fmt.Sprintf(`DELETE FROM exchange_rates WHERE requested_date=? AND base_code=? AND quote_code IN (%s)`, strings.Join(ph, ","))
	if _, err := tx.Exec(delQ, args...); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO exchange_rates (requested_date, base_code, quote_code, rate, fetched_at) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for q, r := range quoteToRate {
		q = strings.ToUpper(strings.TrimSpace(q))
		if q == "" || q == base {
			continue
		}
		if _, err := stmt.Exec(requestedDate, base, q, r, fetchedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
