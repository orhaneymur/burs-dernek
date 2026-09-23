# --- Derleme katmani -------------------------------------------------------
FROM golang:1.23-alpine AS build

WORKDIR /src

# Bagimliliklar once: katman onbellegi korunur
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Tamamen statik, sembol tablosuz tek dosya (~15 MB).
# Ayni katmanda veri dizini de dogru sahiplikle hazirlanir: bos bir birim
# baglandiginda Docker bu sahipligi devralir ve root olmayan kullanici yazabilir.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/burs ./cmd/server \
 && mkdir -p /veri/uploads \
 && chown -R 10001:10001 /veri

# --- Calisma katmani -------------------------------------------------------
FROM scratch

# Veritabanina TLS ile baglanilabilmesi icin kok sertifikalar
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/burs /burs
COPY --from=build --chown=10001:10001 /veri /data

# Root olmayan kullanici (K8s tarafinda fsGroup ile eslesmeli)
USER 10001:10001

ENV DATA_DIR=/data \
    LAFED_ADDR=:8080 \
    TZ=Europe/Istanbul

EXPOSE 8080
ENTRYPOINT ["/burs"]
