package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// SpeakHandler 实名发言申请与后台审核。
type SpeakHandler struct {
	speak *service.SpeakService
}

// NewSpeakHandler 创建 SpeakHandler。
func NewSpeakHandler(speak *service.SpeakService) *SpeakHandler {
	return &SpeakHandler{speak: speak}
}

// Apply 提交实名发言申请。
//
//	@Summary  提交发言权限申请
//	@Tags     me
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.SpeakApplyReq true "真实姓名、联系方式与理由"
//	@Success  200 {object} response.Body
//	@Router   /me/speak-apply [post]
func (h *SpeakHandler) Apply(c *gin.Context) {
	uid, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.SpeakApplyReq
	if !BindJSON(c, &req) {
		return
	}
	state, err := h.speak.Apply(c.Request.Context(), uid, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, state)
}

// State 查询自己的发言权限状态。
//
//	@Summary  查询发言权限状态
//	@Tags     me
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /me/speak-apply [get]
func (h *SpeakHandler) State(c *gin.Context) {
	uid, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	state, err := h.speak.State(c.Request.Context(), uid)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, state)
}

// ListApplications 后台分页查询发言申请。
//
//	@Summary  发言申请列表
//	@Tags     admin
//	@Security BearerAuth
//	@Produce  json
//	@Param    status query string false "none/pending/approved/rejected/all"
//	@Param    page query int false "页码"
//	@Param    pageSize query int false "每页条数"
//	@Success  200 {object} response.Body
//	@Router   /admin/speak-applications [get]
func (h *SpeakHandler) ListApplications(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	items, total, err := h.speak.ListApplications(c.Request.Context(), c.Query("status"), page, pageSize)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{
		"list":     items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// Review 审核发言申请。
//
//	@Summary  审核发言申请
//	@Tags     admin
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id path int true "用户 ID"
//	@Param    body body dto.AdminSpeakReviewReq true "通过/驳回与备注"
//	@Success  200 {object} response.Body
//	@Router   /admin/speak-applications/{id}/review [post]
func (h *SpeakHandler) Review(c *gin.Context) {
	uid, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	targetID, convErr := strconv.ParseInt(c.Param("id"), 10, 64)
	if convErr != nil || targetID <= 0 {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("用户 ID 非法"))
		return
	}
	var req dto.AdminSpeakReviewReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.speak.Review(c.Request.Context(), uid, authctx.Role(c), targetID, req, auditMeta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"reviewed": true})
}
