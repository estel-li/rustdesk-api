package service

import (
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"gorm.io/gorm"
)

type UserService struct {
}

// InfoById 根据用户id取用户信息
func (us *UserService) InfoById(id uint) *model.User {
	u := &model.User{}
	DB.Where("id = ?", id).First(u)
	return u
}

// InfoByUsername 根据用户名取用户信息
func (us *UserService) InfoByUsername(un string) *model.User {
	u := &model.User{}
	DB.Where("username = ?", un).First(u)
	return u
}

// InfoByEmail 根据邮箱取用户信息
func (us *UserService) InfoByEmail(email string) *model.User {
	u := &model.User{}
	DB.Where("email = ?", email).First(u)
	return u
}

// InfoByOpenid 根据openid取用户信息
func (us *UserService) InfoByOpenid(openid string) *model.User {
	u := &model.User{}
	DB.Where("openid = ?", openid).First(u)
	return u
}

// InfoByUsernamePassword 根据用户名密码取用户信息
func (us *UserService) InfoByUsernamePassword(username, password string) *model.User {
	if Config.Ldap.Enable {
		u, err := AllService.LdapService.Authenticate(username, password)
		if err == nil {
			return u
		}
		Logger.Errorf("LDAP authentication failed, %v", err)
		Logger.Warn("Fallback to local database")
	}
	u := &model.User{}
	DB.Where("username = ?", username).First(u)
	if u.Id == 0 {
		return u
	}
	ok, newHash, err := utils.VerifyPassword(u.Password, password)
	if err != nil || !ok {
		return &model.User{}
	}
	if newHash != "" {
		DB.Model(u).Update("password", newHash)
		u.Password = newHash
	}
	return u
}

// InfoByAccesstoken 根据accesstoken取用户信息
func (us *UserService) InfoByAccessToken(token string) (*model.User, *model.UserToken) {
	u := &model.User{}
	ut := &model.UserToken{}
	DB.Where("token = ?", token).First(ut)
	if ut.Id == 0 {
		return u, ut
	}
	if ut.ExpiredAt < time.Now().Unix() {
		return u, ut
	}
	DB.Where("id = ?", ut.UserId).First(u)
	return u, ut
}

// GenerateToken 生成token
func (us *UserService) GenerateToken(u *model.User) string {
	if len(Jwt.Key) > 0 {
		return Jwt.GenerateToken(u.Id)
	}
	return utils.Md5(u.Username + time.Now().String())
}

// Login 登录
func (us *UserService) Login(u *model.User, llog *model.LoginLog) *model.UserToken {
	token := us.GenerateToken(u)
	ut := &model.UserToken{
		UserId:     u.Id,
		Token:      token,
		DeviceUuid: llog.Uuid,
		DeviceId:   llog.DeviceId,
		ExpiredAt:  us.UserTokenExpireTimestamp(),
	}
	DB.Create(ut)
	llog.UserTokenId = ut.UserId
	DB.Create(llog)
	if llog.Uuid != "" {
		AllService.PeerService.UuidBindUserId(llog.DeviceId, llog.Uuid, u.Id)
	}
	return ut
}

// CurUser 获取当前用户
func (us *UserService) CurUser(c *gin.Context) *model.User {
	user, _ := c.Get("curUser")
	u, ok := user.(*model.User)
	if !ok {
		return nil
	}
	return u
}

func (us *UserService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.UserList) {
	res = &model.UserList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.User{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Users)
	return
}

func (us *UserService) ListByIds(ids []uint) (res []*model.User) {
	DB.Where("id in ?", ids).Find(&res)
	return res
}

// ListByGroupId 根据组id取用户列表
func (us *UserService) ListByGroupId(groupId, page, pageSize uint) (res *model.UserList) {
	res = us.List(page, pageSize, func(tx *gorm.DB) {
		tx.Where("group_id = ?", groupId)
	})
	return
}

// ListIdsByGroupId 根据组id取用户id列表
func (us *UserService) ListIdsByGroupId(groupId uint) (ids []uint) {
	DB.Model(&model.User{}).Where("group_id = ?", groupId).Pluck("id", &ids)
	return ids

}

// ListIdAndNameByGroupId 根据组id取用户id和用户名列表
func (us *UserService) ListIdAndNameByGroupId(groupId uint) (res []*model.User) {
	DB.Model(&model.User{}).Where("group_id = ?", groupId).Select("id, username").Find(&res)
	return res
}

// CheckUserEnable 判断用户是否禁用
func (us *UserService) CheckUserEnable(u *model.User) bool {
	return u.Status == model.COMMON_STATUS_ENABLE
}

// Create 创建
func (us *UserService) Create(u *model.User) error {
	// The initial username should be formatted, and the username should be unique
	if us.IsUsernameExists(u.Username) {
		return errors.New("UsernameExists")
	}
	u.Username = us.formatUsername(u.Username)
	var err error
	u.Password, err = utils.EncryptPassword(u.Password)
	if err != nil {
		return err
	}
	res := DB.Create(u).Error
	return res
}

// GetUuidByToken 根据token和user取uuid
func (us *UserService) GetUuidByToken(u *model.User, token string) string {
	ut := &model.UserToken{}
	err := DB.Where("user_id = ? and token = ?", u.Id, token).First(ut).Error
	if err != nil {
		return ""
	}
	return ut.DeviceUuid
}

// Logout 退出登录 -> 删除token, 解绑uuid
func (us *UserService) Logout(u *model.User, token string) error {
	uuid := us.GetUuidByToken(u, token)
	err := DB.Where("user_id = ? and token = ?", u.Id, token).Delete(&model.UserToken{}).Error
	if err != nil {
		return err
	}
	if uuid != "" {
		AllService.PeerService.UuidUnbindUserId(uuid, u.Id)
	}
	return nil
}

// Delete 删除用户和oauth信息
func (us *UserService) Delete(u *model.User) error {
	userCount := us.getAdminUserCount()
	if userCount <= 1 && us.IsAdmin(u) {
		return errors.New("The last admin user cannot be deleted")
	}
	tx := DB.Begin()
	// 删除用户
	if err := tx.Delete(u).Error; err != nil {
		tx.Rollback()
		return err
	}
	// 删除关联的 OAuth 信息
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.UserThird{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	//  删除关联的ab
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.AddressBook{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	//  删除关联的abc
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.AddressBookCollection{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	//  删除关联的abcr
	if err := tx.Where("user_id = ?", u.Id).Delete(&model.AddressBookCollectionRule{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	// 删除关联的peer
	if err := AllService.PeerService.EraseUserId(u.Id); err != nil {
		Logger.Warn("User deleted successfully, but failed to unlink peer.")
		return nil
	}
	return nil
}

// Update 更新
func (us *UserService) Update(u *model.User) error {
	currentUser := us.InfoById(u.Id)
	// 如果当前用户是管理员并且 IsAdmin 不为空，进行检查
	if us.IsAdmin(currentUser) {
		adminCount := us.getAdminUserCount()
		// 如果这是唯一的管理员，确保不能禁用或取消管理员权限
		if adminCount <= 1 && (!us.IsAdmin(u) || u.Status == model.COMMON_STATUS_DISABLED) {
			return errors.New("The last admin user cannot be disabled or demoted")
		}
	}
	// CE-M1-5:显式列更新,避免 GORM Updates 把 form 未携带的指针字段(mfa_required / is_admin)误置 false。
	// 与之前的 DB.Model(u).Updates(u) 等价的列集合在此显式列出;后续如需更新其它列请同步追加。
	return DB.Model(u).Select(
		"username", "email", "nickname", "avatar",
		"group_id", "is_admin", "status", "remark",
		"mfa_required",
	).Updates(u).Error
}

// FlushToken 清空token
func (us *UserService) FlushToken(u *model.User) error {
	return DB.Where("user_id = ?", u.Id).Delete(&model.UserToken{}).Error
}

// FlushTokenByUuid 清空token
func (us *UserService) FlushTokenByUuid(uuid string) error {
	return DB.Where("device_uuid = ?", uuid).Delete(&model.UserToken{}).Error
}

// FlushTokenByUuids 清空token
func (us *UserService) FlushTokenByUuids(uuids []string) error {
	return DB.Where("device_uuid in (?)", uuids).Delete(&model.UserToken{}).Error
}

// UpdatePassword 更新密码
func (us *UserService) UpdatePassword(u *model.User, password string) error {
	var err error
	u.Password, err = utils.EncryptPassword(password)
	if err != nil {
		return err
	}
	err = DB.Model(u).Update("password", u.Password).Error
	if err != nil {
		return err
	}
	err = us.FlushToken(u)
	return err
}

// IsAdmin 是否管理员
func (us *UserService) IsAdmin(u *model.User) bool {
	return u != nil && *u.IsAdmin
}

// RouteNames
func (us *UserService) RouteNames(u *model.User) []string {
	if us.IsAdmin(u) {
		return model.AdminRouteNames
	}
	return model.UserRouteNames
}

// InfoByOauthId 根据oauth的name和openId取用户信息
func (us *UserService) InfoByOauthId(op string, openId string) *model.User {
	ut := AllService.OauthService.UserThirdInfo(op, openId)
	if ut.Id == 0 {
		return nil
	}
	u := us.InfoById(ut.UserId)
	if u.Id == 0 {
		return nil
	}
	return u
}

// RegisterByOauth 注册
func (us *UserService) RegisterByOauth(oauthUser *model.OauthUser, op string) (error, *model.User) {
	Lock.Lock("registerByOauth")
	defer Lock.UnLock("registerByOauth")
	ut := AllService.OauthService.UserThirdInfo(op, oauthUser.OpenId)
	if ut.Id != 0 {
		return nil, us.InfoById(ut.UserId)
	}
	err, oauthType := AllService.OauthService.GetTypeByOp(op)
	if err != nil {
		return err, nil
	}
	//check if this email has been registered
	email := oauthUser.Email
	// only email is not empty
	if email != "" {
		email = strings.ToLower(email)
		// update email to oauthUser, in case it contain upper case
		oauthUser.Email = email
		// call this, if find user by email, it will update the email to local database
		user, ldapErr := AllService.LdapService.GetUserInfoByEmailLocal(email)
		// If we enable ldap, and the error is not ErrLdapUserNotFound, return the error because we could not sure if the user is not found in ldap
		if !(errors.Is(ldapErr, ErrLdapNotEnabled) || errors.Is(ldapErr, ErrLdapUserNotFound) || ldapErr == nil) {
			return ldapErr, user
		}
		if user.Id == 0 {
			// this means the user is not found in ldap, maybe ldao is not enabled
			user = us.InfoByEmail(email)
		}
		if user.Id != 0 {
			ut.FromOauthUser(user.Id, oauthUser, oauthType, op)
			DB.Create(ut)
			return nil, user
		}
	}

	tx := DB.Begin()
	ut = &model.UserThird{}
	ut.FromOauthUser(0, oauthUser, oauthType, op)
	// The initial username should be formatted
	username := us.formatUsername(oauthUser.Username)
	usernameUnique := us.GenerateUsernameByOauth(username)
	user := &model.User{
		Username: usernameUnique,
		GroupId:  1,
	}
	oauthUser.ToUser(user, false)
	tx.Create(user)
	if user.Id == 0 {
		tx.Rollback()
		return errors.New("OauthRegisterFailed"), user
	}
	ut.UserId = user.Id
	tx.Create(ut)
	tx.Commit()
	return nil, user
}

// GenerateUsernameByOauth 生成用户名
func (us *UserService) GenerateUsernameByOauth(name string) string {
	for us.IsUsernameExists(name) {
		name += strconv.Itoa(rand.Intn(10)) // Append a random digit (0-9)
	}
	return name
}

// UserThirdsByUserId
func (us *UserService) UserThirdsByUserId(userId uint) (res []*model.UserThird) {
	DB.Where("user_id = ?", userId).Find(&res)
	return res
}

func (us *UserService) UserThirdInfo(userId uint, op string) *model.UserThird {
	ut := &model.UserThird{}
	DB.Where("user_id = ? and op = ?", userId, op).First(ut)
	return ut
}

// FindLatestUserIdFromLoginLogByUuid 根据uuid和设备id查找最后登录的用户id
func (us *UserService) FindLatestUserIdFromLoginLogByUuid(uuid string, deviceId string) uint {
	llog := &model.LoginLog{}
	DB.Where("uuid = ? and device_id = ?", uuid, deviceId).Order("id desc").First(llog)
	return llog.UserId
}

// IsPasswordEmptyById 根据用户id判断密码是否为空，主要用于第三方登录的自动注册
func (us *UserService) IsPasswordEmptyById(id uint) bool {
	u := &model.User{}
	if DB.Where("id = ?", id).First(u).Error != nil {
		return false
	}
	return u.Password == ""
}

// IsPasswordEmptyByUsername 根据用户id判断密码是否为空，主要用于第三方登录的自动注册
func (us *UserService) IsPasswordEmptyByUsername(username string) bool {
	u := &model.User{}
	if DB.Where("username = ?", username).First(u).Error != nil {
		return false
	}
	return u.Password == ""
}

// IsPasswordEmptyByUser 判断密码是否为空，主要用于第三方登录的自动注册
func (us *UserService) IsPasswordEmptyByUser(u *model.User) bool {
	return us.IsPasswordEmptyById(u.Id)
}

// Register 注册, 如果用户名已存在则返回nil
func (us *UserService) Register(username string, email string, password string, status model.StatusCode) *model.User {
	u := &model.User{
		Username: username,
		Email:    email,
		Password: password,
		GroupId:  1,
		Status:   status,
	}
	err := us.Create(u)
	if err != nil {
		return nil
	}
	return u
}

func (us *UserService) TokenList(page uint, size uint, f func(tx *gorm.DB)) *model.UserTokenList {
	res := &model.UserTokenList{}
	res.Page = int64(page)
	res.PageSize = int64(size)
	tx := DB.Model(&model.UserToken{})
	if f != nil {
		f(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, size))
	tx.Find(&res.UserTokens)
	return res
}

func (us *UserService) TokenInfoById(id uint) *model.UserToken {
	ut := &model.UserToken{}
	DB.Where("id = ?", id).First(ut)
	return ut
}

func (us *UserService) DeleteToken(l *model.UserToken) error {
	return DB.Delete(l).Error
}

// Helper functions, used for formatting username
func (us *UserService) formatUsername(username string) string {
	username = strings.ReplaceAll(username, " ", "")
	username = strings.ToLower(username)
	return username
}

// Helper functions, getUserCount
func (us *UserService) getUserCount() int64 {
	var count int64
	DB.Model(&model.User{}).Count(&count)
	return count
}

// helper functions, getAdminUserCount
func (us *UserService) getAdminUserCount() int64 {
	var count int64
	DB.Model(&model.User{}).Where("is_admin = ?", true).Count(&count)
	return count
}

// UserTokenExpireTimestamp 生成用户token过期时间
func (us *UserService) UserTokenExpireTimestamp() int64 {
	exp := Config.App.TokenExpire
	if exp == 0 {
		//默认七天
		exp = 604800
	}
	return time.Now().Add(exp).Unix()
}

func (us *UserService) RefreshAccessToken(ut *model.UserToken) {
	ut.ExpiredAt = us.UserTokenExpireTimestamp()
	DB.Model(ut).Update("expired_at", ut.ExpiredAt)
}

func (us *UserService) AutoRefreshAccessToken(ut *model.UserToken) {
	if ut.ExpiredAt-time.Now().Unix() < Config.App.TokenExpire.Milliseconds()/3000 {
		us.RefreshAccessToken(ut)
	}
}

func (us *UserService) BatchDeleteUserToken(ids []uint) error {
	return DB.Where("id in ?", ids).Delete(&model.UserToken{}).Error
}

func (us *UserService) VerifyJWT(token string) (uint, error) {
	return Jwt.ParseToken(token)
}

// IsUsernameExists 判断用户名是否存在, it will check the internal database and LDAP(if enabled)
func (us *UserService) IsUsernameExists(username string) bool {
	return us.IsUsernameExistsLocal(username) || AllService.LdapService.IsUsernameExists(username)
}

func (us *UserService) IsUsernameExistsLocal(username string) bool {
	u := &model.User{}
	DB.Where("username = ?", username).First(u)
	return u.Id != 0
}

func (us *UserService) IsEmailExistsLdap(email string) bool {
	return AllService.LdapService.IsEmailExists(email)
}

// EffectiveMfaRequired CE-M1-5 计算"有效强制 MFA 策略" = user.MfaRequired || group.MfaRequired。
// 任一指针为 nil 视为 false,组关系缺失同样视为 false,不 panic。
func (us *UserService) EffectiveMfaRequired(u *model.User) bool {
	if u == nil {
		return false
	}
	if u.MfaRequired != nil && *u.MfaRequired {
		return true
	}
	if u.GroupId == 0 {
		return false
	}
	g := AllService.GroupService.InfoById(u.GroupId)
	if g == nil || g.Id == 0 || g.MfaRequired == nil {
		return false
	}
	return *g.MfaRequired
}

// SetMfaRequired CE-M1-5 管理员切换用户级强制 MFA 开关,同步写审计日志。
// 不会触碰 user_mfa 行;若 required=false,已 enroll 的用户依然保留二步登录。
func (us *UserService) SetMfaRequired(target *model.User, required bool, opUser *model.User, ip string) error {
	if target == nil || target.Id == 0 {
		return errors.New("UserNotFound")
	}
	v := required
	if err := DB.Model(&model.User{}).Where("id = ?", target.Id).
		Update("mfa_required", &v).Error; err != nil {
		return err
	}
	target.MfaRequired = &v
	action := model.LoginLogTypeMfaRequiredSet
	if !required {
		action = model.LoginLogTypeMfaRequiredUnset
	}
	us.writeMfaAudit(opUser, target, action, ip)
	return nil
}

// DisableMfa CE-M1-5 管理员强制关闭目标用户 MFA:删除 user_mfa 行 + 置 mfa_required=false,
// 同步落审计。Disable 操作即便清除 secret 后,如组级 mfa_required=true 仍会触发下次登录的 enroll。
//
// 入参 reason 仅写入 Logger,审计行不持久化 reason(LoginLog 表无该列);CE-M1-6 落地后可迁移到 AuditEvent。
func (us *UserService) DisableMfa(target *model.User, opUser *model.User, ip, reason string) error {
	if target == nil || target.Id == 0 {
		return errors.New("UserNotFound")
	}
	// 1) 删除 user_mfa 行(若不存在则忽略 ErrMfaNotEnrolled,继续把 mfa_required 复位)。
	if err := AllService.MfaService.Disable(target.Id); err != nil && !errors.Is(err, ErrMfaNotEnrolled) {
		return err
	}
	// 2) 把 mfa_required 复位 false,避免立即再次触发 enroll(组级仍生效)。
	v := false
	if err := DB.Model(&model.User{}).Where("id = ?", target.Id).
		Update("mfa_required", &v).Error; err != nil {
		return err
	}
	target.MfaRequired = &v
	us.writeMfaAudit(opUser, target, model.LoginLogTypeMfaDisabledByAdmin, ip)
	if reason != "" {
		Logger.Infof("mfa disabled by admin op=%d target=%d reason=%q", safeUserId(opUser), target.Id, reason)
	}
	return nil
}

// writeMfaAudit 把 MFA 相关高敏操作写到 login_logs;失败仅 Logger.Error,不阻塞业务路径。
// 参考 service/user.go Login 中 DB.Create(&model.LoginLog{...}) 的写法。
// 约定:UserId 写"被操作账号 id",方便 LoginLog 列表按目标账户筛选;操作人 id 落到 Remark/Platform 这类
// 空闲列不合适,这里把 op.Username 暂塞 Client 列(LoginLog.Client 现有值为 webadmin/webclient/app,
// 强制 MFA 审计场景下复用为 "admin:<username>" 字面量,CE-M1-6 落地后会迁出到 AuditEvent)。
func (us *UserService) writeMfaAudit(opUser, target *model.User, action, ip string) {
	if target == nil {
		return
	}
	client := "admin"
	if opUser != nil && opUser.Username != "" {
		client = "admin:" + opUser.Username
	}
	row := &model.LoginLog{
		UserId: target.Id,
		Client: client,
		Ip:     ip,
		Type:   action,
	}
	if err := DB.Create(row).Error; err != nil {
		// 审计落盘失败必须升级为 Logger.Error,但不阻塞业务路径(由调用方决定是否 5xx)。
		Logger.Errorf("write mfa audit fail action=%s target=%d ip=%s err=%v", action, target.Id, ip, err)
	}
}

// safeUserId 在 opUser 为 nil 时返回 0,避免日志 format panic。
func safeUserId(u *model.User) uint {
	if u == nil {
		return 0
	}
	return u.Id
}
