package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lafed/burs/internal/model"
	"github.com/lafed/burs/internal/scoring"
	"github.com/lafed/burs/internal/security"
	"github.com/lafed/burs/internal/store"
)

const draftCookie = "basvuru"

// totalSteps: 1 kimlik, 2 egitim, 3 ekonomik, 4 aile, 5 burs/sosyal, 6 belgeler, 7 ozet
const totalSteps = 7

var stepTitles = map[int]string{
	1: "Kimlik ve İletişim",
	2: "Eğitim Bilgileri",
	3: "Ekonomik Durum",
	4: "Aile Bilgileri",
	5: "Burs ve Sosyal Katılım",
	6: "Belgeler",
	7: "Özet ve Onay",
}

// ---------------------------------------------------------------- Anasayfa

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Burs Başvurusu")
	p.Active = "home"

	period, err := s.st.ActivePeriod(r.Context())
	if err == nil {
		p.Data["Period"] = period
		p.Data["Open"] = period.AcceptingApplications()
	}
	p.Data["Docs"] = model.DocumentKinds
	s.render(w, r, "anasayfa", p)
}

func (s *Server) handleKVKK(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "KVKK Aydınlatma Metni")
	p.Active = "kvkk"
	p.Data["Text"] = s.st.Setting(r.Context(), "kvkk_text", defaultKVKKText)
	s.render(w, r, "kvkk", p)
}

// ---------------------------------------------------------------- Basvuru girisi

func (s *Server) handleApplyIntro(w http.ResponseWriter, r *http.Request) {
	period, err := s.st.PeriodBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı",
			"Aradığınız başvuru dönemi bulunamadı. Bağlantıyı kontrol edin.")
		return
	}
	p := s.newPage(w, r, period.Name)
	p.Data["Period"] = period
	p.Data["Open"] = period.AcceptingApplications()
	p.Data["Docs"] = model.DocumentKinds
	p.Data["KVKK"] = s.st.Setting(r.Context(), "kvkk_text", defaultKVKKText)
	p.Data["MaxUploadMB"] = s.cfg.MaxUploadMB
	s.render(w, r, "basvuru_giris", p)
}

func (s *Server) handleApplyStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	if !s.formLimit.Allow(s.clientIP(r)) {
		s.renderError(w, r, http.StatusTooManyRequests, "Çok fazla deneme",
			"Kısa sürede çok sayıda istek aldık. Lütfen bir süre sonra tekrar deneyin.")
		return
	}
	// Bot tuzagi: gizli alan doldurulmussa sessizce reddet
	if fstr(r, "website") != "" {
		s.renderError(w, r, http.StatusBadRequest, "Geçersiz istek", "Form doğrulanamadı.")
		return
	}

	period, err := s.st.PeriodBySlug(ctx, r.PathValue("slug"))
	if err != nil || !period.AcceptingApplications() {
		s.renderError(w, r, http.StatusForbidden, "Başvuru kapalı",
			"Bu dönem şu anda başvuruya açık değil.")
		return
	}
	if !fbool(r, "kvkk") {
		s.setFlash(w, "hata", "Devam edebilmek için aydınlatma metnini onaylamanız gerekir.")
		http.Redirect(w, r, "/basvuru/"+period.Slug, http.StatusSeeOther)
		return
	}

	app, err := s.st.CreateDraft(ctx, period.ID)
	if err != nil {
		log.Printf("taslak olusturulamadi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Başvuru açılamadı",
			"Teknik bir sorun oluştu. Lütfen tekrar deneyin.")
		return
	}
	app.KVKKAccepted = true
	app.KVKKAcceptedAt = time.Now()
	if err := s.st.SaveApplication(ctx, app); err != nil {
		log.Printf("kvkk onayi kaydedilemedi: %v", err)
	}

	s.setDraftCookie(w, app.DraftToken)
	s.st.Audit(ctx, "ogrenci", "basvuru_acildi", "application", strconv.Itoa(int(app.ID)), period.Slug, s.clientIP(r))
	http.Redirect(w, r, "/basvuru/adim/1", http.StatusSeeOther)
}

func (s *Server) setDraftCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     draftCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
}

// currentDraft cerezdeki taslagi ve donemini yukler.
func (s *Server) currentDraft(r *http.Request) (*model.Application, *model.Period, error) {
	c, err := r.Cookie(draftCookie)
	if err != nil {
		return nil, nil, store.ErrNotFound
	}
	app, err := s.st.AppByDraftToken(r.Context(), c.Value)
	if err != nil {
		return nil, nil, err
	}
	period, err := s.st.PeriodByID(r.Context(), app.PeriodID)
	if err != nil {
		return nil, nil, err
	}
	return app, period, nil
}

// ---------------------------------------------------------------- Form adimlari

func (s *Server) handleStepGet(w http.ResponseWriter, r *http.Request) {
	app, period, err := s.currentDraft(r)
	if err != nil {
		http.Redirect(w, r, "/devam", http.StatusSeeOther)
		return
	}
	if app.Status != model.StatusDraft {
		http.Redirect(w, r, "/sorgula", http.StatusSeeOther)
		return
	}
	step := clampStep(r.PathValue("n"))
	s.renderStep(w, r, app, period, step, nil)
}

func (s *Server) renderStep(w http.ResponseWriter, r *http.Request, app *model.Application, period *model.Period, step int, errs map[string]string) {
	p := s.newPage(w, r, fmt.Sprintf("%d. Adım — %s", step, stepTitles[step]))
	p.Data["App"] = app
	p.Data["Period"] = period
	p.Data["Step"] = step
	p.Data["StepTitle"] = stepTitles[step]
	p.Data["TotalSteps"] = totalSteps
	p.Data["Progress"] = float64(step-1) / float64(totalSteps-1)
	p.Data["Genders"] = model.GenderOptions
	p.Data["Classes"] = model.ClassYearOptions
	p.Data["EduTypes"] = model.EducationTypeOptions
	p.Data["ParentStatus"] = model.ParentStatusOptions
	p.Data["ParentsStatus"] = model.ParentsStatusOptions
	p.Data["Housing"] = model.HousingOptions
	p.Data["DocKinds"] = model.DocumentKinds
	p.Data["MaxUploadMB"] = s.cfg.MaxUploadMB
	p.Data["ResumeURL"] = s.cfg.BaseURL + "/devam/" + app.DraftToken

	if step == 6 || step == 7 {
		docs, err := s.st.DocumentMap(r.Context(), app.ID)
		if err != nil {
			log.Printf("belgeler okunamadi: %v", err)
		}
		p.Data["Docs"] = docs
	}
	if step == 7 {
		missing := s.missingFields(r.Context(), app, period)
		p.Data["Missing"] = missing
		p.Data["Ready"] = len(missing) == 0
	}
	if errs != nil {
		p.Errors = errs
	}
	s.render(w, r, "basvuru_adim", p)
}

func clampStep(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	if n > totalSteps {
		return totalSteps
	}
	return n
}

func (s *Server) handleStepPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	app, period, err := s.currentDraft(r)
	if err != nil {
		http.Redirect(w, r, "/devam", http.StatusSeeOther)
		return
	}
	if app.Status != model.StatusDraft {
		http.Redirect(w, r, "/sorgula", http.StatusSeeOther)
		return
	}
	if !period.AcceptingApplications() {
		s.renderError(w, r, http.StatusForbidden, "Başvuru kapalı",
			"Başvuru süresi sona erdiği için form kaydedilemedi.")
		return
	}

	step := clampStep(r.PathValue("n"))
	errs := s.applyStep(r, app, step)

	if len(errs) > 0 {
		s.renderStep(w, r, app, period, step, errs)
		return
	}
	if err := s.st.SaveApplication(ctx, app); err != nil {
		if errors.Is(err, store.ErrDuplicateApplication) {
			errs["national_id"] = "Bu T.C. kimlik numarasıyla bu dönemde zaten bir başvuru var. \"Başvuruma devam et\" bağlantısını kullanın."
			s.renderStep(w, r, app, period, step, errs)
			return
		}
		log.Printf("basvuru kaydedilemedi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Kaydedilemedi", "Teknik bir sorun oluştu, tekrar deneyin.")
		return
	}

	next := step + 1
	if fstr(r, "yon") == "geri" {
		next = step - 1
	}
	if next < 1 {
		next = 1
	}
	if next > totalSteps {
		next = totalSteps
	}
	http.Redirect(w, r, fmt.Sprintf("/basvuru/adim/%d", next), http.StatusSeeOther)
}

// applyStep ilgili adimin alanlarini modele yazar ve dogrular.
func (s *Server) applyStep(r *http.Request, a *model.Application, step int) map[string]string {
	errs := map[string]string{}
	back := fstr(r, "yon") == "geri"

	switch step {
	case 1:
		tc := strings.TrimSpace(r.FormValue("national_id"))
		a.NationalID = tc
		a.FirstName = clip(fstr(r, "first_name"), 80)
		a.LastName = clip(fstr(r, "last_name"), 80)
		a.BirthDate = fdate(r, "birth_date")
		a.Gender = oneOf(fstr(r, "gender"), model.GenderOptions)
		a.Phone = clip(fstr(r, "phone"), 24)
		a.Email = clip(strings.ToLower(fstr(r, "email")), 190)
		a.City = clip(fstr(r, "city"), 64)
		a.District = clip(fstr(r, "district"), 64)
		a.Address = clip(fstr(r, "address"), 500)
		if back {
			return errs
		}
		if !security.ValidTCKN(tc) {
			errs["national_id"] = "Geçerli bir T.C. kimlik numarası girin."
		}
		if a.FirstName == "" {
			errs["first_name"] = "Adınızı girin."
		}
		if a.LastName == "" {
			errs["last_name"] = "Soyadınızı girin."
		}
		if a.BirthDate.IsZero() {
			errs["birth_date"] = "Doğum tarihinizi girin."
		}
		if len(digits(a.Phone)) < 10 {
			errs["phone"] = "Telefon numaranızı 5XX XXX XX XX biçiminde girin."
		}
		if !strings.Contains(a.Email, "@") || !strings.Contains(a.Email, ".") {
			errs["email"] = "Geçerli bir e-posta adresi girin."
		}
		if a.City == "" {
			errs["city"] = "İl bilgisini girin."
		}

	case 2:
		a.University = clip(fstr(r, "university"), 160)
		a.Faculty = clip(fstr(r, "faculty"), 160)
		a.Department = clip(fstr(r, "department"), 160)
		a.ClassYear = oneOf(fstr(r, "class_year"), model.ClassYearOptions)
		a.StudentNo = clip(fstr(r, "student_no"), 40)
		a.EducationType = oneOf(fstr(r, "education_type"), model.EducationTypeOptions)
		a.GPAScale = fstr(r, "gpa_scale")
		if a.GPAScale != "100" {
			a.GPAScale = "4"
		}
		a.GPA = ffloat(r, "gpa")
		if back {
			return errs
		}
		if a.University == "" {
			errs["university"] = "Üniversite adını girin."
		}
		if a.Department == "" {
			errs["department"] = "Bölüm adını girin."
		}
		if a.ClassYear == "" {
			errs["class_year"] = "Sınıfınızı seçin."
		}
		if a.GPAScale == "4" && a.GPA > 4 {
			errs["gpa"] = "4'lük sistemde ortalama en fazla 4.00 olabilir."
		}
		if a.GPAScale == "100" && a.GPA > 100 {
			errs["gpa"] = "100'lük sistemde ortalama en fazla 100 olabilir."
		}
		if a.GPA == 0 && a.ClassYear != "hazirlik" && a.ClassYear != "1" {
			errs["gpa"] = "Genel not ortalamanızı girin."
		}

	case 3:
		a.HouseholdIncome = fint(r, "household_income")
		a.HouseholdSize = fint(r, "household_size")
		a.FatherStatus = oneOf(fstr(r, "father_status"), model.ParentStatusOptions)
		a.FatherJob = clip(fstr(r, "father_job"), 120)
		a.MotherStatus = oneOf(fstr(r, "mother_status"), model.ParentStatusOptions)
		a.MotherJob = clip(fstr(r, "mother_job"), 120)
		a.OwnsProperty = fbool(r, "owns_property")
		a.OwnsVehicle = fbool(r, "owns_vehicle")
		a.HousingType = oneOf(fstr(r, "housing_type"), model.HousingOptions)
		a.HousingCost = fint(r, "housing_cost")
		if back {
			return errs
		}
		if a.HouseholdSize < 1 {
			errs["household_size"] = "Hanede yaşayan kişi sayısını girin."
		}
		if a.HouseholdSize > 30 {
			errs["household_size"] = "Kişi sayısı çok yüksek görünüyor, kontrol edin."
		}
		if a.HousingType == "" {
			errs["housing_type"] = "Barınma durumunuzu seçin."
		}

	case 4:
		a.SiblingCount = fint(r, "sibling_count")
		a.StudentSiblingCount = fint(r, "student_sibling_count")
		a.ParentsStatus = oneOf(fstr(r, "parents_status"), model.ParentsStatusOptions)
		a.Disability = fbool(r, "disability")
		a.DisabilityNote = clip(fstr(r, "disability_note"), 255)
		a.MartyrRelative = fbool(r, "martyr_relative")
		if back {
			return errs
		}
		if a.ParentsStatus == "" {
			errs["parents_status"] = "Ebeveyn durumunu seçin."
		}
		if a.StudentSiblingCount > a.SiblingCount {
			errs["student_sibling_count"] = "Öğrenci kardeş sayısı, toplam kardeş sayısından fazla olamaz."
		}
		if a.SiblingCount > 20 {
			errs["sibling_count"] = "Kardeş sayısını kontrol edin."
		}

	case 5:
		a.OtherScholarship = fbool(r, "other_scholarship")
		a.OtherScholarshipName = clip(fstr(r, "other_scholarship_name"), 160)
		a.OtherScholarshipAmount = fint(r, "other_scholarship_amount")
		if !a.OtherScholarship {
			a.OtherScholarshipName = ""
			a.OtherScholarshipAmount = 0
		}
		a.VolunteerText = clip(fstr(r, "volunteer_text"), 1500)
		a.SportsClub = clip(fstr(r, "sports_club"), 160)
		a.MotivationText = clip(fstr(r, "motivation_text"), 2500)
		if back {
			return errs
		}
		if a.OtherScholarship && a.OtherScholarshipName == "" {
			errs["other_scholarship_name"] = "Aldığınız bursun adını yazın."
		}

	case 6, 7:
		// Belgeler ayri uc noktadan yuklenir; bu adimlarda kaydedilecek alan yok.
	}
	return errs
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// missingFields kesinlestirme oncesi eksik zorunlu alanlari listeler.
func (s *Server) missingFields(ctx context.Context, a *model.Application, period *model.Period) []string {
	var m []string
	if !security.ValidTCKN(a.NationalID) {
		m = append(m, "T.C. kimlik numarası (1. adım)")
	}
	if a.FirstName == "" || a.LastName == "" {
		m = append(m, "Ad soyad (1. adım)")
	}
	if a.BirthDate.IsZero() {
		m = append(m, "Doğum tarihi (1. adım)")
	}
	if len(digits(a.Phone)) < 10 {
		m = append(m, "Telefon (1. adım)")
	}
	if !strings.Contains(a.Email, "@") {
		m = append(m, "E-posta (1. adım)")
	}
	if a.University == "" || a.Department == "" || a.ClassYear == "" {
		m = append(m, "Eğitim bilgileri (2. adım)")
	}
	if a.HouseholdSize < 1 {
		m = append(m, "Hanedeki kişi sayısı (3. adım)")
	}
	if a.HousingType == "" {
		m = append(m, "Barınma durumu (3. adım)")
	}
	if a.ParentsStatus == "" {
		m = append(m, "Ebeveyn durumu (4. adım)")
	}
	if !a.KVKKAccepted {
		m = append(m, "KVKK aydınlatma onayı")
	}
	if period.RequireDocs {
		docs, err := s.st.DocumentMap(ctx, a.ID)
		if err == nil {
			for _, k := range model.DocumentKinds {
				if k.Required {
					if _, ok := docs[k.Key]; !ok {
						m = append(m, k.Label+" (6. adım)")
					}
				}
			}
		}
	}
	return m
}

// ---------------------------------------------------------------- Kesinlestirme

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	app, period, err := s.currentDraft(r)
	if err != nil {
		http.Redirect(w, r, "/devam", http.StatusSeeOther)
		return
	}
	if app.Status != model.StatusDraft {
		http.Redirect(w, r, "/sorgula", http.StatusSeeOther)
		return
	}
	if !period.AcceptingApplications() {
		s.renderError(w, r, http.StatusForbidden, "Başvuru kapalı", "Başvuru süresi sona erdi.")
		return
	}
	if !fbool(r, "beyan") {
		s.setFlash(w, "hata", "Beyanınızın doğruluğunu onaylamanız gerekiyor.")
		http.Redirect(w, r, "/basvuru/adim/7", http.StatusSeeOther)
		return
	}
	if missing := s.missingFields(r.Context(), app, period); len(missing) > 0 {
		s.setFlash(w, "hata", "Eksik alanlar var: "+strings.Join(missing, ", "))
		http.Redirect(w, r, "/basvuru/adim/7", http.StatusSeeOther)
		return
	}

	crit := scoring.Parse(period.CriteriaJSON)
	res := scoring.Score(app, crit)
	final := scoring.Final(res.Total, 0, false, period.AutoWeight, period.CommitteeWeight)

	if err := s.st.Submit(ctx, app.ID, s.clientIP(r), res.Total, res.JSON(), final); err != nil {
		log.Printf("basvuru kesinlestirilemedi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Gönderilemedi", "Teknik bir sorun oluştu, tekrar deneyin.")
		return
	}

	s.st.Audit(ctx, "ogrenci", "basvuru_gonderildi", "application", strconv.Itoa(int(app.ID)), app.TrackingCode, s.clientIP(r))
	clearCookie(w, draftCookie)
	http.SetCookie(w, &http.Cookie{
		Name: "son_basvuru", Value: app.TrackingCode, Path: "/",
		HttpOnly: true, Secure: s.secureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 1800,
	})
	http.Redirect(w, r, "/basvuru/tamamlandi", http.StatusSeeOther)
}

func (s *Server) handleSubmitted(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("son_basvuru")
	if err != nil {
		http.Redirect(w, r, "/sorgula", http.StatusSeeOther)
		return
	}
	app, err := s.st.AppByTracking(r.Context(), c.Value)
	if err != nil {
		http.Redirect(w, r, "/sorgula", http.StatusSeeOther)
		return
	}
	period, _ := s.st.PeriodByID(r.Context(), app.PeriodID)

	p := s.newPage(w, r, "Başvurunuz Alındı")
	p.Data["App"] = app
	p.Data["Period"] = period
	s.render(w, r, "basvuru_tamam", p)
}

// ---------------------------------------------------------------- Taslaga devam

func (s *Server) handleResumeGet(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Başvuruma Devam Et")
	s.render(w, r, "devam", p)
}

func (s *Server) handleResumePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	if !s.formLimit.Allow("devam:" + s.clientIP(r)) {
		s.renderError(w, r, http.StatusTooManyRequests, "Çok fazla deneme", "Lütfen bir süre sonra tekrar deneyin.")
		return
	}
	tc := fstr(r, "national_id")
	birth := fdate(r, "birth_date")

	p := s.newPage(w, r, "Başvuruma Devam Et")
	p.FormVals = map[string]string{"national_id": tc}

	period, err := s.st.ActivePeriod(ctx)
	if err != nil {
		p.Errors["genel"] = "Şu anda açık bir başvuru dönemi bulunmuyor."
		s.render(w, r, "devam", p)
		return
	}
	app, err := s.st.AppByNationalID(ctx, period.ID, tc)
	if err != nil || app.BirthDate.IsZero() || !sameDay(app.BirthDate, birth) {
		p.Errors["genel"] = "Bu bilgilerle kayıtlı bir başvuru bulunamadı. T.C. kimlik numarası ve doğum tarihini kontrol edin."
		s.render(w, r, "devam", p)
		return
	}
	if app.Status != model.StatusDraft {
		p.Errors["genel"] = "Bu başvuru zaten tamamlanmış. Durumunu \"Başvuru Sorgula\" sayfasından takip edebilirsiniz."
		s.render(w, r, "devam", p)
		return
	}
	s.setDraftCookie(w, app.DraftToken)
	http.Redirect(w, r, "/basvuru/adim/1", http.StatusSeeOther)
}

func (s *Server) handleResumeToken(w http.ResponseWriter, r *http.Request) {
	app, err := s.st.AppByDraftToken(r.Context(), r.PathValue("token"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Bağlantı geçersiz",
			"Bu devam bağlantısı geçerli değil. T.C. kimlik numaranızla devam edebilirsiniz.")
		return
	}
	if app.Status != model.StatusDraft {
		http.Redirect(w, r, "/sorgula", http.StatusSeeOther)
		return
	}
	s.setDraftCookie(w, app.DraftToken)
	http.Redirect(w, r, "/basvuru/adim/1", http.StatusSeeOther)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// ---------------------------------------------------------------- Durum sorgulama

func (s *Server) handleStatusGet(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Başvuru Sorgula")
	p.Active = "sorgula"
	s.render(w, r, "sorgula", p)
}

func (s *Server) handleStatusPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	if !s.formLimit.Allow("sorgu:" + s.clientIP(r)) {
		s.renderError(w, r, http.StatusTooManyRequests, "Çok fazla deneme", "Lütfen bir süre sonra tekrar deneyin.")
		return
	}
	code := strings.ToUpper(fstr(r, "tracking_code"))
	birth := fdate(r, "birth_date")

	p := s.newPage(w, r, "Başvuru Sorgula")
	p.Active = "sorgula"
	p.FormVals = map[string]string{"tracking_code": code}

	app, err := s.st.AppByTracking(ctx, code)
	if err != nil || app.BirthDate.IsZero() || !sameDay(app.BirthDate, birth) {
		p.Errors["genel"] = "Bu bilgilerle kayıtlı bir başvuru bulunamadı."
		s.render(w, r, "sorgula", p)
		return
	}
	period, _ := s.st.PeriodByID(ctx, app.PeriodID)
	p.Data["App"] = app
	p.Data["Period"] = period
	p.Data["Announced"] = period != nil && period.Status == model.PeriodResult
	s.render(w, r, "sorgula", p)
}
