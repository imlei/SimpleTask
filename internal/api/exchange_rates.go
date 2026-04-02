package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const frankfurterAPI = "https://api.frankfurter.dev"
const frankfurterHTTPTimeout = 25 * time.Second

type frankfurterRateRow struct {
	Date  string  `json:"date"`
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
}

type frankfurterCurrencyRow struct {
	ISOCode string `json:"iso_code"`
	Name    string `json:"name"`
}

func httpGetJSON(target string, v any) error {
	client := &http.Client{Timeout: frankfurterHTTPTimeout}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SimpleTask/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("frankfurter: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, v)
}

func fetchFrankfurterRates(requestedDate, base string, quotes []string) (map[string]float64, error) {
	u, err := url.Parse(frankfurterAPI + "/v2/rates")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("date", requestedDate)
	q.Set("base", base)
	if len(quotes) > 0 {
		var parts []string
		for _, c := range quotes {
			c = strings.ToUpper(strings.TrimSpace(c))
			if c == "" || strings.EqualFold(c, base) {
				continue
			}
			parts = append(parts, c)
		}
		if len(parts) > 0 {
			q.Set("quotes", strings.Join(parts, ","))
		}
	}
	u.RawQuery = q.Encode()
	var rows []frankfurterRateRow
	if err := httpGetJSON(u.String(), &rows); err != nil {
		return nil, err
	}
	out := make(map[string]float64)
	for _, r := range rows {
		qc := strings.ToUpper(strings.TrimSpace(r.Quote))
		if qc == "" || strings.EqualFold(qc, base) {
			continue
		}
		out[qc] = r.Rate
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no rates returned (check base and quote currency codes)")
	}
	return out, nil
}

func fetchFrankfurterCurrencyNames() (map[string]string, error) {
	var rows []frankfurterCurrencyRow
	if err := httpGetJSON(frankfurterAPI+"/v2/currencies", &rows); err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, r := range rows {
		c := strings.ToUpper(strings.TrimSpace(r.ISOCode))
		if c == "" {
			continue
		}
		out[c] = strings.TrimSpace(r.Name)
	}
	return out, nil
}

func (s *Server) handleExchangeRates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
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
	watchlist, err := s.Store.ListExchangeRateWatchlistCodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var filteredWatch []string
	for _, c := range watchlist {
		if strings.EqualFold(c, base) {
			continue
		}
		filteredWatch = append(filteredWatch, c)
	}
	watchlist = filteredWatch

	loc := time.Local
	dateStr := strings.TrimSpace(r.URL.Query().Get("date"))
	qFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	var anchor time.Time
	if dateStr == "" {
		anchor = time.Now().In(loc).AddDate(0, 0, -1)
	} else {
		d, err := time.ParseInLocation("2006-01-02", dateStr, loc)
		if err != nil {
			http.Error(w, "invalid date (use YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
		anchor = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
	}
	dates := lastNWeekdaysBeforeOrOn(anchor, 5, loc)
	if len(dates) == 0 {
		http.Error(w, "no dates in window", http.StatusBadRequest)
		return
	}
	todayStr := time.Now().In(loc).Format("2006-01-02")
	liveByDate := map[string]map[string]float64{}

	if len(watchlist) > 0 {
		for _, d := range dates {
			if d == todayStr {
				rates, err := fetchFrankfurterRates(d, base, watchlist)
				if err != nil {
					http.Error(w, "fetch exchange rates: "+err.Error(), http.StatusBadGateway)
					return
				}
				liveByDate[d] = rates
				continue
			}
			n, err := s.Store.CountCachedWatchlistQuotes(d, base, watchlist)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if n >= len(watchlist) {
				continue
			}
			rates, err := fetchFrankfurterRates(d, base, watchlist)
			if err != nil {
				http.Error(w, "fetch exchange rates: "+err.Error(), http.StatusBadGateway)
				return
			}
			fetchedAt := time.Now().UTC().Format(time.RFC3339)
			if err := s.Store.ReplaceExchangeQuotesForDate(d, base, rates, fetchedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	grid, err := s.Store.GetExchangeRatesForDates(base, dates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for d, rs := range liveByDate {
		if grid[d] == nil {
			grid[d] = make(map[string]float64)
		}
		for q, v := range rs {
			grid[d][q] = v
		}
	}
	var names map[string]string
	if len(watchlist) > 0 {
		names, err = fetchFrankfurterCurrencyNames()
		if err != nil {
			http.Error(w, "fetch currency names: "+err.Error(), http.StatusBadGateway)
			return
		}
	} else {
		names = map[string]string{}
	}

	type rowOut struct {
		Code  string             `json:"code"`
		Name  string             `json:"name"`
		Rates map[string]float64 `json:"rates"`
	}
	rows := make([]rowOut, 0, len(watchlist))
	for _, code := range watchlist {
		name := names[code]
		if name == "" {
			name = code
		}
		if qFilter != "" {
			if !strings.Contains(strings.ToLower(code), qFilter) && !strings.Contains(strings.ToLower(name), qFilter) {
				continue
			}
		}
		rates := make(map[string]float64)
		for _, dt := range dates {
			if dm, ok := grid[dt]; ok {
				if v, ok := dm[code]; ok {
					rates[dt] = v
				}
			}
		}
		rows = append(rows, rowOut{Code: code, Name: name, Rates: rates})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"base":     base,
		"dates":    dates,
		"rows":     rows,
		"source":   frankfurterAPI,
		"sourceDoc": "https://frankfurter.dev/",
	})
}
