package model

const (
	AuditActionNew   = "new"
	AuditActionClose = "close"
)

type AuditConn struct {
	IdModel
	Action    string `json:"action" gorm:"default:'';not null;"`
	ConnId    int64  `json:"conn_id" gorm:"default:0;not null;index"`
	PeerId    string `json:"peer_id" gorm:"default:'';not null;index"`
	FromPeer  string `json:"from_peer" gorm:"default:'';not null;"`
	FromName  string `json:"from_name" gorm:"default:'';not null;"`
	Ip        string `json:"ip" gorm:"default:'';not null;"`
	SessionId string `json:"session_id" gorm:"default:'';not null;"`
	Type      int    `json:"type" gorm:"default:0;not null;"`
	Uuid      string `json:"uuid" gorm:"default:'';not null;"`
	CloseTime int64  `json:"close_time" gorm:"default:0;not null;"`
	TimeModel
}

type AuditConnList struct {
	AuditConns []*AuditConn `json:"list"`
	Pagination
}

type AuditFile struct {
	IdModel
	FromPeer string `json:"from_peer" gorm:"default:'';not null;index"`
	Info     string `json:"info" gorm:"default:'';not null;"`
	IsFile   bool   `json:"is_file" gorm:"default:0;not null;"`
	Path     string `json:"path" gorm:"default:'';not null;"`
	PeerId   string `json:"peer_id" gorm:"default:'';not null;index"`
	Type     int    `json:"type" gorm:"default:0;not null;"`
	Uuid     string `json:"uuid" gorm:"default:'';not null;"`
	Ip       string `json:"ip" gorm:"default:'';not null;"`
	Num      int    `json:"num" gorm:"default:0;not null;"`
	FromName string `json:"from_name" gorm:"default:'';not null;"`
	TimeModel
}

type AuditFileList struct {
	AuditFiles []*AuditFile `json:"list"`
	Pagination
}

// CE-M1-6: 统一审计事件表,承接剪贴板 / 告警 / 远程命令 / 录像等新事件。
// 旧的 AuditConn / AuditFile 行为保持不变;此表只增不替。
const (
	AuditEventKindClipboard = "clipboard" // 文本/文件剪贴板
	AuditEventKindAlarm     = "alarm"     // 策略告警 / 异常断开
	AuditEventKindCmd       = "cmd"       // 服务端下发或客户端执行命令
	AuditEventKindRecord    = "record"    // 会话录像开始/结束
)

// AuditEventKinds 是合法 kind 的白名单。未来扩展直接在此追加常量并补入。
var AuditEventKinds = map[string]struct{}{
	AuditEventKindClipboard: {},
	AuditEventKindAlarm:     {},
	AuditEventKindCmd:       {},
	AuditEventKindRecord:    {},
}

// IsValidAuditEventKind 用于 request 层做白名单校验。
func IsValidAuditEventKind(k string) bool {
	_, ok := AuditEventKinds[k]
	return ok
}

// AuditEvent 统一审计事件。
// 复合索引 (kind, created_at) 通过 cmd/apimain.go 中 Migrate() 末尾的 CREATE INDEX 显式建立,
// 这里 tag 上的 priority:1 仅作为说明,GORM 在多个 struct 字段上拼装复合索引比较脆弱,故由 SQL 兜底。
type AuditEvent struct {
	IdModel
	Kind        string `json:"kind"         gorm:"size:32;default:'';not null;index:idx_audit_event_kind_created,priority:1"`
	PeerId      string `json:"peer_id"      gorm:"size:64;default:'';not null;index"`
	FromPeer    string `json:"from_peer"    gorm:"size:64;default:'';not null;index"`
	FromName    string `json:"from_name"    gorm:"size:128;default:'';not null;"`
	SessionId   string `json:"session_id"   gorm:"size:64;default:'';not null;"`
	Ip          string `json:"ip"           gorm:"size:64;default:'';not null;"`
	PayloadJson string `json:"payload_json" gorm:"type:text;default:'';not null;"`
	TimeModel
}

type AuditEventList struct {
	AuditEvents []*AuditEvent `json:"list"`
	Pagination
}
