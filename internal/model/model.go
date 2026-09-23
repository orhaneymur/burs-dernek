// Package model uygulamanin veri yapilarini ve alan sozluklerini tanimlar.
package model

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------- Kullanicilar

type User struct {
	ID           uint32
	Username     string
	FullName     string
	Email        string
	PasswordHash string
	Role         string // admin | komisyon
	IsActive     bool
	MustChange   bool
	LastLoginAt  time.Time
	CreatedAt    time.Time
}

func (u *User) IsAdmin() bool { return u.Role == "admin" }

// ---------------------------------------------------------------- Donemler

const (
	PeriodDraft  = "taslak"
	PeriodOpen   = "acik"
	PeriodClosed = "kapali"
	PeriodResult = "ilan"
)

type Period struct {
	ID              uint32
	Slug            string
	Name            string
	Description     string
	OpensAt         time.Time
	ClosesAt        time.Time
	Quota           int
	ReserveQuota    int
	Status          string
	AutoWeight      float64
	CommitteeWeight float64
	CriteriaJSON    string
	RequireDocs     bool
	ResultNote      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// AcceptingApplications donemin su anda basvuruya acik olup olmadigini soyler.
func (p *Period) AcceptingApplications() bool {
	now := time.Now()
	return p.Status == PeriodOpen && !now.Before(p.OpensAt) && now.Before(p.ClosesAt)
}

func (p *Period) StatusLabel() string {
	switch p.Status {
	case PeriodOpen:
		if !p.AcceptingApplications() {
			return "Açık (tarih dışı)"
		}
		return "Başvurulara açık"
	case PeriodClosed:
		return "Başvurular kapandı"
	case PeriodResult:
		return "Sonuçlar ilan edildi"
	default:
		return "Taslak"
	}
}

// ---------------------------------------------------------------- Basvurular

const (
	StatusDraft     = "taslak"
	StatusSubmitted = "basvuruldu"
	StatusReview    = "incelemede"
	StatusAccepted  = "asil"
	StatusReserve   = "yedek"
	StatusRejected  = "red"
)

var StatusLabels = map[string]string{
	StatusDraft:     "Taslak",
	StatusSubmitted: "Başvuruldu",
	StatusReview:    "İncelemede",
	StatusAccepted:  "Asil",
	StatusReserve:   "Yedek",
	StatusRejected:  "Değerlendirme dışı",
}

type Application struct {
	ID           uint32
	PeriodID     uint32
	TrackingCode string
	DraftToken   string
	Status       string

	NationalID     string // cozulmus haliyle yalnizca bellekte tasinir
	NationalIDHash string
	FirstName      string
	LastName       string
	BirthDate      time.Time
	Gender         string
	Phone          string
	Email          string
	City           string
	District       string
	Address        string

	University    string
	Faculty       string
	Department    string
	ClassYear     string
	StudentNo     string
	EducationType string
	GPA           float64
	GPAScale      string

	HouseholdIncome int
	HouseholdSize   int
	IncomePerCapita int
	FatherStatus    string
	FatherJob       string
	MotherStatus    string
	MotherJob       string
	OwnsProperty    bool
	OwnsVehicle     bool
	HousingType     string
	HousingCost     int

	SiblingCount        int
	StudentSiblingCount int
	ParentsStatus       string
	Disability          bool
	DisabilityNote      string
	MartyrRelative      bool

	OtherScholarship       bool
	OtherScholarshipName   string
	OtherScholarshipAmount int

	VolunteerText  string
	SportsClub     string
	MotivationText string

	KVKKAccepted   bool
	KVKKAcceptedAt time.Time
	SubmitIP       string

	AutoScore      float64
	AutoBreakdown  string
	CommitteeScore float64
	FinalScore     float64
	RankNo         int
	AdminNote      string

	SubmittedAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Yalnizca listeleme sorgularinda doldurulur
	ReviewCount int
	DocCount    int
}

func (a *Application) FullName() string {
	return strings.TrimSpace(a.FirstName + " " + a.LastName)
}

func (a *Application) StatusLabel() string {
	if l, ok := StatusLabels[a.Status]; ok {
		return l
	}
	return a.Status
}

// MaskedNationalID ekranlarda TC'nin yalnizca ilk ve son hanelerini gosterir.
func MaskedNationalID(id string) string {
	if len(id) != 11 {
		return "—"
	}
	return id[:3] + "*****" + id[8:]
}

// GPAOutOf4 farkli sistemleri 4'luk olcege cevirir.
func (a *Application) GPAOutOf4() float64 {
	if a.GPAScale == "100" {
		return a.GPA * 4.0 / 100.0
	}
	return a.GPA
}

func (a *Application) Ref() string {
	return fmt.Sprintf("#%d", a.ID)
}

// ---------------------------------------------------------------- Belgeler

type Document struct {
	ID            uint32
	ApplicationID uint32
	Kind          string
	OriginalName  string
	StoredName    string
	Mime          string
	SizeBytes     int64
	UploadedAt    time.Time
}

type DocumentKind struct {
	Key      string
	Label    string
	Hint     string
	Required bool
}

// DocumentKinds basvuruda istenen belgeleri tanimlar.
// Ogrenciyi yormamak icin tek belge istenir: transkript.
var DocumentKinds = []DocumentKind{
	{
		Key:      "transkript",
		Label:    "Transkript (not durum belgesi)",
		Hint:     "E-Devlet'ten veya üniversitenizin öğrenci bilgi sisteminden alabilirsiniz. PDF, JPG veya PNG.",
		Required: true,
	},
}

// TranscriptKind tek belge turunun anahtari.
const TranscriptKind = "transkript"

func DocumentKindLabel(key string) string {
	for _, k := range DocumentKinds {
		if k.Key == key {
			return k.Label
		}
	}
	return key
}

// ---------------------------------------------------------------- Degerlendirme

type Review struct {
	ID            uint32
	ApplicationID uint32
	ReviewerID    uint32
	ReviewerName  string
	Score         float64
	Notes         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AuditEntry struct {
	ID        uint64
	Actor     string
	Action    string
	Entity    string
	EntityID  string
	Detail    string
	IP        string
	CreatedAt time.Time
}

// ---------------------------------------------------------------- Secenekler

type Option struct {
	Value string
	Label string
}

var (
	GenderOptions = []Option{
		{"kadin", "Kadın"},
		{"erkek", "Erkek"},
		{"belirtmek_istemiyorum", "Belirtmek istemiyorum"},
	}
	EducationTypeOptions = []Option{
		{"orgun", "Örgün öğretim"},
		{"ikinci_ogretim", "İkinci öğretim"},
		{"acik", "Açıköğretim"},
	}
	ClassYearOptions = []Option{
		{"hazirlik", "Hazırlık"},
		{"1", "1. sınıf"},
		{"2", "2. sınıf"},
		{"3", "3. sınıf"},
		{"4", "4. sınıf"},
		{"5", "5. sınıf"},
		{"6", "6. sınıf"},
		{"yuksek_lisans", "Yüksek lisans"},
	}
	ParentStatusOptions = []Option{
		{"calisiyor", "Çalışıyor"},
		{"calismiyor", "Çalışmıyor"},
		{"emekli", "Emekli"},
		{"vefat", "Vefat etti"},
		{"ayri", "Ayrı yaşıyor"},
	}
	ParentsStatusOptions = []Option{
		{"birlikte", "Anne ve baba birlikte"},
		{"bosanmis", "Anne ve baba boşanmış"},
		{"anne_vefat", "Anne vefat etmiş"},
		{"baba_vefat", "Baba vefat etmiş"},
		{"ikisi_vefat", "Her ikisi de vefat etmiş"},
	}
	HousingOptions = []Option{
		{"aile_yani", "Ailemin yanında"},
		{"kira", "Kirada (ev/öğrenci evi)"},
		{"yurt_devlet", "Devlet yurdu (KYK)"},
		{"yurt_ozel", "Özel yurt"},
		{"akraba", "Akraba yanında"},
	}
)

func OptionLabel(opts []Option, value string) string {
	for _, o := range opts {
		if o.Value == value {
			return o.Label
		}
	}
	if value == "" {
		return "—"
	}
	return value
}
