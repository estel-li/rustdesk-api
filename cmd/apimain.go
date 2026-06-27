package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http"
	"github.com/lejianwen/rustdesk-api/v2/http/metrics"
	"github.com/lejianwen/rustdesk-api/v2/lib/cache"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/lib/lock"
	"github.com/lejianwen/rustdesk-api/v2/lib/logger"
	"github.com/lejianwen/rustdesk-api/v2/lib/orm"
	"github.com/lejianwen/rustdesk-api/v2/lib/upload"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const DatabaseVersion = 269

// @title 管理系统API
// @version 1.0
// @description 接口
// @basePath /api
// @securityDefinitions.apikey token
// @in header
// @name api-token
// @securitydefinitions.apikey BearerAuth
// @in header
// @name Authorization

var rootCmd = &cobra.Command{
	Use:   "apimain",
	Short: "RUSTDESK API SERVER",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		InitGlobal()
	},
	Run: func(cmd *cobra.Command, args []string) {
		global.Logger.Info("API SERVER START")
		http.ApiInit()
	},
}

var resetPwdCmd = &cobra.Command{
	Use:     "reset-admin-pwd [pwd]",
	Example: "reset-admin-pwd 123456",
	Short:   "Reset Admin Password",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pwd := args[0]
		admin := service.AllService.UserService.InfoById(1)
		if admin.Id == 0 {
			global.Logger.Warn("user not found! ")
			return
		}
		err := service.AllService.UserService.UpdatePassword(admin, pwd)
		if err != nil {
			global.Logger.Error("reset password fail! ", err)
			return
		}
		global.Logger.Info("reset password success! ")
	},
}
var resetUserPwdCmd = &cobra.Command{
	Use:     "reset-pwd [userId] [pwd]",
	Example: "reset-pwd 2 123456",
	Short:   "Reset User Password",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		userId := args[0]
		pwd := args[1]
		uid, err := strconv.Atoi(userId)
		if err != nil {
			global.Logger.Warn("userId must be int!")
			return
		}
		if uid <= 0 {
			global.Logger.Warn("userId must be greater than 0! ")
			return
		}
		u := service.AllService.UserService.InfoById(uint(uid))
		if u.Id == 0 {
			global.Logger.Warn("user not found! ")
			return
		}
		err = service.AllService.UserService.UpdatePassword(u, pwd)
		if err != nil {
			global.Logger.Warn("reset password fail! ", err)
			return
		}
		global.Logger.Info("reset password success!")
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&global.ConfigPath, "config", "c", "./conf/config.yaml", "choose config file")
	rootCmd.AddCommand(resetPwdCmd, resetUserPwdCmd)
}
func main() {
	if err := rootCmd.Execute(); err != nil {
		global.Logger.Error(err)
		os.Exit(1)
	}
}

func InitGlobal() {
	//配置解析
	global.Viper = config.Init(&global.Config, global.ConfigPath)

	//日志
	global.Logger = logger.New(&logger.Config{
		Path:         global.Config.Logger.Path,
		Level:        global.Config.Logger.Level,
		ReportCaller: global.Config.Logger.ReportCaller,
	})

	global.InitI18n()

	//redis
	global.Redis = redis.NewClient(&redis.Options{
		Addr:     global.Config.Redis.Addr,
		Password: global.Config.Redis.Password,
		DB:       global.Config.Redis.Db,
	})
	// Redis healthcheck:失败仅 Warn,不退出进程(默认 SQLite + 内存缓存部署可正常启动)
	pingRedisClient(global.Redis, global.Logger)

	//cache
	global.Cache = initCacheWithFallback(&global.Config, global.Logger)
	//gorm
	if global.Config.Gorm.Type == config.TypeMysql {

		dsn := fmt.Sprintf("%s:%s@(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s",
			global.Config.Mysql.Username,
			global.Config.Mysql.Password,
			global.Config.Mysql.Addr,
			global.Config.Mysql.Dbname,
			global.Config.Mysql.Tls,
		)

		global.DB = orm.NewMysql(&orm.MysqlConfig{
			Dsn:          dsn,
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	} else if global.Config.Gorm.Type == config.TypePostgresql {
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			global.Config.Postgresql.Host,
			global.Config.Postgresql.Port,
			global.Config.Postgresql.User,
			global.Config.Postgresql.Password,
			global.Config.Postgresql.Dbname,
			global.Config.Postgresql.Sslmode,
			global.Config.Postgresql.TimeZone,
		)
		global.DB = orm.NewPostgresql(&orm.PostgresqlConfig{
			Dsn:          dsn,
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	} else {
		//sqlite
		global.DB = orm.NewSqlite(&orm.SqliteConfig{
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	}

	//validator
	global.ApiInitValidator()

	//oss
	global.Oss = &upload.Oss{
		AccessKeyId:     global.Config.Oss.AccessKeyId,
		AccessKeySecret: global.Config.Oss.AccessKeySecret,
		Host:            global.Config.Oss.Host,
		CallbackUrl:     global.Config.Oss.CallbackUrl,
		ExpireTime:      global.Config.Oss.ExpireTime,
		MaxByte:         global.Config.Oss.MaxByte,
	}

	//jwt
	//fmt.Println(global.Config.Jwt.PrivateKey)
	global.Jwt = jwt.NewJwt(global.Config.Jwt.Key, global.Config.Jwt.ExpireDuration)
	//locker
	global.Lock = lock.NewLocal()

	//service
	service.New(&global.Config, global.DB, global.Logger, global.Jwt, global.Lock)

	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{
		CaptchaThreshold: global.Config.App.CaptchaThreshold,
		BanThreshold:     global.Config.App.BanThreshold,
		AttemptsWindow:   10 * time.Minute,
		BanDuration:      30 * time.Minute,
	})
	global.LoginLimiter.RegisterProvider(utils.B64StringCaptchaProvider{})
	DatabaseAutoUpdate()
}

func DatabaseAutoUpdate() {
	version := DatabaseVersion

	db := global.DB

	if global.Config.Gorm.Type == config.TypeMysql {
		//检查存不存在数据库，不存在则创建
		dbName := db.Migrator().CurrentDatabase()
		if dbName == "" {
			dbName = global.Config.Mysql.Dbname
			// 移除 DSN 中的数据库名称，以便初始连接时不指定数据库
			dsnWithoutDB := fmt.Sprintf("%s:%s@(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
				global.Config.Mysql.Username,
				global.Config.Mysql.Password,
				global.Config.Mysql.Addr,
				"",
			)

			//新链接
			dbWithoutDB := orm.NewMysql(&orm.MysqlConfig{
				Dsn: dsnWithoutDB,
			}, global.Logger)
			// 获取底层的 *sql.DB 对象，并确保在程序退出时关闭连接
			sqlDBWithoutDB, err := dbWithoutDB.DB()
			if err != nil {
				global.Logger.Errorf("获取底层 *sql.DB 对象失败: %v", err)
				return
			}
			defer func() {
				if err := sqlDBWithoutDB.Close(); err != nil {
					global.Logger.Errorf("关闭连接失败: %v", err)
				}
			}()

			err = dbWithoutDB.Exec("CREATE DATABASE IF NOT EXISTS " + dbName + " DEFAULT CHARSET utf8mb4").Error
			if err != nil {
				global.Logger.Error(err)
				return
			}
		}
	}

	if !db.Migrator().HasTable(&model.Version{}) {
		Migrate(uint(version))
	} else {
		//查找最后一个version
		var v model.Version
		db.Last(&v)
		if v.Version < uint(version) {
			Migrate(uint(version))
		}

		// 245迁移
		if v.Version < 245 {
			//oauths 表的 oauth_type 字段设置为 op同样的值
			db.Exec("update oauths set oauth_type = op")
			db.Exec("update oauths set issuer = 'https://accounts.google.com' where op = 'google'")
			db.Exec("update user_thirds set oauth_type = third_type, op = third_type")
			//通过email迁移旧的google授权
			uts := make([]model.UserThird, 0)
			db.Where("oauth_type = ?", "google").Find(&uts)
			for _, ut := range uts {
				if ut.UserId > 0 {
					db.Model(&model.User{}).Where("id = ?", ut.UserId).Update("email", ut.OpenId)
				}
			}
		}
		if v.Version < 246 {
			db.Exec("update oauths set issuer = 'https://accounts.google.com' where op = 'google' and issuer is null")
		}
		if v.Version < 266 {
			// CE-M1-1: user_mfas 表由 AutoMigrate 创建,无历史数据回填。
			// 保留 hook 供后续 CE-M1-2/3 追加(如默认 recovery code 生成、字段补齐等)。
		}
		if v.Version < 267 {
			// CE-M1-5: users / groups 表追加 mfa_required 列(默认 0 = 不强制)。
			// AutoMigrate 在 PostgreSQL / MySQL 下能识别新字段并追加列,SQLite 在新建表时也能添加;
			// 但对存量库,旧 AutoMigrate 不会回填索引,这里手写一遍兜底,保证升级幂等。
			switch global.Config.Gorm.Type {
			case config.TypeMysql:
				_ = db.Exec("ALTER TABLE users ADD COLUMN mfa_required TINYINT(1) NOT NULL DEFAULT 0").Error
				_ = db.Exec("ALTER TABLE `groups` ADD COLUMN mfa_required TINYINT(1) NOT NULL DEFAULT 0").Error
				_ = db.Exec("CREATE INDEX idx_users_mfa_required ON users(mfa_required)").Error
				_ = db.Exec("CREATE INDEX idx_groups_mfa_required ON `groups`(mfa_required)").Error
			case config.TypePostgresql:
				_ = db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_required BOOLEAN NOT NULL DEFAULT FALSE").Error
				_ = db.Exec("ALTER TABLE groups ADD COLUMN IF NOT EXISTS mfa_required BOOLEAN NOT NULL DEFAULT FALSE").Error
				_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_users_mfa_required ON users(mfa_required)").Error
				_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_groups_mfa_required ON groups(mfa_required)").Error
			default:
				// sqlite:ADD COLUMN 重复执行会报错,先用 PRAGMA 嗅探,缺列再补。
				if !db.Migrator().HasColumn(&model.User{}, "mfa_required") {
					_ = db.Exec("ALTER TABLE users ADD COLUMN mfa_required INTEGER NOT NULL DEFAULT 0").Error
				}
				if !db.Migrator().HasColumn(&model.Group{}, "mfa_required") {
					_ = db.Exec("ALTER TABLE `groups` ADD COLUMN mfa_required INTEGER NOT NULL DEFAULT 0").Error
				}
				_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_users_mfa_required ON users(mfa_required)").Error
				_ = db.Exec("CREATE INDEX IF NOT EXISTS idx_groups_mfa_required ON `groups`(mfa_required)").Error
			}
		}
		if v.Version < 268 {
			// CE-M1-6: 新增 audit_events 表 + (kind, created_at) 复合索引。
			// 表本身由上方 Migrate() 中的 AutoMigrate 在版本升级时建出;此处只补复合索引,
			// 兼容旧库已经存在表但缺索引的情形。
			if err := db.AutoMigrate(&model.AuditEvent{}); err != nil {
				global.Logger.Warn("CE-M1-6 audit_events automigrate: ", err)
			}
			ensureAuditEventCompositeIndex()
		}
		if v.Version < 269 {
			// CE-M1-9: 新增 client_builder_artifacts 表(轻量 Client Builder 元数据)。
			// AutoMigrate 已在 Migrate() 中处理首建;此处对老库兜底,保证升级幂等。
			if err := db.AutoMigrate(&model.ClientBuilderArtifact{}); err != nil {
				global.Logger.Warn("CE-M1-9 client_builder_artifacts automigrate: ", err)
			}
		}
	}

}

// pingRedisClient 显式校验 redis 客户端连通性,失败仅 Warn,不退出。
// 满足 ai-development-plan.md:174 "redis 不可达不得让默认部署崩溃" 的硬性约束。
func pingRedisClient(client *redis.Client, log *logrus.Logger) {
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		if log != nil {
			log.Warnf("redis ping failed: %v (continuing without redis)", err)
		}
	}
}

// initCacheWithFallback 根据 cfg.Cache.Type 构造缓存后端;redis 不可达时降级到内存缓存。
// 同步把当前后端类型写入 metrics gauge,供 /metrics 排障使用。
//
// 行为变更:
//   - cfg.Cache.Type == "" 或未知值时,旧逻辑会让 global.Cache 维持 nil,后续 .Get / .Set 直接 panic;
//     新逻辑统一走 cache.New(...) 落到 memory 后端。
func initCacheWithFallback(cfg *config.Config, log *logrus.Logger) cache.Handler {
	typ := cfg.Cache.Type
	switch typ {
	case cache.TypeFile:
		fc := cache.NewFileCache()
		fc.SetDir(cfg.Cache.FileDir)
		metrics.SetCacheBackend(cache.TypeFile)
		return fc
	case cache.TypeRedis:
		rc := cache.NewRedis(&redis.Options{
			Addr:     cfg.Cache.RedisAddr,
			Password: cfg.Cache.RedisPwd,
			DB:       cfg.Cache.RedisDb,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := rc.Ping(ctx); err != nil {
			metrics.IncCachePingFailure(cache.TypeRedis)
			if log != nil {
				log.Warnf("redis cache ping failed: %v; fallback to memory cache", err)
			}
			metrics.SetCacheBackend(cache.TypeMem)
			return cache.NewMemoryCache(0)
		}
		metrics.SetCacheBackend(cache.TypeRedis)
		return rc
	default:
		// 空串 / "memory" / 未知值都走内存,修复历史 nil 隐患
		if typ != "" && typ != cache.TypeMem {
			if log != nil {
				log.Warnf("unknown cache.type %q, falling back to memory", typ)
			}
		}
		metrics.SetCacheBackend(cache.TypeMem)
		return cache.NewMemoryCache(0)
	}
}

func Migrate(version uint) {
	global.Logger.Info("Migrating....", version)
	err := global.DB.AutoMigrate(
		&model.Version{},
		&model.User{},
		&model.UserToken{},
		&model.Tag{},
		&model.AddressBook{},
		&model.Peer{},
		&model.Group{},
		&model.UserThird{},
		&model.Oauth{},
		&model.LoginLog{},
		&model.ShareRecord{},
		&model.AuditConn{},
		&model.AuditFile{},
		&model.AddressBookCollection{},
		&model.AddressBookCollectionRule{},
		&model.ServerCmd{},
		&model.DeviceGroup{},
		&model.UserMfa{},
		&model.AuditEvent{},
		&model.ClientBuilderArtifact{},
	)
	if err != nil {
		global.Logger.Error("migrate err :=>", err)
	}
	// CE-M1-6: AutoMigrate 对复合索引支持不稳,这里显式建一次 (kind, created_at)。
	ensureAuditEventCompositeIndex()
	global.DB.Create(&model.Version{Version: version})
	//如果是初次则创建一个默认用户
	var vc int64
	global.DB.Model(&model.Version{}).Count(&vc)
	if vc == 1 {
		localizer := global.Localizer("")
		defaultGroup, _ := localizer.LocalizeMessage(&i18n.Message{
			ID: "DefaultGroup",
		})
		group := &model.Group{
			Name: defaultGroup,
			Type: model.GroupTypeDefault,
		}
		service.AllService.GroupService.Create(group)

		shareGroup, _ := localizer.LocalizeMessage(&i18n.Message{
			ID: "ShareGroup",
		})
		groupShare := &model.Group{
			Name: shareGroup,
			Type: model.GroupTypeShare,
		}
		service.AllService.GroupService.Create(groupShare)
		//是true
		is_admin := true
		admin := &model.User{
			Username: "admin",
			Nickname: "Admin",
			Status:   model.COMMON_STATUS_ENABLE,
			IsAdmin:  &is_admin,
			GroupId:  1,
		}

		// 生成随机密码
		pwd := utils.RandomString(8)
		global.Logger.Info("Admin Password Is: ", pwd)
		var err error
		admin.Password, err = utils.EncryptPassword(pwd)
		if err != nil {
			global.Logger.Fatalf("failed to generate admin password: %v", err)
		}
		global.DB.Create(admin)
	}

}

// ensureAuditEventCompositeIndex CE-M1-6: 显式建立 audit_events(kind, created_at) 复合索引。
// SQLite / PostgreSQL 支持 CREATE INDEX IF NOT EXISTS;MySQL 不支持,需先用 SHOW INDEX 嗅探。
func ensureAuditEventCompositeIndex() {
	if global.DB == nil {
		return
	}
	switch global.Config.Gorm.Type {
	case config.TypeMysql:
		// MySQL 不支持 IF NOT EXISTS,直接执行并忽略 1061 (duplicate key) 类错误。
		_ = global.DB.Exec("CREATE INDEX idx_audit_event_kind_created ON audit_events(kind, created_at)").Error
	default:
		// SQLite / PostgreSQL
		if err := global.DB.Exec("CREATE INDEX IF NOT EXISTS idx_audit_event_kind_created ON audit_events(kind, created_at)").Error; err != nil {
			global.Logger.Warn("ensureAuditEventCompositeIndex: ", err)
		}
	}
}
