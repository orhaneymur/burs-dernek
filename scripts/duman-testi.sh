#!/usr/bin/env bash
# Uctan uca duman testi: donem acar, ogrenci basvurusu yapar, belge yukler,
# basvuruyu gonderir, komisyon puani girer ve siralamayi uygular.
#
# Kullanim:  BASE=http://localhost:8090 ./scripts/duman-testi.sh
set -euo pipefail

BASE="${BASE:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-gelistirme-sifresi}"
SLUG="test-$(date +%s)"

TMP="$(mktemp -d)"
YONETIM="$TMP/yonetim.cookie"
OGRENCI="$TMP/ogrenci.cookie"
trap 'rm -rf "$TMP"' EXIT

gecti=0; kaldi=0
kontrol() { # kontrol "aciklama" "beklenen" "gelen"
  if [ "$2" = "$3" ]; then
    printf '  \033[32mOK\033[0m   %s\n' "$1"; gecti=$((gecti+1))
  else
    printf '  \033[31mHATA\033[0m %s (beklenen: %s, gelen: %s)\n' "$1" "$2" "$3"; kaldi=$((kaldi+1))
  fi
}

# Windows (Git Bash) uzerinde curl'a dosya yolunu dogru bicimde verir
yol() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi; }

csrf() { # csrf <cookie-dosyasi>
  awk '$6 == "csrf" { print $7 }' "$1" | tail -1
}

durum() { # durum <cookie> <yol> [alan=deger ...]
  local jar="$1" yol="$2"; shift 2
  local args=()
  for kv in "$@"; do args+=(--data-urlencode "$kv"); done
  curl -s -o "$TMP/govde.html" -w '%{http_code}' -b "$jar" -c "$jar" \
    "${args[@]}" "$BASE$yol"
}

echo "== 1. Saglik kontrolu"
kontrol "GET /saglik" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/saglik")"
kontrol "GET /" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/")"
kontrol "GET /kvkk" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/kvkk")"
kontrol "GET /sorgula" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/sorgula")"
kontrol "yetkisiz /yonetim/ girisi yonlendirir" "303" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/yonetim/")"

echo "== 2. Yonetici girisi"
curl -s -o /dev/null -c "$YONETIM" "$BASE/yonetim/giris"
kod=$(durum "$YONETIM" "/yonetim/giris" "csrf=$(csrf "$YONETIM")" "username=$ADMIN_USER" "password=$ADMIN_PASS")
kontrol "POST /yonetim/giris" "303" "$kod"
kontrol "GET /yonetim/ (oturum acik)" "200" "$(durum "$YONETIM" "/yonetim/")"

echo "== 3. Donem olusturma"
BASLANGIC=$(date -d '-1 day' +%Y-%m-%dT%H:%M 2>/dev/null || date -v-1d +%Y-%m-%dT%H:%M)
BITIS=$(date -d '+30 days' +%Y-%m-%dT%H:%M 2>/dev/null || date -v+30d +%Y-%m-%dT%H:%M)
kod=$(durum "$YONETIM" "/yonetim/donem/yeni" \
  "csrf=$(csrf "$YONETIM")" "name=Duman Testi Donemi" "slug=$SLUG" \
  "description=Otomatik test" "opens_at=$BASLANGIC" "closes_at=$BITIS" \
  "quota=2" "reserve_quota=1" "auto_weight=60" "committee_weight=40" \
  "status=acik" "require_docs=on")
kontrol "POST /yonetim/donem/yeni" "303" "$kod"
kontrol "GET /basvuru/$SLUG" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/basvuru/$SLUG")"

echo "== 4. Ogrenci basvurusu"
curl -s -o /dev/null -c "$OGRENCI" "$BASE/basvuru/$SLUG"
kod=$(durum "$OGRENCI" "/basvuru/$SLUG" "csrf=$(csrf "$OGRENCI")" "kvkk=on")
kontrol "basvuru baslatma" "303" "$kod"

kod=$(durum "$OGRENCI" "/basvuru/adim/1" "csrf=$(csrf "$OGRENCI")" "yon=ileri" \
  "national_id=10000000146" "first_name=Ayse" "last_name=Yilmaz" \
  "birth_date=2004-05-10" "gender=kadin" "phone=5551112233" \
  "email=ayse@example.com" "city=Kirklareli" "district=Luleburgaz" "address=Test Mah. 1")
kontrol "adim 1 (kimlik)" "303" "$kod"

kod=$(durum "$OGRENCI" "/basvuru/adim/1" "csrf=$(csrf "$OGRENCI")" "yon=ileri" \
  "national_id=11111111111" "first_name=Ayse" "last_name=Yilmaz" \
  "birth_date=2004-05-10" "phone=5551112233" "email=ayse@example.com" "city=Kirklareli")
kontrol "gecersiz T.C. reddedilir" "200" "$kod"
grep -q "Geçerli bir T.C." "$TMP/govde.html" && kontrol "T.C. hata mesaji gosterilir" "var" "var" \
  || kontrol "T.C. hata mesaji gosterilir" "var" "yok"

kod=$(durum "$OGRENCI" "/basvuru/adim/2" "csrf=$(csrf "$OGRENCI")" "yon=ileri" \
  "university=Trakya Universitesi" "faculty=Muhendislik" "department=Bilgisayar Muh." \
  "class_year=3" "student_no=1234567" "education_type=orgun" "gpa=3,20" "gpa_scale=4")
kontrol "adim 2 (egitim)" "303" "$kod"

kod=$(durum "$OGRENCI" "/basvuru/adim/3" "csrf=$(csrf "$OGRENCI")" "yon=ileri" \
  "household_income=24000" "household_size=4" "father_status=calismiyor" \
  "mother_status=calismiyor" "housing_type=kira" "housing_cost=6000")
kontrol "adim 3 (ekonomik durum)" "303" "$kod"

kod=$(durum "$OGRENCI" "/basvuru/adim/4" "csrf=$(csrf "$OGRENCI")" "yon=ileri" \
  "sibling_count=3" "student_sibling_count=2" "parents_status=bosanmis")
kontrol "adim 4 (aile)" "303" "$kod"

kod=$(durum "$OGRENCI" "/basvuru/adim/4" "csrf=$(csrf "$OGRENCI")" "yon=ileri" \
  "sibling_count=1" "student_sibling_count=3" "parents_status=bosanmis")
kontrol "ogrenci kardes > kardes reddedilir" "200" "$kod"

kod=$(durum "$OGRENCI" "/basvuru/adim/5" "csrf=$(csrf "$OGRENCI")" "yon=ileri" \
  "volunteer_text=Mahalledeki ilkokul ogrencilerine iki yildir ucretsiz matematik dersi veriyorum." \
  "sports_club=Luleburgaz Genclik Kulubu" "motivation_text=Ailemin gelir durumu nedeniyle destege ihtiyacim var.")
kontrol "adim 5 (burs ve sosyal)" "303" "$kod"

echo "== 5. Belge yukleme"
printf '%%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%%%EOF\n' > "$TMP/belge.pdf"
for tur in ogrenci_belgesi gelir_belgesi ikametgah; do
  kod=$(curl -s -o "$TMP/govde.html" -w '%{http_code}' -b "$OGRENCI" -c "$OGRENCI" \
    -F "csrf=$(csrf "$OGRENCI")" -F "kind=$tur" -F "dosya=@$(yol "$TMP/belge.pdf");type=application/pdf" \
    "$BASE/basvuru/belge")
  kontrol "belge yuklendi: $tur" "303" "$kod"
done

printf 'bu bir metin dosyasidir\n' > "$TMP/kotu.txt"
kod=$(curl -s -o /dev/null -w '%{http_code}' -b "$OGRENCI" -c "$OGRENCI" \
  -F "csrf=$(csrf "$OGRENCI")" -F "kind=transkript" -F "dosya=@$(yol "$TMP/kotu.txt")" "$BASE/basvuru/belge")
kontrol "izinsiz dosya turu reddedilir (yonlendirme + uyari)" "303" "$kod"

echo "== 6. Basvuruyu gonderme"
kod=$(durum "$OGRENCI" "/basvuru/adim/7")
kontrol "ozet ekrani" "200" "$kod"
kod=$(durum "$OGRENCI" "/basvuru/tamamla" "csrf=$(csrf "$OGRENCI")" "beyan=on")
kontrol "POST /basvuru/tamamla" "303" "$kod"

curl -s -b "$OGRENCI" -c "$OGRENCI" "$BASE/basvuru/tamamlandi" > "$TMP/tamam.html"
TAKIP=$(grep -o 'LF-[A-Z0-9]\{4\}-[A-Z0-9]\{4\}' "$TMP/tamam.html" | head -1)
if [ -n "$TAKIP" ]; then kontrol "takip kodu uretildi ($TAKIP)" "var" "var"; else kontrol "takip kodu uretildi" "var" "yok"; fi

echo "== 7. Durum sorgulama"
curl -s -o /dev/null -c "$TMP/sorgu.cookie" "$BASE/sorgula"
kod=$(durum "$TMP/sorgu.cookie" "/sorgula" "csrf=$(csrf "$TMP/sorgu.cookie")" \
  "tracking_code=$TAKIP" "birth_date=2004-05-10")
kontrol "dogru bilgilerle sorgulama" "200" "$kod"
grep -q "Değerlendirme sürüyor" "$TMP/govde.html" \
  && kontrol "durum gosteriliyor" "var" "var" || kontrol "durum gosteriliyor" "var" "yok"

kod=$(durum "$TMP/sorgu.cookie" "/sorgula" "csrf=$(csrf "$TMP/sorgu.cookie")" \
  "tracking_code=$TAKIP" "birth_date=1999-01-01")
grep -q "bulunamadı" "$TMP/govde.html" \
  && kontrol "yanlis dogum tarihi reddedilir" "var" "var" || kontrol "yanlis dogum tarihi reddedilir" "var" "yok"

echo "== 8. Yonetim tarafi"
kod=$(durum "$YONETIM" "/yonetim/basvurular")
kontrol "basvuru listesi" "200" "$kod"
ID=$(grep -o '/yonetim/basvuru/[0-9]\+' "$TMP/govde.html" | head -1 | grep -o '[0-9]\+')
if [ -n "$ID" ]; then kontrol "listede basvuru var (#$ID)" "var" "var"; else kontrol "listede basvuru var" "var" "yok"; fi

kod=$(durum "$YONETIM" "/yonetim/basvuru/$ID")
kontrol "basvuru detayi" "200" "$kod"
grep -q "Kriter Kırılımı" "$TMP/govde.html" \
  && kontrol "puan kirilimi gosteriliyor" "var" "var" || kontrol "puan kirilimi gosteriliyor" "var" "yok"
DONEM_ID=$(grep -o 'donem=[0-9]\+' "$TMP/govde.html" | head -1 | grep -o '[0-9]\+')

BELGE_ID=$(grep -o '/yonetim/belge/[0-9]\+' "$TMP/govde.html" | head -1 | grep -o '[0-9]\+')
kontrol "belge indirme" "200" \
  "$(curl -s -o /dev/null -w '%{http_code}' -b "$YONETIM" "$BASE/yonetim/belge/$BELGE_ID")"
kontrol "belge oturumsuz indirilemez" "303" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/yonetim/belge/$BELGE_ID")"

kod=$(durum "$YONETIM" "/yonetim/basvuru/$ID/puan" "csrf=$(csrf "$YONETIM")" "puan=85" "notlar=Durumu uygun.")
kontrol "komisyon puani kaydedildi" "303" "$kod"

kod=$(durum "$YONETIM" "/yonetim/siralama/hesapla" "csrf=$(csrf "$YONETIM")" "donem=$DONEM_ID")
kontrol "puanlar yeniden hesaplandi" "303" "$kod"
kod=$(durum "$YONETIM" "/yonetim/siralama/uygula" "csrf=$(csrf "$YONETIM")" "donem=$DONEM_ID" "kontenjan=2" "yedek=1")
kontrol "kontenjan uygulandi" "303" "$kod"

kod=$(durum "$YONETIM" "/yonetim/basvuru/$ID")
grep -q "badge-asil" "$TMP/govde.html" \
  && kontrol "basvuru asil listeye alindi" "var" "var" || kontrol "basvuru asil listeye alindi" "var" "yok"

curl -s -b "$YONETIM" "$BASE/yonetim/disaaktar?donem=$DONEM_ID" > "$TMP/disa.csv"
grep -q "Takip Kodu" "$TMP/disa.csv" \
  && kontrol "CSV disa aktarim" "var" "var" || kontrol "CSV disa aktarim" "var" "yok"

echo "== 9. Guvenlik kontrolleri"
kontrol "CSRF'siz gonderim reddedilir" "403" \
  "$(curl -s -o /dev/null -w '%{http_code}' -b "$YONETIM" -X POST \
     --data "puan=99" "$BASE/yonetim/basvuru/$ID/puan")"
kontrol "komisyon disi sayfa yetkisi (kayitlar)" "200" "$(durum "$YONETIM" "/yonetim/kayitlar")"

echo
printf 'Sonuc: \033[32m%d gecti\033[0m, \033[31m%d kaldi\033[0m\n' "$gecti" "$kaldi"
[ "$kaldi" -eq 0 ]
