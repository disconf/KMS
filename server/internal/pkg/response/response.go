// Package response 提供统一的 HTTP 响应封装。
//
// 成功：{"code":0,"message":"ok","data":{...},"requestId":"..."}
// 失败：{"code":20005,"message":"验证码不正确","requestId":"..."}
//
// HTTP 状态码与业务码保持一致语义（401 未登录、403 无权限、429 限流等），
// 前端既可读 code 也可依赖 status，二者不会冲突。
package response

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/reqid"
)

// Body 是统一响应体。
type Body struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// PageData 是统一分页结构，前端约定 {list,total,page,pageSize}。
type PageData struct {
	List     any   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

// NewPage 构造分页数据，list 为 nil 时输出空数组而非 null，避免前端判空分支。
func NewPage(list any, total int64, page, pageSize int) *PageData {
	if list == nil {
		list = []any{}
	}
	return &PageData{List: list, Total: total, Page: page, PageSize: pageSize}
}

// OK 返回 200 成功响应。
func OK(c *gin.Context, data any) { write(c, errcode.OK, data) }

// Created 返回 201 成功响应（资源创建场景）。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{
		Code:      errcode.OK.Code,
		Message:   errcode.OK.Msg,
		Data:      data,
		RequestID: reqid.From(c),
	})
}

// Page 返回 200 分页响应。
func Page(c *gin.Context, list any, total int64, page, pageSize int) {
	OK(c, NewPage(list, total, page, pageSize))
}

// Fail 返回业务错误响应，自动归一未知错误并把细节记录到日志。
func Fail(c *gin.Context, err error) {
	e := errcode.From(err)

	if e.Code == errcode.ErrInternal.Code {
		logDetail(c, e)
	}
	c.AbortWithStatusJSON(e.HTTP(), Body{
		Code:      e.Code,
		Message:   e.Msg,
		RequestID: reqid.From(c),
	})
}

// FailWithMsg 在保留错误码的前提下覆盖提示文案。
func FailWithMsg(c *gin.Context, err error, msg string) {
	Fail(c, errcode.From(err).WithMsg(msg))
}

// FailCode 直接以错误码返回。
func FailCode(c *gin.Context, e *errcode.Error) { Fail(c, e) }

func write(c *gin.Context, e *errcode.Error, data any) {
	c.JSON(e.HTTP(), Body{
		Code:      e.Code,
		Message:   e.Msg,
		Data:      data,
		RequestID: reqid.From(c),
	})
}

func logDetail(c *gin.Context, e *errcode.Error) {
	attrs := []any{
		slog.Int("code", e.Code),
		slog.String("path", c.FullPath()),
		slog.String("method", c.Request.Method),
		slog.String("request_id", reqid.From(c)),
	}
	if cause := e.Unwrap(); cause != nil {
		attrs = append(attrs, slog.String("cause", cause.Error()))
	}
	logger.L().Error("未处理的服务端错误", attrs...)
}
