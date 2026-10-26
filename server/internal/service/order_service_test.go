package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/storage"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/service/payment"
)

// orderFixture 用临时 SQLite 搭一条真实的 repository → service 链路。
//
// 不 mock 仓储是刻意的：订单模块里最容易出错的地方恰恰在 SQL 那一层——
// 带条件的原子 UPDATE、唯一索引、事务内的余额累加。用内存里的假仓储测，
// 这些全都会被绕过，测出来的是"逻辑长得对"，不是"真的对"。
type orderFixture struct {
	svc       *OrderService
	db        *gorm.DB
	users     *repository.UserRepo
	experts   *repository.ExpertRepo
	questions *repository.QuestionRepo
	orders    *repository.OrderRepo
	buyerID   int64
	expertID  int64
	otherID   int64
	adminID   int64
	payCfg    config.PaymentConfig
}

func newOrderFixture(t *testing.T) *orderFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err, "加载默认配置失败")

	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "order.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err, "初始化测试数据库失败")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	// 附件签名需要存储驱动，否则 pkg/storage 会在 New 里直接报错。
	store, err := storage.New(storage.Options{
		Driver:   "local",
		LocalDir: t.TempDir(),
		BaseURL:  "http://localhost:8080/files",
	})
	require.NoError(t, err)
	uploads := NewUploadService(store, "test-sign-secret", 20)

	ctx := context.Background()
	users := repository.NewUserRepo(db)
	experts := repository.NewExpertRepo(db)

	buyer := &model.User{Phone: model.StrPtr("13800001001"), Nickname: "下单人",
		Role: model.RoleUser, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, buyer))
	expertUser := &model.User{Phone: model.StrPtr("13900001001"), Nickname: "陈专家",
		Role: model.RoleExpert, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, expertUser))
	other := &model.User{Phone: model.StrPtr("13900001002"), Nickname: "周专家",
		Role: model.RoleExpert, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, other))
	admin := &model.User{Phone: model.StrPtr("13800001000"), Nickname: "管理员",
		Role: model.RoleAdmin, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, admin))

	// 只有第一位专家是审核通过且开放接单的：这样"资格不够"的用例
	// 才有现成的反例，不必再造一个用户。
	require.NoError(t, experts.Create(ctx, &model.Expert{
		UserID: expertUser.ID, RealName: "陈志远",
		PricePerHourCents: 40000, PricePerTaskCents: 20000,
		ServiceModes:   model.StringList{"im", "voice", "video", "screen"},
		AcceptingOrder: true, Status: model.ExpertStatusApproved,
	}))
	require.NoError(t, experts.Create(ctx, &model.Expert{
		UserID: other.ID, RealName: "周明辉",
		PricePerHourCents: 30000,
		ServiceModes:      model.StringList{"im"},
		AcceptingOrder:    false, Status: model.ExpertStatusApproved,
	}))

	payCfg := cfg.Payment
	// 测试里把两处时限调短，否则"过期"类用例要么等很久，要么只能靠改系统时间。
	payCfg.PayTimeout = config.Duration(15 * time.Minute)
	payCfg.AutoConfirmAfter = config.Duration(48 * time.Hour)

	orders := repository.NewOrderRepo(db)
	// 抽佣比例默认走配置文件；库里没设过时 GetCommissionRateBP 会回落它。
	settingSvc := NewSettingService(
		repository.NewSettingRepo(db), repository.NewAuditRepo(db), payCfg.CommissionRateBP,
	)
	svc := NewOrderService(
		orders,
		repository.NewQuestionRepo(db),
		repository.NewReviewRepo(db),
		repository.NewServiceSessionRepo(db),
		repository.NewDisputeRepo(db),
		repository.NewSettlementRepo(db),
		experts, users, uploads,
		payment.NewMock(), payCfg, settingSvc,
	)

	return &orderFixture{
		svc: svc, db: db, users: users, experts: experts,
		questions: repository.NewQuestionRepo(db), orders: orders,
		buyerID: buyer.ID, expertID: expertUser.ID, otherID: other.ID, adminID: admin.ID,
		payCfg: payCfg,
	}
}

// publishQuestion 发布一条悬赏，返回它对应的订单号。
func (f *orderFixture) publishQuestion(t *testing.T) string {
	t.Helper()
	detail, err := f.svc.CreateQuestion(context.Background(), f.buyerID, dto.QuestionCreateReq{
		Title:             "K8s 节点频繁 NotReady 需要定位",
		Description:       "三个 worker 节点每隔几小时就 NotReady，kubelet 日志里有 PLEG 报错。",
		SkillIDs:          []string{"1"},
		BudgetCents:       20000,
		ExpectResponseMin: 30,
		ExpireHours:       72,
	})
	require.NoError(t, err)
	return detail.OrderNo
}

// grabAndPay 把一条悬赏推进到"服务中"。
func (f *orderFixture) grabAndPay(t *testing.T, orderNo string) {
	t.Helper()
	ctx := context.Background()
	_, err := f.svc.Grab(ctx, f.expertID, orderNo)
	require.NoError(t, err)
	_, err = f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: payment.ChannelMock})
	require.NoError(t, err)
}

// expertBalance 读专家当前可提现余额。
func (f *orderFixture) expertBalance(t *testing.T) int64 {
	t.Helper()
	e, err := f.experts.FindByUserID(context.Background(), f.expertID)
	require.NoError(t, err)
	return e.BalanceCents
}

/* ============================ 悬赏发布 ============================ */

func TestCreateQuestionCreatesOrderInSameTransaction(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	orderNo := f.publishQuestion(t)

	// 问题与订单一体：能查到订单，就一定有一条挂在上面的问题。
	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderTypeQuestion, o.Type)
	assert.Equal(t, model.OrderStatusPendingAccept, o.Status)
	assert.Equal(t, int64(0), o.ExpertID, "悬赏发布时还没有专家")
	assert.NotZero(t, o.QuestionID, "订单必须挂到问题上")
	// 悬赏单此时还没进入付款环节，有效期由问题自身的 expired_at 管，
	// 所以支付截止时间应当是空的。抢到单的那一刻才会填上。
	assert.Nil(t, o.PayExpireAt, "待接单的悬赏还没到付款环节")

	q, err := f.questions.GetByID(ctx, o.QuestionID)
	require.NoError(t, err)
	assert.Equal(t, model.QuestionStatusOpen, q.Status)
	assert.Equal(t, o.OrderNo, orderNo)

	// 金额恒等式在创建时就要成立，不能等到结算才算。
	assert.Equal(t, o.AmountCents, o.CommissionCents+o.ExpertAmountCents)
}

// 抽佣比例必须在下单那一刻从系统配置读取并定格成订单快照：
// 后台改比例只影响之后创建的单，已下单的不能被追溯改账——
// 否则运营调高抽佣会静默吞掉专家已经谈好的收入。
func TestCommissionRateComesFromSystemSetting(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	// 库里没设过 → 回落配置文件默认值（15%）。预算 20000 分。
	first := f.publishQuestion(t)
	o1, err := f.orders.GetByOrderNo(ctx, first)
	require.NoError(t, err)
	assert.Equal(t, 1500, o1.CommissionRateBP)
	assert.Equal(t, int64(3000), o1.CommissionCents)
	assert.Equal(t, int64(17000), o1.ExpertAmountCents)

	// 后台把抽佣调到 20%，新单按 20% 算。
	_, err = f.svc.settings.SetCommissionRateBP(
		ctx, f.adminID, model.RoleAdmin, 2000, AuditMeta{IP: "127.0.0.1"})
	require.NoError(t, err)

	second := f.publishQuestion(t)
	o2, err := f.orders.GetByOrderNo(ctx, second)
	require.NoError(t, err)
	assert.Equal(t, 2000, o2.CommissionRateBP)
	assert.Equal(t, int64(4000), o2.CommissionCents)
	assert.Equal(t, int64(16000), o2.ExpertAmountCents)

	// 老单快照一分钱都不动。
	o1Again, err := f.orders.GetByOrderNo(ctx, first)
	require.NoError(t, err)
	assert.Equal(t, 1500, o1Again.CommissionRateBP)
	assert.Equal(t, int64(3000), o1Again.CommissionCents)
}

func TestCreateQuestionRejectsBadBudget(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	minCents, maxCents := dto.BudgetLimits()

	_, err := f.svc.CreateQuestion(ctx, f.buyerID, dto.QuestionCreateReq{
		Title:       "预算过低的悬赏问题应该被拒绝",
		Description: "这是一段足够长的描述，用来确保校验失败的原因是预算而不是描述长度。",
		SkillIDs:    []string{"1"}, BudgetCents: minCents - 1,
	})
	assert.ErrorIs(t, err, errcode.ErrBudgetInvalid)

	_, err = f.svc.CreateQuestion(ctx, f.buyerID, dto.QuestionCreateReq{
		Title:       "预算过高的悬赏问题应该被拒绝",
		Description: "这是一段足够长的描述，用来确保校验失败的原因是预算而不是描述长度。",
		SkillIDs:    []string{"1"}, BudgetCents: maxCents + 1,
	})
	assert.ErrorIs(t, err, errcode.ErrBudgetInvalid)

	// 附件必须来自问题附件场景，否则可以拿私有文件的 key 冒充附件。
	_, err = f.svc.CreateQuestion(ctx, f.buyerID, dto.QuestionCreateReq{
		Title:       "带非法附件的悬赏问题应该被拒绝",
		Description: "这是一段足够长的描述，用来确保校验失败的原因是附件而不是别的字段。",
		SkillIDs:    []string{"1"}, BudgetCents: 20000,
		Attachments: []string{"expert/cert/private.pdf"},
	})
	assert.ErrorIs(t, err, errcode.ErrResourceFileInvalid)
}

/* ============================ 抢单 ============================ */

func TestGrabAssignsOrderAndMovesToPendingPay(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	orderNo := f.publishQuestion(t)
	detail, err := f.svc.Grab(ctx, f.expertID, orderNo)
	require.NoError(t, err)

	assert.Equal(t, model.OrderStatusPendingPay, detail.Status)
	assert.Equal(t, f.expertID, detail.ExpertID)

	// 抢到单就必须同时开支付倒计时：订单一进入待支付就该有截止时间，
	// 否则超时关单那条定时任务永远扫不到它。
	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	require.NotNil(t, o.PayExpireAt, "转入待支付时必须落支付截止时间")
	assert.WithinDuration(t, time.Now().Add(f.payCfg.PayTimeout.Std()), *o.PayExpireAt, 2*time.Minute)

	// 问题要同步转成已匹配，否则"我的问题"页会一直显示待响应。
	q, err := f.questions.GetByID(ctx, detail.QuestionID)
	require.NoError(t, err)
	assert.Equal(t, model.QuestionStatusMatched, q.Status)
	assert.NotNil(t, q.MatchedAt)
}

func TestGrabIsRejectedForUnqualifiedExpert(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	// otherID 虽是审核通过的专家，但没有开放接单。
	_, err := f.svc.Grab(ctx, f.otherID, orderNo)
	assert.ErrorIs(t, err, errcode.ErrExpertNotGrabable)

	// 平台管理员没有专家档案，连"专家"都不是：报错要说清是"还没入驻"，
	// 而不是笼统的"不能接单"——后者会让人以为自己是专家只是没开接单开关。
	_, err = f.svc.Grab(ctx, f.adminID, orderNo)
	assert.ErrorIs(t, err, errcode.ErrExpertNotApplied)
}

func TestGrabRejectsOwnQuestion(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	// 让下单人也拥有一份专家档案，才能走到"不能接自己的单"这一条。
	require.NoError(t, f.experts.Create(ctx, &model.Expert{
		UserID: f.buyerID, RealName: "自己", PricePerHourCents: 10000,
		ServiceModes: model.StringList{"im"}, AcceptingOrder: true,
		Status: model.ExpertStatusApproved,
	}))
	_, err := f.svc.Grab(ctx, f.buyerID, orderNo)
	assert.ErrorIs(t, err, errcode.ErrQuestionOwn)
}

func TestConcurrentGrabOnlyOneWins(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	// 造出第二个有资格抢单的专家：并发抢单的裁决必须由数据库那句
	// WHERE expert_id = 0 来下，所以两个候选人都要真的够格，
	// 否则测的就只是资格校验，而不是竞态。
	// otherID 的档案在 fixture 里已有，这里只是把接单开关打开。
	otherProfile, err := f.experts.FindByUserID(ctx, f.otherID)
	require.NoError(t, err)
	require.NoError(t, f.experts.UpdateFields(ctx, otherProfile.ID,
		map[string]any{"accepting_order": true}))

	const racers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	var wins int
	var losers []error
	// 让所有人尽量在同一瞬间发出请求。
	start := make(chan struct{})

	for i := 0; i < racers; i++ {
		expert := f.expertID
		if i%2 == 1 {
			expert = f.otherID
		}
		wg.Add(1)
		go func(expertID int64) {
			defer wg.Done()
			<-start
			_, err := f.svc.Grab(ctx, expertID, orderNo)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
				return
			}
			losers = append(losers, err)
		}(expert)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, wins, "同一笔悬赏只能被一个人抢到")
	require.Len(t, losers, racers-1)
	for _, err := range losers {
		// 抢输有两种表现：单已被别人接走，或状态已经不是待接单了
		// （抢到的人可能已经把它推到下一步）。两者都是"没抢到"。
		assert.True(t,
			errors.Is(err, errcode.ErrOrderTaken) || errors.Is(err, errcode.ErrOrderStateInvalid),
			"抢输的错误应是已被接走或状态已变，实际 %v", err)
	}
}

/* ============================ 支付 ============================ */

func TestPayRejectsUnavailableAndUnknownChannels(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	_, err := f.svc.Grab(ctx, f.expertID, orderNo)
	require.NoError(t, err)

	// 通道名拼错：这是前端传错了，属于参数问题。
	_, err = f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: "not-a-channel"})
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)

	// 通道名合法但没接通：不能假装成功，也**不能静默换成别的通道**。
	// 这一条曾经真的漏过——写成"认识的通道才切 provider"，
	// 未接通的微信支付就落到了默认的 mock 上，订单被立刻置成已支付。
	_, err = f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: payment.ChannelWechat})
	assert.ErrorIs(t, err, errcode.ErrPayChannelUnavailable)

	// 上面两次都不该动过订单状态。
	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusPendingPay, o.Status)
	assert.Nil(t, o.PaidAt, "支付没成功就不该有支付时间")
}

func TestOfflinePayLeavesOrderPending(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	_, err := f.svc.Grab(ctx, f.expertID, orderNo)
	require.NoError(t, err)

	res, err := f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: payment.ChannelOffline})
	require.NoError(t, err)
	assert.True(t, res.Manual, "线下通道不能在线唤起支付")
	assert.False(t, res.Paid, "线下转账没有到账，不能算已支付")
	assert.NotEmpty(t, res.Instruction, "线下通道必须给出下一步指引")

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusPendingPay, o.Status)
	assert.Nil(t, o.PaidAt)

	// 后台人工确认后才算收到钱。
	require.NoError(t, f.svc.AdminMarkPaid(ctx, orderNo, dto.AdminOrderMarkPaidReq{
		ThirdPartyNo: "BANK202610150001",
	}))
	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusInService, o.Status)
	require.NotNil(t, o.PaidAt)
	assert.Equal(t, "BANK202610150001", o.ThirdPartyNo)
}

func TestPayIsIdempotentAgainstDoubleClick(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	_, err := f.svc.Grab(ctx, f.expertID, orderNo)
	require.NoError(t, err)

	_, err = f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: payment.ChannelMock})
	require.NoError(t, err)
	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	firstPaidAt := *o.PaidAt

	// 第二次点支付不能把 paid_at 覆盖一遍，也不能被当成"又收到一笔钱"。
	_, err = f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: payment.ChannelMock})
	assert.ErrorIs(t, err, errcode.ErrPayAlreadyPaid)

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.True(t, firstPaidAt.Equal(*o.PaidAt), "重复支付不该改写支付时间")
}

func TestPayRejectsExpiredOrder(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	_, err := f.svc.Grab(ctx, f.expertID, orderNo)
	require.NoError(t, err)

	// 把支付截止时间拨到过去，模拟"用户点得比定时任务扫得慢"。
	past := time.Now().Add(-time.Minute)
	require.NoError(t, f.orders.UpdateFields(ctx, mustOrderID(t, f, orderNo),
		map[string]any{"pay_expire_at": past}))

	_, err = f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: payment.ChannelMock})
	assert.ErrorIs(t, err, errcode.ErrOrderPayExpired)
}

func TestPayChannelsMarksAvailability(t *testing.T) {
	f := newOrderFixture(t)
	items := f.svc.PayChannels()
	require.NotEmpty(t, items)

	byName := map[string]dto.PayChannelItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	assert.True(t, byName[payment.ChannelMock].Available)
	assert.True(t, byName[payment.ChannelOffline].Available)
	assert.False(t, byName[payment.ChannelWechat].Available, "未接通的通道不能标成可用")
	// 当前装配的是 mock，前端据此知道"不给选择时默认走哪个"。
	assert.True(t, byName[payment.ChannelMock].Current)
	for _, it := range items {
		assert.NotEmpty(t, it.Label, "通道要有中文名，否则前端只能显示英文枚举")
	}
}

/* ============================ 取消 ============================ */

func TestCancelQuestionClosesOrderAndQuestion(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	require.NoError(t, f.svc.CancelQuestion(ctx, f.buyerID, o.QuestionID, dto.QuestionCancelReq{Reason: "不需要了"}))

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusCancelled, o.Status)
	assert.Equal(t, "不需要了", o.CancelReason)

	q, err := f.questions.GetByID(ctx, o.QuestionID)
	require.NoError(t, err)
	assert.Equal(t, model.QuestionStatusCancelled, q.Status)
	assert.NotNil(t, q.ClosedAt)
}

func TestPaidOrderCannotBeCancelled(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	// 钱已经进来，退出只能走争议/退款；直接取消会让退款额、平台佣金、
	// 专家实收三笔账无处落。
	err := f.svc.Cancel(ctx, f.buyerID, orderNo, dto.OrderCancelReq{Reason: "我不想要了"})
	assert.ErrorIs(t, err, errcode.ErrOrderCancelDenied)

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusInService, o.Status)
}

func TestCancelOwnershipIsEnforced(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	// 服务专家也不能替下单人取消订单。
	err := f.svc.Cancel(ctx, f.expertID, orderNo, dto.OrderCancelReq{})
	assert.ErrorIs(t, err, errcode.ErrOrderNotOwner)
}

/* ============================ 履约与结算 ============================ */

func TestFinishConfirmSettlesAndCreditsExpert(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	amount, commission, expertAmount := o.AmountCents, o.CommissionCents, o.ExpertAmountCents
	require.Equal(t, amount, commission+expertAmount)
	require.Equal(t, int64(0), f.expertBalance(t), "结算前余额应为 0")

	// 只有服务专家能标记完成。
	require.ErrorIs(t, f.svc.FinishService(ctx, f.buyerID, orderNo), errcode.ErrOrderNotExpert)
	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusPendingConfirm, o.Status)
	require.NotNil(t, o.ConfirmDeadline, "进入待确认时必须落自动确认截止时间")

	// 确认完成即结算，不需要用户再点一次。
	require.NoError(t, f.svc.ConfirmComplete(ctx, f.buyerID, orderNo))

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusSettled, o.Status)
	require.NotNil(t, o.ConfirmedAt)

	assert.Equal(t, expertAmount, f.expertBalance(t), "结算后余额应等于专家实收")

	detail, err := f.svc.OrderDetail(ctx, f.buyerID, orderNo)
	require.NoError(t, err)
	require.NotNil(t, detail.Settlement, "详情里要能看到分账凭证")
	assert.Equal(t, commission, detail.Settlement.CommissionCents)
	assert.Equal(t, expertAmount, detail.Settlement.ExpertAmountCents)
	assert.Equal(t, amount, detail.Settlement.AmountCents)
	// 时间轴至少要有关键节点，前端直接渲染即可。
	assert.NotEmpty(t, detail.Timeline)
}

func TestSettleIsIdempotent(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)
	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))
	require.NoError(t, f.svc.ConfirmComplete(ctx, f.buyerID, orderNo))

	before := f.expertBalance(t)

	// 再结算一次：必须被拒，且余额不能再涨。
	assert.ErrorIs(t, f.svc.AdminSettle(ctx, orderNo, dto.AdminSettleReq{}), errcode.ErrSettleDuplicated)
	assert.Equal(t, before, f.expertBalance(t), "重复结算不能重复打钱")

	// 后台强制流转到 settled 也受状态机约束：订单已结算，settled 不是它的下一步。
	// （不是 ErrSettleDuplicated——状态机在更早的一步就把它拦下了，
	// 原地流转本来就是禁止的，这条规则对所有状态一视同仁。）
	assert.ErrorIs(t, f.svc.AdminTransition(ctx, orderNo, dto.AdminOrderTransitionReq{To: model.OrderStatusSettled}),
		errcode.ErrOrderStateInvalid)
	assert.Equal(t, before, f.expertBalance(t))

	// 兜底扫描不该重复打钱。
	settled, err := f.orders.ListSettleable(ctx, 50)
	require.NoError(t, err)
	for _, s := range settled {
		assert.NotEqual(t, orderNo, s.OrderNo, "已结算的订单不该出现在待结算列表里")
	}
}

func TestSettleRefusesUnbalancedSnapshot(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	// 先走合法流转把订单推到"已完成"（不能直接用 UpdateFields 改状态，
	// 仓储层拒绝改 status，这正是我们想要的防线）。
	require.NoError(t, f.orders.Transition(ctx, orderNo,
		model.OrderStatusInService, model.OrderStatusCompleted,
		map[string]any{"confirmed_at": time.Now()}))

	// 再人为把佣金改坏，模拟历史脏数据。金额不平就不能把钱打出去。
	require.NoError(t, f.orders.UpdateFields(ctx, o.ID, map[string]any{
		"commission_cents": o.CommissionCents + 1,
	}))

	err = f.svc.AdminSettle(ctx, orderNo, dto.AdminSettleReq{})
	assert.ErrorIs(t, err, errcode.ErrSettleAmountBad)
	assert.Equal(t, int64(0), f.expertBalance(t), "金额不平不能动专家余额")
}

func TestCommissionUsesOrderSnapshotNotCurrentConfig(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)
	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	snapshotCommission := o.CommissionCents
	snapshotExpert := o.ExpertAmountCents

	// 结算前把配置里的抽佣比例改成 50%。结算必须仍按订单快照来，
	// 否则"下单后运营调高抽佣"会静默吞掉专家已经谈好的收入。
	f.svc.payCfg.CommissionRateBP = 5000
	require.NoError(t, f.svc.ConfirmComplete(ctx, f.buyerID, orderNo))

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, snapshotCommission, o.CommissionCents, "佣金应保持下单时的快照")
	assert.Equal(t, snapshotExpert, f.expertBalance(t), "专家实收应保持下单时的快照")
}

func TestConfirmRequiresRightState(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	_, err := f.svc.Grab(ctx, f.expertID, orderNo)
	require.NoError(t, err)

	// 还没开始服务就确认完成：状态机要拦住。
	assert.ErrorIs(t, f.svc.ConfirmComplete(ctx, f.buyerID, orderNo), errcode.ErrOrderStateInvalid)

	// 待确认阶段只有下单人能确认。
	_, err = f.svc.Pay(ctx, f.buyerID, orderNo, dto.OrderPayReq{Channel: payment.ChannelMock})
	require.NoError(t, err)
	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))
	assert.ErrorIs(t, f.svc.ConfirmComplete(ctx, f.expertID, orderNo), errcode.ErrOrderNotOwner)
}

func TestAutoConfirmRespectsDeadline(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)
	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))

	// 截止时间还没到：定时任务即使被手动触发也不能提前把订单完成。
	n, err := f.svc.AutoConfirmExpired(ctx, 50)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusPendingConfirm, o.Status)

	// 把截止时间拨到过去，这时才该轮到自动确认。
	require.NoError(t, f.orders.UpdateFields(ctx, o.ID, map[string]any{
		"confirm_deadline": time.Now().Add(-time.Minute),
	}))
	n, err = f.svc.AutoConfirmExpired(ctx, 50)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusSettled, o.Status, "自动确认后也要走结算，不能停在已完成")
}

func TestAutoCancelExpiredPaymentsLeavesPaidOrderAlone(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	expiredNo := f.publishQuestion(t)
	_, err := f.svc.Grab(ctx, f.expertID, expiredNo)
	require.NoError(t, err)
	require.NoError(t, f.orders.UpdateFields(ctx, mustOrderID(t, f, expiredNo),
		map[string]any{"pay_expire_at": time.Now().Add(-time.Hour)}))

	// 另一单已经付过款：即使它的支付截止时间也过期了，也不能被取消。
	paidNo := f.publishQuestion(t)
	f.grabAndPay(t, paidNo)
	require.NoError(t, f.orders.UpdateFields(ctx, mustOrderID(t, f, paidNo),
		map[string]any{"pay_expire_at": time.Now().Add(-time.Hour)}))

	n, err := f.svc.AutoCancelExpiredPayments(ctx, 50)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	o, err := f.orders.GetByOrderNo(ctx, expiredNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusCancelled, o.Status)

	o, err = f.orders.GetByOrderNo(ctx, paidNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusInService, o.Status, "已支付的订单不能被超时关单误伤")
}

/* ============================ 评价 ============================ */

func TestReviewOnlyAfterCompletionAndOnlyOnce(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	req := dto.OrderReviewReq{Score: 5, Content: "定位很快，讲得也清楚，问题当场就解决了。"}

	// 还没完成不能评价。
	_, err := f.svc.Review(ctx, f.buyerID, orderNo, req)
	assert.ErrorIs(t, err, errcode.ErrReviewNotAllowed)

	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))
	require.NoError(t, f.svc.ConfirmComplete(ctx, f.buyerID, orderNo))

	// 结算之后仍要能评价：用户点完确认的下一秒就去评价，按钮不该是灰的。
	_, err = f.svc.Review(ctx, f.buyerID, orderNo, req)
	require.NoError(t, err)

	// 一单一评，重复提交要被唯一索引挡住。
	_, err = f.svc.Review(ctx, f.buyerID, orderNo, req)
	assert.ErrorIs(t, err, errcode.ErrReviewDuplicated)
}

func TestReviewUpdatesExpertRatingAndReplyIsExpertOnly(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)
	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))
	require.NoError(t, f.svc.ConfirmComplete(ctx, f.buyerID, orderNo))

	_, err := f.svc.Review(ctx, f.buyerID, orderNo, dto.OrderReviewReq{
		Score: 4, ProfessionalScore: 5, SpeedScore: 3, CommunicationScore: 4,
		Content: "整体不错，响应稍慢了一点，但问题最终解决了。", Recommend: true,
	})
	require.NoError(t, err)

	e, err := f.experts.FindByUserID(ctx, f.expertID)
	require.NoError(t, err)
	assert.Equal(t, 4.0, e.Rating, "收到评价后应刷新专家评分")
	assert.Equal(t, int64(1), e.OrderCount, "完成订单数应被刷新")

	// 回复只允许服务专家。
	reply := dto.OrderReviewReplyReq{Reply: "感谢反馈，后续我会更快响应。"}
	assert.ErrorIs(t, f.svc.ReplyReview(ctx, f.buyerID, orderNo, reply), errcode.ErrOrderNotExpert)
	require.NoError(t, f.svc.ReplyReview(ctx, f.expertID, orderNo, reply))

	detail, err := f.svc.OrderDetail(ctx, f.buyerID, orderNo)
	require.NoError(t, err)
	require.NotNil(t, detail.Review)
	assert.Equal(t, reply.Reply, detail.Review.Reply)
	require.NotNil(t, detail.Review.RepliedAt)
}

/* ============================ 争议 ============================ */

func TestDisputeFlowAndResolution(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	item, err := f.svc.RaiseDispute(ctx, f.buyerID, orderNo, dto.OrderDisputeReq{
		Reason: "服务内容与约定不一致，希望能退一部分费用。",
	})
	require.NoError(t, err)
	require.NotNil(t, item.Deadline, "争议要有处理时限")

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusDisputed, o.Status)

	// 同一订单不能同时挂两条未结案争议。
	_, err = f.svc.RaiseDispute(ctx, f.expertID, orderNo, dto.OrderDisputeReq{
		Reason: "我也觉得需要平台介入处理这件事。",
	})
	assert.ErrorIs(t, err, errcode.ErrDisputeExists)

	// 争议中不能走确认完成这条路。
	assert.ErrorIs(t, f.svc.ConfirmComplete(ctx, f.buyerID, orderNo), errcode.ErrOrderStateInvalid)

	// 争议不能一步跳到已结算：那会让订单绕过"已完成"，既没有确认时间，
	// 也失去了评价入口。
	assert.ErrorIs(t, f.svc.AdminTransition(ctx, orderNo,
		dto.AdminOrderTransitionReq{To: model.OrderStatusSettled}), errcode.ErrOrderStateInvalid)

	require.NoError(t, f.svc.AdminResolveDispute(ctx, f.adminID, item.ID,
		model.OrderStatusRefunded, "经核实服务确有缺失，按约定全额退款。"))

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusRefunded, o.Status)
	assert.Equal(t, o.AmountCents, o.RefundCents)

	// 结案后的争议仍要能在详情里看到结论，否则用户只看到"已退款"。
	detail, err := f.svc.OrderDetail(ctx, f.buyerID, orderNo)
	require.NoError(t, err)
	require.NotNil(t, detail.Dispute, "结案后的争议不能从详情里消失")
	assert.Equal(t, model.DisputeStatusResolved, detail.Dispute.Status)
	assert.NotEmpty(t, detail.Dispute.Result)

	// 退款订单不该能评价。
	_, err = f.svc.Review(ctx, f.buyerID, orderNo, dto.OrderReviewReq{
		Score: 1, Content: "这单是退款收尾的，不该允许评价。",
	})
	assert.ErrorIs(t, err, errcode.ErrReviewNotAllowed)
}

func TestDisputeResolveToCompletedSettles(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	item, err := f.svc.RaiseDispute(ctx, f.buyerID, orderNo, dto.OrderDisputeReq{
		Reason: "对服务效果有疑问，希望平台判断。",
	})
	require.NoError(t, err)

	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	expected := o.ExpertAmountCents

	require.NoError(t, f.svc.AdminResolveDispute(ctx, f.adminID, item.ID,
		model.OrderStatusCompleted, "服务确有交付，判定完成。"))

	o, err = f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusSettled, o.Status, "判定完成后要一并结算")
	assert.Equal(t, expected, f.expertBalance(t))
}

/* ============================ 预约下单 ============================ */

func TestCreateBookingPricesOnServer(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	item, err := f.svc.CreateBooking(ctx, f.buyerID, dto.OrderCreateReq{
		ExpertUserID: f.expertID,
		ServiceMode:  "im",
		DurationMin:  90,
	})
	require.NoError(t, err)

	e, err := f.experts.FindByUserID(ctx, f.expertID)
	require.NoError(t, err)
	// 按小时定价 × 1.5 小时，向上取整到分。
	want := (e.PricePerHourCents*90 + 59) / 60
	assert.Equal(t, want, item.AmountCents, "金额必须由服务端算，不能来自请求")
	assert.Equal(t, want, item.CommissionCents+item.ExpertAmountCents)
	assert.Equal(t, model.OrderStatusPendingPay, item.Status)
	assert.Equal(t, f.expertID, item.ExpertID)
	assert.NotEmpty(t, item.ExpertName, "列表里要直接带昵称，省掉前端再发一轮请求")
}

func TestCreateBookingValidations(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	// 不能给自己下单。
	_, err := f.svc.CreateBooking(ctx, f.expertID, dto.OrderCreateReq{
		ExpertUserID: f.expertID, ServiceMode: "im", DurationMin: 60,
	})
	assert.ErrorIs(t, err, errcode.ErrOrderSelfDealing)

	// 未开放接单的专家不能下单。这一条会先于"服务方式不支持"命中——
	// 对用户来说先解释"这个人现在不接单"更有意义。
	_, err = f.svc.CreateBooking(ctx, f.buyerID, dto.OrderCreateReq{
		ExpertUserID: f.otherID, ServiceMode: "im", DurationMin: 60,
	})
	assert.ErrorIs(t, err, errcode.ErrExpertNotAccepting)

	// 把接单开关打开后，才轮到"服务方式"这一层校验：
	// otherID 只支持 im，不支持 video。
	otherProfile, err := f.experts.FindByUserID(ctx, f.otherID)
	require.NoError(t, err)
	require.NoError(t, f.experts.UpdateFields(ctx, otherProfile.ID,
		map[string]any{"accepting_order": true}))
	_, err = f.svc.CreateBooking(ctx, f.buyerID, dto.OrderCreateReq{
		ExpertUserID: f.otherID, ServiceMode: "video", DurationMin: 60,
	})
	assert.ErrorIs(t, err, errcode.ErrServiceModeDenied)

	// 时长超上限。
	_, err = f.svc.CreateBooking(ctx, f.buyerID, dto.OrderCreateReq{
		ExpertUserID: f.expertID, ServiceMode: "im", DurationMin: dto.MaxBookingDurationMin() + 1,
	})
	assert.ErrorIs(t, err, errcode.ErrBookTimeInvalid)

	// 预约时间在过去。
	past := time.Now().Add(-time.Hour)
	_, err = f.svc.CreateBooking(ctx, f.buyerID, dto.OrderCreateReq{
		ExpertUserID: f.expertID, ServiceMode: "im", DurationMin: 60, BookedAt: &past,
	})
	assert.ErrorIs(t, err, errcode.ErrBookTimeInvalid)

	// 用档案 ID 当用户 ID 传进来，必须落到"查不到这个人"，而不是下单给别人。
	_, err = f.svc.CreateBooking(ctx, f.buyerID, dto.OrderCreateReq{
		ExpertUserID: 999999, ServiceMode: "im", DurationMin: 60,
	})
	assert.Error(t, err)
}

/* ============================ 会话与访问控制 ============================ */

func TestSessionLifecycle(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	first, err := f.svc.StartSession(ctx, f.expertID, orderNo, "video")
	require.NoError(t, err)
	require.NotNil(t, first.StartTime)

	// 复用：反复进入沟通不该多出一条会话，否则时长会被算成几倍。
	again, err := f.svc.StartSession(ctx, f.buyerID, orderNo, "")
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "已有进行中的会话应被复用")

	// 新建会话时才要求通道合法。
	ended, err := f.svc.EndSession(ctx, f.buyerID, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.SessionStatusEnded, ended.Status)
	require.NotNil(t, ended.EndTime)
	assert.GreaterOrEqual(t, ended.DurationSec, int64(0), "时长由服务端算，且不能为负")

	_, err = f.svc.StartSession(ctx, f.buyerID, orderNo, "")
	assert.ErrorIs(t, err, errcode.ErrParamInvalid, "会话已结束后新建必须给通道")

	_, err = f.svc.StartSession(ctx, f.buyerID, orderNo, "carrier-pigeon")
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)
}

func TestFinishClosesLingeringSession(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	sess, err := f.svc.StartSession(ctx, f.expertID, orderNo, "im")
	require.NoError(t, err)

	// 专家直接点完成，忘了点结束会话：那条会话不能一直挂着继续计时。
	require.NoError(t, f.svc.FinishService(ctx, f.expertID, orderNo))

	detail, err := f.svc.OrderDetail(ctx, f.buyerID, orderNo)
	require.NoError(t, err)
	require.Len(t, detail.Sessions, 1)
	assert.Equal(t, sess.ID, detail.Sessions[0].ID)
	assert.Equal(t, model.SessionStatusEnded, detail.Sessions[0].Status)
	require.NotNil(t, detail.Sessions[0].EndTime)
}

func TestOrderAccessControl(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	// 下单人与服务专家都能看；无关的人不能。
	_, err := f.svc.OrderDetail(ctx, f.buyerID, orderNo)
	require.NoError(t, err)

	_, err = f.svc.OrderDetail(ctx, f.otherID, orderNo)
	assert.ErrorIs(t, err, errcode.ErrOrderNotOwner)

	// 后台不受"只有双方能看"的限制。
	_, err = f.svc.AdminDetail(ctx, orderNo)
	require.NoError(t, err)

	// 列表按侧分：我下的单 vs 我接的单。
	items, total, err := f.svc.ListOrders(ctx, f.expertID, dto.OrderListQuery{Side: "expert"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, items, "还没抢单时『我接的单』应为空")

	items, total, err = f.svc.ListOrders(ctx, f.buyerID, dto.OrderListQuery{Side: "user"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.False(t, items[0].Reviewed)

	// 待接单的悬赏还没进入付款环节，所以不该有支付倒计时——
	// 带上它前端会渲染一个"还剩 xx 分钟"的假倒计时。
	assert.Nil(t, items[0].PayExpireAt)
	assert.Nil(t, items[0].ConfirmDeadline)

	counts, err := f.svc.OrderCounts(ctx, f.buyerID, "user")
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts.Total)
	assert.Equal(t, int64(1), counts.ByStag[model.OrderStatusPendingAccept])
}

func TestListHidesStaleCountdowns(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)
	f.grabAndPay(t, orderNo)

	items, _, err := f.svc.ListOrders(ctx, f.buyerID, dto.OrderListQuery{Side: "user"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	// 服务中的订单不该再带支付倒计时，否则前端会渲染一个过期的倒计时。
	assert.Nil(t, items[0].PayExpireAt)
	assert.Nil(t, items[0].ConfirmDeadline)
}

func TestHallOnlyShowsUnassignedOpenQuestions(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	taken := f.publishQuestion(t)
	free := f.publishQuestion(t)
	_, err := f.svc.Grab(ctx, f.expertID, taken)
	require.NoError(t, err)

	items, total, err := f.svc.Hall(ctx, dto.HallListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)

	require.Len(t, items, 1)
	assert.Equal(t, free, items[0].OrderNo, "大厅里必须带上抢单要用的订单号")
	assert.NotZero(t, items[0].OrderID)
	assert.NotZero(t, items[0].QuestionItem.ID, "大厅每条记录的内容来自它挂着的问题")

	// 已匹配的问题不再出现在大厅，即使订单还在等待支付。
	assert.NotEqual(t, taken, items[0].OrderNo)
}

/* ============================ 后台流转 ============================ */

func TestAdminTransitionStillObeysStateMachine(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	// 待接单不能一步跳到已完成：状态机里没有这条路。
	err := f.svc.AdminTransition(ctx, orderNo, dto.AdminOrderTransitionReq{To: model.OrderStatusCompleted})
	assert.ErrorIs(t, err, errcode.ErrOrderStateInvalid)

	// 合法的一步要能走通，并带上该状态应有的时间戳。
	require.NoError(t, f.svc.AdminTransition(ctx, orderNo, dto.AdminOrderTransitionReq{
		To: model.OrderStatusCancelled, Remark: "客户电话说明不需要了",
	}))
	o, err := f.orders.GetByOrderNo(ctx, orderNo)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusCancelled, o.Status)
	require.NotNil(t, o.CancelledAt)
	assert.Equal(t, "客户电话说明不需要了", o.CancelReason)
}

func TestAdminTransitionRejectsUnknownStatus(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	orderNo := f.publishQuestion(t)

	err := f.svc.AdminTransition(ctx, orderNo, dto.AdminOrderTransitionReq{To: "whatever"})
	assert.ErrorIs(t, err, errcode.ErrOrderStateInvalid)
}

/* ============================ 辅助 ============================ */

func mustOrderID(t *testing.T, f *orderFixture, orderNo string) int64 {
	t.Helper()
	o, err := f.orders.GetByOrderNo(context.Background(), orderNo)
	require.NoError(t, err)
	return o.ID
}
