package payment

import (
	"context"
	"fmt"
)

// Mock 是开发环境用的通道：预支付立即成功，随后由订单服务直接置为已支付。
//
// 它存在的意义是让订单全链路（下单 → 支付 → 服务 → 确认 → 结算）能在本地
// 零外部依赖地跑完，而不是让人们为了联调先申请一个商户号。
// 生产环境由 config.Validate 硬拦，不允许装配到 mock。
type Mock struct{}

// NewMock 创建 Mock 通道。
func NewMock() *Mock { return &Mock{} }

// Name 返回通道名。
func (m *Mock) Name() string { return "mock" }

// Prepay 直接返回成功。
//
// 第三方单号带 MOCK- 前缀是刻意留的：对账时一眼就能分辨哪些是联调产生的假单，
// 否则测试数据混进真实流水里，日后只能靠时间范围猜。
func (m *Mock) Prepay(_ context.Context, req PrepayRequest) (PrepayResult, error) {
	if req.AmountCents <= 0 {
		return PrepayResult{}, fmt.Errorf("预支付金额必须为正数，当前 %d", req.AmountCents)
	}
	return PrepayResult{
		ThirdPartyNo: "MOCK-" + req.OrderNo,
		Params: map[string]string{
			"channel": "mock",
			"orderNo": req.OrderNo,
		},
		Manual:      false,
		Instruction: "开发环境模拟支付：提交后订单将立即进入服务中",
	}, nil
}

// VerifyCallback 开发环境没有真实回调，任何回调都视为伪造。
//
// 这里不能"为了方便"直接放行：一旦放行，任何人构造一个 POST 就能把
// 自己的订单置为已支付——而且这条路径在开发环境是通的，上线时最容易忘记关。
func (m *Mock) VerifyCallback(_ context.Context, _ CallbackInput) (CallbackResult, error) {
	return CallbackResult{}, fakeCallbackErr(m.Name())
}
