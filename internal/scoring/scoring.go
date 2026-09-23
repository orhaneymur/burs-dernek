// Package scoring basvurularin kural tabanli on puanini hesaplar.
//
// Kriter agirliklari ve gelir kademeleri her donem icin yonetim panelinden
// degistirilebilir; bu paket yalnizca hesaplama mantigini icerir.
package scoring

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/lafed/burs/internal/model"
)

// IncomeBand kisi basi aylik gelir ust siniri ve o banda karsilik gelen orandir.
type IncomeBand struct {
	UpTo  int     `json:"ust_sinir"` // TL, dahil
	Ratio float64 `json:"oran"`      // 0..1
}

// Criteria bir donemin puanlama yapilandirmasidir.
type Criteria struct {
	IncomeWeight      float64 `json:"gelir_agirlik"`
	AcademicWeight    float64 `json:"akademik_agirlik"`
	HousingWeight     float64 `json:"barinma_agirlik"`
	SiblingWeight     float64 `json:"kardes_agirlik"`
	ParentsWeight     float64 `json:"ebeveyn_agirlik"`
	SpecialWeight     float64 `json:"ozel_durum_agirlik"`
	ScholarshipWeight float64 `json:"diger_burs_agirlik"`
	SocialWeight      float64 `json:"sosyal_agirlik"`

	IncomeBands []IncomeBand `json:"gelir_kademeleri"`

	MinGPA         float64 `json:"asgari_gano"`          // 4'luk olcek
	PropertyPenalty float64 `json:"mulkiyet_kesintisi"`  // gayrimenkul/arac beyani icin puan kesintisi
}

// Default federasyonun panelden degistirebilecegi baslangic yapilandirmasi.
func Default() Criteria {
	return Criteria{
		IncomeWeight:      30,
		AcademicWeight:    20,
		HousingWeight:     10,
		SiblingWeight:     10,
		ParentsWeight:     10,
		SpecialWeight:     10,
		ScholarshipWeight: 5,
		SocialWeight:      5,
		IncomeBands: []IncomeBand{
			{UpTo: 7500, Ratio: 1.00},
			{UpTo: 12500, Ratio: 0.85},
			{UpTo: 17500, Ratio: 0.70},
			{UpTo: 25000, Ratio: 0.50},
			{UpTo: 35000, Ratio: 0.30},
			{UpTo: 50000, Ratio: 0.10},
		},
		MinGPA:          2.0,
		PropertyPenalty: 5,
	}
}

// Parse donemde saklanan JSON'u okur; bos veya bozuksa varsayilana doner.
func Parse(raw string) Criteria {
	c := Default()
	if strings.TrimSpace(raw) == "" {
		return c
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Default()
	}
	if len(c.IncomeBands) == 0 {
		c.IncomeBands = Default().IncomeBands
	}
	sort.Slice(c.IncomeBands, func(i, j int) bool { return c.IncomeBands[i].UpTo < c.IncomeBands[j].UpTo })
	return c
}

func (c Criteria) JSON() string {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func (c Criteria) TotalWeight() float64 {
	return c.IncomeWeight + c.AcademicWeight + c.HousingWeight + c.SiblingWeight +
		c.ParentsWeight + c.SpecialWeight + c.ScholarshipWeight + c.SocialWeight
}

// Item tek bir kriterin sonucudur.
type Item struct {
	Key    string  `json:"anahtar"`
	Label  string  `json:"kriter"`
	Detail string  `json:"aciklama"`
	Weight float64 `json:"agirlik"`
	Ratio  float64 `json:"oran"`
	Points float64 `json:"puan"`
}

// Result otomatik puanlamanin tum ciktisi.
type Result struct {
	Total   float64  `json:"toplam"`
	Items   []Item   `json:"kriterler"`
	Penalty float64  `json:"kesinti"`
	Flags   []string `json:"uyarilar"`
}

func (r Result) JSON() string {
	b, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(b)
}

func ParseResult(raw string) (Result, bool) {
	var r Result
	if strings.TrimSpace(raw) == "" {
		return r, false
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return r, false
	}
	return r, true
}

// Score basvurunun 0-100 arasi otomatik on puanini hesaplar.
func Score(a *model.Application, c Criteria) Result {
	res := Result{}
	add := func(key, label, detail string, weight, ratio float64) {
		ratio = clamp(ratio)
		res.Items = append(res.Items, Item{
			Key:    key,
			Label:  label,
			Detail: detail,
			Weight: weight,
			Ratio:  ratio,
			Points: round2(weight * ratio),
		})
	}

	// 1) Kisi basi gelir
	perCapita := a.IncomePerCapita
	if perCapita == 0 && a.HouseholdSize > 0 {
		perCapita = a.HouseholdIncome / a.HouseholdSize
	}
	incomeRatio := c.incomeRatio(perCapita)
	add("gelir", "Kişi başı gelir", fmt.Sprintf("%s TL/ay", thousands(perCapita)), c.IncomeWeight, incomeRatio)

	// 2) Akademik basari (2.00 tabani, 4.00 tavani)
	gpa := a.GPAOutOf4()
	academicRatio := 0.0
	if gpa > 0 {
		academicRatio = (gpa - 2.0) / 2.0
	}
	academicDetail := fmt.Sprintf("GANO %.2f / 4.00", gpa)
	if a.ClassYear == "hazirlik" || a.ClassYear == "1" {
		// Transkripti olmayan yeni ogrenciler ortalamadan cezalandirilmasin
		academicRatio = 0.5
		academicDetail = "Yeni kayıt / hazırlık — taban puan uygulandı"
	}
	add("akademik", "Akademik başarı", academicDetail, c.AcademicWeight, academicRatio)

	// 3) Barinma
	housingRatio := map[string]float64{
		"kira":        1.00,
		"yurt_ozel":   0.90,
		"yurt_devlet": 0.60,
		"akraba":      0.50,
		"aile_yani":   0.20,
	}[a.HousingType]
	add("barinma", "Barınma durumu", model.OptionLabel(model.HousingOptions, a.HousingType), c.HousingWeight, housingRatio)

	// 4) Kardes yuku
	sibRatio := float64(a.SiblingCount)*0.15 + float64(a.StudentSiblingCount)*0.25
	add("kardes", "Kardeş / öğrenci kardeş",
		fmt.Sprintf("%d kardeş, %d öğrenci", a.SiblingCount, a.StudentSiblingCount),
		c.SiblingWeight, sibRatio)

	// 5) Ebeveyn durumu
	parentRatio := map[string]float64{
		"birlikte":    0.00,
		"bosanmis":    0.60,
		"anne_vefat":  0.85,
		"baba_vefat":  0.85,
		"ikisi_vefat": 1.00,
	}[a.ParentsStatus]
	if a.FatherStatus == "calismiyor" || a.MotherStatus == "calismiyor" {
		parentRatio += 0.15
	}
	add("ebeveyn", "Ebeveyn durumu", model.OptionLabel(model.ParentsStatusOptions, a.ParentsStatus), c.ParentsWeight, parentRatio)

	// 6) Ozel durum (engellilik, sehit-gazi yakinligi)
	specialRatio := 0.0
	var special []string
	if a.Disability {
		specialRatio += 0.7
		special = append(special, "engelli birey")
	}
	if a.MartyrRelative {
		specialRatio += 0.7
		special = append(special, "şehit/gazi yakını")
	}
	specialDetail := "Beyan yok"
	if len(special) > 0 {
		specialDetail = strings.Join(special, ", ")
	}
	add("ozel_durum", "Özel durum", specialDetail, c.SpecialWeight, specialRatio)

	// 7) Baska burs almama
	scholarshipRatio := 1.0
	scholarshipDetail := "Başka burs almıyor"
	if a.OtherScholarship {
		switch {
		case a.OtherScholarshipAmount == 0:
			scholarshipRatio = 0.35
		case a.OtherScholarshipAmount <= 1500:
			scholarshipRatio = 0.50
		case a.OtherScholarshipAmount <= 3500:
			scholarshipRatio = 0.25
		default:
			scholarshipRatio = 0.0
		}
		scholarshipDetail = strings.TrimSpace(a.OtherScholarshipName)
		if scholarshipDetail == "" {
			scholarshipDetail = "Burs alıyor"
		}
		if a.OtherScholarshipAmount > 0 {
			scholarshipDetail += fmt.Sprintf(" (%s TL/ay)", thousands(a.OtherScholarshipAmount))
		}
	}
	add("diger_burs", "Başka burs durumu", scholarshipDetail, c.ScholarshipWeight, scholarshipRatio)

	// 8) Sosyal katilim
	socialRatio := 0.0
	var social []string
	if len([]rune(strings.TrimSpace(a.VolunteerText))) >= 40 {
		socialRatio += 0.6
		social = append(social, "gönüllülük beyanı")
	}
	if strings.TrimSpace(a.SportsClub) != "" {
		socialRatio += 0.4
		social = append(social, "kulüp/dernek üyeliği")
	}
	socialDetail := "Beyan yok"
	if len(social) > 0 {
		socialDetail = strings.Join(social, ", ")
	}
	add("sosyal", "Sosyal katılım", socialDetail, c.SocialWeight, socialRatio)

	// Toplam: agirliklar 100'u bulmuyorsa orantila
	var sum float64
	for _, it := range res.Items {
		sum += it.Points
	}
	if tw := c.TotalWeight(); tw > 0 && math.Abs(tw-100) > 0.001 {
		sum = sum * 100 / tw
	}

	// Kesinti: gayrimenkul / arac beyani
	if (a.OwnsProperty || a.OwnsVehicle) && c.PropertyPenalty > 0 {
		res.Penalty = c.PropertyPenalty
		sum -= c.PropertyPenalty
	}

	res.Total = round2(math.Max(0, math.Min(100, sum)))
	res.Flags = flags(a, c, perCapita, gpa)
	return res
}

func flags(a *model.Application, c Criteria, perCapita int, gpa float64) []string {
	var out []string
	if gpa > 0 && gpa < c.MinGPA && a.ClassYear != "hazirlik" && a.ClassYear != "1" {
		out = append(out, fmt.Sprintf("GANO asgari sınırın altında (%.2f < %.2f)", gpa, c.MinGPA))
	}
	if a.OwnsProperty {
		out = append(out, "Ailede gayrimenkul beyanı var")
	}
	if a.OwnsVehicle {
		out = append(out, "Ailede araç beyanı var")
	}
	if a.OtherScholarship {
		out = append(out, "Başka bir kurumdan burs alıyor")
	}
	if len(c.IncomeBands) > 0 && perCapita > c.IncomeBands[len(c.IncomeBands)-1].UpTo {
		out = append(out, "Kişi başı gelir en üst kademenin üzerinde")
	}
	if a.HouseholdSize > 0 && a.HouseholdIncome == 0 {
		out = append(out, "Hane geliri sıfır beyan edilmiş — belge kontrolü gerekir")
	}
	return out
}

func (c Criteria) incomeRatio(perCapita int) float64 {
	if perCapita <= 0 {
		return 0
	}
	for _, b := range c.IncomeBands {
		if perCapita <= b.UpTo {
			return b.Ratio
		}
	}
	return 0
}

// Final otomatik puan ile komisyon puanini donem agirliklariyla birlestirir.
// Komisyon puani girilmemisse otomatik puan tek basina kullanilir.
func Final(auto, committee float64, hasCommittee bool, autoW, commW float64) float64 {
	if !hasCommittee || autoW+commW == 0 {
		return round2(auto)
	}
	return round2((auto*autoW + committee*commW) / (autoW + commW))
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ".")
}
