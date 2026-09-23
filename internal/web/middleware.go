package web

import (
	"context"
	"crypto/subtle"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lafed/burs/internal/model"
	"github.com/lafed/burs/internal/security"
)

type ctxKey string

const userKey ctxKey = "kullanici"

func userFrom(r *http.Request) *model.User {
	u, _ := r.Context().Value(userKey).(*model.User)
	return u
}

// auth oturum acmis her kullaniciyi (yonetici veya komisyon) kabul eder.
func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("oturum")
		if err != nil {
			redirectToLogin(w, r)
			return
		}
		u, err := s.st.UserBySession(r.Context(), c.Value)
		if err != nil {
			clearCookie(w, "oturum")
			redirectToLogin(w, r)
			return
		}
		if !s.checkCSRF(r) {
			s.renderError(w, r, http.StatusForbidden, "Oturum doğrulaması",
				"Form doğrulaması başarısız oldu. Sayfayı yenileyip tekrar deneyin.")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), userKey, u))
		h(w, r)
	})
}

// admin yalnizca yonetici rolune izin verir.
func (s *Server) admin(h http.HandlerFunc) http.Handler {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r)
		if u == nil || !u.IsAdmin() {
			s.renderError(w, r, http.StatusForbidden, "Yetki yok",
				"Bu bölüm yalnızca federasyon yöneticileri içindir.")
			return
		}
		h(w, r)
	})
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := "/yonetim/giris"
	if r.Method == http.MethodGet && r.URL.Path != "/yonetim/giris" {
		target += "?hedef=" + url.QueryEscape(r.URL.RequestURI())
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// ---------------------------------------------------------------- CSRF

const csrfCookie = "csrf"

func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) >= 32 {
		return c.Value
	}
	token := security.RandomToken(24)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   12 * 3600,
	})
	// Ayni istekte tekrar okunabilsin diye request'e de ekle
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
	return token
}

func (s *Server) checkCSRF(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return true
	}
	c, err := r.Cookie(csrfCookie)
	if err != nil {
		return false
	}
	sent := r.FormValue("csrf")
	if sent == "" {
		sent = r.Header.Get("X-CSRF-Token")
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(sent)) == 1
}

func (s *Server) secureCookies() bool {
	return strings.HasPrefix(s.cfg.BaseURL, "https://")
}

// ---------------------------------------------------------------- Flash mesajlari

type flash struct {
	Kind string // basarili | hata | bilgi
	Text string
}

func (s *Server) setFlash(w http.ResponseWriter, kind, text string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "flash",
		Value:    url.QueryEscape(kind + "|" + text),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60,
	})
}

func takeFlash(w http.ResponseWriter, r *http.Request) *flash {
	c, err := r.Cookie("flash")
	if err != nil || c.Value == "" {
		return nil
	}
	clearCookie(w, "flash")
	raw, err := url.QueryUnescape(c.Value)
	if err != nil {
		return nil
	}
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		return nil
	}
	return &flash{Kind: parts[0], Text: parts[1]}
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}

// ---------------------------------------------------------------- Ortak katmanlar

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'self'")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/saglik" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
	})
}

// clientIP ingress arkasinda gercek istemci adresini bulur.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			parts := strings.Split(v, ",")
			return strings.TrimSpace(parts[0])
		}
		if v := r.Header.Get("X-Real-IP"); v != "" {
			return strings.TrimSpace(v)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
