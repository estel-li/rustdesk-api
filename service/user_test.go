package service

import (
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/lib/lock"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

// setupUserMfaPolicyTest 复用 mfa_test 风格,但额外迁移 Group / LoginLog 表,
// 并把 AllService 装一份最小依赖(只用到 GroupService / UserService)。
func setupUserMfaPolicyTest(t *testing.T) (*UserService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:memdb_user_policy_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlDB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.User{}, &model.Group{}, &model.LoginLog{}, &model.UserMfa{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	cfg := &config.Config{}
	cfg.Admin.Title = "RustDeskTest"
	cfg.Jwt.Key = "test-jwt-key-policy-1234567890"

	DB = db
	Config = cfg
	Logger = logrus.New()
	Logger.SetLevel(logrus.PanicLevel)
	Lock = lock.NewLocal()

	AllService = new(Service)
	AllService.UserService = &UserService{}
	AllService.GroupService = &GroupService{}
	AllService.MfaService = &MfaService{}
	return AllService.UserService, db
}

// helperBool 取 *bool;辅助构造测试数据。
func helperBool(v bool) *bool { return &v }

func createGroupWithMfa(t *testing.T, db *gorm.DB, name string, mfa *bool) *model.Group {
	t.Helper()
	g := &model.Group{Name: name, Type: model.GroupTypeDefault, MfaRequired: mfa}
	if err := db.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	return g
}

func createUserWithMfa(t *testing.T, db *gorm.DB, name string, groupId uint, mfa *bool) *model.User {
	t.Helper()
	u := &model.User{Username: name, GroupId: groupId, MfaRequired: mfa, Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

// 1. TestEffectiveMfaRequired_UserOnly
func TestEffectiveMfaRequired_UserOnly(t *testing.T) {
	us, db := setupUserMfaPolicyTest(t)
	g := createGroupWithMfa(t, db, "g", helperBool(false))
	u := createUserWithMfa(t, db, "u", g.Id, helperBool(true))
	if !us.EffectiveMfaRequired(u) {
		t.Fatalf("expected effective=true when user.MfaRequired=true")
	}
}

// 2. TestEffectiveMfaRequired_GroupOnly
func TestEffectiveMfaRequired_GroupOnly(t *testing.T) {
	us, db := setupUserMfaPolicyTest(t)
	g := createGroupWithMfa(t, db, "g", helperBool(true))
	u := createUserWithMfa(t, db, "u", g.Id, helperBool(false))
	if !us.EffectiveMfaRequired(u) {
		t.Fatalf("expected effective=true when group.MfaRequired=true")
	}
}

// 3. TestEffectiveMfaRequired_Both
func TestEffectiveMfaRequired_Both(t *testing.T) {
	us, db := setupUserMfaPolicyTest(t)
	g := createGroupWithMfa(t, db, "g", helperBool(true))
	u := createUserWithMfa(t, db, "u", g.Id, helperBool(true))
	if !us.EffectiveMfaRequired(u) {
		t.Fatalf("expected effective=true when both true")
	}
}

// 4. TestEffectiveMfaRequired_Neither
func TestEffectiveMfaRequired_Neither(t *testing.T) {
	us, db := setupUserMfaPolicyTest(t)
	g := createGroupWithMfa(t, db, "g", helperBool(false))
	u := createUserWithMfa(t, db, "u", g.Id, helperBool(false))
	if us.EffectiveMfaRequired(u) {
		t.Fatalf("expected effective=false when neither set")
	}
}

// 5. TestEffectiveMfaRequired_NilUserPointer:user.MfaRequired = nil 视为 false,不 panic。
func TestEffectiveMfaRequired_NilUserPointer(t *testing.T) {
	us, db := setupUserMfaPolicyTest(t)
	g := createGroupWithMfa(t, db, "g", nil)
	u := createUserWithMfa(t, db, "u", g.Id, nil)
	if us.EffectiveMfaRequired(u) {
		t.Fatalf("expected effective=false when both pointers nil")
	}
}

// TestSetMfaRequired_WritesAudit:SetMfaRequired 应该把 mfa_required 落库且写一条审计日志。
func TestSetMfaRequired_WritesAudit(t *testing.T) {
	us, db := setupUserMfaPolicyTest(t)
	g := createGroupWithMfa(t, db, "g", helperBool(false))
	u := createUserWithMfa(t, db, "u", g.Id, helperBool(false))
	op := createUserWithMfa(t, db, "admin", g.Id, helperBool(false))

	if err := us.SetMfaRequired(u, true, op, "1.2.3.4"); err != nil {
		t.Fatalf("SetMfaRequired: %v", err)
	}
	got := us.InfoById(u.Id)
	if got.MfaRequired == nil || !*got.MfaRequired {
		t.Fatalf("expected mfa_required=true persisted, got %+v", got.MfaRequired)
	}
	var logs []model.LoginLog
	if err := db.Where("type = ?", model.LoginLogTypeMfaRequiredSet).Find(&logs).Error; err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log of type mfa_required_set, got %d", len(logs))
	}
	if logs[0].UserId != u.Id {
		t.Fatalf("audit target user_id mismatch: got %d want %d", logs[0].UserId, u.Id)
	}
	if logs[0].Ip != "1.2.3.4" {
		t.Fatalf("audit ip mismatch: %q", logs[0].Ip)
	}
}

// TestDisableMfa_WritesAudit:DisableMfa 应该把 mfa_required 复位且写 mfa_disabled_by_admin 审计。
func TestDisableMfa_WritesAudit(t *testing.T) {
	us, db := setupUserMfaPolicyTest(t)
	g := createGroupWithMfa(t, db, "g", helperBool(false))
	u := createUserWithMfa(t, db, "u", g.Id, helperBool(true))
	op := createUserWithMfa(t, db, "admin", g.Id, helperBool(false))

	if err := us.DisableMfa(u, op, "9.9.9.9", "lost device"); err != nil {
		t.Fatalf("DisableMfa: %v", err)
	}
	got := us.InfoById(u.Id)
	if got.MfaRequired == nil || *got.MfaRequired {
		t.Fatalf("expected mfa_required=false after disable, got %+v", got.MfaRequired)
	}
	var logs []model.LoginLog
	if err := db.Where("type = ?", model.LoginLogTypeMfaDisabledByAdmin).Find(&logs).Error; err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log of type mfa_disabled_by_admin, got %d", len(logs))
	}
	if logs[0].UserId != u.Id {
		t.Fatalf("audit target user_id mismatch: got %d want %d", logs[0].UserId, u.Id)
	}
}
