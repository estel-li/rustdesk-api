package api

import (
	"encoding/json"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"strconv"
)

type AuditConnForm struct {
	Action    string   `json:"action"`
	ConnId    int64    `json:"conn_id"`
	Id        string   `json:"id"`
	Peer      []string `json:"peer"`
	Ip        string   `json:"ip"`
	SessionId float64  `json:"session_id"`
	Type      int      `json:"type"`
	Uuid      string   `json:"uuid"`
}

func (a *AuditConnForm) ToAuditConn() *model.AuditConn {
	fp := ""
	fn := ""
	if len(a.Peer) >= 1 {
		fp = a.Peer[0]
		if len(a.Peer) == 2 {
			fn = a.Peer[1]
		}
	}
	ssid := strconv.FormatFloat(a.SessionId, 'f', -1, 64)
	return &model.AuditConn{
		Action:    a.Action,
		ConnId:    a.ConnId,
		PeerId:    a.Id,
		FromPeer:  fp,
		FromName:  fn,
		Ip:        a.Ip,
		SessionId: ssid,
		Type:      a.Type,
		Uuid:      a.Uuid,
	}
}

type AuditFileForm struct {
	Id     string `json:"id"`
	Info   string `json:"info"`
	IsFile bool   `json:"is_file"`
	Path   string `json:"path"`
	PeerId string `json:"peer_id"`
	Type   int    `json:"type"`
	Uuid   string `json:"uuid"`
}
type AuditFileInfo struct {
	Ip   string `json:"ip"`
	Name string `json:"name"`
	Num  int    `json:"num"`
}

func (a *AuditFileForm) ToAuditFile() *model.AuditFile {
	fi := &AuditFileInfo{}
	err := json.Unmarshal([]byte(a.Info), fi)
	if err != nil {
		global.Logger.Warn("ToAuditFile", err)
	}

	return &model.AuditFile{
		PeerId:   a.Id,
		Info:     a.Info,
		IsFile:   a.IsFile,
		FromPeer: a.PeerId,
		Path:     a.Path,
		Type:     a.Type,
		Uuid:     a.Uuid,
		FromName: fi.Name,
		Ip:       fi.Ip,
		Num:      fi.Num,
	}
}

// CE-M1-6: 统一审计事件入口表单。
// payload_json 必须由客户端做哈希/截断,服务端只做长度校验,不解析内容。
const AuditEventPayloadMaxBytes = 16 * 1024

// AuditEventForm 客户端 POST /api/audit/event 的请求体。
type AuditEventForm struct {
	Kind        string `json:"kind"`
	PeerId      string `json:"peer_id"`
	FromPeer    string `json:"from_peer"`
	FromName    string `json:"from_name"`
	SessionId   string `json:"session_id"`
	Ip          string `json:"ip"`
	PayloadJson string `json:"payload_json"`
}

// Validate 检查 kind 白名单与 payload 大小;返回错误字符串(空串表示通过)。
// 注意:服务端不解析 payload_json,只做长度校验,避免敏感剪贴板内容入库。
func (a *AuditEventForm) Validate() string {
	if !model.IsValidAuditEventKind(a.Kind) {
		return "ParamsError: unknown kind"
	}
	if len(a.PayloadJson) > AuditEventPayloadMaxBytes {
		return "ParamsError: payload too large"
	}
	return ""
}

// ToAuditEvent 把表单映射成 GORM 模型。
func (a *AuditEventForm) ToAuditEvent() *model.AuditEvent {
	return &model.AuditEvent{
		Kind:        a.Kind,
		PeerId:      a.PeerId,
		FromPeer:    a.FromPeer,
		FromName:    a.FromName,
		SessionId:   a.SessionId,
		Ip:          a.Ip,
		PayloadJson: a.PayloadJson,
	}
}
