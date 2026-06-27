package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	apiResp "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
)

type Login struct {
}

// Login 登录
// @Tags 登录
// @Summary 登录
// @Description 登录(支持两步登录,启用 MFA 的用户返回 tfa_check + ticket,需调用 /api/login-mfa 完成二次校验)
// @Accept  json
// @Produce  json
// @Param body body api.LoginForm true "登录表单"
// @Success 200 {object} apiResp.LoginRes
// @Failure 500 {object} response.ErrorResponse
// @Router /login [post]
func (l *Login) Login(c *gin.Context) {
	if global.Config.App.DisablePwdLogin {
		response.Error(c, response.TranslateMsg(c, "PwdLoginDisabled"))
		return
	}

	// 检查登录限制
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()

	f := &api.LoginForm{}
	err := c.ShouldBindJSON(f)
	//fmt.Println(f)
	if err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), c.ClientIP()))
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "ParamsError", c.RemoteIP(), c.ClientIP()))
		response.Error(c, errList[0])
		return
	}

	u := service.AllService.UserService.InfoByUsernamePassword(f.Username, f.Password)

	if u.Id == 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s", "UsernameOrPasswordError", c.RemoteIP(), c.ClientIP()))
		response.Error(c, response.TranslateMsg(c, "UsernameOrPasswordError"))
		return
	}

	if !service.AllService.UserService.CheckUserEnable(u) {
		response.Error(c, response.TranslateMsg(c, "UserDisabled"))
		return
	}

	//根据refer判断是webclient还是app
	ref := c.GetHeader("referer")
	if ref != "" {
		f.DeviceInfo.Type = model.LoginLogClientWeb
	}

	// CE-M1-3 两步登录:密码校验通过后判断 MFA 是否启用;启用则签发 ticket,等待 /api/login-mfa 二次校验。
	if service.AllService.MfaService.IsEnrolled(u.Id) {
		// 密码已正确,清掉先前累计的失败次数(与 admin/login 流程一致),避免 MFA 阶段被误 ban。
		loginLimiter.RemoveAttempts(clientIp)
		ticket, _, err := service.AllService.MfaTicketService.Issue(u.Id, clientIp, f.Id)
		if err != nil {
			global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s err=%v", "MfaTicketIssue", c.RemoteIP(), clientIp, err))
			response.Error(c, response.TranslateMsg(c, "SystemError"))
			return
		}
		c.JSON(http.StatusOK, apiResp.LoginRes{
			Type:        "tfa_check",
			TfaType:     "totp",
			MfaRequired: true,
			Ticket:      ticket,
		})
		return
	}

	// CE-M1-5 强制 MFA:用户尚未 enroll 但策略命中。
	// 默认 ForceEnrollOnRequired=true,签发 enroll-purpose ticket,客户端走 /api/mfa/enroll-then-verify。
	// 若关闭该开关,则直接拒绝登录,提示用户联系管理员关闭强制位或线下完成 enroll。
	if service.AllService.UserService.EffectiveMfaRequired(u) {
		loginLimiter.RemoveAttempts(clientIp)
		forceEnroll := true
		if global.Config.Mfa.ForceEnrollOnRequired != nil {
			forceEnroll = *global.Config.Mfa.ForceEnrollOnRequired
		}
		if !forceEnroll {
			global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s uid=%d", "MfaEnrollRequired", clientIp, u.Id))
			response.Fail(c, 113, response.TranslateMsg(c, "MfaEnrollRequired"))
			return
		}
		ticket, _, err := service.AllService.MfaTicketService.IssueWithPurpose(
			u.Id, clientIp, f.Id, "enroll",
			service.AllService.MfaTicketService.EffectiveEnrollTicketTTL(),
		)
		if err != nil {
			global.Logger.Warn(fmt.Sprintf("Login Fail: %s %s %s err=%v", "MfaEnrollTicketIssue", c.RemoteIP(), clientIp, err))
			response.Error(c, response.TranslateMsg(c, "SystemError"))
			return
		}
		// 同步落审计,便于追踪强制 enroll 触发频次。审计写盘异常仅 Warn,不阻塞登录路径。
		go func(uid uint, ip string) {
			defer func() { _ = recover() }()
			row := &model.LoginLog{UserId: uid, Ip: ip, Type: model.LoginLogTypeMfaEnrollForced}
			if err := global.DB.Create(row).Error; err != nil {
				global.Logger.Warnf("write mfa_enroll_forced audit fail uid=%d err=%v", uid, err)
			}
		}(u.Id, clientIp)
		c.JSON(http.StatusOK, apiResp.LoginRes{
			Type:           "tfa_check",
			TfaType:        "totp",
			MfaRequired:    true,
			EnrollRequired: true,
			Ticket:         ticket,
		})
		return
	}

	ut := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId:   u.Id,
		Client:   f.DeviceInfo.Type,
		DeviceId: f.Id,
		Uuid:     f.Uuid,
		Ip:       c.ClientIP(),
		Type:     model.LoginLogTypeAccount,
		Platform: f.DeviceInfo.Os,
	})

	c.JSON(http.StatusOK, apiResp.LoginRes{
		AccessToken: ut.Token,
		Type:        "access_token",
		User:        *(&apiResp.UserPayload{}).FromUser(u),
	})
}

// LoginMfa 两步登录第二步,校验 ticket + TOTP / recovery code。
// @Tags 登录
// @Summary 两步登录-MFA 校验
// @Description 携带首步签发的 ticket 与 TOTP / recovery code 完成二次校验;成功返回与 /api/login 一致的 access_token。
// @Accept  json
// @Produce  json
// @Param body body api.LoginMfaForm true "MFA 校验表单"
// @Success 200 {object} apiResp.LoginRes
// @Failure 400 {object} response.ErrorResponse
// @Router /login-mfa [post]
func (l *Login) LoginMfa(c *gin.Context) {
	loginLimiter := global.LoginLimiter
	clientIp := c.ClientIP()

	f := &api.LoginMfaForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("LoginMfa Fail: %s %s %s", "ParamsError", c.RemoteIP(), clientIp))
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("LoginMfa Fail: %s %s %s", "ParamsError", c.RemoteIP(), clientIp))
		response.Error(c, errList[0])
		return
	}

	// 1) 校验 ticket。错误统一返回 MfaTicketInvalid,避免泄漏内部状态,过期单独提示便于客户端引导。
	claims, err := service.AllService.MfaTicketService.Verify(f.Ticket, clientIp)
	if err != nil {
		loginLimiter.RecordFailedAttempt(clientIp)
		global.Logger.Warn(fmt.Sprintf("LoginMfa Fail: %s %s %s err=%v", "TicketInvalid", c.RemoteIP(), clientIp, err))
		if errors.Is(err, service.ErrTicketExpired) {
			response.Error(c, response.TranslateMsg(c, "MfaTicketExpired"))
			return
		}
		response.Error(c, response.TranslateMsg(c, "MfaTicketInvalid"))
		return
	}

	// 2) 校验 code:type=recovery 走一次性恢复码,默认走 TOTP。失败计入 limiter + ticket 内 attempt。
	mfaType := f.Type
	if mfaType == "" {
		mfaType = "totp"
	}
	var (
		ok     bool
		verErr error
	)
	if mfaType == "recovery" {
		ok, verErr = service.AllService.MfaService.ConsumeRecoveryCode(claims.UID, f.Code)
	} else {
		ok, verErr = service.AllService.MfaService.Verify(claims.UID, f.Code)
	}
	if verErr != nil || !ok {
		loginLimiter.RecordFailedAttempt(clientIp)
		_, exceed := service.AllService.MfaTicketService.IncAttempt(claims)
		if exceed {
			// 超过单 ticket 错误上限,立即作废 nonce,后续即使猜对也无法重用同一 ticket。
			service.AllService.MfaTicketService.Consume(claims)
		}
		global.Logger.Warn(fmt.Sprintf("LoginMfa Fail: %s %s %s uid=%d err=%v", "CodeError", c.RemoteIP(), clientIp, claims.UID, verErr))
		response.Error(c, response.TranslateMsg(c, "MfaCodeError"))
		return
	}

	// 3) ticket 一旦成功消费即作废,杜绝并发提交带来的重放。
	service.AllService.MfaTicketService.Consume(claims)

	u := service.AllService.UserService.InfoById(claims.UID)
	if u == nil || u.Id == 0 {
		response.Error(c, response.TranslateMsg(c, "UsernameOrPasswordError"))
		return
	}
	if !service.AllService.UserService.CheckUserEnable(u) {
		response.Error(c, response.TranslateMsg(c, "UserDisabled"))
		return
	}

	// 根据 referer 推断设备类型,与首步 Login 保持一致。
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
		AccessToken: ut.Token,
		Type:        "access_token",
		User:        *(&apiResp.UserPayload{}).FromUser(u),
	})
}

// LoginOptions
// @Tags 登录
// @Summary 登录选项
// @Description 登录选项
// @Accept  json
// @Produce  json
// @Success 200 {object} []string
// @Failure 500 {object} response.ErrorResponse
// @Router /login-options [get]
func (l *Login) LoginOptions(c *gin.Context) {
	ops := service.AllService.OauthService.GetOauthProviders()
	if global.Config.App.WebSso {
		ops = append(ops, model.OauthTypeWebauth)
	}
	var oidcItems []map[string]string
	for _, v := range ops {
		oidcItems = append(oidcItems, map[string]string{"name": v})
	}
	common, err := json.Marshal(oidcItems)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "SystemError")+err.Error())
		return
	}
	var res []string
	res = append(res, "common-oidc/"+string(common))
	for _, v := range ops {
		res = append(res, "oidc/"+v)
	}
	c.JSON(http.StatusOK, res)
}

// Logout
// @Tags 登录
// @Summary 登出
// @Description 登出
// @Accept  json
// @Produce  json
// @Success 200 {string} string
// @Failure 500 {object} response.ErrorResponse
// @Router /logout [post]
func (l *Login) Logout(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	token, ok := c.Get("token")
	if ok {
		service.AllService.UserService.Logout(u, token.(string))
	}
	c.JSON(http.StatusOK, nil)

}
