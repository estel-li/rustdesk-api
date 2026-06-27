package service

import (
	"strconv"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type GroupService struct {
}

// InfoById 根据用户id取用户信息
func (us *GroupService) InfoById(id uint) *model.Group {
	u := &model.Group{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (us *GroupService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.GroupList) {
	res = &model.GroupList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Group{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Groups)
	return
}

// Create 创建
func (us *GroupService) Create(u *model.Group) error {
	res := DB.Create(u).Error
	return res
}
func (us *GroupService) Delete(u *model.Group) error {
	return DB.Delete(u).Error
}

// Update 更新
func (us *GroupService) Update(u *model.Group) error {
	// CE-M1-5:显式列更新,避免 GORM Updates 把 form 未携带的 mfa_required 误置 false。
	return DB.Model(u).Select("name", "type", "mfa_required").Updates(u).Error
}

// SetMfaRequired CE-M1-5 管理员切换组级强制 MFA 开关,同步把审计日志写到 login_logs。
// LoginLog.UserId 处填 0(组级操作无目标账户),组 id 字面塞到 Client 列 "group:<id>" 便于检索。
func (us *GroupService) SetMfaRequired(g *model.Group, required bool, opUser *model.User, ip string) error {
	if g == nil || g.Id == 0 {
		return DB.Model(&model.Group{}).Where("id = ?", 0).Error // 触发空更新,显式失败
	}
	v := required
	if err := DB.Model(&model.Group{}).Where("id = ?", g.Id).
		Update("mfa_required", &v).Error; err != nil {
		return err
	}
	g.MfaRequired = &v
	action := model.LoginLogTypeMfaRequiredSet
	if !required {
		action = model.LoginLogTypeMfaRequiredUnset
	}
	client := "admin"
	if opUser != nil && opUser.Username != "" {
		client = "admin:" + opUser.Username
	}
	row := &model.LoginLog{
		UserId: 0,
		Client: client,
		Ip:     ip,
		Type:   action,
		Uuid:   "group:" + strconv.FormatUint(uint64(g.Id), 10),
	}
	if err := DB.Create(row).Error; err != nil {
		Logger.Errorf("write group mfa audit fail group=%d action=%s err=%v", g.Id, action, err)
	}
	return nil
}

// DeviceGroupInfoById 根据用户id取用户信息
func (us *GroupService) DeviceGroupInfoById(id uint) *model.DeviceGroup {
	u := &model.DeviceGroup{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (us *GroupService) DeviceGroupList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.DeviceGroupList) {
	res = &model.DeviceGroupList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.DeviceGroup{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.DeviceGroups)
	return
}

func (us *GroupService) DeviceGroupCreate(u *model.DeviceGroup) error {
	res := DB.Create(u).Error
	return res
}
func (us *GroupService) DeviceGroupDelete(u *model.DeviceGroup) error {
	return DB.Delete(u).Error
}

func (us *GroupService) DeviceGroupUpdate(u *model.DeviceGroup) error {
	return DB.Model(u).Updates(u).Error
}
