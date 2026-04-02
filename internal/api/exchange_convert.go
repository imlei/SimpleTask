package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// lastNWeekdaysBeforeOrOn 从 anchor 所在日（若周末则回退到当周最后一个工作日）起向前取 n 个工作日，按时间正序（最早的在左）
func lastNWeekdaysBeforeOrOn(anchor time.Time, n int, loc *time.Location) []string {
	if n <= 0 {
		return nil
	}
	d := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, loc)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	var rev []string
	cur := d
	for len(rev) < n {
		if cur.Weekday() != time.Saturday && cur.Weekday() != time.Sunday {
			rev = append(rev, cur.Format("2006-01-02"))
		}
		cur = cur.AddDate(0, 0, -1)
		if cur.Before(time.Date(1999, 1, 1, 0, 0, 0, 0, loc)) {
			break
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func dateStrIsToday(dateStr string, loc *time.Location) bool {
	d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(dateStr), loc)
	if err != nil {
		return false
	}
	now := time.Now().In(loc)
	t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return d.Equal(t)
}

func uniqueQuoteCodes(base string, codes ...string) []string {
	seen := map[string]bool{}
	var out []string
	base = strings.ToUpper(strings.TrimSpace(base))
	for _, c := range codes {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" || c == base || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// ensureRatesForQuotes 取指定日期的报价（相对 base）；persist=false 时不写入库（用于当日即时汇率）
func (s *Server) ensureRatesForQuotes(dateStr, base string, quotes []string, persist bool) (map[string]float64, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	uq := uniqueQuoteCodes(base, quotes...)
	if len(uq) == 0 {
		return map[string]float64{}, nil
	}
	loc := time.Local
	if dateStrIsToday(dateStr, loc) {
		return fetchFrankfurterRates(dateStr, base, uq)
	}
	grid, err := s.Store.GetExchangeRatesForDates(base, []string{dateStr})
	if err != nil {
		return nil, err
	}
	dm := grid[dateStr]
	if dm == nil {
		dm = map[string]float64{}
	}
	out := make(map[string]float64)
	var missing []string
	for _, q := range uq {
		if v, ok := dm[q]; ok {
			out[q] = v
		} else {
			missing = append(missing, q)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	rates, err := fetchFrankfurterRates(dateStr, base, missing)
	if err != nil {
		return nil, err
	}
	if persist {
		fetchedAt := time.Now().UTC().Format(time.RFC3339)
		if err := s.Store.ReplaceExchangeQuotesForDate(dateStr, base, rates, fetchedAt); err != nil {
			return nil, err
		}
	}
	for k, v := range rates {
		out[k] = v
	}
	return out, nil
}

func convertBetweenCurrencies(amount float64, from, to, base string, rates map[string]float64) (amountTo float64, cross float64, label string) {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	base = strings.ToUpper(strings.TrimSpace(base))
	if from == to {
		return amount, 1, fmt.Sprintf("1 %s = 1 %s", from, to)
	}
	toBase := func(amt float64, cur string) float64 {
		if cur == base {
			return amt
		}
		r := rates[cur]
		if r == 0 {
			return 0
		}
		return amt / r
	}
	fromBaseAmt := func(amt float64, cur string) float64 {
		if cur == base {
			return amt
		}
		r := rates[cur]
		return amt * r
	}
	inBase := toBase(amount, from)
	out := fromBaseAmt(inBase, to)
	if amount != 0 {
		cross = out / amount
	}
	label = fmt.Sprintf("1 %s = %s %s", from, formatRateCross(cross), to)
	return out, cross, label
}

func formatRateCross(v float64) string {
	if v == 0 || v != v {
		return "—"
	}
	a := v
	if a >= 1 {
		return fmt.Sprintf("%.4f", v)
	}
	s := fmt.Sprintf("%.8f", v)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

func (s *Server) handleExchangeRatesConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	dateStr := strings.TrimSpace(r.URL.Query().Get("date"))
	from := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("from")))
	to := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("to")))
	amtStr := strings.TrimSpace(r.URL.Query().Get("amount"))
	if dateStr == "" || from == "" || to == "" {
		http.Error(w, "date, from, to are required (YYYY-MM-DD, ISO codes)", http.StatusBadRequest)
		return
	}
	if len(from) != 3 || len(to) != 3 {
		http.Error(w, "from and to must be 3-letter ISO codes", http.StatusBadRequest)
		return
	}
	amount := 1.0
	if amtStr != "" {
		v, err := strconv.ParseFloat(amtStr, 64)
		if err != nil || v < 0 {
			http.Error(w, "invalid amount", http.StatusBadRequest)
			return
		}
		amount = v
	}
	fixed := strings.TrimSpace(r.URL.Query().Get("fixed"))
	if fixed != "" && fixed != "from" && fixed != "to" {
		http.Error(w, "fixed must be from or to", http.StatusBadRequest)
		return
	}
	if fixed == "" {
		fixed = "from"
	}
	st, err := s.Store.GetSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	base := strings.ToUpper(strings.TrimSpace(st.BaseCurrency))
	if base == "" {
		base = "CAD"
	}
	loc := time.Local
	if _, err := time.ParseInLocation("2006-01-02", dateStr, loc); err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}
	persist := !dateStrIsToday(dateStr, loc)
	rates, err := s.ensureRatesForQuotes(dateStr, base, []string{from, to}, persist)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var amountFrom, amountTo, cross float64
	var rateLabel string
	if fixed == "from" {
		amountFrom = amount
		if amountFrom == 0 && from != to {
			amountTo = 0
			_, cross, rateLabel = convertBetweenCurrencies(1, from, to, base, rates)
		} else {
			amountTo, cross, rateLabel = convertBetweenCurrencies(amountFrom, from, to, base, rates)
		}
	} else {
		amountTo = amount
		if amountTo == 0 && from != to {
			amountFrom = 0
			_, cross, rateLabel = convertBetweenCurrencies(1, from, to, base, rates)
		} else {
			amountFrom, _, _ = convertBetweenCurrencies(amountTo, to, from, base, rates)
			_, cross, rateLabel = convertBetweenCurrencies(1, from, to, base, rates)
		}
	}
	if cross == 0 && from != to {
		http.Error(w, "missing exchange rate for this pair or date", http.StatusBadRequest)
		return
	}
	live := dateStrIsToday(dateStr, loc)
	note := "Mid-market rate from Frankfurter"
	if live {
		note += " (not saved to database)"
	} else {
		note += " (cached when possible)"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"base":       base,
		"date":       dateStr,
		"from":       from,
		"to":         to,
		"amountFrom": amountFrom,
		"amountTo":   amountTo,
		"rate":       cross,
		"rateLabel":  rateLabel,
		"live":       live,
		"note":       note,
	})
}

func (s *Server) handleExchangeRatesCurrencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	names, err := fetchFrankfurterCurrencyNames()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	type row struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	var rows []row
	for code, name := range names {
		rows = append(rows, row{Code: code, Name: name})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Code < rows[j].Code })
	writeJSON(w, http.StatusOK, rows)
}
