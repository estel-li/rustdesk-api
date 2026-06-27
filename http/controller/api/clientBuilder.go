package api

import (
	"errors"
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// ClientBuilder 公开下载控制器(token 一次性短期有效)。
//
// 这些端点不走鉴权中间件:下载凭证就是 URL 里的 token,失败/过期分别 404/410。
type ClientBuilder struct{}

// Download 流式输出基础 EXE,设置 Content-Disposition 把文件名改成
// Configuration String 形态(filename*=UTF-8''<encoded>)。
// GET /api/client-builder/download/:token
func (cb *ClientBuilder) Download(c *gin.Context) {
	token := c.Param("token")
	payload, err := service.AllService.ClientBuilderService.Resolve(token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTokenExpired):
			c.Status(http.StatusGone)
		default:
			c.Status(http.StatusNotFound)
		}
		return
	}
	art, err := service.AllService.ClientBuilderService.LocateArtifact(payload)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", service.ContentDispositionFilename(payload.Filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(art.LocalPath)
	// 不记录 key/api 等字段;只记 token 前缀 + artifact id。
	if global.Logger != nil {
		short := token
		if len(short) > 8 {
			short = short[:8]
		}
		global.Logger.Infof("[client-builder] download artifact=%d token=%s", payload.ArtifactId, short)
	}
}

// QR 返回 download_url 的二维码 PNG。
// GET /api/client-builder/qr/:token
func (cb *ClientBuilder) QR(c *gin.Context) {
	token := c.Param("token")
	payload, err := service.AllService.ClientBuilderService.Resolve(token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTokenExpired):
			c.Status(http.StatusGone)
		default:
			c.Status(http.StatusNotFound)
		}
		return
	}
	_ = payload
	downloadUrl := service.AllService.ClientBuilderService.DownloadURL(token)
	png, err := service.AllService.ClientBuilderService.QRPng(downloadUrl)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

// 简洁的 Landing 模板,host/key/api/relay 全部走 html/template 自动转义。
var clientBuilderLandingTpl = template.Must(template.New("cb-landing").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>RustDesk Client Download</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;max-width:560px;margin:40px auto;padding:0 16px;color:#222}
h1{font-size:1.4rem;margin-bottom:8px}
.field{margin:6px 0;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;word-break:break-all}
.btn{display:inline-block;margin-top:16px;padding:10px 18px;background:#1d6cf2;color:#fff;border-radius:6px;text-decoration:none}
.qr{display:block;margin:16px 0;border:1px solid #eee;padding:8px;background:#fff;width:240px;height:240px}
small{color:#888}
</style>
</head>
<body>
<h1>RustDesk Custom Client</h1>
<p>This download wraps your server configuration into the file name. Run it on Windows and RustDesk will pick up the settings on first launch.</p>
<div class="field"><b>id-server:</b> {{.Host}}</div>
{{if .Relay}}<div class="field"><b>relay-server:</b> {{.Relay}}</div>{{end}}
{{if .Api}}<div class="field"><b>api-server:</b> {{.Api}}</div>{{end}}
{{if .Key}}<div class="field"><b>key:</b> {{.Key}}</div>{{end}}
<img class="qr" alt="QR" src="{{.QRUrl}}">
<a class="btn" href="{{.DownloadUrl}}">Download {{.Filename}}</a>
<p><small>Link expires at {{.ExpiresAt}}.</small></p>
</body>
</html>`))

// Landing 简单 HTML 页:展示四元组明文 + 下载按钮 + 二维码。
// GET /api/client-builder/landing/:token
func (cb *ClientBuilder) Landing(c *gin.Context) {
	token := c.Param("token")
	payload, err := service.AllService.ClientBuilderService.Resolve(token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTokenExpired):
			c.Status(http.StatusGone)
		default:
			c.Status(http.StatusNotFound)
		}
		return
	}
	data := struct {
		Host        string
		Relay       string
		Api         string
		Key         string
		QRUrl       string
		DownloadUrl string
		Filename    string
		ExpiresAt   string
	}{
		Host:        payload.Host,
		Relay:       payload.Relay,
		Api:         payload.Api,
		Key:         payload.Key,
		QRUrl:       service.AllService.ClientBuilderService.QRURL(token),
		DownloadUrl: service.AllService.ClientBuilderService.DownloadURL(token),
		Filename:    payload.Filename,
		ExpiresAt:   formatUnixToRFC3339(payload.ExpiresAt),
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := clientBuilderLandingTpl.Execute(c.Writer, data); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
}

func formatUnixToRFC3339(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format("2006-01-02T15:04:05Z")
}
