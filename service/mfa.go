package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"strconv"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
	"github.com/lejianwen/rustdesk-api/v2/utils"
)

// MfaService 围绕 RFC6238 TOTP 提供 enroll / verify / recovery-code 全套流程,
// 是 CE-M1-3 两步登录状态机与 CE-M1-5 强制 MFA 的底层依赖。
//
// 约定:
//   - secret 落库前由 utils.EncryptSecret 加密,key 经 utils.DeriveMfaKey 派生(优先 Config.Mfa.SecretKey,
//     缺省回退 HKDF(Config.Jwt.Key))。
//   - recovery code 落库只存 bcrypt(hash),明文仅在 GenerateRecoveryCodes 返回值中出现,
//     调用方必须只展示一次;严禁写入日志。
//   - 所有方法在 user_mfas 表缺失时优雅退化为 ErrMfaNotEnrolled,不 panic(参见 CE-M1-2 §8)。
//   - Verify 错误次数请由上层 limiter(参考 utils.LoginLimiter)计入。
//
// 错误哨兵命名风格对齐 service/ldap.go。
var (
	ErrMfaNotEnrolled     = errors.New("MfaNotEnrolled")
	ErrMfaAlreadyEnrolled = errors.New("MfaAlreadyEnrolled")
	ErrMfaInvalid         = errors.New("MfaInvalid")
	ErrMfaRecoveryUsed    = errors.New("MfaRecoveryUsed")
	ErrMfaUserNotFound    = errors.New("MfaUserNotFound")
)

// MfaService 不持有自己的句柄,直接复用全局 DB / Logger / Config / Lock。
type MfaService struct{}

// defaultRecoveryIssuer 在 Config.Mfa.Issuer / Config.Admin.Title 全部为空时使用。
const defaultMfaIssuer = "RustDesk API"

// totpPeriodSeconds = 30s,Digits=6,Algorithm=SHA1,Skew=±1 step。
// 保留为常量便于测试 helper 复用。
const (
	totpPeriodSeconds = 30
	totpSecretSize    = 20 // 160-bit,RFC6238 推荐
	totpSkew          = 1
)

// tableMissing 在 user_mfas 表不存在时返回 true。
// 防御性退化,避免在 CE-M1-1 迁移未完成的环境中 panic;调用方拿到 true 即返回 ErrMfaNotEnrolled。
func (s *MfaService) tableMissing() bool {
	if DB == nil {
		return true
	}
	mig := DB.Migrator()
	if mig == nil {
		return true
	}
	return !mig.HasTable(&model.UserMfa{})
}

// loadByUser 按 user_id 拉取记录;ErrRecordNotFound 时 (nil, nil)。
func (s *MfaService) loadByUser(userId uint) (*model.UserMfa, error) {
	row := &model.UserMfa{}
	err := DB.Where("user_id = ?", userId).First(row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return row, nil
}

// issuer 取 Config.Mfa.Issuer,空回退 Config.Admin.Title,再空回退 defaultMfaIssuer。
func (s *MfaService) issuer() string {
	if Config != nil {
		if v := Config.Mfa.Issuer; v != "" {
			return v
		}
		if v := Config.Admin.Title; v != "" {
			return v
		}
	}
	return defaultMfaIssuer
}

// accountName 优先用 user.Email,缺省回退 user.Username。
// Authenticator app 在 issuer:account 形式下展示账号,空字符串会被某些客户端拒绝,务必兜底。
func (s *MfaService) accountName(u *model.User) string {
	if u.Email != "" {
		return u.Email
	}
	if u.Username != "" {
		return u.Username
	}
	return strconv.FormatUint(uint64(u.Id), 10)
}

// mfaKey 用配置派生 32 byte 主密钥,供 EncryptSecret / DecryptSecret 使用。
func (s *MfaService) mfaKey() ([]byte, error) {
	if Config == nil {
		return nil, utils.ErrCipherEmptyKey
	}
	return utils.DeriveMfaKey(Config.Mfa.SecretKey, Config.Jwt.Key)
}

// Enroll 生成 TOTP secret 并返回 otpauth:// URL 渲染出的 PNG 二维码。
//
// 行为:
//   - 用户不存在返回 ErrMfaUserNotFound。
//   - 已激活(enabled_at 非空)返回 ErrMfaAlreadyEnrolled。
//   - 既有 pending 记录会被覆盖(允许重新扫码),旧 recovery_codes 一并清空。
//   - 仅落库 secret + 状态 pending(enabled_at 为 nil),需要后续调用 Verify 完成激活。
//
// 安全:
//   - secret 通过 utils.EncryptSecret 落库,日志中绝不输出明文。
//   - QR PNG 包含 secret,HTTP 层必须设置 Cache-Control: no-store。
//
// 并发:通过 Lock.Lock("mfa:enroll:<userId>") 串行化,避免并发 enroll 产生双条记录。
func (s *MfaService) Enroll(userId uint) (secret string, qrPNG []byte, err error) {
	if s.tableMissing() {
		return "", nil, ErrMfaNotEnrolled
	}
	lockKey := "mfa:enroll:" + strconv.FormatUint(uint64(userId), 10)
	Lock.Lock(lockKey)
	defer Lock.UnLock(lockKey)

	user := (&UserService{}).InfoById(userId)
	if user == nil || user.Id == 0 {
		return "", nil, ErrMfaUserNotFound
	}

	existing, err := s.loadByUser(userId)
	if err != nil {
		return "", nil, err
	}
	if existing != nil && existing.EnabledAt != nil && *existing.EnabledAt > 0 {
		return "", nil, ErrMfaAlreadyEnrolled
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer(),
		AccountName: s.accountName(user),
		Period:      totpPeriodSeconds,
		SecretSize:  totpSecretSize,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", nil, err
	}
	secret = key.Secret()

	img, err := key.Image(256, 256)
	if err != nil {
		return "", nil, err
	}
	var buf bytes.Buffer
	if err = png.Encode(&buf, img); err != nil {
		return "", nil, err
	}
	qrPNG = buf.Bytes()

	cipherKey, err := s.mfaKey()
	if err != nil {
		return "", nil, err
	}
	encrypted, err := utils.EncryptSecret(cipherKey, secret)
	if err != nil {
		return "", nil, err
	}

	emptyJSON := custom_types.AutoJson(json.RawMessage(`[]`))
	if existing == nil {
		row := &model.UserMfa{
			UserId:        userId,
			Secret:        encrypted,
			RecoveryCodes: emptyJSON,
		}
		if err = DB.Create(row).Error; err != nil {
			return "", nil, err
		}
	} else {
		existing.Secret = encrypted
		existing.RecoveryCodes = emptyJSON
		existing.EnabledAt = nil
		existing.LastUsedAt = nil
		if err = DB.Save(existing).Error; err != nil {
			return "", nil, err
		}
	}

	return secret, qrPNG, nil
}

// Verify 校验 TOTP code(RFC6238, 6 位, period=30s, skew=±1 step)。
//
// 返回语义:
//   - (true,  nil) 校验成功;若记录处于 pending(enabled_at 为 nil)则同时激活并写入 enabled_at;
//     不论新旧,均会更新 last_used_at。
//   - (false, nil) code 不匹配。
//   - (false, ErrMfaNotEnrolled) 用户没有 user_mfa 记录(或表缺失)。
//
// 调用方应把 (false, nil) 计入 limiter(参考 service/user.go 中 Login 流程的 utils.LoginLimiter)。
func (s *MfaService) Verify(userId uint, code string) (bool, error) {
	if s.tableMissing() {
		return false, ErrMfaNotEnrolled
	}
	row, err := s.loadByUser(userId)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, ErrMfaNotEnrolled
	}

	cipherKey, err := s.mfaKey()
	if err != nil {
		return false, err
	}
	plainSecret, err := utils.DecryptSecret(cipherKey, row.Secret)
	if err != nil {
		return false, err
	}

	ok, err := totp.ValidateCustom(code, plainSecret, time.Now().UTC(), totp.ValidateOpts{
		Period:    totpPeriodSeconds,
		Skew:      totpSkew,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		// otp 包对长度错误等输入异常会返回 error;视为校验失败,不暴露细节。
		return false, nil
	}
	if !ok {
		return false, nil
	}

	now := time.Now().Unix()
	updates := map[string]interface{}{
		"last_used_at": now,
	}
	if row.EnabledAt == nil || *row.EnabledAt == 0 {
		updates["enabled_at"] = now
	}
	if err = DB.Model(&model.UserMfa{}).Where("user_id = ?", userId).Updates(updates).Error; err != nil {
		return false, err
	}
	return true, nil
}

// GenerateRecoveryCodes 生成 12 条 10 字符 base32 recovery code(无填充),
// 将 bcrypt(code) 数组以 JSON 写入 user_mfa.recovery_codes(覆盖旧值)。
//
// 仅在用户已激活(IsEnrolled == true)时允许调用;未激活返回 ErrMfaNotEnrolled。
// 返回明文 codes 切片,调用方负责仅向用户展示一次,严禁回写库或写入日志。
func (s *MfaService) GenerateRecoveryCodes(userId uint) ([]string, error) {
	if s.tableMissing() {
		return nil, ErrMfaNotEnrolled
	}
	lockKey := "mfa:recovery:" + strconv.FormatUint(uint64(userId), 10)
	Lock.Lock(lockKey)
	defer Lock.UnLock(lockKey)

	row, err := s.loadByUser(userId)
	if err != nil {
		return nil, err
	}
	if row == nil || row.EnabledAt == nil || *row.EnabledAt == 0 {
		return nil, ErrMfaNotEnrolled
	}

	codes, err := utils.GenerateRecoveryCodes(utils.DefaultRecoveryCodeCount, utils.DefaultRecoveryCodeLength)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(codes))
	for _, c := range codes {
		h, err := utils.HashRecoveryCode(c)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, h)
	}
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return nil, err
	}
	if err = DB.Model(&model.UserMfa{}).Where("user_id = ?", userId).
		Update("recovery_codes", custom_types.AutoJson(encoded)).Error; err != nil {
		return nil, err
	}
	return codes, nil
}

// ConsumeRecoveryCode 比对 code 与已存 hash;命中则把对应 hash 从数组中移除并落库,返回 (true, nil)。
//
// 返回语义:
//   - (true,  nil) 命中并消费成功;同时更新 last_used_at。
//   - (false, ErrMfaRecoveryUsed) code 不在 hash 列表中(被消费过或从未发放过)。
//   - (false, ErrMfaNotEnrolled)  用户未 enroll 或未激活,或 user_mfas 表缺失。
//
// 通过 Lock.Lock("mfa:recovery:<userId>") 串行化,防止并发消费同一 recovery code。
func (s *MfaService) ConsumeRecoveryCode(userId uint, code string) (bool, error) {
	if s.tableMissing() {
		return false, ErrMfaNotEnrolled
	}
	lockKey := "mfa:recovery:" + strconv.FormatUint(uint64(userId), 10)
	Lock.Lock(lockKey)
	defer Lock.UnLock(lockKey)

	row, err := s.loadByUser(userId)
	if err != nil {
		return false, err
	}
	if row == nil || row.EnabledAt == nil || *row.EnabledAt == 0 {
		return false, ErrMfaNotEnrolled
	}

	var hashes []string
	raw := row.RecoveryCodes.String()
	if raw != "" && raw != "null" {
		if err = json.Unmarshal([]byte(raw), &hashes); err != nil {
			return false, err
		}
	}

	matched := -1
	for i, h := range hashes {
		ok, vErr := utils.VerifyRecoveryCode(h, code)
		if vErr != nil {
			// bcrypt 内部异常(hash 损坏等)交由日志,但不要把 code/hash 明文写进去。
			Logger.Warnf("mfa recovery verify error user=%d idx=%d: %v", userId, i, vErr)
			continue
		}
		if ok {
			matched = i
			break
		}
	}
	if matched < 0 {
		return false, ErrMfaRecoveryUsed
	}

	hashes = append(hashes[:matched], hashes[matched+1:]...)
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return false, err
	}
	now := time.Now().Unix()
	if err = DB.Model(&model.UserMfa{}).Where("user_id = ?", userId).Updates(map[string]interface{}{
		"recovery_codes": custom_types.AutoJson(encoded),
		"last_used_at":   now,
	}).Error; err != nil {
		return false, err
	}
	return true, nil
}

// Disable 物理删除 user_mfa 记录;无对应记录返回 ErrMfaNotEnrolled。
// 调用方应在 CE-M1-6 落审计日志(disable 是高敏操作)。
func (s *MfaService) Disable(userId uint) error {
	if s.tableMissing() {
		return ErrMfaNotEnrolled
	}
	res := DB.Where("user_id = ?", userId).Delete(&model.UserMfa{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMfaNotEnrolled
	}
	return nil
}

// IsEnrolled 仅当用户存在 user_mfa 记录且 enabled_at 非空且 > 0 时返回 true。
// 表缺失或查询失败均视为未启用,不抛错。
func (s *MfaService) IsEnrolled(userId uint) bool {
	if s.tableMissing() {
		return false
	}
	row := &model.UserMfa{}
	err := DB.Select("enabled_at").Where("user_id = ?", userId).First(row).Error
	if err != nil {
		return false
	}
	return row.EnabledAt != nil && *row.EnabledAt > 0
}
