package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

// secret_cipher.go 提供 MFA secret 等敏感字段的对称加解密辅助。
// 项目原本没有对称加密工具(grep crypto/aes 无命中),本文件为 CE-M1-1 新增。

const (
	// MfaKeySize AES-256-GCM 要求 32 byte 主密钥。
	MfaKeySize = 32
	// mfaHkdfInfo HKDF info 字符串,保证 key 派生上下文唯一。
	mfaHkdfInfo = "rustdesk-mfa-secret"
)

// ErrCipherKeySize raw key 长度不匹配。
var ErrCipherKeySize = errors.New("mfa cipher key must be 32 bytes")

// ErrCipherEmptyKey jwt key 与 raw key 同时为空,无法派生密钥。
var ErrCipherEmptyKey = errors.New("mfa cipher key is empty: both mfa.secret-key and jwt.key are blank")

// EncryptSecret 用 AES-256-GCM 加密 plaintext;key 必须为 32 byte。
// 返回 base64(nonce||ciphertext||tag),适合塞进 varchar 列。
func EncryptSecret(key []byte, plaintext string) (string, error) {
	if len(key) != MfaKeySize {
		return "", ErrCipherKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptSecret 与 EncryptSecret 对偶;密文被篡改或 key 不匹配返回 error。
func DecryptSecret(key []byte, ciphertext string) (string, error) {
	if len(key) != MfaKeySize {
		return "", ErrCipherKeySize
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("mfa cipher: ciphertext too short")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DeriveMfaKey 解析配置项得到 32 byte 主密钥:
//  1. rawKey 优先,尝试以 hex / base64 解析,长度为 32 直接返回;
//  2. 失败或 rawKey 为空时,以 jwtKey 作为输入材料,HKDF-SHA256 派生 32 byte 输出;
//  3. 当 rawKey 与 jwtKey 都为空时返回 ErrCipherEmptyKey。
func DeriveMfaKey(rawKey, jwtKey string) ([]byte, error) {
	if rawKey != "" {
		if k, ok := tryParseFixedKey(rawKey); ok {
			return k, nil
		}
		// 解析失败时退回到 HKDF 模式,把 rawKey 作为派生输入。
		return hkdfExtractExpand([]byte(rawKey)), nil
	}
	if jwtKey == "" {
		return nil, ErrCipherEmptyKey
	}
	return hkdfExtractExpand([]byte(jwtKey)), nil
}

// tryParseFixedKey 仅当 raw key 为 hex(64 字符)或 base64(精确 32 byte)时返回。
func tryParseFixedKey(rawKey string) ([]byte, bool) {
	if b, err := hex.DecodeString(rawKey); err == nil && len(b) == MfaKeySize {
		return b, true
	}
	if b, err := base64.StdEncoding.DecodeString(rawKey); err == nil && len(b) == MfaKeySize {
		return b, true
	}
	if b, err := base64.RawStdEncoding.DecodeString(rawKey); err == nil && len(b) == MfaKeySize {
		return b, true
	}
	return nil, false
}

// hkdfExtractExpand 用固定 salt + info 做 HKDF,保证相同输入永远得到相同 key。
func hkdfExtractExpand(ikm []byte) []byte {
	// 固定 salt 维持派生确定性;不同 info 隔离不同用途的 key。
	salt := []byte("rustdesk-api-mfa-salt-v1")
	r := hkdf.New(sha256.New, ikm, salt, []byte(mfaHkdfInfo))
	out := make([]byte, MfaKeySize)
	_, _ = io.ReadFull(r, out)
	return out
}
