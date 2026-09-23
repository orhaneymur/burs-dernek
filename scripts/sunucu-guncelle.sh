#!/usr/bin/env bash
# Sunucuda calistirilir: depoyu gunceller, imaji yeniden uretir, k3s'e aktarir
# ve yeni surumu yayina alir.
#
#   ssh root@116.203.80.86 '/opt/burs-dernek/scripts/sunucu-guncelle.sh'
#
# Belirli bir surum etiketi vermek icin:  sunucu-guncelle.sh 1.1.0
set -euo pipefail

DEPO="${DEPO:-/opt/burs-dernek}"
AD="${AD:-burs}"          # namespace
cd "$DEPO"

echo "== Depo guncelleniyor"
# Dagitim kopyasidir: yereldeki farklar (ornegin dosya izinleri) korunmaz
git config core.fileMode false
git fetch --quiet origin
git reset --quiet --hard origin/main
chmod +x scripts/*.sh

SURUM="${1:-$(git rev-parse --short HEAD)}"
IMAJ="lafed-burs:${SURUM}"

echo "== Imaj uretiliyor: $IMAJ"
docker build -q -t "$IMAJ" .

echo "== k3s containerd deposuna aktariliyor"
docker save "$IMAJ" | k3s ctr images import -

echo "== Yayina aliniyor"
kubectl -n "$AD" set image deploy/burs "burs=$IMAJ"
kubectl -n "$AD" rollout status deploy/burs --timeout=180s

echo "== Saglik kontrolu"
# Ingress yeni pod'u fark edene kadar kisa bir sure yeniden denenir
KOD=""
for _ in 1 2 3 4 5 6; do
  KOD=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: burs.lafed.org.tr" http://127.0.0.1/saglik || true)
  [ "$KOD" = "200" ] && break
  sleep 3
done
echo "ingress -> $KOD"

echo "== Eski imajlar temizleniyor (son 3 surum korunur)"
docker images lafed-burs --format '{{.Repository}}:{{.Tag}} {{.CreatedAt}}' \
  | sort -k2 -r | tail -n +4 | cut -d' ' -f1 \
  | xargs -r docker rmi >/dev/null 2>&1 || true

echo "Tamamlandi: $IMAJ"
