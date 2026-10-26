package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/service"
)

// QaHandler 问答广场接口。
//
// 读接口允许游客（OptionalAuth），写接口挂在登录组下。
// 不把读也强制登录：问答是最典型的搜索引擎落地页场景，
// 拦在登录后面等于放弃这块流量。
type QaHandler struct {
	qa *service.QaService
}

// NewQaHandler 创建 QaHandler。
func NewQaHandler(qa *service.QaService) *QaHandler { return &QaHandler{qa: qa} }

// viewer 从上下文取当前访问者，未登录返回空值。
func qaViewer(c *gin.Context) service.QaViewer {
	id, ok := authctx.Get(c)
	if !ok {
		return service.QaViewer{}
	}
	return service.QaViewer{UserID: id.UserID, Role: id.Role, ExpertStatus: id.ExpertStatus}
}

// List 问题列表。
//
//	@Summary  问答列表
//	@Tags     qa
//	@Produce  json
//	@Param    page     query int    false "页码"
//	@Param    pageSize query int    false "每页条数"
//	@Param    status   query string false "open|resolved|closed"
//	@Param    skillId  query int    false "技术标签 ID"
//	@Param    unanswered query bool  false "只看零回答"
//	@Param    keyword  query string false "关键词"
//	@Success  200 {object} response.Body
//	@Router   /qa/questions [get]
func (h *QaHandler) List(c *gin.Context) {
	var q dto.PageQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	f := repository.QaFilter{
		Status:        strings.TrimSpace(c.Query("status")),
		Keyword:       strings.TrimSpace(c.Query("keyword")),
		KeywordGroups: service.SearchGroups(c.Query("keyword")),
		Unanswered:    c.Query("unanswered") == "true" || c.Query("unanswered") == "1",
	}
	if id, err := strconv.ParseInt(c.Query("skillId"), 10, 64); err == nil && id > 0 {
		f.SkillID = id
	}

	page, err := h.qa.List(c.Request.Context(), f, q, qaViewer(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// Detail 问题详情。
//
//	@Summary  问答详情
//	@Tags     qa
//	@Produce  json
//	@Param    id path int true "问题 ID"
//	@Success  200 {object} response.Body
//	@Router   /qa/questions/{id} [get]
func (h *QaHandler) Detail(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.qa.Detail(c.Request.Context(), id, qaViewer(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Ask 发布问题。
//
//	@Summary  发布问题
//	@Tags     qa
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.QaAskReq true "问题"
//	@Success  200 {object} response.Body
//	@Router   /qa/questions [post]
func (h *QaHandler) Ask(c *gin.Context) {
	var req dto.QaAskReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.qa.Ask(c.Request.Context(), qaViewer(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Answer 发布回答。
//
//	@Summary  回答问题
//	@Tags     qa
//	@Accept   json
//	@Produce  json
//	@Param    id   path int            true "问题 ID"
//	@Param    body body dto.QaAnswerReq true "回答"
//	@Success  200 {object} response.Body
//	@Router   /qa/questions/{id}/answers [post]
func (h *QaHandler) Answer(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.QaAnswerReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.qa.Answer(c.Request.Context(), qaViewer(c), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// UpdateAnswer 编辑自己的回答。
//
//	@Summary  编辑回答
//	@Tags     qa
//	@Accept   json
//	@Produce  json
//	@Param    id   path int             true "回答 ID"
//	@Param    body body dto.QaAnswerReq true "回答"
//	@Success  200 {object} response.Body
//	@Router   /qa/answers/{id} [put]
func (h *QaHandler) UpdateAnswer(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.QaAnswerReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.qa.UpdateAnswer(c.Request.Context(), qaViewer(c), id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"updated": true})
}

// DeleteAnswer 删除回答。
//
//	@Summary  删除回答
//	@Tags     qa
//	@Produce  json
//	@Param    id path int true "回答 ID"
//	@Success  200 {object} response.Body
//	@Router   /qa/answers/{id} [delete]
func (h *QaHandler) DeleteAnswer(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.qa.DeleteAnswer(c.Request.Context(), qaViewer(c), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Accept 采纳回答。
//
//	@Summary  采纳回答
//	@Tags     qa
//	@Produce  json
//	@Param    id       path int true "问题 ID"
//	@Param    answerId path int true "回答 ID"
//	@Success  200 {object} response.Body
//	@Router   /qa/questions/{id}/accept/{answerId} [post]
func (h *QaHandler) Accept(c *gin.Context) {
	qid, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	aid, ok := PathInt64(c, "answerId")
	if !ok {
		return
	}
	if err := h.qa.Accept(c.Request.Context(), qaViewer(c), qid, aid); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"accepted": true})
}

// Close 关闭问题。
//
//	@Summary  关闭问题
//	@Tags     qa
//	@Produce  json
//	@Param    id path int true "问题 ID"
//	@Success  200 {object} response.Body
//	@Router   /qa/questions/{id}/close [post]
func (h *QaHandler) Close(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.qa.Close(c.Request.Context(), qaViewer(c), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"closed": true})
}

// Delete 删除自己的问题。
//
//	@Summary  删除问题
//	@Tags     qa
//	@Produce  json
//	@Param    id path int true "问题 ID"
//	@Success  200 {object} response.Body
//	@Router   /qa/questions/{id} [delete]
func (h *QaHandler) Delete(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	if err := h.qa.DeleteQuestion(c.Request.Context(), qaViewer(c), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Like 点赞回答。
//
//	@Summary  点赞回答
//	@Tags     qa
//	@Produce  json
//	@Param    id path int true "回答 ID"
//	@Success  200 {object} response.Body
//	@Router   /qa/answers/{id}/like [post]
func (h *QaHandler) Like(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	liked, score, err := h.qa.Like(c.Request.Context(), qaViewer(c), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"liked": liked, "score": score})
}

// Unlike 取消点赞。
//
//	@Summary  取消点赞
//	@Tags     qa
//	@Produce  json
//	@Param    id path int true "回答 ID"
//	@Success  200 {object} response.Body
//	@Router   /qa/answers/{id}/like [delete]
func (h *QaHandler) Unlike(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	score, err := h.qa.Unlike(c.Request.Context(), qaViewer(c), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"liked": false, "score": score})
}

// MyPoints 我的贡献积分。
//
//	@Summary  我的贡献
//	@Tags     qa
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /qa/my/points [get]
func (h *QaHandler) MyPoints(c *gin.Context) {
	resp, err := h.qa.MyPoints(c.Request.Context(), qaViewer(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// ============================ 后台 ============================

// AdminPending 待审回答列表。
//
//	@Summary  待审回答
//	@Tags     admin-qa
//	@Produce  json
//	@Param    page     query int false "页码"
//	@Param    pageSize query int false "每页条数"
//	@Success  200 {object} response.Body
//	@Router   /admin/qa/answers/pending [get]
func (h *QaHandler) AdminPending(c *gin.Context) {
	var q dto.PageQuery
	if !BindQuery(c, &q) {
		return
	}
	list, err := h.qa.PendingAnswers(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

// AdminReview 审核回答。
//
//	@Summary  审核回答
//	@Tags     admin-qa
//	@Accept   json
//	@Produce  json
//	@Param    id   path int              true "回答 ID"
//	@Param    body body dto.QaReviewReq  true "审核"
//	@Success  200 {object} response.Body
//	@Router   /admin/qa/answers/{id}/review [post]
func (h *QaHandler) AdminReview(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.QaReviewReq
	if !BindJSON(c, &req) {
		return
	}
	idn, _ := authctx.Get(c)
	if err := h.qa.ReviewAnswer(c.Request.Context(), idn.UserID, idn.Role, id, req, auditMeta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"reviewed": true})
}

// AdminToArticle 问答转教程草稿。
//
//	@Summary  问答转教程
//	@Tags     admin-qa
//	@Accept   json
//	@Produce  json
//	@Param    id   path int              true "问题 ID"
//	@Param    body body dto.QaToArticleReq true "转换参数"
//	@Success  200 {object} response.Body
//	@Router   /admin/qa/questions/{id}/to-article [post]
func (h *QaHandler) AdminToArticle(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.QaToArticleReq
	if !BindJSON(c, &req) {
		return
	}
	idn, _ := authctx.Get(c)
	resp, err := h.qa.ToArticle(c.Request.Context(), idn.UserID, idn.Role, id, req, auditMeta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// AdminDeleteQuestion 后台删帖（违规内容）。
//
//	@Summary  后台删除问题
//	@Tags     admin-qa
//	@Produce  json
//	@Param    id path int true "问题 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/qa/questions/{id} [delete]
func (h *QaHandler) AdminDeleteQuestion(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	idn, _ := authctx.Get(c)
	viewer := service.QaViewer{UserID: idn.UserID, Role: model.RoleAdmin, ExpertStatus: idn.ExpertStatus}
	if err := h.qa.DeleteQuestion(c.Request.Context(), viewer, id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// 后台删除问题不额外校验：AdminDeleteQuestion 里已把 viewer 角色置为 admin，
// 走的是与前台同一套权限判断，避免"后台一套、前台一套"两处口径不一致。
