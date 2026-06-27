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

// setupAuditEventTest CE-M1-6: 用纯内存 sqlite 初始化 service.DB,
// 并 AutoMigrate audit_events 以及兼容性需要的 audit_conns / audit_files。
func setupAuditEventTest(t *testing.T) *AuditService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:memdb_audit_event_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlDB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.AuditEvent{}, &model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	cfg := &config.Config{}
	cfg.Admin.Title = "RustDeskTest"

	DB = db
	Config = cfg
	Logger = logrus.New()
	Logger.SetLevel(logrus.PanicLevel)
	Lock = lock.NewLocal()

	AllService = new(Service)
	AllService.AuditService = &AuditService{}
	return AllService.AuditService
}

// 1. 列表 / 过滤
func TestAuditService_CreateAndList(t *testing.T) {
	as := setupAuditEventTest(t)

	// 3 条 clipboard + 2 条 alarm
	for i := 0; i < 3; i++ {
		if err := as.CreateAuditEvent(&model.AuditEvent{
			Kind:        model.AuditEventKindClipboard,
			PeerId:      "peer-c",
			PayloadJson: "{}",
		}); err != nil {
			t.Fatalf("create clipboard: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := as.CreateAuditEvent(&model.AuditEvent{
			Kind:        model.AuditEventKindAlarm,
			PeerId:      "peer-a",
			PayloadJson: "{}",
		}); err != nil {
			t.Fatalf("create alarm: %v", err)
		}
	}

	// 按 kind=clipboard 过滤
	res := as.AuditEventList(1, 10, func(tx *gorm.DB) {
		tx.Where("kind = ?", model.AuditEventKindClipboard)
		tx.Order("id desc")
	})
	if res.Total != 3 {
		t.Fatalf("expected total=3, got %d", res.Total)
	}
	if len(res.AuditEvents) != 3 {
		t.Fatalf("expected len=3, got %d", len(res.AuditEvents))
	}
	for _, ev := range res.AuditEvents {
		if ev.Kind != model.AuditEventKindClipboard {
			t.Fatalf("unexpected kind in clipboard filter: %s", ev.Kind)
		}
	}

	// 无 where -> 全部 5 条
	all := as.AuditEventList(1, 10, nil)
	if all.Total != 5 {
		t.Fatalf("expected total=5, got %d", all.Total)
	}
}

// 2. 批量删除 + InfoById
func TestAuditService_BatchDeleteAuditEvent(t *testing.T) {
	as := setupAuditEventTest(t)

	ids := make([]uint, 0, 5)
	for i := 0; i < 5; i++ {
		ev := &model.AuditEvent{Kind: model.AuditEventKindCmd, PeerId: "p", PayloadJson: "{}"}
		if err := as.CreateAuditEvent(ev); err != nil {
			t.Fatalf("create: %v", err)
		}
		ids = append(ids, ev.Id)
	}
	toDel := []uint{ids[0], ids[1], ids[2]}
	if err := as.BatchDeleteAuditEvent(toDel); err != nil {
		t.Fatalf("batch delete: %v", err)
	}

	res := as.AuditEventList(1, 10, nil)
	if res.Total != 2 {
		t.Fatalf("expected total=2 after batch delete, got %d", res.Total)
	}

	for _, id := range toDel {
		got := as.EventInfoById(id)
		if got.Id != 0 {
			t.Fatalf("expected zero-value AuditEvent for deleted id %d, got Id=%d", id, got.Id)
		}
	}
	// 未删除的应能查到
	left := as.EventInfoById(ids[3])
	if left.Id != ids[3] {
		t.Fatalf("expected to find id=%d, got %d", ids[3], left.Id)
	}
}

// 3. 单条删除
func TestAuditService_DeleteAuditEvent(t *testing.T) {
	as := setupAuditEventTest(t)
	ev := &model.AuditEvent{Kind: model.AuditEventKindRecord, PeerId: "p", PayloadJson: "{}"}
	if err := as.CreateAuditEvent(ev); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := as.DeleteAuditEvent(ev); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := as.EventInfoById(ev.Id); got.Id != 0 {
		t.Fatalf("expected zero-value after delete, got Id=%d", got.Id)
	}
}

// 4. 现有 AuditConn / AuditFile 在新表迁移后仍可正常工作 - 兼容性用例
func TestAuditFile_StillWorks_AfterMigration(t *testing.T) {
	as := setupAuditEventTest(t)
	af := &model.AuditFile{
		PeerId:   "p1",
		FromPeer: "p2",
		Info:     "{}",
		Path:     "/tmp",
	}
	if err := as.CreateAuditFile(af); err != nil {
		t.Fatalf("create audit_file: %v", err)
	}
	got := as.FileInfoById(af.Id)
	if got.Id != af.Id || got.PeerId != "p1" {
		t.Fatalf("audit_file not preserved: %+v", got)
	}
	// 同样测试 AuditConn
	ac := &model.AuditConn{PeerId: "p1", ConnId: 42, FromPeer: "p2", Action: model.AuditActionNew}
	if err := as.CreateAuditConn(ac); err != nil {
		t.Fatalf("create audit_conn: %v", err)
	}
	if got := as.ConnInfoById(ac.Id); got.Id != ac.Id || got.ConnId != 42 {
		t.Fatalf("audit_conn not preserved: %+v", got)
	}
}
