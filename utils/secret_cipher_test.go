package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptSecret_RoundTrip(t *testing.T) {
	key := make([]byte, MfaKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	plain := "JBSWY3DPEHPK3PXP"
	ct, err := EncryptSecret(key, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == "" || ct == plain {
		t.Fatalf("ciphertext invalid")
	}
	out, err := DecryptSecret(key, ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out != plain {
		t.Fatalf("roundtrip mismatch: got %q want %q", out, plain)
	}
}

func TestEncryptSecret_KeySizeMismatch(t *testing.T) {
	if _, err := EncryptSecret([]byte("short"), "x"); err == nil {
		t.Fatalf("expected key size error on encrypt")
	}
	if _, err := DecryptSecret([]byte("short"), "AAAA"); err == nil {
		t.Fatalf("expected key size error on decrypt")
	}
}

func TestDecryptSecret_Tampered(t *testing.T) {
	key := make([]byte, MfaKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	ct, err := EncryptSecret(key, "hello world")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	// 翻转最后一个字节(GCM tag 部分),保证篡改可被检测。
	raw[len(raw)-1] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := DecryptSecret(key, tampered); err == nil {
		t.Fatalf("expected error on tampered ciphertext")
	}
}

func TestDeriveMfaKey_FallbackToJwt(t *testing.T) {
	k1, err := DeriveMfaKey("", "hello")
	if err != nil {
		t.Fatalf("derive 1: %v", err)
	}
	if len(k1) != MfaKeySize {
		t.Fatalf("expected %d byte key, got %d", MfaKeySize, len(k1))
	}
	k2, err := DeriveMfaKey("", "hello")
	if err != nil {
		t.Fatalf("derive 2: %v", err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatalf("derivation not deterministic")
	}

	// 不同 jwtKey 必须得到不同的 key。
	k3, err := DeriveMfaKey("", "world")
	if err != nil {
		t.Fatalf("derive 3: %v", err)
	}
	if bytes.Equal(k1, k3) {
		t.Fatalf("different jwt keys must yield different mfa keys")
	}
}

func TestDeriveMfaKey_EmptyAllFails(t *testing.T) {
	if _, err := DeriveMfaKey("", ""); err == nil {
		t.Fatalf("expected error when both keys empty")
	}
}

func TestDeriveMfaKey_RawHex(t *testing.T) {
	// 64 个 hex 字符 = 32 byte key
	rawHex := strings.Repeat("ab", MfaKeySize)
	k, err := DeriveMfaKey(rawHex, "")
	if err != nil {
		t.Fatalf("derive hex: %v", err)
	}
	if len(k) != MfaKeySize {
		t.Fatalf("expected %d byte key, got %d", MfaKeySize, len(k))
	}
	// 与 HKDF 派生不同(直接 raw key)
	hkdfKey, err := DeriveMfaKey("", "ab")
	if err != nil {
		t.Fatalf("derive jwt: %v", err)
	}
	if bytes.Equal(k, hkdfKey) {
		t.Fatalf("raw hex key collided with hkdf key")
	}
}
