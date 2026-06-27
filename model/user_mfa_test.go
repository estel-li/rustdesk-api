package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB 用纯内存 sqlite 跑 AutoMigrate,避免污染仓库目录。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 每次打开 ":memory:" 都得到独立隔离的库,避免不同测试间数据互相污染。
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlDB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestUserMfa_AutoMigrate_Create(t *testing.T) {
	db := newTestDB(t)
	if err := db.AutoMigrate(&UserMfa{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	codes := custom_types.AutoJson(json.RawMessage(`["h1","h2"]`))
	row := &UserMfa{
		UserId:        1,
		Secret:        "enc",
		RecoveryCodes: codes,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.Id == 0 {
		t.Fatalf("expected id assigned by AutoIncrement")
	}

	var got UserMfa
	if err := db.First(&got, row.Id).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if got.UserId != 1 || got.Secret != "enc" {
		t.Fatalf("read back mismatch: %+v", got)
	}
	if got.EnabledAt != nil {
		t.Fatalf("expected EnabledAt nil, got %v", *got.EnabledAt)
	}
	if got.LastUsedAt != nil {
		t.Fatalf("expected LastUsedAt nil, got %v", *got.LastUsedAt)
	}
	// recovery_codes JSON 内容应该原样回放。
	if strings.TrimSpace(got.RecoveryCodes.String()) != `["h1","h2"]` {
		t.Fatalf("recovery codes mismatch: %s", got.RecoveryCodes.String())
	}
}

func TestUserMfa_UniqueUserId(t *testing.T) {
	db := newTestDB(t)
	if err := db.AutoMigrate(&UserMfa{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.Create(&UserMfa{UserId: 7, Secret: "a"}).Error; err != nil {
		t.Fatalf("create 1: %v", err)
	}
	err := db.Create(&UserMfa{UserId: 7, Secret: "b"}).Error
	if err == nil {
		t.Fatalf("expected unique index violation on duplicate user_id")
	}
}

func TestUserMfa_RecoveryCodes_Empty(t *testing.T) {
	db := newTestDB(t)
	if err := db.AutoMigrate(&UserMfa{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	row := &UserMfa{UserId: 2, Secret: "x"}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got UserMfa
	if err := db.First(&got, row.Id).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	// AutoJson 在 zero value 落库时会被序列化为 "null"(json.RawMessage(nil).MarshalJSON()),
	// 读回后维持 "null";若列被真正写成空字符串则会走 auto_json.go:30-33 的回退,得到 "[]"。
	// 任一情况都视为"空 recovery codes",service 层 Unmarshal 时统一按 0 个 hash 处理。
	if got.RecoveryCodes == nil {
		t.Fatalf("expected non-nil AutoJson")
	}
	s := strings.TrimSpace(got.RecoveryCodes.String())
	if s != "[]" && s != "null" {
		t.Fatalf("expected JSON [] or null, got %q", s)
	}
}

func TestUserMfa_EnabledAt_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	if err := db.AutoMigrate(&UserMfa{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	now := time.Now().Unix()
	row := &UserMfa{UserId: 3, Secret: "x", EnabledAt: &now, LastUsedAt: &now}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got UserMfa
	if err := db.First(&got, row.Id).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if got.EnabledAt == nil || *got.EnabledAt != now {
		t.Fatalf("EnabledAt roundtrip failed: %+v", got.EnabledAt)
	}
	if got.LastUsedAt == nil || *got.LastUsedAt != now {
		t.Fatalf("LastUsedAt roundtrip failed: %+v", got.LastUsedAt)
	}
}

func TestUserMfa_OldVersionCompat(t *testing.T) {
	// 模拟已有 v=265 schema:先建 User + Version 表,再加 UserMfa,断言原表保留。
	db := newTestDB(t)
	if err := db.AutoMigrate(&Version{}, &User{}); err != nil {
		t.Fatalf("old migrate: %v", err)
	}
	if err := db.Create(&Version{Version: 265}).Error; err != nil {
		t.Fatalf("seed version: %v", err)
	}
	// 再跑一遍含 UserMfa 的 AutoMigrate,等价于服务端启动后的 Migrate(266)
	if err := db.AutoMigrate(&Version{}, &User{}, &UserMfa{}); err != nil {
		t.Fatalf("new migrate: %v", err)
	}
	// 老的 User 表与 Version 数据应仍可用
	if !db.Migrator().HasTable(&User{}) {
		t.Fatalf("users table dropped during migration")
	}
	if !db.Migrator().HasTable(&UserMfa{}) {
		t.Fatalf("user_mfas table missing after migration")
	}
	var cnt int64
	if err := db.Model(&Version{}).Where("version = ?", 265).Count(&cnt).Error; err != nil {
		t.Fatalf("count old version: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected to preserve old version row, got %d", cnt)
	}
	// 新表可写
	if err := db.Create(&UserMfa{UserId: 99, Secret: "z"}).Error; err != nil {
		t.Fatalf("create on migrated table: %v", err)
	}
}
