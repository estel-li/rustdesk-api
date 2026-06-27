package utils

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
)

// EncryptPassword hashes the input password using bcrypt.
// An error is returned if hashing fails.
func EncryptPassword(password string) (string, error) {
	bs, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

// VerifyPassword checks the input password against the stored hash.
// When a legacy MD5 hash is provided, the password is rehashed with bcrypt
// and the new hash is returned. Any internal bcrypt error is returned.
func VerifyPassword(hash, input string) (bool, string, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(input))
	if err == nil {
		return true, "", nil
	}

	var invalidPrefixErr bcrypt.InvalidHashPrefixError
	if errors.As(err, &invalidPrefixErr) || errors.Is(err, bcrypt.ErrHashTooShort) {
		// Try fallback to legacy MD5 hash verification
		if hash == Md5(input+"rustdesk-api") {
			newHash, err2 := bcrypt.GenerateFromPassword([]byte(input), bcrypt.DefaultCost)
			if err2 != nil {
				return true, "", err2
			}
			return true, string(newHash), nil
		}
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, "", nil
	}
	return false, "", err
}

// HashRecoveryCode hashes a MFA recovery code with bcrypt.
// Recovery code 一律走 bcrypt,落库后不可逆;明文仅在 enroll 接口一次性返回前端。
func HashRecoveryCode(code string) (string, error) {
	bs, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

// VerifyRecoveryCode 校验输入是否匹配 bcrypt(hash);不匹配 matched=false 且 err=nil。
// 内部错误(非 mismatch)以 err 返回,便于上层落审计日志。
func VerifyRecoveryCode(hash, input string) (matched bool, err error) {
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(input))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	return false, err
}
