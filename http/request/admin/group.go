package admin

import "github.com/lejianwen/rustdesk-api/v2/model"

type GroupForm struct {
	Id          uint   `json:"id"`
	Name        string `json:"name" validate:"required"`
	Type        int    `json:"type"`
	MfaRequired *bool  `json:"mfa_required"` // CE-M1-5 组级强制 MFA 开关
}

func (gf *GroupForm) FromGroup(group *model.Group) *GroupForm {
	gf.Id = group.Id
	gf.Name = group.Name
	gf.Type = group.Type
	gf.MfaRequired = group.MfaRequired
	return gf
}

func (gf *GroupForm) ToGroup() *model.Group {
	group := &model.Group{}
	group.Id = gf.Id
	group.Name = gf.Name
	group.Type = gf.Type
	group.MfaRequired = gf.MfaRequired
	return group
}

// GroupMfaToggleForm CE-M1-5 切换组级强制 MFA 开关。
type GroupMfaToggleForm struct {
	GroupId     uint  `json:"group_id" validate:"required,gt=0"`
	MfaRequired *bool `json:"mfa_required" validate:"required"`
}

type DeviceGroupForm struct {
	Id   uint   `json:"id"`
	Name string `json:"name" validate:"required"`
}

func (gf *DeviceGroupForm) ToDeviceGroup() *model.DeviceGroup {
	group := &model.DeviceGroup{}
	group.Id = gf.Id
	group.Name = gf.Name
	return group
}
