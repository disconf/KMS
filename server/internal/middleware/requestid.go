package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/opsexpert/platform/internal/pkg/reqid"
)

// RequestID 为每个请求分配链路 ID。优先复用上游传入的 X-Request-Id，
// 便于网关/前端把一次跨服务调用串起来。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(reqid.HeaderKey)
		if id == "" {
			id = uuid.NewString()
		}
		reqid.With(c, id)
		c.Header(reqid.HeaderKey, id)
		c.Next()
	}
}
