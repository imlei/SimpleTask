// Package calculator implements CRA payroll deduction formulas per T4127 / T4001 (Rev. 25, 2025).
// All monetary values are in CAD. Rates must be reviewed and updated each tax year.
package calculator

// TaxYear holds all CRA parameters for a given year.
// Source: T4001(E) Rev. 25 (2025), T4127 Payroll Deductions Formulas 2025.
type TaxYear struct {
	Year int

	// ── CPP (Canada Pension Plan) ────────────────────────────
	CPPRate              float64 // 5.95% (2025)
	YMPE                 float64 // $71,300  — Year's Maximum Pensionable Earnings
	YBE                  float64 // $3,500   — Year's Basic Exemption
	CPPMaxEmployeeAnnual float64 // $4,034.10

	// CPP2 (second additional contribution, 2024+)
	CPP2Rate             float64 // 4%
	YAMPE                float64 // $81,200  — Year's Additional Maximum Pensionable Earnings
	CPP2MaxEmployeeAnnual float64

	// QPP (Quebec Pension Plan) — replaces CPP for QC employees
	QPPRate              float64 // 6.40%
	QPP2Rate             float64 // 4%
	QPPMaxEmployeeAnnual float64
	QPP2MaxEmployeeAnnual float64

	// ── EI (Employment Insurance) ────────────────────────────
	EIRate               float64 // 1.64% (non-QC)
	EIRateQC             float64 // 1.31% (QC — QPIP covers parental)
	MaxInsurableEarnings float64 // $65,700
	EIMaxEmployeeAnnual  float64 // $1,077.48 (non-QC)
	EIMaxEmployeeAnnualQC float64 // $860.67 (QC)
	EIEmployerFactor     float64 // 1.4

	// ── Federal Income Tax ──────────────────────────────────
	FederalBPA    float64         // $16,129 — Basic Personal Amount
	FederalBands  []TaxBand
	FederalRate   float64 // bottom bracket rate, used for credit calculation (15%)

	// ── Provincial Rates ────────────────────────────────────
	Provincial map[string]ProvincialRates
}

// TaxBand describes one bracket: income up to Upper is taxed at Rate.
// Upper == 0 means "no upper limit" (top bracket).
type TaxBand struct {
	Upper float64 // 0 = unlimited
	Rate  float64
}

// ProvincialRates holds one province's income tax parameters.
type ProvincialRates struct {
	BPA   float64
	Bands []TaxBand
	// BottomRate is used for CPP/EI provincial credit calculation.
	BottomRate float64
	// SurtaxThresholds: ON has surtax; others are nil.
	SurtaxThresholds []SurtaxThreshold
}

type SurtaxThreshold struct {
	Over float64
	Rate float64 // additional % applied to provincial tax
}

// Rates2025 returns the official 2025 parameters.
// Source: T4001(E) Rev. 25, T4127 Payroll Deductions Formulas (113th edition, 2025).
func Rates2025() TaxYear {
	return TaxYear{
		Year: 2025,

		// CPP
		CPPRate:               0.0595,
		YMPE:                  71_300,
		YBE:                   3_500,
		CPPMaxEmployeeAnnual:  4_034.10,
		CPP2Rate:              0.04,
		YAMPE:                 81_200,
		CPP2MaxEmployeeAnnual: (81_200 - 71_300) * 0.04, // $396.00

		// QPP (Quebec)
		QPPRate:               0.0640,
		QPP2Rate:              0.04,
		QPPMaxEmployeeAnnual:  (71_300 - 3_500) * 0.0640, // $4,323.20
		QPP2MaxEmployeeAnnual: (81_200 - 71_300) * 0.04,  // $396.00

		// EI
		EIRate:               0.0164,
		EIRateQC:             0.0131,
		MaxInsurableEarnings: 65_700,
		EIMaxEmployeeAnnual:  1_077.48,
		EIMaxEmployeeAnnualQC: 860.67,
		EIEmployerFactor:     1.4,

		// Federal
		FederalBPA:  16_129,
		FederalRate: 0.15,
		FederalBands: []TaxBand{
			{Upper: 57_375, Rate: 0.15},
			{Upper: 114_750, Rate: 0.205},
			{Upper: 158_519, Rate: 0.26},
			{Upper: 220_000, Rate: 0.29},
			{Upper: 0, Rate: 0.33},
		},

		Provincial: map[string]ProvincialRates{
			"BC": {
				BPA:        11_981,
				BottomRate: 0.0506,
				Bands: []TaxBand{
					{Upper: 45_654, Rate: 0.0506},
					{Upper: 91_310, Rate: 0.0770},
					{Upper: 104_835, Rate: 0.1050},
					{Upper: 127_299, Rate: 0.1229},
					{Upper: 172_602, Rate: 0.1470},
					{Upper: 240_716, Rate: 0.1680},
					{Upper: 0, Rate: 0.2050},
				},
			},
			"ON": {
				BPA:        11_865,
				BottomRate: 0.0505,
				Bands: []TaxBand{
					{Upper: 51_446, Rate: 0.0505},
					{Upper: 102_894, Rate: 0.0915},
					{Upper: 150_000, Rate: 0.1116},
					{Upper: 220_000, Rate: 0.1216},
					{Upper: 0, Rate: 0.1316},
				},
				SurtaxThresholds: []SurtaxThreshold{
					{Over: 5_710, Rate: 0.20},
					{Over: 7_307, Rate: 0.36},
				},
			},
			"AB": {
				BPA:        21_003,
				BottomRate: 0.10,
				Bands: []TaxBand{
					{Upper: 148_269, Rate: 0.10},
					{Upper: 177_922, Rate: 0.12},
					{Upper: 237_230, Rate: 0.13},
					{Upper: 355_845, Rate: 0.14},
					{Upper: 0, Rate: 0.15},
				},
			},
			"QC": {
				// Federal TD1 applies; provincial uses Form TP-1015.3-V.
				// Using QC 2025 brackets (Revenu Québec).
				BPA:        17_183,
				BottomRate: 0.14,
				Bands: []TaxBand{
					{Upper: 51_780, Rate: 0.14},
					{Upper: 103_545, Rate: 0.19},
					{Upper: 126_000, Rate: 0.24},
					{Upper: 0, Rate: 0.2575},
				},
			},
			"MB": {
				BPA:        15_780,
				BottomRate: 0.108,
				Bands: []TaxBand{
					{Upper: 47_000, Rate: 0.108},
					{Upper: 100_000, Rate: 0.1275},
					{Upper: 0, Rate: 0.174},
				},
			},
			"SK": {
				BPA:        17_661,
				BottomRate: 0.105,
				Bands: []TaxBand{
					{Upper: 49_720, Rate: 0.105},
					{Upper: 142_058, Rate: 0.125},
					{Upper: 0, Rate: 0.145},
				},
			},
			"NB": {
				// NB reformed brackets took effect 2023; 2025 rates per T4127 (113th ed.)
				BPA:        12_458,
				BottomRate: 0.094,
				Bands: []TaxBand{
					{Upper: 47_715, Rate: 0.094},
					{Upper: 95_431, Rate: 0.1482},
					{Upper: 176_756, Rate: 0.1652},
					{Upper: 0, Rate: 0.1784},
				},
			},
			"NS": {
				BPA:        8_481,
				BottomRate: 0.0879,
				Bands: []TaxBand{
					{Upper: 29_590, Rate: 0.0879},
					{Upper: 59_180, Rate: 0.1495},
					{Upper: 93_000, Rate: 0.1667},
					{Upper: 150_000, Rate: 0.175},
					{Upper: 0, Rate: 0.21},
				},
			},
			"NL": {
				BPA:        10_818,
				BottomRate: 0.087,
				Bands: []TaxBand{
					{Upper: 43_198, Rate: 0.087},
					{Upper: 86_395, Rate: 0.145},
					{Upper: 154_244, Rate: 0.158},
					{Upper: 215_943, Rate: 0.178},
					{Upper: 275_870, Rate: 0.198},
					{Upper: 0, Rate: 0.213},
				},
			},
			"PE": {
				BPA:        12_000,
				BottomRate: 0.0965,
				Bands: []TaxBand{
					{Upper: 32_656, Rate: 0.0965},
					{Upper: 64_313, Rate: 0.1363},
					{Upper: 105_000, Rate: 0.1665},
					{Upper: 140_000, Rate: 0.18},
					{Upper: 0, Rate: 0.1875},
				},
			},
			"NT": {
				BPA:        16_593,
				BottomRate: 0.059,
				Bands: []TaxBand{
					{Upper: 50_597, Rate: 0.059},
					{Upper: 101_198, Rate: 0.086},
					{Upper: 164_525, Rate: 0.122},
					{Upper: 0, Rate: 0.1405},
				},
			},
			"NU": {
				BPA:        17_925,
				BottomRate: 0.04,
				Bands: []TaxBand{
					{Upper: 53_268, Rate: 0.04},
					{Upper: 106_537, Rate: 0.07},
					{Upper: 173_205, Rate: 0.09},
					{Upper: 0, Rate: 0.115},
				},
			},
			"YT": {
				// Yukon BPA mirrors federal BPA (T4127 2025)
				BPA:        16_129,
				BottomRate: 0.064,
				Bands: []TaxBand{
					{Upper: 57_375, Rate: 0.064},
					{Upper: 114_750, Rate: 0.09},
					{Upper: 500_000, Rate: 0.109},
					{Upper: 0, Rate: 0.128},
				},
			},
		},
	}
}
