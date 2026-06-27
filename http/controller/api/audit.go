package api

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"time"
)

type Audit struct {
}

// AuditConn
// @Tags 审计
// @Summary 审计连接
// @Description 审计连接
// @Accept  json
// @Produce  json
// @Param body body request.AuditConnForm true "审计连接"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/conn [post]
func (a *Audit) AuditConn(c *gin.Context) {
	af := &request.AuditConnForm{}
	err := c.ShouldBindBodyWith(af, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	/*ttt := &gin.H{}
	c.ShouldBindBodyWith(ttt, binding.JSON)
	fmt.Println(ttt)*/
	ac := af.ToAuditConn()
	if af.Action == model.AuditActionNew {
		service.AllService.AuditService.CreateAuditConn(ac)
	} else if af.Action == model.AuditActionClose {
		ex := service.AllService.AuditService.InfoByPeerIdAndConnId(af.Id, af.ConnId)
		if ex.Id != 0 {
			ex.CloseTime = time.Now().Unix()
			service.AllService.AuditService.UpdateAuditConn(ex)
		}
	} else if af.Action == "" {
		ex := service.AllService.AuditService.InfoByPeerIdAndConnId(af.Id, af.ConnId)
		if ex.Id != 0 {
			up := &model.AuditConn{
				IdModel:   model.IdModel{Id: ex.Id},
				FromPeer:  ac.FromPeer,
				FromName:  ac.FromName,
				SessionId: ac.SessionId,
				Type:      ac.Type,
			}
			service.AllService.AuditService.UpdateAuditConn(up)
		}
	}
	response.Success(c, "")
}

// AuditFile
// @Tags 审计
// @Summary 审计文件
// @Description 审计文件
// @Accept  json
// @Produce  json
// @Param body body request.AuditFileForm true "审计文件"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/file [post]
func (a *Audit) AuditFile(c *gin.Context) {
	aff := &request.AuditFileForm{}
	err := c.ShouldBindBodyWith(aff, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	//ttt := &gin.H{}
	//c.ShouldBindBodyWith(ttt, binding.JSON)
	//fmt.Println(ttt)
	af := aff.ToAuditFile()
	service.AllService.AuditService.CreateAuditFile(af)
	response.Success(c, "")
}

// AuditEvent CE-M1-6: 统一审计事件入口。
// 客户端先做哈希/截断,服务端仅做白名单 + 16KB 长度校验,不解析 payload_json 内容。
// payload_json must not contain raw clipboard/file content; client must hash & truncate.
// @Tags 审计
// @Summary 审计事件
// @Description 统一审计事件入口(剪贴板/告警/命令/录像);payload_json 由客户端哈希截断,服务端不解析。
// @Accept  json
// @Produce  json
// @Param body body request.AuditEventForm true "审计事件"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/event [post]
func (a *Audit) AuditEvent(c *gin.Context) {
	aef := &request.AuditEventForm{}
	if err := c.ShouldBindBodyWith(aef, binding.JSON); err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if msg := aef.Validate(); msg != "" {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+msg)
		return
	}
	ev := aef.ToAuditEvent()
	if err := service.AllService.AuditService.CreateAuditEvent(ev); err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, "")
}
