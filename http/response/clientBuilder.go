package response

// ClientBuilderBuildResponse Build 接口的成功响应。
// QrPngBase64 不包含 data: 前缀,前端按需拼 "data:image/png;base64," + QrPngBase64。
type ClientBuilderBuildResponse struct {
	Token        string `json:"token"`
	Filename     string `json:"filename"`
	DownloadUrl  string `json:"download_url"`
	LandingUrl   string `json:"landing_url"`
	QrPngBase64  string `json:"qr_png_base64"`
	ExpiresAt    string `json:"expires_at"`     // RFC3339
	ArtifactId   uint   `json:"artifact_id"`
	CachedReused bool   `json:"cached_reused"` // 是否复用了同四元组缓存
}

// ClientBuilderArtifactItem List 列表单项。
type ClientBuilderArtifactItem struct {
	Id          uint   `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Sha256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	Version     string `json:"version"`
	Active      int    `json:"active"`
	CreatedBy   uint   `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpstreamUrl string `json:"upstream_url"`
}
