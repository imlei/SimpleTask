package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tasktracker/internal/models"
)

func (s *Store) ListBankAccounts() ([]models.BankAccount, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	def := ""
	_ = s.db.QueryRow(`SELECT COALESCE(default_bank_account_id,'') FROM app_settings WHERE id=1`).Scan(&def)
	rows, err := s.db.Query(`SELECT id, label, micr_country, bank_institution, bank_transit, bank_routing_aba, bank_account,
		bank_cheque_number, micr_line_override, default_cheque_currency
		FROM bank_accounts ORDER BY id`)
	if err != nil {
		return nil, def, err
	}
	defer rows.Close()
	out := make([]models.BankAccount, 0)
	for rows.Next() {
		var b models.BankAccount
		if err := rows.Scan(&b.ID, &b.Label, &b.MICRCountry, &b.BankInstitution, &b.BankTransit, &b.BankRoutingABA, &b.BankAccount,
			&b.BankChequeNumber, &b.MICRLineOverride, &b.DefaultChequeCurrency); err != nil {
			continue
		}
		out = append(out, b)
	}
	return out, strings.TrimSpace(def), rows.Err()
}

func (s *Store) GetBankAccount(id string) (models.BankAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return models.BankAccount{}, ErrNotFound
	}
	var b models.BankAccount
	err := s.db.QueryRow(`SELECT id, label, micr_country, bank_institution, bank_transit, bank_routing_aba, bank_account,
		bank_cheque_number, micr_line_override, default_cheque_currency
		FROM bank_accounts WHERE id=?`, id).Scan(
		&b.ID, &b.Label, &b.MICRCountry, &b.BankInstitution, &b.BankTransit, &b.BankRoutingABA, &b.BankAccount,
		&b.BankChequeNumber, &b.MICRLineOverride, &b.DefaultChequeCurrency,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BankAccount{}, ErrNotFound
	}
	if err != nil {
		return models.BankAccount{}, err
	}
	return b, nil
}

func (s *Store) GetDefaultBankAccount() (models.BankAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	def := ""
	_ = s.db.QueryRow(`SELECT COALESCE(default_bank_account_id,'') FROM app_settings WHERE id=1`).Scan(&def)
	def = strings.TrimSpace(def)
	if def != "" {
		var b models.BankAccount
		err := s.db.QueryRow(`SELECT id, label, micr_country, bank_institution, bank_transit, bank_routing_aba, bank_account,
			bank_cheque_number, micr_line_override, default_cheque_currency
			FROM bank_accounts WHERE id=?`, def).Scan(
			&b.ID, &b.Label, &b.MICRCountry, &b.BankInstitution, &b.BankTransit, &b.BankRoutingABA, &b.BankAccount,
			&b.BankChequeNumber, &b.MICRLineOverride, &b.DefaultChequeCurrency,
		)
		if err == nil {
			return b, nil
		}
	}
	// fallback: first account
	var b models.BankAccount
	err := s.db.QueryRow(`SELECT id, label, micr_country, bank_institution, bank_transit, bank_routing_aba, bank_account,
		bank_cheque_number, micr_line_override, default_cheque_currency
		FROM bank_accounts ORDER BY id LIMIT 1`).Scan(
		&b.ID, &b.Label, &b.MICRCountry, &b.BankInstitution, &b.BankTransit, &b.BankRoutingABA, &b.BankAccount,
		&b.BankChequeNumber, &b.MICRLineOverride, &b.DefaultChequeCurrency,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BankAccount{}, ErrNotFound
	}
	if err != nil {
		return models.BankAccount{}, err
	}
	return b, nil
}

func (s *Store) CreateBankAccount(in models.BankAccount) (models.BankAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	in = normalizeBankAccount(in)
	if strings.TrimSpace(in.ID) == "" {
		in.ID = s.nextBankAccountIDLocked()
	}
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO bank_accounts (id, label, micr_country, bank_institution, bank_transit, bank_routing_aba, bank_account,
		bank_cheque_number, micr_line_override, default_cheque_currency, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID, in.Label, in.MICRCountry, in.BankInstitution, in.BankTransit, in.BankRoutingABA, in.BankAccount,
		in.BankChequeNumber, in.MICRLineOverride, in.DefaultChequeCurrency, now, now,
	)
	if err != nil {
		return models.BankAccount{}, err
	}
	// 若没有默认账户，则设为默认
	var def string
	_ = s.db.QueryRow(`SELECT COALESCE(default_bank_account_id,'') FROM app_settings WHERE id=1`).Scan(&def)
	if strings.TrimSpace(def) == "" {
		_, _ = s.db.Exec(`UPDATE app_settings SET default_bank_account_id=? WHERE id=1`, in.ID)
	}
	return in, nil
}

func (s *Store) UpdateBankAccount(id string, patch models.BankAccount) (models.BankAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return models.BankAccount{}, ErrNotFound
	}
	patch.ID = id
	patch = normalizeBankAccount(patch)
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE bank_accounts SET label=?, micr_country=?, bank_institution=?, bank_transit=?, bank_routing_aba=?, bank_account=?,
		bank_cheque_number=?, micr_line_override=?, default_cheque_currency=?, updated_at=?
		WHERE id=?`,
		patch.Label, patch.MICRCountry, patch.BankInstitution, patch.BankTransit, patch.BankRoutingABA, patch.BankAccount,
		patch.BankChequeNumber, patch.MICRLineOverride, patch.DefaultChequeCurrency, now,
		id,
	)
	if err != nil {
		return models.BankAccount{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.BankAccount{}, ErrNotFound
	}
	return patch, nil
}

func (s *Store) DeleteBankAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotFound
	}
	var def string
	_ = s.db.QueryRow(`SELECT COALESCE(default_bank_account_id,'') FROM app_settings WHERE id=1`).Scan(&def)
	def = strings.TrimSpace(def)
	res, err := s.db.Exec(`DELETE FROM bank_accounts WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	// 若删的是默认，则清空默认（前端会提示去重新选择）
	if def == id {
		_, _ = s.db.Exec(`UPDATE app_settings SET default_bank_account_id='' WHERE id=1`)
	}
	return nil
}

func (s *Store) SetDefaultBankAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if id == "" {
		_, err := s.db.Exec(`UPDATE app_settings SET default_bank_account_id='' WHERE id=1`)
		return err
	}
	var exists int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM bank_accounts WHERE id=?`, id).Scan(&exists)
	if exists == 0 {
		return ErrNotFound
	}
	_, err := s.db.Exec(`UPDATE app_settings SET default_bank_account_id=? WHERE id=1`, id)
	return err
}

func (s *Store) IncrementDefaultChequeNumber() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	def := ""
	_ = s.db.QueryRow(`SELECT COALESCE(default_bank_account_id,'') FROM app_settings WHERE id=1`).Scan(&def)
	def = strings.TrimSpace(def)
	if def == "" {
		return "", ErrNotFound
	}
	var cur string
	err := s.db.QueryRow(`SELECT bank_cheque_number FROM bank_accounts WHERE id=?`, def).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	next := incrementChequeString(cur)
	_, err = s.db.Exec(`UPDATE bank_accounts SET bank_cheque_number=?, updated_at=? WHERE id=?`, next, time.Now().Format(time.RFC3339), def)
	if err != nil {
		return "", err
	}
	return next, nil
}

func incrementChequeString(s string) string {
	raw := strings.TrimSpace(s)
	d := raw
	d = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, d)
	if d == "" {
		d = "0"
	}
	width := len(d)
	if width < 6 {
		width = 6
	}
	n, err := strconv.ParseInt(d, 10, 64)
	if err != nil {
		n = 0
	}
	n++
	return fmt.Sprintf("%0*d", width, n)
}

func normalizeBankAccount(in models.BankAccount) models.BankAccount {
	in.ID = strings.TrimSpace(in.ID)
	in.Label = clampLen(strings.TrimSpace(in.Label), maxBankStrLen)
	mc := strings.ToUpper(strings.TrimSpace(in.MICRCountry))
	if mc != "US" {
		mc = "CA"
	}
	in.MICRCountry = mc
	in.BankInstitution = clampLen(strings.TrimSpace(in.BankInstitution), maxBankStrLen)
	in.BankTransit = clampLen(strings.TrimSpace(in.BankTransit), maxBankStrLen)
	in.BankRoutingABA = clampLen(strings.TrimSpace(in.BankRoutingABA), maxBankStrLen)
	in.BankAccount = clampLen(strings.TrimSpace(in.BankAccount), maxBankStrLen)
	in.BankChequeNumber = clampLen(strings.TrimSpace(in.BankChequeNumber), maxBankStrLen)
	if in.BankChequeNumber == "" {
		in.BankChequeNumber = "000001"
	}
	in.MICRLineOverride = clampLen(strings.TrimSpace(in.MICRLineOverride), maxMICROverrideLen)
	cc := strings.ToUpper(strings.TrimSpace(in.DefaultChequeCurrency))
	if cc == "" {
		cc = "CAD"
	}
	if len(cc) > 8 {
		cc = cc[:8]
	}
	in.DefaultChequeCurrency = cc
	if in.Label == "" {
		in.Label = "Bank account"
	}
	return in
}

func (s *Store) nextBankAccountIDLocked() string {
	rows, err := s.db.Query(`SELECT id FROM bank_accounts WHERE id LIKE 'B%'`)
	if err != nil {
		return "B0001"
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			continue
		}
		if strings.HasPrefix(id, "B") && len(id) > 1 {
			if v, err := strconv.Atoi(strings.TrimPrefix(id, "B")); err == nil && v > max {
				max = v
			}
		}
	}
	return fmt.Sprintf("B%04d", max+1)
}

