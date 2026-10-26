package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// ChannelHandler 栏目导航接口。
type ChannelHandler struct {
	channels *service.ChannelService
}

// NewChannelHandler 创建 ChannelHandler。
func NewChannelHandler(channels *service.ChannelService) *ChannelHandler {
	return &ChannelHandler{channels: channels}
}

// List 返回全部栏目及其内容数，前端据此渲染导航栏。
//
//	@Summary  栏目导航
//	@Tags     kb
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /channels [get]
func (h *ChannelHandler) List(c *gin.Context) {
	response.OK(c, gin.H{"list": h.channels.List(c.Request.Context())})
}

// ============================ 开源工具推荐 ============================

// ToolHandler 工具栏目的前台接口。
type ToolHandler struct {
	tools *service.ToolService
	// books 用来在工具详情里补上「系统教程」区块。
	//
	// 为什么不把 ToolBookService 注入 ToolService：两个 service 互相依赖
	// 之后，任何一个的构造函数变动都会牵动另一个（以及所有测试）。
	// 组装放在 handler 这一层，service 之间保持单向。
	books *service.ToolBookService
}

// NewToolHandler 创建 ToolHandler。
func NewToolHandler(tools *service.ToolService, books *service.ToolBookService) *ToolHandler {
	return &ToolHandler{tools: tools, books: books}
}

// List 工具列表。
//
//	@Summary  工具列表
//	@Tags     kb-tool
//	@Produce  json
//	@Param    page         query int    false "页码"
//	@Param    pageSize     query int    false "每页条数"
//	@Param    categoryId   query int    false "分类 ID"
//	@Param    categorySlug query string false "分类标识"
//	@Param    keyword      query string false "关键词"
//	@Param    language     query string false "实现语言"
//	@Param    license      query string false "开源协议"
//	@Param    tag          query string false "标签"
//	@Param    featuredOnly query bool   false "仅看精选"
//	@Param    minStars     query int    false "最小 Star 数"
//	@Param    sort         query string false "排序 latest|hot|stars|sort"
//	@Success  200 {object} response.Body
//	@Router   /tools [get]
func (h *ToolHandler) List(c *gin.Context) {
	var q dto.ToolListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.tools.ListTools(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Detail 工具详情。key 可以是 ID 或 slug。
//
//	@Summary  工具详情
//	@Tags     kb-tool
//	@Produce  json
//	@Param    key path string true "工具 ID 或 slug"
//	@Success  200 {object} response.Body
//	@Router   /tools/{key} [get]
func (h *ToolHandler) Detail(c *gin.Context) {
	ctx := c.Request.Context()
	detail, err := h.tools.ToolDetail(ctx, c.Param("key"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	// 教程集取不到不影响工具详情本身：那只是页面上的一个区块，
	// 为它把整页 500 掉，等于让用户连工具简介都看不到。
	if h.books != nil {
		books, berr := h.books.BooksOf(ctx, detail.ID)
		if berr != nil {
			logger.Warn("读取工具教程集失败，详情页将不展示该区块",
				"tool_id", detail.ID, "err", berr.Error())
			books = []dto.ToolBookItem{}
		}
		detail.Books = books
	}
	response.OK(c, detail)
}

// Filters 工具筛选器的可选值。
//
//	@Summary  工具筛选项
//	@Tags     kb-tool
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /tools/filters [get]
func (h *ToolHandler) Filters(c *gin.Context) {
	response.OK(c, h.tools.Filters(c.Request.Context()))
}

// ============================ 资源下载 ============================

// ResourceHandler 资源下载栏目的前台接口。
type ResourceHandler struct {
	resources *service.ResourceService
}

// NewResourceHandler 创建 ResourceHandler。
func NewResourceHandler(resources *service.ResourceService) *ResourceHandler {
	return &ResourceHandler{resources: resources}
}

// List 资源列表。
//
//	@Summary  资源列表
//	@Tags     kb-resource
//	@Produce  json
//	@Param    page         query int    false "页码"
//	@Param    pageSize     query int    false "每页条数"
//	@Param    categoryId   query int    false "分类 ID"
//	@Param    categorySlug query string false "分类标识"
//	@Param    keyword      query string false "关键词"
//	@Param    tag          query string false "标签"
//	@Param    fileExt      query string false "文件类型"
//	@Param    sort         query string false "排序 latest|hot|download"
//	@Success  200 {object} response.Body
//	@Router   /resources [get]
func (h *ResourceHandler) List(c *gin.Context) {
	var q dto.ResourceListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.resources.ListResources(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Detail 资源详情。key 可以是 ID 或 slug。
//
//	@Summary  资源详情
//	@Tags     kb-resource
//	@Produce  json
//	@Param    key path string true "资源 ID 或 slug"
//	@Success  200 {object} response.Body
//	@Router   /resources/{key} [get]
func (h *ResourceHandler) Detail(c *gin.Context) {
	detail, err := h.resources.ResourceDetail(c.Request.Context(), c.Param("key"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Download 取下载凭证。
//
// 挂在 OptionalAuth 上而不是 Auth：绝大多数资源不要求登录，
// 强制登录会把"下载即用"这个最好的获客钩子掐掉。
// 需要登录的资源由 service 依据 needLogin 单独拦截。
//
//	@Summary  获取下载地址
//	@Tags     kb-resource
//	@Produce  json
//	@Param    key path string true "资源 ID 或 slug"
//	@Success  200 {object} response.Body
//	@Router   /resources/{key}/download [get]
func (h *ResourceHandler) Download(c *gin.Context) {
	item, err := h.resources.Download(c.Request.Context(), c.Param("key"), authctx.UserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// FileExts 可选的文件类型筛选项。
//
//	@Summary  资源文件类型
//	@Tags     kb-resource
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /resources/file-exts [get]
func (h *ResourceHandler) FileExts(c *gin.Context) {
	response.OK(c, gin.H{"list": h.resources.FileExts(c.Request.Context())})
}
