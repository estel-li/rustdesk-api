package config

import "time"

// Mfa 用户多因子认证(TOTP)相关配置。
// 留空时由 utils.DeriveMfaKey 走 HKDF(Jwt.Key) 派生 32 byte 加密密钥。
type Mfa struct {
	SecretKey string `mapstructure:"secret-key"` // 32 byte 原始 key(hex/base64);留空则 HKDF(Jwt.Key)
	Issuer    string `mapstructure:"issuer"`     // TOTP otpauth issuer,留空走 service.defaultMfaIssuer("Estel Remote")

	// TicketTTL CE-M1-3 两步登录 ticket 有效期,默认 3 分钟,最大 5 分钟(超出由 service.IssueTicket clamp)。
	TicketTTL time.Duration `mapstructure:"ticket-ttl"`
	// TicketBindIP 是否将 ticket 与首步 client IP 绑定;反向代理 / NAT 场景下可关闭。
	TicketBindIP bool `mapstructure:"ticket-bind-ip"`
	// LoginMfaMaxAttempts 单 ticket 内 /api/login-mfa 错误次数上限,超过即把 ticket 标记为已消费。
	LoginMfaMaxAttempts int `mapstructure:"login-mfa-max-attempts"`

	// EnrollTicketTTL CE-M1-5 强制 MFA 时 /api/login 返回的 enroll-purpose ticket 有效期,默认 5 分钟。
	// 与登录 ticket 区分,允许 ops 单独放宽给用户扫码 + verify 的时间。
	EnrollTicketTTL time.Duration `mapstructure:"enroll-ticket-ttl"`
	// ForceEnrollOnRequired CE-M1-5 当强制策略命中且账号尚未 enroll 时:
	//   true(默认)→ 走 /api/mfa/enroll-then-verify 让用户当场扫码激活;
	//   false        → 直接拒绝登录(返回 MfaEnrollRequired)。
	// 用 *bool 默认 nil 表示按代码默认走 true,nil 与 false 严格区分。
	ForceEnrollOnRequired *bool `mapstructure:"force-enroll-on-required"`
}

// 默认值常量,在 service 层通过 EffectiveTicketTTL / EffectiveMaxAttempts 兜底。
const (
	DefaultMfaTicketTTL       = 3 * time.Minute
	MaxMfaTicketTTL           = 5 * time.Minute
	DefaultMfaLoginMaxTries   = 5
	DefaultMfaEnrollTicketTTL = 5 * time.Minute
	MaxMfaEnrollTicketTTL     = 15 * time.Minute
)
