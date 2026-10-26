package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminOrderHandler 后台订单管理。
//
// 与用户端 handler 分开，因为两者的授权模型不同：这里整组要求 admin 角色，
// 并且只有这里的接口能"越过订单双方"去看和改。混在一个 handler 里，
// 后来加接口的人很容易把一条后台能力挂到用户端路由组上。
type AdminOrderHandler struct {
	orders *service.OrderService
}

// NewAdminOrderHandler 创建 AdminOrderHandler。
func NewAdminOrderHandler(orders *service.OrderService) *AdminOrderHandler {
	return &AdminOrderHandler{orders: orders}
}

// List 订单列表（全量筛选）。
//
//	@Summary  后台订单列表
//	@Tags     admin-order
//	@Security BearerAuth
//	@Produce  json
//	@Param    status    query string false "订单状态"
//	@Param    type      query string false "question|booking"
//	@Param    keyword   query string false "订单号或标题"
//	@Param    expertId  query int    false "专家 ID"
//	@Param    userId    query int    false "下单人 ID"
//	@Success  200 {object} response.Body
//	@Router   /admin/orders [get]
func (h *AdminOrderHandler) List(c *gin.Context) {
	var q dto.AdminOrderListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()
	items, total, err := h.orders.AdminList(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Detail 订单详情（不受"只有双方能看"限制）。
//
//	@Summary  后台订单详情
//	@Tags     admin-order
//	@Security BearerAuth
//	@Produce  json
//	@Param    orderNo path string true "订单号"
//	@Success  200 {object} response.Body
//	@Router   /admin/orders/{orderNo} [get]
func (h *AdminOrderHandler) Detail(c *gin.Context) {
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	detail, err := h.orders.AdminDetail(c.Request.Context(), orderNo)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// MarkPaid 人工确认收款（offline 通道）。
//
//	@Summary  人工确认收款
//	@Tags     admin-order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string                        true "订单号"
//	@Param    body    body dto.AdminOrderMarkPaidReq     true "流水号"
//	@Success  200 {object} response.Body
//	@Router   /admin/orders/{orderNo}/mark-paid [post]
func (h *AdminOrderHandler) MarkPaid(c *gin.Context) {
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	var req dto.AdminOrderMarkPaidReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.orders.AdminMarkPaid(c.Request.Context(), orderNo, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"paid": true})
}

// Transition 强制流转状态（仍受状态机约束）。
//
//	@Summary  强制流转订单状态
//	@Tags     admin-order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string                        true "订单号"
//	@Param    body    body dto.AdminOrderTransitionReq   true "目标状态"
//	@Success  200 {object} response.Body
//	@Router   /admin/orders/{orderNo}/transition [post]
func (h *AdminOrderHandler) Transition(c *gin.Context) {
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	var req dto.AdminOrderTransitionReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.orders.AdminTransition(c.Request.Context(), orderNo, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"transitioned": true})
}

// Settle 手动触发结算。
//
//	@Summary  手动结算
//	@Tags     admin-order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string            true "订单号"
//	@Param    body    body dto.AdminSettleReq true "备注"
//	@Success  200 {object} response.Body
//	@Router   /admin/orders/{orderNo}/settle [post]
func (h *AdminOrderHandler) Settle(c *gin.Context) {
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	var req dto.AdminSettleReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.orders.AdminSettle(c.Request.Context(), orderNo, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"settled": true})
}

// ResolveDispute 处理争议。
//
//	@Summary  处理争议
//	@Tags     admin-order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                            true "争议 ID"
//	@Param    body body dto.AdminResolveDisputeReq     true "处理结论"
//	@Success  200 {object} response.Body
//	@Router   /admin/disputes/{id}/resolve [post]
func (h *AdminOrderHandler) ResolveDispute(c *gin.Context) {
	adminID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.AdminResolveDisputeReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.orders.AdminResolveDispute(c.Request.Context(), adminID, id, req.Resolution, req.Result); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"resolved": true})
}
