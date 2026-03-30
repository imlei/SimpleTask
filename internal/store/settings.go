package store

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const maxLogoDataLen = 600000 // ~450KB base64

// AppSettings 存于 app_settings 单行（id=1）
type AppSettings struct {
	CompanyName     string `json:"companyName"`
	LogoDataURL     string `json:"logoDataUrl"`
	BaseURL         string `json:"baseUrl"`
	SMTPHost        string `json:"smtpHost"`
	SMTPPort        int    `json:"smtpPort"`
	SMTPUser        string `json:"smtpUser"`
	SMTPFrom        string `json:"smtpFrom"`
	SMTPStartTLS    bool   `json:"smtpStartTls"`
	SMTPImplicitTLS bool   `json:"smtpImplicitTls"`
	// 仅 GET 返回：是否已保存过密码（不明文）
	SMTPPassSet bool `json:"smtpPassSet"`
}

func (s *Store) loadSettingsRow() (AppSettings, error) {
	var st AppSettings
	var pass string
	var startTLS, implicitTLS int
	var port int
	err := s.db.QueryRow(`SELECT company_name, logo_data_url, base_url,
		smtp_host, smtp_port, smtp_user, smtp_pass, smtp_from, smtp_starttls, smtp_tls
		FROM app_settings WHERE id=1`).Scan(
		&st.CompanyName, &st.LogoDataURL, &st.BaseURL,
		&st.SMTPHost, &port, &st.SMTPUser, &pass, &st.SMTPFrom, &startTLS, &implicitTLS,
	)
	if err != nil {
		return st, err
	}
	st.SMTPPort = port
	if st.SMTPPort <= 0 {
		st.SMTPPort = 587
	}
	st.SMTPStartTLS = startTLS != 0
	st.SMTPImplicitTLS = implicitTLS != 0
	st.SMTPPassSet = pass != ""
	return st, nil
}

// GetSettings 用于 API（密码位不返回，仅 smtpPassSet）
func (s *Store) GetSettings() (AppSettings, error) {
	return s.loadSettingsRow()
}

// GetPublicBranding 登录页等：公司名 + logo
func (s *Store) GetPublicBranding() (companyName, logoDataURL string) {
	st, err := s.loadSettingsRow()
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(st.CompanyName), st.LogoDataURL
}

// GetBaseURL 优先数据库
func (s *Store) GetBaseURL() string {
	st, err := s.loadSettingsRow()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(st.BaseURL)
}

// UpdateSettings 合并更新；smtpPass 为空字符串表示不修改密码列
func (s *Store) UpdateSettings(in AppSettings, updateSMTPPass bool, smtpPassNew string) error {
	in.CompanyName = strings.TrimSpace(in.CompanyName)
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.LogoDataURL = strings.TrimSpace(in.LogoDataURL)
	if len(in.LogoDataURL) > maxLogoDataLen {
		return fmt.Errorf("logo too large (max ~450KB)")
	}
	if in.SMTPPort <= 0 {
		in.SMTPPort = 587
	}
	start := 0
	if in.SMTPStartTLS {
		start = 1
	}
	tls := 0
	if in.SMTPImplicitTLS {
		tls = 1
	}

	if updateSMTPPass {
		_, err := s.db.Exec(`UPDATE app_settings SET
			company_name=?, logo_data_url=?, base_url=?,
			smtp_host=?, smtp_port=?, smtp_user=?, smtp_pass=?, smtp_from=?, smtp_starttls=?, smtp_tls=?
			WHERE id=1`,
			in.CompanyName, in.LogoDataURL, in.BaseURL,
			strings.TrimSpace(in.SMTPHost), in.SMTPPort, strings.TrimSpace(in.SMTPUser), smtpPassNew, strings.TrimSpace(in.SMTPFrom), start, tls,
		)
		return err
	}
	_, err := s.db.Exec(`UPDATE app_settings SET
		company_name=?, logo_data_url=?, base_url=?,
		smtp_host=?, smtp_port=?, smtp_user=?, smtp_from=?, smtp_starttls=?, smtp_tls=?
		WHERE id=1`,
		in.CompanyName, in.LogoDataURL, in.BaseURL,
		strings.TrimSpace(in.SMTPHost), in.SMTPPort, strings.TrimSpace(in.SMTPUser), strings.TrimSpace(in.SMTPFrom), start, tls,
	)
	return err
}

// MailConfiguredInDB 是否在库中配置了 SMTP 主机
func (s *Store) MailConfiguredInDB() bool {
	st, err := s.loadSettingsRow()
	if err != nil {
		return false
	}
	return strings.TrimSpace(st.SMTPHost) != ""
}

// GetSMTPPassword 仅内部用于组装 Mailer
func (s *Store) GetSMTPPassword() string {
	var pass string
	_ = s.db.QueryRow(`SELECT smtp_pass FROM app_settings WHERE id=1`).Scan(&pass)
	return pass
}

// BuildMailConfig 若库中有 smtp_host 则返回完整配置（含库中密码）；否则 nil（由调用方用环境变量）
func (s *Store) BuildMailConfig() (host string, port int, user, pass, from string, startTLS, implicitTLS bool, baseURL string) {
	st, err := s.loadSettingsRow()
	if err != nil || strings.TrimSpace(st.SMTPHost) == "" {
		return "", 0, "", "", "", false, false, ""
	}
	pass = ""
	_ = s.db.QueryRow(`SELECT smtp_pass FROM app_settings WHERE id=1`).Scan(&pass)
	return strings.TrimSpace(st.SMTPHost), st.SMTPPort, strings.TrimSpace(st.SMTPUser), pass,
		strings.TrimSpace(st.SMTPFrom), st.SMTPStartTLS, st.SMTPImplicitTLS, strings.TrimSpace(st.BaseURL)
}

// EnvSMTPHost 用于与库合并提示
func EnvSMTPHost() string {
	return strings.TrimSpace(os.Getenv("SMTP_HOST"))
}

func EnvBaseURL() string {
	return strings.TrimSpace(os.Getenv("BASE_URL"))
}

// DefaultSMTPPort from env
func EnvSMTPPort() int {
	p := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if p == "" {
		return 587
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 587
	}
	return n
}
