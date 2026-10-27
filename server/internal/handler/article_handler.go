package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// ArticleHandler 知识库前台接口。除点赞外均为游客可读。
type ArticleHandler struct {
	articles *service.ArticleService
}

// searchSuggestHotLimit 是检索建议里热门词的最大条数。
// 给太多等于没有建议——用户扫不完，也不会点。
const searchSuggestHotLimit = 8

// NewArticleHandler 创建 ArticleHandler。
func NewArticleHandler(articles *service.ArticleService) *ArticleHandler {
	return &ArticleHandler{articles: articles}
}

// ListCategories 分类列表（含各分类已发布文章数）。
//
//	@Summary  知识库分类列表
//	@Tags     kb
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /kb/categories [get]
func (h *ArticleHandler) ListCategories(c *gin.Context) {
	items, err := h.articles.ListCategories(c.Request.Context(), false)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// ListTags 标签聚合列表，按文章数倒序。
//
//	@Summary  知识库标签列表
//	@Tags     kb
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /kb/tags [get]
func (h *ArticleHandler) ListTags(c *gin.Context) {
	items, err := h.articles.ListTags(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// SearchSuggest 把用户的问句翻译成检索词，并附带热门关键词。
//
// 前端用它做两件事：一是告诉用户「我按这几个词帮你找」——
// 检索不准时这比沉默要有用；二是零结果时给出可点选的相关词，
// 比一句「没有找到」更能把人留在站内。
//
//	@Summary  检索词解析与建议
//	@Tags     kb
//	@Produce  json
//	@Param    q query string false "用户原始输入，可以是完整问句"
//	@Success  200 {object} response.Body
//	@Router   /kb/search/suggest [get]
func (h *ArticleHandler) SearchSuggest(c *gin.Context) {
	raw, _ := c.GetQuery("q")
	plan := service.PlanSearch(raw)

	// 热门词用标签聚合，不额外建表：标签本身就是运营在维护的话题词。
	// 拉不到也不让接口失败——检索词解析才是主产出。
	hot := []string{}
	if tags, err := h.articles.ListTags(c.Request.Context()); err == nil {
		for _, t := range tags {
			if len(hot) >= searchSuggestHotLimit {
				break
			}
			hot = append(hot, t.Name)
		}
	}

	response.OK(c, gin.H{
		"query":    plan.Raw,
		"terms":    plan.Terms,
		"expanded": plan.Expanded,
		"hot":      hot,
	})
}

// ListArticles 文章列表，支持分类 / 关键词 / 标签过滤与多种排序。
//
//	@Summary  知识库文章列表
//	@Tags     kb
//	@Produce  json
//	@Param    page         query int    false "页码，从 1 开始"
//	@Param    pageSize     query int    false "每页条数，最大 100"
//	@Param    categoryId   query int    false "分类 ID"
//	@Param    categorySlug query string false "分类 slug"
//	@Param    keyword      query string false "关键词，匹配标题/摘要/正文/标签"
//	@Param    tag          query string false "标签，精确匹配"
//	@Param    sort         query string false "排序 latest|hot|like"
//	@Success  200 {object} response.Body
//	@Router   /kb/articles [get]
func (h *ArticleHandler) ListArticles(c *gin.Context) {
	var q dto.ArticleListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.articles.ListArticles(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// ArticleDetail 文章详情。key 可传数字 ID 或 slug。
//
//	@Summary  知识库文章详情
//	@Tags     kb
//	@Produce  json
//	@Param    key path string true "文章 ID 或 slug"
//	@Success  200 {object} response.Body
//	@Router   /kb/articles/{key} [get]
func (h *ArticleHandler) ArticleDetail(c *gin.Context) {
	key := c.Param("key")
	// 未登录时 viewerID 为 0，详情里的 liked 恒为 false。
	detail, err := h.articles.ArticleDetail(c.Request.Context(), key, authctx.UserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// ToggleLike 点赞 / 取消点赞（需登录）。
//
//	@Summary  点赞或取消点赞
//	@Tags     kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    key path string true "文章 ID 或 slug"
//	@Success  200 {object} response.Body
//	@Router   /kb/articles/{key}/like [post]
func (h *ArticleHandler) ToggleLike(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	result, err := h.articles.ToggleLike(c.Request.Context(), userID, c.Param("key"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Contribute 用户投稿（需登录）。落库状态固定为待审核，不可自选。
//
//	@Summary  投稿文章
//	@Tags     kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ArticleContributeReq true "投稿内容"
//	@Success  201 {object} response.Body
//	@Router   /kb/articles/contribute [post]
func (h *ArticleHandler) Contribute(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ArticleContributeReq
	if !BindJSON(c, &req) {
		return
	}
	// 免审直发的判定只在中间件复核过的身份上做：
	// 角色与专家状态都取自数据库，不信客户端也不信令牌快照。
	direct := false
	if idn, ok := authctx.Get(c); ok {
		viewer := service.QaViewer{UserID: idn.UserID, Role: idn.Role, ExpertStatus: idn.ExpertStatus, SpeakStatus: idn.SpeakStatus}
		direct = viewer.CanPublishDirectly()
	}
	// 只读账号连投稿入口都进不来：投稿是"发布"，不是"浏览"。
	if !RequireSpeak(c) {
		return
	}
	detail, err := h.articles.Contribute(c.Request.Context(), userID, req, direct)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, detail)
}

// MyContributions 我的投稿列表（需登录）。含审核状态与驳回原因。
//
//	@Summary  我的投稿
//	@Tags     kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    page     query int false "页码"
//	@Param    pageSize query int false "每页条数"
//	@Success  200 {object} response.Body
//	@Router   /me/contributions [get]
func (h *ArticleHandler) MyContributions(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var q dto.PageQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()
	items, total, err := h.articles.MyContributions(c.Request.Context(), userID, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}
