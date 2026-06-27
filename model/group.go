package model

const (
	GroupTypeDefault = 1 // 默认
	GroupTypeShare   = 2 // 共享
)

type Group struct {
	IdModel
	Name string `json:"name" gorm:"default:'';not null;"`
	Type int    `json:"type" gorm:"default:1;not null;"`
	// MfaRequired CE-M1-5 组级强制 MFA 开关:与 user.mfa_required 取或得到生效策略。
	MfaRequired *bool `json:"mfa_required" gorm:"default:0;not null;index"`
	TimeModel
}

type GroupList struct {
	Groups []*Group `json:"list"`
	Pagination
}

type DeviceGroup struct {
	IdModel
	Name string `json:"name" gorm:"default:'';not null;"`
	TimeModel
}

type DeviceGroupList struct {
	DeviceGroups []*DeviceGroup `json:"list"`
	Pagination
}
