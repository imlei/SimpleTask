// Package writecheque provides cheque generation logic: MICR line building,
// amount-to-words conversion, and the core ChequeData type used for rendering.
package writecheque

import (
	"fmt"
	"math"
	"strings"
)

// ChequeData holds all fields needed to render a printable cheque.
type ChequeData struct {
	// Issuer (company)
	CompanyName     string
	CompanyStreet   string
	CompanyCity     string
	CompanyProvince string
	CompanyPostal   string
	// Cheque fields
	CheckNo     string
	Date        string // display format: 2026/04/16
	Payee       string
	AmountBox   string // e.g. "CAD 2,500.00"
	AmountWords string // e.g. "TWO THOUSAND FIVE HUNDRED AND 00/100 DOLLARS"
	Memo        string
	MICRLine    string
	// Bank info (printed on cheque face and stub)
	BankName       string
	BankAddress    string
	BankCity       string
	BankProvince   string
	BankPostalCode string
	// Pass-through
	Currency string
}

// ---- MICR generation -------------------------------------------------------

const micrDelim = "\u2446" // E13B field separator character

// BuildMICR generates the MICR line from bank parameters.
// country must be "CA", "US", or "EU".
// If micrOverride is non-empty it is returned as-is.
func BuildMICR(country, institution, transit, routingABA, account, iban, micrOverride, chequeNo string) string {
	if ovr := strings.TrimSpace(micrOverride); ovr != "" {
		return ovr
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	chequeNo = strings.TrimSpace(chequeNo)

	switch country {
	case "US":
		rt := padLeft(digitsOnly(routingABA), 9)
		ac := digitsOnly(account)
		if len(rt) != 9 || ac == "" {
			return ""
		}
		ch := padLeft(digitsOnly(chequeNo), 6)
		return micrDelim + rt + micrDelim + ac + micrDelim + ch + micrDelim

	case "EU":
		return "" // IBAN/SEPA does not use E13B MICR

	default: // CA (CPA standard)
		inst := padLeft(digitsOnly(institution), 3)
		tr := padLeft(digitsOnly(transit), 5)
		block8 := inst + tr
		ac := padLeft(digitsOnly(account), 12)
		if ac == "" {
			return ""
		}
		ch := padLeft(digitsOnly(chequeNo), 5)
		return micrDelim + block8 + micrDelim + ac + micrDelim + ch + micrDelim
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

// ---- Amount to words -------------------------------------------------------

var small = []string{
	"zero", "one", "two", "three", "four", "five", "six", "seven",
	"eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen",
	"fifteen", "sixteen", "seventeen", "eighteen", "nineteen",
}
var tens = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}

func wordsUnder100(n int) string {
	if n < 20 {
		return small[n]
	}
	t := n / 10
	o := n % 10
	if o == 0 {
		return tens[t]
	}
	return tens[t] + "-" + small[o]
}

func wordsUnder1000(n int) string {
	h := n / 100
	rest := n % 100
	var parts []string
	if h > 0 {
		parts = append(parts, small[h]+" hundred")
	}
	if rest > 0 {
		parts = append(parts, wordsUnder100(rest))
	}
	return strings.Join(parts, " ")
}

func intToWords(n int64) string {
	if n == 0 {
		return "zero"
	}
	bi := int(n / 1_000_000_000)
	mi := int((n % 1_000_000_000) / 1_000_000)
	th := int((n % 1_000_000) / 1_000)
	re := int(n % 1_000)
	var parts []string
	if bi > 0 {
		parts = append(parts, wordsUnder1000(bi)+" billion")
	}
	if mi > 0 {
		parts = append(parts, wordsUnder1000(mi)+" million")
	}
	if th > 0 {
		parts = append(parts, wordsUnder1000(th)+" thousand")
	}
	if re > 0 {
		parts = append(parts, wordsUnder1000(re))
	}
	return strings.Join(parts, " ")
}

// AmountToWords converts a monetary amount to the English cheque words format.
// e.g. 2500.00 → "TWO THOUSAND FIVE HUNDRED AND 00/100 DOLLARS"
func AmountToWords(amount float64) string {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return ""
	}
	if amount >= 1e12 {
		return "AMOUNT TOO LARGE"
	}
	centsTotal := int64(math.Round(amount * 100))
	dollars := centsTotal / 100
	cents := centsTotal % 100
	w := intToWords(dollars)
	if w == "" {
		return ""
	}
	line := fmt.Sprintf("%s and %02d/100 dollars", w, cents)
	return strings.ToUpper(line)
}

// FormatAmountBox returns the amount box string, e.g. "CAD 2,500.00"
func FormatAmountBox(amount float64, currency string) string {
	if currency == "" {
		currency = "CAD"
	}
	// Format with thousands separator
	abs := math.Abs(amount)
	intPart := int64(abs)
	fracPart := int(math.Round((abs-float64(intPart))*100)) % 100
	intStr := formatInt(intPart)
	return fmt.Sprintf("%s %s.%02d", strings.ToUpper(currency), intStr, fracPart)
}

func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	start := len(s) % 3
	if start > 0 {
		result.WriteString(s[:start])
	}
	for i := start; i < len(s); i += 3 {
		if i > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
