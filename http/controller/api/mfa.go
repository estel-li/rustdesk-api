package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	apiResp "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Mfa CE-M1-5 强制 MFA 当场 enroll + 校验:接续 /api/login 返回的 enroll_required 分支。
type Mfa struct {
}

// EnrollThenVerifyForm 两段式:
//
//	第一轮:body 只带 ticket,返回 { secret, qr_png } 让客户端展示二维码;
//	第二轮:body 带 ticket + code,服务端 verify 通过后落 enabled_at,生成 recovery_codes,签发正式 token。
type EnrollThenVerifyForm struct {
	Ticket string `json:"ticket" validate:"required,gte=10,lte=512"`
	Code   string `json:"code"   validate:"omitempty,gte=6,lte=8"`
}

// EnrollThenVerify CE-M1-5 强制 MFA 当场 enroll + verify 入口。
// @Tags 登录
// @Summary 强制 MFA enroll + verify
// @Description 携带 /api/login 返回的 enroll-purpose ticket:第一轮返回 secret/qr,第二轮带 code 完成激活并发 token。
// @Accept  json
// @Produce  json
// @Param body body api.EnrollThenVerifyForm true "enroll 表单"
// @Success 200 {object} apiResp.LoginRes
// @Failure 400 {object} response.ErrorResponse
// @Router /mfa/enroll-then-verify [post]
func (m *Mfa) EnrollThenVerify(c *gin.Context) {
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()

	f := &EnrollThenVerifyForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("EnrollThenVerify Fail: %s %s err=%v", "ParamsError", clientIp, err))
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if errList := global.Validator.ValidStruct(c, f); len(errList) > 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		response.Error(c, errList[0])
		return
	}

	// 1) 校验 ticket(签名 / 过期 / nonce / IP);purpose 必须为 enroll。
	claims, err := service.AllService.MfaTicketService.Verify(f.Ticket, clientIp)
	if err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("EnrollThenVerify Fail: %s %s err=%v", "TicketInvalid", clientIp, err))
		if errors.Is(err, service.ErrTicketExpired) {
			response.Error(c, response.TranslateMsg(c, "MfaTicketExpired"))
			return
		}
		response.Error(c, response.TranslateMsg(c, "MfaTicketInvalid"))
		return
	}
	if claims.Purpose != "enroll" {
		loginLimiter.RecordFailedAttempt(clientIp)
		response.Error(c, response.TranslateMsg(c, "MfaTicketInvalid"))
		return
	}

	u := service.AllService.UserService.InfoById(claims.UID)
	if u == nil || u.Id == 0 {
		response.Error(c, response.TranslateMsg(c, "UsernameOrPasswordError"))
		return
	}
	if !service.AllService.UserService.CheckUserEnable(u) {
		response.Error(c, response.TranslateMsg(c, "UserDisabled"))
		return
	}

	// 2) 第一轮:无 code → 返回 secret + qr_png(base64),保留 ticket 等待第二轮。
	if f.Code == "" {
		secret, qr, eErr := service.AllService.MfaService.Enroll(u.Id)
		if eErr != nil {
			// 已 enroll 但又触发强制 enroll 分支属反常,但理论存在(策略刚开 + 旧 secret 残留)。
			// 此时直接 401,引导客户端走 /api/login-mfa 普通流程。
			if errors.Is(eErr, service.ErrMfaAlreadyEnrolled) {
				response.Error(c, response.TranslateMsg(c, "MfaTicketInvalid"))
				return
			}
			global.Logger.Warn(fmt.Sprintf("EnrollThenVerify Fail: %s uid=%d err=%v", "Enroll", u.Id, eErr))
			response.Error(c, response.TranslateMsg(c, "SystemError"))
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, apiResp.LoginRes{
			Type:           "tfa_check",
			TfaType:        "totp",
			MfaRequired:    true,
			EnrollRequired: true,
			Secret:         secret,
			QrPng:          base64.StdEncoding.EncodeToString(qr),
			Ticket:         f.Ticket, // 复用原 ticket 进入第二轮,无需重新签发
		})
		return
	}

	// 3) 第二轮:验证 code。
	ok, vErr := service.AllService.MfaService.Verify(u.Id, f.Code)
	if vErr != nil || !ok {
		loginLimiter.RecordFailedAttempt(clientIp)
		_, exceed := service.AllService.MfaTicketService.IncAttempt(claims)
		if exceed {
			service.AllService.MfaTicketService.Consume(claims)
		}
		global.Logger.Warn(fmt.Sprintf("EnrollThenVerify Fail: %s uid=%d err=%v", "CodeError", u.Id, vErr))
		response.Error(c, response.TranslateMsg(c, "MfaCodeError"))
		return
	}

	// 4) 一次性生成 recovery codes(明文仅本次返回)。
	codes, rcErr := service.AllService.MfaService.GenerateRecoveryCodes(u.Id)
	if rcErr != nil {
		// 即便 recovery codes 失败也允许登录,但需告警以便修复。
		global.Logger.Warnf("EnrollThenVerify recovery_codes fail uid=%d err=%v", u.Id, rcErr)
	}

	// 5) 消费 ticket,发正式 token。
	service.AllService.MfaTicketService.Consume(claims)

	clientType := model.LoginLogClientWeb
	if c.GetHeader("referer") == "" {
		clientType = ""
	}
	ut := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId:   u.Id,
		Client:   clientType,
		DeviceId: claims.Device,
		Ip:       clientIp,
		Type:     model.LoginLogTypeAccount,
	})
	loginLimiter.RemoveAttempts(clientIp)

	c.JSON(http.StatusOK, apiResp.LoginRes{
		AccessToken:   ut.Token,
		Type:          "access_token",
		User:          *(&apiResp.UserPayload{}).FromUser(u),
		RecoveryCodes: codes,
	})
}
