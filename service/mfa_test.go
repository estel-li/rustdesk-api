package service

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/lib/lock"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

// setupMfaTest 用纯内存 sqlite + 默认 logger / lock + 最小 config 初始化全局服务。
// 每个测试自取独立 DB,Cleanup 关掉句柄。
func setupMfaTest(t *testing.T) (*MfaService, *model.User) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:memdb_mfa_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlDB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.User{}, &model.UserMfa{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	cfg := &config.Config{}
	cfg.Admin.Title = "RustDeskTest"
	cfg.Jwt.Key = "test-jwt-key-for-mfa-derivation-1234567890"

	DB = db
	Config = cfg
	Logger = logrus.New()
	Logger.SetLevel(logrus.PanicLevel)
	Lock = lock.NewLocal()

	user := &model.User{Username: "alice", Email: "alice@example.com"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return &MfaService{}, user
}

// generateCurrentCode 用任务卡同款参数从明文 secret 生成当前 TOTP code。
func generateCurrentCode(t *testing.T, secret string, ts time.Time) string {
	t.Helper()
	code, err := totp.GenerateCodeCustom(secret, ts, totp.ValidateOpts{
		Period:    totpPeriodSeconds,
		Skew:      totpSkew,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("totp generate: %v", err)
	}
	return code
}

func TestMfa_EnrollHappyPath(t *testing.T) {
	s, user := setupMfaTest(t)
	secret, qr, err := s.Enroll(user.Id)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// 20 byte base32 (NoPadding) → 32 字符
	if len(secret) != 32 {
		t.Fatalf("expected 32-char base32 secret, got %d (%q)", len(secret), secret)
	}
	if len(qr) == 0 {
		t.Fatalf("qr empty")
	}
	// PNG magic: 89 50 4E 47 0D 0A 1A 0A
	pngSig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(qr) < 8 || string(qr[:8]) != string(pngSig) {
		t.Fatalf("qr is not PNG, first bytes: %x", qr[:min(8, len(qr))])
	}
	row := &model.UserMfa{}
	if err := DB.Where("user_id = ?", user.Id).First(row).Error; err != nil {
		t.Fatalf("load mfa row: %v", err)
	}
	if row.EnabledAt != nil && *row.EnabledAt > 0 {
		t.Fatalf("enabled_at should be pending, got %v", row.EnabledAt)
	}
	if row.Secret == secret {
		t.Fatalf("secret should be encrypted at rest, but matches plaintext")
	}
}

func TestMfa_VerifyCorrectCodeActivates(t *testing.T) {
	s, user := setupMfaTest(t)
	secret, _, err := s.Enroll(user.Id)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	code := generateCurrentCode(t, secret, time.Now().UTC())
	ok, err := s.Verify(user.Id, code)
	if err != nil || !ok {
		t.Fatalf("verify expected (true, nil), got (%v, %v)", ok, err)
	}
	row := &model.UserMfa{}
	if err := DB.Where("user_id = ?", user.Id).First(row).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if row.EnabledAt == nil || *row.EnabledAt <= 0 {
		t.Fatalf("enabled_at not set after successful verify")
	}
	if row.LastUsedAt == nil || *row.LastUsedAt <= 0 {
		t.Fatalf("last_used_at not set after successful verify")
	}
}

func TestMfa_VerifyWrongCode(t *testing.T) {
	s, user := setupMfaTest(t)
	if _, _, err := s.Enroll(user.Id); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	ok, err := s.Verify(user.Id, "000000")
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if ok {
		t.Fatalf("verify expected false for wrong code")
	}
	row := &model.UserMfa{}
	if err := DB.Where("user_id = ?", user.Id).First(row).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if row.EnabledAt != nil && *row.EnabledAt > 0 {
		t.Fatalf("enabled_at should stay pending on wrong code")
	}
}

func TestMfa_VerifyNotEnrolled(t *testing.T) {
	s, user := setupMfaTest(t)
	ok, err := s.Verify(user.Id, "123456")
	if ok {
		t.Fatalf("expected false when not enrolled")
	}
	if err != ErrMfaNotEnrolled {
		t.Fatalf("expected ErrMfaNotEnrolled, got %v", err)
	}
}

func TestMfa_RecoveryCodeShapeAndOneTime(t *testing.T) {
	s, user := setupMfaTest(t)
	secret, _, err := s.Enroll(user.Id)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if _, err := s.Verify(user.Id, generateCurrentCode(t, secret, time.Now().UTC())); err != nil {
		t.Fatalf("activate verify: %v", err)
	}
	codes, err := s.GenerateRecoveryCodes(user.Id)
	if err != nil {
		t.Fatalf("generate recovery: %v", err)
	}
	if len(codes) != 12 {
		t.Fatalf("expected 12 codes, got %d", len(codes))
	}
	for i, c := range codes {
		if len(c) != 10 {
			t.Fatalf("code %d wrong length %d", i, len(c))
		}
	}
	ok, err := s.ConsumeRecoveryCode(user.Id, codes[0])
	if err != nil || !ok {
		t.Fatalf("first consume expected (true, nil), got (%v, %v)", ok, err)
	}
	ok, err = s.ConsumeRecoveryCode(user.Id, codes[0])
	if ok {
		t.Fatalf("second consume should fail")
	}
	if err != ErrMfaRecoveryUsed {
		t.Fatalf("expected ErrMfaRecoveryUsed, got %v", err)
	}
	// 其他 code 仍可使用
	ok, err = s.ConsumeRecoveryCode(user.Id, codes[1])
	if err != nil || !ok {
		t.Fatalf("sibling consume expected (true, nil), got (%v, %v)", ok, err)
	}
}

func TestMfa_RecoveryCodeBeforeEnrollRejected(t *testing.T) {
	s, user := setupMfaTest(t)
	if _, _, err := s.Enroll(user.Id); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	// 此时 pending,未经过 Verify 激活
	if _, err := s.GenerateRecoveryCodes(user.Id); err != ErrMfaNotEnrolled {
		t.Fatalf("expected ErrMfaNotEnrolled, got %v", err)
	}
	if ok, err := s.ConsumeRecoveryCode(user.Id, "ABCDEFGHIJ"); ok || err != ErrMfaNotEnrolled {
		t.Fatalf("expected (false, ErrMfaNotEnrolled), got (%v, %v)", ok, err)
	}
}

func TestMfa_AlreadyEnrolled(t *testing.T) {
	s, user := setupMfaTest(t)
	secret, _, err := s.Enroll(user.Id)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if _, err := s.Verify(user.Id, generateCurrentCode(t, secret, time.Now().UTC())); err != nil {
		t.Fatalf("activate verify: %v", err)
	}
	sec2, qr2, err := s.Enroll(user.Id)
	if err != ErrMfaAlreadyEnrolled {
		t.Fatalf("expected ErrMfaAlreadyEnrolled, got %v", err)
	}
	if sec2 != "" || len(qr2) != 0 {
		t.Fatalf("expected zero secret/qr on already-enrolled, got %q / %d bytes", sec2, len(qr2))
	}
}

func TestMfa_DisableThenReEnroll(t *testing.T) {
	s, user := setupMfaTest(t)
	secret1, _, err := s.Enroll(user.Id)
	if err != nil {
		t.Fatalf("enroll1: %v", err)
	}
	if _, err := s.Verify(user.Id, generateCurrentCode(t, secret1, time.Now().UTC())); err != nil {
		t.Fatalf("activate verify: %v", err)
	}
	if !s.IsEnrolled(user.Id) {
		t.Fatalf("IsEnrolled should be true")
	}
	if err := s.Disable(user.Id); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if s.IsEnrolled(user.Id) {
		t.Fatalf("IsEnrolled should be false after disable")
	}
	// Disable on already-gone returns ErrMfaNotEnrolled
	if err := s.Disable(user.Id); err != ErrMfaNotEnrolled {
		t.Fatalf("expected ErrMfaNotEnrolled, got %v", err)
	}
	secret2, _, err := s.Enroll(user.Id)
	if err != nil {
		t.Fatalf("enroll2: %v", err)
	}
	if secret2 == secret1 {
		t.Fatalf("new enroll should produce a different secret; both = %s", strings.Repeat("*", len(secret1)))
	}
}

func TestMfa_ExpiredCodeRejected(t *testing.T) {
	s, user := setupMfaTest(t)
	secret, _, err := s.Enroll(user.Id)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if _, err := s.Verify(user.Id, generateCurrentCode(t, secret, time.Now().UTC())); err != nil {
		t.Fatalf("activate verify: %v", err)
	}
	oldCode := generateCurrentCode(t, secret, time.Now().UTC().Add(-5*time.Minute))
	ok, err := s.Verify(user.Id, oldCode)
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if ok {
		t.Fatalf("verify should reject code outside skew window")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
