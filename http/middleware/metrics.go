package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/metrics"
)

// Metrics gin 中间件,统计每个请求的方法 / 路径模板 / 响应状态。
// path 标签使用 c.FullPath() 避免高基数;若未匹配路由(404)则降级为 "unknown"。
// 中间件应注册在 Limiter 之前,以便统计被限流的 4xx 请求。
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		metrics.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	}
}
