// Package security sifreleme, kimlik dogrulama yardimcilari ve hiz limiti icerir.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Keyring tek bir ana anahtardan amac bazli alt anahtarlar turetir.
type Keyring struct {
	encKey  []byte // AES-256-GCM
	macKey  []byte // HMAC-SHA256
	aead    cipher.AEAD
}

func NewKeyring(master []byte) (*Keyring, error) {
	if len(master) < 32 {
		return nil, errors.New("ana anahtar en az 32 bayt olmali")
	}
	enc := derive(master, "lafed-burs:encrypt:v1")
	mac := derive(master, "lafed-burs:hmac:v1")

	block, err := aes.NewCipher(enc)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Keyring{encKey: enc, macKey: mac, aead: aead}, nil
}

func derive(master []byte, label string) []byte {
	h := hmac.New(sha256.New, master)
	h.Write([]byte(label))
	return h.Sum(nil)
}

// Encrypt duz metni AES-256-GCM ile sifreler; cikti: nonce || ciphertext.
func (k *Keyring) Encrypt(plain string) ([]byte, error) {
	if plain == "" {
		return nil, nil
	}
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return k.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (k *Keyring) Decrypt(blob []byte) (string, error) {
	if len(blob) == 0 {
		return "", nil
	}
	ns := k.aead.NonceSize()
	if len(blob) < ns {
		return "", errors.New("gecersiz sifreli veri")
	}
	out, err := k.aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Fingerprint mukerrer basvuru kontrolu icin deterministik, geri donusu
// olmayan bir etiket uretir (ham TC veritabaninda aranabilir kalmaz).
func (k *Keyring) Fingerprint(value string) string {
	h := hmac.New(sha256.New, k.macKey)
	h.Write([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(h.Sum(nil))
}

// Sign cerez gibi kisa verileri imzalar.
func (k *Keyring) Sign(value string) string {
	h := hmac.New(sha256.New, k.macKey)
	h.Write([]byte(value))
	return value + "." + hex.EncodeToString(h.Sum(nil))[:32]
}

func (k *Keyring) Verify(signed string) (string, bool) {
	i := strings.LastIndex(signed, ".")
	if i < 0 {
		return "", false
	}
	value, sig := signed[:i], signed[i+1:]
	expected := k.Sign(value)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signed)) == 1 && sig != "" {
		return value, true
	}
	return "", false
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// RandomToken n bayt rastgele veriden hex token uretir.
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("rastgele sayi uretilemedi: %v", err))
	}
	return hex.EncodeToString(b)
}

// trackingAlphabet karistirilmasi kolay harf/rakamlari (I, O, 0, 1) disarida birakir.
const trackingAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// TrackingCode ogrenciye verilen, telefonda okunabilir takip kodu uretir:
// LF-XXXX-XXXX
func TrackingCode() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, 0, 13)
	out = append(out, 'L', 'F', '-')
	for i, v := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, trackingAlphabet[int(v)%len(trackingAlphabet)])
	}
	return string(out)
}

// ValidTCKN T.C. kimlik numarasinin bicim ve saglama kurallarini dogrular.
func ValidTCKN(id string) bool {
	id = strings.TrimSpace(id)
	if len(id) != 11 || id[0] == '0' {
		return false
	}
	var d [11]int
	for i := 0; i < 11; i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
		d[i] = int(id[i] - '0')
	}
	odd := d[0] + d[2] + d[4] + d[6] + d[8]
	even := d[1] + d[3] + d[5] + d[7]
	if (odd*7-even)%10 != d[9] {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		sum += d[i]
	}
	return sum%10 == d[10]
}
