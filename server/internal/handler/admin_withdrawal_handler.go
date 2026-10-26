package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminWithdrawalHandler 提现审批后台接口。整组由 admin 角色守卫保护。
//
// 这几条是「钱离开平台」的唯一出口：审核通过只是把钱从待审核挪到待打款，
// 真正扣冻结在标记已打款那一步。所以打款必须单独一个动作，
// 不能做成「通过即打款」——那样等于把核对收款人的步骤省掉了。
type AdminWithdrawalHandler struct {
	experts *service.ExpertService
}

// NewAdminWithdrawalHandler 创建 AdminWithdrawalHandler。
func NewAdminWithdrawalHandler(experts *service.ExpertService) *AdminWithdrawalHandler {
	return &AdminWithdrawalHandler{experts: experts}
}

// List 提现列表（可按状态与提现单号 / 专家姓名筛选）。
//
//	@Summary  后台提现列表
//	@Tags     admin-withdrawal
//	@Security BearerAuth
//	@Produce  json
//	@Param    page     query int    false "页码"
//	@Param    pageSize query int    false "每页条数"
//	@Param    status   query string false "pending|approved|paid|rejected|failed"
//	@Param    keyword  query string false "提现单号或专家姓名"
//	@Success  200 {object} response.Body
//	@Router   /admin/withdrawals [get]
func (h *AdminWithdrawalHandler) List(c *gin.Context) {
	var q dto.AdminWithdrawalListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.experts.AdminListWithdrawals(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Review 审核通过或驳回。
//
// 审核人 ID 只从令牌取：钱的操作必须能追到人，允许前端传操作人等于允许伪造审批记录。
//
//	@Summary  审核提现
//	@Tags     admin-withdrawal
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                       true "提现 ID"
//	@Param    body body dto.WithdrawalReviewReq   true "approve | reject + 备注"
//	@Success  200 {object} response.Body
//	@Router   /admin/withdrawals/{id}/review [post]
func (h *AdminWithdrawalHandler) Review(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.WithdrawalReviewReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.experts.AdminReviewWithdrawal(c.Request.Context(), adminID, id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"reviewed": true})
}

// MarkPaid 标记已打款（线下转账 / 渠道回执核对后）。
//
//	@Summary  标记提现已打款
//	@Tags     admin-withdrawal
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                     true "提现 ID"
//	@Param    body body dto.WithdrawalPaidReq   true "流水号或备注"
//	@Success  200 {object} response.Body
//	@Router   /admin/withdrawals/{id}/mark-paid [post]
func (h *AdminWithdrawalHandler) MarkPaid(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.WithdrawalPaidReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.experts.AdminMarkWithdrawalPaid(c.Request.Context(), adminID, id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"paid": true})
}

// Fail 标记打款失败，冻结金额退回余额。
//
//	@Summary  标记提现打款失败
//	@Tags     admin-withdrawal
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                     true "提现 ID"
//	@Param    body body dto.WithdrawalPaidReq   true "失败原因（必填）"
//	@Success  200 {object} response.Body
//	@Router   /admin/withdrawals/{id}/fail [post]
func (h *AdminWithdrawalHandler) Fail(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.WithdrawalPaidReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.experts.AdminFailWithdrawal(c.Request.Context(), adminID, id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"failed": true})
}
