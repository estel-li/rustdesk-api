package admin

import (
	"encoding/base64"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	adminreq "github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// ClientBuilder 后台轻量 Client Builder 控制器。
//
// CE-M1-9:不做编译/签名,仅生成"按 RustDesk Configuration String 文件名重命名"
// 的下载产物。
type ClientBuilder struct{}

// UploadBase 上传基础 EXE。
// POST /api/admin/client_builder/base/upload  (multipart/form-data)
func (cb *ClientBuilder) UploadBase(c *gin.Context) {
	if !global.Config.ClientBuilder.Enabled {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	req := &adminreq.ClientBuilderUploadBaseReq{}
	if err := c.ShouldBind(req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if errList := global.Validator.ValidStruct(c, req); len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	if req.Source == "upstream" {
		// 暂未实现:Pro 才有的"按 URL 抓"流程。
		response.Fail(c, 101, "upstream source not implemented yet")
		return
	}
	fh, err := c.FormFile("file")
	if err != nil || fh == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+"file required")
		return
	}
	maxMB := global.Config.ClientBuilder.MaxBaseMB
	if maxMB > 0 && fh.Size > int64(maxMB)*1024*1024 {
		response.Fail(c, 101, "file exceeds max-base-mb")
		return
	}
	src, err := fh.Open()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	defer src.Close()

	curUser := service.AllService.UserService.CurUser(c)
	var createdBy uint
	if curUser != nil {
		createdBy = curUser.Id
	}
	art, err := service.AllService.ClientBuilderService.CreateBase(
		src, fh.Size, "upload", req.Sha256, req.Version, req.Name, createdBy,
	)
	if err != nil {
		// 不要把 key 之类敏感字段塞日志;这里 err 不含敏感数据。
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{
		"id":         art.Id,
		"name":       art.Name,
		"sha256":     art.Sha256,
		"size_bytes": art.SizeBytes,
		"version":    art.Version,
		"active":     art.Active,
	})
}

// ListBase 列表。
// GET /api/admin/client_builder/base/list?page=1&page_size=20
func (cb *ClientBuilder) ListBase(c *gin.Context) {
	q := &adminreq.PageQuery{}
	if err := c.ShouldBindQuery(q); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	res := service.AllService.ClientBuilderService.ListBases(q.Page, q.PageSize)
	response.Success(c, res)
}

// DeleteBase 软删(Active=0)。
// POST /api/admin/client_builder/base/delete
func (cb *ClientBuilder) DeleteBase(c *gin.Context) {
	req := &adminreq.ClientBuilderDeleteBaseReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if err := service.AllService.ClientBuilderService.DeleteBase(req.Id); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, nil)
}

// Build 生成下载 token + 二维码。
// POST /api/admin/client_builder/build
func (cb *ClientBuilder) Build(c *gin.Context) {
	if !global.Config.ClientBuilder.Enabled {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	req := &adminreq.ClientBuilderBuildReq{}
	if err := c.ShouldBindJSON(req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if errList := global.Validator.ValidStruct(c, req); len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	curUser := service.AllService.UserService.CurUser(c)
	var createdBy uint
	if curUser != nil {
		createdBy = curUser.Id
	}
	result, err := service.AllService.ClientBuilderService.Build(
		req.ArtifactId, req.IdServer, req.Key, req.ApiServer, req.RelayServer, createdBy,
	)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	downloadUrl := service.AllService.ClientBuilderService.DownloadURL(result.Token)
	landingUrl := service.AllService.ClientBuilderService.LandingURL(result.Token)
	qrBytes, qrErr := service.AllService.ClientBuilderService.QRPng(downloadUrl)
	qrB64 := ""
	if qrErr == nil {
		qrB64 = base64.StdEncoding.EncodeToString(qrBytes)
	}
	resp := &response.ClientBuilderBuildResponse{
		Token:        result.Token,
		Filename:     result.Filename,
		DownloadUrl:  downloadUrl,
		LandingUrl:   landingUrl,
		QrPngBase64:  qrB64,
		ExpiresAt:    result.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		ArtifactId:   req.ArtifactId,
		CachedReused: result.CachedReused,
	}
	// 审计日志:只记 created_by + artifact_id + token 前 8 字符,
	// 永远不记录 key 明文。
	if global.Logger != nil {
		tokenShort := result.Token
		if len(tokenShort) > 8 {
			tokenShort = tokenShort[:8]
		}
		global.Logger.Infof("[client-builder] build user=%d artifact=%d token=%s reused=%v",
			createdBy, req.ArtifactId, tokenShort, result.CachedReused)
	}
	response.Success(c, resp)
}
