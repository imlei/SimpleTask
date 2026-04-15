package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"simpletask/internal/crypto"
	"simpletask/internal/models"
)

// sinMask returns ***-***-XXX where XXX = last 3 digits.
// CRA standard: never expose full SIN via API.
func sinMask(plain string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, plain)
	if len(digits) < 3 {
		return "***-***-***"
	}
	return "***-***-" + digits[len(digits)-3:]
}

func (s *Store) ListPayrollEmployees(companyID, statusFilter string) []models.PayrollEmployee {
	s.mu.Lock()
	defer s.mu.Unlock()

	q := `SELECT id, company_id, legal_name, nickname, email, mobile,
	             member_type, position, status, province, sin_encrypted,
	             date_of_birth, hire_date, salary_type, pay_rate, pay_rate_unit,
	             pays_per_year, pay_frequency, hours_per_week,
	             td1_federal, td1_provincial,
	             paid_ytd_other_payroll, auto_vacation,
	             created_at, updated_at,
	             COALESCE(address,''), COALESCE(gender,''), COALESCE(marital_status,''), COALESCE(notes,'')
	      FROM payroll_employees
	      WHERE company_id = ?`
	args := []any{companyID}

	if statusFilter != "" && statusFilter != "all" {
		q += ` AND status = ?`
		args = append(args, statusFilter)
	}
	q += ` ORDER BY legal_name ASC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []models.PayrollEmployee{}
	}
	defer rows.Close()

	var list []models.PayrollEmployee
	for rows.Next() {
		e, sinEnc := s.scanEmployee(rows.Scan)
		e.SINMasked = s.decryptSINMasked(sinEnc)
		list = append(list, e)
	}
	if list == nil {
		return []models.PayrollEmployee{}
	}
	return list
}

func (s *Store) GetPayrollEmployee(id string) (models.PayrollEmployee, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(`
		SELECT id, company_id, legal_name, nickname, email, mobile,
		       member_type, position, status, province, sin_encrypted,
		       date_of_birth, hire_date, salary_type, pay_rate, pay_rate_unit,
		       pays_per_year, pay_frequency, hours_per_week,
		       td1_federal, td1_provincial,
		       paid_ytd_other_payroll, auto_vacation,
		       created_at, updated_at,
		       COALESCE(address,''), COALESCE(gender,''), COALESCE(marital_status,''), COALESCE(notes,'')
		FROM payroll_employees WHERE id = ?`, id)

	var sinEnc string
	e, sinEncFromScan := s.scanEmployeeRow(row.Scan)
	sinEnc = sinEncFromScan
	if e.ID == "" {
		return e, ErrNotFound
	}
	e.SINMasked = s.decryptSINMasked(sinEnc)
	return e, nil
}

func (s *Store) CreatePayrollEmployee(e models.PayrollEmployee) (models.PayrollEmployee, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e.ID = s.nextEmployeeID()
	now := time.Now().UTC().Format(time.RFC3339)
	e.CreatedAt = now
	e.UpdatedAt = now
	if e.Status == "" {
		e.Status = "active"
	}
	if e.TD1Federal == 0 {
		e.TD1Federal = 16129 // 2025 basic personal amount
	}
	if e.PaysPerYear == 0 {
		e.PaysPerYear = 26
	}

	sinEnc := ""
	if plain := strings.TrimSpace(e.SIN); plain != "" {
		var err error
		sinEnc, err = crypto.Encrypt(s.encKey, []byte(plain))
		if err != nil {
			return e, fmt.Errorf("failed to encrypt SIN: %w", err)
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO payroll_employees
		  (id, company_id, legal_name, nickname, email, mobile,
		   member_type, position, status, province, sin_encrypted,
		   date_of_birth, hire_date, salary_type, pay_rate, pay_rate_unit,
		   pays_per_year, pay_frequency, hours_per_week,
		   td1_federal, td1_provincial,
		   paid_ytd_other_payroll, auto_vacation,
		   created_at, updated_at,
		   address, gender, marital_status, notes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.CompanyID, e.LegalName, e.Nickname, e.Email, e.Mobile,
		e.MemberType, e.Position, e.Status, e.Province, sinEnc,
		e.DateOfBirth, e.HireDate, e.SalaryType, e.PayRate, e.PayRateUnit,
		e.PaysPerYear, e.PayFrequency, e.HoursPerWeek,
		e.TD1Federal, e.TD1Provincial,
		boolInt(e.PaidYTDOtherPayroll), boolInt(e.AutoVacation),
		e.CreatedAt, e.UpdatedAt,
		e.Address, e.Gender, e.MaritalStatus, e.Notes,
	)
	if err != nil {
		return e, err
	}

	e.SIN = "" // never return SIN in response
	e.SINMasked = s.decryptSINMasked(sinEnc)
	return e, nil
}

func (s *Store) UpdatePayrollEmployee(id string, patch models.PayrollEmployee) (models.PayrollEmployee, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var createdAt, existingSINEnc string
	err := s.db.QueryRow(`SELECT created_at, sin_encrypted FROM payroll_employees WHERE id = ?`, id).
		Scan(&createdAt, &existingSINEnc)
	if err != nil {
		return patch, ErrNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)
	patch.ID = id
	patch.CreatedAt = createdAt
	patch.UpdatedAt = now

	// Only re-encrypt SIN if a new one is provided
	sinEnc := existingSINEnc
	if plain := strings.TrimSpace(patch.SIN); plain != "" {
		sinEnc, err = crypto.Encrypt(s.encKey, []byte(plain))
		if err != nil {
			return patch, fmt.Errorf("failed to encrypt SIN: %w", err)
		}
	}

	_, err = s.db.Exec(`
		UPDATE payroll_employees SET
		  legal_name=?, nickname=?, email=?, mobile=?,
		  member_type=?, position=?, status=?, province=?, sin_encrypted=?,
		  date_of_birth=?, hire_date=?, salary_type=?, pay_rate=?, pay_rate_unit=?,
		  pays_per_year=?, pay_frequency=?, hours_per_week=?,
		  td1_federal=?, td1_provincial=?,
		  paid_ytd_other_payroll=?, auto_vacation=?,
		  address=?, gender=?, marital_status=?, notes=?,
		  updated_at=?
		WHERE id=?`,
		patch.LegalName, patch.Nickname, patch.Email, patch.Mobile,
		patch.MemberType, patch.Position, patch.Status, patch.Province, sinEnc,
		patch.DateOfBirth, patch.HireDate, patch.SalaryType, patch.PayRate, patch.PayRateUnit,
		patch.PaysPerYear, patch.PayFrequency, patch.HoursPerWeek,
		patch.TD1Federal, patch.TD1Provincial,
		boolInt(patch.PaidYTDOtherPayroll), boolInt(patch.AutoVacation),
		patch.Address, patch.Gender, patch.MaritalStatus, patch.Notes,
		now, id,
	)
	if err != nil {
		return patch, err
	}

	patch.SIN = ""
	patch.SINMasked = s.decryptSINMasked(sinEnc)
	return patch, nil
}

func (s *Store) TerminatePayrollEmployee(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(
		`UPDATE payroll_employees SET status='terminated', updated_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── helpers ────────────────────────────────────────────────────────────────

type scanFunc func(dest ...any) error

func (s *Store) scanEmployee(scan scanFunc) (models.PayrollEmployee, string) {
	var e models.PayrollEmployee
	var sinEnc string
	var paidYTD, autoVac int
	_ = scan(
		&e.ID, &e.CompanyID, &e.LegalName, &e.Nickname, &e.Email, &e.Mobile,
		&e.MemberType, &e.Position, &e.Status, &e.Province, &sinEnc,
		&e.DateOfBirth, &e.HireDate, &e.SalaryType, &e.PayRate, &e.PayRateUnit,
		&e.PaysPerYear, &e.PayFrequency, &e.HoursPerWeek,
		&e.TD1Federal, &e.TD1Provincial,
		&paidYTD, &autoVac,
		&e.CreatedAt, &e.UpdatedAt,
		&e.Address, &e.Gender, &e.MaritalStatus, &e.Notes,
	)
	e.PaidYTDOtherPayroll = paidYTD != 0
	e.AutoVacation = autoVac != 0
	return e, sinEnc
}

func (s *Store) scanEmployeeRow(scan scanFunc) (models.PayrollEmployee, string) {
	return s.scanEmployee(scan)
}

func (s *Store) decryptSINMasked(sinEnc string) string {
	if sinEnc == "" {
		return ""
	}
	plain, err := crypto.Decrypt(s.encKey, sinEnc)
	if err != nil || plain == "" {
		return "***-***-***"
	}
	return sinMask(plain)
}

func (s *Store) nextEmployeeID() string {
	rows, err := s.db.Query(`SELECT id FROM payroll_employees WHERE id LIKE 'EMP%'`)
	if err != nil {
		return "EMP00001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "EMP") {
			if v, err := strconv.Atoi(strings.TrimPrefix(id, "EMP")); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("EMP%05d", max+1)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
