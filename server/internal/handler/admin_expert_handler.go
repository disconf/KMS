package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminExpertHandler 专家审核后台接口。整组由 admin 角色守卫保护。
type AdminExpertHandler struct {
	experts *service.ExpertService
}

// NewAdminExpertHandler 创建 AdminExpertHandler。
func NewAdminExpertHandler(experts *service.ExpertService) *AdminExpertHandler {
	return &AdminExpertHandler{experts: experts}
}

// List 后台专家列表，可查全部审核状态。
//
//	@Summary  后台专家列表
//	@Tags     admin-expert
//	@Security BearerAuth
//	@Produce  json
//	@Param    page     query int    false "页码"
//	@Param    pageSize query int    false "每页条数"
//	@Param    status   query string false "审核状态 pending|approved|rejected|suspended"
//	@Param    keyword  query string false "昵称 / 真实姓名 / 身份证号关键词"
//	@Success  200 {object} response.Body
//	@Router   /admin/experts [get]
func (h *AdminExpertHandler) List(c *gin.Context) {
	var q dto.AdminExpertListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.experts.AdminList(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Detail 审核详情，含资质材料的带签名下载地址。
//
//	@Summary  后台专家详情
//	@Tags     admin-expert
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "专家 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/experts/{id} [get]
func (h *AdminExpertHandler) Detail(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.experts.AdminDetail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Audit 审核通过或驳回。
//
// 审核人 ID 只从令牌取：审计日志必须能追溯到真实操作人，
// 允许前端传 operator 等于允许伪造审核记录。
//
//	@Summary  审核专家
//	@Tags     admin-expert
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                  true "专家 ID"
//	@Param    body body dto.ExpertAuditReq   true "审核结论"
//	@Success  200 {object} response.Body
//	@Router   /admin/experts/{id}/audit [post]
func (h *AdminExpertHandler) Audit(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ExpertAuditReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.experts.Audit(c.Request.Context(), adminID, id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"audited": true})
}
