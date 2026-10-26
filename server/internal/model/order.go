package model

import "time"

// 订单状态机（与前端 orderStatusMap 一一对应，不允许自行扩展）。
//
//	悬赏型：pending_accept → pending_pay → in_service → pending_confirm → completed → settled
//	预约型：pending_pay → in_service → pending_confirm → completed → settled
//	异常分支：disputed → refunded | completed；以及 cancelled
const (
	OrderStatusPendingAccept  = "pending_accept"  // 待接单
	OrderStatusPendingPay     = "pending_pay"     // 待支付
	OrderStatusInService      = "in_service"      // 服务中
	OrderStatusPendingConfirm = "pending_confirm" // 待确认
	OrderStatusCompleted      = "completed"       // 已完成
	OrderStatusSettled        = "settled"         // 已结算
	OrderStatusDisputed       = "disputed"        // 争议
	OrderStatusRefunded       = "refunded"        // 已退款
	OrderStatusCancelled      = "cancelled"       // 已取消
)

// 订单类型。
const (
	OrderTypeQuestion = "question" // 悬赏问答
	OrderTypeBooking  = "booking"  // 预约 / 即时连线
)

// 支付方式。
const (
	PayChannelWechat  = "wechat"
	PayChannelAlipay  = "alipay"
	PayChannelOffline = "offline" // MVP 阶段人工确认
)

// 问题状态。
const (
	QuestionStatusOpen      = "open"      // 待响应
	QuestionStatusMatched   = "matched"   // 已有专家接单
	QuestionStatusClosed    = "closed"    // 已解决
	QuestionStatusExpired   = "expired"   // 超时未响应
	QuestionStatusCancelled = "cancelled" // 用户取消
)

// Question 用户发布的悬赏问题。
type Question struct {
	Base

	UserID      int64      `gorm:"index;comment:发布用户 ID" json:"userId"`
	Title       string     `gorm:"size:200;index;comment:问题标题（故障现象）" json:"title"`
	Description string     `gorm:"type:text;comment:详细描述，Markdown" json:"description"`
	Attachments StringList `gorm:"comment:日志/截图附件" json:"attachments"`
	SkillIDs    StringList `gorm:"comment:技术栈标签 ID 列表" json:"skillIds"`

	BudgetCents       int64 `gorm:"default:0;comment:悬赏金额（分）" json:"budgetCents"`
	ExpectResponseMin int   `gorm:"default:60;comment:期望响应时间（分钟）" json:"expectResponseMin"`

	Status    string     `gorm:"size:16;index;default:open;comment:状态" json:"status"`
	ExpiredAt time.Time  `gorm:"index;comment:悬赏有效期" json:"expiredAt"`
	MatchedAt *time.Time `gorm:"comment:被接单时间" json:"matchedAt"`
	ClosedAt  *time.Time `gorm:"comment:关闭时间" json:"closedAt"`
}

// TableName 固定表名。
func (Question) TableName() string { return "questions" }

// Order 交易订单。金额全部以「分」存储。
type Order struct {
	Base

	OrderNo string `gorm:"size:32;uniqueIndex;comment:订单号" json:"orderNo"`
	Type    string `gorm:"size:16;index;comment:订单类型 question/booking" json:"type"`

	UserID     int64 `gorm:"index;comment:下单用户 ID" json:"userId"`
	ExpertID   int64 `gorm:"index;comment:服务专家 ID（users.id）" json:"expertId"`
	QuestionID int64 `gorm:"comment:关联问题 ID（悬赏型）" json:"questionId"`

	Title       string `gorm:"size:200;comment:订单标题快照" json:"title"`
	ServiceMode string `gorm:"size:16;comment:服务方式快照" json:"serviceMode"`

	// 金额（分）
	AmountCents       int64 `gorm:"comment:订单总额（分）" json:"amountCents"`
	CommissionRateBP  int   `gorm:"default:1500;comment:平台抽佣，基点 1500=15%" json:"commissionRateBp"`
	CommissionCents   int64 `gorm:"comment:平台佣金（分）" json:"commissionCents"`
	ExpertAmountCents int64 `gorm:"comment:专家实收（分）" json:"expertAmountCents"`
	RefundCents       int64 `gorm:"default:0;comment:已退款（分）" json:"refundCents"`

	Status string `gorm:"size:24;index;comment:订单状态" json:"status"`

	// 时间线
	AcceptedAt      *time.Time `gorm:"comment:专家接单时间" json:"acceptedAt"`
	PayExpireAt     *time.Time `gorm:"comment:支付截止时间（下单后 15 分钟）" json:"payExpireAt"`
	PaidAt          *time.Time `gorm:"comment:支付时间" json:"paidAt"`
	StartedAt       *time.Time `gorm:"comment:服务开始时间" json:"startedAt"`
	FinishedAt      *time.Time `gorm:"comment:专家标记完成时间" json:"finishedAt"`
	ConfirmedAt     *time.Time `gorm:"comment:用户确认时间" json:"confirmedAt"`
	ConfirmDeadline *time.Time `gorm:"comment:自动确认截止时间（完成 +48h）" json:"confirmDeadline"`
	SettledAt       *time.Time `gorm:"comment:结算时间" json:"settledAt"`
	CancelledAt     *time.Time `gorm:"comment:取消时间" json:"cancelledAt"`
	// RefundedAt 与 RefundCents 必须成对：只有金额没有时间，退款就归不到哪一天，
	// 对账表的"当日退款"只能去猜订单的其他时间字段，一猜就错。
	RefundedAt *time.Time `gorm:"comment:退款时间" json:"refundedAt"`

	// 支付
	PayChannel   string `gorm:"size:16;comment:支付渠道" json:"payChannel"`
	ThirdPartyNo string `gorm:"size:64;comment:第三方交易号" json:"-"`
	CancelReason string `gorm:"size:255;comment:取消原因" json:"cancelReason"`

	// 预约信息（booking 型）
	BookedAt    *time.Time `gorm:"comment:预约开始时间" json:"bookedAt"`
	DurationMin int        `gorm:"default:0;comment:预约时长（分钟）" json:"durationMin"`

	// 其他
	Remark string `gorm:"size:500;comment:用户备注" json:"remark"`
}

// TableName 固定表名。
func (Order) TableName() string { return "orders" }

// CanTransitionTo 集中定义订单状态机，任何状态流转都必须走这里。
//
// 用法：先判后写。仓储层的状态流转一律用「带 from 条件的原子 UPDATE」，
// 于是这里的判断是给人读的规则说明，落库时的 WHERE status = from 才是并发下的真防线。
func (o *Order) CanTransitionTo(next string) bool {
	if next == o.Status {
		return false // 原地流转没有意义，还会让时间线出现两个相同时间点
	}
	allowed, ok := orderTransitions[o.Status]
	if !ok {
		return false
	}
	_, ok = allowed[next]
	return ok
}

// IsTerminal 判断是否已到终态（终态不可再流转）。
func (o *Order) IsTerminal() bool {
	switch o.Status {
	case OrderStatusSettled, OrderStatusRefunded, OrderStatusCancelled:
		return true
	}
	return false
}

// IsPaid 判断是否已经完成支付（用于区分"免费取消"与"必须走退款"）。
func (o *Order) IsPaid() bool { return o.PaidAt != nil }

// orderTransitions 是状态流转的唯一权威。
//
// 相对设计文档的「异常分支：任意状态 ──> 已取消」，这里有意收紧了取消的口子：
// 只有**付款前**的 pending_accept / pending_pay 能直接取消。付款之后必须走 disputed → refunded，
// 否则一笔已经收到钱的订单可以"取消"掉，退款金额、平台佣金、专家收入三笔账全部无处落。
// 收紧的代价只是把「用户付完钱想反悔」从"取消"改成"申请退款"，语义上本来也该如此。
//
// 另一处收紧是**删掉了 disputed → settled**。仲裁判定专家胜诉时，正确路径是
// disputed → completed → settled：先落 completed 才有 confirmed_at（仲裁确认时间），
// 评价入口与履约统计也才接得上。直连 settled 会让订单跳过"已完成"，
// 看上去钱结算了，但订单是几时完成、用户能不能评价，全都查不出来。
var orderTransitions = map[string]map[string]struct{}{
	OrderStatusPendingAccept:  set(OrderStatusPendingPay, OrderStatusCancelled),
	OrderStatusPendingPay:     set(OrderStatusInService, OrderStatusCancelled),
	OrderStatusInService:      set(OrderStatusPendingConfirm, OrderStatusDisputed),
	OrderStatusPendingConfirm: set(OrderStatusCompleted, OrderStatusDisputed, OrderStatusInService),
	OrderStatusCompleted:      set(OrderStatusSettled, OrderStatusDisputed),
	OrderStatusDisputed:       set(OrderStatusRefunded, OrderStatusCompleted, OrderStatusInService),
}

func set(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[it] = struct{}{}
	}
	return m
}

// 服务会话状态。
const (
	SessionStatusCreated   = "created"
	SessionStatusOngoing   = "ongoing"
	SessionStatusEnded     = "ended"
	SessionStatusCancelled = "cancelled"
)

// ServiceSession 一次服务会话。MVP 阶段不自研音视频，meeting_url 指向第三方会议。
type ServiceSession struct {
	Base

	OrderID    int64  `gorm:"index;comment:订单 ID" json:"orderId"`
	Channel    string `gorm:"size:16;comment:通道 im/voice/video/screen" json:"channel"`
	MeetingURL string `gorm:"size:512;comment:第三方会议链接" json:"meetingUrl"`
	Provider   string `gorm:"size:32;comment:会议服务商 tencent/feishu/manual" json:"provider"`

	Status      string     `gorm:"size:16;index;default:created;comment:状态" json:"status"`
	StartTime   *time.Time `gorm:"comment:实际开始时间（服务端计时基准）" json:"startTime"`
	EndTime     *time.Time `gorm:"comment:实际结束时间（服务端计时基准）" json:"endTime"`
	DurationSec int64      `gorm:"default:0;comment:时长（秒），由服务端计算" json:"durationSec"`

	RecordingURL        string `gorm:"size:512;comment:录像地址（V1.0 起）" json:"recordingUrl"`
	RecordingAuthorized bool   `gorm:"default:false;comment:双方是否授权录制" json:"recordingAuthorized"`
}

// TableName 固定表名。
func (ServiceSession) TableName() string { return "service_sessions" }

// Review 用户对专家服务的评价，一单一评。
type Review struct {
	Base

	OrderID  int64 `gorm:"uniqueIndex;comment:订单 ID" json:"orderId"`
	UserID   int64 `gorm:"index;comment:评价人" json:"userId"`
	ExpertID int64 `gorm:"index;comment:被评价专家" json:"expertId"`

	Score              int `gorm:"comment:综合评分 1-5" json:"score"`
	ProfessionalScore  int `gorm:"default:0;comment:专业度 1-5" json:"professionalScore"`
	SpeedScore         int `gorm:"default:0;comment:响应速度 1-5" json:"speedScore"`
	CommunicationScore int `gorm:"default:0;comment:沟通 1-5" json:"communicationScore"`

	Content   string `gorm:"size:1000;comment:评价内容" json:"content"`
	Recommend bool   `gorm:"default:true;comment:是否愿意推荐" json:"recommend"`
	Anonymous bool   `gorm:"default:false;comment:是否匿名展示" json:"anonymous"`

	Reply     string     `gorm:"size:1000;comment:专家回复" json:"reply"`
	RepliedAt *time.Time `gorm:"comment:回复时间" json:"repliedAt"`
	// 差评申诉
	Appealed   bool   `gorm:"default:false;comment:是否已申诉" json:"appealed"`
	AppealText string `gorm:"size:1000;comment:申诉理由" json:"appealText"`
}

// TableName 固定表名。
func (Review) TableName() string { return "reviews" }
