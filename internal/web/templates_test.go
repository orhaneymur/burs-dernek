package web

import "testing"

// Tum sablonlarin ayristigini ve beklenen sayfa adlarinin bulundugunu dogrular.
func TestTemplatesParse(t *testing.T) {
	s := &Server{}
	if err := s.loadTemplates(); err != nil {
		t.Fatalf("sablonlar yuklenemedi: %v", err)
	}
	want := []string{
		"basvuru", "kapali", "basvuru_tamam", "kvkk", "hata",
		"admin_giris", "admin_panel", "admin_donemler", "admin_donem_form",
		"admin_kriterler", "admin_basvurular", "admin_basvuru", "admin_siralama",
		"admin_kullanicilar", "admin_kayitlar", "admin_ayarlar", "admin_sifre",
	}
	for _, name := range want {
		tpl, ok := s.tpl[name]
		if !ok {
			t.Errorf("%s sablonu yok", name)
			continue
		}
		if tpl.Lookup("layout") == nil {
			t.Errorf("%s icinde layout tanimi yok", name)
		}
		if tpl.Lookup("content") == nil {
			t.Errorf("%s icinde content tanimi yok", name)
		}
	}
}
