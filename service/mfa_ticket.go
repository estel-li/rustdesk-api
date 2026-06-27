package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// MfaTicketService 负责 CE-M1-3 两步登录中的短期 ticket 签发 / 校验 / 消费。
//
// 设计要点:
//   - ticket 是短期 JWT(HS256,默认 3 分钟,最大 5 分钟),签名密钥复用 global.Jwt.Key,
//     不引入独立密钥,以便 ops 旋转 jwt.key 即可让所有未消费 ticket 失效。
//   - claims 携带 uid / ip / device / jti,jti 落 global.Cache 用作 nonce,实现抗重放。
//   - cache.Handler 没有 Delete,因此 Consume 用 Set 覆盖 + 保留 TTL 的方式标记 consumed=true,
//     过期由 cache TTL 自然 GC。Verify 见到 consumed=true 即视为重放失败。
//   - 由调用方在 Verify 失败时记入 LoginLimiter(参考 controller/api/login.go 的现有用法)。
//
// 错误哨兵命名对齐 service/mfa.go;不暴露内部 jwt 细节给上层。
var (
	ErrTicketInvalid      = errors.New("MfaTicketInvalid")
	ErrTicketExpired      = errors.New("MfaTicketExpired")
	ErrTicketConsumed     = errors.New("MfaTicketConsumed")
	ErrTicketIPMismatch   = errors.New("MfaTicketIPMismatch")
	ErrTicketNonceMissing = errors.New("MfaTicketNonceMissing")
)

// MfaTicketIssuer JWT iss claim,固定字符串,便于审计区分 access_token 与 ticket。
const MfaTicketIssuer = "rustdesk-api/mfa"

// nonceKeyPrefix / attemptKeyPrefix 在 global.Cache 中划出独立命名空间,避免与业务 key 撞库。
const (
	nonceKeyPrefix   = "mfa:ticket:nonce:"
	attemptKeyPrefix = "mfa:ticket:fail:"
)

// MfaTicketService 不持有句柄,所有依赖通过 global 读取。
type MfaTicketService struct{}

// MfaTicketClaims 短 JWT 自定义 claims;UID 用于二次校验时找用户,IP 用于绑定首步 client IP,
// Device 来源首步 LoginForm.Id,允许为空(老版本客户端可能不带 id)。
// Purpose CE-M1-5:""/"login" 走 /api/login-mfa,"enroll" 走 /api/mfa/enroll-then-verify。
type MfaTicketClaims struct {
	UID     uint   `json:"uid"`
	IP      string `json:"ip"`
	Device  string `json:"dev,omitempty"`
	JTI     string `json:"jti"`
	Purpose string `json:"prp,omitempty"`
	jwt.RegisteredClaims
}

// ticketNonce 是 cache 中存放的 nonce 值;Consumed=true 表示该 ticket 已被成功消费,后续提交即为重放。
type ticketNonce struct {
	Consumed bool  `json:"consumed"`
	UID      uint  `json:"uid"`
	Exp      int64 `json:"exp"`
}

// EffectiveTicketTTL 读 Config.Mfa.TicketTTL,缺省回退 DefaultMfaTicketTTL,超过 MaxMfaTicketTTL 时 clamp。
func (s *MfaTicketService) EffectiveTicketTTL() time.Duration {
	ttl := config.DefaultMfaTicketTTL
	if Config != nil && Config.Mfa.TicketTTL > 0 {
		ttl = Config.Mfa.TicketTTL
	}
	if ttl > config.MaxMfaTicketTTL {
		ttl = config.MaxMfaTicketTTL
	}
	return ttl
}

// EffectiveMaxAttempts 单 ticket 最大错误次数;<=0 时回退默认值 5。
func (s *MfaTicketService) EffectiveMaxAttempts() int {
	if Config != nil && Config.Mfa.LoginMfaMaxAttempts > 0 {
		return Config.Mfa.LoginMfaMaxAttempts
	}
	return config.DefaultMfaLoginMaxTries
}

// bindIP 受配置 Mfa.TicketBindIP 控制,默认 true。
func (s *MfaTicketService) bindIP() bool {
	if Config == nil {
		return true
	}
	return Config.Mfa.TicketBindIP
}

// signingKey 复用 global.Jwt.Key;在测试场景下允许 fallback 到 Jwt.Key(service 包内全局)。
func (s *MfaTicketService) signingKey() ([]byte, error) {
	if global.Jwt != nil && len(global.Jwt.Key) > 0 {
		return global.Jwt.Key, nil
	}
	if Jwt != nil && len(Jwt.Key) > 0 {
		return Jwt.Key, nil
	}
	return nil, errors.New("MfaTicketSigningKeyMissing")
}

// cache 读 global.Cache;在单测早期未初始化时返回 nil,IssueTicket / Verify / Consume / IncAttempt
// 会退化为"无重放保护",生产部署必须保证 cache 可用(memory / redis 任一)。
func (s *MfaTicketService) cacheReady() bool {
	return global.Cache != nil
}

// Issue 为指定 uid 签发短期 ticket(默认 login purpose,等价于 IssueWithPurpose(uid, ip, device, "login"))。
func (s *MfaTicketService) Issue(uid uint, ip, device string) (string, string, error) {
	return s.IssueWithPurpose(uid, ip, device, "login", s.EffectiveTicketTTL())
}

// IssueWithPurpose CE-M1-5 增加 purpose 维度;ttl<=0 时回退 EffectiveTicketTTL。
// purpose="enroll" 用于 /api/mfa/enroll-then-verify;其它走 /api/login-mfa。
func (s *MfaTicketService) IssueWithPurpose(uid uint, ip, device, purpose string, ttl time.Duration) (string, string, error) {
	key, err := s.signingKey()
	if err != nil {
		return "", "", err
	}
	if ttl <= 0 {
		ttl = s.EffectiveTicketTTL()
	}
	jti := uuid.NewString()
	now := time.Now()
	exp := now.Add(ttl)

	claims := MfaTicketClaims{
		UID:     uid,
		IP:      ip,
		Device:  device,
		JTI:     jti,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    MfaTicketIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        jti,
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
	if err != nil {
		return "", "", err
	}
	if global.Cache != nil {
		nonce := ticketNonce{Consumed: false, UID: uid, Exp: exp.Unix()}
		_ = global.Cache.Set(nonceKeyPrefix+jti, nonce, int(ttl.Seconds()))
	}
	return token, jti, nil
}

// EffectiveEnrollTicketTTL CE-M1-5 读取 enroll-purpose ticket 的有效期,
// 与登录 ticket 区分:默认 5 分钟,最大 15 分钟(留更长扫码 + verify 窗口)。
func (s *MfaTicketService) EffectiveEnrollTicketTTL() time.Duration {
	ttl := config.DefaultMfaEnrollTicketTTL
	if Config != nil && Config.Mfa.EnrollTicketTTL > 0 {
		ttl = Config.Mfa.EnrollTicketTTL
	}
	if ttl > config.MaxMfaEnrollTicketTTL {
		ttl = config.MaxMfaEnrollTicketTTL
	}
	return ttl
}

// Verify 校验 ticket:解析签名 → 比对过期 / 签发者 → 比对 IP(若绑定)→ 检查 nonce 状态。
//
// 返回 (claims, nil) 表示通过基础校验,调用方可继续按 type 走 TOTP / recovery 流程;
// 失败按 §6 列出的 error 哨兵返回,便于上层 TranslateMsg。
func (s *MfaTicketService) Verify(token, ip string) (*MfaTicketClaims, error) {
	key, err := s.signingKey()
	if err != nil {
		return nil, ErrTicketInvalid
	}
	claims := &MfaTicketClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTicketInvalid
		}
		return key, nil
	}, jwt.WithIssuer(MfaTicketIssuer), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTicketExpired
		}
		return nil, ErrTicketInvalid
	}
	if !parsed.Valid {
		return nil, ErrTicketInvalid
	}
	if claims.JTI == "" {
		return nil, ErrTicketInvalid
	}
	if s.bindIP() && claims.IP != "" && claims.IP != ip {
		return nil, ErrTicketIPMismatch
	}
	// nonce 校验:cache 未初始化(单测早期场景)时跳过,但生产路径强制要求。
	if global.Cache != nil {
		nonce := ticketNonce{}
		_ = global.Cache.Get(nonceKeyPrefix+claims.JTI, &nonce)
		if nonce.UID == 0 && !nonce.Consumed {
			// 既无 uid 又非已消费 → 视为 nonce 不存在(被 TTL 清理或从未签发)。
			return nil, ErrTicketNonceMissing
		}
		if nonce.Consumed {
			return nil, ErrTicketConsumed
		}
	}
	return claims, nil
}

// Consume 把 nonce 标记为已消费,后续重放即被 Verify 拦截。
// remainingTTL 由 ticket exp 推算,保证 cache 条目随 ticket 一并过期,避免占用持久空间。
func (s *MfaTicketService) Consume(claims *MfaTicketClaims) {
	if claims == nil || global.Cache == nil {
		return
	}
	ttl := int(time.Until(claims.ExpiresAt.Time).Seconds())
	if ttl <= 0 {
		ttl = 1
	}
	nonce := ticketNonce{Consumed: true, UID: claims.UID, Exp: claims.ExpiresAt.Unix()}
	_ = global.Cache.Set(nonceKeyPrefix+claims.JTI, nonce, ttl)
}

// IncAttempt 对单 ticket 累计错误次数;超阈值返回 exceed=true,调用方应立即 Consume。
// 使用与 ticket 同 TTL 的独立 key,避免污染 nonce 结构。
func (s *MfaTicketService) IncAttempt(claims *MfaTicketClaims) (current int, exceed bool) {
	if claims == nil || global.Cache == nil {
		return 0, false
	}
	key := attemptKeyPrefix + claims.JTI
	cnt := 0
	_ = global.Cache.Get(key, &cnt)
	cnt++
	ttl := int(time.Until(claims.ExpiresAt.Time).Seconds())
	if ttl <= 0 {
		ttl = 1
	}
	_ = global.Cache.Set(key, cnt, ttl)
	return cnt, cnt >= s.EffectiveMaxAttempts()
}
