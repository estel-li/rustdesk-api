package model

import (
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
)

// UserMfa 记录单个用户的 TOTP MFA 状态。
// 一个用户最多一条记录,表通过 user_id 唯一索引保证。
//   - Secret 落库前必须经过 utils.EncryptSecret(AES-256-GCM),库内不存明文。
//   - RecoveryCodes 是 JSON 数组,元素为 bcrypt(recovery_code) 字符串。
//   - EnabledAt 为 nil 表示尚未启用 MFA;非空表示 enroll 完成的时间戳。
//   - LastUsedAt 为 nil 表示从未使用过;后续 CE-M1-3 校验通过后写入。
type UserMfa struct {
	IdModel
	UserId        uint                  `json:"user_id"        gorm:"column:user_id;default:0;not null;uniqueIndex:uniq_user_mfa_user_id"`
	Secret        string                `json:"-"              gorm:"column:secret;type:varchar(512);default:'';not null"` // AES-GCM(base64) 后的 TOTP secret
	RecoveryCodes custom_types.AutoJson `json:"recovery_codes" gorm:"column:recovery_codes;type:text"`                     // JSON 数组,元素为 bcrypt(recovery_code) 字符串
	EnabledAt     *int64                `json:"enabled_at"     gorm:"column:enabled_at;default:null;index"`                // unix 秒;NULL 表示尚未启用
	LastUsedAt    *int64                `json:"last_used_at"   gorm:"column:last_used_at;default:null"`                    // unix 秒
	TimeModel
}

func (UserMfa) TableName() string { return "user_mfas" }

type UserMfaList struct {
	UserMfas []*UserMfa `json:"list,omitempty"`
	Pagination
}
