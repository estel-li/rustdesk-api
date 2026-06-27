package metrics

import (
	"context"
	stdhttp "net/http"
	"sync"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// Registry 是 rustdesk-api 进程级 Prometheus 注册表单例。
// 业务侧应通过 Register() 注册自定义 collector,避免污染全局 DefaultRegisterer。
var (
	Registry = prometheus.NewRegistry()

	// HTTPRequestsTotal /HTTPRequestDuration 由 middleware.Metrics 使用,定义在此处以便复用同一注册表。
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rustdesk_api_http_requests_total",
			Help: "Total HTTP requests processed, partitioned by method, path template, and status code.",
		},
		[]string{"method", "path", "status"},
	)
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rustdesk_api_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds, partitioned by method and path template.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	// CacheBackendGauge 标识当前 cache 后端 (memory/file/redis)。
	CacheBackendGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rustdesk_api_cache_backend",
			Help: "Current rustdesk-api cache backend in use (1 for active backend).",
		},
		[]string{"type"},
	)
	// CachePingFailuresTotal redis/外部缓存 healthcheck 失败计数。
	CachePingFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rustdesk_api_cache_ping_failures_total",
			Help: "Number of cache backend Ping() failures observed at startup or runtime.",
		},
		[]string{"backend"},
	)

	startOnce sync.Once
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		HTTPRequestsTotal,
		HTTPRequestDuration,
		CacheBackendGauge,
		CachePingFailuresTotal,
	)
}

// Register 暴露给业务层向 metrics 注册表追加 collector;重复注册返回错误。
func Register(c prometheus.Collector) error {
	return Registry.Register(c)
}

// SetCacheBackend 在缓存后端确定后调用,将 gauge 翻到对应 label。
func SetCacheBackend(backend string) {
	CacheBackendGauge.Reset()
	CacheBackendGauge.WithLabelValues(backend).Set(1)
}

// IncCachePingFailure 在 healthcheck 失败时累加计数。
func IncCachePingFailure(backend string) {
	CachePingFailuresTotal.WithLabelValues(backend).Inc()
}

// StartServer 在独立端口暴露 /metrics。
// 失败仅写日志,不 panic,以兼容默认 SQLite + 无 metrics 端口可用的退化场景。
// 多次调用是幂等的(sync.Once 保护)。
func StartServer(cfg config.Metrics, logger *logrus.Logger) {
	if !cfg.Enable {
		if logger != nil {
			logger.Info("metrics server disabled by config")
		}
		return
	}
	bind, path := normalize(cfg)

	startOnce.Do(func() {
		mux := stdhttp.NewServeMux()
		mux.Handle(path, promhttp.HandlerFor(Registry, promhttp.HandlerOpts{Registry: Registry}))
		srv := &stdhttp.Server{
			Addr:              bind,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			if logger != nil {
				logger.Infof("metrics server listening on %s%s", bind, path)
			}
			if err := srv.ListenAndServe(); err != nil && err != stdhttp.ErrServerClosed {
				if logger != nil {
					logger.Errorf("metrics server exited: %v", err)
				}
			}
		}()
	})
}

func normalize(cfg config.Metrics) (string, string) {
	bind := cfg.Bind
	if bind == "" {
		bind = config.DefaultMetricsBind
	}
	path := cfg.Path
	if path == "" {
		path = config.DefaultMetricsPath
	}
	return bind, path
}

// StartServerForTest 是测试辅助,绕开 sync.Once 并返回可关闭的 server。
// 仅供 *_test.go 调用。
func StartServerForTest(cfg config.Metrics) (*stdhttp.Server, error) {
	if !cfg.Enable {
		return nil, nil
	}
	bind, path := normalize(cfg)
	mux := stdhttp.NewServeMux()
	mux.Handle(path, promhttp.HandlerFor(Registry, promhttp.HandlerOpts{Registry: Registry}))
	srv := &stdhttp.Server{
		Addr:              bind,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- srv.ListenAndServe()
	}()
	select {
	case err := <-listenErr:
		return nil, err
	case <-time.After(80 * time.Millisecond):
		return srv, nil
	}
}

// ShutdownForTest 测试用关闭。
func ShutdownForTest(srv *stdhttp.Server) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
