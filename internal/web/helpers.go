package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lafed/burs/internal/model"
)

var turkishMonths = [...]string{
	"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran",
	"Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık",
}

func (s *Server) funcMap() template.FuncMap {
	return template.FuncMap{
		"tarih": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return fmt.Sprintf("%d %s %d", t.Day(), turkishMonths[int(t.Month())-1], t.Year())
		},
		"tarihKisa": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.Format("02.01.2006")
		},
		"tarihSaat": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.Format("02.01.2006 15:04")
		},
		"tarihInput": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"tarihSaatInput": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02T15:04")
		},
		"para":     func(n int) string { return thousands(n) },
		"puan":     func(f float64) string { return strings.Replace(fmt.Sprintf("%.2f", f), ".", ",", 1) },
		"yuzde":    func(f float64) string { return fmt.Sprintf("%.0f%%", f*100) },
		"secenek":  model.OptionLabel,
		"maskeTC":  model.MaskedNationalID,
		"belgeAdi": model.DocumentKindLabel,
		"durumEtiketi": func(status string) string {
			if l, ok := model.StatusLabels[status]; ok {
				return l
			}
			return status
		},
		"evetHayir": func(b bool) string {
			if b {
				return "Evet"
			}
			return "Hayır"
		},
		"bosDegil": func(s string) string {
			if strings.TrimSpace(s) == "" {
				return "—"
			}
			return s
		},
		"kb": func(n int64) string {
			if n < 1024 {
				return fmt.Sprintf("%d B", n)
			}
			if n < 1024*1024 {
				return fmt.Sprintf("%.0f KB", float64(n)/1024)
			}
			return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
		},
		"topla":   func(a, b int) int { return a + b },
		"carp":    func(a, b float64) float64 { return a * b },
		"mul":     func(a, b float64) float64 { return a * b },
		"cikar":   func(a, b int) int { return a - b },
		"esit":    func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
		"dizi":    func(vals ...any) []any { return vals },
		"sozluk": func(vals ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(vals); i += 2 {
				m[fmt.Sprint(vals[i])] = vals[i+1]
			}
			return m
		},
		"satirlar": func(s string) []string {
			if strings.TrimSpace(s) == "" {
				return nil
			}
			return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
		},
		"barGenislik": func(ratio float64) template.CSS {
			if ratio < 0 {
				ratio = 0
			}
			if ratio > 1 {
				ratio = 1
			}
			return template.CSS(fmt.Sprintf("%.1f%%", ratio*100))
		},
	}
}

func thousands(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) > 3 {
		var parts []string
		for len(s) > 3 {
			parts = append([]string{s[len(s)-3:]}, parts...)
			s = s[:len(s)-3]
		}
		parts = append([]string{s}, parts...)
		s = strings.Join(parts, ".")
	}
	if neg {
		return "-" + s
	}
	return s
}

// ---------------------------------------------------------------- Form okuma

func fstr(r *http.Request, name string) string {
	return strings.TrimSpace(r.FormValue(name))
}

func fint(r *http.Request, name string) int {
	v := strings.TrimSpace(r.FormValue(name))
	v = strings.NewReplacer(".", "", " ", "", "₺", "", "TL", "").Replace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func ffloat(r *http.Request, name string) float64 {
	v := strings.TrimSpace(r.FormValue(name))
	v = strings.Replace(v, ",", ".", 1)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

func fbool(r *http.Request, name string) bool {
	v := r.FormValue(name)
	return v == "on" || v == "1" || v == "evet" || v == "true"
}

func fdate(r *http.Request, name string) time.Time {
	v := strings.TrimSpace(r.FormValue(name))
	if v == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func fid(r *http.Request, name string) uint32 {
	n, err := strconv.ParseUint(r.PathValue(name), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

// oneOf gelen degerin izinli secenekler arasinda olmasini garanti eder.
func oneOf(value string, opts []model.Option) string {
	for _, o := range opts {
		if o.Value == value {
			return value
		}
	}
	return ""
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func slugify(s string) string {
	repl := strings.NewReplacer(
		"ç", "c", "Ç", "c", "ğ", "g", "Ğ", "g", "ı", "i", "İ", "i",
		"ö", "o", "Ö", "o", "ş", "s", "Ş", "s", "ü", "u", "Ü", "u", " ", "-")
	s = repl.Replace(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if out == "" {
		out = "donem"
	}
	return out
}
