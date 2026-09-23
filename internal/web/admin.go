package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lafed/burs/internal/model"
	"github.com/lafed/burs/internal/scoring"
	"github.com/lafed/burs/internal/security"
	"github.com/lafed/burs/internal/store"
)

// ---------------------------------------------------------------- Giris / cikis

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("oturum"); err == nil {
		if _, err := s.st.UserBySession(r.Context(), c.Value); err == nil {
			http.Redirect(w, r, "/yonetim/", http.StatusSeeOther)
			return
		}
	}
	p := s.newPage(w, r, "Yönetim Girişi")
	p.Data["Target"] = r.URL.Query().Get("hedef")
	s.render(w, r, "admin_giris", p)
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	ip := s.clientIP(r)
	if !s.loginLimit.Allow(ip) {
		s.renderError(w, r, http.StatusTooManyRequests, "Çok fazla deneme",
			"Çok sayıda hatalı giriş denemesi yapıldı. 15 dakika sonra tekrar deneyin.")
		return
	}

	username := fstr(r, "username")
	password := r.FormValue("password")

	u, err := s.st.UserByUsername(ctx, username)
	if err != nil || !u.IsActive || !security.CheckPassword(u.PasswordHash, password) {
		s.st.Audit(ctx, username, "giris_basarisiz", "user", "", "", ip)
		p := s.newPage(w, r, "Yönetim Girişi")
		p.Errors["genel"] = "Kullanıcı adı veya şifre hatalı."
		p.FormVals = map[string]string{"username": username}
		s.render(w, r, "admin_giris", p)
		return
	}

	token, err := s.st.CreateSession(ctx, u.ID, ip, r.UserAgent())
	if err != nil {
		log.Printf("oturum olusturulamadi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Giriş yapılamadı", "Teknik bir sorun oluştu.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "oturum", Value: token, Path: "/", HttpOnly: true,
		Secure: s.secureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 8 * 3600,
	})
	s.st.Audit(ctx, u.Username, "giris", "user", strconv.Itoa(int(u.ID)), "", ip)

	target := r.FormValue("hedef")
	if !strings.HasPrefix(target, "/yonetim") {
		target = "/yonetim/"
	}
	if u.MustChange {
		target = "/yonetim/sifre"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("oturum"); err == nil {
		s.st.DeleteSession(r.Context(), c.Value)
	}
	clearCookie(w, "oturum")
	http.Redirect(w, r, "/yonetim/giris", http.StatusSeeOther)
}

// ---------------------------------------------------------------- Panel

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)

	p := s.newPage(w, r, "Yönetim Paneli")
	p.Active = "panel"

	periods, err := s.st.Periods(ctx)
	if err != nil {
		log.Printf("donemler okunamadi: %v", err)
	}
	p.Data["Periods"] = periods

	period := s.selectedPeriod(r, periods)
	if period != nil {
		p.Data["Period"] = period
		if st, err := s.st.PeriodStats(ctx, period.ID); err == nil {
			p.Data["Stats"] = st
		}
		if pending, err := s.st.PendingForReviewer(ctx, period.ID, u.ID); err == nil {
			p.Data["Pending"] = pending
		}
		p.Data["ApplyURL"] = s.cfg.BaseURL + "/basvuru/" + period.Slug
	}
	s.render(w, r, "admin_panel", p)
}

// selectedPeriod ?donem= parametresini, yoksa en guncel donemi secer.
func (s *Server) selectedPeriod(r *http.Request, periods []*model.Period) *model.Period {
	if id := r.URL.Query().Get("donem"); id != "" {
		n, err := strconv.ParseUint(id, 10, 32)
		if err == nil {
			for _, p := range periods {
				if p.ID == uint32(n) {
					return p
				}
			}
		}
	}
	if len(periods) > 0 {
		return periods[0]
	}
	return nil
}

func (s *Server) handlePasswordGet(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Şifre Değiştir")
	s.render(w, r, "admin_sifre", p)
}

func (s *Server) handlePasswordPost(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	current := r.FormValue("mevcut")
	yeni := r.FormValue("yeni")
	tekrar := r.FormValue("tekrar")

	p := s.newPage(w, r, "Şifre Değiştir")
	switch {
	case !security.CheckPassword(u.PasswordHash, current):
		p.Errors["genel"] = "Mevcut şifreniz hatalı."
	case len([]rune(yeni)) < 10:
		p.Errors["genel"] = "Yeni şifre en az 10 karakter olmalı."
	case yeni != tekrar:
		p.Errors["genel"] = "Yeni şifre tekrarı eşleşmiyor."
	default:
		if err := s.st.SetPassword(r.Context(), u.ID, yeni); err != nil {
			p.Errors["genel"] = "Şifre güncellenemedi."
		} else {
			s.st.Audit(r.Context(), u.Username, "sifre_degistirildi", "user", strconv.Itoa(int(u.ID)), "", s.clientIP(r))
			s.setFlash(w, "basarili", "Şifreniz güncellendi.")
			http.Redirect(w, r, "/yonetim/", http.StatusSeeOther)
			return
		}
	}
	s.render(w, r, "admin_sifre", p)
}

// ---------------------------------------------------------------- Donemler

func (s *Server) handlePeriods(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Başvuru Dönemleri")
	p.Active = "donemler"
	periods, err := s.st.Periods(r.Context())
	if err != nil {
		log.Printf("donemler okunamadi: %v", err)
	}
	p.Data["Periods"] = periods
	p.Data["BaseURL"] = s.cfg.BaseURL
	s.render(w, r, "admin_donemler", p)
}

func (s *Server) handlePeriodForm(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Dönem")
	p.Active = "donemler"

	if id := fid(r, "id"); id > 0 {
		period, err := s.st.PeriodByID(r.Context(), id)
		if err != nil {
			s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı", "Bu dönem kayıtlarda yok.")
			return
		}
		p.Data["Period"] = period
		p.Title = period.Name
	} else {
		now := time.Now()
		p.Data["Period"] = &model.Period{
			Name:            fmt.Sprintf("%d-%d Öğretim Yılı Burs Başvurusu", now.Year(), now.Year()+1),
			OpensAt:         now,
			ClosesAt:        now.AddDate(0, 1, 0),
			Quota:           25,
			ReserveQuota:    10,
			Status:          model.PeriodDraft,
			AutoWeight:      0.6,
			CommitteeWeight: 0.4,
			RequireDocs:     true,
		}
		p.Data["New"] = true
	}
	s.render(w, r, "admin_donem_form", p)
}

func (s *Server) readPeriodForm(r *http.Request, p *model.Period) map[string]string {
	errs := map[string]string{}
	p.Name = clip(fstr(r, "name"), 160)
	p.Slug = slugify(fstr(r, "slug"))
	if p.Slug == "donem" && p.Name != "" {
		p.Slug = slugify(p.Name)
	}
	p.Description = clip(fstr(r, "description"), 2000)
	p.OpensAt = fdate(r, "opens_at")
	p.ClosesAt = fdate(r, "closes_at")
	p.Quota = fint(r, "quota")
	p.ReserveQuota = fint(r, "reserve_quota")
	p.RequireDocs = fbool(r, "require_docs")
	p.ResultNote = clip(fstr(r, "result_note"), 2000)

	auto := ffloat(r, "auto_weight")
	comm := ffloat(r, "committee_weight")
	if auto+comm <= 0 {
		auto, comm = 60, 40
	}
	p.AutoWeight = auto / (auto + comm)
	p.CommitteeWeight = comm / (auto + comm)

	status := fstr(r, "status")
	switch status {
	case model.PeriodDraft, model.PeriodOpen, model.PeriodClosed, model.PeriodResult:
		p.Status = status
	default:
		p.Status = model.PeriodDraft
	}

	if p.Name == "" {
		errs["name"] = "Dönem adı gerekli."
	}
	if p.OpensAt.IsZero() || p.ClosesAt.IsZero() {
		errs["tarih"] = "Başlangıç ve bitiş tarihlerini girin."
	} else if !p.ClosesAt.After(p.OpensAt) {
		errs["tarih"] = "Bitiş tarihi başlangıçtan sonra olmalı."
	}
	return errs
}

func (s *Server) handlePeriodCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	period := &model.Period{CriteriaJSON: scoring.Default().JSON()}
	errs := s.readPeriodForm(r, period)

	if len(errs) > 0 {
		p := s.newPage(w, r, "Yeni Dönem")
		p.Active = "donemler"
		p.Errors = errs
		p.Data["Period"] = period
		p.Data["New"] = true
		s.render(w, r, "admin_donem_form", p)
		return
	}
	id, err := s.st.CreatePeriod(ctx, period)
	if err != nil {
		p := s.newPage(w, r, "Yeni Dönem")
		p.Active = "donemler"
		p.Errors["genel"] = "Dönem kaydedilemedi: " + err.Error()
		p.Data["Period"] = period
		p.Data["New"] = true
		s.render(w, r, "admin_donem_form", p)
		return
	}
	s.st.Audit(ctx, userFrom(r).Username, "donem_olusturuldu", "period", strconv.Itoa(int(id)), period.Slug, s.clientIP(r))
	s.setFlash(w, "basarili", "Dönem oluşturuldu. Başvuru bağlantısı: "+s.cfg.BaseURL+"/basvuru/"+period.Slug)
	http.Redirect(w, r, "/yonetim/donemler", http.StatusSeeOther)
}

func (s *Server) handlePeriodUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	period, err := s.st.PeriodByID(ctx, fid(r, "id"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı", "Bu dönem kayıtlarda yok.")
		return
	}
	errs := s.readPeriodForm(r, period)
	if len(errs) > 0 {
		p := s.newPage(w, r, period.Name)
		p.Active = "donemler"
		p.Errors = errs
		p.Data["Period"] = period
		s.render(w, r, "admin_donem_form", p)
		return
	}
	if err := s.st.UpdatePeriod(ctx, period); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Kaydedilemedi", err.Error())
		return
	}
	s.st.Audit(ctx, userFrom(r).Username, "donem_guncellendi", "period", strconv.Itoa(int(period.ID)), period.Slug, s.clientIP(r))
	s.setFlash(w, "basarili", "Dönem güncellendi.")
	http.Redirect(w, r, "/yonetim/donemler", http.StatusSeeOther)
}

func (s *Server) handlePeriodStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := fid(r, "id")
	status := fstr(r, "status")
	switch status {
	case model.PeriodDraft, model.PeriodOpen, model.PeriodClosed, model.PeriodResult:
	default:
		s.setFlash(w, "hata", "Geçersiz durum.")
		http.Redirect(w, r, "/yonetim/donemler", http.StatusSeeOther)
		return
	}
	if err := s.st.SetPeriodStatus(ctx, id, status); err != nil {
		s.setFlash(w, "hata", "Durum güncellenemedi.")
	} else {
		s.st.Audit(ctx, userFrom(r).Username, "donem_durumu", "period", strconv.Itoa(int(id)), status, s.clientIP(r))
		s.setFlash(w, "basarili", "Dönem durumu güncellendi.")
	}
	http.Redirect(w, r, "/yonetim/donemler", http.StatusSeeOther)
}

// ---------------------------------------------------------------- Kriterler

func (s *Server) handleCriteriaGet(w http.ResponseWriter, r *http.Request) {
	period, err := s.st.PeriodByID(r.Context(), fid(r, "id"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı", "Bu dönem kayıtlarda yok.")
		return
	}
	p := s.newPage(w, r, "Puanlama Kriterleri")
	p.Active = "donemler"
	p.Data["Period"] = period
	p.Data["Criteria"] = scoring.Parse(period.CriteriaJSON)
	s.render(w, r, "admin_kriterler", p)
}

func (s *Server) handleCriteriaPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	period, err := s.st.PeriodByID(ctx, fid(r, "id"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı", "Bu dönem kayıtlarda yok.")
		return
	}

	c := scoring.Parse(period.CriteriaJSON)
	c.IncomeWeight = ffloat(r, "gelir")
	c.AcademicWeight = ffloat(r, "akademik")
	c.HousingWeight = ffloat(r, "barinma")
	c.SiblingWeight = ffloat(r, "kardes")
	c.ParentsWeight = ffloat(r, "ebeveyn")
	c.SpecialWeight = ffloat(r, "ozel")
	c.ScholarshipWeight = ffloat(r, "burs")
	c.SocialWeight = ffloat(r, "sosyal")
	c.MinGPA = ffloat(r, "asgari_gano")
	c.PropertyPenalty = ffloat(r, "mulkiyet")

	// Gelir kademeleri: satir satir "ust_sinir" ve "oran"
	limits := r.Form["band_limit"]
	ratios := r.Form["band_ratio"]
	var bands []scoring.IncomeBand
	for i := range limits {
		limit, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(limits[i]), ".", ""))
		if err != nil || limit <= 0 || i >= len(ratios) {
			continue
		}
		ratio, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(ratios[i]), ",", ".", 1), 64)
		if err != nil || ratio < 0 || ratio > 1 {
			continue
		}
		bands = append(bands, scoring.IncomeBand{UpTo: limit, Ratio: ratio})
	}
	if len(bands) > 0 {
		c.IncomeBands = bands
	}

	if c.TotalWeight() <= 0 {
		s.setFlash(w, "hata", "Ağırlıkların toplamı sıfır olamaz.")
		http.Redirect(w, r, fmt.Sprintf("/yonetim/donem/%d/kriterler", period.ID), http.StatusSeeOther)
		return
	}

	period.CriteriaJSON = c.JSON()
	if err := s.st.UpdatePeriod(ctx, period); err != nil {
		s.setFlash(w, "hata", "Kriterler kaydedilemedi.")
	} else {
		s.st.Audit(ctx, userFrom(r).Username, "kriterler_guncellendi", "period", strconv.Itoa(int(period.ID)), "", s.clientIP(r))
		s.setFlash(w, "basarili", "Kriterler kaydedildi. Puanları yeniden hesaplamayı unutmayın.")
	}
	http.Redirect(w, r, fmt.Sprintf("/yonetim/donem/%d/kriterler", period.ID), http.StatusSeeOther)
}

// ---------------------------------------------------------------- Basvurular

func (s *Server) handleAppList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := s.newPage(w, r, "Başvurular")
	p.Active = "basvurular"

	periods, _ := s.st.Periods(ctx)
	p.Data["Periods"] = periods
	period := s.selectedPeriod(r, periods)
	if period == nil {
		s.render(w, r, "admin_basvurular", p)
		return
	}
	p.Data["Period"] = period

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("sayfa"))
	if page < 1 {
		page = 1
	}
	const perPage = 50

	f := store.Filter{
		PeriodID:   period.ID,
		Status:     q.Get("durum"),
		Query:      q.Get("ara"),
		University: q.Get("universite"),
		Sort:       q.Get("sirala"),
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	}
	if v := q.Get("asgari"); v != "" {
		f.MinScore, _ = strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
	}

	apps, total, err := s.st.ListApplications(ctx, f)
	if err != nil {
		log.Printf("basvurular okunamadi: %v", err)
		p.Errors["genel"] = "Başvurular okunamadı."
	}
	unis, _ := s.st.Universities(ctx, period.ID)

	p.Data["Apps"] = apps
	p.Data["Total"] = total
	p.Data["Page"] = page
	p.Data["PerPage"] = perPage
	p.Data["Pages"] = (total + perPage - 1) / perPage
	p.Data["Universities"] = unis
	p.Data["Filter"] = map[string]string{
		"durum": q.Get("durum"), "ara": q.Get("ara"),
		"universite": q.Get("universite"), "sirala": q.Get("sirala"), "asgari": q.Get("asgari"),
	}
	p.Data["Classes"] = model.ClassYearOptions
	p.Data["StatusOptions"] = []model.Option{
		{Value: model.StatusSubmitted, Label: "Başvuruldu"},
		{Value: model.StatusReview, Label: "İncelemede"},
		{Value: model.StatusAccepted, Label: "Asil"},
		{Value: model.StatusReserve, Label: "Yedek"},
		{Value: model.StatusRejected, Label: "Değerlendirme dışı"},
		{Value: model.StatusDraft, Label: "Taslak (tamamlanmamış)"},
	}
	s.render(w, r, "admin_basvurular", p)
}

func (s *Server) handleAppDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.st.AppByID(ctx, fid(r, "id"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Başvuru bulunamadı", "Bu başvuru kayıtlarda yok.")
		return
	}
	period, _ := s.st.PeriodByID(ctx, app.PeriodID)
	u := userFrom(r)

	p := s.newPage(w, r, app.FullName()+" — Başvuru")
	p.Active = "basvurular"
	p.Data["App"] = app
	p.Data["Period"] = period
	p.Data["Docs"], _ = s.st.DocumentsFor(ctx, app.ID)
	p.Data["Reviews"], _ = s.st.ReviewsFor(ctx, app.ID)

	if mine, err := s.st.ReviewBy(ctx, app.ID, u.ID); err == nil {
		p.Data["MyReview"] = mine
	}
	if br, ok := scoring.ParseResult(app.AutoBreakdown); ok {
		p.Data["Breakdown"] = br
	} else if period != nil {
		br := scoring.Score(app, scoring.Parse(period.CriteriaJSON))
		p.Data["Breakdown"] = br
	}
	// TC yalnizca yoneticiye acik gosterilir
	if u.IsAdmin() {
		p.Data["NationalID"] = app.NationalID
	} else {
		p.Data["NationalID"] = model.MaskedNationalID(app.NationalID)
	}
	p.Data["Genders"] = model.GenderOptions
	p.Data["ParentStatus"] = model.ParentStatusOptions
	p.Data["ParentsStatus"] = model.ParentsStatusOptions
	p.Data["Housing"] = model.HousingOptions
	p.Data["Classes"] = model.ClassYearOptions
	p.Data["EduTypes"] = model.EducationTypeOptions
	s.render(w, r, "admin_basvuru", p)
}

func (s *Server) handleReviewSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)
	app, err := s.st.AppByID(ctx, fid(r, "id"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Başvuru bulunamadı", "Bu başvuru kayıtlarda yok.")
		return
	}
	score := ffloat(r, "puan")
	if score < 0 || score > 100 {
		s.setFlash(w, "hata", "Puan 0 ile 100 arasında olmalı.")
		http.Redirect(w, r, fmt.Sprintf("/yonetim/basvuru/%d", app.ID), http.StatusSeeOther)
		return
	}
	notes := clip(fstr(r, "notlar"), 2000)

	if err := s.st.SaveReview(ctx, app.ID, u.ID, score, notes); err != nil {
		log.Printf("degerlendirme kaydedilemedi: %v", err)
		s.setFlash(w, "hata", "Değerlendirme kaydedilemedi.")
		http.Redirect(w, r, fmt.Sprintf("/yonetim/basvuru/%d", app.ID), http.StatusSeeOther)
		return
	}
	if app.Status == model.StatusSubmitted {
		_ = s.st.SetAppStatus(ctx, app.ID, model.StatusReview)
	}
	if err := s.recalcApplication(ctx, app.ID); err != nil {
		log.Printf("puan guncellenemedi: %v", err)
	}
	s.st.Audit(ctx, u.Username, "degerlendirme", "application", strconv.Itoa(int(app.ID)), fmt.Sprintf("%.2f", score), s.clientIP(r))
	s.setFlash(w, "basarili", "Değerlendirmeniz kaydedildi.")
	http.Redirect(w, r, fmt.Sprintf("/yonetim/basvuru/%d", app.ID), http.StatusSeeOther)
}

// recalcApplication otomatik puani yeniden hesaplar, komisyon ortalamasini
// alir ve nihai puani gunceller.
func (s *Server) recalcApplication(ctx context.Context, appID uint32) error {
	app, err := s.st.AppByID(ctx, appID)
	if err != nil {
		return err
	}
	period, err := s.st.PeriodByID(ctx, app.PeriodID)
	if err != nil {
		return err
	}
	res := scoring.Score(app, scoring.Parse(period.CriteriaJSON))
	avg, n, err := s.st.AverageReview(ctx, appID)
	if err != nil {
		return err
	}
	committee := sql.NullFloat64{}
	final := res.Total
	if n > 0 && avg.Valid {
		committee = sql.NullFloat64{Float64: avg.Float64, Valid: true}
		final = scoring.Final(res.Total, avg.Float64, true, period.AutoWeight, period.CommitteeWeight)
	} else {
		final = scoring.Final(res.Total, 0, false, period.AutoWeight, period.CommitteeWeight)
	}
	return s.st.UpdateScores(ctx, appID, res.Total, res.JSON(), committee, final)
}

func (s *Server) handleAppStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := fid(r, "id")
	status := fstr(r, "status")
	if _, ok := model.StatusLabels[status]; !ok || status == model.StatusDraft {
		s.setFlash(w, "hata", "Geçersiz durum.")
		http.Redirect(w, r, fmt.Sprintf("/yonetim/basvuru/%d", id), http.StatusSeeOther)
		return
	}
	if err := s.st.SetAppStatus(ctx, id, status); err != nil {
		s.setFlash(w, "hata", "Durum güncellenemedi.")
	} else {
		s.st.Audit(ctx, userFrom(r).Username, "basvuru_durumu", "application", strconv.Itoa(int(id)), status, s.clientIP(r))
		s.setFlash(w, "basarili", "Başvuru durumu güncellendi.")
	}
	http.Redirect(w, r, fmt.Sprintf("/yonetim/basvuru/%d", id), http.StatusSeeOther)
}

func (s *Server) handleAppNote(w http.ResponseWriter, r *http.Request) {
	id := fid(r, "id")
	if err := s.st.SetAdminNote(r.Context(), id, clip(fstr(r, "not"), 2000)); err != nil {
		s.setFlash(w, "hata", "Not kaydedilemedi.")
	} else {
		s.setFlash(w, "basarili", "Not kaydedildi.")
	}
	http.Redirect(w, r, fmt.Sprintf("/yonetim/basvuru/%d", id), http.StatusSeeOther)
}

func (s *Server) handleAppDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := fid(r, "id")
	app, err := s.st.AppByID(ctx, id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Başvuru bulunamadı", "Bu başvuru kayıtlarda yok.")
		return
	}
	docs, _ := s.st.DocumentsFor(ctx, id)
	if err := s.st.DeleteApp(ctx, id); err != nil {
		s.setFlash(w, "hata", "Başvuru silinemedi.")
		http.Redirect(w, r, fmt.Sprintf("/yonetim/basvuru/%d", id), http.StatusSeeOther)
		return
	}
	for _, d := range docs {
		_ = os.Remove(filepath.Join(s.cfg.UploadDir(), filepath.FromSlash(d.StoredName)))
	}
	_ = os.Remove(filepath.Join(s.cfg.UploadDir(), strconv.Itoa(int(id))))
	s.st.Audit(ctx, userFrom(r).Username, "basvuru_silindi", "application", strconv.Itoa(int(id)), app.TrackingCode, s.clientIP(r))
	s.setFlash(w, "bilgi", "Başvuru ve belgeleri silindi.")
	http.Redirect(w, r, "/yonetim/basvurular?donem="+strconv.Itoa(int(app.PeriodID)), http.StatusSeeOther)
}

// ---------------------------------------------------------------- Siralama

func (s *Server) handleRanking(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := s.newPage(w, r, "Sıralama ve Kontenjan")
	p.Active = "siralama"

	periods, _ := s.st.Periods(ctx)
	p.Data["Periods"] = periods
	period := s.selectedPeriod(r, periods)
	if period == nil {
		s.render(w, r, "admin_siralama", p)
		return
	}
	apps, err := s.st.AllForPeriod(ctx, period.ID, false)
	if err != nil {
		p.Errors["genel"] = "Başvurular okunamadı."
	}
	p.Data["Period"] = period
	p.Data["Apps"] = apps
	p.Data["Quota"] = period.Quota
	p.Data["Reserve"] = period.ReserveQuota
	if st, err := s.st.PeriodStats(ctx, period.ID); err == nil {
		p.Data["Stats"] = st
	}
	s.render(w, r, "admin_siralama", p)
}

func (s *Server) handleRecalculate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	periodID, _ := strconv.ParseUint(fstr(r, "donem"), 10, 32)
	apps, err := s.st.AllForPeriod(ctx, uint32(periodID), false)
	if err != nil {
		s.setFlash(w, "hata", "Başvurular okunamadı.")
		http.Redirect(w, r, "/yonetim/siralama", http.StatusSeeOther)
		return
	}
	n := 0
	for _, a := range apps {
		if err := s.recalcApplication(ctx, a.ID); err != nil {
			log.Printf("%d numarali basvuru hesaplanamadi: %v", a.ID, err)
			continue
		}
		n++
	}
	// Siralama numaralarini guncelle
	s.renumber(ctx, uint32(periodID))
	s.st.Audit(ctx, userFrom(r).Username, "puanlar_hesaplandi", "period", fstr(r, "donem"), strconv.Itoa(n), s.clientIP(r))
	s.setFlash(w, "basarili", fmt.Sprintf("%d başvurunun puanı yeniden hesaplandı.", n))
	http.Redirect(w, r, "/yonetim/siralama?donem="+fstr(r, "donem"), http.StatusSeeOther)
}

func (s *Server) renumber(ctx context.Context, periodID uint32) {
	apps, err := s.st.AllForPeriod(ctx, periodID, false)
	if err != nil {
		return
	}
	for i, a := range apps {
		if err := s.st.SetRank(ctx, a.ID, i+1); err != nil {
			log.Printf("sira numarasi yazilamadi: %v", err)
		}
	}
}

// handleApplyQuota siralamaya gore asil ve yedek listelerini isaretler.
func (s *Server) handleApplyQuota(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	periodID64, _ := strconv.ParseUint(fstr(r, "donem"), 10, 32)
	periodID := uint32(periodID64)

	period, err := s.st.PeriodByID(ctx, periodID)
	if err != nil {
		s.setFlash(w, "hata", "Dönem bulunamadı.")
		http.Redirect(w, r, "/yonetim/siralama", http.StatusSeeOther)
		return
	}
	quota := fint(r, "kontenjan")
	reserve := fint(r, "yedek")
	if quota == 0 {
		quota = period.Quota
	}
	if reserve == 0 {
		reserve = period.ReserveQuota
	}

	apps, err := s.st.AllForPeriod(ctx, periodID, false)
	if err != nil {
		s.setFlash(w, "hata", "Başvurular okunamadı.")
		http.Redirect(w, r, "/yonetim/siralama?donem="+fstr(r, "donem"), http.StatusSeeOther)
		return
	}

	asil, yedek := 0, 0
	for i, a := range apps {
		// Elle "değerlendirme dışı" işaretlenenler listeye alınmaz
		if a.Status == model.StatusRejected {
			continue
		}
		_ = s.st.SetRank(ctx, a.ID, i+1)
		switch {
		case asil < quota:
			_ = s.st.SetAppStatus(ctx, a.ID, model.StatusAccepted)
			asil++
		case yedek < reserve:
			_ = s.st.SetAppStatus(ctx, a.ID, model.StatusReserve)
			yedek++
		default:
			_ = s.st.SetAppStatus(ctx, a.ID, model.StatusReview)
		}
	}
	s.st.Audit(ctx, userFrom(r).Username, "kontenjan_uygulandi", "period", fstr(r, "donem"),
		fmt.Sprintf("asil=%d yedek=%d", asil, yedek), s.clientIP(r))
	s.setFlash(w, "basarili", fmt.Sprintf("%d asil, %d yedek aday işaretlendi. Sonuçları öğrencilere göstermek için dönemi \"Sonuçlar ilan edildi\" durumuna alın.", asil, yedek))
	http.Redirect(w, r, "/yonetim/siralama?donem="+fstr(r, "donem"), http.StatusSeeOther)
}

// ---------------------------------------------------------------- Disa aktarim

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	periodID64, _ := strconv.ParseUint(r.URL.Query().Get("donem"), 10, 32)
	period, err := s.st.PeriodByID(ctx, uint32(periodID64))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı", "Dışa aktarılacak dönem seçilmedi.")
		return
	}
	apps, err := s.st.AllForPeriod(ctx, period.ID, false)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Dışa aktarılamadı", err.Error())
		return
	}

	filename := fmt.Sprintf("burs-basvurulari-%s-%s.csv", period.Slug, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	// Excel'in UTF-8 tanimasi icin BOM
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cols := []string{
		"Sıra", "Takip Kodu", "Durum", "Ad", "Soyad", "T.C. Kimlik No", "Doğum Tarihi", "Telefon", "E-posta",
		"İl", "İlçe", "Üniversite", "Fakülte", "Bölüm", "Sınıf", "Öğretim", "GANO", "Ölçek",
		"Hane Geliri", "Hane Kişi", "Kişi Başı Gelir", "Baba Durumu", "Anne Durumu", "Gayrimenkul", "Araç",
		"Barınma", "Barınma Gideri", "Kardeş", "Öğrenci Kardeş", "Ebeveyn Durumu", "Engellilik", "Şehit/Gazi Yakını",
		"Başka Burs", "Burs Adı", "Burs Tutarı", "Otomatik Puan", "Komisyon Puanı", "Nihai Puan",
		"Belge Sayısı", "Başvuru Tarihi", "Yönetici Notu",
	}
	writeCSVRow(w, cols)

	for _, a := range apps {
		docs, _ := s.st.DocumentsFor(ctx, a.ID)
		birth := ""
		if !a.BirthDate.IsZero() {
			birth = a.BirthDate.Format("02.01.2006")
		}
		submitted := ""
		if !a.SubmittedAt.IsZero() {
			submitted = a.SubmittedAt.Format("02.01.2006 15:04")
		}
		writeCSVRow(w, []string{
			strconv.Itoa(a.RankNo), a.TrackingCode, a.StatusLabel(), a.FirstName, a.LastName,
			"'" + a.NationalID, birth, "'" + a.Phone, a.Email,
			a.City, a.District, a.University, a.Faculty, a.Department,
			model.OptionLabel(model.ClassYearOptions, a.ClassYear),
			model.OptionLabel(model.EducationTypeOptions, a.EducationType),
			fmt.Sprintf("%.2f", a.GPA), a.GPAScale,
			strconv.Itoa(a.HouseholdIncome), strconv.Itoa(a.HouseholdSize), strconv.Itoa(a.IncomePerCapita),
			model.OptionLabel(model.ParentStatusOptions, a.FatherStatus),
			model.OptionLabel(model.ParentStatusOptions, a.MotherStatus),
			boolTR(a.OwnsProperty), boolTR(a.OwnsVehicle),
			model.OptionLabel(model.HousingOptions, a.HousingType), strconv.Itoa(a.HousingCost),
			strconv.Itoa(a.SiblingCount), strconv.Itoa(a.StudentSiblingCount),
			model.OptionLabel(model.ParentsStatusOptions, a.ParentsStatus),
			boolTR(a.Disability), boolTR(a.MartyrRelative),
			boolTR(a.OtherScholarship), a.OtherScholarshipName, strconv.Itoa(a.OtherScholarshipAmount),
			fmt.Sprintf("%.2f", a.AutoScore), fmt.Sprintf("%.2f", a.CommitteeScore), fmt.Sprintf("%.2f", a.FinalScore),
			strconv.Itoa(len(docs)), submitted, a.AdminNote,
		})
	}
	s.st.Audit(ctx, userFrom(r).Username, "disa_aktarildi", "period", strconv.Itoa(int(period.ID)),
		strconv.Itoa(len(apps)), s.clientIP(r))
}

// writeCSVRow Excel'in Turkce yerel ayarinda dogru acilmasi icin ; ayraci kullanir.
func writeCSVRow(w http.ResponseWriter, cells []string) {
	for i, c := range cells {
		if i > 0 {
			_, _ = w.Write([]byte(";"))
		}
		c = strings.ReplaceAll(c, "\"", "\"\"")
		c = strings.ReplaceAll(c, "\r\n", " ")
		c = strings.ReplaceAll(c, "\n", " ")
		_, _ = w.Write([]byte("\"" + c + "\""))
	}
	_, _ = w.Write([]byte("\r\n"))
}

func boolTR(b bool) string {
	if b {
		return "Evet"
	}
	return "Hayır"
}

// ---------------------------------------------------------------- Kullanicilar

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Kullanıcılar")
	p.Active = "kullanicilar"
	users, err := s.st.Users(r.Context())
	if err != nil {
		p.Errors["genel"] = "Kullanıcılar okunamadı."
	}
	p.Data["Users"] = users
	s.render(w, r, "admin_kullanicilar", p)
}

func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := strings.ToLower(fstr(r, "username"))
	fullName := clip(fstr(r, "full_name"), 128)
	email := clip(fstr(r, "email"), 190)
	role := fstr(r, "role")
	password := r.FormValue("password")

	if role != "admin" {
		role = "komisyon"
	}
	switch {
	case len(username) < 3:
		s.setFlash(w, "hata", "Kullanıcı adı en az 3 karakter olmalı.")
	case fullName == "":
		s.setFlash(w, "hata", "Ad soyad gerekli.")
	case len([]rune(password)) < 10:
		s.setFlash(w, "hata", "Şifre en az 10 karakter olmalı.")
	default:
		if _, err := s.st.CreateUser(ctx, username, fullName, email, password, role); err != nil {
			s.setFlash(w, "hata", "Kullanıcı eklenemedi (kullanıcı adı zaten olabilir).")
		} else {
			s.st.Audit(ctx, userFrom(r).Username, "kullanici_eklendi", "user", username, role, s.clientIP(r))
			s.setFlash(w, "basarili", "Kullanıcı eklendi.")
		}
	}
	http.Redirect(w, r, "/yonetim/kullanicilar", http.StatusSeeOther)
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := fid(r, "id")
	if u := userFrom(r); u != nil && u.ID == id {
		s.setFlash(w, "hata", "Kendi hesabınızı silemezsiniz.")
		http.Redirect(w, r, "/yonetim/kullanicilar", http.StatusSeeOther)
		return
	}
	if err := s.st.DeleteUser(ctx, id); err != nil {
		s.setFlash(w, "hata", "Kullanıcı silinemedi.")
	} else {
		s.st.Audit(ctx, userFrom(r).Username, "kullanici_silindi", "user", strconv.Itoa(int(id)), "", s.clientIP(r))
		s.setFlash(w, "bilgi", "Kullanıcı silindi.")
	}
	http.Redirect(w, r, "/yonetim/kullanicilar", http.StatusSeeOther)
}

func (s *Server) handleUserPassword(w http.ResponseWriter, r *http.Request) {
	id := fid(r, "id")
	password := r.FormValue("password")
	if len([]rune(password)) < 10 {
		s.setFlash(w, "hata", "Şifre en az 10 karakter olmalı.")
	} else if err := s.st.SetPassword(r.Context(), id, password); err != nil {
		s.setFlash(w, "hata", "Şifre güncellenemedi.")
	} else {
		s.st.Audit(r.Context(), userFrom(r).Username, "sifre_sifirlandi", "user", strconv.Itoa(int(id)), "", s.clientIP(r))
		s.setFlash(w, "basarili", "Şifre güncellendi.")
	}
	http.Redirect(w, r, "/yonetim/kullanicilar", http.StatusSeeOther)
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "İşlem Kayıtları")
	p.Active = "kayitlar"
	entries, err := s.st.AuditTail(r.Context(), 300)
	if err != nil {
		p.Errors["genel"] = "Kayıtlar okunamadı."
	}
	p.Data["Entries"] = entries
	s.render(w, r, "admin_kayitlar", p)
}

// jsonResponse kucuk JSON yanitlari icin yardimci.
func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
