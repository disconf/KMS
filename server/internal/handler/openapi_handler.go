// Package handler — /open/v1 开放只读接口。
//
// 面向用户脚本：查文档、搜文章、抽题。只读、需密钥、有限流。
// 与 /api/v1 前台接口完全分开：前台接口未来会加个性化（推荐、历史），
// 开放接口保持稳定——脚本的兼容成本远高于页面。
package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/repository"
)

// OpenAPIHandler 开放只读接口。
type OpenAPIHandler struct {
	articles *repository.ArticleRepo
	quizzes  *repository.QuizRepo
}

// NewOpenAPIHandler 创建。
func NewOpenAPIHandler(articles *repository.ArticleRepo, quizzes *repository.QuizRepo) *OpenAPIHandler {
	return &OpenAPIHandler{articles: articles, quizzes: quizzes}
}

// Ping 探活 + 密钥自检：脚本启动时先 ping 一下，确认密钥有效再开跑。
//
//	@Summary  密钥探活
//	@Tags     open
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /open/v1/ping [get]
func (h *OpenAPIHandler) Ping(c *gin.Context) {
	response.OK(c, gin.H{
		"ok":      true,
		"service": "ops-expert open api",
		"version": "v1",
	})
}

// ListArticles 文章列表。
//
//	@Summary  已发布文章列表
//	@Tags     open
//	@Produce  json
//	@Param    channel  query string false "栏目筛选"
//	@Param    q        query string false "关键词"
//	@Param    page     query int    false "页码"
//	@Param    pageSize query int    false "条数（≤50）"
//	@Success  200 {object} response.Body
//	@Router   /open/v1/articles [get]
func (h *OpenAPIHandler) ListArticles(c *gin.Context) {
	page, pageSize := openPageParams(c)
	items, total, err := h.articles.List(c.Request.Context(), repository.ArticleFilter{
		Status:  model.ArticleStatusPublished,
		Channel: strings.TrimSpace(c.Query("channel")),
		Keyword: strings.TrimSpace(c.Query("q")),
	}, (page-1)*pageSize, pageSize)
	if err != nil {
		response.Fail(c, err)
		return
	}
	out := make([]dto.OpenArticleItem, 0, len(items))
	for _, a := range items {
		out = append(out, openArticleItem(a))
	}
	response.OK(c, gin.H{"items": out, "total": total, "page": page, "pageSize": pageSize})
}

// ArticleDetail 文章详情（Markdown 原文）。
//
//	@Summary  文章详情
//	@Tags     open
//	@Produce  json
//	@Param    slug path string true "文章 slug"
//	@Success  200 {object} response.Body
//	@Router   /open/v1/articles/{slug} [get]
func (h *OpenAPIHandler) ArticleDetail(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		response.FailCode(c, errcode.ErrParamInvalid)
		return
	}
	a, err := h.articles.GetBySlug(c.Request.Context(), slug)
	if err != nil || a.Status != model.ArticleStatusPublished {
		response.FailCode(c, errcode.ErrArticleNotPublished)
		return
	}
	item := openArticleItem(*a)
	response.OK(c, dto.OpenArticleDetail{
		OpenArticleItem: item,
		Content:         a.Content,
	})
}

// RandomQuizzes 随机抽题。
//
//	@Summary  随机抽题
//	@Tags     open
//	@Produce  json
//	@Param    category query int false "分类 ID"
//	@Param    count    query int false "数量（≤20）"
//	@Success  200 {object} response.Body
//	@Router   /open/v1/quizzes/random [get]
func (h *OpenAPIHandler) RandomQuizzes(c *gin.Context) {
	count := 5
	if v := c.Query("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 20 {
			count = n
		}
	}
	var categoryID int64
	if v := c.Query("category"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			categoryID = n
		}
	}
	items, err := h.quizzes.SampleRandom(c.Request.Context(), repository.QuizFilter{
		Status:     model.QuizStatusPublished,
		CategoryID: categoryID,
	}, count)
	if err != nil {
		response.Fail(c, err)
		return
	}
	out := make([]dto.OpenQuizItem, 0, len(items))
	for _, q := range items {
		options := make([]string, 0, len(q.Options))
		for _, o := range q.Options {
			options = append(options, o.Key+". "+o.Text)
		}
		out = append(out, dto.OpenQuizItem{
			ID:        q.ID,
			Direction: quizCategoryName(q.CategoryID),
			Stem:      q.Stem,
			Options:   options,
			Answer:    strings.Join(q.Correct, ","),
			Analysis:  q.Analysis,
		})
	}
	response.OK(c, gin.H{"items": out})
}

// openArticleItem 文章 → 开放条目。
func openArticleItem(a model.Article) dto.OpenArticleItem {
	var publishedAt = a.CreatedAt
	if a.PublishedAt != nil {
		publishedAt = *a.PublishedAt
	}
	return dto.OpenArticleItem{
		Title:       a.Title,
		Slug:        a.Slug,
		Summary:     a.Summary,
		Channel:     a.Channel,
		Tags:        []string(a.Tags),
		Views:       a.ViewCount,
		PublishedAt: publishedAt,
	}
}

// openPageParams 分页参数（开放接口上限收紧到 50）。
func openPageParams(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ = strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}
	return page, pageSize
}

// quizCategoryName 分类名占位：开放接口第一期直接给 ID，
// 分类名要与 model.AllCategories 联动，等前端开发者文档页一起做。
func quizCategoryName(id int64) string {
	return strconv.FormatInt(id, 10)
}
