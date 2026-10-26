// Package handler 是 HTTP 接入层：只做参数绑定、调用 service、输出统一响应。
// 任何业务分支都不应该出现在这里。
package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/response"
)

// BindJSON 绑定并校验 JSON 请求体，失败时直接写出统一错误响应并返回 false。
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("请求参数不合法："+cleanBindError(err)))
		return false
	}
	return true
}

// BindQuery 绑定并校验查询参数。
func BindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("查询参数不合法："+cleanBindError(err)))
		return false
	}
	return true
}

// cleanBindError 把 validator 的英文错误精简成更容易定位的片段。
// 完整内容仍会通过响应返回，便于前端联调时快速定位字段。
func cleanBindError(err error) string {
	return err.Error()
}

// PathInt64 解析路径参数为 int64 主键，非法时写出统一错误响应并返回 false。
func PathInt64(c *gin.Context, name string) (int64, bool) {
	raw := strings.TrimSpace(c.Param(name))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("路径参数 "+name+" 不合法"))
		return 0, false
	}
	return id, true
}
