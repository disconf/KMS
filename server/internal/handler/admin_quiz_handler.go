package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminQuizHandler 运维题库的后台录题接口。整组由 admin 角色守卫保护。
type AdminQuizHandler struct {
	quizzes *service.QuizService
}

// NewAdminQuizHandler 创建 AdminQuizHandler。
func NewAdminQuizHandler(quizzes *service.QuizService) *AdminQuizHandler {
	return &AdminQuizHandler{quizzes: quizzes}
}

// List 后台题目列表，含答案与解析。
//
//	@Summary  后台题目列表
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    page       query int    false "页码"
//	@Param    pageSize   query int    false "每页条数"
//	@Param    status     query string false "状态 draft|published|offline"
//	@Param    categoryId query int    false "分类 ID"
//	@Param    difficulty query string false "难度"
//	@Param    keyword    query string false "关键词"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions [get]
func (h *AdminQuizHandler) List(c *gin.Context) {
	var q dto.AdminQuizListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.quizzes.AdminList(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Get 后台读取单题。
//
//	@Summary  后台题目详情
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "题目 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions/{id} [get]
func (h *AdminQuizHandler) Get(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	item, err := h.quizzes.AdminGet(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Create 录入题目。
//
//	@Summary  录入题目
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.QuizQuestionCreateReq true "题目内容"
//	@Success  201 {object} response.Body
//	@Router   /admin/kb/quiz/questions [post]
func (h *AdminQuizHandler) Create(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.QuizQuestionCreateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.quizzes.Create(c.Request.Context(), adminID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// Update 更新题目。改动选项 / 题型 / 答案时三者会整体重新校验。
//
//	@Summary  更新题目
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                          true "题目 ID"
//	@Param    body body dto.QuizQuestionUpdateReq    true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions/{id} [put]
func (h *AdminQuizHandler) Update(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.QuizQuestionUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.quizzes.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Delete 删除题目。
//
//	@Summary  删除题目
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "题目 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions/{id} [delete]
func (h *AdminQuizHandler) Delete(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.quizzes.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Publish 发布题目。
//
//	@Summary  发布题目
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "题目 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions/{id}/publish [post]
func (h *AdminQuizHandler) Publish(c *gin.Context) {
	h.transition(c, h.quizzes.Publish)
}

// Offline 下线题目。
//
//	@Summary  下线题目
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "题目 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions/{id}/offline [post]
func (h *AdminQuizHandler) Offline(c *gin.Context) {
	h.transition(c, h.quizzes.Offline)
}

// transition 收敛发布 / 下线两个动作的重复代码。
func (h *AdminQuizHandler) transition(c *gin.Context, action func(context.Context, int64) (*dto.AdminQuizItem, error)) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	item, err := action(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Import 批量导入题目。
//
//	@Summary  批量导入题目
//	@Description 收 JSON 数组（前端把表格转成 JSON 再提交）。先全量校验再一次性落库，
//	@Description 任一行不合法则一条都不写入，错误清单带行号返回。
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.QuizQuestionImportReq true "题目数组"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions/import [post]
func (h *AdminQuizHandler) Import(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.QuizQuestionImportReq
	if !BindJSON(c, &req) {
		return
	}
	result, err := h.quizzes.Import(c.Request.Context(), adminID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Review 审核投稿题目。
//
//	@Summary  审核题目投稿
//	@Description 只有 pending 状态可审；驳回必须填原因。通过后题目立即发布并给作者记 5 贡献分。
//	@Tags     admin-kb
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                  true "题目 ID"
//	@Param    body body dto.QuizReviewReq    true "审核动作"
//	@Success  200 {object} response.Body
//	@Router   /admin/kb/quiz/questions/{id}/review [post]
func (h *AdminQuizHandler) Review(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	reviewerID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.QuizReviewReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.quizzes.ReviewQuestion(c.Request.Context(), id, reviewerID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}
