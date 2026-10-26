package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// ToolBookHandler 教程集的前台接口。
//
// 只有详情一个端点：工具详情页已经把该工具下所有已发布教程集的目录树
// 一并带回来了（`ToolDetail.books`），所以这里只服务「独立打开一本教程集」
// 的场景——从收藏夹、分享链接直接进来，不需要先加载整个工具页。
type ToolBookHandler struct {
	books *service.ToolBookService
}

// NewToolBookHandler 创建 ToolBookHandler。
func NewToolBookHandler(books *service.ToolBookService) *ToolBookHandler {
	return &ToolBookHandler{books: books}
}

// Detail 教程集详情。草稿与已下线的书返回 404，不透露存在性。
//
//	@Summary  教程集详情
//	@Tags     kb-tool
//	@Produce  json
//	@Param    id path int true "教程集 ID"
//	@Success  200 {object} response.Body
//	@Router   /tool-books/{id} [get]
func (h *ToolBookHandler) Detail(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.books.BookDetail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// ============================ 后台 ============================

// AdminToolBookHandler 教程集的后台维护接口。
type AdminToolBookHandler struct {
	books *service.ToolBookService
}

// NewAdminToolBookHandler 创建 AdminToolBookHandler。
func NewAdminToolBookHandler(books *service.ToolBookService) *AdminToolBookHandler {
	return &AdminToolBookHandler{books: books}
}

// List 某个工具下的全部教程集（含草稿与失效节点）。
//
//	@Summary  工具的教程集列表
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "工具 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tools/{id}/books [get]
func (h *AdminToolBookHandler) List(c *gin.Context) {
	toolID, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	items, err := h.books.AdminBooks(c.Request.Context(), toolID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// Create 给某个工具新建一本教程集（固定草稿）。
//
//	@Summary  新建教程集
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                    true "工具 ID"
//	@Param    body body dto.ToolBookCreateReq  true "教程集信息"
//	@Success  201  {object} response.Body
//	@Router   /admin/kb/tools/{id}/books [post]
func (h *AdminToolBookHandler) Create(c *gin.Context) {
	toolID, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ToolBookCreateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.books.CreateBook(c.Request.Context(), userID, toolID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// Get 后台读取单本教程集（含完整目录树）。
//
//	@Summary  教程集详情（后台）
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "教程集 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tool-books/{id} [get]
func (h *AdminToolBookHandler) Get(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	item, err := h.books.AdminBook(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Update 局部更新教程集信息。
//
//	@Summary  更新教程集
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                    true "教程集 ID"
//	@Param    body body dto.ToolBookUpdateReq  true "要改的字段"
//	@Success  200  {object} response.Body
//	@Router   /admin/kb/tool-books/{id} [put]
func (h *AdminToolBookHandler) Update(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ToolBookUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.books.UpdateBook(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Delete 删除教程集（章节一并清理）。
//
//	@Summary  删除教程集
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "教程集 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tool-books/{id} [delete]
func (h *AdminToolBookHandler) Delete(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.books.DeleteBook(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Publish 发布教程集（要求目录非空）。
//
//	@Summary  发布教程集
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "教程集 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tool-books/{id}/publish [post]
func (h *AdminToolBookHandler) Publish(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	item, err := h.books.PublishBook(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Offline 下线教程集。
//
//	@Summary  下线教程集
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "教程集 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tool-books/{id}/offline [post]
func (h *AdminToolBookHandler) Offline(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	item, err := h.books.OfflineBook(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Outline 读取目录大纲文本（后台编辑页回填）。
//
//	@Summary  读取章节大纲
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "教程集 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/tool-books/{id}/outline [get]
func (h *AdminToolBookHandler) Outline(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	doc, err := h.books.Outline(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, doc)
}

// SetOutline 用大纲文本整体替换目录。
//
//	@Summary  保存章节大纲
//	@Description 格式：# 章标题 / ## 节标题 | 文章slug或外链 | 导语。整体替换，两次保存之间是完整的一棵树。
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                          true "教程集 ID"
//	@Param    body body dto.ToolChapterOutlineReq    true "大纲文本"
//	@Success  200  {object} response.Body
//	@Router   /admin/kb/tool-books/{id}/outline [put]
func (h *AdminToolBookHandler) SetOutline(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ToolChapterOutlineReq
	if !BindJSON(c, &req) {
		return
	}
	result, err := h.books.SetOutline(c.Request.Context(), id, req.Outline)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}
