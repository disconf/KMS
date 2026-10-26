package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allStatuses 是状态机上出现过的全部状态。
// 新增状态时必须同步加进来，否则下面的全量用例会漏掉它与其它状态的组合。
var allStatuses = []string{
	OrderStatusPendingAccept,
	OrderStatusPendingPay,
	OrderStatusInService,
	OrderStatusPendingConfirm,
	OrderStatusCompleted,
	OrderStatusSettled,
	OrderStatusDisputed,
	OrderStatusRefunded,
	OrderStatusCancelled,
}

// allowedTransitions 是**独立于 orderTransitions 重写一遍**的期望表。
//
// 不能直接断言 orderTransitions 的内容——那就是把实现抄一遍，改错了断言也跟着错。
// 这份表是按设计文档的状态图逐个核对出来的，改动状态机的人必须同时改这里，
// 也就被迫回答"我改的是不是产品要的"。
var allowedTransitions = map[string][]string{
	OrderStatusPendingAccept:  {OrderStatusPendingPay, OrderStatusCancelled},
	OrderStatusPendingPay:     {OrderStatusInService, OrderStatusCancelled},
	OrderStatusInService:      {OrderStatusPendingConfirm, OrderStatusDisputed},
	OrderStatusPendingConfirm: {OrderStatusCompleted, OrderStatusDisputed, OrderStatusInService},
	OrderStatusCompleted:      {OrderStatusSettled, OrderStatusDisputed},
	OrderStatusDisputed:       {OrderStatusRefunded, OrderStatusCompleted, OrderStatusInService},
	OrderStatusSettled:        {},
	OrderStatusRefunded:       {},
	OrderStatusCancelled:      {},
}

// TestOrderTransitionMatrixIsExact 穷举 9×9 全部组合，与期望表逐个比对。
//
// 只测几条主干路径是不够的：状态机出错的方式是"多放行一条"，而不是"少放行一条"。
// 多放行的方向往往是钱——比如允许 disputed 直接 settled、或者允许 cancelled 再流转。
// 只有穷举才能发现多出来的那一条。
func TestOrderTransitionMatrixIsExact(t *testing.T) {
	// 先确认期望表本身覆盖了全部状态（漏一个就意味着那行没被测）。
	require.Len(t, allowedTransitions, len(allStatuses), "期望表与状态清单长度不一致")

	for _, from := range allStatuses {
		expect, ok := allowedTransitions[from]
		require.Truef(t, ok, "状态 %s 不在期望表里，新增状态要同步补进来", from)
		allowed := make(map[string]bool, len(expect))
		for _, to := range expect {
			allowed[to] = true
		}

		for _, to := range allStatuses {
			o := &Order{Status: from}
			got := o.CanTransitionTo(to)
			if allowed[to] {
				assert.Truef(t, got, "%s -> %s 应当放行", from, to)
			} else {
				assert.Falsef(t, got, "%s -> %s 不该放行", from, to)
			}
		}
	}
}

// TestOrderSelfTransitionRejected 原地流转必须被拒。
//
// 允许的话会出现"确认了两次完成"，时间线上留下两个 confirmed_at 语义点，
// 结算任务按状态扫描时还可能重复入账。
func TestOrderSelfTransitionRejected(t *testing.T) {
	for _, s := range allStatuses {
		o := &Order{Status: s}
		assert.Falsef(t, o.CanTransitionTo(s), "%s 不该能流转到自身", s)
	}
}

// TestOrderTerminalStatesAreSealed 终态必须出不去。
func TestOrderTerminalStatesAreSealed(t *testing.T) {
	for _, s := range []string{OrderStatusSettled, OrderStatusRefunded, OrderStatusCancelled} {
		o := &Order{Status: s}
		assert.Truef(t, o.IsTerminal(), "%s 应是终态", s)
		for _, to := range allStatuses {
			assert.Falsef(t, o.CanTransitionTo(to), "终态 %s 不该能流转到 %s", s, to)
		}
	}
}

// TestOrderNonTerminalStatesAreOpen 非终态至少要有一条出路，否则订单会卡死。
func TestOrderNonTerminalStatesAreOpen(t *testing.T) {
	for _, s := range allStatuses {
		o := &Order{Status: s}
		if o.IsTerminal() {
			continue
		}
		out := 0
		for _, to := range allStatuses {
			if o.CanTransitionTo(to) {
				out++
			}
		}
		assert.NotZerof(t, out, "%s 不是终态却没有任何出路，订单会卡在这里", s)
	}
}

// TestOrderHappyPaths 两条主干链路必须走得通。
func TestOrderHappyPaths(t *testing.T) {
	// 悬赏问答型：发布问题即落 pending_accept，专家抢单 → pending_pay。
	questionFlow := []string{
		OrderStatusPendingAccept,
		OrderStatusPendingPay,
		OrderStatusInService,
		OrderStatusPendingConfirm,
		OrderStatusCompleted,
		OrderStatusSettled,
	}
	assertPathWalkable(t, "悬赏问答型", questionFlow)

	// 预约连线型：创建即待支付。
	bookingFlow := []string{
		OrderStatusPendingPay,
		OrderStatusInService,
		OrderStatusPendingConfirm,
		OrderStatusCompleted,
		OrderStatusSettled,
	}
	assertPathWalkable(t, "预约连线型", bookingFlow)
}

func assertPathWalkable(t *testing.T, name string, path []string) {
	t.Helper()
	o := &Order{Status: path[0]}
	for _, next := range path[1:] {
		require.Truef(t, o.CanTransitionTo(next),
			"%s 链路走不通：%s -> %s", name, o.Status, next)
		o.Status = next
	}
	assert.Equal(t, path[len(path)-1], o.Status)
}

// TestDisputedCannotSettleDirectly 守住"仲裁不能跳过完成态"这条。
//
// 这是一条回归用例：状态机原本允许 disputed -> settled。
// 直连结算的问题是订单从未落 completed，于是 confirmed_at 为空、评价入口消失，
// 平台看板上这笔单"钱结算了但履约没完成"，对账时无从解释。正确路径是
// disputed -> completed -> settled。
func TestDisputedCannotSettleDirectly(t *testing.T) {
	o := &Order{Status: OrderStatusDisputed}
	assert.False(t, o.CanTransitionTo(OrderStatusSettled), "争议不能直接结算，必须先落已完成")

	// 正路依然通畅，别把口子焊死了。
	require.True(t, o.CanTransitionTo(OrderStatusCompleted))
	o.Status = OrderStatusCompleted
	assert.True(t, o.CanTransitionTo(OrderStatusSettled))
}

// TestPaidOrderCannotBeCancelled 守住"付款后只能走退款，不能取消"这条。
//
// 也是回归用例：设计文档的异常分支写的是"任意状态 ──> 已取消"，
// 照着实现会让一笔已收款订单被"取消"掉——退款额、平台佣金、专家收入三笔账
// 全部无处落，且订单状态看起来干净得像什么都没发生。
func TestPaidOrderCannotBeCancelled(t *testing.T) {
	for _, s := range []string{
		OrderStatusInService,
		OrderStatusPendingConfirm,
		OrderStatusCompleted,
		OrderStatusSettled,
	} {
		o := &Order{Status: s}
		assert.Falsef(t, o.CanTransitionTo(OrderStatusCancelled),
			"已进入服务流程的 %s 不该能直接取消；已收款订单的退出必须走争议/退款", s)
	}

	// 付款前两个状态仍然可以取消（含超时自动取消）。
	assert.True(t, (&Order{Status: OrderStatusPendingAccept}).CanTransitionTo(OrderStatusCancelled))
	assert.True(t, (&Order{Status: OrderStatusPendingPay}).CanTransitionTo(OrderStatusCancelled))
}

// TestIsPaidTracksPaidAt 支付态以 paid_at 为准，不看状态。
//
// 因为 in_service 既可能来自"已支付"，也可能来自争议后"驳回争议继续服务"，
// 用状态反推是否收过钱会漏判，进而把已收款订单当成未收款处理。
func TestIsPaidTracksPaidAt(t *testing.T) {
	assert.False(t, (&Order{Status: OrderStatusPendingPay}).IsPaid())

	now := time.Now()
	assert.True(t, (&Order{Status: OrderStatusRefunded, PaidAt: &now}).IsPaid(),
		"已退款的订单历史上确实收过钱，IsPaid 应为真")
}

// TestUnknownStatusRejectsEverything 未知状态一律不放行。
//
// 数据库里存进一个拼错的状态（比如手工改库）时，不能因为"没查到这个 key"
// 就默认放行——那会让脏数据顺着状态机流下去。
func TestUnknownStatusRejectsEverything(t *testing.T) {
	o := &Order{Status: "totally_wrong"}
	assert.False(t, o.IsTerminal())
	for _, to := range allStatuses {
		assert.Falsef(t, o.CanTransitionTo(to), "未知状态不该能流转到 %s", to)
	}
}
