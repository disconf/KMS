package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// ExpertHandler 专家域接口：公开展示 + 专家端自助操作。
//
// 公开展示与专家端分成两组路由挂载，因为两者的鉴权强度完全不同：
// 前者允许游客，后者必须登录且（多数操作）必须已通过审核。
type ExpertHandler struct {
	experts *service.ExpertService
}

// NewExpertHandler 创建 ExpertHandler。
func NewExpertHandler(experts *service.ExpertService) *ExpertHandler {
	return &ExpertHandler{experts: experts}
}

// ============================ 公开展示 ============================

// List 专家列表，支持按技能、价格、评分、在线状态筛选。
//
//	@Summary  专家列表
//	@Tags     expert
//	@Produce  json
//	@Param    page          query int     false "页码"
//	@Param    pageSize      query int     false "每页条数"
//	@Param    skillId       query int     false "技能 ID"
//	@Param    keyword       query string  false "昵称或技能关键词"
//	@Param    minPriceCents query int     false "最低时薪（分）"
//	@Param    maxPriceCents query int     false "最高时薪（分）"
//	@Param    onlineOnly    query bool    false "仅看接单中的专家"
//	@Param    minRating     query number  false "最低评分"
//	@Param    sort          query string  false "排序 rating|orders|price_asc|price_desc|latest"
//	@Success  200 {object} response.Body
//	@Router   /experts [get]
func (h *ExpertHandler) List(c *gin.Context) {
	var q dto.ExpertListQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	items, total, err := h.experts.ListExperts(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, q.Page, q.PageSize)
}

// Detail 专家公开详情。仅审核通过的专家可见，其余一律按不存在返回。
//
//	@Summary  专家详情
//	@Tags     expert
//	@Produce  json
//	@Param    id path int true "专家 ID"
//	@Success  200 {object} response.Body
//	@Router   /experts/{id} [get]
func (h *ExpertHandler) Detail(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	detail, err := h.experts.ExpertDetail(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Reviews 专家公开评价分页。
//
// 与详情分开：评价要能"加载更多"，塞进详情会让首屏返回体随评价数增长。
// 汇总（总数 / 均分 / 星级分布）跟着这一页一起下发，前端不必再请求一次。
//
//	@Summary  专家评价列表
//	@Tags     expert
//	@Produce  json
//	@Param    id       path int true  "专家 ID"
//	@Param    page     query int false "页码，默认 1"
//	@Param    pageSize query int false "每页条数，默认 20，上限 100"
//	@Success  200 {object} response.Body
//	@Router   /experts/{id}/reviews [get]
func (h *ExpertHandler) Reviews(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	var q dto.PageQuery
	if !BindQuery(c, &q) {
		return
	}
	q.Normalize()

	page, err := h.experts.ReviewsOfExpert(c.Request.Context(), id, q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, page)
}

// Skills 技能字典。申请表单与筛选器共用同一份数据。
//
//	@Summary  技能字典
//	@Tags     expert
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /skills [get]
func (h *ExpertHandler) Skills(c *gin.Context) {
	items, err := h.experts.ListSkills(c.Request.Context(), true)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}

// ============================ 入驻申请 ============================

// Apply 提交专家入驻申请。
//
// 刻意不加 RequireApprovedExpert：这个接口本来就是给"还不是专家"的人用的。
// 但也刻意不接收 userID 参数——身份只能来自令牌，否则可以替别人提交资质。
//
//	@Summary  提交入驻申请
//	@Tags     expert
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ExpertApplyReq true "申请材料"
//	@Success  201 {object} response.Body
//	@Router   /expert/apply [post]
func (h *ExpertHandler) Apply(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ExpertApplyReq
	if !BindJSON(c, &req) {
		return
	}
	status, err := h.experts.Apply(c.Request.Context(), userID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, status)
}

// ApplyStatus 查询自己的入驻申请状态。
//
// 从未申请过时返回 status 为空串而不是 404：前端只需按 status 分派页面，
// 不必为一个"还没申请"的正常状态单独处理错误分支。
//
//	@Summary  入驻申请状态
//	@Tags     expert
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /expert/apply [get]
func (h *ExpertHandler) ApplyStatus(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	status, err := h.experts.ApplyStatus(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, status)
}

// ============================ 专家端 ============================

// Dashboard 专家工作台概览。
//
// 只要求角色为 expert，不要求审核通过：被驳回 / 审核中的用户同样要能进来
// 看到自己的状态横幅，否则一进工作台就是 403 空白页。
//
//	@Summary  专家工作台
//	@Tags     expert-self
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /expert/dashboard [get]
func (h *ExpertHandler) Dashboard(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	data, err := h.experts.Dashboard(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// UpdateServices 更新服务设置。
//
//	@Summary  更新服务设置
//	@Tags     expert-self
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ExpertServiceUpdateReq true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /expert/services [put]
func (h *ExpertHandler) UpdateServices(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ExpertServiceUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.experts.UpdateServices(c.Request.Context(), userID, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"updated": true})
}

// UpdateProfile 更新对外主页内容。
//
//	@Summary  更新专家主页
//	@Tags     expert-self
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ExpertProfileUpdateReq true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /expert/profile [put]
func (h *ExpertHandler) UpdateProfile(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ExpertProfileUpdateReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.experts.UpdateProfile(c.Request.Context(), userID, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"updated": true})
}

// Wallet 收入钱包：余额、冻结金额、累计收入与最近明细。
//
//	@Summary  收入钱包
//	@Tags     expert-self
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /expert/wallet [get]
func (h *ExpertHandler) Wallet(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	data, err := h.experts.Wallet(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, data)
}

// CreateWithdrawal 申请提现。
//
//	@Summary  申请提现
//	@Tags     expert-self
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.WithdrawalCreateReq true "提现信息"
//	@Success  201 {object} response.Body
//	@Router   /expert/withdrawals [post]
func (h *ExpertHandler) CreateWithdrawal(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.WithdrawalCreateReq
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.experts.CreateWithdrawal(c.Request.Context(), userID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// ListWithdrawals 提现记录。
//
//	@Summary  提现记录
//	@Tags     expert-self
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /expert/withdrawals [get]
func (h *ExpertHandler) ListWithdrawals(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	items, err := h.experts.ListWithdrawals(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"list": items})
}
