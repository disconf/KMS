package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminArticleHandler 知识库后台接口。整组由 admin 角色守卫保护。
type AdminArticleHandler struct {
	articles *service.ArticleService
}

// NewAdminArticleHandler 创建 AdminArticleHandler。
func NewAdminArticleHandler(articles *service.ArticleService) *AdminArticleHandler {
	return &AdminArticleHandler{articles: articles}
}

// ============================ 分类 ============================

// ListCategories 全部分类（含停用），文章数按全部状态统计。
//
//	@Summary  后台分类列表
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/categories [get]
func (h *AdminArticleHandler) ListCategories(c *gin.Context) {
	items, err := h.articles.ListCategories(c.Request.Context(), true)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// CreateCategory 新建分类。
//
//	@Summary  新建分类
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.CategoryCreateReq true "分类信息"
//	@Success  201 {object} response.Body
//	@Router   /admin/kb/categories [post]
func (h *AdminArticleHandler) CreateCategory(c *gin.Context) {
	var req dto.CategoryCreateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.articles.CreateCategory(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// UpdateCategory 更新分类。
//
//	@Summary  更新分类
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                    true "分类 ID"
//	@Param    body body dto.CategoryUpdateReq true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/categories/{id} [put]
func (h *AdminArticleHandler) UpdateCategory(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.CategoryUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.articles.UpdateCategory(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// DeleteCategory 删除分类。分类下仍有文章时返回冲突错误。
//
//	@Summary  删除分类
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "分类 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/categories/{id} [delete]
func (h *AdminArticleHandler) DeleteCategory(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.articles.DeleteCategory(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// ============================ 文章 ============================

// List 后台文章列表，可查草稿与已下线。
//
//	@Summary  后台文章列表
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    page       query int    false "页码"
//	@Param    pageSize   query int    false "每页条数"
//	@Param    status     query string false "状态 draft|published|offline"
//	@Param    categoryId query int    false "分类 ID"
//	@Param    keyword    query string false "关键词"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/articles [get]
func (h *AdminArticleHandler) List(c *gin.Context) {
	var q dto.AdminArticleListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.articles.AdminListArticles(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Get 后台读取文章详情（含草稿）。
//
//	@Summary  后台文章详情
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "文章 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/articles/{id} [get]
func (h *AdminArticleHandler) Get(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.articles.AdminGetArticle(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Create 新建文章。status 省略即存草稿。
//
//	@Summary  新建文章
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ArticleCreateReq true "文章内容"
//	@Success  201 {object} response.Body
//	@Router   /admin/kb/articles [post]
func (h *AdminArticleHandler) Create(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ArticleCreateReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.articles.CreateArticle(c.Request.Context(), adminID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, detail)
}

// Update 更新文章，未传字段保持不变。
//
//	@Summary  更新文章
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                   true "文章 ID"
//	@Param    body body dto.ArticleUpdateReq true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/articles/{id} [put]
func (h *AdminArticleHandler) Update(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ArticleUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.articles.UpdateArticle(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Delete 删除文章。
//
//	@Summary  删除文章
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "文章 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/articles/{id} [delete]
func (h *AdminArticleHandler) Delete(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.articles.DeleteArticle(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Publish 发布文章。
//
//	@Summary  发布文章
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "文章 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/articles/{id}/publish [post]
func (h *AdminArticleHandler) Publish(c *gin.Context) {
	h.transition(c, h.articles.PublishArticle)
}

// Offline 下线文章。
//
//	@Summary  下线文章
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "文章 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/articles/{id}/offline [post]
func (h *AdminArticleHandler) Offline(c *gin.Context) {
	h.transition(c, h.articles.OfflineArticle)
}

// transition 收敛发布 / 下线两个动作的重复代码：解析 ID → 调 service → 输出。
func (h *AdminArticleHandler) transition(c *gin.Context, action func(context.Context, int64) (*dto.AdminArticleDetail, error)) {
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

// Review 审核待审内容：approve 通过并发布，reject 驳回并记录原因。
//
//	@Summary  审核投稿
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                  true "文章 ID"
//	@Param    body body dto.ArticleReviewReq true "审核动作与意见"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/articles/{id}/review [post]
func (h *AdminArticleHandler) Review(c *gin.Context) {
	reviewerID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.ArticleReviewReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.articles.ReviewArticle(c.Request.Context(), reviewerID, id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}
