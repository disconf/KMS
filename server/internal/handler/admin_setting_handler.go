package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminSettingHandler 后台系统配置：抽佣比例与技能字典。整组由 admin 角色守卫保护。
//
// 分类不在这里：它是知识库四面（文章/工具/题库/资源）共用的维度，
// 已在 /admin/kb/categories 下，配置页只是把它和技能摆在同一屏上。
type AdminSettingHandler struct {
	settings *service.SettingService
	skills   *service.SkillAdminService
}

// NewAdminSettingHandler 创建 AdminSettingHandler。
func NewAdminSettingHandler(settings *service.SettingService, skills *service.SkillAdminService) *AdminSettingHandler {
	return &AdminSettingHandler{settings: settings, skills: skills}
}

// Overview 系统配置总览。
//
//	@Summary  系统配置总览
//	@Tags     admin-setting
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /admin/settings [get]
func (h *AdminSettingHandler) Overview(c *gin.Context) {
	view, err := h.settings.Overview(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// UpdateCommission 修改抽佣比例（基点，1500 = 15%）。
//
//	@Summary  修改平台抽佣比例
//	@Tags     admin-setting
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.AdminCommissionUpdateReq true "抽佣基点"
//	@Success  200 {object} response.Body
//	@Router   /admin/settings/commission [put]
func (h *AdminSettingHandler) UpdateCommission(c *gin.Context) {
	operator, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.AdminCommissionUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	view, err := h.settings.SetCommissionRateBP(
		c.Request.Context(), operator, authctx.Role(c), req.RateBP, auditMeta(c),
	)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, view)
}

// ListSkills 技能字典全量列表（含停用）。
//
//	@Summary  后台技能字典列表
//	@Tags     admin-setting
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /admin/skills [get]
func (h *AdminSettingHandler) ListSkills(c *gin.Context) {
	items, err := h.skills.List(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// CreateSkill 新增技能。
//
//	@Summary  新增技能
//	@Tags     admin-setting
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.AdminSkillCreateReq true "技能信息"
//	@Success  201 {object} response.Body
//	@Router   /admin/skills [post]
func (h *AdminSettingHandler) CreateSkill(c *gin.Context) {
	var req dto.AdminSkillCreateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.skills.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// UpdateSkill 更新技能。
//
//	@Summary  更新技能
//	@Tags     admin-setting
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                      true "技能 ID"
//	@Param    body body dto.AdminSkillUpdateReq  true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /admin/skills/{id} [put]
func (h *AdminSettingHandler) UpdateSkill(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.AdminSkillUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.skills.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// DeleteSkill 删除技能。被专家引用时返回冲突，提示改用停用。
//
//	@Summary  删除技能
//	@Tags     admin-setting
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "技能 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/skills/{id} [delete]
func (h *AdminSettingHandler) DeleteSkill(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.skills.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
