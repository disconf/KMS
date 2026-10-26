package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminNewsHandler 运维资讯订阅源的后台管理接口。
//
// 独立成一个 handler 而不是塞进 admin_content_handler：订阅源不是内容本身，
// 它是「内容从哪来」的配置，生命周期与文章完全不同。
type AdminNewsHandler struct {
	news *service.NewsService
}

// NewAdminNewsHandler 创建 AdminNewsHandler。
func NewAdminNewsHandler(news *service.NewsService) *AdminNewsHandler {
	return &AdminNewsHandler{news: news}
}

// ListSources 订阅源列表，带最近抓取状态。
//
//	@Summary  资讯订阅源列表
//	@Tags     admin-news
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /admin/news/sources [get]
func (h *AdminNewsHandler) ListSources(c *gin.Context) {
	items, err := h.news.AdminListSources(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// CreateSource 新增订阅源。
//
//	@Summary  新增资讯订阅源
//	@Tags     admin-news
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Success  201 {object} response.Body
//	@Router   /admin/news/sources [post]
func (h *AdminNewsHandler) CreateSource(c *gin.Context) {
	var req dto.NewsSourceReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.news.CreateSource(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// UpdateSource 更新订阅源。
//
//	@Summary  更新资讯订阅源
//	@Tags     admin-news
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Router   /admin/news/sources/{id} [put]
func (h *AdminNewsHandler) UpdateSource(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.NewsSourceUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.news.UpdateSource(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// DeleteSource 删除订阅源，不影响已抓取的文章。
//
//	@Summary  删除资讯订阅源
//	@Tags     admin-news
//	@Security BearerAuth
//	@Router   /admin/news/sources/{id} [delete]
func (h *AdminNewsHandler) DeleteSource(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.news.DeleteSource(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// FetchNow 立即抓取一个源。
//
//	@Summary  立即抓取指定订阅源
//	@Tags     admin-news
//	@Security BearerAuth
//	@Produce  json
//	@Router   /admin/news/sources/{id}/fetch [post]
func (h *AdminNewsHandler) FetchNow(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	res, err := h.news.FetchNow(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
