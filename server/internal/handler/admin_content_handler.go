package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminToolHandler 开源工具推荐的后台录入接口。整组由 admin 角色守卫保护。
type AdminToolHandler struct {
	tools *service.ToolService
}

// NewAdminToolHandler 创建 AdminToolHandler。
func NewAdminToolHandler(tools *service.ToolService) *AdminToolHandler {
	return &AdminToolHandler{tools: tools}
}

// List 后台工具列表，可查草稿与已下线。
//
//	@Summary  后台工具列表
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    page       query int    false "页码"
//	@Param    pageSize   query int    false "每页条数"
//	@Param    status     query string false "状态 draft|published|offline"
//	@Param    categoryId query int    false "分类 ID"
//	@Param    keyword    query string false "关键词"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tools [get]
func (h *AdminToolHandler) List(c *gin.Context) {
	var q dto.AdminToolListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.tools.AdminList(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Get 后台工具详情。
//
//	@Summary  后台工具详情
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "工具 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tools/{id} [get]
func (h *AdminToolHandler) Get(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.tools.AdminGet(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Create 新建工具。status 省略即存草稿。
//
//	@Summary  新建工具
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ToolCreateReq true "工具内容"
//	@Success  201 {object} response.Body
//	@Router   /admin/kb/tools [post]
func (h *AdminToolHandler) Create(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ToolCreateReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.tools.Create(c.Request.Context(), adminID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, detail)
}

// Update 更新工具，未传字段保持不变。
//
//	@Summary  更新工具
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                true "工具 ID"
//	@Param    body body dto.ToolUpdateReq  true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tools/{id} [put]
func (h *AdminToolHandler) Update(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ToolUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.tools.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Delete 删除工具。
//
//	@Summary  删除工具
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "工具 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tools/{id} [delete]
func (h *AdminToolHandler) Delete(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.tools.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Publish 发布工具。
//
//	@Summary  发布工具
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "工具 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tools/{id}/publish [post]
func (h *AdminToolHandler) Publish(c *gin.Context) {
	h.transition(c, h.tools.Publish)
}

// Offline 下线工具。
//
//	@Summary  下线工具
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "工具 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tools/{id}/offline [post]
func (h *AdminToolHandler) Offline(c *gin.Context) {
	h.transition(c, h.tools.Offline)
}

// transition 收敛发布 / 下线两个动作的重复代码：解析 ID → 调 service → 输出。
func (h *AdminToolHandler) transition(c *gin.Context, action func(context.Context, int64) (*dto.AdminToolDetail, error)) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := action(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// ListGuides 读取工具的教程关联清单（后台编辑用）。
//
//	@Summary  工具的四类实战教程
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Router   /admin/kb/tools/{id}/guides [get]
func (h *AdminToolHandler) ListGuides(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	guides, err := h.tools.GuidesForAdmin(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": guides})
}

// SetGuides 整体替换工具的教程关联。
//
//	@Summary  设置工具的四类实战教程
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Router   /admin/kb/tools/{id}/guides [put]
func (h *AdminToolHandler) SetGuides(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ToolGuidesReq
	if !BindJSON(c, &req) {
		return
	}
	guides, err := h.tools.SetGuides(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": guides})
}

// ============================ 资源下载 ============================

// AdminResourceHandler 资源下载栏目的后台接口。
type AdminResourceHandler struct {
	resources *service.ResourceService
}

// NewAdminResourceHandler 创建 AdminResourceHandler。
func NewAdminResourceHandler(resources *service.ResourceService) *AdminResourceHandler {
	return &AdminResourceHandler{resources: resources}
}

// List 后台资源列表，可查草稿与已下线。
//
//	@Summary  后台资源列表
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    page       query int    false "页码"
//	@Param    pageSize   query int    false "每页条数"
//	@Param    status     query string false "状态 draft|published|offline"
//	@Param    categoryId query int    false "分类 ID"
//	@Param    keyword    query string false "关键词"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/resources [get]
func (h *AdminResourceHandler) List(c *gin.Context) {
	var q dto.AdminResourceListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.resources.AdminList(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Get 后台资源详情（含 FileKey）。
//
//	@Summary  后台资源详情
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "资源 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/resources/{id} [get]
func (h *AdminResourceHandler) Get(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.resources.AdminGet(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Create 新建资源。fileKey 必须是 /upload（scene=resource_file）返回的 key。
//
//	@Summary  新建资源
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ResourceCreateReq true "资源内容"
//	@Success  201 {object} response.Body
//	@Router   /admin/kb/resources [post]
func (h *AdminResourceHandler) Create(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ResourceCreateReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.resources.Create(c.Request.Context(), adminID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, detail)
}

// Update 更新资源。换文件时大小与校验和会自动重算。
//
//	@Summary  更新资源
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                    true "资源 ID"
//	@Param    body body dto.ResourceUpdateReq  true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/resources/{id} [put]
func (h *AdminResourceHandler) Update(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ResourceUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.resources.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Delete 删除资源，并在无人引用时回收文件。
//
//	@Summary  删除资源
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "资源 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/resources/{id} [delete]
func (h *AdminResourceHandler) Delete(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.resources.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Publish 发布资源。文件缺失时拒绝发布。
//
//	@Summary  发布资源
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "资源 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/resources/{id}/publish [post]
func (h *AdminResourceHandler) Publish(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.resources.Publish(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Offline 下线资源。
//
//	@Summary  下线资源
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "资源 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/resources/{id}/offline [post]
func (h *AdminResourceHandler) Offline(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.resources.Offline(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// ListLinks 读取资源的外部下载链接。
//
//	@Summary  资源下载链接
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Router   /admin/kb/resources/{id}/links [get]
func (h *AdminResourceHandler) ListLinks(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	links, err := h.resources.LinksForAdmin(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": links})
}

// SetLinks 整体替换资源的外部下载链接。
//
//	@Summary  设置资源下载链接
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Router   /admin/kb/resources/{id}/links [put]
func (h *AdminResourceHandler) SetLinks(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ResourceLinksReq
	if !BindJSON(c, &req) {
		return
	}
	links, err := h.resources.SetLinks(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": links})
}
