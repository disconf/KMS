package payment

import (
	"context"
	"fmt"
)

// Offline 是线下转账通道：预支付不下单，等用户在订单里等客服确认。
//
// MVP 阶段最实用的通道。它的价值不是"能收款"，而是让整条订单链路
// 在有真实资金往来之前就能跑起来，同时把"未在线支付"这件事显式表达出来——
// 订单停在待支付并显示一段明确的指引，而不是卡在一个转圈中的加载态。
type Offline struct{}

// NewOffline 创建线下通道。
func NewOffline() *Offline { return &Offline{} }

// Name 返回通道名。
func (o *Offline) Name() string { return "offline" }

// Prepay 不做任何在线动作，只回一段操作指引。
//
// Manual=true 是关键：订单服务据此知道"不能把订单推进到服务中"。
// 如果这里也返回 Manual=false，订单会在用户还没转账时就进入服务中，
// 而钱永远不会到——这是最难查的一类问题，因为所有状态都是合法的。
func (o *Offline) Prepay(_ context.Context, req PrepayRequest) (PrepayResult, error) {
	if req.AmountCents <= 0 {
		return PrepayResult{}, fmt.Errorf("预支付金额必须为正数，当前 %d", req.AmountCents)
	}
	return PrepayResult{
		ThirdPartyNo: "",
		Params: map[string]string{
			"channel": "offline",
			"orderNo": req.OrderNo,
		},
		Manual: true,
		Instruction: fmt.Sprintf(
			"请向平台对公账户转账 %s 元，并在转账备注中填写订单号 %s；"+
				"到账后订单会自动进入服务中，也可联系客服加急确认。",
			formatYuan(req.AmountCents), req.OrderNo),
	}, nil
}

// VerifyCallback 线下转账没有回调。
func (o *Offline) VerifyCallback(_ context.Context, _ CallbackInput) (CallbackResult, error) {
	return CallbackResult{}, fakeCallbackErr(o.Name())
}

// formatYuan 把「分」转成展示用的「元」，固定两位小数。
//
// 不用浮点除法：fmt.Sprintf("%.2f", float64(cents)/100) 在
// 0.1 这类无法精确表示的值上会打印出 xx.099999 的近似结果，
// 而这是要显示给用户看的转账金额，错一位就会被转错钱。
func formatYuan(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}
