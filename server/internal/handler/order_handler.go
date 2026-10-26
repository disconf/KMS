package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// OrderHandler 悬赏问题、订单与接单大厅的用户端接口。
//
// 路由分三层挂载，因为三层的身份要求不同：
// 大厅是专家才看得到的（游客进来看到一堆别人的悬赏没有意义），
// 悬赏与订单是登录即可的，而争议处理在后台 handler 里。
type OrderHandler struct {
	orders *service.OrderService
}

// NewOrderHandler 创建 OrderHandler。
func NewOrderHandler(orders *service.OrderService) *OrderHandler {
	return &OrderHandler{orders: orders}
}

/* ============================ 悬赏问题 ============================ */

// CreateQuestion 发布悬赏问题。
//
//	@Summary  发布悬赏
//	@Tags     question
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.QuestionCreateReq true "悬赏内容"
//	@Success  201 {object} response.Body
//	@Router   /questions [post]
func (h *OrderHandler) CreateQuestion(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.QuestionCreateReq
	if !BindJSON(c, &req) {
		return
	}
	detail, err := h.orders.CreateQuestion(c.Request.Context(), userID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, detail)
}

// ListMyQuestions 我发布的悬赏。
//
//	@Summary  我的悬赏
//	@Tags     question
//	@Security BearerAuth
//	@Produce  json
//	@Param    status query string false "问题状态"
//	@Param    sort   query string false "排序 latest|amount"
//	@Success  200 {object} response.Body
//	@Router   /questions [get]
func (h *OrderHandler) ListMyQuestions(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var q dto.QuestionListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()
	items, total, err := h.orders.ListMyQuestions(c.Request.Context(), userID, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// QuestionDetail 悬赏详情。
//
//	@Summary  悬赏详情
//	@Tags     question
//	@Security BearerAuth
//	@Produce  json
//	@Param    id path int true "问题 ID"
//	@Success  200 {object} response.Body
//	@Router   /questions/{id} [get]
func (h *OrderHandler) QuestionDetail(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.orders.QuestionDetail(c.Request.Context(), userID, id, isAdmin(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// CancelQuestion 取消悬赏。
//
//	@Summary  取消悬赏
//	@Tags     question
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    id   path int                    true "问题 ID"
//	@Param    body body dto.QuestionCancelReq  true "取消原因"
//	@Success  200 {object} response.Body
//	@Router   /questions/{id}/cancel [post]
func (h *OrderHandler) CancelQuestion(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var req dto.QuestionCancelReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.orders.CancelQuestion(c.Request.Context(), userID, id, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"cancelled": true})
}

/* ============================ 接单大厅 ============================ */

// Hall 接单大厅。
//
//	@Summary  接单大厅
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    skillId query string false "技能 ID"
//	@Param    sort    query string false "排序 latest|amount|expiring"
//	@Success  200 {object} response.Body
//	@Router   /orders/hall [get]
func (h *OrderHandler) Hall(c *gin.Context) {
	var q dto.HallListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()
	items, total, err := h.orders.Hall(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Grab 抢单。
//
//	@Summary  抢单
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    orderNo path string true "订单号"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/grab [post]
func (h *OrderHandler) Grab(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	detail, err := h.orders.Grab(c.Request.Context(), userID, orderNo)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

/* ============================ 订单 ============================ */

// CreateBooking 预约专家下单。
//
//	@Summary  预约下单
//	@Tags     order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.OrderCreateReq true "预约信息"
//	@Success  201 {object} response.Body
//	@Router   /orders [post]
func (h *OrderHandler) CreateBooking(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.OrderCreateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.orders.CreateBooking(c.Request.Context(), userID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// List 订单列表。
//
//	@Summary  订单列表
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    side    query string false "user=我下的单 expert=我接的单"
//	@Param    status  query string false "订单状态"
//	@Param    type    query string false "question|booking"
//	@Param    keyword query string false "订单号或标题"
//	@Success  200 {object} response.Body
//	@Router   /orders [get]
func (h *OrderHandler) List(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var q dto.OrderListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()
	items, total, err := h.orders.ListOrders(c.Request.Context(), userID, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Counts 订单角标。
//
//	@Summary  订单各状态数量
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    side query string false "user|expert"
//	@Success  200 {object} response.Body
//	@Router   /orders/counts [get]
func (h *OrderHandler) Counts(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	counts, err := h.orders.OrderCounts(c.Request.Context(), userID, c.Query("side"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, counts)
}

// Detail 订单详情。
//
//	@Summary  订单详情
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    orderNo path string true "订单号"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo} [get]
func (h *OrderHandler) Detail(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	detail, err := h.orders.OrderDetail(c.Request.Context(), userID, orderNo)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// PayChannels 可选的支付通道。
//
//	@Summary  可选支付通道
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /orders/pay-channels [get]
func (h *OrderHandler) PayChannels(c *gin.Context) {
	response.OK(c, gin.H{"list": h.orders.PayChannels()})
}

// Pay 发起支付。
//
//	@Summary  发起支付
//	@Tags     order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string           true "订单号"
//	@Param    body    body dto.OrderPayReq  true "支付通道"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/pay [post]
func (h *OrderHandler) Pay(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.OrderPayReq
	if !BindJSON(c, &req) {
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	result, err := h.orders.Pay(c.Request.Context(), userID, orderNo, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Cancel 取消订单。
//
//	@Summary  取消订单
//	@Tags     order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string              true "订单号"
//	@Param    body    body dto.OrderCancelReq  true "取消原因"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/cancel [post]
func (h *OrderHandler) Cancel(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.OrderCancelReq
	if !BindJSON(c, &req) {
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	if err := h.orders.Cancel(c.Request.Context(), userID, orderNo, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"cancelled": true})
}

/* ============================ 服务会话 ============================ */

// StartSession 开始服务会话。
//
// channel 只在需要**新建**会话时必填：如果订单已有进行中的会话，
// 这个接口会直接把它复用返回，此时不传 channel 也能成功。
//
//	@Summary  开始会话
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    orderNo path  string true  "订单号"
//	@Param    channel query string false "im|voice|video|screen（已有会话时可省略）"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/sessions [post]
func (h *OrderHandler) StartSession(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	item, err := h.orders.StartSession(c.Request.Context(), userID, orderNo, c.Query("channel"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// EndSession 结束当前会话。
//
//	@Summary  结束会话
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    orderNo path string true "订单号"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/sessions/end [post]
func (h *OrderHandler) EndSession(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	item, err := h.orders.EndSession(c.Request.Context(), userID, orderNo)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

/* ============================ 履约推进 ============================ */

// Finish 专家标记服务完成。
//
//	@Summary  标记服务完成
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    orderNo path string true "订单号"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/finish [post]
func (h *OrderHandler) Finish(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	if err := h.orders.FinishService(c.Request.Context(), userID, orderNo); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"finished": true})
}

// Confirm 用户确认完成（随后自动结算）。
//
//	@Summary  确认完成
//	@Tags     order
//	@Security BearerAuth
//	@Produce  json
//	@Param    orderNo path string true "订单号"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/confirm [post]
func (h *OrderHandler) Confirm(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	if err := h.orders.ConfirmComplete(c.Request.Context(), userID, orderNo); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"confirmed": true})
}

/* ============================ 评价与争议 ============================ */

// Review 提交评价。
//
//	@Summary  提交评价
//	@Tags     order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string              true "订单号"
//	@Param    body    body dto.OrderReviewReq  true "评价内容"
//	@Success  201 {object} response.Body
//	@Router   /orders/{orderNo}/review [post]
func (h *OrderHandler) Review(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.OrderReviewReq
	if !BindJSON(c, &req) {
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	item, err := h.orders.Review(c.Request.Context(), userID, orderNo, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// ReplyReview 专家回复评价。
//
//	@Summary  回复评价
//	@Tags     order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string                    true "订单号"
//	@Param    body    body dto.OrderReviewReplyReq   true "回复内容"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/review/reply [post]
func (h *OrderHandler) ReplyReview(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.OrderReviewReplyReq
	if !BindJSON(c, &req) {
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	if err := h.orders.ReplyReview(c.Request.Context(), userID, orderNo, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"replied": true})
}

// AppealReview 专家申诉评价。
//
//	@Summary  申诉评价
//	@Tags     order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string                     true "订单号"
//	@Param    body    body dto.OrderReviewAppealReq   true "申诉理由"
//	@Success  200 {object} response.Body
//	@Router   /orders/{orderNo}/review/appeal [post]
func (h *OrderHandler) AppealReview(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.OrderReviewAppealReq
	if !BindJSON(c, &req) {
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	if err := h.orders.AppealReview(c.Request.Context(), userID, orderNo, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"appealed": true})
}

// RaiseDispute 发起争议。
//
//	@Summary  发起争议
//	@Tags     order
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    orderNo path string               true "订单号"
//	@Param    body    body dto.OrderDisputeReq  true "争议内容"
//	@Success  201 {object} response.Body
//	@Router   /orders/{orderNo}/dispute [post]
func (h *OrderHandler) RaiseDispute(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.OrderDisputeReq
	if !BindJSON(c, &req) {
		return
	}
	orderNo, ok := orderNoParam(c)
	if !ok {
		return
	}
	item, err := h.orders.RaiseDispute(c.Request.Context(), userID, orderNo, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

/* ============================ 内部 ============================ */

// orderNoParam 读订单号路径参数并做形状校验。
//
// 订单号是路由里唯一的字符串主键，光判非空还不够：把自增 id 当订单号传进来
// 是前端最容易犯的错，直接放过就是一次必然查不到库的往返，日志里还会留下
// 一堆"订单不存在"掩盖真正的问题。形状规则由 idgen 提供——订单号的格式
// 定义在那里，校验也应当跟着它走。存在性仍然由仓储层回答。
func orderNoParam(c *gin.Context) (string, bool) {
	raw := strings.TrimSpace(c.Param("orderNo"))
	if !idgen.ValidOrderNo(raw) {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("订单号不合法"))
		return "", false
	}
	return raw, true
}

// isAdmin 判断当前登录身份是否为管理员。
//
// 详情接口用一个 isAdmin 布尔而不是再挂一套 admin 路由：同一个 URL 下
// 管理员越权读取是"看得更多"，不是"做别的事"，多一条路由只会让前端
// 需要在两套地址之间挑。
func isAdmin(c *gin.Context) bool {
	return authctx.Role(c) == model.RoleAdmin
}
