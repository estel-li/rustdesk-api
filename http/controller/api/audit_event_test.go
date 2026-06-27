package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// setupAuditEventAPITest CE-M1-6: 构造最小依赖,挂一个仅含 /api/audit/event 的 gin 路由用于集成测试。
func setupAuditEventAPITest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:memdb_audit_event_api_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlDB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.AuditEvent{}, &model.AuditFile{}, &model.AuditConn{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	// 最小 i18n localizer:bundle 为空,LocalizeMessage 找不到 key 时直接回 messageId。
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
	au := &Audit{}
	r.POST("/api/audit/event", au.AuditEvent)
	r.POST("/api/audit/file", au.AuditFile)
	return r, db
}

func doJSON(t *testing.T, r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	buf := &bytes.Buffer{}
	if body != nil {
		_ = json.NewEncoder(buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 3. happy path: 一次成功插入并落库
func TestAuditEventAPI_HappyPath(t *testing.T) {
	r, db := setupAuditEventAPITest(t)
	w := doJSON(t, r, http.MethodPost, "/api/audit/event", map[string]interface{}{
		"kind":         "clipboard",
		"peer_id":      "1",
		"payload_json": "{}",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var cnt int64
	db.Model(&model.AuditEvent{}).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("expected 1 row inserted, got %d", cnt)
	}
	row := &model.AuditEvent{}
	db.First(row)
	if time.Time(row.CreatedAt).IsZero() {
		t.Fatalf("created_at should be set, got %v", row.CreatedAt)
	}
}

// 4. payload 超 16KB 拒绝
func TestAuditEventAPI_PayloadTooLarge(t *testing.T) {
	r, db := setupAuditEventAPITest(t)
	// 构造 17KB
	big := strings.Repeat("a", 17*1024)
	w := doJSON(t, r, http.MethodPost, "/api/audit/event", map[string]interface{}{
		"kind":         "clipboard",
		"peer_id":      "1",
		"payload_json": big,
	})
	// response.Error 返回 HTTP 400
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ParamsError") {
		t.Fatalf("expected error body to contain ParamsError, got %s", w.Body.String())
	}
	var cnt int64
	db.Model(&model.AuditEvent{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("expected 0 rows after rejection, got %d", cnt)
	}
}

// 5. kind 不在白名单
func TestAuditEventAPI_UnknownKind(t *testing.T) {
	r, db := setupAuditEventAPITest(t)
	w := doJSON(t, r, http.MethodPost, "/api/audit/event", map[string]interface{}{
		"kind":         "randomstring",
		"peer_id":      "1",
		"payload_json": "{}",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ParamsError") {
		t.Fatalf("expected error body to contain ParamsError, got %s", w.Body.String())
	}
	var cnt int64
	db.Model(&model.AuditEvent{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("expected 0 rows after rejection, got %d", cnt)
	}
}

// 8. 兼容性:原 /api/audit/file 端点仍能写入 audit_files,不被新表影响
func TestAuditFile_StillWorks_AfterMigration_API(t *testing.T) {
	r, db := setupAuditEventAPITest(t)
	w := doJSON(t, r, http.MethodPost, "/api/audit/file", map[string]interface{}{
		"id":      "peer-target",
		"peer_id": "peer-from",
		"info":    "{\"name\":\"alice\",\"ip\":\"10.0.0.1\",\"num\":2}",
		"is_file": true,
		"path":    "/tmp/x",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var cnt int64
	db.Model(&model.AuditFile{}).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("audit_files row not written, got %d", cnt)
	}
	// 新表不应被写入
	db.Model(&model.AuditEvent{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("audit_events should remain empty when /audit/file is used, got %d", cnt)
	}
}
