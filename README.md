# LAFED Burs Başvuru ve Değerlendirme Sistemi

Üniversite öğrencilerinden çevrim içi burs başvurusu toplayan, başvuruları ölçütlere göre
otomatik puanlayan ve burs komisyonunun değerlendirmesiyle sıralama üreten hafif bir web uygulaması.

**Canlı:** <https://burs.lafed.org.tr> · **Sunucu:** 116.203.80.86 (k3s, `burs` namespace) · **Depo:** <https://github.com/orhaneymur/burs-dernek>

| | |
|---|---|
| Dil / çatı | Go 1.23, standart kütüphane (`net/http` + `html/template`) |
| Veritabanı | MariaDB / MySQL |
| Ön yüz | Sunucu taraflı HTML + tek CSS dosyası; JavaScript olmadan da tam çalışır |
| Docker imajı | **12,9 MB** (`scratch` tabanlı tek statik binary) |
| Çalışma anı bellek | **~5 MB** (uygulama), ~85 MB (MariaDB) — canlı ölçüm |
| Dış bağımlılık | Yalnızca MySQL sürücüsü ve `bcrypt` |

---

## İçindekiler

1. [Ne yapar?](#ne-yapar)
2. [Hızlı deneme (yerel)](#hızlı-deneme-yerel)
3. [Kubernetes'e kurulum](#kubernetese-kurulum)
4. [İlk ayarlar](#ilk-ayarlar)
5. [Puanlama nasıl çalışır?](#puanlama-nasıl-çalışır)
6. [Ortam değişkenleri](#ortam-değişkenleri)
7. [Yedekleme ve geri yükleme](#yedekleme-ve-geri-yükleme)
8. [Güvenlik ve KVKK](#güvenlik-ve-kvkk)
9. [Geliştirme](#geliştirme)
10. [Sorun giderme](#sorun-giderme)

---

## Ne yapar?

**Öğrenci tarafı** — tek bir bağlantı (`/basvuru/2026-2027`) üzerinden:

- 7 adımlık başvuru formu: kimlik, eğitim, ekonomik durum, aile, burs/sosyal, belgeler, özet
- Her adım otomatik kaydedilir; öğrenci yarıda bırakıp **T.C. kimlik no + doğum tarihi** ile geri dönebilir
- Belge yükleme (PDF/JPG/PNG), içerik imzasına göre tür doğrulaması
- Gönderimde **takip kodu** (`LF-XXXX-XXXX`) verilir; durum sorgulaması bu kodla yapılır
- KVKK aydınlatma metni ve açık rıza onayı, onay zamanı kayıt altında

**Federasyon tarafı** — `/yonetim`:

- **Dönem yönetimi:** tarih aralığı, kontenjan (asil + yedek), durum (taslak → açık → kapalı → ilan)
- **Puanlama kriterleri:** 8 kriterin ağırlıkları ve gelir kademeleri panelden düzenlenir
- **Başvuru listesi:** üniversite, durum, asgari puan ve serbest metin filtreleri; sayfalama
- **Başvuru detayı:** tüm beyanlar, belgeler, kriter kırılımı ve dikkat uyarıları
- **Komisyon puanlama:** her üye 0-100 arası kendi puanını ve notunu girer, ortalaması alınır
- **Sıralama ve kontenjan:** nihai puana göre listeyi asil/yedek olarak işaretler
- **CSV dışa aktarım** (Excel uyumlu), **işlem kayıtları**, kullanıcı yönetimi, kurum ayarları

**Roller:** *Yönetici* (tüm yetkiler) ve *Komisyon üyesi* (başvuruları görür ve puanlar; dönem,
kullanıcı ve sıralama işlemlerine erişemez, T.C. kimlik numaralarını maskeli görür).

---

## Hızlı deneme (yerel)

Docker ve Docker Compose yeterlidir:

```bash
docker compose up --build
```

- Başvuru sayfası: <http://localhost:8080>
- Yönetim girişi: <http://localhost:8080/yonetim/giris> — `admin` / `gelistirme-sifresi`

8080 portu doluysa: `PORT=8090 docker compose up --build`

Uçtan uca duman testi (dönem açar, başvuru yapar, belge yükler, puanlar, sıralar):

```bash
BASE=http://localhost:8080 ./scripts/duman-testi.sh
```

---

## Kubernetes'e kurulum

Sistem `116.203.80.86` sunucusunda **k3s** üzerinde, `burs` namespace'inde çalışmaktadır.
Ingress **Traefik**, depolama **local-path**, TLS **Cloudflare** tarafında sonlanır.
Aşağıdaki adımlar bu kurulumun birebir tekrarıdır.

### 1. Depoyu sunucuya alın

```bash
cd /opt && git clone https://github.com/orhaneymur/burs-dernek.git
cd burs-dernek
```

### 2. İmajı üretip küme deposuna aktarın

Harici bir kayıt defteri gerekmez; imaj sunucuda üretilip k3s'in containerd deposuna aktarılır:

```bash
docker build -t lafed-burs:1.0.0 .
docker save lafed-burs:1.0.0 | k3s ctr images import -
```

> Sunucuda Docker Hub oturumu süresi dolmuşsa `docker logout` ile anonim çekime geçin.

### 3. Namespace ve diskleri oluşturun

```bash
kubectl apply -f k8s/00-temel.yaml
```

### 4. Gizli değerleri üretin

Gerçek şifreler depoda tutulmaz; tek seferlik üretilip Secret'a yazılır:

```bash
DB_SIFRE=$(openssl rand -base64 24 | tr -d '/+=' | cut -c1-24)
ROOT_SIFRE=$(openssl rand -base64 24 | tr -d '/+=' | cut -c1-24)
ADMIN_SIFRE=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-16)

kubectl -n burs create secret generic burs-gizli   --from-literal=DATABASE_DSN="burs:${DB_SIFRE}@tcp(burs-mariadb:3306)/burs"   --from-literal=SECRET_KEY="$(openssl rand -hex 32)"   --from-literal=BOOTSTRAP_ADMIN_USER="admin"   --from-literal=BOOTSTRAP_ADMIN_PASS="${ADMIN_SIFRE}"   --from-literal=MARIADB_ROOT_PASSWORD="${ROOT_SIFRE}"   --from-literal=MARIADB_PASSWORD="${DB_SIFRE}"

echo "İlk yönetici şifresi: ${ADMIN_SIFRE}"
```

> `SECRET_KEY` T.C. kimlik numaralarını şifreler. **Kaybederseniz kayıtlı T.C. numaraları
> çözülemez.** Kümeden ayrıca yedekleyin:
> `kubectl -n burs get secret burs-gizli -o jsonpath='{.data.SECRET_KEY}' | base64 -d`

### 5. Bileşenleri kurun

```bash
kubectl apply -f k8s/10-mariadb.yaml    # sunucunuzda MySQL varsa atlayın
kubectl apply -f k8s/20-uygulama.yaml
kubectl apply -f k8s/30-yedekleme.yaml
kubectl -n burs rollout status deploy/burs
```

### 6. Cloudflare ve ingress

Cloudflare proxy'si origin'e **443** üzerinden bağlanır. Bu yüzden ingress'te
`traefik.ingress.kubernetes.io/router.entrypoints` **tanımlanmaz** — Traefik router'ı
hem `web` (80) hem `websecure` (443) entrypoint'inde oluşturur; aksi hâlde Cloudflare
üzerinden 404 alınır.

Doğrulama:

```bash
curl -H 'Host: burs.lafed.org.tr' http://127.0.0.1/saglik      # origin 80
curl -k -H 'Host: burs.lafed.org.tr' https://127.0.0.1/saglik  # origin 443
curl https://burs.lafed.org.tr/saglik                          # Cloudflare üzerinden
```

Cloudflare'ı **Full (strict)** moduna alacaksanız origin'e geçerli bir sertifika gerekir
(cert-manager veya Cloudflare Origin CA) ve ingress'e `tls:` bölümü eklenmelidir.

### 7. Canlı doğrulama

```bash
BASE=https://burs.lafed.org.tr ADMIN_USER=admin ADMIN_PASS='<sifre>' ./scripts/duman-testi.sh
```

Test bir deneme dönemi ve başvurusu oluşturur; sonrasında temizlemeyi unutmayın:

```sql
DELETE FROM periods WHERE slug LIKE 'test-%';
```

### Yeni sürüm yayınlama

```bash
ssh root@116.203.80.86 '/opt/burs-dernek/scripts/sunucu-guncelle.sh'
```

Betik depoyu günceller, imajı yeniden üretir, k3s'e aktarır, `deploy/burs`'u yeni imaja
geçirir, sağlık kontrolü yapar ve eski imajları temizler.

### Mevcut bir MySQL sunucusunu kullanmak

`k8s/10-mariadb.yaml` dosyasını uygulamayın ve Secret'taki `DATABASE_DSN` değerini kendi
sunucunuza yönlendirin. Veritabanı `utf8mb4` / `utf8mb4_unicode_ci` olmalıdır:

```sql
CREATE DATABASE burs CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'burs'@'%' IDENTIFIED BY 'sifre';
GRANT ALL PRIVILEGES ON burs.* TO 'burs'@'%';
```

### Neden tek kopya (replica: 1)?

Yüklenen belgeler `ReadWriteOnce` bir disk üzerinde tutulur ve hız limiti bellekte çalışır.
Birkaç bin başvuru için tek kopya fazlasıyla yeterlidir (uygulama canlıda ~5 MB bellek
kullanıyor). Yatay ölçekleme gerekirse belgelerin S3/MinIO'ya taşınması gerekir.

## İlk ayarlar

1. **Giriş yapın.** `BOOTSTRAP_ADMIN_PASS` boş bırakıldıysa şifre ilk açılışta günlüklere yazılır:
   `kubectl -n burs logs -l app=burs | grep 'Ilk yonetici'`
2. **Şifrenizi değiştirin** (sağ üst → Şifre). İlk hesap bunu zorunlu olarak ister.
3. **Kurum ayarları** → federasyonun resmî adını, iletişim bilgisini ve **KVKK aydınlatma metnini**
   kendi kurumunuza göre düzenleyin. Varsayılan metin bir taslaktır; hukuki sorumluluk kurumunuza aittir.
4. **Kullanıcılar** → burs komisyonu üyelerini *komisyon* rolüyle ekleyin.
5. **Dönemler → Yeni dönem** → tarih aralığı, kontenjan ve ağırlıkları girin, durumu
   **"Başvurulara açık"** yapın.
6. Panelde görünen **başvuru bağlantısını** öğrencilerle paylaşın:
   `https://burs.lafed.org.tr/basvuru/2026-2027`
7. Gerekirse **Kriterler** sayfasından ağırlıkları ve gelir kademelerini güncelleyin.

### Dönem bittiğinde

1. Dönemi **"Başvurular kapalı"** durumuna alın (yeni başvuru alınmaz, mevcutlar değerlendirilir)
2. Komisyon üyeleri başvuruları puanlar
3. **Sıralama** → "Puanları yeniden hesapla" → "Kontenjanı uygula"
4. Listeyi kontrol edip gerekirse tek tek düzeltin (başvuru detayından durum değiştirilebilir)
5. Dönemi **"Sonuçlar ilan edildi"** durumuna alın — öğrenciler takip koduyla sonucu görebilir

---

## Puanlama nasıl çalışır?

**Nihai puan = otomatik puan × %60 + komisyon puanı × %40** (ağırlıklar dönem bazında değiştirilir).
Komisyon puanı girilmemişse yalnızca otomatik puan kullanılır.

Otomatik puan 0-100 ölçeğindedir; her kriter 0-1 arası bir oran üretir ve ağırlığıyla çarpılır.
Ağırlıkların toplamı 100 değilse sistem orantılayarak 100'e normalize eder.

| Kriter | Varsayılan ağırlık | Oran nasıl belirlenir |
|---|---:|---|
| Kişi başı gelir | 30 | Hane geliri ÷ kişi sayısı, panelden düzenlenen kademelere göre |
| Akademik başarı | 20 | GANO 2,00 taban – 4,00 tavan. Hazırlık/1. sınıfa 0,5 taban oran |
| Barınma durumu | 10 | Kira 1,00 · özel yurt 0,90 · devlet yurdu 0,60 · akraba 0,50 · aile yanı 0,20 |
| Kardeş yükü | 10 | Kardeş başına 0,15 + öğrenci kardeş başına 0,25 (en çok 1,00) |
| Ebeveyn durumu | 10 | Boşanmış 0,60 · bir ebeveyn vefat 0,85 · ikisi 1,00; çalışmayan ebeveyn +0,15 |
| Özel durum | 10 | Engelli birey 0,70 · şehit/gazi yakını 0,70 (en çok 1,00) |
| Başka burs almama | 5 | Almıyorsa 1,00; tutara göre 0,50 / 0,25 / 0,00 |
| Sosyal katılım | 5 | Gönüllülük beyanı 0,60 + kulüp üyeliği 0,40 |

**Kesinti:** ailede gayrimenkul veya araç beyanı varsa toplam puandan 5 puan düşülür (ayarlanabilir).

**Varsayılan gelir kademeleri** (kişi başı aylık, TL): ≤7.500 → 1,00 · ≤12.500 → 0,85 ·
≤17.500 → 0,70 · ≤25.000 → 0,50 · ≤35.000 → 0,30 · ≤50.000 → 0,10 · üzeri → 0,00.
**Bu değerleri her dönem güncel ekonomik koşullara göre gözden geçirin.**

Sistem hiçbir başvuruyu kendiliğinden elemez; asgari GANO altında kalma, mülkiyet beyanı,
sıfır gelir beyanı gibi durumlar başvuru detayında **uyarı** olarak gösterilir, kararı komisyon verir.

---

## Ortam değişkenleri

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `DATABASE_DSN` | — | **Zorunlu.** `kullanici:sifre@tcp(sunucu:3306)/veritabani` |
| `BASE_URL` | `https://burs.lafed.org.tr` | Bağlantı üretimi ve güvenli çerez kararı |
| `SECRET_KEY` | (otomatik üretilir) | 64 haneli hex. T.C. şifreleme + imzalama anahtarı |
| `DATA_DIR` | `/data` | Belgelerin ve otomatik anahtarın tutulduğu dizin |
| `LAFED_ADDR` | `:8080` | Dinlenecek adres |
| `MAX_UPLOAD_MB` | `8` | Dosya başına yükleme sınırı |
| `TRUST_PROXY` | `true` | Ingress arkasında gerçek istemci IP'si için |
| `BOOTSTRAP_ADMIN_USER` | `admin` | İlk yönetici kullanıcı adı |
| `BOOTSTRAP_ADMIN_PASS` | (rastgele) | İlk yönetici şifresi; boşsa üretilip günlüğe yazılır |
| `TZ` | `Europe/Istanbul` | Saat dilimi (veri imaja gömülüdür) |

`SECRET_KEY` verilmezse `DATA_DIR/secret.key` dosyasında üretilir. Üretimde Secret olarak vermeniz önerilir.

---

## Yedekleme ve geri yükleme

`k8s/30-yedekleme.yaml` her gece 03:15'te veritabanı dökümünü ve belge arşivini
`burs-yedek` diskine yazar, 30 günden eskileri siler.

```bash
make yedek-al                                   # elle tetikle
kubectl -n burs get cronjob,job                 # durum
```

**Geri yükleme:**

```bash
# Veritabanı
kubectl -n burs exec -i deploy/burs-mariadb -- \
  sh -c 'mariadb -u root -p"$MARIADB_ROOT_PASSWORD" burs' < burs-20260101-0315.sql

# Belgeler
kubectl -n burs cp belgeler-20260101-0315.tar.gz <burs-pod>:/tmp/
kubectl -n burs exec <burs-pod> -- tar -xzf /tmp/belgeler-20260101-0315.tar.gz -C /data
```

> Yedeklerin bir kopyasını küme dışında da saklayın. `secret.key` dosyasını (veya `SECRET_KEY`
> değerini) yedeklemeden ayrı, güvenli bir yerde tutun — o olmadan T.C. numaraları çözülemez.

---

## Güvenlik ve KVKK

- **Şifreleme:** T.C. kimlik numaraları AES-256-GCM ile şifreli saklanır. Mükerrer başvuru
  kontrolü için ayrıca HMAC-SHA256 parmak izi tutulur (geri döndürülemez).
- **Belgeler:** web kökünden erişilemez; yalnızca oturum açmış yetkililerin indirme
  uç noktasından sunulur ve **her görüntüleme işlem kaydına yazılır**.
- **Oturum:** şifreler bcrypt (maliyet 12) ile saklanır, oturumlar veritabanında tutulur ve
  8 saatte sona erer. Giriş denemeleri 15 dakikada 10 ile sınırlıdır.
- **CSRF:** tüm POST istekleri çerez ile form alanının eşleşmesini gerektirir.
- **Spam önlemleri:** IP başına saatlik başvuru/sorgu limiti, gizli tuzak alan, aynı dönemde
  aynı T.C. ile ikinci başvurunun engellenmesi.
- **Başlıklar:** `Content-Security-Policy` (inline script yok), `X-Frame-Options: DENY`,
  `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`.
- **Kapsayıcı:** root olmayan kullanıcı, salt okunur kök dosya sistemi, tüm capability'ler düşürülür.
- **Arama motorları:** tüm sayfalarda `noindex`.

**KVKK sorumluluğu:** Varsayılan aydınlatma metni bir taslaktır. Yayına almadan önce kurumunuzun
resmî unvanı, iletişim bilgileri ve saklama süreleriyle güncelleyin (Yönetim → Kurum Ayarları).
Saklama süresi dolan başvuruların silinmesi şu anda **elle** yapılır; yönetim panelinden başvuru
silindiğinde belgeleri de diskten kaldırılır.

---

## Geliştirme

```
cmd/server/          giriş noktası
internal/config/     ortam değişkenleri
internal/model/      veri yapıları, alan sözlükleri, seçenek listeleri
internal/security/   şifreleme, parola, T.C. doğrulama, hız limiti
internal/scoring/    puanlama motoru (kriterler ve ağırlıklar)
internal/store/      veritabanı katmanı + şema geçişleri (embed)
internal/web/        HTTP uç noktaları, şablonlar, statik dosyalar (embed)
scripts/             uçtan uca duman testi
k8s/                 Kubernetes manifestleri
```

Şablonlar ve statik dosyalar binary içine gömülüdür (`go:embed`) — dağıtımda tek dosya yeterlidir.

```bash
make test        # birim testleri (puanlama, güvenlik, şablon ayrıştırma)
make vet
make dev         # docker compose ile yerel ortam
```

Yeni bir şema değişikliği için `internal/store/migrations/` altına `002_*.sql` ekleyin;
açılışta sırayla ve bir kez uygulanır.

---

## Sorun giderme

| Belirti | Bakılacak yer |
|---|---|
| Pod `CrashLoopBackOff` | `make gunluk` — genelde `DATABASE_DSN` hatalı veya DB henüz hazır değil (uygulama 60 sn bekler) |
| "Bu dönem şu anda başvuruya açık değil" | Dönem durumu "Başvurulara açık" mı, tarih aralığı güncel mi |
| Belge yüklenmiyor | Ingress'te `proxy-body-size` ≥ `MAX_UPLOAD_MB`, dosya PDF/JPG/PNG mi |
| Türkçe karakter bozuk | Veritabanı `utf8mb4_unicode_ci` mi |
| Giriş yapılamıyor, şifre bilinmiyor | `kubectl -n burs exec deploy/burs-mariadb -- mariadb ...` ile `users` tablosundan hesabı silip pod'u yeniden başlatın; ilk yönetici yeniden oluşturulur |
| Sonuçlar öğrenciye görünmüyor | Dönem "Sonuçlar ilan edildi" durumunda mı |

---

Bu depo sürüm denetimi için hazırdır ancak henüz bir işleme (commit) yapılmamıştır.
