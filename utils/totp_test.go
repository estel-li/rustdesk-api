package utils

import (
	"testing"
)

func TestGenerateRecoveryCodes_Shape(t *testing.T) {
	codes, err := GenerateRecoveryCodes(12, 10)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(codes) != 12 {
		t.Fatalf("expected 12 codes, got %d", len(codes))
	}
	seen := make(map[string]struct{}, len(codes))
	for i, c := range codes {
		if len(c) != 10 {
			t.Fatalf("code %d wrong length %d: %q", i, len(c), c)
		}
		if !IsValidRecoveryCodeShape(c) {
			t.Fatalf("code %d charset violation: %q", i, c)
		}
		if _, dup := seen[c]; dup {
			t.Fatalf("duplicate recovery code generated: %q", c)
		}
		seen[c] = struct{}{}
	}
}

func TestGenerateRecoveryCodes_InvalidParams(t *testing.T) {
	cases := []struct {
		count, length int
	}{
		{0, 10},
		{12, 0},
		{12, -1},
		{12, 100}, // 超过 8 byte 能编码的最大长度(16)
	}
	for _, c := range cases {
		if _, err := GenerateRecoveryCodes(c.count, c.length); err == nil {
			t.Fatalf("expected error for params %+v", c)
		}
	}
}

func TestRecoveryCodeHashRoundtrip(t *testing.T) {
	codes, err := GenerateRecoveryCodes(3, 10)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, code := range codes {
		hash, err := HashRecoveryCode(code)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		matched, err := VerifyRecoveryCode(hash, code)
		if err != nil || !matched {
			t.Fatalf("verify expected true, got matched=%v err=%v", matched, err)
		}
		matched, err = VerifyRecoveryCode(hash, code+"X")
		if err != nil {
			t.Fatalf("verify wrong code returned err: %v", err)
		}
		if matched {
			t.Fatalf("verify wrong code should not match")
		}
		// 大小写敏感:base32 字母表为 A-Z2-7,小写不视作同一 code。
		matchedLower, err := VerifyRecoveryCode(hash, lowerASCII(code))
		if err != nil {
			t.Fatalf("verify lower returned err: %v", err)
		}
		if matchedLower && lowerASCII(code) != code {
			t.Fatalf("verify should be case-sensitive")
		}
	}
}

func TestIsValidRecoveryCodeShape(t *testing.T) {
	good := []string{"AAAAAAAAAA", "ABCD234567", "ZZZZZZ7777"}
	for _, g := range good {
		if !IsValidRecoveryCodeShape(g) {
			t.Fatalf("expected valid: %q", g)
		}
	}
	bad := []string{"", "abcdefghij", "1234567890", "AAAA1AAAAA", "AAAA9AAAAA", "AAAA-AAAAA"}
	for _, b := range bad {
		if IsValidRecoveryCodeShape(b) {
			t.Fatalf("expected invalid: %q", b)
		}
	}
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
