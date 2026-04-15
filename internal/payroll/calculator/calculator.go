package calculator

import (
	"math"

	"github.com/shopspring/decimal"
)

// Input holds all values needed to compute one employee's payroll for a single period.
type Input struct {
	Province    string  // 2-letter CRA province code
	PayPeriods  int     // pays per year: 52 | 26 | 24 | 12
	GrossPay    float64 // this period's gross pay (before any deductions) — used for income tax
	TD1Federal  float64 // federal personal claim amount (default = BPA)
	TD1Prov     float64 // provincial personal claim amount (0 = use BPA default)

	// Optional earnings-type overrides (0 = fall back to GrossPay).
	// Use when additional earnings have different CPP/EI treatment than base pay.
	// E.g. non-cash benefits count for income tax but not EI.
	CPPGross float64 // earnings subject to CPP/QPP (0 = use GrossPay)
	EIGross  float64 // insurable earnings for EI (0 = use GrossPay)

	// YTD totals BEFORE this period (used for annual max checks).
	// Caller must aggregate from prior periods in the same calendar year.
	YTDGross  float64 // year-to-date gross pay
	YTDCPPEe  float64 // year-to-date CPP (or QPP) employee contributions
	YTDCPP2Ee float64 // year-to-date CPP2 employee contributions
	YTDEIEe   float64 // year-to-date EI employee premiums
}

// Result holds the computed deductions and net pay for one period.
type Result struct {
	GrossPay float64

	// Employee deductions
	CPPEmployee  float64 // CPP (or QPP if QC)
	CPP2Employee float64
	EIEmployee   float64
	FederalTax   float64
	ProvincialTax float64
	TotalDeductions float64
	NetPay          float64

	// Employer contributions (for PD7A remittance reporting)
	CPPEmployer  float64
	CPP2Employer float64
	EIEmployer   float64

	// Remittance total = employee CPP+CPP2+EI+IT + employer CPP+CPP2+EI
	RemittanceTotal float64

	// Informational
	IsQC         bool
	PayPeriods   int
	Province     string
}

// Calculate computes all payroll deductions per CRA T4127 (2025) formulas.
func Calculate(in Input, rates TaxYear) Result {
	if in.PayPeriods <= 0 {
		in.PayPeriods = 26
	}

	gross := round2(in.GrossPay)
	isQC := in.Province == "QC"

	// Resolve optional per-type gross overrides.
	cppGross := gross
	if in.CPPGross > 0 {
		cppGross = round2(in.CPPGross)
	}
	eiGross := gross
	if in.EIGross > 0 {
		eiGross = round2(in.EIGross)
	}

	// ── CPP / QPP ───────────────────────────────────────────────────────────────
	cpp, cpp2 := calcCPP(cppGross, in.YTDGross, in.PayPeriods, in.YTDCPPEe, in.YTDCPP2Ee, isQC, rates)

	// ── EI ──────────────────────────────────────────────────────────────────────
	ei := calcEI(eiGross, in.YTDGross, in.YTDEIEe, isQC, rates)

	// ── Income Tax (T4127 annualized method) ────────────────────────────────────
	fedTax, provTax := calcTax(gross, in.PayPeriods, cpp, ei, in.TD1Federal, in.TD1Prov, in.Province, isQC, rates)

	// ── Employer contributions ───────────────────────────────────────────────────
	cppEr := round2(cpp)   // 1:1 match
	cpp2Er := round2(cpp2) // 1:1 match
	eiEr := round2(ei * rates.EIEmployerFactor)

	totalDed := round2(cpp + cpp2 + ei + fedTax + provTax)
	net := round2(gross - totalDed)

	remittance := round2(cpp + cppEr + cpp2 + cpp2Er + ei + eiEr + fedTax + provTax)

	return Result{
		GrossPay:        gross,
		CPPEmployee:     cpp,
		CPP2Employee:    cpp2,
		EIEmployee:      ei,
		FederalTax:      fedTax,
		ProvincialTax:   provTax,
		TotalDeductions: totalDed,
		NetPay:          net,
		CPPEmployer:     cppEr,
		CPP2Employer:    cpp2Er,
		EIEmployer:      eiEr,
		RemittanceTotal: remittance,
		IsQC:            isQC,
		PayPeriods:      in.PayPeriods,
		Province:        in.Province,
	}
}

// ── CPP / QPP ──────────────────────────────────────────────────────────────────

// calcCPP computes CPP (or QPP) employee contributions for one pay period.
// ytdGross is the employee's total gross earnings in the current calendar year
// BEFORE this period — used to correctly scope the CPP2 band.
func calcCPP(gross, ytdGross float64, periods int, ytdCPP, ytdCPP2 float64, isQC bool, r TaxYear) (cpp, cpp2 float64) {
	// ── CPP1 / QPP1 ────────────────────────────────────────────────────────────
	rate := r.CPPRate
	maxAnnual := r.CPPMaxEmployeeAnnual
	if isQC {
		rate = r.QPPRate
		maxAnnual = r.QPPMaxEmployeeAnnual
	}

	// Period exemption = YBE / pays_per_year  (T4001 §2.2)
	periodExemption := round4(r.YBE / float64(periods))

	// CPP base = max(0, gross - periodExemption)
	base := gross - periodExemption
	if base < 0 {
		base = 0
	}
	cpp = round4(base * rate)

	// Annual maximum cap
	remaining := round4(maxAnnual - ytdCPP)
	if remaining < 0 {
		remaining = 0
	}
	if cpp > remaining {
		cpp = remaining
	}
	cpp = round2(cpp)

	// ── CPP2 / QPP2 (T4001 §2.3) ───────────────────────────────────────────────
	// CPP2 applies to pensionable earnings in the band (YMPE, YAMPE].
	// Use the YTD running-total method: determine what portion of this period's
	// gross falls inside that band based on cumulative YTD gross.
	//
	//   ytdAfter = ytdGross + gross  (YTD after this period)
	//   cpp2Base = min(ytdAfter, YAMPE) - max(ytdGross, YMPE)
	//            = portion of this period's earnings inside (YMPE, YAMPE]
	//
	// No period exemption applies to CPP2.
	cpp2Rate := r.CPP2Rate
	maxCPP2 := r.CPP2MaxEmployeeAnnual
	if isQC {
		cpp2Rate = r.QPP2Rate
		maxCPP2 = r.QPP2MaxEmployeeAnnual
	}

	ympe := r.YMPE
	yampe := r.YAMPE
	ytdAfter := ytdGross + gross
	cpp2Base := math.Max(0, math.Min(ytdAfter, yampe)-math.Max(ytdGross, ympe))
	if cpp2Base > 0 {
		cpp2 = round4(cpp2Base * cpp2Rate)
		remaining2 := round4(maxCPP2 - ytdCPP2)
		if remaining2 < 0 {
			remaining2 = 0
		}
		if cpp2 > remaining2 {
			cpp2 = remaining2
		}
		cpp2 = round2(cpp2)
	}

	return cpp, cpp2
}

// ── EI ─────────────────────────────────────────────────────────────────────────

func calcEI(gross, ytdGross, ytdEI float64, isQC bool, r TaxYear) float64 {
	rate := r.EIRate
	maxPremium := r.EIMaxEmployeeAnnual
	if isQC {
		rate = r.EIRateQC
		maxPremium = r.EIMaxEmployeeAnnualQC
	}

	// Insurable earnings this period: capped at remaining insurable room
	remaining := r.MaxInsurableEarnings - ytdGross
	if remaining <= 0 {
		return 0
	}
	insurable := gross
	if insurable > remaining {
		insurable = remaining
	}

	ei := round4(insurable * rate)

	// Annual premium cap
	premRemaining := round4(maxPremium - ytdEI)
	if premRemaining < 0 {
		premRemaining = 0
	}
	if ei > premRemaining {
		ei = premRemaining
	}
	return round2(ei)
}

// ── Income Tax (T4127 Annualized Method) ─────────────────────────────────────────

func calcTax(gross float64, periods int, cppEe, eiEe, td1Fed, td1Prov float64,
	province string, isQC bool, r TaxYear) (fedTax, provTax float64) {

	// Step 1: Annualized gross
	A := gross * float64(periods)

	// Federal credits
	if td1Fed <= 0 {
		td1Fed = r.FederalBPA
	}
	K1 := td1Fed * r.FederalRate  // basic personal amount credit
	K2 := cppEe * float64(periods) * r.FederalRate // CPP credit (annual)
	K3 := eiEe * float64(periods) * r.FederalRate  // EI credit (annual)

	T1annual := grossTaxFromBands(A, r.FederalBands) - K1 - K2 - K3
	if T1annual < 0 {
		T1annual = 0
	}
	fedTax = round2(T1annual / float64(periods))

	// Provincial tax
	provRates, ok := r.Provincial[province]
	if !ok {
		// Unknown province: use federal rate as fallback estimate
		provTax = round2(fedTax * 0.40)
		return
	}
	if td1Prov <= 0 {
		td1Prov = provRates.BPA
	}
	K1P := td1Prov * provRates.BottomRate
	K2P := cppEe * float64(periods) * provRates.BottomRate
	K3P := eiEe * float64(periods) * provRates.BottomRate

	T2annual := grossTaxFromBands(A, provRates.Bands) - K1P - K2P - K3P
	if T2annual < 0 {
		T2annual = 0
	}

	// Ontario surtax
	if len(provRates.SurtaxThresholds) > 0 {
		T2annual = applyONSurtax(T2annual, provRates.SurtaxThresholds)
	}

	provTax = round2(T2annual / float64(periods))
	return
}

// grossTaxFromBands calculates tax from graduated brackets (no credits applied).
func grossTaxFromBands(income float64, bands []TaxBand) float64 {
	if income <= 0 {
		return 0
	}
	tax := 0.0
	prev := 0.0
	for _, b := range bands {
		upper := b.Upper
		if upper == 0 {
			upper = math.MaxFloat64
		}
		if income <= prev {
			break
		}
		chunk := math.Min(income, upper) - prev
		if chunk > 0 {
			tax += chunk * b.Rate
		}
		prev = upper
	}
	return tax
}

// applyONSurtax applies Ontario surtax to an annual provincial tax amount.
// Rules: 20% on provincial tax > $5,710; additional 36% on provincial tax > $7,307.
func applyONSurtax(annualProvTax float64, thresholds []SurtaxThreshold) float64 {
	surtax := 0.0
	for _, t := range thresholds {
		if annualProvTax > t.Over {
			surtax += (annualProvTax - t.Over) * t.Rate
		}
	}
	return annualProvTax + surtax
}

// ── Rounding helpers ─────────────────────────────────────────────────────────────

func round2(v float64) float64 {
	d := decimal.NewFromFloat(v).Round(2)
	f, _ := d.Float64()
	return f
}

func round4(v float64) float64 {
	d := decimal.NewFromFloat(v).Round(4)
	f, _ := d.Float64()
	return f
}
