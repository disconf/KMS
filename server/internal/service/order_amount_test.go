package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/dto"
)

// TestSplitCommissionKeepsTotalIntact 是金额模块最重要的一条。
//
// 抽佣拆分必须满足恒等式：佣金 + 专家实收 == 订单总额。
// 用「各算各的」写法（两个方向都乘一遍再除）在除不尽时会少 1 分或多 1 分，
// 表现是专家收入与平台佣金加起来对不上流水；如果方向相反还会出现
// 「付出去的钱比收进来的多」，跑一段时间后余额就为负。
//
// 这里把所有"10000 除不尽"的金额与常见抽佣比例都跑一遍，
// 靠穷举而不是靠想清楚——这类舍入问题人脑推不准。
func TestSplitCommissionKeepsTotalIntact(t *testing.T) {
	rates := []int{0, 1, 100, 500, 1000, 1500, 2000, 3333, 5000, 9999, 10000}

	for cents := int64(1); cents <= 2000; cents++ {
		for _, bp := range rates {
			commission, expert := splitCommission(cents, bp)

			assert.Equalf(t, cents, commission+expert,
				"总额 %d 分、抽佣 %d bp 时拆分不平：佣金 %d + 实收 %d",
				cents, bp, commission, expert)
			assert.GreaterOrEqualf(t, commission, int64(0), "佣金不能为负")
			assert.GreaterOrEqualf(t, expert, int64(0), "实收不能为负")
		}
	}
}

// TestSplitCommissionZeroRate 抽佣为 0 时全部归专家。
func TestSplitCommissionZeroRate(t *testing.T) {
	commission, expert := splitCommission(19900, 0)
	assert.Equal(t, int64(0), commission)
	assert.Equal(t, int64(19900), expert)
}

// TestSplitCommissionFullRate 100% 抽佣也要保证实收不为负。
func TestSplitCommissionFullRate(t *testing.T) {
	commission, expert := splitCommission(19900, 10000)
	assert.Equal(t, int64(19900), commission)
	assert.Equal(t, int64(0), expert)

	// 超过 100% 的异常配置被钳住，不能把实收算成负数。
	commission, expert = splitCommission(19900, 20000)
	assert.Equal(t, int64(19900), commission)
	assert.Equal(t, int64(0), expert)
}

// TestSplitCommissionHandlesNonPositiveAmount 非正金额一律返回 0。
//
// 不能让它顺着公式算出负数然后流转下去：一笔 -100 分的订单
// 会把专家的余额扣掉 85 分，而这笔钱从哪来的没人说得清。
func TestSplitCommissionHandlesNonPositiveAmount(t *testing.T) {
	for _, cents := range []int64{0, -1, -19900} {
		commission, expert := splitCommission(cents, 1500)
		assert.Equalf(t, int64(0), commission, "金额 %d 的佣金应为 0", cents)
		assert.Equalf(t, int64(0), expert, "金额 %d 的实收应为 0", cents)
	}
}

// TestSplitCommissionRoundsDownOnPlatformSide 除不尽时差额落在平台侧。
//
// 199 分的 15% 是 29.85 分，向下取整为 29 分，专家拿 170 分。
// 这个方向是刻意选的：宁可平台少收一分，也不要出现"专家实收比应得少"
// 这类需要人工向专家解释的差异。
func TestSplitCommissionRoundsDownOnPlatformSide(t *testing.T) {
	commission, expert := splitCommission(199, 1500)
	assert.Equal(t, int64(29), commission)
	assert.Equal(t, int64(170), expert)
}

// TestBookingAmountRoundsUpToCent 按时长折算要向上取整到分。
func TestBookingAmountRoundsUpToCent(t *testing.T) {
	// 每小时 199 元（19900 分）。
	const hourly int64 = 19900

	// 恰好一小时。
	got, err := bookingAmount(hourly, 0, 60)
	require.NoError(t, err)
	assert.Equal(t, int64(19900), got)

	// 半小时。
	got, err = bookingAmount(hourly, 0, 30)
	require.NoError(t, err)
	assert.Equal(t, int64(9950), got)

	// 20 分钟 → 19900*20/60 = 6633.33…，向上取整为 6634。
	// 截断会得到 6633，长期下来平台与专家都在少收钱。
	got, err = bookingAmount(hourly, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(6634), got, "不足一分钟的部分应向上取整到分")

	// 1 分钟也必须收到钱，不能截断成 0。
	got, err = bookingAmount(hourly, 0, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(332), got)
	assert.Greater(t, got, int64(0), "再短的时长也不能算成 0 元")
}

// TestBookingAmountFallsBackToTaskPrice 没给时长时用按次定价。
func TestBookingAmountFallsBackToTaskPrice(t *testing.T) {
	got, err := bookingAmount(19900, 5000, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), got, "没给时长应按次计价，而不是按 0 分钟算出 0 元")

	// 给了时长但专家没配小时价 → 回落到按次价。
	got, err = bookingAmount(0, 5000, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), got)
}

// TestBookingAmountRejectsUnpricedExpert 未定价的专家不能被下成 0 元单。
//
// 允许 0 元成交等于留了一条"免费领取服务"的通道，而且它看起来完全合法
// （金额是 0 不是负数），只有对账时才会发现整段时间的收入是 0。
func TestBookingAmountRejectsUnpricedExpert(t *testing.T) {
	_, err := bookingAmount(0, 0, 60)
	require.Error(t, err)

	// 时长为 0 且只有小时价，同样算不出金额。
	_, err = bookingAmount(19900, 0, 0)
	require.Error(t, err)
}

// TestNormalizeBudgetEnforcesRange 悬赏金额必须落在上下限内。
func TestNormalizeBudgetEnforcesRange(t *testing.T) {
	min, max := dto.BudgetLimits()

	for _, bad := range []int64{0, -1, min - 1, max + 1} {
		_, err := normalizeBudget(bad)
		assert.Errorf(t, err, "悬赏金额 %d 分不该通过校验", bad)
	}
	for _, ok := range []int64{min, max, (min + max) / 2} {
		got, err := normalizeBudget(ok)
		require.NoErrorf(t, err, "悬赏金额 %d 分应通过校验", ok)
		assert.Equal(t, ok, got)
	}
}

// TestYuanTextIsExact 元文本必须精确到分。
func TestYuanTextIsExact(t *testing.T) {
	cases := map[int64]string{
		1: "0.01", 10: "0.10", 100: "1.00", 199: "1.99",
		19900: "199.00", 1029: "10.29", 100000: "1000.00",
		0: "0.00", -199: "-1.99",
	}
	for cents, want := range cases {
		assert.Equalf(t, want, yuanText(cents), "%d 分应格式化为 %s", cents, want)
	}
}
