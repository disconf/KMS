package dto

import "time"

// 订单与悬赏的出入参。
//
// 两条约定：
//  1. **金额一律以「分」为 int64 传输**，字段名带 Cents 后缀，由前端格式化。
//     接口层不做元/分换算，避免出现两套口径。
//  2. **不返回状态文案**。所有 status 都是机器值（pending_pay 等），
//     中文标签与按钮显隐由前端的 orderStatusMap 决定。
//     后端再输出一份标签就等于有两个真相来源，早晚会不一致。

// ============================ 悬赏问题 ============================

// QuestionCreateReq 发布悬赏。
//
// 这里没有 ExpertID：悬赏是"挂出去等人接"，专家由抢单产生。
type QuestionCreateReq struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description" binding:"required"`
	// Attachments 是上传接口返回的 key 列表，不是 URL。
	Attachments []string `json:"attachments" binding:"omitempty,max=6"`
	// SkillIDs 是技能标签 ID，用于把问题推给对口的专家。
	SkillIDs []string `json:"skillIds" binding:"required,min=1,max=5"`

	BudgetCents       int64 `json:"budgetCents" binding:"required"`
	ExpectResponseMin int   `json:"expectResponseMin"`
	// ExpireHours 悬赏有效期（小时）。不传时用默认值。
	ExpireHours int `json:"expireHours"`
}

// QuestionItem 问题列表项。
type QuestionItem struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"userId"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Attachments []FileRef  `json:"attachments"`
	SkillIDs    []string   `json:"skillIds"`
	BudgetCents int64      `json:"budgetCents"`
	Status      string     `json:"status"`
	ExpiredAt   time.Time  `json:"expiredAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	MatchedAt   *time.Time `json:"matchedAt,omitempty"`
	ClosedAt    *time.Time `json:"closedAt,omitempty"`
}

// QuestionDetail 问题详情，带上它对应的订单，前端才能给出"去支付 / 看订单"的入口。
type QuestionDetail struct {
	QuestionItem
	OrderNo     string `json:"orderNo"`
	OrderStatus string `json:"orderStatus"`
	// OrderID 供内部跳转用，前端路由用订单号。
	OrderID int64 `json:"orderId"`
}

// QuestionListQuery 我的问题列表。
type QuestionListQuery struct {
	PageQuery
	Status string `form:"status"`
	Sort   string `form:"sort"`
}

// QuestionCancelReq 取消悬赏。
type QuestionCancelReq struct {
	Reason string `json:"reason" binding:"omitempty,max=200"`
}

// ============================ 订单 ============================

// OrderCreateReq 预约专家下单（booking 型）。
//
// **刻意没有 AmountCents 字段。** 金额必须由服务端按专家的定价算出来：
// 接受前端传金额等于让用户自己定价，改一个数字就能用 1 分钱买到一小时服务。
// 悬赏型的金额来自用户填的预算，但同样会在服务端做上下限校验。
type OrderCreateReq struct {
	// ExpertUserID 是**专家本人的用户 ID**，不是专家档案 ID。
	//
	// 这两个 ID 在接口里同时存在，前端刚从专家详情页过来时手里多半是档案 ID
	// （ExpertCard.id），传错会落到另一个人身上或查不到。字段名带 User 就是为了
	// 让这个区别写在接口上——订单表里的 expert_id 一直存的是 users.id
	// （抢单时落进去的就是抢单人的用户 ID），这里跟随同一语义，不另起一套。
	ExpertUserID int64      `json:"expertUserId" binding:"required"`
	Title        string     `json:"title"`
	ServiceMode  string     `json:"serviceMode" binding:"required,oneof=im voice video screen"`
	BookedAt     *time.Time `json:"bookedAt"`
	// DurationMin 预约时长（分钟）。与专家的按小时定价相乘得出金额。
	DurationMin int    `json:"durationMin" binding:"min=0"`
	Remark      string `json:"remark" binding:"omitempty,max=500"`
}

// OrderPayReq 发起支付。
//
// Channel 刻意**不用 oneof 标签**约束：可选的通道由当前装配的支付 provider
// 决定（开发环境是 mock，线上可能是微信），把它写死在结构体标签里就等于
// 在 DTO 里维护第二份通道清单。第一次跑就撞上了——mock 通道被 oneof 挡在门外，
// 开发环境根本没法下单。合法性交给 payment.IsKnownChannel 判断。
type OrderPayReq struct {
	Channel string `json:"channel" binding:"required"`
	// OpenID 微信 JSAPI 支付需要，其他通道可忽略。
	OpenID string `json:"openId"`
}

// PayChannelItem 一个可选支付通道。
type PayChannelItem struct {
	Name string `json:"name"`
	// Label 是给用户看的中文名。
	Label string `json:"label"`
	// Available 为 false 表示通道已实现但当前未开通，前端应置灰而不是隐藏，
	// 这样用户知道"以后会有"，而不是以为平台本来就不支持。
	Available bool `json:"available"`
	// Current 标记当前服务实际装配的通道。同一订单重复发起支付时用得上。
	Current bool `json:"current"`
}

// OrderPayResult 支付发起结果。
type OrderPayResult struct {
	OrderNo string `json:"orderNo"`
	// Channel 是本次发起所用的通道。
	Channel string `json:"channel"`
	// Paid 为 true 表示这次请求已经把订单置为已支付（mock 通道会这样）。
	Paid bool `json:"paid"`
	// Manual 为 true 时不能在线唤起支付，前端要展示 Instruction 而不是拉起收银台。
	Manual bool `json:"manual"`
	// Instruction 是给用户看的下一步说明。
	Instruction string `json:"instruction,omitempty"`
	// Params 是唤起支付所需参数，结构由通道决定，前端原样透传即可。
	Params map[string]string `json:"params,omitempty"`
}

// OrderCancelReq 取消订单。
type OrderCancelReq struct {
	Reason string `json:"reason" binding:"omitempty,max=200"`
}

// OrderReviewReq 提交评价。
type OrderReviewReq struct {
	Score              int    `json:"score" binding:"required,min=1,max=5"`
	ProfessionalScore  int    `json:"professionalScore" binding:"omitempty,min=1,max=5"`
	SpeedScore         int    `json:"speedScore" binding:"omitempty,min=1,max=5"`
	CommunicationScore int    `json:"communicationScore" binding:"omitempty,min=1,max=5"`
	Content            string `json:"content" binding:"required"`
	Recommend          bool   `json:"recommend"`
	Anonymous          bool   `json:"anonymous"`
}

// OrderReviewReplyReq 专家回复评价。
type OrderReviewReplyReq struct {
	Reply string `json:"reply" binding:"required"`
}

// OrderReviewAppealReq 专家申诉差评。
type OrderReviewAppealReq struct {
	AppealText string `json:"appealText" binding:"required"`
}

// OrderDisputeReq 发起争议。
type OrderDisputeReq struct {
	Reason   string   `json:"reason" binding:"required"`
	Evidence []string `json:"evidence" binding:"omitempty,max=6"`
}

// OrderListQuery 订单列表。
type OrderListQuery struct {
	PageQuery
	// Side 决定看哪一侧的订单：user=我下的单，expert=我接的单。
	// 默认 user。这两个视图共用一套订单表，差别只在筛选字段。
	Side   string `form:"side" binding:"oneof=user expert"`
	Status string `form:"status"`
	Type   string `form:"type" binding:"omitempty,oneof=question booking"`
	// Keyword 匹配订单号与标题。
	Keyword string `form:"keyword"`
	Sort    string `form:"sort" binding:"omitempty,oneof=latest oldest amount"`
}

// OrderItem 订单列表项。
//
// 不带状态中文标签：前端读 orderStatusMap。这里只给"数据"，
// 包括倒计时用的截止时间——由前端算出剩余秒数，比服务端算好更准，
// 因为服务端算出的秒数在网传输路上就开始过时了。
type OrderItem struct {
	ID      int64  `json:"id"`
	OrderNo string `json:"orderNo"`
	Type    string `json:"type"`
	Status  string `json:"status"`

	Title       string `json:"title"`
	ServiceMode string `json:"serviceMode"`

	UserID   int64 `json:"userId"`
	ExpertID int64 `json:"expertId"`
	// ExpertName / UserName 是展示用的昵称，列表页不用再发一轮请求。
	ExpertName string `json:"expertName,omitempty"`
	UserName   string `json:"userName,omitempty"`

	QuestionID int64 `json:"questionId,omitempty"`

	AmountCents       int64 `json:"amountCents"`
	CommissionCents   int64 `json:"commissionCents"`
	ExpertAmountCents int64 `json:"expertAmountCents"`

	// PayChannel 已支付时有值。
	PayChannel string `json:"payChannel,omitempty"`
	// Reviewed 表示该订单是否已评价，前端据此决定"去评价"还是"查看评价"。
	Reviewed bool `json:"reviewed"`

	CreatedAt time.Time `json:"createdAt"`
	// PayExpireAt / ConfirmDeadline 是两处倒计时的截止时间。
	// 两个都可能为空：未进入倒计时的订单没有这两项。
	PayExpireAt     *time.Time `json:"payExpireAt,omitempty"`
	ConfirmDeadline *time.Time `json:"confirmDeadline,omitempty"`

	BookedAt    *time.Time `json:"bookedAt,omitempty"`
	DurationMin int        `json:"durationMin,omitempty"`
	Remark      string     `json:"remark,omitempty"`
}

// OrderDetail 订单详情。
type OrderDetail struct {
	OrderItem
	// CancelReason 取消原因，仅在已取消时有值。
	// 用户主动取消与支付超时被系统关单都落到 cancelled，只给一个"已取消"
	// 等于让用户自己猜发生了什么——而"我明明点了支付"是最需要解释的场景。
	CancelReason string `json:"cancelReason,omitempty"`
	// Timeline 是已发生的关键时间点，按时间正序，前端直接渲染成时间轴。
	Timeline []OrderTimelineNode `json:"timeline"`
	// Sessions 是服务会话记录，MVP 阶段只展示会议入口与时长。
	Sessions []OrderSessionItem `json:"sessions"`
	// Review 已评价时返回。
	Review *OrderReviewItem `json:"review,omitempty"`
	// Dispute 有未结案争议时返回。
	Dispute *OrderDisputeItem `json:"dispute,omitempty"`
	// Settlement 已结算时返回。
	Settlement *OrderSettlementItem `json:"settlement,omitempty"`
}

// OrderTimelineNode 时间轴上的一个节点。
type OrderTimelineNode struct {
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

// OrderSessionItem 服务会话。
type OrderSessionItem struct {
	ID          int64      `json:"id"`
	Channel     string     `json:"channel"`
	Status      string     `json:"status"`
	MeetingURL  string     `json:"meetingUrl"`
	Provider    string     `json:"provider"`
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	DurationSec int64      `json:"durationSec"`
}

// OrderReviewItem 评价。
type OrderReviewItem struct {
	Score              int        `json:"score"`
	ProfessionalScore  int        `json:"professionalScore"`
	SpeedScore         int        `json:"speedScore"`
	CommunicationScore int        `json:"communicationScore"`
	Content            string     `json:"content"`
	Recommend          bool       `json:"recommend"`
	Anonymous          bool       `json:"anonymous"`
	Reply              string     `json:"reply,omitempty"`
	RepliedAt          *time.Time `json:"repliedAt,omitempty"`
	Appealed           bool       `json:"appealed"`
	CreatedAt          time.Time  `json:"createdAt"`
}

// OrderDisputeItem 争议。
type OrderDisputeItem struct {
	ID        int64      `json:"id"`
	Reason    string     `json:"reason"`
	Evidence  []FileRef  `json:"evidence"`
	Status    string     `json:"status"`
	Result    string     `json:"result,omitempty"`
	Deadline  *time.Time `json:"deadline,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	HandledAt *time.Time `json:"handledAt,omitempty"`
}

// OrderSettlementItem 分账凭证。
type OrderSettlementItem struct {
	SettlementNo      string     `json:"settlementNo"`
	AmountCents       int64      `json:"amountCents"`
	CommissionCents   int64      `json:"commissionCents"`
	ExpertAmountCents int64      `json:"expertAmountCents"`
	CommissionRateBP  int        `json:"commissionRateBp"`
	Status            string     `json:"status"`
	SettledAt         *time.Time `json:"settledAt,omitempty"`
}

// OrderCountItem 各状态订单数，用于订单中心的 Tab 角标。
type OrderCountItem struct {
	Total  int64            `json:"total"`
	ByStag map[string]int64 `json:"byStatus"`
}

// ============================ 接单大厅 ============================

// HallListQuery 接单大厅列表。
type HallListQuery struct {
	PageQuery
	// SkillID 只看某个技术方向。
	SkillID string `form:"skillId"`
	Sort    string `form:"sort" binding:"omitempty,oneof=latest amount expiring"`
}

// HallItem 大厅里的一条悬赏。
//
// 单独一个类型而不是复用 QuestionItem：大厅的每条记录其实是"一笔待接单的订单"，
// 只是内容取自它挂着的问题。前端要发起抢单必须拿得到订单标识，而 QuestionItem
// 是问题域的通用结构，不该为了大厅塞进订单字段——那样"我的问题"列表也会
// 带上一串对自己毫无意义的订单号。
type HallItem struct {
	QuestionItem
	// OrderID 供内部跳转与埋点用。
	OrderID int64 `json:"orderId"`
	// OrderNo 是抢单接口要传的标识。
	OrderNo string `json:"orderNo"`
}

// ============================ 后台订单管理 ============================

// AdminOrderListQuery 后台订单筛选。
type AdminOrderListQuery struct {
	PageQuery
	Status   string `form:"status"`
	Type     string `form:"type"`
	Keyword  string `form:"keyword"`
	ExpertID int64  `form:"expertId"`
	UserID   int64  `form:"userId"`
	// CreatedFrom / CreatedTo 是 RFC3339 时间，左闭右开。
	CreatedFrom *time.Time `form:"createdFrom" time_format:"2006-01-02T15:04:05Z07:00"`
	CreatedTo   *time.Time `form:"createdTo" time_format:"2006-01-02T15:04:05Z07:00"`
	Sort        string     `form:"sort"`
}

// AdminOrderTransitionReq 管理员强制流转状态。
//
// 强制流转会绕过 CanTransitionTo 吗？不会。任何人都不能把订单改成
// 状态机里不存在的一步——后台需要的是"处理争议后推进到下一状态"，
// 而不是"随便改成已完成"。所以这里仍受状态机约束。
type AdminOrderTransitionReq struct {
	To     string `json:"to" binding:"required"`
	Remark string `json:"remark" binding:"omitempty,max=200"`
}

// AdminOrderMarkPaidReq 后台人工确认收款（offline 通道用）。
type AdminOrderMarkPaidReq struct {
	// ThirdPartyNo 线下转账流水号，对账时要留痕。
	ThirdPartyNo string `json:"thirdPartyNo"`
	Remark       string `json:"remark" binding:"omitempty,max=200"`
}

// AdminSettleReq 手动触发结算。
type AdminSettleReq struct {
	Remark string `json:"remark" binding:"omitempty,max=200"`
}

// AdminResolveDisputeReq 处理争议。
//
// Resolution 只能取三种结论，它们对应订单的落点：
// refunded（退款）/ completed（判定服务已完成）/ in_service（驳回争议、恢复服务中）。
// 用订单状态值本身而不是"approve/reject"这类词：一是省掉一层映射，
// 二是这三种结论本来就以订单状态收口，中间多一层词表就多一个不会同步的地方。
type AdminResolveDisputeReq struct {
	Resolution string `json:"resolution" binding:"required,oneof=refunded completed in_service"`
	Result     string `json:"result" binding:"required"`
}

// ============================ 长度与金额约束 ============================
//
// 集中在此处，service 层校验与接口文档共用一套数字，避免两边写岔。

const (
	questionTitleMinLen = 5
	questionTitleMaxLen = 200
	// questionDescMinLen 定在 10 个字：运维问题要排障，少于 10 字基本无法判断
	// 现象，专家只能来回追问，问答型服务的响应时长会被这类问题拉垮。
	questionDescMinLen = 10
	questionDescMaxLen = 5000

	// 悬赏金额上下限（分）。
	budgetMinCents = 1000   // 10 元
	budgetMaxCents = 500000 // 5000 元

	maxQuestionAttachments = 6
	maxQuestionSkills      = 5

	// 悬赏有效期（小时）。
	minExpireHours     = 1
	maxExpireHours     = 168 // 7 天
	defaultExpireHours = 72  // 3 天

	// 期望响应时间（分钟）。
	minExpectResponseMin     = 5
	maxExpectResponseMin     = 1440 // 24 小时
	defaultExpectResponseMin = 60

	reviewContentMinLen = 10
	reviewContentMaxLen = 1000

	disputeReasonMinLen = 10
	disputeReasonMaxLen = 500

	// maxBookingDurationMin 预约时长上限（分钟）。一天服务不超过 8 小时，
	// 否则按小时价算出的金额会大得离谱，多半是前端把单位传错了。
	maxBookingDurationMin = 480
)

// BudgetLimits 返回悬赏金额上下限（分）。
func BudgetLimits() (min, max int64) { return budgetMinCents, budgetMaxCents }

// QuestionLimits 返回问题标题与描述的长度限制。
func QuestionLimits() (titleMin, titleMax, descMin, descMax int) {
	return questionTitleMinLen, questionTitleMaxLen, questionDescMinLen, questionDescMaxLen
}

// QuestionExtraLimits 返回附件与技能标签个数上限。
func QuestionExtraLimits() (attachments, skills int) {
	return maxQuestionAttachments, maxQuestionSkills
}

// ExpireHoursRange 返回悬赏有效期范围与默认值（小时）。
func ExpireHoursRange() (min, max, def int) {
	return minExpireHours, maxExpireHours, defaultExpireHours
}

// ExpectResponseRange 返回期望响应时间范围与默认值（分钟）。
func ExpectResponseRange() (min, max, def int) {
	return minExpectResponseMin, maxExpectResponseMin, defaultExpectResponseMin
}

// ReviewLimits 返回评价内容长度限制。
func ReviewLimits() (min, max int) { return reviewContentMinLen, reviewContentMaxLen }

// DisputeLimits 返回争议原因长度限制。
func DisputeLimits() (min, max int) { return disputeReasonMinLen, disputeReasonMaxLen }

// MaxBookingDurationMin 返回预约时长上限（分钟）。
func MaxBookingDurationMin() int { return maxBookingDurationMin }
