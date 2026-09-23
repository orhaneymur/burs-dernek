# Kullanım Kılavuzu (Federasyon Personeli ve Komisyon Üyeleri)

Bu kılavuz teknik bilgi gerektirmez. Sistemi günlük işleyişte kullanacak kişiler içindir.

Adres: **https://burs.lafed.org.tr/yonetim/giris**

---

## 1. Bir burs dönemi açmak

1. Üst menüden **Dönemler** → **Yeni dönem**
2. Alanları doldurun:
   - **Dönem adı:** öğrencinin göreceği başlık, örn. *2026-2027 Öğretim Yılı Burs Başvurusu*
   - **Bağlantı adresi:** kısa ve sade olsun, örn. `2026-2027`. Başvuru bağlantısı
     `https://burs.lafed.org.tr/basvuru/2026-2027` olur.
   - **Açıklama:** başvuru sayfasının üstünde görünür (kimler başvurabilir, burs tutarı vb.)
   - **Başlangıç / bitiş:** bitiş anından sonra form kapanır, yarım kalan başvurular gönderilemez
   - **Kontenjan:** kaç öğrenciye burs verileceği; ayrıca yedek sayısı
   - **Ağırlıklar:** otomatik puan ile komisyon puanının payı (varsayılan %60 / %40)
3. **Durum**'u seçin:
   - *Taslak* — bağlantı çalışmaz, hazırlık aşaması
   - *Başvurulara açık* — öğrenciler başvurabilir
   - *Başvurular kapalı* — yeni başvuru alınmaz, değerlendirme sürer
   - *Sonuçlar ilan edildi* — öğrenciler takip koduyla sonucu görür
4. Kaydedin. Panelde görünen başvuru bağlantısını duyurularınızda paylaşın.

> Dönem durumunu **Dönemler** listesindeki açılır menüden tek tıkla değiştirebilirsiniz.

---

## 2. Puanlama ölçütlerini ayarlamak

**Dönemler → Kriterler** sayfasından:

- Sekiz kriterin ağırlığını değiştirebilirsiniz (toplamları 100 olmak zorunda değil)
- **Gelir kademeleri** en önemli bölümdür: kişi başı aylık gelire karşılık gelen puan oranı.
  Her yıl asgari ücret ve enflasyona göre güncelleyin.
- **Asgari GANO:** altında kalan başvurular listede uyarı ile işaretlenir, otomatik elenmez
- **Mülkiyet kesintisi:** ailede ev/araç beyanı olan başvurulardan düşülecek puan

Değişiklikten sonra **Sıralama → Puanları yeniden hesapla** demeyi unutmayın.

---

## 3. Komisyon üyesi eklemek

**Kullanıcılar** sayfasından ad soyad, kullanıcı adı ve şifre girerek ekleyin.
Rol olarak **Komisyon üyesi** seçin. Komisyon üyeleri:

- Başvuruları görür ve puanlar
- T.C. kimlik numaralarını maskeli görür
- Dönem, kullanıcı, sıralama ve ayar sayfalarına giremez

Şifreyi üyeye güvenli bir kanaldan iletin; ilk girişte değiştirmesini isteyin.

---

## 4. Başvuruları değerlendirmek

1. **Başvurular** sayfasından dönemi seçin. Filtreler: durum, üniversite, asgari puan, arama.
2. Bir başvuruya tıklayın. Sol tarafta tüm beyanlar, sağ tarafta:
   - **Puan durumu:** otomatik puan, komisyon ortalaması, nihai puan
   - **Kriter kırılımı:** her ölçütten kaç puan aldığı ve nedeni
   - **Dikkat edilecekler:** düşük GANO, mülkiyet beyanı, sıfır gelir beyanı gibi uyarılar
   - **Belgeler:** tıklayınca yeni sekmede açılır (her görüntüleme kayda geçer)
3. **Komisyon Değerlendirmeniz** kutusuna 0-100 arası puanınızı ve notunuzu yazıp kaydedin.
4. Her üye kendi puanını girer; sistem ortalamayı alır ve nihai puanı günceller.

> Bir başvuru ilk puanlandığında durumu otomatik olarak *İncelemede* olur.

---

## 5. Sonuçları belirlemek

1. Tüm değerlendirmeler bittikten sonra **Sıralama** sayfasına gidin
2. **Puanları yeniden hesapla** — en güncel kriterler ve komisyon puanlarıyla sıralamayı tazeler
3. Tabloyu inceleyin: yeşil satırlar kontenjan içinde, sarı satırlar yedek sınırında
4. Gerekirse tek tek başvurulara girip durumu **Değerlendirme dışı** yapın (eksik/yanlış beyan vb.);
   bu başvurular listeye alınmaz
5. **Kontenjanı uygula** — asil ve yedek listeleri işaretlenir
6. Listeyi son bir kez kontrol edin, gerekirse elle düzeltin
7. **Dönemler** → dönem durumunu **Sonuçlar ilan edildi** yapın

Artık öğrenciler `https://burs.lafed.org.tr/sorgula` adresinden takip kodu ve doğum tarihiyle
sonuçlarını görebilir.

---

## 6. Kayıtları dışa aktarmak

**Başvurular** veya **Sıralama** sayfasındaki **CSV indir** düğmesi tüm başvuruları
Excel'de açılabilir biçimde indirir (noktalı virgül ayraçlı, Türkçe karakter uyumlu).

Excel'de açarken sütunlar tek hücrede görünürse: *Veri → Metni Sütunlara Dönüştür → Ayrılmış →
Noktalı virgül*.

> Dışa aktarılan dosya T.C. kimlik numaralarını ve iletişim bilgilerini içerir.
> Paylaşımına dikkat edin, gerekmedikçe bilgisayarınızda saklamayın.

---

## 7. Öğrencilerden gelen sık sorular

| Soru | Cevap |
|---|---|
| "Formu yarıda bıraktım, bilgilerim kayboldu mu?" | Hayır. Ana sayfadaki **Başvuruma devam et** bağlantısından T.C. kimlik numarası ve doğum tarihiyle kaldığı yerden devam eder. |
| "Takip kodumu kaybettim." | Başvurular sayfasından adıyla arayıp kodu kendisine iletebilirsiniz. |
| "Belgeyi yükleyemiyorum." | Dosya PDF, JPG veya PNG olmalı ve 8 MB'ı geçmemeli. Telefonla çekilen fotoğraflar genelde uygundur. |
| "Hazırlık sınıfındayım, not ortalamam yok." | GANO alanını boş bırakabilir; sistem taban puan uygular. |
| "Başvurumu göndermiştim, düzeltebilir miyim?" | Gönderilen başvuru kilitlenir. Gerekirse yönetici başvuruyu silip öğrenciden yeniden başvurmasını isteyebilir. |
| "Sonuçlar ne zaman açıklanacak?" | Dönem *Sonuçlar ilan edildi* durumuna alındığında sorgulama sayfasından görünür. |

---

## 8. Dikkat edilecekler

- **Beyan esaslıdır:** sistem kimlik doğrulaması yapmaz. Asil listeye girenlerden belgelerin
  asıllarını görmek ve beyanları karşılaştırmak komisyonun sorumluluğundadır.
- **Kişisel veri:** başvuru ekranlarındaki bilgiler kişisel veridir; ekran görüntüsü almaktan,
  yazıcıdan çıkarıp ortada bırakmaktan kaçının. Her belge görüntüleme kayıt altındadır
  (**Kayıtlar** sayfası).
- **Şifre paylaşmayın:** her komisyon üyesinin kendi hesabı olmalıdır; puanların kime ait olduğu
  bu sayede belli olur.
- **Dönem kapanış tarihini** başvuru duyurusundakiyle aynı tutun; öğrenciler son gün yoğunlaşır.
