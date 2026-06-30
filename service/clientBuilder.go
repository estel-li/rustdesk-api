package service

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	barcode "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

// ClientBuilderService 实现 CE-M1-9:轻量 Client Builder。
//
// 关键边界:
//   - 不做编译、不签名、不改 PE 资源。仅将一份预上传的基础 EXE 按
//     RustDesk 客户端识别的 Configuration String 文件名复制(实际是流式重命名)
//     输出给浏览器。
//   - key 字段只走 global.Cache(memory/redis),绝不入库,绝不进入日志。
//   - 下载凭证 token = 32 byte crypto/rand,默认 TTL 7 天,仅承担"取数据"
//     的能力,不携带任何鉴权身份。
type ClientBuilderService struct{}

// TokenPayload 是缓存里 client_builder:token:<token> 的反序列化目标。
type TokenPayload struct {
	ArtifactId uint   `json:"artifact_id"`
	Filename   string `json:"filename"`
	Host       string `json:"host"`
	Key        string `json:"key"`
	Api        string `json:"api"`
	Relay      string `json:"relay"`
	CreatedBy  uint   `json:"created_by"`
	ExpiresAt  int64  `json:"expires_at"` // unix seconds
}

// BuildResult 是 Build 的返回值,后续由 controller 转换为 HTTP 响应。
type BuildResult struct {
	Token        string
	Filename     string
	ExpiresAt    time.Time
	Payload      *TokenPayload
	CachedReused bool
}

var (
	ErrClientBuilderDisabled  = errors.New("client builder disabled")
	ErrArtifactNotFound       = errors.New("artifact not found")
	ErrArtifactInactive       = errors.New("artifact inactive")
	ErrArtifactFileMissing    = errors.New("artifact file missing")
	ErrEmptyHost              = errors.New("host (id_server) required")
	ErrSha256Mismatch         = errors.New("sha256 mismatch")
	ErrFilenameTooLong        = errors.New("composed filename exceeds 240 chars")
	ErrTokenExpired           = errors.New("token expired")
	ErrTokenNotFound          = errors.New("token not found")
	ErrBaseExceedsMaxSize     = errors.New("base file exceeds max-base-mb")
	ErrUpstreamNotImplemented = errors.New("upstream source not implemented yet")
)

// cacheKey 命名空间。
const cacheKeyPrefix = "client_builder:token:"
const reuseKeyPrefix = "client_builder:reuse:"

// ---------------------------------------------------------------------------
// 文件名拼装
// ---------------------------------------------------------------------------

// BuildFilename 按 CE-M1-9 §4.4 拼接 EstelRemote-host=...,...exe。
// host 必填(否则返回 ErrEmptyHost);其余字段为空时整段省略,避免客户端把
// "key=" 这类空段当作有效配置。
func BuildFilename(host, key, api, relay string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", ErrEmptyHost
	}
	var b strings.Builder
	b.WriteString("EstelRemote-host=")
	b.WriteString(safeFieldEncode(host))
	if k := strings.TrimSpace(key); k != "" {
		b.WriteString(",key=")
		b.WriteString(safeFieldEncode(k))
	}
	if a := strings.TrimSpace(api); a != "" {
		b.WriteString(",api=")
		b.WriteString(safeFieldEncode(a))
	}
	if r := strings.TrimSpace(relay); r != "" {
		b.WriteString(",relay=")
		b.WriteString(safeFieldEncode(r))
	}
	b.WriteString(".exe")
	name := b.String()
	if len(name) > 240 {
		return "", ErrFilenameTooLong
	}
	return name, nil
}

// safeFieldEncode 对单个字段值做保守编码:
//   - 客户端 custom_server.rs:39 的解析逻辑不做 percent-decode,因此我们
//     原样保留 `:` `=` `/` `+` `.` `-` 这类在 host/api/key 中常见的字符;
//   - 只对会破坏文件名解析或文件系统的字符做替换:`,`(段分隔)、空白、
//     控制字符、Windows 文件名禁用字符(<>:"/\|?* 之中尤其要小心的是反斜杠
//     和正斜杠,但这两个在合法 host/api 中并不出现;若出现仍 percent-encode);
//   - 同时把非 ASCII printable 替换为 %XX,避免编码歧义。
func safeFieldEncode(s string) string {
	// 第一步:对 `,` 等强制替换。
	replacer := strings.NewReplacer(
		",", "%2C",
		" ", "%20",
		"\t", "%09",
		"\n", "%0A",
		"\r", "%0D",
		"<", "%3C",
		">", "%3E",
		"\"", "%22",
		"|", "%7C",
		"?", "%3F",
		"*", "%2A",
		"\\", "%5C",
	)
	s = replacer.Replace(s)
	// 第二步:扫一遍非 printable / 非 ASCII。
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c >= 0x7f {
			b.WriteString(fmt.Sprintf("%%%02X", c))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// ParseFilenameOracle 是 BuildFilename 的反函数;它实现等价于
// rustdesk/src/custom_server.rs:39 的解析逻辑(大小写不敏感前缀、`,` 分段),
// 仅在单测中使用,作为"客户端能正确解析"的 oracle。
func ParseFilenameOracle(filename string) (host, key, api, relay string, ok bool) {
	s := filename
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, ".exe.exe") {
		s = s[:len(s)-8]
		lower = strings.ToLower(s)
	} else if strings.HasSuffix(lower, ".exe") {
		s = s[:len(s)-4]
		lower = strings.ToLower(s)
	}
	idx := strings.Index(lower, "host=")
	if idx < 0 {
		return "", "", "", "", false
	}
	stripped := s[idx:]
	parts := strings.Split(stripped, ",")
	for _, el := range parts {
		l := strings.ToLower(el)
		switch {
		case strings.HasPrefix(l, "host="):
			host = el[5:]
		case strings.HasPrefix(l, "key="):
			key = el[4:]
		case strings.HasPrefix(l, "api="):
			api = el[4:]
		case strings.HasPrefix(l, "relay="):
			relay = el[6:]
		}
	}
	return host, key, api, relay, true
}

// ---------------------------------------------------------------------------
// 基础 EXE 增删查
// ---------------------------------------------------------------------------

// CreateBase 接收 multipart 上传或预下载(暂未实现 upstream)。
// localPath = <BaseDir>/<sha256>.exe。sha256 在落盘前以流式方式校验。
func (s *ClientBuilderService) CreateBase(
	src multipart.File, size int64,
	source, declaredSha256, version, name string,
	createdBy uint,
) (*model.ClientBuilderArtifact, error) {
	cfg := global.Config.ClientBuilder
	maxBytes := int64(cfg.MaxBaseMB) * 1024 * 1024
	if maxBytes > 0 && size > maxBytes {
		return nil, ErrBaseExceedsMaxSize
	}
	if err := os.MkdirAll(cfg.BaseDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir base-dir: %w", err)
	}
	// 流式 sha256 计算 + 临时写盘
	tmp, err := os.CreateTemp(cfg.BaseDir, ".upload-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	hasher := sha256.New()
	mw := io.MultiWriter(tmp, hasher)
	if _, err := io.Copy(mw, src); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("copy stream: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	actualSha := hex.EncodeToString(hasher.Sum(nil))
	if strings.ToLower(strings.TrimSpace(declaredSha256)) != actualSha {
		_ = os.Remove(tmpPath)
		return nil, ErrSha256Mismatch
	}
	finalPath := filepath.Join(cfg.BaseDir, actualSha+".exe")
	// 若已存在(同一份基础 EXE 之前传过),复用磁盘文件;DB 也按 sha 去重。
	if _, err := os.Stat(finalPath); err == nil {
		_ = os.Remove(tmpPath)
	} else {
		if err := os.Rename(tmpPath, finalPath); err != nil {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("rename to final: %w", err)
		}
	}
	// DB 去重:已有同 sha 且 active=1 则直接返回。
	existing := &model.ClientBuilderArtifact{}
	if err := DB.Where("sha256 = ? AND active = 1", actualSha).First(existing).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if name == "" {
		short := actualSha
		if len(short) > 8 {
			short = short[:8]
		}
		name = "rustdesk-portable-" + short
	}
	rec := &model.ClientBuilderArtifact{
		Name:      name,
		Source:    source,
		Sha256:    actualSha,
		SizeBytes: size,
		Version:   version,
		LocalPath: finalPath,
		Active:    1,
		CreatedBy: createdBy,
	}
	if err := DB.Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// ListBases 分页列表。
func (s *ClientBuilderService) ListBases(page, pageSize uint) *model.ClientBuilderArtifactList {
	res := &model.ClientBuilderArtifactList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.ClientBuilderArtifact{}).Where("active = 1").Order("id DESC")
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize)).Find(&res.Artifacts)
	return res
}

// Info 按 id 取记录,未找到返回 id=0 的零值。
func (s *ClientBuilderService) Info(id uint) *model.ClientBuilderArtifact {
	a := &model.ClientBuilderArtifact{}
	DB.Where("id = ?", id).First(a)
	return a
}

// DeleteBase 软删:Active=0,文件保留(因为可能仍有 token 引用)。
// 调用方需保证管理员鉴权。
func (s *ClientBuilderService) DeleteBase(id uint) error {
	return DB.Model(&model.ClientBuilderArtifact{}).Where("id = ?", id).
		Update("active", 0).Error
}

// ---------------------------------------------------------------------------
// Build:校验四元组、生成 token、缓存 payload
// ---------------------------------------------------------------------------

// Build 校验 artifact + 拼装文件名 + 签发短期 token。返回 BuildResult。
// 同四元组 + artifactId 在 TTL 内复用同一 token(减少缓存键膨胀)。
func (s *ClientBuilderService) Build(
	artifactId uint,
	host, key, api, relay string,
	createdBy uint,
) (*BuildResult, error) {
	if !global.Config.ClientBuilder.Enabled {
		return nil, ErrClientBuilderDisabled
	}
	art := s.Info(artifactId)
	if art == nil || art.Id == 0 {
		return nil, ErrArtifactNotFound
	}
	if art.Active != 1 {
		return nil, ErrArtifactInactive
	}
	if _, err := os.Stat(art.LocalPath); err != nil {
		return nil, ErrArtifactFileMissing
	}
	filename, err := BuildFilename(host, key, api, relay)
	if err != nil {
		return nil, err
	}
	ttl := global.Config.ClientBuilder.LinkTTLHours
	if ttl <= 0 {
		ttl = 168
	}
	ttlSec := ttl * 3600

	reuseKey := reuseKeyPrefix + reuseFingerprint(artifactId, host, key, api, relay)
	if global.Cache != nil {
		var cachedToken string
		if err := global.Cache.Get(reuseKey, &cachedToken); err == nil && cachedToken != "" {
			payload := &TokenPayload{}
			if err := global.Cache.Get(cacheKeyPrefix+cachedToken, payload); err == nil {
				if payload.ExpiresAt > time.Now().Unix() {
					return &BuildResult{
						Token:        cachedToken,
						Filename:     payload.Filename,
						ExpiresAt:    time.Unix(payload.ExpiresAt, 0).UTC(),
						Payload:      payload,
						CachedReused: true,
					}, nil
				}
			}
		}
	}
	token, err := newToken(32)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(time.Duration(ttlSec) * time.Second).UTC()
	payload := &TokenPayload{
		ArtifactId: artifactId,
		Filename:   filename,
		Host:       host,
		Key:        key,
		Api:        api,
		Relay:      relay,
		CreatedBy:  createdBy,
		ExpiresAt:  expiresAt.Unix(),
	}
	if global.Cache != nil {
		if err := global.Cache.Set(cacheKeyPrefix+token, payload, ttlSec); err != nil {
			return nil, err
		}
		// 用 reuse 索引把同四元组在 TTL 内收敛到同一个 token。
		_ = global.Cache.Set(reuseKey, token, ttlSec)
	}
	return &BuildResult{
		Token:     token,
		Filename:  filename,
		ExpiresAt: expiresAt,
		Payload:   payload,
	}, nil
}

// Resolve 从缓存里取 payload;过期或不存在时返回明确错误。
func (s *ClientBuilderService) Resolve(token string) (*TokenPayload, error) {
	if token == "" || global.Cache == nil {
		return nil, ErrTokenNotFound
	}
	payload := &TokenPayload{}
	if err := global.Cache.Get(cacheKeyPrefix+token, payload); err != nil {
		return nil, ErrTokenNotFound
	}
	if payload.ExpiresAt > 0 && payload.ExpiresAt <= time.Now().Unix() {
		return nil, ErrTokenExpired
	}
	return payload, nil
}

// LocateArtifact 取 token 对应的 artifact 实体并做路径穿越校验。
func (s *ClientBuilderService) LocateArtifact(payload *TokenPayload) (*model.ClientBuilderArtifact, error) {
	art := s.Info(payload.ArtifactId)
	if art == nil || art.Id == 0 {
		return nil, ErrArtifactNotFound
	}
	// 路径穿越兜底:art.LocalPath 必须在 BaseDir 之内。
	cleanBase, _ := filepath.Abs(filepath.Clean(global.Config.ClientBuilder.BaseDir))
	cleanPath, _ := filepath.Abs(filepath.Clean(art.LocalPath))
	if cleanBase != "" && !strings.HasPrefix(cleanPath, cleanBase) {
		return nil, ErrArtifactFileMissing
	}
	if _, err := os.Stat(art.LocalPath); err != nil {
		return nil, ErrArtifactFileMissing
	}
	return art, nil
}

// QRPng 生成 320×320 的二维码 PNG 字节。使用 boombuler/barcode(已通过
// pquerna/otp 引入为间接依赖),避免新增第三方包。
func (s *ClientBuilderService) QRPng(payload string) ([]byte, error) {
	bc, err := qr.Encode(payload, qr.M, qr.Auto)
	if err != nil {
		return nil, err
	}
	bc, err = barcode.Scale(bc, 320, 320)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, bc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PublicBaseURL 取对外可访问的 API base url(用于拼 download/landing/qr 链接)。
func (s *ClientBuilderService) PublicBaseURL() string {
	if u := strings.TrimRight(global.Config.ClientBuilder.PublicBaseUrl, "/"); u != "" {
		return u
	}
	return strings.TrimRight(global.Config.Rustdesk.ApiServer, "/")
}

// DownloadURL / LandingURL / QRURL 是组装下载链接的小帮手。
func (s *ClientBuilderService) DownloadURL(token string) string {
	return s.PublicBaseURL() + "/api/client-builder/download/" + token
}
func (s *ClientBuilderService) LandingURL(token string) string {
	return s.PublicBaseURL() + "/api/client-builder/landing/" + token
}
func (s *ClientBuilderService) QRURL(token string) string {
	return s.PublicBaseURL() + "/api/client-builder/qr/" + token
}

// ContentDispositionFilename 按 RFC 5987 生成 filename*=UTF-8''<encoded>,
// 避免 `,` `=` 这类被中间件当成 header 分隔符。
func ContentDispositionFilename(name string) string {
	encoded := url.PathEscape(name)
	return fmt.Sprintf("attachment; filename=\"download.exe\"; filename*=UTF-8''%s", encoded)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func reuseFingerprint(artifactId uint, host, key, api, relay string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%s|%s", artifactId, host, key, api, relay)
	return hex.EncodeToString(h.Sum(nil))
}
