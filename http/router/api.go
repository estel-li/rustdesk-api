package router

import (
	"github.com/gin-gonic/gin"
	_ "github.com/lejianwen/rustdesk-api/v2/docs/api"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/controller/api"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"
)

func ApiInit(g *gin.Engine) {

	//g.Use(middleware.Cors())
	//swagger
	if global.Config.App.ShowSwagger == 1 {
		g.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName("api")))
	}
	// 加载 HTML 模板
	g.LoadHTMLGlob("resources/templates/*")

	frg := g.Group("/api")

	{
		i := &api.Index{}
		frg.GET("/", i.Index)
		frg.GET("/version", i.Version)

		frg.POST("/heartbeat", i.Heartbeat)
	}

	{
		l := &api.Login{}
		// 如果返回oidc则可以通过oidc登录
		frg.GET("/login-options", l.LoginOptions)
		frg.POST("/login", l.Login)
		// CE-M1-3 两步登录第二步:必须放在 frg.Use(middleware.RustAuth()) 之前的公开组。
		frg.POST("/login-mfa", l.LoginMfa)

	}

	{
		// CE-M1-5 强制 MFA enroll + verify:接续 /api/login 返回的 enroll_required 分支。
		mfa := &api.Mfa{}
		frg.POST("/mfa/enroll-then-verify", mfa.EnrollThenVerify)
	}

	{
		o := &api.Oauth{}
		// [method:POST] [uri:/api/oidc/auth]
		frg.POST("/oidc/auth", o.OidcAuth)
		// [method:GET] [uri:/api/oidc/auth-query?code=abc&id=xxxxx&uuid=xxxxx]
		frg.GET("/oidc/auth-query", o.OidcAuthQuery)
		//api/oauth/callback
		frg.GET("/oauth/callback", o.OauthCallback)
		frg.GET("/oauth/login", o.OauthCallback)
		frg.GET("/oauth/msg", o.Message)

		frg.GET("/oidc/callback", o.OauthCallback)
		frg.GET("/oidc/login", o.OauthCallback)
		frg.GET("/oidc/msg", o.Message)
	}
	{
		pe := &api.Peer{}
		//提交系统信息
		frg.POST("/sysinfo", pe.SysInfo)
		frg.POST("/sysinfo_ver", pe.SysInfoVer)
	}

	if global.Config.App.WebClient == 1 {
		WebClientRoutes(frg)
	}

	{
		au := &api.Audit{}
		//[method:POST] [uri:/api/audit/conn]
		frg.POST("/audit/conn", au.AuditConn)
		//[method:POST] [uri:/api/audit/file]
		frg.POST("/audit/file", au.AuditFile)
		// CE-M1-6: 统一审计事件入口(剪贴板/告警/命令/录像);与 conn/file 同段,未鉴权。
		frg.POST("/audit/event", au.AuditEvent)
	}

	// CE-M1-9 轻量 Client Builder 公开端点:token 一次性短期凭证,不走鉴权
	// 中间件。enabled=false 时整组不注册,等价回滚。
	if global.Config.ClientBuilder.Enabled {
		cb := &api.ClientBuilder{}
		frg.GET("/client-builder/download/:token", cb.Download)
		frg.GET("/client-builder/landing/:token", cb.Landing)
		frg.GET("/client-builder/qr/:token", cb.QR)
	}

	frg.Use(middleware.RustAuth())
	{
		u := &api.User{}
		frg.GET("/user/info", u.Info)
		frg.POST("/currentUser", u.Info)
	}
	{
		l := &api.Login{}
		frg.POST("/logout", l.Logout)
	}
	{
		gr := &api.Group{}
		frg.GET("/users", gr.Users)
		frg.GET("/peers", gr.Peers)
		// /api/device-group/accessible?current=1&pageSize=100
		frg.GET("/device-group/accessible", gr.Device)
	}

	{
		ab := &api.Ab{}
		//获取地址
		frg.GET("/ab", ab.Ab)
		//更新地址
		frg.POST("/ab", ab.UpAb)
	}

	PersonalRoutes(frg)
	//访问静态文件
	g.StaticFS("/upload", http.Dir(global.Config.Gin.ResourcesPath+"/public/upload"))
}

func PersonalRoutes(frg *gin.RouterGroup) {
	{
		ab := &api.Ab{}
		frg.POST("/ab/personal", ab.Personal)
		//[method:POST] [uri:/api/ab/settings] Request
		frg.POST("/ab/settings", ab.Settings)
		// [method:POST] [uri:/api/ab/shared/profiles?current=1&pageSize=100]
		frg.POST("/ab/shared/profiles", ab.SharedProfiles)
		//[method:POST] [uri:/api/ab/peers?current=1&pageSize=100&ab=1]
		frg.POST("/ab/peers", ab.Peers)
		// [method:POST] [uri:/api/ab/tags/1]
		frg.POST("/ab/tags/:guid", ab.PTags)
		//[method:POST] api/ab/peer/add/1
		frg.POST("/ab/peer/add/:guid", ab.PeerAdd)
		//[method:DELETE] [uri:/api/ab/peer/1]
		frg.DELETE("/ab/peer/:guid", ab.PeerDel)
		//[method:PUT] [uri:/api/ab/peer/update/1]
		frg.PUT("/ab/peer/update/:guid", ab.PeerUpdate)
		//[method:POST] [uri:/api/ab/tag/add/1]
		frg.POST("/ab/tag/add/:guid", ab.TagAdd)
		//[method:PUT] [uri:/api/ab/tag/rename/1]
		frg.PUT("/ab/tag/rename/:guid", ab.TagRename)
		//[method:PUT] [uri:/api/ab/tag/update/1]
		frg.PUT("/ab/tag/update/:guid", ab.TagUpdate)
		//[method:DELETE] [uri:/api/ab/tag/1]
		frg.DELETE("/ab/tag/:guid", ab.TagDel)

	}

}

func WebClientRoutes(frg *gin.RouterGroup) {
	w := &api.WebClient{}
	{
		frg.POST("/shared-peer", w.SharedPeer)
	}
	{
		frg.POST("/server-config", middleware.RustAuth(), w.ServerConfig)
		frg.POST("/server-config-v2", middleware.RustAuth(), w.ServerConfigV2)
	}

}
