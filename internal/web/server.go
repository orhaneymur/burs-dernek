// Package web HTTP sunucusunu, sablonlari ve tum uc noktalari icerir.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lafed/burs/internal/config"
	"github.com/lafed/burs/internal/model"
	"github.com/lafed/burs/internal/security"
	"github.com/lafed/burs/internal/store"
)

//go:embed templates/*.html templates/pages/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	cfg  *config.Config
	st   *store.Store
	keys *security.Keyring
	tpl  map[string]*template.Template

	// assetVer statik dosyalarin icerigine bagli kisa damgadir; CSS/JS
	// adreslerine eklenir ve yeni surumde ara onbellekleri (Cloudflare,
	// tarayici) kendiliginden gecersiz kilar.
	assetVer string

	formLimit   *security.Limiter
	loginLimit  *security.Limiter
	uploadLimit *security.Limiter
}

func New(cfg *config.Config, st *store.Store, keys *security.Keyring) (*Server, error) {
	s := &Server{
		cfg:         cfg,
		st:          st,
		keys:        keys,
		formLimit:   security.NewLimiter(40, time.Hour),
		loginLimit:  security.NewLimiter(10, 15*time.Minute),
		uploadLimit: security.NewLimiter(60, time.Hour),
	}
	s.assetVer = assetVersion()
	if err := s.loadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

// assetVersion gomulu statik dosyalarin ozetinden kisa bir damga uretir.
func assetVersion() string {
	h := sha256.New()
	err := fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := staticFS.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(b)
		return nil
	})
	if err != nil {
		return strconv.FormatInt(time.Now().Unix(), 36)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

func (s *Server) loadTemplates() error {
	pages, err := fs.Glob(templateFS, "templates/pages/*.html")
	if err != nil {
		return err
	}
	s.tpl = make(map[string]*template.Template, len(pages))

	for _, page := range pages {
		name := strings.TrimSuffix(page[len("templates/pages/"):], ".html")
		layout := "templates/layout.html"
		if strings.HasPrefix(name, "admin_") {
			layout = "templates/admin_layout.html"
		}
		t, err := template.New("layout").Funcs(s.funcMap()).ParseFS(templateFS, layout, "templates/partials.html", page)
		if err != nil {
			return fmt.Errorf("%s sablonu yuklenemedi: %w", page, err)
		}
		s.tpl[name] = t
	}
	log.Printf("%d sablon yuklendi", len(s.tpl))
	return nil
}

// Handler tum rotalari baglar.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// --- Statik dosyalar
	sub, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(fileServer)))

	// --- Genel sayfalar
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /saglik", s.handleHealth)
	mux.HandleFunc("GET /kvkk", s.handleKVKK)

	// --- Basvuru akisi (tek sayfa)
	mux.HandleFunc("GET /basvuru/{slug}", s.handleApplyForm)
	mux.HandleFunc("POST /basvuru/{slug}", s.handleApplySubmit)
	mux.HandleFunc("GET /basvurunuz-alindi", s.handleSubmitted)

	// --- Yonetim
	mux.HandleFunc("GET /yonetim/giris", s.handleLoginGet)
	mux.HandleFunc("POST /yonetim/giris", s.handleLoginPost)
	mux.HandleFunc("POST /yonetim/cikis", s.handleLogout)

	mux.Handle("GET /yonetim/{$}", s.auth(s.handleDashboard))
	mux.Handle("GET /yonetim/sifre", s.auth(s.handlePasswordGet))
	mux.Handle("POST /yonetim/sifre", s.auth(s.handlePasswordPost))

	mux.Handle("GET /yonetim/donemler", s.admin(s.handlePeriods))
	mux.Handle("GET /yonetim/donem/yeni", s.admin(s.handlePeriodForm))
	mux.Handle("POST /yonetim/donem/yeni", s.admin(s.handlePeriodCreate))
	mux.Handle("GET /yonetim/donem/{id}", s.admin(s.handlePeriodForm))
	mux.Handle("POST /yonetim/donem/{id}", s.admin(s.handlePeriodUpdate))
	mux.Handle("POST /yonetim/donem/{id}/durum", s.admin(s.handlePeriodStatus))
	mux.Handle("GET /yonetim/donem/{id}/kriterler", s.admin(s.handleCriteriaGet))
	mux.Handle("POST /yonetim/donem/{id}/kriterler", s.admin(s.handleCriteriaPost))

	mux.Handle("GET /yonetim/basvurular", s.auth(s.handleAppList))
	mux.Handle("GET /yonetim/basvuru/{id}", s.auth(s.handleAppDetail))
	mux.Handle("POST /yonetim/basvuru/{id}/puan", s.auth(s.handleReviewSave))
	mux.Handle("POST /yonetim/basvuru/{id}/durum", s.admin(s.handleAppStatus))
	mux.Handle("POST /yonetim/basvuru/{id}/not", s.admin(s.handleAppNote))
	mux.Handle("POST /yonetim/basvuru/{id}/sil", s.admin(s.handleAppDelete))
	mux.Handle("GET /yonetim/belge/{id}", s.auth(s.handleDocDownload))

	mux.Handle("GET /yonetim/siralama", s.admin(s.handleRanking))
	mux.Handle("POST /yonetim/siralama/hesapla", s.admin(s.handleRecalculate))
	mux.Handle("POST /yonetim/siralama/uygula", s.admin(s.handleApplyQuota))
	mux.Handle("GET /yonetim/disaaktar", s.admin(s.handleExport))

	mux.Handle("GET /yonetim/ayarlar", s.admin(s.handleSettingsGet))
	mux.Handle("POST /yonetim/ayarlar", s.admin(s.handleSettingsPost))

	mux.Handle("GET /yonetim/kullanicilar", s.admin(s.handleUsers))
	mux.Handle("POST /yonetim/kullanicilar", s.admin(s.handleUserCreate))
	mux.Handle("POST /yonetim/kullanici/{id}/sil", s.admin(s.handleUserDelete))
	mux.Handle("POST /yonetim/kullanici/{id}/sifre", s.admin(s.handleUserPassword))
	mux.Handle("GET /yonetim/kayitlar", s.admin(s.handleAuditLog))

	return securityHeaders(logRequests(mux))
}

// ---------------------------------------------------------------- Sablon veri kabugu

type pageData struct {
	Title    string
	Active   string
	User     *model.User
	Flash    *flash
	CSRF     string
	BaseURL  string
	AssetVer string
	Year     int
	OrgName  string
	Contact  string
	Data     map[string]any
	Errors   map[string]string
	FormVals map[string]string
}

func (s *Server) newPage(w http.ResponseWriter, r *http.Request, title string) *pageData {
	ctx := r.Context()
	p := &pageData{
		Title:   title,
		CSRF:    s.csrfToken(w, r),
		BaseURL:  s.cfg.BaseURL,
		AssetVer: s.assetVer,
		Year:     time.Now().Year(),
		OrgName: s.st.Setting(ctx, "org_name", "LAFED Federasyonu"),
		Contact: s.st.Setting(ctx, "contact", "burs@lafed.org.tr"),
		Data:     map[string]any{},
		Errors:   map[string]string{},
		FormVals: map[string]string{},
		Flash:   takeFlash(w, r),
	}
	if u := userFrom(r); u != nil {
		p.User = u
	}
	return p
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data *pageData) {
	t, ok := s.tpl[page]
	if !ok {
		log.Printf("sablon bulunamadi: %s", page)
		http.Error(w, "Sayfa şablonu bulunamadı", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("sablon calistirilamadi (%s): %v", page, err)
	}
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	w.WriteHeader(code)
	p := s.newPage(w, r, title)
	p.Data["Message"] = message
	p.Data["Code"] = code
	s.render(w, r, "hata", p)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DB.PingContext(r.Context()); err != nil {
		http.Error(w, "veritabani erisilemiyor", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"durum":"calisiyor"}`)
}

func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			// Adres icerige bagli; guvenle uzun sure saklanabilir
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		next.ServeHTTP(w, r)
	})
}
