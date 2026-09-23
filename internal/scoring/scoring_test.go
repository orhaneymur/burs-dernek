package scoring

import (
	"math"
	"testing"

	"github.com/lafed/burs/internal/model"
)

func baseApp() *model.Application {
	return &model.Application{
		ClassYear:       "3",
		GPA:             3.00,
		GPAScale:        "4",
		HouseholdIncome: 30000,
		HouseholdSize:   4, // kisi basi 7.500 -> ilk kademe
		IncomePerCapita: 7500,
		HousingType:     "kira",
		SiblingCount:    2,
		ParentsStatus:   "birlikte",
	}
}

func TestIncomeBands(t *testing.T) {
	c := Default()
	cases := []struct {
		perCapita int
		want      float64
	}{
		{0, 0}, {5000, 1.0}, {7500, 1.0}, {7501, 0.85},
		{17500, 0.70}, {26000, 0.30}, {60000, 0},
	}
	for _, tc := range cases {
		if got := c.incomeRatio(tc.perCapita); got != tc.want {
			t.Errorf("kisi basi %d: %v bekleniyordu, %v geldi", tc.perCapita, tc.want, got)
		}
	}
}

func TestScoreRange(t *testing.T) {
	c := Default()
	// En dusuk durum: yuksek gelir, dusuk not, aile yaninda
	low := &model.Application{
		ClassYear: "3", GPA: 1.5, GPAScale: "4",
		HouseholdIncome: 200000, HouseholdSize: 2, IncomePerCapita: 100000,
		HousingType: "aile_yani", ParentsStatus: "birlikte", OtherScholarship: true,
		OtherScholarshipAmount: 5000,
	}
	res := Score(low, c)
	if res.Total < 0 || res.Total > 100 {
		t.Fatalf("puan araligin disinda: %v", res.Total)
	}
	if res.Total > 20 {
		t.Errorf("zayif basvuru icin beklenenden yuksek puan: %v", res.Total)
	}

	// En yuksek durum
	high := &model.Application{
		ClassYear: "3", GPA: 4.0, GPAScale: "4",
		HouseholdIncome: 0, HouseholdSize: 6, IncomePerCapita: 0,
		HousingType: "kira", SiblingCount: 4, StudentSiblingCount: 3,
		ParentsStatus: "ikisi_vefat", Disability: true, MartyrRelative: true,
		VolunteerText: "Uzun yillardir mahallemizdeki cocuklara ucretsiz ders veriyorum ve dernek faaliyetlerine katiliyorum.",
		SportsClub:    "Gençlik Spor Kulübü",
	}
	res = Score(high, c)
	// Gelir 0 beyan edildiginde kisi basi gelir 0 kabul edilir ve gelir kriterinden puan alinmaz
	if res.Total < 50 {
		t.Errorf("guclu basvuru icin beklenenden dusuk puan: %v", res.Total)
	}
	if len(res.Flags) == 0 {
		t.Error("sifir gelir beyani icin uyari bekleniyordu")
	}
}

func TestPropertyPenalty(t *testing.T) {
	c := Default()
	a := baseApp()
	before := Score(a, c).Total
	a.OwnsProperty = true
	after := Score(a, c)
	if math.Abs((before-after.Total)-c.PropertyPenalty) > 0.01 {
		t.Errorf("mulkiyet kesintisi uygulanmadi: %v -> %v", before, after.Total)
	}
	if after.Penalty != c.PropertyPenalty {
		t.Errorf("kesinti alani yanlis: %v", after.Penalty)
	}
}

func TestFreshmanGetsBaseAcademicScore(t *testing.T) {
	c := Default()
	a := baseApp()
	a.ClassYear = "1"
	a.GPA = 0
	res := Score(a, c)
	for _, it := range res.Items {
		if it.Key == "akademik" {
			if it.Ratio != 0.5 {
				t.Errorf("1. sinif icin taban oran 0.5 olmali, %v geldi", it.Ratio)
			}
			return
		}
	}
	t.Error("akademik kriter bulunamadi")
}

func TestWeightNormalization(t *testing.T) {
	c := Default()
	c.IncomeWeight = 60 // toplam 130 olur
	a := baseApp()
	res := Score(a, c)
	if res.Total > 100 {
		t.Errorf("normalize edilmemis: %v", res.Total)
	}
}

func TestGPAScale100(t *testing.T) {
	a := baseApp()
	a.GPAScale = "100"
	a.GPA = 75
	if got := a.GPAOutOf4(); math.Abs(got-3.0) > 0.001 {
		t.Errorf("100'luk donusum hatali: %v", got)
	}
}

func TestFinalWeighting(t *testing.T) {
	if got := Final(80, 60, true, 0.6, 0.4); math.Abs(got-72) > 0.001 {
		t.Errorf("nihai puan hatali: %v", got)
	}
	if got := Final(80, 60, false, 0.6, 0.4); got != 80 {
		t.Errorf("komisyon puani yokken otomatik puan kullanilmali: %v", got)
	}
}

func TestParseFallsBackToDefault(t *testing.T) {
	c := Parse("bozuk json")
	if c.IncomeWeight != Default().IncomeWeight {
		t.Error("bozuk yapilandirmada varsayilana donulmeli")
	}
	if len(Parse("").IncomeBands) == 0 {
		t.Error("bos yapilandirmada gelir kademeleri dolu olmali")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	c := Default()
	c.IncomeWeight = 42
	parsed := Parse(c.JSON())
	if parsed.IncomeWeight != 42 {
		t.Errorf("JSON gidis donus hatali: %v", parsed.IncomeWeight)
	}
	res := Score(baseApp(), c)
	back, ok := ParseResult(res.JSON())
	if !ok || math.Abs(back.Total-res.Total) > 0.001 {
		t.Errorf("sonuc JSON gidis donus hatali: %v", back)
	}
}
