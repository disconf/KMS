package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/reqid"
	"github.com/opsexpert/platform/internal/pkg/response"
)

// Recovery 捕获 panic，记录堆栈并返回统一错误响应，避免进程被打挂。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.L().Error("请求处理发生 panic",
					slog.String("path", c.Request.URL.Path),
					slog.String("method", c.Request.Method),
					slog.String("request_id", reqid.From(c)),
					slog.Any("panic", r),
					slog.String("stack", string(debug.Stack())),
				)

				// 已经写出部分响应时无法再改状态码，只能中断。
				if c.Writer.Written() {
					c.Abort()
					return
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, response.Body{
					Code:      errcode.ErrInternal.Code,
					Message:   errcode.ErrInternal.Msg,
					RequestID: reqid.From(c),
				})
			}
		}()
		c.Next()
	}
}

// NoRoute 统一 404 响应，避免 gin 默认的纯文本输出破坏前端解析。
func NoRoute() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusNotFound, response.Body{
			Code:      errcode.ErrNotFound.Code,
			Message:   fmt.Sprintf("接口不存在: %s %s", c.Request.Method, c.Request.URL.Path),
			RequestID: reqid.From(c),
		})
	}
}
