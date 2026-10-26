package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/storage"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/service/payment"
)

// questionKeyPrefix 悬赏附件的存储目录前缀。
//
// 与资源模块同样的道理：必须校验提交的 key 确实来自问题附件场景，
// 否则可以拿一张专家资质证书的 key 当问题附件挂出去，
// 而那个文件本来是私有的——一次引用就把私有文件变成了公开可访问。
const questionKeyPrefix = "question/"

// OrderService 悬赏问题与订单的业务逻辑。
//
// 问题与订单放在同一个 service 里，因为它们是一体的：
// 发布悬赏必然同时产生一笔订单，订单详情又必然回读问题内容。
// 拆成两个 service 会让"发布悬赏"这个动作在两者之间来回跳，
// 而它恰恰是必须原子完成的一步。
type OrderService struct {
	orders      *repository.OrderRepo
	questions   *repository.QuestionRepo
	reviews     *repository.ReviewRepo
	sessions    *repository.ServiceSessionRepo
	disputes    *repository.DisputeRepo
	settlements *repository.SettlementRepo
	experts     *repository.ExpertRepo
	users       *repository.UserRepo
	uploads     *UploadService

	pay    payment.Provider
	payCfg config.PaymentConfig
	// settings 提供后台可调的抽佣比例。nil 时退回 payCfg 里的配置值，
	// 这样只测订单状态的单测不必凭空造一套配置仓储。
	settings *SettingService
}

// NewOrderService 创建 OrderService。
func NewOrderService(
	orders *repository.OrderRepo,
	questions *repository.QuestionRepo,
	reviews *repository.ReviewRepo,
	sessions *repository.ServiceSessionRepo,
	disputes *repository.DisputeRepo,
	settlements *repository.SettlementRepo,
	experts *repository.ExpertRepo,
	users *repository.UserRepo,
	uploads *UploadService,
	pay payment.Provider,
	payCfg config.PaymentConfig,
	settings *SettingService,
) *OrderService {
	return &OrderService{
		orders: orders, questions: questions, reviews: reviews, sessions: sessions,
		disputes: disputes, settlements: settlements, experts: experts, users: users,
		uploads: uploads, pay: pay, payCfg: payCfg, settings: settings,
	}
}

// commissionRateBP 取本次下单要写进订单快照的抽佣基点。
//
// 来源优先级：后台系统配置 > 配置文件默认值。
// 查库失败直接让下单失败——不能用一个"不知道对不对"的比例算钱；
// 而错一分钱在对账时都要人工核，代价远高于让用户重试一次。
func (s *OrderService) commissionRateBP(ctx context.Context) (int, error) {
	if s.settings == nil {
		return s.payCfg.CommissionRateBP, nil
	}
	return s.settings.GetCommissionRateBP(ctx)
}

/* ============================ 悬赏问题 ============================ */

// CreateQuestion 发布悬赏问题，同时落一笔待接单的订单。
//
// 订单在发布时而不是被抢单时创建，理由是状态机里有 pending_accept 这个状态：
// 它描述的是"这笔悬赏挂在那里等人接"。若订单等到抢单才创建，
// pending_accept 就永远用不上，而抢单的并发控制也就失去了落脚点——
// 现在抢单是一句带条件的 UPDATE，正是靠这个已存在的订单行来裁决先后。
func (s *OrderService) CreateQuestion(ctx context.Context, userID int64, req dto.QuestionCreateReq) (*dto.QuestionDetail, error) {
	title, err := normalizeQuestionTitle(req.Title)
	if err != nil {
		return nil, err
	}
	desc, err := normalizeQuestionDesc(req.Description)
	if err != nil {
		return nil, err
	}
	attachments, err := s.normalizeAttachments(req.Attachments)
	if err != nil {
		return nil, err
	}
	skills, err := normalizeSkillIDs(req.SkillIDs)
	if err != nil {
		return nil, err
	}
	budget, err := normalizeBudget(req.BudgetCents)
	if err != nil {
		return nil, err
	}
	expectMin, err := normalizeExpectResponse(req.ExpectResponseMin)
	if err != nil {
		return nil, err
	}
	expireHours, err := normalizeExpireHours(req.ExpireHours)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiredAt := now.Add(time.Duration(expireHours) * time.Hour)

	q := &model.Question{
		UserID:            userID,
		Title:             title,
		Description:       desc,
		Attachments:       model.StringList(attachments),
		SkillIDs:          model.StringList(skills),
		BudgetCents:       budget,
		ExpectResponseMin: expectMin,
		Status:            model.QuestionStatusOpen,
		ExpiredAt:         expiredAt,
	}

	// 抽佣比例在下单这一刻定格，写进订单快照。
	// 之后再怎么调比例都不影响这一单——专家谈好的收入不能被后台改配置悄悄吞掉。
	rateBP, err := s.commissionRateBP(ctx)
	if err != nil {
		return nil, err
	}
	commission, expertAmount := splitCommission(budget, rateBP)
	o := &model.Order{
		OrderNo:           idgen.OrderNo(),
		Type:              model.OrderTypeQuestion,
		UserID:            userID,
		ExpertID:          0, // 还没人接
		Title:             title,
		AmountCents:       budget,
		CommissionRateBP:  rateBP,
		CommissionCents:   commission,
		ExpertAmountCents: expertAmount,
		Status:            model.OrderStatusPendingAccept,
	}

	if err := s.questions.CreateWithOrder(ctx, q, o); err != nil {
		return nil, err
	}
	return s.questionDetail(ctx, q.ID)
}

// ListMyQuestions 我发布的问题。
func (s *OrderService) ListMyQuestions(ctx context.Context, userID int64, q dto.QuestionListQuery) ([]dto.QuestionItem, int64, error) {
	q.Normalize()
	items, total, err := s.questions.List(ctx, repository.QuestionFilter{
		UserID: userID,
		Status: q.Status,
		Sort:   q.Sort,
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	return s.toQuestionItems(items), total, nil
}

// QuestionDetail 问题详情。只有发布者与管理员能看。
func (s *OrderService) QuestionDetail(ctx context.Context, userID int64, questionID int64, isAdmin bool) (*dto.QuestionDetail, error) {
	q, err := s.questions.GetByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if !isAdmin && q.UserID != userID {
		return nil, errcode.ErrForbidden.WithMsg("只能查看自己发布的问题")
	}
	return s.questionDetail(ctx, questionID)
}

// CancelQuestion 取消悬赏。
//
// 走问题的状态机而不是直接改字段：问题与订单是两个状态机，
// 取消悬赏必须同时把订单也置为已取消，否则大厅里还会挂着这个单。
func (s *OrderService) CancelQuestion(ctx context.Context, userID, questionID int64, req dto.QuestionCancelReq) error {
	q, err := s.questions.GetByID(ctx, questionID)
	if err != nil {
		return err
	}
	if q.UserID != userID {
		return errcode.ErrForbidden.WithMsg("只能取消自己发布的问题")
	}
	if q.Status != model.QuestionStatusOpen && q.Status != model.QuestionStatusMatched {
		return errcode.ErrQuestionClosed
	}

	o, err := s.orders.GetByQuestionID(ctx, questionID)
	if err != nil {
		return err
	}
	// 已经被专家接单并且付过款的问题不能撤：钱已经进来，
	// 退出必须走争议/退款，否则退款金额与平台佣金无处落。
	if o.IsPaid() {
		return errcode.ErrOrderCancelDenied.WithMsg("该悬赏已支付，请改为申请退款")
	}

	now := time.Now()
	if err := s.questions.Transition(ctx, questionID, q.Status, model.QuestionStatusCancelled,
		map[string]any{"closed_at": now}); err != nil {
		return err
	}
	if o.Status == model.OrderStatusPendingAccept || o.Status == model.OrderStatusPendingPay {
		extra := map[string]any{"cancelled_at": now}
		if reason := strings.TrimSpace(req.Reason); reason != "" {
			extra["cancel_reason"] = truncateRunes(reason, 200)
		}
		if err := s.orders.Transition(ctx, o.OrderNo, o.Status, model.OrderStatusCancelled, extra); err != nil {
			return err
		}
	}
	return nil
}

/* ============================ 接单大厅 ============================ */

// Hall 接单大厅：还没人接的悬赏。
//
// 读的是订单而不是问题表：只有"落了订单的悬赏"才算真的挂出来了，
// 读问题表的话，一个订单创建失败但问题写成功的历史脏数据会出现在大厅里，
// 专家点了却接不了单。
func (s *OrderService) Hall(ctx context.Context, q dto.HallListQuery) ([]dto.HallItem, int64, error) {
	q.Normalize()
	orders, total, err := s.orders.List(ctx, repository.OrderFilter{
		OnlyUnassigned: true,
		Status:         model.OrderStatusPendingAccept,
		Sort:           hallSort(q.Sort),
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	if len(orders) == 0 {
		return []dto.HallItem{}, total, nil
	}

	ids := make([]int64, 0, len(orders))
	for _, o := range orders {
		if o.QuestionID > 0 {
			ids = append(ids, o.QuestionID)
		}
	}
	qm, err := s.questions.ListByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.HallItem, 0, len(orders))
	for _, o := range orders {
		question, ok := qm[o.QuestionID]
		if !ok {
			continue
		}
		if q.SkillID != "" && !containsString([]string(question.SkillIDs), q.SkillID) {
			continue
		}
		items = append(items, dto.HallItem{
			QuestionItem: s.toQuestionItem(*question),
			OrderID:      o.ID,
			OrderNo:      o.OrderNo,
		})
	}
	return items, total, nil
}

// Grab 专家抢单。
//
// 入参用订单号而不是自增 ID：订单号是大厅列表里本来就有的字段，
// 前端拿着它就能发起抢单；用 ID 的话大厅响应里还得再额外带一个内部主键，
// 而这个主键除了抢单没有别的用途，属于让内部实现泄漏到接口上。
//
// 抢单前的资格校验放在 app 层，但**最终裁决交给数据库**：
// 两个专家同时点，资格都合法，真正决定谁抢到的是 repo.Grab 里那个
// WHERE expert_id = 0 AND status = 'pending_accept'。
func (s *OrderService) Grab(ctx context.Context, expertUserID int64, orderNo string) (*dto.OrderDetail, error) {
	if err := s.assertCanGrab(ctx, expertUserID); err != nil {
		return nil, err
	}

	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o.UserID == expertUserID {
		return nil, errcode.ErrQuestionOwn
	}

	// 抢到单的同时开支付倒计时：订单一进入待支付就该有截止时间，
	// 否则超时关单扫不到它，用户能无限期挂着不付款。
	now := time.Now()
	grabbed, err := s.orders.Grab(ctx, o.ID, expertUserID, now, now.Add(s.payCfg.PayTimeout.Std()))
	if err != nil {
		return nil, err
	}

	// 问题转"已匹配"。这一步失败不回滚抢单：订单已经明确归这位专家了，
	// 此时把订单退回 pending_accept 反而会把单重新放回大厅被别人抢。
	// 问题状态偏旧只会让"我的问题"页多显示一会儿"待响应"，是可见且可修的状态。
	if grabbed.QuestionID > 0 {
		_ = s.questions.Transition(ctx, grabbed.QuestionID,
			model.QuestionStatusOpen, model.QuestionStatusMatched,
			map[string]any{"matched_at": time.Now()})
	}
	return s.orderDetail(ctx, grabbed.OrderNo)
}

// assertCanGrab 校验专家具备接单资格。
func (s *OrderService) assertCanGrab(ctx context.Context, expertUserID int64) error {
	e, err := s.experts.FindByUserID(ctx, expertUserID)
	if err != nil {
		return err
	}
	if !e.IsApproved() || !e.AcceptingOrder {
		return errcode.ErrExpertNotGrabable
	}
	return nil
}

/* ============================ 预约下单 ============================ */

// CreateBooking 预约专家。
//
// 金额由服务端按专家定价算出，请求里没有金额字段——接受前端传金额
// 等于让用户自己定价。
func (s *OrderService) CreateBooking(ctx context.Context, userID int64, req dto.OrderCreateReq) (*dto.OrderItem, error) {
	e, err := s.experts.FindByUserID(ctx, req.ExpertUserID)
	if err != nil {
		return nil, err
	}
	if e.UserID == userID {
		return nil, errcode.ErrOrderSelfDealing
	}
	if !e.IsApproved() || !e.AcceptingOrder {
		return nil, errcode.ErrExpertNotAccepting
	}
	if !containsString([]string(e.ServiceModes), req.ServiceMode) {
		return nil, errcode.ErrServiceModeDenied
	}

	if req.DurationMin < 0 || req.DurationMin > dto.MaxBookingDurationMin() {
		return nil, errcode.ErrBookTimeInvalid.WithMsg(
			fmt.Sprintf("预约时长需在 0-%d 分钟之间", dto.MaxBookingDurationMin()))
	}
	if req.BookedAt != nil && req.BookedAt.Before(time.Now().Add(-time.Minute)) {
		return nil, errcode.ErrBookTimeInvalid.WithMsg("预约时间不能早于当前时间")
	}

	amount, err := bookingAmount(e.PricePerHourCents, e.PricePerTaskCents, req.DurationMin)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = fmt.Sprintf("%s 的%s服务", e.RealName, serviceModeLabel(req.ServiceMode))
	}
	title, err = normalizeQuestionTitle(title)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	payExpire := now.Add(s.payCfg.PayTimeout.Std())
	rateBP, err := s.commissionRateBP(ctx)
	if err != nil {
		return nil, err
	}
	commission, expertAmount := splitCommission(amount, rateBP)

	o := &model.Order{
		OrderNo:           idgen.OrderNo(),
		Type:              model.OrderTypeBooking,
		UserID:            userID,
		ExpertID:          e.UserID, // 用档案里读出来的用户 ID，而不是原样透传请求值
		Title:             title,
		ServiceMode:       req.ServiceMode,
		AmountCents:       amount,
		CommissionRateBP:  rateBP,
		CommissionCents:   commission,
		ExpertAmountCents: expertAmount,
		Status:            model.OrderStatusPendingPay,
		PayExpireAt:       &payExpire,
		BookedAt:          req.BookedAt,
		DurationMin:       req.DurationMin,
		Remark:            truncateRunes(req.Remark, 500),
	}
	if err := s.orders.Create(ctx, o); err != nil {
		return nil, err
	}
	item, err := s.toOrderItem(ctx, *o)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

/* ============================ 订单列表与详情 ============================ */

// ListOrders 订单列表。side=user 是我下的单，side=expert 是我接的单。
func (s *OrderService) ListOrders(ctx context.Context, userID int64, q dto.OrderListQuery) ([]dto.OrderItem, int64, error) {
	q.Normalize()
	f := repository.OrderFilter{
		Status:  q.Status,
		Type:    q.Type,
		Keyword: q.Keyword,
		Sort:    q.Sort,
	}
	if q.Side == "expert" {
		f.ExpertID = userID
		// 只看已经分配给我的：expert_id 为 0 的单在"我接的单"里没有意义。
		f.OnlyAssigned = true
	} else {
		f.UserID = userID
	}

	orders, total, err := s.orders.List(ctx, f, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items, err := s.toOrderItems(ctx, orders)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// OrderCounts 订单中心的 Tab 角标。
func (s *OrderService) OrderCounts(ctx context.Context, userID int64, side string) (*dto.OrderCountItem, error) {
	f := repository.OrderFilter{}
	if side == "expert" {
		f.ExpertID = userID
		f.OnlyAssigned = true
	} else {
		f.UserID = userID
	}
	byStatus, err := s.orders.CountByStatus(ctx, f)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, n := range byStatus {
		total += n
	}
	return &dto.OrderCountItem{Total: total, ByStag: byStatus}, nil
}

// OrderDetail 订单详情。只有下单人与服务专家能看。
func (s *OrderService) OrderDetail(ctx context.Context, userID int64, orderNo string) (*dto.OrderDetail, error) {
	if _, err := s.loadOrderFor(ctx, orderNo, userID); err != nil {
		return nil, err
	}
	return s.orderDetail(ctx, orderNo)
}

/* ============================ 订单主流程 ============================ */

// Pay 发起支付。
//
// 三个通道的行为差异全部收在 payment.Provider 里：
//   - mock：Prepay 成功后立刻走 markPaid，订单直接进服务中；
//   - offline：Prepay 返回 Manual=true 的指引，订单留在待支付等人工确认；
//   - 未接通的通道：返回 501，订单不动。
func (s *OrderService) Pay(ctx context.Context, userID int64, orderNo string, req dto.OrderPayReq) (*dto.OrderPayResult, error) {
	o, err := s.loadOrderFor(ctx, orderNo, userID)
	if err != nil {
		return nil, err
	}
	if o.UserID != userID {
		return nil, errcode.ErrOrderNotOwner
	}
	if o.Status != model.OrderStatusPendingPay {
		if o.IsPaid() {
			return nil, errcode.ErrPayAlreadyPaid
		}
		return nil, errcode.ErrOrderStateInvalid
	}
	// 支付过期由定时任务扫，但用户点得比扫描快时也要拦住：
	// 过期后再付的钱需要人工退，属于"可以避免的人工介入"。
	if o.PayExpireAt != nil && o.PayExpireAt.Before(time.Now()) {
		return nil, errcode.ErrOrderPayExpired
	}

	channel := strings.TrimSpace(req.Channel)
	// 先判"认不认识"：不认识说明前端传错了，是参数问题（400）。
	// "通没通"不在这里判——那是 provider 自己的回答（未接通返回 501）。
	// 两件事混在一处判断，第一版就在这里踩过：写成"认识的通道才切 provider"，
	// 结果选了未接通的微信支付会**静默落到默认的 mock 通道**上，
	// 用户以为在用微信、实际订单被 mock 立刻置成了已支付。
	if !payment.IsKnownChannel(channel) {
		return nil, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("不支持的支付通道 %q", channel))
	}

	// 用户选的通道优先，没选就落到启动时装配的通道。
	// 每个通道的行为差异由它自己的 provider 回答，这里不做任何猜测性降级。
	p := s.pay
	if channel != s.pay.Name() {
		chosen, err := payment.New(config.PaymentConfig{Provider: channel})
		if err != nil {
			return nil, errcode.ErrParamInvalid.WithMsg(
				fmt.Sprintf("不支持的支付通道 %q", channel))
		}
		p = chosen
	}
	return s.prepayWith(ctx, p, o, req)
}

// PayChannels 返回可用的支付通道，供前端渲染收银台。
//
// 不把这份清单写在前端：通道能不能用取决于后端装配了哪个 provider，
// 前端硬编码一份，接通微信支付时就得跟着发版。
func (s *OrderService) PayChannels() []dto.PayChannelItem {
	current := s.pay.Name()
	all := []string{payment.ChannelMock, payment.ChannelOffline, payment.ChannelWechat, payment.ChannelAlipay}
	out := make([]dto.PayChannelItem, 0, len(all))
	for _, name := range all {
		out = append(out, dto.PayChannelItem{
			Name:      name,
			Label:     payment.ChannelLabel(name),
			Available: payment.IsAvailableChannel(name),
			Current:   name == current,
		})
	}
	return out
}

func (s *OrderService) prepayWith(ctx context.Context, p payment.Provider, o *model.Order, req dto.OrderPayReq) (*dto.OrderPayResult, error) {
	res, err := p.Prepay(ctx, payment.PrepayRequest{
		OrderNo:     o.OrderNo,
		AmountCents: o.AmountCents,
		Subject:     o.Title,
		NotifyURL:   s.payCfg.NotifyURL,
		OpenID:      req.OpenID,
	})
	if err != nil {
		return nil, err
	}

	out := &dto.OrderPayResult{
		OrderNo:     o.OrderNo,
		Channel:     p.Name(),
		Manual:      res.Manual,
		Instruction: res.Instruction,
		Params:      res.Params,
	}
	if res.Manual {
		// 线下通道只记下用户选了它，订单仍然留在待支付等人工确认。
		// 这里绝不能顺手把订单置为已支付——钱还没到。
		if err := s.orders.UpdateFields(ctx, o.ID, map[string]any{"pay_channel": p.Name()}); err != nil {
			return nil, err
		}
		return out, nil
	}

	if err := s.markPaid(ctx, o, p.Name(), res.ThirdPartyNo); err != nil {
		return nil, err
	}
	out.Paid = true
	return out, nil
}

// markPaid 是"订单收到钱"的唯一入口，mock 支付与后台人工确认都走这里。
//
// 走 Transition 的 CAS：两个并发的支付请求只有一个能把 pending_pay 改走，
// 另一个拿到 ErrOrderStateInvalid 而不是把已支付时间覆盖一遍。
func (s *OrderService) markPaid(ctx context.Context, o *model.Order, channel, thirdPartyNo string) error {
	now := time.Now()
	if err := s.orders.Transition(ctx, o.OrderNo,
		model.OrderStatusPendingPay, model.OrderStatusInService,
		map[string]any{
			"paid_at":        now,
			"pay_channel":    channel,
			"third_party_no": thirdPartyNo,
			"started_at":     now,
		}); err != nil {
		return err
	}
	return nil
}

// Cancel 取消订单。只有还没付款的订单能取消。
func (s *OrderService) Cancel(ctx context.Context, userID int64, orderNo string, req dto.OrderCancelReq) error {
	o, err := s.loadOrderFor(ctx, orderNo, userID)
	if err != nil {
		return err
	}
	if o.UserID != userID {
		return errcode.ErrOrderNotOwner
	}
	if o.IsPaid() || !o.CanTransitionTo(model.OrderStatusCancelled) {
		return errcode.ErrOrderCancelDenied
	}

	now := time.Now()
	if err := s.orders.Transition(ctx, o.OrderNo, o.Status, model.OrderStatusCancelled,
		map[string]any{
			"cancelled_at":  now,
			"cancel_reason": truncateRunes(req.Reason, 200),
		}); err != nil {
		return err
	}
	// 悬赏单被取消时，对应问题也要收口，否则"我的问题"里会一直挂着待响应。
	if o.QuestionID > 0 && o.Status == model.OrderStatusPendingAccept {
		_ = s.questions.Transition(ctx, o.QuestionID,
			model.QuestionStatusOpen, model.QuestionStatusCancelled,
			map[string]any{"closed_at": now})
	}
	return nil
}

// StartSession 开始一次服务会话（双方都可发起）。
func (s *OrderService) StartSession(ctx context.Context, userID int64, orderNo, channel string) (*dto.OrderSessionItem, error) {
	o, err := s.loadOrderFor(ctx, orderNo, userID)
	if err != nil {
		return nil, err
	}
	if o.Status != model.OrderStatusInService {
		return nil, errcode.ErrOrderStateInvalid.WithMsg("订单不在服务中，无法开始会话")
	}

	// 已有进行中的会话就复用，不新建。
	//
	// **这一步必须在通道校验之前。** 复用是幂等的"进入沟通"，调用方手里
	// 未必还留着上次用的通道（用户隔了半小时再点进来，前端早就重新加载过）；
	// 若先校验 channel，它会拿到一个"服务通道不合法"，而它真正想问的是
	// "把那个还在进行的会话给我"。只有真的要新建会话时，channel 才必须合法。
	if existing, err := s.sessions.ActiveOfOrder(ctx, o.ID); err == nil && existing != nil {
		item := toSessionItem(*existing)
		return &item, nil
	} else if err != nil && !errors.Is(err, errcode.ErrSessionNotFound) {
		return nil, err
	}

	if !validSessionChannel(channel) {
		return nil, errcode.ErrParamInvalid.WithMsg("服务通道不合法")
	}

	now := time.Now()
	sess := &model.ServiceSession{
		OrderID:    o.ID,
		Channel:    channel,
		Provider:   "manual", // MVP 阶段不自研音视频，由双方线下约线上会议
		Status:     model.SessionStatusOngoing,
		StartTime:  &now,
		MeetingURL: "",
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	item := toSessionItem(*sess)
	return &item, nil
}

// EndSession 结束当前会话，时长由服务端算。
func (s *OrderService) EndSession(ctx context.Context, userID int64, orderNo string) (*dto.OrderSessionItem, error) {
	o, err := s.loadOrderFor(ctx, orderNo, userID)
	if err != nil {
		return nil, err
	}
	sess, err := s.sessions.ActiveOfOrder(ctx, o.ID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var duration int64
	if sess.StartTime != nil {
		duration = int64(now.Sub(*sess.StartTime).Seconds())
		if duration < 0 {
			// 时钟回拨或本机时间被改过，宁可为 0 也不要记一个负数时长。
			duration = 0
		}
	}
	if err := s.sessions.End(ctx, sess.ID, now, duration); err != nil {
		return nil, err
	}
	sess.Status = model.SessionStatusEnded
	sess.EndTime = &now
	sess.DurationSec = duration
	item := toSessionItem(*sess)
	return &item, nil
}

// FinishService 专家标记服务完成 → 待确认。
func (s *OrderService) FinishService(ctx context.Context, expertUserID int64, orderNo string) error {
	o, err := s.loadOrderFor(ctx, orderNo, expertUserID)
	if err != nil {
		return err
	}
	if o.ExpertID != expertUserID {
		return errcode.ErrOrderNotExpert
	}
	if !o.CanTransitionTo(model.OrderStatusPendingConfirm) {
		return errcode.ErrOrderStateInvalid
	}

	// 结束还有活着的会话：专家点了完成却没点结束会话，
	// 那条会话会一直挂着并继续计时，订单时长就没底了。
	if sess, err := s.sessions.ActiveOfOrder(ctx, o.ID); err == nil && sess != nil {
		now := time.Now()
		var duration int64
		if sess.StartTime != nil {
			duration = int64(now.Sub(*sess.StartTime).Seconds())
			if duration < 0 {
				duration = 0
			}
		}
		_ = s.sessions.End(ctx, sess.ID, now, duration)
	}

	now := time.Now()
	deadline := now.Add(s.payCfg.AutoConfirmAfter.Std())
	return s.orders.Transition(ctx, o.OrderNo,
		model.OrderStatusInService, model.OrderStatusPendingConfirm,
		map[string]any{
			"finished_at":      now,
			"confirm_deadline": deadline,
		})
}

// ConfirmComplete 用户确认完成 → 已完成。
func (s *OrderService) ConfirmComplete(ctx context.Context, userID int64, orderNo string) error {
	return s.confirm(ctx, orderNo, userID, false)
}

// confirm 是"确认完成"的唯一实现，用户手动确认与超时自动确认共用。
//
// 共用很重要：两条路径如果各写一遍，很容易出现"手动确认落了 confirmed_at、
// 自动确认忘了落"，于是时间轴上有的订单有确认时间、有的没有，
// 履约统计口径就分叉了。from 判定的差别只有一处：自动确认要求订单确实已到 deadline。
func (s *OrderService) confirm(ctx context.Context, orderNo string, userID int64, auto bool) error {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if !auto {
		if o.UserID != userID {
			return errcode.ErrOrderNotOwner
		}
	} else if o.ConfirmDeadline == nil || o.ConfirmDeadline.After(time.Now()) {
		// 自动确认必须等 deadline 真的到了：定时任务被手动触发、
		// 或者系统时间被调过时，不能让订单提前完成。
		return errcode.ErrOrderStateInvalid.WithMsg("未到自动确认时间")
	}
	if !o.CanTransitionTo(model.OrderStatusCompleted) {
		return errcode.ErrOrderStateInvalid
	}

	now := time.Now()
	if err := s.orders.Transition(ctx, o.OrderNo,
		model.OrderStatusPendingConfirm, model.OrderStatusCompleted,
		map[string]any{"confirmed_at": now}); err != nil {
		return err
	}

	// 完成即结算。让用户再点一次"确认结算"没有意义——
	// 确认完成本来就是用户对"服务已交付"的表态，钱到这一步就该给专家。
	return s.settle(ctx, o.OrderNo, "订单完成自动结算")
}

// RaiseDispute 发起争议（用户或专家都可以）。
func (s *OrderService) RaiseDispute(ctx context.Context, userID int64, orderNo string, req dto.OrderDisputeReq) (*dto.OrderDisputeItem, error) {
	o, err := s.loadOrderFor(ctx, orderNo, userID)
	if err != nil {
		return nil, err
	}
	reason, err := normalizeDisputeReason(req.Reason)
	if err != nil {
		return nil, err
	}
	evidence, err := s.normalizeAttachments(req.Evidence)
	if err != nil {
		return nil, err
	}

	// 已有未结案争议就不要再开一个：一个订单同时挂两条争议，
	// 处理后到底是哪条结论生效说不清。
	if existing, err := s.disputes.OpenOfOrder(ctx, o.ID); err == nil && existing != nil {
		return nil, errcode.ErrDisputeExists
	} else if err != nil && !errors.Is(err, errcode.ErrNotFound) {
		return nil, err
	}

	if !o.CanTransitionTo(model.OrderStatusDisputed) {
		return nil, errcode.ErrOrderStateInvalid.WithMsg("当前订单状态不支持发起争议")
	}

	now := time.Now()
	d := &model.Dispute{
		OrderID:  o.ID,
		UserID:   userID,
		ExpertID: o.ExpertID,
		Reason:   reason,
		Evidence: model.StringList(evidence),
		Status:   model.DisputeStatusOpen,
		// 方案约定 3 个工作日，这里按自然日 +3 记，
		// 展示层再折算成工作日；存截止时间比存"承诺几天"更难被改错。
		Deadline: timePtr(now.AddDate(0, 0, 3)),
	}
	if err := s.disputes.Create(ctx, d); err != nil {
		return nil, err
	}
	if err := s.orders.Transition(ctx, o.OrderNo, o.Status, model.OrderStatusDisputed,
		map[string]any{}); err != nil {
		// 订单没转成功就把争议删掉，否则会留下一条"争议中但订单不在争议状态"的记录，
		// 后台看板上这条争议永远处理不干净。
		_ = s.disputes.UpdateFields(ctx, d.ID, map[string]any{
			"status": model.DisputeStatusRejected,
			"result": "订单状态已变化，本次争议未生效",
		})
		return nil, err
	}
	item := toDisputeItem(*d)
	return &item, nil
}

// Review 提交评价。只有下单人能评，一单一评。
func (s *OrderService) Review(ctx context.Context, userID int64, orderNo string, req dto.OrderReviewReq) (*dto.OrderReviewItem, error) {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o.UserID != userID {
		return nil, errcode.ErrOrderNotOwner
	}
	// 已结算与已完成都可以评价：结算发生在完成之后，
	// 若把条件写成"只有 completed"，用户点完确认的下一秒就去评价时会发现按钮是灰的。
	if o.Status != model.OrderStatusCompleted && o.Status != model.OrderStatusSettled {
		return nil, errcode.ErrReviewNotAllowed
	}
	content, err := normalizeReviewContent(req.Content)
	if err != nil {
		return nil, err
	}

	rv := &model.Review{
		OrderID:            o.ID,
		UserID:             userID,
		ExpertID:           o.ExpertID,
		Score:              req.Score,
		ProfessionalScore:  req.ProfessionalScore,
		SpeedScore:         req.SpeedScore,
		CommunicationScore: req.CommunicationScore,
		Content:            content,
		Recommend:          req.Recommend,
		Anonymous:          req.Anonymous,
	}
	// 一单一评由唯一索引守：并发双击提交也只有一条能落进去。
	if err := s.reviews.Create(ctx, rv); err != nil {
		return nil, err
	}
	// 评价会改变专家评分，顺手刷新信誉数据。失败不回滚评价——
	// 评价本身是用户权益，信誉数字是派生数据，可以下次刷新补上。
	_ = s.refreshExpertRating(ctx, o.ExpertID)

	item := toReviewItem(*rv)
	return &item, nil
}

// ReplyReview 专家回复评价。
func (s *OrderService) ReplyReview(ctx context.Context, expertUserID int64, orderNo string, req dto.OrderReviewReplyReq) error {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if o.ExpertID != expertUserID {
		return errcode.ErrOrderNotExpert
	}
	reply := strings.TrimSpace(req.Reply)
	if reply == "" {
		return errcode.ErrParamMissing.WithMsg("回复内容不能为空")
	}
	return s.reviews.Reply(ctx, o.ID, truncateRunes(reply, 1000), time.Now())
}

// AppealReview 专家对被评价的结果提出申诉。
//
// 申诉不改变评分——评分是评价人的表达，不能被被评价方改写；
// 它只是给评论区挂一个"已申诉"标记，等后台人工复核。
func (s *OrderService) AppealReview(ctx context.Context, expertUserID int64, orderNo string, req dto.OrderReviewAppealReq) error {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if o.ExpertID != expertUserID {
		return errcode.ErrOrderNotExpert
	}
	text := strings.TrimSpace(req.AppealText)
	if text == "" {
		return errcode.ErrParamMissing.WithMsg("申诉理由不能为空")
	}
	return s.reviews.Appeal(ctx, o.ID, truncateRunes(text, 1000))
}

/* ============================ 后台 ============================ */

// AdminList 后台订单列表。
func (s *OrderService) AdminList(ctx context.Context, q dto.AdminOrderListQuery) ([]dto.OrderItem, int64, error) {
	q.Normalize()
	orders, total, err := s.orders.List(ctx, repository.OrderFilter{
		Status:      q.Status,
		Type:        q.Type,
		Keyword:     q.Keyword,
		ExpertID:    q.ExpertID,
		UserID:      q.UserID,
		CreatedFrom: q.CreatedFrom,
		CreatedTo:   q.CreatedTo,
		Sort:        q.Sort,
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items, err := s.toOrderItems(ctx, orders)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// AdminDetail 后台订单详情，不受"只有双方能看"的限制。
func (s *OrderService) AdminDetail(ctx context.Context, orderNo string) (*dto.OrderDetail, error) {
	return s.orderDetail(ctx, orderNo)
}

// AdminMarkPaid 后台人工确认收款（offline 通道）。
func (s *OrderService) AdminMarkPaid(ctx context.Context, orderNo string, req dto.AdminOrderMarkPaidReq) error {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return err
	}
	channel := o.PayChannel
	if channel == "" {
		channel = model.PayChannelOffline
	}
	return s.markPaid(ctx, o, channel, truncateRunes(req.ThirdPartyNo, 64))
}

// AdminTransition 后台强制流转。
//
// 仍然受状态机约束：后台需要的是"处理完争议后把订单推进到下一状态"，
// 而不是"随便改成已完成"。放开状态机等于把订单数据变成可以手改的文本，
// 之后所有基于状态的统计都不再可信。
func (s *OrderService) AdminTransition(ctx context.Context, orderNo string, req dto.AdminOrderTransitionReq) error {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return err
	}
	to := strings.TrimSpace(req.To)
	if !o.CanTransitionTo(to) {
		return errcode.ErrOrderStateInvalid.WithMsg(
			fmt.Sprintf("订单当前状态 %s 不能流转到 %s", o.Status, to))
	}

	extra := map[string]any{}
	now := time.Now()
	switch to {
	case model.OrderStatusSettled:
		// 结算必须走 settle，那里才写分账记录与专家余额。
		// 直接改状态会出现"订单显示已结算但专家一分钱没收到"。
		return s.settle(ctx, orderNo, truncateRunes(req.Remark, 200))
	case model.OrderStatusCompleted:
		extra["confirmed_at"] = now
	case model.OrderStatusRefunded:
		// 退款要退钱，不只是改状态。这笔钱来自平台已收的款，
		// 专家此时还没拿到（结算才到账），所以不需要从专家余额扣；
		// 实际出款由人工执行，这里只把状态与退款金额落准，供对账使用。
		// refunded_at 必须和金额一起写：对账按"退款发生在哪天"归集，
		// 少了这个时间，这笔退款只能落到订单的创建日或支付日，报表就错行了。
		extra["refund_cents"] = o.AmountCents
		extra["refunded_at"] = now
	case model.OrderStatusCancelled:
		extra["cancelled_at"] = now
		extra["cancel_reason"] = truncateRunes(req.Remark, 200)
	}
	return s.orders.Transition(ctx, orderNo, o.Status, to, extra)
}

// AdminSettle 手动触发结算。
func (s *OrderService) AdminSettle(ctx context.Context, orderNo string, req dto.AdminSettleReq) error {
	return s.settle(ctx, orderNo, truncateRunes(req.Remark, 200))
}

// AdminResolveDispute 处理争议。
//
// 三种结论：退款、判定完成、驳回（恢复服务中）。
func (s *OrderService) AdminResolveDispute(ctx context.Context, adminID, disputeID int64, resolution, result string) error {
	d, err := s.disputes.GetByID(ctx, disputeID)
	if err != nil {
		return err
	}
	o, err := s.orders.GetByID(ctx, d.OrderID)
	if err != nil {
		return err
	}

	var orderTo string
	switch resolution {
	case model.OrderStatusRefunded:
		orderTo = model.OrderStatusRefunded
	case model.OrderStatusCompleted:
		orderTo = model.OrderStatusCompleted
	case model.OrderStatusInService:
		orderTo = model.OrderStatusInService
	default:
		return errcode.ErrParamInvalid.WithMsg("争议处理结论不合法")
	}
	if !o.CanTransitionTo(orderTo) {
		return errcode.ErrOrderStateInvalid
	}

	now := time.Now()
	if err := s.disputes.Resolve(ctx, disputeID,
		[]string{model.DisputeStatusOpen, model.DisputeStatusProcessing},
		model.DisputeStatusResolved, truncateRunes(result, 1000), adminID, now); err != nil {
		return err
	}
	extra := map[string]any{}
	if orderTo == model.OrderStatusCompleted {
		extra["confirmed_at"] = now
	}
	if orderTo == model.OrderStatusRefunded {
		extra["refund_cents"] = o.AmountCents
		extra["refunded_at"] = now
	}
	if err := s.orders.Transition(ctx, o.OrderNo, o.Status, orderTo, extra); err != nil {
		return err
	}
	if orderTo == model.OrderStatusCompleted {
		return s.settle(ctx, o.OrderNo, "争议判定完成，自动结算")
	}
	return nil
}

/* ============================ 定时任务入口 ============================ */

// AutoCancelExpiredPayments 取消支付超时的订单。
//
// 逐条只做一件事：把这条订单的 pending_pay 改掉。
// 不用"批量 UPDATE ... WHERE ..."一次改完，是因为每笔都要留下取消原因与时间，
// 而且单条失败不影响其他单——批量语句一旦中间出错，整批的反馈都是糊的。
func (s *OrderService) AutoCancelExpiredPayments(ctx context.Context, limit int) (int, error) {
	orders, err := s.orders.ListExpiredPendingPay(ctx, time.Now(), limit)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, o := range orders {
		err := s.orders.Transition(ctx, o.OrderNo,
			model.OrderStatusPendingPay, model.OrderStatusCancelled,
			map[string]any{
				"cancelled_at":  time.Now(),
				"cancel_reason": "支付超时自动取消",
			})
		if err != nil {
			// 状态已被并发改掉是正常情况（用户刚好在这一刻付了款），跳过继续。
			if errors.Is(err, errcode.ErrOrderStateInvalid) {
				continue
			}
			return done, err
		}
		done++
	}
	return done, nil
}

// AutoConfirmExpired 把到了自动确认时间的订单确认完成并结算。
func (s *OrderService) AutoConfirmExpired(ctx context.Context, limit int) (int, error) {
	orders, err := s.orders.ListExpiredPendingConfirm(ctx, time.Now(), limit)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, o := range orders {
		if err := s.confirm(ctx, o.OrderNo, 0, true); err != nil {
			if errors.Is(err, errcode.ErrOrderStateInvalid) {
				continue
			}
			return done, err
		}
		done++
	}
	return done, nil
}

// SettleCompleted 结算已完成但还没结算的订单（兜底）。
//
// confirm 里已经顺手结算了，这是防止"确认成功但结算那一步失败"留下的孤儿订单——
// 没有这个兜底，那笔订单会永远停在已完成，专家永远拿不到钱。
func (s *OrderService) SettleCompleted(ctx context.Context, limit int) (int, error) {
	orders, err := s.orders.ListSettleable(ctx, limit)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, o := range orders {
		if err := s.settle(ctx, o.OrderNo, "定时任务兜底结算"); err != nil {
			if errors.Is(err, errcode.ErrSettleNotAllowed) || errors.Is(err, errcode.ErrSettleDuplicated) {
				continue
			}
			return done, err
		}
		done++
	}
	return done, nil
}

/* ============================ 内部：结算与装配 ============================ */

// settle 结算一笔已完成订单：落分账记录、把订单置为已结算、专家余额到账。
//
// 三件事在 SettleOrder 的事务里完成，顺序与理由见那里的注释。
func (s *OrderService) settle(ctx context.Context, orderNo, remark string) error {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if o.Status != model.OrderStatusCompleted {
		if o.Status == model.OrderStatusSettled {
			return errcode.ErrSettleDuplicated
		}
		return errcode.ErrSettleNotAllowed
	}
	if o.ExpertID <= 0 {
		return errcode.ErrSettleAmountBad.WithMsg("订单没有服务专家，无法结算")
	}

	// 金额以订单上的快照为准，不用当前配置重算：抽佣比例是可配置的，
	// 下单时是 15%、结算时改成 20% 的话重算会悄悄吞掉专家的收入。
	commission, expertAmount := o.CommissionCents, o.ExpertAmountCents
	if commission+expertAmount != o.AmountCents {
		// 快照本身不平（历史数据或人工改过），不能就这么把钱打出去。
		return errcode.ErrSettleAmountBad
	}

	now := time.Now()
	st := &model.Settlement{
		SettlementNo:      idgen.SettlementNo(),
		OrderID:           o.ID,
		OrderNo:           o.OrderNo,
		ExpertID:          o.ExpertID,
		AmountCents:       o.AmountCents,
		CommissionCents:   commission,
		ExpertAmountCents: expertAmount,
		CommissionRateBP:  o.CommissionRateBP,
		Status:            model.SettlementStatusSettled,
		SettledAt:         &now,
		Remark:            remark,
	}
	if err := s.settlements.SettleOrder(ctx, st, now); err != nil {
		return err
	}
	_ = s.refreshExpertRating(ctx, o.ExpertID)
	return nil
}

// refreshExpertRating 重算专家的信誉数据。
//
// 派生数据的刷新失败不该让主流程失败，所以调用方一律忽略错误；
// 下次有评价或结算时会再刷一遍，最坏情况只是评分晚一点更新。
func (s *OrderService) refreshExpertRating(ctx context.Context, expertUserID int64) error {
	if expertUserID <= 0 {
		return nil
	}
	e, err := s.experts.FindByUserID(ctx, expertUserID)
	if err != nil {
		return err
	}
	stats, err := s.reviews.StatsOfExpert(ctx, expertUserID)
	if err != nil {
		return err
	}
	completed, err := s.orders.CountByExpert(ctx, expertUserID,
		model.OrderStatusCompleted, model.OrderStatusSettled)
	if err != nil {
		return err
	}

	fields := map[string]any{"order_count": completed}
	// 还没收到评价时不要写 0 分：0 分会让专家页显示"评分 0.0"，
	// 比保留默认的 5 分更容易劝退用户。
	if stats.Total > 0 {
		fields["rating"] = stats.AvgScore
		fields["good_rate"] = stats.GoodRate
	}
	return s.experts.UpdateFields(ctx, e.ID, fields)
}

// loadOrderFor 取订单并校验访问权（下单人或服务专家）。
func (s *OrderService) loadOrderFor(ctx context.Context, orderNo string, userID int64) (*model.Order, error) {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o.UserID == userID {
		return o, nil
	}
	if o.ExpertID != 0 && o.ExpertID == userID {
		return o, nil
	}
	// 统一回 403 而不是 404：订单号是随机段，猜不出来，
	// 但"这不是你的订单"比"订单不存在"更能让用户发现是自己点错了入口。
	return nil, errcode.ErrOrderNotOwner
}

// toOrderItem 给单个订单补上展示信息。
func (s *OrderService) toOrderItem(ctx context.Context, o model.Order) (dto.OrderItem, error) {
	items, err := s.toOrderItems(ctx, []model.Order{o})
	if err != nil {
		return dto.OrderItem{}, err
	}
	if len(items) == 0 {
		return dto.OrderItem{}, errcode.ErrOrderNotFound
	}
	return items[0], nil
}

// toOrderItems 批量装配订单列表项。
//
// 昵称与"是否已评价"都按批查：列表页一页 20 条，
// 逐条查用户与评价就是 40 次往返，翻页会明显卡。
func (s *OrderService) toOrderItems(ctx context.Context, orders []model.Order) ([]dto.OrderItem, error) {
	if len(orders) == 0 {
		return []dto.OrderItem{}, nil
	}

	userIDs := make([]int64, 0, len(orders)*2)
	orderIDs := make([]int64, 0, len(orders))
	for _, o := range orders {
		userIDs = append(userIDs, o.UserID)
		if o.ExpertID > 0 {
			userIDs = append(userIDs, o.ExpertID)
		}
		orderIDs = append(orderIDs, o.ID)
	}
	users, err := s.users.FindByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	reviewed, err := s.reviewedOrderIDs(ctx, orderIDs)
	if err != nil {
		return nil, err
	}

	items := make([]dto.OrderItem, 0, len(orders))
	for _, o := range orders {
		item := dto.OrderItem{
			ID:                o.ID,
			OrderNo:           o.OrderNo,
			Type:              o.Type,
			Status:            o.Status,
			Title:             o.Title,
			ServiceMode:       o.ServiceMode,
			UserID:            o.UserID,
			ExpertID:          o.ExpertID,
			QuestionID:        o.QuestionID,
			AmountCents:       o.AmountCents,
			CommissionCents:   o.CommissionCents,
			ExpertAmountCents: o.ExpertAmountCents,
			PayChannel:        o.PayChannel,
			Reviewed:          reviewed[o.ID],
			CreatedAt:         o.CreatedAt,
			PayExpireAt:       o.PayExpireAt,
			ConfirmDeadline:   o.ConfirmDeadline,
			BookedAt:          o.BookedAt,
			DurationMin:       o.DurationMin,
			Remark:            o.Remark,
		}
		if u, ok := users[o.UserID]; ok {
			item.UserName = u.Nickname
		}
		if u, ok := users[o.ExpertID]; ok {
			item.ExpertName = u.Nickname
		}
		// 服务中与待确认之外的状态不返回倒计时字段：
		// 前端读到 payExpireAt 就会渲染倒计时，一个早已完成的订单上
		// 挂着一个过期倒计时会让人以为订单快超时了。
		if o.Status != model.OrderStatusPendingPay {
			item.PayExpireAt = nil
		}
		if o.Status != model.OrderStatusPendingConfirm {
			item.ConfirmDeadline = nil
		}
		items = append(items, item)
	}
	return items, nil
}

// reviewedOrderIDs 批量判断哪些订单已评价。
func (s *OrderService) reviewedOrderIDs(ctx context.Context, orderIDs []int64) (map[int64]bool, error) {
	return s.reviews.ReviewedIDs(ctx, orderIDs)
}

// orderDetail 组装订单详情。
func (s *OrderService) orderDetail(ctx context.Context, orderNo string) (*dto.OrderDetail, error) {
	o, err := s.orders.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	item, err := s.toOrderItem(ctx, *o)
	if err != nil {
		return nil, err
	}

	detail := &dto.OrderDetail{
		OrderItem:    item,
		CancelReason: o.CancelReason,
		Timeline:     buildTimeline(*o),
		Sessions:     []dto.OrderSessionItem{},
	}

	if sessions, err := s.sessions.ListByOrder(ctx, o.ID); err == nil {
		detail.Sessions = make([]dto.OrderSessionItem, 0, len(sessions))
		for _, sess := range sessions {
			detail.Sessions = append(detail.Sessions, toSessionItem(sess))
		}
	}
	if rv, err := s.reviews.GetByOrderID(ctx, o.ID); err == nil && rv != nil {
		// toReviewItem 本身不带任何发布者身份字段（昵称只在评价人自己的
		// 「我的评价」接口里拼），所以匿名与否在订单详情里无需再脱敏。
		review := toReviewItem(*rv)
		detail.Review = &review
	}
	// 展示最近一条争议，包含已结案的：结案后从详情里消失的话，
	// 用户只看到订单变成"已退款"，找不到处理结论。
	if d, err := s.disputes.LatestOfOrder(ctx, o.ID); err == nil && d != nil {
		dispute := toDisputeItem(*d)
		dispute.Evidence = s.toFileRefs([]string(d.Evidence))
		detail.Dispute = &dispute
	}
	if st, err := s.settlements.GetByOrderID(ctx, o.ID); err == nil && st != nil {
		detail.Settlement = &dto.OrderSettlementItem{
			SettlementNo:      st.SettlementNo,
			AmountCents:       st.AmountCents,
			CommissionCents:   st.CommissionCents,
			ExpertAmountCents: st.ExpertAmountCents,
			CommissionRateBP:  st.CommissionRateBP,
			Status:            st.Status,
			SettledAt:         st.SettledAt,
		}
	}
	return detail, nil
}

// questionDetail 读问题并带上它的订单标识。
func (s *OrderService) questionDetail(ctx context.Context, questionID int64) (*dto.QuestionDetail, error) {
	q, err := s.questions.GetByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	detail := &dto.QuestionDetail{QuestionItem: s.toQuestionItem(*q)}
	if o, err := s.orders.GetByQuestionID(ctx, questionID); err == nil && o != nil {
		detail.OrderNo = o.OrderNo
		detail.OrderStatus = o.Status
		detail.OrderID = o.ID
	}
	return detail, nil
}

// toFileRefs 把存储 key 列表转成带访问地址的引用列表。
func (s *OrderService) toFileRefs(keys []string) []dto.FileRef {
	out := make([]dto.FileRef, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		url, exp, _ := s.uploads.AssignURL(k)
		out = append(out, dto.FileRef{Key: k, URL: url, ExpireAt: exp})
	}
	return out
}

// normalizeAttachments 校验附件 key 并去掉空值。
func (s *OrderService) normalizeAttachments(keys []string) ([]string, error) {
	maxAttachments, _ := dto.QuestionExtraLimits()
	if len(keys) > maxAttachments {
		return nil, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("附件最多 %d 个", maxAttachments))
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		// 必须来自问题附件场景，否则可以拿私有文件的 key 混进来，
		// 一次引用就把本来私有的文件变成了公开可访问的附件。
		if !strings.HasPrefix(k, questionKeyPrefix) {
			return nil, errcode.ErrResourceFileInvalid.WithMsg(
				"附件必须通过「问题附件」场景上传（scene=" + string(storage.SceneQuestionAttachment) + "）")
		}
		out = append(out, k)
	}
	return out, nil
}

/* ============================ 纯函数小工具 ============================ */

func normalizeQuestionTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	min, max, _, _ := dto.QuestionLimits()
	n := len([]rune(title))
	switch {
	case n == 0:
		return "", errcode.ErrTitleInvalid.WithMsg("请填写问题标题")
	case n < min:
		return "", errcode.ErrTitleInvalid.WithMsg(fmt.Sprintf("标题不能少于 %d 个字", min))
	case n > max:
		return "", errcode.ErrTitleInvalid.WithMsg(fmt.Sprintf("标题不能超过 %d 个字", max))
	}
	return title, nil
}

func normalizeQuestionDesc(raw string) (string, error) {
	desc := strings.TrimSpace(raw)
	_, _, min, max := dto.QuestionLimits()
	n := len([]rune(desc))
	switch {
	case n < min:
		// 描述太短的后果不是"不礼貌"，而是专家没法判断现象，只能反复追问，
		// 悬赏问答的响应时长会被这类问题拉垮。
		return "", errcode.ErrQuestionTooShort
	case n > max:
		return "", errcode.ErrContentInvalid.WithMsg(fmt.Sprintf("问题描述不能超过 %d 个字", max))
	}
	return desc, nil
}

func normalizeSkillIDs(ids []string) ([]string, error) {
	_, maxSkills := dto.QuestionExtraLimits()
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, errcode.ErrQuestionSkillReq
	}
	if len(out) > maxSkills {
		return nil, errcode.ErrSkillInvalid.WithMsg(fmt.Sprintf("技术栈最多选 %d 个", maxSkills))
	}
	return out, nil
}

func normalizeExpectResponse(minutes int) (int, error) {
	min, max, def := dto.ExpectResponseRange()
	if minutes == 0 {
		return def, nil
	}
	if minutes < min || minutes > max {
		return 0, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("期望响应时间需在 %d-%d 分钟之间", min, max))
	}
	return minutes, nil
}

func normalizeExpireHours(hours int) (int, error) {
	min, max, def := dto.ExpireHoursRange()
	if hours == 0 {
		return def, nil
	}
	if hours < min || hours > max {
		return 0, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("悬赏有效期需在 %d-%d 小时之间", min, max))
	}
	return hours, nil
}

func normalizeReviewContent(raw string) (string, error) {
	content := strings.TrimSpace(raw)
	min, max := dto.ReviewLimits()
	n := len([]rune(content))
	switch {
	case n < min:
		return "", errcode.ErrReviewContent
	case n > max:
		return "", errcode.ErrReviewContent.WithMsg(fmt.Sprintf("评价内容不能超过 %d 个字", max))
	}
	return content, nil
}

func normalizeDisputeReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	min, max := dto.DisputeLimits()
	n := len([]rune(reason))
	switch {
	case n < min:
		return "", errcode.ErrDisputeReasonReq.WithMsg(
			fmt.Sprintf("请把争议原因写清楚，不少于 %d 个字", min))
	case n > max:
		return "", errcode.ErrDisputeReasonReq.WithMsg(
			fmt.Sprintf("争议原因不能超过 %d 个字", max))
	}
	return reason, nil
}

// buildTimeline 按时间正序拼出订单已发生的关键节点。
func buildTimeline(o model.Order) []dto.OrderTimelineNode {
	nodes := make([]dto.OrderTimelineNode, 0, 8)
	add := func(status string, at *time.Time) {
		if at != nil {
			nodes = append(nodes, dto.OrderTimelineNode{Status: status, At: *at})
		}
	}
	add(model.OrderStatusPendingAccept, timePtr(o.CreatedAt))
	add(model.OrderStatusPendingPay, o.AcceptedAt)
	add(model.OrderStatusInService, o.PaidAt)
	add(model.OrderStatusPendingConfirm, o.FinishedAt)
	add(model.OrderStatusCompleted, o.ConfirmedAt)
	add(model.OrderStatusSettled, o.SettledAt)
	add(model.OrderStatusDisputed, nil)
	add(model.OrderStatusCancelled, o.CancelledAt)

	// 按时间排序；同一时刻的（比如支付即开始服务）保持插入顺序。
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && nodes[j].At.Before(nodes[j-1].At); j-- {
			nodes[j], nodes[j-1] = nodes[j-1], nodes[j]
		}
	}
	return nodes
}

func (s *OrderService) toQuestionItems(items []model.Question) []dto.QuestionItem {
	out := make([]dto.QuestionItem, 0, len(items))
	for _, q := range items {
		out = append(out, s.toQuestionItem(q))
	}
	return out
}

func (s *OrderService) toQuestionItem(q model.Question) dto.QuestionItem {
	return dto.QuestionItem{
		ID:          q.ID,
		UserID:      q.UserID,
		Title:       q.Title,
		Description: q.Description,
		Attachments: s.toFileRefs([]string(q.Attachments)),
		SkillIDs:    []string(q.SkillIDs),
		BudgetCents: q.BudgetCents,
		Status:      q.Status,
		ExpiredAt:   q.ExpiredAt,
		CreatedAt:   q.CreatedAt,
		MatchedAt:   q.MatchedAt,
		ClosedAt:    q.ClosedAt,
	}
}

func toSessionItem(s model.ServiceSession) dto.OrderSessionItem {
	return dto.OrderSessionItem{
		ID:          s.ID,
		Channel:     s.Channel,
		Status:      s.Status,
		MeetingURL:  s.MeetingURL,
		Provider:    s.Provider,
		StartTime:   s.StartTime,
		EndTime:     s.EndTime,
		DurationSec: s.DurationSec,
	}
}

func toReviewItem(r model.Review) dto.OrderReviewItem {
	return dto.OrderReviewItem{
		Score:              r.Score,
		ProfessionalScore:  r.ProfessionalScore,
		SpeedScore:         r.SpeedScore,
		CommunicationScore: r.CommunicationScore,
		Content:            r.Content,
		Recommend:          r.Recommend,
		Anonymous:          r.Anonymous,
		Reply:              r.Reply,
		RepliedAt:          r.RepliedAt,
		Appealed:           r.Appealed,
		CreatedAt:          r.CreatedAt,
	}
}

func toDisputeItem(d model.Dispute) dto.OrderDisputeItem {
	return dto.OrderDisputeItem{
		ID:        d.ID,
		Reason:    d.Reason,
		Status:    d.Status,
		Result:    d.Result,
		Deadline:  d.Deadline,
		CreatedAt: d.CreatedAt,
		HandledAt: d.HandledAt,
	}
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func validSessionChannel(channel string) bool {
	switch strings.TrimSpace(channel) {
	case "im", "voice", "video", "screen":
		return true
	}
	return false
}

func serviceModeLabel(mode string) string {
	switch mode {
	case model.ServiceModeIM:
		return "图文"
	case model.ServiceModeVoice:
		return "语音"
	case model.ServiceModeVideo:
		return "视频"
	case model.ServiceModeScreen:
		return "视频+屏幕共享"
	default:
		return mode
	}
}

func hallSort(sort string) string {
	switch sort {
	case "latest", "amount", "expiring":
		return sort
	default:
		return "amount"
	}
}
