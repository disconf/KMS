package middleware

import (
	"context"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/config"
)

// CORS 按配置装配跨域策略。
func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     cfg.AllowOrigins,
		AllowMethods:     cfg.AllowMethods,
		AllowHeaders:     cfg.AllowHeaders,
		ExposeHeaders:    cfg.ExposeHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge.Std(),
	})
}

// Timeout 给请求上下文设置超时。
//
// 它的作用边界要说清楚：会把 deadline 注入 context，因此所有以 WithContext(ctx)
// 发起的数据库/缓存/HTTP 调用都会提前失败。但它无法强行中断已经在跑的 handler goroutine，
// 所以它只是"防止慢查询拖垮连接池"的兜底，不是万能的熔断。
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
