package web

import (
	"net/http"
)

// defaultKVKKText yonetim panelinden degistirilebilir; burada yalnizca
// ilk kurulumda gosterilecek taslak metin bulunur.
const defaultKVKKText = `6698 sayılı Kişisel Verilerin Korunması Kanunu ("KVKK") kapsamında, burs başvurunuz sırasında paylaştığınız kişisel verilerin nasıl işlendiğine dair bilgilendirmedir.

1. Veri Sorumlusu
Burs başvurularınız, ilçe federasyonumuz tarafından veri sorumlusu sıfatıyla işlenmektedir.

2. İşlenen Veriler
Kimlik bilgileriniz (ad, soyad, T.C. kimlik numarası, doğum tarihi), iletişim bilgileriniz (telefon, e-posta, adres), eğitim bilgileriniz, aile ve ekonomik durumunuza ilişkin beyanlarınız ile başvuruya eklediğiniz belgeler işlenmektedir. Engellilik ve şehit/gazi yakınlığı gibi özel nitelikli veriler yalnızca açık rızanıza dayanarak, burs değerlendirmesinde öncelik tespiti amacıyla işlenir.

3. İşleme Amacı ve Hukuki Sebep
Verileriniz, burs başvurunuzun alınması, değerlendirilmesi, sıralamanın oluşturulması ve sonucun tarafınıza bildirilmesi amacıyla; KVKK m.5/2-(c) ve m.5/2-(f) ile özel nitelikli veriler bakımından açık rızanıza dayanılarak işlenir.

4. Aktarım
Verileriniz üçüncü kişilerle paylaşılmaz; yalnızca burs değerlendirme komisyonu üyeleri ve federasyon yetkilileri tarafından görülebilir. Yasal zorunluluk hâlinde yetkili kamu kurumlarına aktarılabilir.

5. Saklama Süresi
Başvuru verileri, başvuru döneminin sona ermesinden itibaren 2 yıl süreyle saklanır; sürenin dolmasının ardından silinir veya anonim hâle getirilir. Bursu kabul edilen öğrencilerin verileri, burs ilişkisi devam ettiği sürece ve ilgili mevzuatın öngördüğü süre boyunca saklanır.

6. Güvenlik
T.C. kimlik numarası gibi hassas alanlar veritabanında şifrelenerek saklanır; yüklenen belgelere yalnızca yetkilendirilmiş kullanıcılar erişebilir ve her erişim kayıt altına alınır.

7. Haklarınız
KVKK m.11 uyarınca kişisel verilerinize erişme, düzeltilmesini veya silinmesini isteme, işlemeye itiraz etme haklarına sahipsiniz. Taleplerinizi federasyonun iletişim adresine yazılı olarak iletebilirsiniz.

Başvuru formunu göndererek, beyanlarınızın doğru olduğunu ve gerçeğe aykırı beyan hâlinde başvurunuzun geçersiz sayılacağını kabul etmiş olursunuz.`

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := s.newPage(w, r, "Kurum Ayarları")
	p.Active = "ayarlar"
	p.Data["OrgName"] = s.st.Setting(ctx, "org_name", "LAFED Federasyonu")
	p.Data["Contact"] = s.st.Setting(ctx, "contact", "burs@lafed.org.tr")
	p.Data["Phone"] = s.st.Setting(ctx, "phone", "")
	p.Data["Address"] = s.st.Setting(ctx, "address", "")
	p.Data["KVKKText"] = s.st.Setting(ctx, "kvkk_text", defaultKVKKText)
	s.render(w, r, "admin_ayarlar", p)
}

func (s *Server) handleSettingsPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fields := map[string]string{
		"org_name":  clip(fstr(r, "org_name"), 160),
		"contact":   clip(fstr(r, "contact"), 190),
		"phone":     clip(fstr(r, "phone"), 40),
		"address":   clip(fstr(r, "address"), 400),
		"kvkk_text": clip(fstr(r, "kvkk_text"), 20000),
	}
	for k, v := range fields {
		if v == "" {
			continue
		}
		if err := s.st.SetSetting(ctx, k, v); err != nil {
			s.setFlash(w, "hata", "Ayarlar kaydedilemedi.")
			http.Redirect(w, r, "/yonetim/ayarlar", http.StatusSeeOther)
			return
		}
	}
	s.st.Audit(ctx, userFrom(r).Username, "ayarlar_guncellendi", "settings", "", "", s.clientIP(r))
	s.setFlash(w, "basarili", "Ayarlar kaydedildi.")
	http.Redirect(w, r, "/yonetim/ayarlar", http.StatusSeeOther)
}
