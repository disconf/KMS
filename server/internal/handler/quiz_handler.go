package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// QuizHandler 运维题库的前台接口：浏览、练习、作答、错题本。
type QuizHandler struct {
	quizzes *service.QuizService
}

// NewQuizHandler 创建 QuizHandler。
func NewQuizHandler(quizzes *service.QuizService) *QuizHandler {
	return &QuizHandler{quizzes: quizzes}
}

// Categories 题库分类卡片：每个运维方向的题目数与我的刷题进度。
//
//	@Summary  题库分类（按运维方向刷题）
//	@Tags     kb-quiz
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /quiz/categories [get]
func (h *QuizHandler) Categories(c *gin.Context) {
	// 用 OptionalAuth 的登录态：游客也能看到分类与题数，只是进度全为 0。
	items, err := h.quizzes.Categories(c.Request.Context(), authctx.UserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// List 题目列表。出参不含答案与解析。//
//
//	@Summary  题目列表
//	@Tags     kb-quiz
//	@Produce  json
//	@Param    page         query int    false "页码"
//	@Param    pageSize     query int    false "每页条数"
//	@Param    categoryId   query int    false "分类 ID"
//	@Param    categorySlug query string false "分类标识"
//	@Param    keyword      query string false "关键词"
//	@Param    difficulty   query string false "难度 easy|medium|hard"
//	@Param    type         query string false "题型 single|multiple|judge"
//	@Param    tag          query string false "标签"
//	@Param    sort         query string false "排序 latest|hot|hard"
//	@Success  200 {object} response.Body
//	@Router   /quiz/questions [get]
func (h *QuizHandler) List(c *gin.Context) {
	var q dto.QuizListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.quizzes.ListQuestions(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Practice 随机抽题组成一轮练习。抽不到题时返回业务错误而非空列表。
//
//	@Summary  随机抽题
//	@Tags     kb-quiz
//	@Produce  json
//	@Param    count      query int    false "抽题数量，默认 10，上限 50"
//	@Param    categoryId query int    false "分类 ID"
//	@Param    difficulty query string false "难度"
//	@Param    type       query string false "题型"
//	@Success  200 {object} response.Body
//	@Router   /quiz/practice [get]
func (h *QuizHandler) Practice(c *gin.Context) {
	var q dto.QuizPracticeQuery
	if !BindQuery(c, &q) {
		return
	}
	items, err := h.quizzes.Practice(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// Answer 单题作答并即时判分，返回正确答案与解析。
//
//	@Summary  单题作答
//	@Tags     kb-quiz
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                true "题目 ID"
//	@Param    body body dto.QuizAnswerReq  true "作答内容"
//	@Success  200 {object} response.Body
//	@Router   /quiz/questions/{id}/answer [post]
func (h *QuizHandler) Answer(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.QuizAnswerReq
	if !BindJSON(c, &req) {
		return
	}
	result, err := h.quizzes.Answer(c.Request.Context(), userID, id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Submit 整卷提交，返回成绩单与逐题解析。
//
//	@Summary  交卷判分
//	@Tags     kb-quiz
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.QuizSubmitReq true "作答内容"
//	@Success  201 {object} response.Body
//	@Router   /quiz/attempts [post]
func (h *QuizHandler) Submit(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.QuizSubmitReq
	if !BindJSON(c, &req) {
		return
	}
	result, err := h.quizzes.Submit(c.Request.Context(), userID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, result)
}

// Attempts 答题历史。
//
//	@Summary  答题历史
//	@Tags     kb-quiz
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /quiz/attempts [get]
func (h *QuizHandler) Attempts(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.quizzes.Attempts(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// Stats 个人答题总览。
//
//	@Summary  我的答题统计
//	@Tags     kb-quiz
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /quiz/stats [get]
func (h *QuizHandler) Stats(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	stats, err := h.quizzes.Stats(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, stats)
}

// WrongBook 错题本。口径是「最近一次作答仍然答错」。
//
//	@Summary  错题本
//	@Tags     kb-quiz
//	@Security BearerAuth
//	@Produce  json
//	@Param    page       query int false "页码"
//	@Param    pageSize   query int false "每页条数"
//	@Param    categoryId query int false "分类 ID"
//	@Success  200 {object} response.Body
//	@Router   /quiz/wrong-book [get]
func (h *QuizHandler) WrongBook(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var q dto.QuizWrongQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.quizzes.WrongBook(c.Request.Context(), userID, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Contribute 用户投稿一道题（需登录）。
//
//	@Summary  投稿题目
//	@Description 投稿固定进入待审核，不接受调用方指定状态；解析必填。
//	@Description 已过审专家与管理员直发（与教程投稿同一套免审判定）。
//	@Tags     kb-quiz
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.QuizContributeReq true "题目内容"
//	@Success  201 {object} response.Body
//	@Router   /quiz/contribute [post]
func (h *QuizHandler) Contribute(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.QuizContributeReq
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
	// 题目投稿同样属于"发布"，只读账号一律挡住。
	if !RequireSpeak(c) {
		return
	}
	resp, err := h.quizzes.Contribute(c.Request.Context(), userID, req, direct)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, resp)
}

// MyContributions 我投的题（需登录）。
//
//	@Summary  我的题目投稿
//	@Tags     kb-quiz
//	@Security BearerAuth
//	@Produce  json
//	@Param    page     query int false "页码"
//	@Param    pageSize query int false "每页条数"
//	@Success  200 {object} response.Body
//	@Router   /quiz/my/contributions [get]
func (h *QuizHandler) MyContributions(c *gin.Context) {
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
	items, total, err := h.quizzes.MyContributions(c.Request.Context(), userID, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}
