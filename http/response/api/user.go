package api

import "github.com/lejianwen/rustdesk-api/v2/model"

/*
	pub enum UserStatus {
	    Disabled = 0,
	    Normal = 1,
	    Unverified = -1,
	}
*/

/*
UserPayload
String name = ”;
String email = ”;
String note = ”;
UserStatus status;
bool isAdmin = false;
*/
type UserPayload struct {
	Name    string                 `json:"name"`
	Email   string                 `json:"email"`
	Note    string                 `json:"note"`
	IsAdmin *bool                  `json:"is_admin"`
	Status  int                    `json:"status"`
	Info    map[string]interface{} `json:"info"`
}

func (up *UserPayload) FromUser(user *model.User) *UserPayload {
	up.Name = user.Username
	up.Email = user.Email
	up.IsAdmin = user.IsAdmin
	up.Status = int(user.Status)
	up.Info = map[string]interface{}{}
	return up
}

/*
	class HttpType {
	  static const kAuthReqTypeAccount = "account";
	  static const kAuthReqTypeMobile = "mobile";
	  static const kAuthReqTypeSMSCode = "sms_code";
	  static const kAuthReqTypeEmailCode = "email_code";
	  static const kAuthReqTypeTfaCode = "tfa_code";

	  static const kAuthResTypeToken = "access_token";
	  static const kAuthResTypeEmailCheck = "email_check";
	  static const kAuthResTypeTfaCheck = "tfa_check";
	}
*/
type LoginRes struct {
	Type        string      `json:"type"`
	AccessToken string      `json:"access_token,omitempty"`
	User        UserPayload `json:"user,omitempty"`
	Secret      string      `json:"secret,omitempty"`
	TfaType     string      `json:"tfa_type,omitempty"`
	// MfaRequired CE-M1-3 两步登录:首步密码正确且用户已启用 MFA 时为 true。
	MfaRequired bool `json:"mfa_required,omitempty"`
	// EnrollRequired CE-M1-5 强制 MFA 命中且账号尚未 enroll 时为 true;
	// 客户端应携 ticket 调用 /api/mfa/enroll-then-verify 完成扫码 + 校验。
	EnrollRequired bool `json:"enroll_required,omitempty"`
	// Ticket CE-M1-3 / CE-M1-5 两步登录:短期一次性 JWT,3~5 分钟内提交给 /api/login-mfa 或 /api/mfa/enroll-then-verify。
	Ticket string `json:"ticket,omitempty"`
	// RecoveryCodes CE-M1-5 enroll-then-verify 成功后一次性返回的恢复码明文,仅本次返回。
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
	// QrPng CE-M1-5 enroll-then-verify 第一轮返回的二维码 PNG 的 base64。
	QrPng string `json:"qr_png,omitempty"`
}
