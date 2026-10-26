package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/reqid"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminUserHandler 后台用户治理接口。整组由 admin 角色守卫保护。
//
// 这里没有「删除用户」：账号一旦删除，订单、评价、结算记录就都挂在一个空 ID 上，
// 对账与申诉全部断线。真不需要的人封禁即可——封禁可逆，删除不可逆。
type AdminUserHandler struct {
	users *service.UserAdminService
}

// NewAdminUserHandler 创建 AdminUserHandler。
func NewAdminUserHandler(users *service.UserAdminService) *AdminUserHandler {
	return &AdminUserHandler{users: users}
}

// List 用户列表。
//
//	@Summary  后台用户列表
//	@Tags     admin-user
//	@Security BearerAuth
//	@Produce  json
//	@Param    page     query int    false "页码"
//	@Param    pageSize query int    false "每页条数"
//	@Param    keyword  query string false "完整手机号（全等）或昵称/邮箱（模糊）"
//	@Param    role     query string false "user|expert|admin"
//	@Param    status   query string false "active|disabled"
//	@Success  200 {object} response.Body
//	@Router   /admin/users [get]
func (h *AdminUserHandler) List(c *gin.Context) {
	var q dto.AdminUserListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.users.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Detail 用户详情：账号、专家档案、订单聚合、操作留痕。
//
//	@Summary  后台用户详情
//	@Tags     admin-user
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "用户 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/users/{id} [get]
func (h *AdminUserHandler) Detail(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.users.Detail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// UpdateStatus 封禁 / 解封。
//
//	@Summary  封禁或解封账号
//	@Tags     admin-user
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                          true "用户 ID"
//	@Param    body body dto.AdminUpdateStatusReq     true "status + 原因（封禁必填）"
//	@Success  200 {object} response.Body
//	@Router   /admin/users/{id}/status [post]
func (h *AdminUserHandler) UpdateStatus(c *gin.Context) {
	operator, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.AdminUpdateStatusReq
	if !BindJSON(c, &req) {
		return
	}

	if err := h.users.UpdateStatus(
		c.Request.Context(), operator, authctx.Role(c), id, req, auditMeta(c),
	); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"status": req.Status})
}

// UpdateRole 修改角色。
//
//	@Summary  修改用户角色
//	@Tags     admin-user
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                        true "用户 ID"
//	@Param    body body dto.AdminUpdateRoleReq     true "目标角色"
//	@Success  200 {object} response.Body
//	@Router   /admin/users/{id}/role [post]
func (h *AdminUserHandler) UpdateRole(c *gin.Context) {
	operator, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.AdminUpdateRoleReq
	if !BindJSON(c, &req) {
		return
	}

	if err := h.users.UpdateRole(
		c.Request.Context(), operator, authctx.Role(c), id, req, auditMeta(c),
	); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"role": req.Role})
}

// auditMeta 收集审计要留的请求上下文。
func auditMeta(c *gin.Context) service.AuditMeta {
	return service.AuditMeta{
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		RequestID: reqid.From(c),
	}
}
