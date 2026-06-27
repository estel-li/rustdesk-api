package utils

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
)

// totp.go 提供 MFA recovery code 的生成 / 字符集校验工具。
// bcrypt hash / verify 直接复用 utils/password.go 中的 HashRecoveryCode、VerifyRecoveryCode,
// 避免出现两套同名 API。

// 默认 recovery code 形状:12 条 × 10 字符,RFC4648 base32 字母表(A-Z2-7),无 padding。
// CE-M1-2 任务卡 §4.5 要求,manual 抄写时排除歧义字符。
const (
	DefaultRecoveryCodeCount  = 12
	DefaultRecoveryCodeLength = 10
)

// ErrRecoveryCodeParams 入参不合法(count<=0 / length<=0 / length>16)。
var ErrRecoveryCodeParams = errors.New("invalid recovery code params")

// 8 byte 熵足以编码 16 个 base32 字符,这是单条 code 的最大长度上限。
const recoveryRawBytes = 8

// GenerateRecoveryCodes 生成 count 条 base32 recovery code,每条恰好 length 个字符。
// 1. crypto/rand 读取 8 byte 随机熵;
// 2. base32(NoPadding) 编码为 16 字符;
// 3. 截断到前 length 字符。
//
// 严禁回退到 math/rand:crypto/rand 失败时直接返回 error,由上层决定如何处理(参见任务卡 §8)。
func GenerateRecoveryCodes(count, length int) ([]string, error) {
	if count <= 0 || length <= 0 || length > recoveryRawBytes*8/5 {
		return nil, ErrRecoveryCodeParams
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		buf := make([]byte, recoveryRawBytes)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		encoded := enc.EncodeToString(buf)
		if len(encoded) < length {
			// 理论上 8 byte → 13 字符(NoPadding),不会落到这里;保底防御。
			return nil, errors.New("recovery code encoding too short")
		}
		out = append(out, encoded[:length])
	}
	return out, nil
}

// IsValidRecoveryCodeShape 判断 code 是否落在 RFC4648 base32 字母表内(A-Z2-7),用于测试 / 输入清洗。
// 长度交给调用方校验;空字符串视为非法。
func IsValidRecoveryCodeShape(code string) bool {
	if code == "" {
		return false
	}
	for _, r := range code {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '2' && r <= '7':
		default:
			return false
		}
	}
	return true
}
