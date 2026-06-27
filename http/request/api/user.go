package api

/*
*

	message LoginRequest {
	  string username = 1;
	  bytes password = 2;
	  string my_id = 4;
	  string my_name = 5;
	  OptionMessage option = 6;
	  oneof union {
	    FileTransfer file_transfer = 7;
	    PortForward port_forward = 8;
	  }
	  bool video_ack_required = 9;
	  uint64 session_id = 10;
	  string version = 11;
	  OSLogin os_login = 12;
	  string my_platform = 13;
	  bytes hwid = 14;
	}
*/

type DeviceInfoInLogin struct {
	Name string `json:"name" label:"name"`
	Os   string `json:"os" label:"os"`
	Type string `json:"type" label:"type"`
}

type LoginForm struct {
	AutoLogin  bool              `json:"autoLogin" label:"自动登录"`
	DeviceInfo DeviceInfoInLogin `json:"deviceInfo" label:"设备信息"`
	Id         string            `json:"id"  label:"id"`
	Type       string            `json:"type"  label:"type"`
	Uuid       string            `json:"uuid"  label:"uuid"`
	Username   string            `json:"username" validate:"required,gte=2,lte=32" label:"用户名"`
	Password   string            `json:"password,omitempty" validate:"gte=4,lte=32" label:"密码"`
}

// LoginMfaForm CE-M1-3 两步登录第二步:携带首步签发的 ticket + TOTP / recovery code 完成 MFA 校验。
// Type 留空时默认按 totp 处理,recovery 用于一次性恢复码。
type LoginMfaForm struct {
	Ticket string `json:"ticket" validate:"required,gte=10,lte=512" label:"ticket"`
	Code   string `json:"code"   validate:"required,gte=6,lte=16"    label:"验证码"`
	Type   string `json:"type"   validate:"omitempty,oneof=totp recovery" label:"类型"`
}

type UserListQuery struct {
	Page       uint   `json:"page" form:"page" validate:"required" label:"页码"`
	PageSize   uint   `json:"pageSize" form:"pageSize" validate:"required" label:"每页数量"`
	Status     int    `json:"status" form:"status" label:"状态"`
	Accessible string `json:"accessible" form:"accessible"`
}

type PeerListQuery struct {
	Page       uint   `json:"page" form:"page" validate:"required" label:"页码"`
	PageSize   uint   `json:"pageSize" form:"pageSize" validate:"required" label:"每页数量"`
	Status     int    `json:"status" form:"status" label:"状态"`
	Accessible string `json:"accessible" form:"accessible"`
}
