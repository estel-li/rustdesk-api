package config

// Metrics Prometheus /metrics 服务配置
// 默认绑定在 loopback 上,不与业务 API (21114) 抢占。
type Metrics struct {
	Enable bool   `mapstructure:"enable"`
	Bind   string `mapstructure:"bind"`
	Path   string `mapstructure:"path"`
}

// 默认值,代码侧兜底,避免依赖 yaml 写出
const (
	DefaultMetricsBind = "127.0.0.1:21115"
	DefaultMetricsPath = "/metrics"
)

// Init 给空字段填充默认值,在 viper Unmarshal 后调用。
func (m *Metrics) Init() {
	if m.Bind == "" {
		m.Bind = DefaultMetricsBind
	}
	if m.Path == "" {
		m.Path = DefaultMetricsPath
	}
}
