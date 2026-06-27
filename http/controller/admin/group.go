package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"strconv"
)

type Group struct {
}

// Detail 群组
// @Tags 群组
// @Summary 群组详情
// @Description 群组详情
// @Accept  json
// @Produce  json
// @Param id path int true "ID"
// @Success 200 {object} response.Response{data=model.Group}
// @Failure 500 {object} response.Response
// @Router /admin/group/detail/{id} [get]
// @Security token
func (ct *Group) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	u := service.AllService.GroupService.InfoById(uint(iid))
	if u.Id > 0 {
		response.Success(c, u)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
	return
}

// Create 创建群组
// @Tags 群组
// @Summary 创建群组
// @Description 创建群组
// @Accept  json
// @Produce  json
// @Param body body admin.GroupForm true "群组信息"
// @Success 200 {object} response.Response{data=model.Group}
// @Failure 500 {object} response.Response
// @Router /admin/group/create [post]
// @Security token
func (ct *Group) Create(c *gin.Context) {
	f := &admin.GroupForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToGroup()
	err := service.AllService.GroupService.Create(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// List 列表
// @Tags 群组
// @Summary 群组列表
// @Description 群组列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Success 200 {object} response.Response{data=model.GroupList}
// @Failure 500 {object} response.Response
// @Router /admin/group/list [get]
// @Security token
func (ct *Group) List(c *gin.Context) {
	query := &admin.PageQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.GroupService.List(query.Page, query.PageSize, nil)
	response.Success(c, res)
}

// Update 编辑
// @Tags 群组
// @Summary 群组编辑
// @Description 群组编辑
// @Accept  json
// @Produce  json
// @Param body body admin.GroupForm true "群组信息"
// @Success 200 {object} response.Response{data=model.Group}
// @Failure 500 {object} response.Response
// @Router /admin/group/update [post]
// @Security token
func (ct *Group) Update(c *gin.Context) {
	f := &admin.GroupForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToGroup()
	err := service.AllService.GroupService.Update(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 删除
// @Tags 群组
// @Summary 群组删除
// @Description 群组删除
// @Accept  json
// @Produce  json
// @Param body body admin.GroupForm true "群组信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/group/delete [post]
// @Security token
func (ct *Group) Delete(c *gin.Context) {
	f := &admin.GroupForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.Id
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.GroupService.InfoById(f.Id)
	if u.Id > 0 {
		err := service.AllService.GroupService.Delete(u)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// SetMfaRequired CE-M1-5 切换组级强制 MFA 开关。
// @Tags 群组
// @Summary 切换组级强制 MFA
// @Description 管理员切换某群组的强制 MFA 开关;组内所有用户的有效策略 = group.mfa_required OR user.mfa_required。
// @Accept  json
// @Produce  json
// @Param body body admin.GroupMfaToggleForm true "切换信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/group/mfa/required [post]
// @Security token
func (ct *Group) SetMfaRequired(c *gin.Context) {
	f := &admin.GroupMfaToggleForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	if f.MfaRequired == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	g := service.AllService.GroupService.InfoById(f.GroupId)
	if g.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	opUser := service.AllService.UserService.CurUser(c)
	if err := service.AllService.GroupService.SetMfaRequired(g, *f.MfaRequired, opUser, c.ClientIP()); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}
