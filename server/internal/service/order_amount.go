package service

import (
	"fmt"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// 本文件收敛订单金额的所有算法。
//
// 单独成文件的原因：金额算错是唯一"不会报错、但会真的赔钱"的缺陷。
// 集中在一处，就能对每一种边界（0 分、1 分、除不尽、抽佣 0%）写测试，
// 也能让人一眼看出"专家实收 + 平台佣金 = 订单总额"这条恒等式在哪保证。

// splitCommission 按基点抽佣拆分订单金额。
//
// 返回的 (commission, expert) 满足恒等式 commission + expert == amountCents。
//
// 关键在**不要各算各的**：
//
//	commission = amount * bp / 10000        // 整数除法，向下取整
//	expert     = amount * (10000-bp) / 10000 // 另一个方向各算各的
//
// 这样写两个数各有各的舍入，加起来可能比总额少 1 分或多 1 分，对账时无解。
// 正确做法是只对佣金取整，专家实收用减法得出——无论怎么舍入总额恒等，
// 少的那一分只落在平台侧，不会出现"付出去的钱比收进来的多"。
func splitCommission(amountCents int64, rateBP int) (commission, expert int64) {
	if amountCents <= 0 {
		return 0, 0
	}
	if rateBP <= 0 {
		return 0, amountCents
	}
	if rateBP >= 10000 {
		// 极端配置（100% 抽佣）也允许，但要保证不出现负的实收。
		return amountCents, 0
	}

	commission = amountCents * int64(rateBP) / 10000
	if commission < 0 {
		commission = 0
	}
	if commission > amountCents {
		commission = amountCents
	}
	return commission, amountCents - commission
}

// bookingAmount 按专家定价算出预约订单的金额。
//
// 金额一律由服务端算，**不接受前端传**：接受的话，改一个数字就能用 1 分钱
// 买一小时服务，而且从接口调用上完全看不出异常。
//
// 计价顺序：
//  1. 有预约时长且专家配了小时价 → 按时长折算，向上取整到分；
//  2. 否则专家配了按次价 → 用按次价；
//  3. 两者都没有 → 拒绝下单，而不是拿 0 元成交。
//
// 第 3 条是刻意的：允许 0 元订单等于留了一条"免费获得服务"的通道，
// 而且它看起来完全合法（金额字段是 0，不是负数），只有对账时才发现收入是 0。
func bookingAmount(pricePerHourCents, pricePerTaskCents int64, durationMin int) (int64, error) {
	if durationMin > 0 && pricePerHourCents > 0 {
		// 向上取整到分：(a*b + 分母 - 1) / 分母。
		// 不用截断：截断会让"约 59 分钟"比"约 1 小时"便宜，用户很快会发现
		// 并一直卡在 59 分钟下单，平台与专家都在少收钱。
		return (pricePerHourCents*int64(durationMin) + 59) / 60, nil
	}
	if pricePerTaskCents > 0 {
		return pricePerTaskCents, nil
	}
	return 0, errcode.ErrPriceInvalid.WithMsg("该专家尚未设置服务价格，暂时无法下单")
}

// normalizeBudget 校验并归一悬赏金额。
//
// 上下限不只是防刷：悬赏金额会展示在接单大厅里，
// 一个 0.01 元的悬赏会让整个大厅显得不可信，把认真接单的专家赶走。
func normalizeBudget(cents int64) (int64, error) {
	min, max := dto.BudgetLimits()
	switch {
	case cents <= 0:
		return 0, errcode.ErrBudgetInvalid.WithMsg("悬赏金额必须大于 0")
	case cents < min:
		return 0, errcode.ErrBudgetInvalid.WithMsg("悬赏金额不能低于 " + yuanText(min) + " 元")
	case cents > max:
		return 0, errcode.ErrBudgetInvalid.WithMsg("悬赏金额不能超过 " + yuanText(max) + " 元")
	}
	return cents, nil
}

// yuanText 把「分」转成展示用的「元」文本，固定两位小数。
//
// 用整数除法与取模拼字符串，不走 float64：float64(cents)/100 在 0.1
// 这类无法精确表示的取值上会打印出 xx.099999… 的近似结果，
// 而这段文字会直接出现在错误提示与转账指引里。
func yuanText(cents int64) string {
	if cents < 0 {
		return "-" + yuanText(-cents)
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
