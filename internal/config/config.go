// Package config uygulamanin tum ayarlarini ortam degiskenlerinden okur.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr        string // dinlenecek adres, orn ":8080"
	DSN         string // MySQL/MariaDB baglanti dizesi
	DataDir     string // yuklenen belgelerin ve anahtarin tutuldugu dizin (PVC)
	BaseURL     string // https://burs.lafed.org.tr
	Secret      []byte // sifreleme ve imzalama anahtari (32 bayt)
	MaxUploadMB int64
	TrustProxy  bool // X-Forwarded-For basligina guvenilsin mi (ingress arkasinda evet)
	Debug       bool

	// Ilk kurulumda olusturulacak yonetici hesabi
	BootstrapUser string
	BootstrapPass string
}

func Load() (*Config, error) {
	c := &Config{
		Addr:          env("LAFED_ADDR", ":8080"),
		DSN:           os.Getenv("DATABASE_DSN"),
		DataDir:       env("DATA_DIR", "/data"),
		BaseURL:       strings.TrimRight(env("BASE_URL", "https://burs.lafed.org.tr"), "/"),
		MaxUploadMB:   envInt("MAX_UPLOAD_MB", 8),
		TrustProxy:    envBool("TRUST_PROXY", true),
		Debug:         envBool("DEBUG", false),
		BootstrapUser: env("BOOTSTRAP_ADMIN_USER", "admin"),
		BootstrapPass: os.Getenv("BOOTSTRAP_ADMIN_PASS"),
	}

	if c.DSN == "" {
		return nil, fmt.Errorf("DATABASE_DSN tanimli degil")
	}
	// Driver'in DATETIME kolonlarini time.Time olarak dondurmesi ve UTF-8 icin
	c.DSN = ensureDSNParams(c.DSN)

	if err := os.MkdirAll(filepath.Join(c.DataDir, "uploads"), 0o750); err != nil {
		return nil, fmt.Errorf("veri dizini olusturulamadi: %w", err)
	}

	secret, err := loadOrCreateSecret(c.DataDir)
	if err != nil {
		return nil, err
	}
	c.Secret = secret

	return c, nil
}

func (c *Config) UploadDir() string { return filepath.Join(c.DataDir, "uploads") }

// loadOrCreateSecret anahtari once SECRET_KEY ortam degiskeninden okur.
// Tanimli degilse veri dizininde uretip saklar; boylece ilk calistirmada
// ek yapilandirma gerekmez, ama uretimde Secret olarak verilmesi onerilir.
func loadOrCreateSecret(dataDir string) ([]byte, error) {
	if v := os.Getenv("SECRET_KEY"); v != "" {
		b, err := hex.DecodeString(strings.TrimSpace(v))
		if err != nil || len(b) < 32 {
			// Hex degilse ham metni kabul et, en az 32 karakter olmali
			if len(v) < 32 {
				return nil, fmt.Errorf("SECRET_KEY en az 32 karakter (veya 64 haneli hex) olmali")
			}
			return []byte(v), nil
		}
		return b, nil
	}

	path := filepath.Join(dataDir, "secret.key")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 64 {
		out, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err == nil {
			return out, nil
		}
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, fmt.Errorf("anahtar yazilamadi: %w", err)
	}
	return key, nil
}

func ensureDSNParams(dsn string) string {
	add := func(dsn, kv string) string {
		key := strings.SplitN(kv, "=", 2)[0]
		if strings.Contains(dsn, key+"=") {
			return dsn
		}
		if strings.Contains(dsn, "?") {
			return dsn + "&" + kv
		}
		return dsn + "?" + kv
	}
	dsn = add(dsn, "parseTime=true")
	dsn = add(dsn, "charset=utf8mb4")
	dsn = add(dsn, "collation=utf8mb4_unicode_ci")
	dsn = add(dsn, "loc=Local")
	dsn = add(dsn, "multiStatements=true")
	return dsn
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	return def
}
