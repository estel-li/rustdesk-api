package admin

type AuditQuery struct {
	PeerId   string `form:"peer_id"`
	FromPeer string `form:"from_peer"`
	PageQuery
}

type AuditConnLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
type AuditFileLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}

// CE-M1-6: 后台审计事件列表查询。kind 精确匹配,留空表示全部。
type AuditEventQuery struct {
	Kind     string `form:"kind"`
	PeerId   string `form:"peer_id"`
	FromPeer string `form:"from_peer"`
	PageQuery
}

// AuditEventLogIds 后台批量删除请求体。
type AuditEventLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
