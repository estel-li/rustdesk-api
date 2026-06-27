package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// setupAdminAuditEventTest CE-M1-6: 挂一个最小路由,调用未经鉴权中间件的 controller。
func setupAdminAuditEventTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:memdb_admin_audit_event_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlDB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.AuditEvent{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	bundle := i18n.NewBundle(language.English)
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, "en") }
	global.Logger = logrus.New()
	global.Logger.SetLevel(logrus.PanicLevel)
	global.DB = db

	service.DB = db
	service.Logger = global.Logger
	service.AllService = new(service.Service)
	service.AllService.AuditService = &service.AuditService{}

	r := gin.New()
	cont := &Audit{}
	r.GET("/api/admin/audit_event/list", cont.EventList)
	return r, db
}

// 6. FilterByKind:写 2 条 clipboard + 2 条 alarm,按 kind=alarm 查
func TestAdminAuditEventList_FilterByKind(t *testing.T) {
	r, db := setupAdminAuditEventTest(t)

	for i := 0; i < 2; i++ {
		if err := db.Create(&model.AuditEvent{Kind: model.AuditEventKindClipboard, PeerId: "p", PayloadJson: "{}"}).Error; err != nil {
			t.Fatalf("create clipboard: %v", err)
		}
		if err := db.Create(&model.AuditEvent{Kind: model.AuditEventKindAlarm, PeerId: "p", PayloadJson: "{}"}).Error; err != nil {
			t.Fatalf("create alarm: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit_event/list?kind=alarm&page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := struct {
		Code int `json:"code"`
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if body.Data.Total != 2 {
		t.Fatalf("expected total=2, got %d", body.Data.Total)
	}
	if len(body.Data.List) != 2 {
		t.Fatalf("expected len=2, got %d", len(body.Data.List))
	}
	for _, ev := range body.Data.List {
		if ev["kind"] != model.AuditEventKindAlarm {
			t.Fatalf("unexpected kind in alarm filter: %v", ev["kind"])
		}
	}
}

// 7. 不传 kind:返回所有事件,按 id desc 排序
func TestAdminAuditEventList_BackwardCompat_NoKind(t *testing.T) {
	r, db := setupAdminAuditEventTest(t)
	if err := db.Create(&model.AuditEvent{Kind: model.AuditEventKindClipboard, PayloadJson: "{}"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Create(&model.AuditEvent{Kind: model.AuditEventKindCmd, PayloadJson: "{}"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Create(&model.AuditEvent{Kind: model.AuditEventKindAlarm, PayloadJson: "{}"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit_event/list?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	body := struct {
		Code int `json:"code"`
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if body.Data.Total != 3 {
		t.Fatalf("expected total=3, got %d", body.Data.Total)
	}
	// id desc:第一个应是最后插入的(alarm)
	if len(body.Data.List) == 0 || body.Data.List[0]["kind"] != model.AuditEventKindAlarm {
		t.Fatalf("expected alarm first under id desc, got list=%+v", body.Data.List)
	}
}
