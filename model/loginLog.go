package model

type LoginLog struct {
	IdModel
	UserId      uint   `json:"user_id" gorm:"default:0;not null;"`
	Client      string `json:"client"` //webadmin,webclient,app,
	DeviceId    string `json:"device_id"`
	Uuid        string `json:"uuid"`
	Ip          string `json:"ip"`
	Type        string `json:"type"`     //account,oauth
	Platform    string `json:"platform"` //windows,linux,mac,android,ios
	UserTokenId uint   `json:"user_token_id" gorm:"default:0;not null;"`
	IsDeleted   uint   `json:"is_deleted" gorm:"default:0;not null;"`
	TimeModel
}

const (
	LoginLogClientWebAdmin = "webadmin"
	LoginLogClientWeb      = "webclient"
	LoginLogClientApp      = "app"
)

const (
	LoginLogTypeAccount = "account"
	LoginLogTypeOauth   = "oauth"
	// CE-M1-5 强制 MFA 审计枚举:登录日志复用此字段记录管理员高敏操作。
	LoginLogTypeMfaRequiredSet     = "mfa_required_set"
	LoginLogTypeMfaRequiredUnset   = "mfa_required_unset"
	LoginLogTypeMfaDisabledByAdmin = "mfa_disabled_by_admin"
	LoginLogTypeMfaEnrollForced    = "mfa_enroll_forced"
)

const (
	IsDeletedNo  = 0
	IsDeletedYes = 1
)

type LoginLogList struct {
	LoginLogs []*LoginLog `json:"list"`
	Pagination
}
