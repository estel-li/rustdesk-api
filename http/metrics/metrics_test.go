package metrics_test

import (
	"context"
	"fmt"
	"io"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/http/metrics"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

// TestMetricsHandler_ExposesGoMetrics 启动 metrics server,GET /metrics 应返回 200 + 含 go_goroutines 与 rustdesk_api_cache_backend。
func TestMetricsHandler_ExposesGoMetrics(t *testing.T) {
	port := freePort(t)
	bind := fmt.Sprintf("127.0.0.1:%d", port)
	cfg := config.Metrics{Enable: true, Bind: bind, Path: "/metrics"}

	srv, err := metrics.StartServerForTest(cfg)
	if err != nil {
		t.Fatalf("start metrics: %v", err)
	}
	defer metrics.ShutdownForTest(srv)

	if err := waitForPort(bind, 1*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	// 设置 cache backend gauge 以便有非零样本
	metrics.SetCacheBackend("memory")

	resp, err := stdhttp.Get("http://" + bind + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "go_goroutines") {
		t.Fatalf("expected go_goroutines in /metrics body")
	}
	if !strings.Contains(bodyStr, "rustdesk_api_cache_backend") {
		t.Fatalf("expected rustdesk_api_cache_backend in /metrics body")
	}
}

// TestMetricsMiddleware_CountsRequests 通过 gin 引擎挂中间件,调用路由两次,/metrics 应见到对应 counter。
func TestMetricsMiddleware_CountsRequests(t *testing.T) {
	// 重置以避免污染其他用例
	metrics.HTTPRequestsTotal.Reset()

	g := gin.New()
	g.Use(middleware.Metrics())
	g.GET("/api/version", func(c *gin.Context) {
		c.String(200, "v1")
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/version", nil)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	}

	// 抓取 metrics 内容
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	handler := promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{})
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `rustdesk_api_http_requests_total{`) {
		t.Fatalf("missing http_requests_total in body: %s", body)
	}
	// 校验 path label 与计数(=2)
	if !strings.Contains(body, `path="/api/version"`) {
		t.Fatalf("expected path=/api/version label, got: %s", body)
	}
	if !strings.Contains(body, "} 2") {
		t.Fatalf("expected counter value 2 somewhere in body, got: %s", body)
	}
}

// TestMetricsServer_DisabledByConfig 当 Enable=false 时,不应监听端口。
func TestMetricsServer_DisabledByConfig(t *testing.T) {
	port := freePort(t)
	bind := fmt.Sprintf("127.0.0.1:%d", port)
	cfg := config.Metrics{Enable: false, Bind: bind, Path: "/metrics"}

	srv, err := metrics.StartServerForTest(cfg)
	if err != nil {
		t.Fatalf("start metrics: %v", err)
	}
	if srv != nil {
		metrics.ShutdownForTest(srv)
		t.Fatalf("expected nil server when Enable=false")
	}

	// 直接 Dial 确认端口未被监听
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", bind)
	if err == nil {
		conn.Close()
		t.Fatalf("expected dial to fail when metrics disabled")
	}
}
