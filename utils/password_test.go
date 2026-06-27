package utils

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestVerifyPasswordMD5(t *testing.T) {
	hash := Md5("secret" + "rustdesk-api")
	ok, newHash, err := VerifyPassword(hash, "secret")
	if err != nil {
		t.Fatalf("md5 verify failed: %v", err)
	}
	if !ok || newHash == "" {
		t.Fatalf("md5 migration failed")
	}
	if bcrypt.CompareHashAndPassword([]byte(newHash), []byte("secret")) != nil {
		t.Fatalf("invalid rehash")
	}
}

func TestVerifyPasswordBcrypt(t *testing.T) {
	b, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	ok, newHash, err := VerifyPassword(string(b), "pass")
	if err != nil || !ok || newHash != "" {
		t.Fatalf("bcrypt verify failed")
	}
}

func TestVerifyPasswordMigrate(t *testing.T) {
	md5hash := Md5("mypass" + "rustdesk-api")
	ok, newHash, err := VerifyPassword(md5hash, "mypass")
	if err != nil || !ok || newHash == "" {
		t.Fatalf("expected bcrypt rehash")
	}
	if bcrypt.CompareHashAndPassword([]byte(newHash), []byte("mypass")) != nil {
		t.Fatalf("rehash not valid bcrypt")
	}
}

func TestHashRecoveryCode_VerifyOk(t *testing.T) {
	code := "ABCD-1234"
	hash, err := HashRecoveryCode(code)
	if err != nil {
		t.Fatalf("hash err: %v", err)
	}
	if hash == code || hash == "" {
		t.Fatalf("unexpected hash %q", hash)
	}
	ok, err := VerifyRecoveryCode(hash, code)
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if !ok {
		t.Fatalf("expected match")
	}
}

func TestHashRecoveryCode_VerifyMismatch(t *testing.T) {
	hash, err := HashRecoveryCode("good-code")
	if err != nil {
		t.Fatalf("hash err: %v", err)
	}
	ok, err := VerifyRecoveryCode(hash, "bad-code")
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if ok {
		t.Fatalf("expected mismatch")
	}
}

func TestVerifyRecoveryCode_BadHash(t *testing.T) {
	ok, err := VerifyRecoveryCode("not-a-bcrypt-hash", "x")
	if err == nil {
		t.Fatalf("expected error on malformed hash")
	}
	if ok {
		t.Fatalf("expected matched=false on malformed hash")
	}
}
