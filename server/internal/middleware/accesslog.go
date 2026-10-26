package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/reqid"
)

// slowRequestThreshold 超过该耗时的请求会以 warn 级别记录，便于发现慢接口。
const slowRequestThreshold = time.Second

// AccessLog 输出结构化访问日志。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("latency", latency),
			slog.String("ip", c.ClientIP()),
			slog.String("request_id", reqid.From(c)),
		}
		if query != "" {
			attrs = append(attrs, slog.String("query", query))
		}
		if uid := c.GetHeader("X-User-Id"); uid != "" {
			attrs = append(attrs, slog.String("user_id", uid))
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("errors", c.Errors.String()))
		}

		switch {
		case c.Writer.Status() >= 500:
			logger.L().Error("请求处理失败", attrs...)
		case latency >= slowRequestThreshold:
			logger.L().Warn("慢请求", attrs...)
		default:
			logger.L().Info("请求完成", attrs...)
		}
	}
}
