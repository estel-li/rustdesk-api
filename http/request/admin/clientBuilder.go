package admin

// ClientBuilderBuildReq Build 接口请求体(POST /api/admin/client_builder/build)。
// 字段命名与 rustdesk/src/custom_server.rs 解析约定保持一致(host/key/api/relay)。
// host 必填;其余三项可空,空字段在最终文件名中整段省略。
type ClientBuilderBuildReq struct {
	ArtifactId  uint   `json:"artifact_id"  validate:"required,gt=0" label:"基础 EXE id"`
	IdServer    string `json:"id_server"    validate:"required,max=255" label:"id-server"`
	RelayServer string `json:"relay_server" validate:"max=255" label:"relay-server"`
	ApiServer   string `json:"api_server"   validate:"max=512" label:"api-server"`
	Key         string `json:"key"          validate:"max=512" label:"key"`
}

// ClientBuilderUploadBaseReq 上传/登记一份基础 EXE。
// source=upload 时使用 multipart `file` 字段;source=upstream 时使用 upstream_url。
type ClientBuilderUploadBaseReq struct {
	Source      string `form:"source"        validate:"required,oneof=upload upstream"`
	UpstreamUrl string `form:"upstream_url"  validate:"max=512"`
	Sha256      string `form:"sha256"        validate:"required,len=64,hexadecimal"`
	Version     string `form:"version"       validate:"max=32"`
	Name        string `form:"name"          validate:"max=128"`
}

// ClientBuilderDeleteBaseReq 删除基础 EXE。
type ClientBuilderDeleteBaseReq struct {
	Id uint `json:"id" validate:"required,gt=0"`
}
