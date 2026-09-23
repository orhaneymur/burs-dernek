#!/usr/bin/env bash
# Uctan uca duman testi: donem acar, tek sayfalik basvuruyu transkriptle birlikte
# gonderir, komisyon puani girer ve siralamayi uygular.
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
var_mi() { # var_mi "aciklama" "desen" [dosya]
  if grep -q "$2" "${3:-$TMP/govde.html}"; then kontrol "$1" "var" "var"; else kontrol "$1" "var" "yok"; fi
}

# Windows (Git Bash) uzerinde curl'a dosya yolunu dogru bicimde verir
yol() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi; }

csrf() { awk '$6 == "csrf" { print $7 }' "$1" | tail -1; }

durum() { # durum <cookie> <yol> [alan=deger ...]
  local jar="$1" yol="$2"; shift 2
  local args=()
  for kv in "$@"; do args+=(--data-urlencode "$kv"); done
  curl -s -o "$TMP/govde.html" -w '%{http_code}' -b "$jar" -c "$jar" "${args[@]}" "$BASE$yol"
}

echo "== 1. Genel sayfalar"
kontrol "GET /saglik" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/saglik")"
kontrol "GET /" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/")"
kontrol "GET /kvkk" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/kvkk")"
kontrol "kaldirilan /sorgula 404 verir" "404" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/sorgula")"
kontrol "yetkisiz /yonetim/ yonlendirir" "303" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/yonetim/")"

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

echo "== 4. Basvuru formu (tek sayfa)"
curl -s -o "$TMP/form.html" -c "$OGRENCI" "$BASE/basvuru/$SLUG"
kontrol "GET /basvuru/$SLUG" "200" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/basvuru/$SLUG")"
var_mi "form tek sayfa (7 bolum)" 'id="bolum-onay"' "$TMP/form.html"
var_mi "yalnizca transkript isteniyor" 'name="transkript"' "$TMP/form.html"
if grep -q 'name="ogrenci_belgesi"\|name="ikametgah"\|name="gelir_belgesi"' "$TMP/form.html"; then
  kontrol "diger belgeler kaldirildi" "yok" "var"
else
  kontrol "diger belgeler kaldirildi" "yok" "yok"
fi

printf '%%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%%%EOF\n' > "$TMP/transkript.pdf"

# Turkce karakterler kabuk kodlamasindan etkilenmesin diye UTF-8 dosyadan gonderilir
printf 'Ay\xc5\x9fe G\xc3\xbcl\xc5\x9fah' > "$TMP/ad.txt"
printf '\xc3\x87if\xc3\xa7io\xc4\x9flu' > "$TMP/soyad.txt"
printf 'K\xc4\xb1rklareli' > "$TMP/il.txt"
printf '\xc4\xb0stanbul Teknik \xc3\x9cniversitesi' > "$TMP/uni.txt"
printf '\xc5\x9eehir ve B\xc3\xb6lge Planlama' > "$TMP/bolum.txt"

gonder() { # gonder <ek-alanlar...>  -> HTTP kodu
  curl -s -o "$TMP/govde.html" -w '%{http_code}' -b "$OGRENCI" -c "$OGRENCI" \
    -F "csrf=$(csrf "$OGRENCI")" \
    -F "national_id=${TC:-10000000146}" \
    -F "first_name=<$(yol "$TMP/ad.txt")" -F "last_name=<$(yol "$TMP/soyad.txt")" \
    -F "birth_date=2004-05-10" -F "gender=kadin" -F "phone=5551112233" \
    -F "email=ayse@example.com" -F "city=<$(yol "$TMP/il.txt")" -F "district=Luleburgaz" \
    -F "university=<$(yol "$TMP/uni.txt")" -F "department=<$(yol "$TMP/bolum.txt")" \
    -F "class_year=3" -F "gpa=3,20" -F "gpa_scale=4" \
    -F "household_income=24000" -F "household_size=4" -F "housing_type=kira" \
    -F "father_status=calismiyor" -F "mother_status=calismiyor" \
    -F "sibling_count=3" -F "student_sibling_count=2" -F "parents_status=bosanmis" \
    -F "sports_club=Luleburgaz Genclik Kulubu" \
    -F "volunteer_text=Mahalledeki ogrencilere ucretsiz ders veriyorum." \
    -F "motivation_text=Ailemin gelir durumu nedeniyle destege ihtiyacim var." \
    "$@" "$BASE/basvuru/$SLUG"
}

echo "== 5. Dogrulama kurallari"
kod=$(TC=11111111111 gonder -F "kvkk=on" -F "beyan=on" -F "transkript=@$(yol "$TMP/transkript.pdf");type=application/pdf")
kontrol "gecersiz T.C. reddedilir" "400" "$kod"
var_mi "T.C. hata mesaji gosterilir" "Geçerli bir T.C."
var_mi "girilen degerler korunur (Turkce)" "Teknik Üniversitesi"

kod=$(gonder -F "beyan=on" -F "transkript=@$(yol "$TMP/transkript.pdf");type=application/pdf")
kontrol "KVKK onayi olmadan reddedilir" "400" "$kod"

kod=$(gonder -F "kvkk=on" -F "beyan=on")
kontrol "transkriptsiz reddedilir" "400" "$kod"
var_mi "transkript uyarisi gosterilir" "Transkriptinizi yükleyin"

printf 'metin dosyasi\n' > "$TMP/kotu.txt"
kod=$(gonder -F "kvkk=on" -F "beyan=on" -F "transkript=@$(yol "$TMP/kotu.txt")")
kontrol "izinsiz dosya turu reddedilir" "400" "$kod"

echo "== 6. Basarili gonderim"
kod=$(gonder -F "kvkk=on" -F "beyan=on" -F "transkript=@$(yol "$TMP/transkript.pdf");type=application/pdf")
kontrol "POST /basvuru/$SLUG" "303" "$kod"

curl -s -b "$OGRENCI" -c "$OGRENCI" "$BASE/basvurunuz-alindi" > "$TMP/tamam.html"
var_mi "onay ekrani gosteriliyor" "Başvurunuz alındı" "$TMP/tamam.html"
if grep -q 'LF-[A-Z0-9]\{4\}-[A-Z0-9]\{4\}' "$TMP/tamam.html"; then
  kontrol "ogrenciye referans numarasi gosterilmiyor" "yok" "var"
else
  kontrol "ogrenciye referans numarasi gosterilmiyor" "yok" "yok"
fi

kod=$(gonder -F "kvkk=on" -F "beyan=on" -F "transkript=@$(yol "$TMP/transkript.pdf");type=application/pdf")
kontrol "ayni T.C. ile ikinci basvuru engellenir" "400" "$kod"
var_mi "mukerrer basvuru mesaji" "zaten bir başvuru"

echo "== 7. Yonetim tarafi"
kod=$(durum "$YONETIM" "/yonetim/basvurular")
kontrol "basvuru listesi" "200" "$kod"
ID=$(grep -o '/yonetim/basvuru/[0-9]\+' "$TMP/govde.html" | head -1 | grep -o '[0-9]\+')
if [ -n "$ID" ]; then kontrol "listede basvuru var (#$ID)" "var" "var"; else kontrol "listede basvuru var" "var" "yok"; fi

kod=$(durum "$YONETIM" "/yonetim/basvuru/$ID")
kontrol "basvuru detayi" "200" "$kod"
var_mi "puan kirilimi gosteriliyor" "Kriter Kırılımı"
var_mi "Turkce karakterler dogru kaydedildi" "Çifçioğlu"
DONEM_ID=$(grep -o 'donem=[0-9]\+' "$TMP/govde.html" | head -1 | grep -o '[0-9]\+')
BELGE_ID=$(grep -o '/yonetim/belge/[0-9]\+' "$TMP/govde.html" | head -1 | grep -o '[0-9]\+')

kontrol "transkript indirme" "200" "$(curl -s -o /dev/null -w '%{http_code}' -b "$YONETIM" "$BASE/yonetim/belge/$BELGE_ID")"
kontrol "belge oturumsuz indirilemez" "303" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/yonetim/belge/$BELGE_ID")"

kod=$(durum "$YONETIM" "/yonetim/basvuru/$ID/puan" "csrf=$(csrf "$YONETIM")" "puan=85" "notlar=Durumu uygun.")
kontrol "komisyon puani kaydedildi" "303" "$kod"
kod=$(durum "$YONETIM" "/yonetim/siralama/hesapla" "csrf=$(csrf "$YONETIM")" "donem=$DONEM_ID")
kontrol "puanlar yeniden hesaplandi" "303" "$kod"
kod=$(durum "$YONETIM" "/yonetim/siralama/uygula" "csrf=$(csrf "$YONETIM")" "donem=$DONEM_ID" "kontenjan=2" "yedek=1")
kontrol "kontenjan uygulandi" "303" "$kod"
kod=$(durum "$YONETIM" "/yonetim/basvuru/$ID")
var_mi "basvuru asil listeye alindi" "badge-asil"

curl -s -b "$YONETIM" "$BASE/yonetim/disaaktar?donem=$DONEM_ID" > "$TMP/disa.csv"
var_mi "CSV disa aktarim" "Takip Kodu" "$TMP/disa.csv"

echo "== 8. Guvenlik"
kontrol "CSRF'siz gonderim reddedilir" "403" \
  "$(curl -s -o /dev/null -w '%{http_code}' -b "$YONETIM" -X POST --data "puan=99" "$BASE/yonetim/basvuru/$ID/puan")"
kontrol "islem kayitlari erisilebilir" "200" "$(durum "$YONETIM" "/yonetim/kayitlar")"

echo
printf 'Sonuc: \033[32m%d gecti\033[0m, \033[31m%d kaldi\033[0m\n' "$gecti" "$kaldi"
[ "$kaldi" -eq 0 ]
