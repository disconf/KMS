// Package reqid 负责请求链路 ID 在 context 中的存取，供中间件与响应体共用，避免包循环依赖。
package reqid

import "github.com/gin-gonic/gin"

// CtxKey 是 gin.Context 中存放 request id 的键名。
const CtxKey = "request_id"

// HeaderKey 是回写到响应头与日志中的字段名。
const HeaderKey = "X-Request-Id"

// With 把 request id 写入请求上下文。
func With(c *gin.Context, id string) { c.Set(CtxKey, id) }

// From 从请求上下文读取 request id，缺失时返回空串。
func From(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(CtxKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
