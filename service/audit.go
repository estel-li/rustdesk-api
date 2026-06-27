package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

type AuditService struct {
}

func (as *AuditService) AuditConnList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AuditConnList) {
	res = &model.AuditConnList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AuditConn{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.AuditConns)
	return
}

// Create 创建
func (as *AuditService) CreateAuditConn(u *model.AuditConn) error {
	res := DB.Create(u).Error
	return res
}
func (as *AuditService) DeleteAuditConn(u *model.AuditConn) error {
	return DB.Delete(u).Error
}

// Update 更新
func (as *AuditService) UpdateAuditConn(u *model.AuditConn) error {
	return DB.Model(u).Updates(u).Error
}

// InfoByPeerIdAndConnId
func (as *AuditService) InfoByPeerIdAndConnId(peerId string, connId int64) (res *model.AuditConn) {
	res = &model.AuditConn{}
	DB.Where("peer_id = ? and conn_id = ?", peerId, connId).First(res)
	return
}

// ConnInfoById
func (as *AuditService) ConnInfoById(id uint) (res *model.AuditConn) {
	res = &model.AuditConn{}
	DB.Where("id = ?", id).First(res)
	return
}

// FileInfoById
func (as *AuditService) FileInfoById(id uint) (res *model.AuditFile) {
	res = &model.AuditFile{}
	DB.Where("id = ?", id).First(res)
	return
}

func (as *AuditService) AuditFileList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AuditFileList) {
	res = &model.AuditFileList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AuditFile{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.AuditFiles)
	return
}

// CreateAuditFile
func (as *AuditService) CreateAuditFile(u *model.AuditFile) error {
	res := DB.Create(u).Error
	return res
}
func (as *AuditService) DeleteAuditFile(u *model.AuditFile) error {
	return DB.Delete(u).Error
}

// Update 更新
func (as *AuditService) UpdateAuditFile(u *model.AuditFile) error {
	return DB.Model(u).Updates(u).Error
}

func (as *AuditService) BatchDeleteAuditConn(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.AuditConn{}).Error
}

func (as *AuditService) BatchDeleteAuditFile(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.AuditFile{}).Error
}

// === CE-M1-6: AuditEvent 系列 ===

// AuditEventList 分页查询统一审计事件;where 闭包内可叠加 kind / peer_id 过滤与排序。
func (as *AuditService) AuditEventList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.AuditEventList) {
	res = &model.AuditEventList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.AuditEvent{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.AuditEvents)
	return
}

// CreateAuditEvent 写入一条审计事件。
func (as *AuditService) CreateAuditEvent(u *model.AuditEvent) error {
	return DB.Create(u).Error
}

// EventInfoById 按主键取一条;未命中返回零值结构体(Id=0)。
func (as *AuditService) EventInfoById(id uint) (res *model.AuditEvent) {
	res = &model.AuditEvent{}
	DB.Where("id = ?", id).First(res)
	return
}

// DeleteAuditEvent 删除一条审计事件。
func (as *AuditService) DeleteAuditEvent(u *model.AuditEvent) error {
	return DB.Delete(u).Error
}

// BatchDeleteAuditEvent 批量按 id 删除。
func (as *AuditService) BatchDeleteAuditEvent(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.AuditEvent{}).Error
}
