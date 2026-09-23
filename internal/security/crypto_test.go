package security

import (
	"strings"
	"testing"
	"time"
)

func testKeyring(t *testing.T) *Keyring {
	t.Helper()
	k, err := NewKeyring([]byte("bu-anahtar-en-az-otuziki-bayt-uzunlugundadir"))
	if err != nil {
		t.Fatalf("anahtar olusturulamadi: %v", err)
	}
	return k
}

func TestEncryptDecrypt(t *testing.T) {
	k := testKeyring(t)
	const tc = "12345678950"
	blob, err := k.Encrypt(tc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), tc) {
		t.Error("sifreli metin duz veriyi icermemeli")
	}
	got, err := k.Decrypt(blob)
	if err != nil || got != tc {
		t.Fatalf("cozme hatali: %q %v", got, err)
	}
	if b, _ := k.Encrypt(""); b != nil {
		t.Error("bos deger icin nil beklenir")
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	k := testKeyring(t)
	a := k.Fingerprint("10000000146")
	b := k.Fingerprint(" 10000000146 ")
	if a != b {
		t.Error("bosluklar temizlenmeli")
	}
	if a == k.Fingerprint("10000000147") {
		t.Error("farkli girdiler ayni parmak izini vermemeli")
	}
	if len(a) != 64 {
		t.Errorf("beklenen uzunluk 64, gelen %d", len(a))
	}
}

func TestSignVerify(t *testing.T) {
	k := testKeyring(t)
	signed := k.Sign("deger")
	if v, ok := k.Verify(signed); !ok || v != "deger" {
		t.Errorf("imza dogrulanamadi: %q %v", v, ok)
	}
	if _, ok := k.Verify(signed + "x"); ok {
		t.Error("bozuk imza kabul edildi")
	}
	if _, ok := k.Verify("imzasiz"); ok {
		t.Error("imzasiz deger kabul edildi")
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("cok-gizli-sifre")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "cok-gizli-sifre") {
		t.Error("dogru sifre reddedildi")
	}
	if CheckPassword(hash, "yanlis") {
		t.Error("yanlis sifre kabul edildi")
	}
}

func TestValidTCKN(t *testing.T) {
	valid := []string{"10000000146", "11111111110"}
	for _, v := range valid {
		if !ValidTCKN(v) {
			t.Errorf("%s gecerli sayilmaliydi", v)
		}
	}
	invalid := []string{"", "1234567890", "012345678900", "abcdefghijk", "12345678901", "00000000000"}
	for _, v := range invalid {
		if ValidTCKN(v) {
			t.Errorf("%s gecersiz sayilmaliydi", v)
		}
	}
}

func TestTrackingCodeFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		c := TrackingCode()
		if len(c) != 12 || !strings.HasPrefix(c, "LF-") || c[7] != '-' {
			t.Fatalf("bicim hatali: %q", c)
		}
		if strings.ContainsAny(c[3:], "IO01") {
			t.Fatalf("karistirilabilir karakter icermemeli: %q", c)
		}
		if seen[c] {
			t.Fatalf("tekrar eden kod: %q", c)
		}
		seen[c] = true
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("%d. istek izinli olmaliydi", i+1)
		}
	}
	if l.Allow("ip") {
		t.Error("sinir asildiginda reddedilmeliydi")
	}
	if !l.Allow("baska-ip") {
		t.Error("farkli anahtar etkilenmemeli")
	}
}
