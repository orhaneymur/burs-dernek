package web

import (
	"errors"
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

// Basvuru tek sayfada alinir: ogrenci formu doldurur, transkriptini ekler ve
// tek gonderimde basvurusunu tamamlar. Ara kayit, adim veya sorgulama yoktur.

// ---------------------------------------------------------------- Anasayfa

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	period, err := s.st.ActivePeriod(r.Context())
	if err == nil && period.AcceptingApplications() {
		s.renderForm(w, r, period, &model.Application{GPAScale: "4"}, nil)
		return
	}

	p := s.newPage(w, r, "Burs Başvurusu")
	p.Active = "home"
	if err == nil {
		p.Data["Period"] = period
	}
	s.render(w, r, "kapali", p)
}

// handleApplyForm belirli bir donemin basvuru sayfasini acar.
func (s *Server) handleApplyForm(w http.ResponseWriter, r *http.Request) {
	period, err := s.st.PeriodBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı",
			"Aradığınız başvuru dönemi bulunamadı. Bağlantıyı kontrol edin.")
		return
	}
	if !period.AcceptingApplications() {
		p := s.newPage(w, r, period.Name)
		p.Active = "home"
		p.Data["Period"] = period
		s.render(w, r, "kapali", p)
		return
	}
	s.renderForm(w, r, period, &model.Application{GPAScale: "4"}, nil)
}

func (s *Server) renderForm(w http.ResponseWriter, r *http.Request, period *model.Period, app *model.Application, errs map[string]string) {
	p := s.newPage(w, r, period.Name)
	p.Active = "home"
	p.Data["Period"] = period
	p.Data["App"] = app
	p.Data["Genders"] = model.GenderOptions
	p.Data["Classes"] = model.ClassYearOptions
	p.Data["ParentStatus"] = model.ParentStatusOptions
	p.Data["ParentsStatus"] = model.ParentsStatusOptions
	p.Data["Housing"] = model.HousingOptions
	p.Data["MaxUploadMB"] = s.cfg.MaxUploadMB
	p.Data["KVKK"] = s.st.Setting(r.Context(), "kvkk_text", defaultKVKKText)
	p.Data["RequireTranscript"] = period.RequireDocs
	if len(errs) > 0 {
		p.Errors = errs
		p.Data["HataVar"] = true
	}
	s.render(w, r, "basvuru", p)
}

// ---------------------------------------------------------------- Gonderim

func (s *Server) handleApplySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	maxBytes := s.cfg.MaxUploadMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+2<<20)

	// Alanlarin cogu metin; buyuk dosya gecici dizine tasar
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		s.renderError(w, r, http.StatusRequestEntityTooLarge, "Dosya çok büyük",
			"Transkript dosyanız en fazla "+strconv.FormatInt(s.cfg.MaxUploadMB, 10)+" MB olabilir.")
		return
	}
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	if !s.formLimit.Allow(s.clientIP(r)) {
		s.renderError(w, r, http.StatusTooManyRequests, "Çok fazla deneme",
			"Kısa sürede çok sayıda istek aldık. Lütfen bir süre sonra tekrar deneyin.")
		return
	}
	// Bot tuzagi
	if fstr(r, "website") != "" {
		s.renderError(w, r, http.StatusBadRequest, "Geçersiz istek", "Form doğrulanamadı.")
		return
	}

	period, err := s.st.PeriodBySlug(ctx, r.PathValue("slug"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Dönem bulunamadı", "Başvuru dönemi bulunamadı.")
		return
	}
	if !period.AcceptingApplications() {
		s.renderError(w, r, http.StatusForbidden, "Başvuru kapalı",
			"Başvuru süresi sona erdiği için form gönderilemedi.")
		return
	}

	app, errs := readApplicationForm(r)

	// Transkript: hazirlik ve 1. sinif disinda zorunlu
	transcriptRequired := period.RequireDocs && app.ClassYear != "hazirlik" && app.ClassYear != "1"
	file, header, ferr := r.FormFile("transkript")
	if ferr == nil {
		defer file.Close()
	} else if transcriptRequired {
		errs["transkript"] = "Transkriptinizi yükleyin (PDF, JPG veya PNG)."
	}

	// Ayni donemde ayni T.C. ile ikinci basvuru engellenir
	if len(errs) == 0 {
		if _, err := s.st.AppByNationalID(ctx, period.ID, app.NationalID); err == nil {
			errs["national_id"] = "Bu T.C. kimlik numarasıyla bu dönemde zaten bir başvuru yapılmış."
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		s.renderForm(w, r, period, app, errs)
		return
	}

	// Once basvuru satiri, sonra alanlar, sonra belge, en son kesinlestirme
	saved, err := s.st.CreateDraft(ctx, period.ID)
	if err != nil {
		log.Printf("basvuru satiri olusturulamadi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Başvuru kaydedilemedi",
			"Teknik bir sorun oluştu. Lütfen tekrar deneyin.")
		return
	}
	app.ID = saved.ID
	app.PeriodID = period.ID
	app.TrackingCode = saved.TrackingCode
	app.KVKKAccepted = true
	app.KVKKAcceptedAt = time.Now()

	if err := s.st.SaveApplication(ctx, app); err != nil {
		_ = s.st.DeleteApp(ctx, saved.ID)
		if errors.Is(err, store.ErrDuplicateApplication) {
			errs["national_id"] = "Bu T.C. kimlik numarasıyla bu dönemde zaten bir başvuru yapılmış."
			w.WriteHeader(http.StatusBadRequest)
			s.renderForm(w, r, period, app, errs)
			return
		}
		log.Printf("basvuru kaydedilemedi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Başvuru kaydedilemedi",
			"Teknik bir sorun oluştu. Lütfen tekrar deneyin.")
		return
	}

	if ferr == nil {
		if err := s.storeDocument(ctx, saved.ID, model.TranscriptKind, file, header); err != nil {
			_ = s.st.DeleteApp(ctx, saved.ID)
			errs["transkript"] = err.Error()
			w.WriteHeader(http.StatusBadRequest)
			s.renderForm(w, r, period, app, errs)
			return
		}
	}

	res := scoring.Score(app, scoring.Parse(period.CriteriaJSON))
	final := scoring.Final(res.Total, 0, false, period.AutoWeight, period.CommitteeWeight)
	if err := s.st.Submit(ctx, saved.ID, s.clientIP(r), res.Total, res.JSON(), final); err != nil {
		log.Printf("basvuru kesinlestirilemedi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Başvuru tamamlanamadı",
			"Teknik bir sorun oluştu. Lütfen tekrar deneyin.")
		return
	}

	s.st.Audit(ctx, "ogrenci", "basvuru_gonderildi", "application",
		strconv.Itoa(int(saved.ID)), saved.TrackingCode, s.clientIP(r))

	http.SetCookie(w, &http.Cookie{
		Name: "son_basvuru", Value: saved.TrackingCode, Path: "/",
		HttpOnly: true, Secure: s.secureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 3600,
	})
	http.Redirect(w, r, "/basvurunuz-alindi", http.StatusSeeOther)
}

// readApplicationForm tek sayfalik formu okur ve dogrular.
func readApplicationForm(r *http.Request) (*model.Application, map[string]string) {
	errs := map[string]string{}
	a := &model.Application{}

	// Kimlik ve iletisim
	a.NationalID = strings.TrimSpace(r.FormValue("national_id"))
	a.FirstName = clip(fstr(r, "first_name"), 80)
	a.LastName = clip(fstr(r, "last_name"), 80)
	a.BirthDate = fdate(r, "birth_date")
	a.Gender = oneOf(fstr(r, "gender"), model.GenderOptions)
	a.Phone = clip(fstr(r, "phone"), 24)
	a.Email = clip(strings.ToLower(fstr(r, "email")), 190)
	a.City = clip(fstr(r, "city"), 64)
	a.District = clip(fstr(r, "district"), 64)

	// Egitim
	a.University = clip(fstr(r, "university"), 160)
	a.Department = clip(fstr(r, "department"), 160)
	a.ClassYear = oneOf(fstr(r, "class_year"), model.ClassYearOptions)
	a.GPAScale = fstr(r, "gpa_scale")
	if a.GPAScale != "100" {
		a.GPAScale = "4"
	}
	a.GPA = ffloat(r, "gpa")

	// Ekonomik durum
	a.HouseholdIncome = fint(r, "household_income")
	a.HouseholdSize = fint(r, "household_size")
	a.HousingType = oneOf(fstr(r, "housing_type"), model.HousingOptions)
	a.FatherStatus = oneOf(fstr(r, "father_status"), model.ParentStatusOptions)
	a.MotherStatus = oneOf(fstr(r, "mother_status"), model.ParentStatusOptions)
	a.OwnsProperty = fbool(r, "owns_property")
	a.OwnsVehicle = fbool(r, "owns_vehicle")

	// Aile
	a.SiblingCount = fint(r, "sibling_count")
	a.StudentSiblingCount = fint(r, "student_sibling_count")
	a.ParentsStatus = oneOf(fstr(r, "parents_status"), model.ParentsStatusOptions)
	a.Disability = fbool(r, "disability")
	a.MartyrRelative = fbool(r, "martyr_relative")

	// Burs ve sosyal
	a.OtherScholarship = fbool(r, "other_scholarship")
	a.OtherScholarshipName = clip(fstr(r, "other_scholarship_name"), 160)
	a.OtherScholarshipAmount = fint(r, "other_scholarship_amount")
	if !a.OtherScholarship {
		a.OtherScholarshipName = ""
		a.OtherScholarshipAmount = 0
	}
	a.SportsClub = clip(fstr(r, "sports_club"), 160)
	a.VolunteerText = clip(fstr(r, "volunteer_text"), 1200)
	a.MotivationText = clip(fstr(r, "motivation_text"), 2000)

	// --- Dogrulama
	if !security.ValidTCKN(a.NationalID) {
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
		errs["phone"] = "Telefonunuzu 5XX XXX XX XX biçiminde girin."
	}
	if !strings.Contains(a.Email, "@") || !strings.Contains(a.Email, ".") {
		errs["email"] = "Geçerli bir e-posta adresi girin."
	}
	if a.City == "" {
		errs["city"] = "İl bilgisini girin."
	}
	if a.University == "" {
		errs["university"] = "Üniversitenizi girin."
	}
	if a.Department == "" {
		errs["department"] = "Bölümünüzü girin."
	}
	if a.ClassYear == "" {
		errs["class_year"] = "Sınıfınızı seçin."
	}
	if a.GPAScale == "4" && a.GPA > 4 {
		errs["gpa"] = "4'lük sistemde ortalama en fazla 4,00 olabilir."
	}
	if a.GPAScale == "100" && a.GPA > 100 {
		errs["gpa"] = "100'lük sistemde ortalama en fazla 100 olabilir."
	}
	if a.GPA == 0 && a.ClassYear != "hazirlik" && a.ClassYear != "1" {
		errs["gpa"] = "Genel not ortalamanızı girin."
	}
	if a.HouseholdSize < 1 || a.HouseholdSize > 30 {
		errs["household_size"] = "Hanenizde yaşayan kişi sayısını girin."
	}
	if a.HousingType == "" {
		errs["housing_type"] = "Barınma durumunuzu seçin."
	}
	if a.ParentsStatus == "" {
		errs["parents_status"] = "Anne ve babanızın durumunu seçin."
	}
	if a.StudentSiblingCount > a.SiblingCount {
		errs["student_sibling_count"] = "Öğrenci kardeş sayısı, kardeş sayısından fazla olamaz."
	}
	if a.OtherScholarship && a.OtherScholarshipName == "" {
		errs["other_scholarship_name"] = "Aldığınız bursun kurumunu yazın."
	}
	if !fbool(r, "kvkk") {
		errs["kvkk"] = "Devam edebilmek için aydınlatma metnini onaylamanız gerekir."
	}
	if !fbool(r, "beyan") {
		errs["beyan"] = "Beyanınızın doğruluğunu onaylamanız gerekir."
	}
	return a, errs
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

// ---------------------------------------------------------------- Sonuc ekrani

func (s *Server) handleSubmitted(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("son_basvuru")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	app, err := s.st.AppByTracking(r.Context(), c.Value)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	period, _ := s.st.PeriodByID(r.Context(), app.PeriodID)

	p := s.newPage(w, r, "Başvurunuz Alındı")
	p.Data["App"] = app
	p.Data["Period"] = period
	s.render(w, r, "basvuru_tamam", p)
}

// ---------------------------------------------------------------- KVKK

func (s *Server) handleKVKK(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "KVKK Aydınlatma Metni")
	p.Active = "kvkk"
	p.Data["Text"] = s.st.Setting(r.Context(), "kvkk_text", defaultKVKKText)
	s.render(w, r, "kvkk", p)
}
