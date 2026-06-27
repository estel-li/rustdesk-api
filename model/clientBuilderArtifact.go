package model

// ClientBuilderArtifact 描述一份"基础" RustDesk Windows portable EXE
// 元数据;真正的 EXE 字节保存在 client-builder.base-dir 目录下,文件名以
// sha256.exe 命名,数据库只保存路径与校验信息。
//
// CE-M1-9:轻量 Client Builder。本表只服务于"复制 + 改名"流程,不参与编译。
type ClientBuilderArtifact struct {
	IdModel
	Name        string `json:"name"         gorm:"size:128;not null;default:''"`     // 友好名,如 "rustdesk-1.4.2-portable"
	Source      string `json:"source"       gorm:"size:16;not null;default:'upload'"` // upload | upstream
	UpstreamUrl string `json:"upstream_url" gorm:"size:512;not null;default:''"`      // source=upstream 时存放 URL
	Sha256      string `json:"sha256"       gorm:"size:64;not null;default:'';index"` // 小写 hex,索引避免重复
	SizeBytes   int64  `json:"size_bytes"   gorm:"not null;default:0"`
	Version     string `json:"version"      gorm:"size:32;not null;default:''"` // 例 "1.4.2"
	LocalPath   string `json:"local_path"   gorm:"size:512;not null;default:''"`
	Active      int    `json:"active"       gorm:"not null;default:1;index"` // 1 启用 / 0 停用
	CreatedBy   uint   `json:"created_by"   gorm:"not null;default:0"`
	TimeModel
}

// TableName 覆盖默认表名(GORM 默认会复数化为 client_builder_artifacts,
// 这里显式声明,便于手写 SQL / 文档检索)。
func (*ClientBuilderArtifact) TableName() string {
	return "client_builder_artifacts"
}

// ClientBuilderArtifactList 分页列表(沿用其它 service 的 *List 命名风格)。
type ClientBuilderArtifactList struct {
	Pagination
	Artifacts []*ClientBuilderArtifact `json:"artifacts"`
}
