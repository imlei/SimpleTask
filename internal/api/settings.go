package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"tasktracker/internal/mail"
	"tasktracker/internal/store"
)

func (s *Server) effectiveMailer() *mail.Mailer {
	host, port, user, pass, from, stTLS, impTLS, _ := s.Store.BuildMailConfig()
	if strings.TrimSpace(host) != "" {
		fr := strings.TrimSpace(from)
		if fr == "" {
			fr = strings.TrimSpace(user)
		}
		return mail.New(&mail.Config{
			Host: host, Port: port, User: user, Pass: pass, From: fr,
			StartTLS: stTLS, ImplicitTLS: impTLS, BaseURL: s.Store.GetBaseURL(),
		})
	}
	return s.Mail
}

func (s *Server) handleSettingsPublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name, logo := s.Store.GetPublicBranding()
	writeJSON(w, http.StatusOK, map[string]string{
		"companyName": name,
		"logoDataUrl": logo,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		st, err := s.Store.GetSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out := map[string]any{
			"companyName":     st.CompanyName,
			"logoDataUrl":     st.LogoDataURL,
			"baseUrl":         st.BaseURL,
			"smtpHost":        st.SMTPHost,
			"smtpPort":        st.SMTPPort,
			"smtpUser":        st.SMTPUser,
			"smtpFrom":        st.SMTPFrom,
			"smtpStartTls":    st.SMTPStartTLS,
			"smtpImplicitTls": st.SMTPImplicitTLS,
			"smtpPassSet":     st.SMTPPassSet,
			"envSmtpHost":     store.EnvSMTPHost(),
			"envBaseUrl":      store.EnvBaseURL(),
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPut:
		var body struct {
			CompanyName     string `json:"companyName"`
			LogoDataURL     string `json:"logoDataUrl"`
			BaseURL         string `json:"baseUrl"`
			SMTPHost        string `json:"smtpHost"`
			SMTPPort        int    `json:"smtpPort"`
			SMTPUser        string `json:"smtpUser"`
			SMTPPass        string `json:"smtpPass"`
			SMTPFrom        string `json:"smtpFrom"`
			SMTPStartTLS    *bool  `json:"smtpStartTls"`
			SMTPImplicitTLS *bool  `json:"smtpImplicitTls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cur, err := s.Store.GetSettings()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		in := store.AppSettings{
			CompanyName: body.CompanyName,
			LogoDataURL: body.LogoDataURL,
			BaseURL:     body.BaseURL,
			SMTPHost:    body.SMTPHost,
			SMTPPort:    body.SMTPPort,
			SMTPUser:    body.SMTPUser,
			SMTPFrom:    body.SMTPFrom,
		}
		if body.SMTPPort <= 0 {
			in.SMTPPort = cur.SMTPPort
			if in.SMTPPort <= 0 {
				in.SMTPPort = 587
			}
		}
		if body.SMTPStartTLS != nil {
			in.SMTPStartTLS = *body.SMTPStartTLS
		} else {
			in.SMTPStartTLS = cur.SMTPStartTLS
		}
		if body.SMTPImplicitTLS != nil {
			in.SMTPImplicitTLS = *body.SMTPImplicitTLS
		} else {
			in.SMTPImplicitTLS = cur.SMTPImplicitTLS
		}
		pass := strings.TrimSpace(body.SMTPPass)
		updatePass := pass != ""
		if err := s.Store.UpdateSettings(in, updatePass, pass); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) publicBaseURL(r *http.Request) string {
	if u := strings.TrimSpace(s.Store.GetBaseURL()); u != "" {
		return strings.TrimRight(u, "/")
	}
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return "http://" + r.Host
}
