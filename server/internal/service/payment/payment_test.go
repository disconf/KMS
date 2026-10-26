package payment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

func TestMockPrepaySucceeds(t *testing.T) {
	res, err := NewMock().Prepay(context.Background(), PrepayRequest{
		OrderNo:     "OE20261015120000123456",
		AmountCents: 19900,
	})
	require.NoError(t, err)

	// 第三方单号带 MOCK- 前缀，对账时能和真实流水一眼分开。
	assert.Contains(t, res.ThirdPartyNo, "MOCK-")
	assert.Contains(t, res.ThirdPartyNo, "OE20261015120000123456")
	assert.False(t, res.Manual, "mock 是在线通道，不该要求用户线下操作")
}

func TestMockPrepayRejectsNonPositiveAmount(t *testing.T) {
	for _, cents := range []int64{0, -1} {
		_, err := NewMock().Prepay(context.Background(), PrepayRequest{AmountCents: cents})
		assert.Error(t, err, "金额 %d 不该放行", cents)
	}
}

// TestMockRejectsAnyCallback 是安全用例。
//
// mock 通道在本地是通的，如果它顺手"为了方便"把回调放行，
// 那任何人构造一个 POST /payments/callback 就能把自己的订单置为已支付。
// 这条路径在开发环境能跑通、上线时最容易忘记关，所以必须从源头拒绝。
func TestMockRejectsAnyCallback(t *testing.T) {
	_, err := NewMock().VerifyCallback(context.Background(), CallbackInput{
		Body: []byte(`{"orderNo":"OE1","success":true,"amountCents":1}`),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrPayCallbackInvalid)
}

func TestOfflinePrepayIsManual(t *testing.T) {
	res, err := NewOffline().Prepay(context.Background(), PrepayRequest{
		OrderNo:     "OE20261015120000123456",
		AmountCents: 19900,
	})
	require.NoError(t, err)

	// Manual 必须为 true：订单服务据此知道"不能把订单推进到服务中"。
	// 若这里为 false，订单会在用户还没转账时就进入服务中，而钱永远不会到——
	// 这类问题最难查，因为所有状态看起来都是合法的。
	assert.True(t, res.Manual, "线下通道必须标记为 Manual")
	assert.Contains(t, res.Instruction, "OE20261015120000123456", "指引里要带订单号，用户转账备注要用")
	assert.Contains(t, res.Instruction, "199.00", "指引里要给出明确的转账金额")
}

func TestOfflineRejectsAnyCallback(t *testing.T) {
	_, err := NewOffline().VerifyCallback(context.Background(), CallbackInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrPayCallbackInvalid)
}

// TestUnavailableChannelsSaySo 未接通的通道必须明确拒绝。
//
// 这是本包最重要的一条：微信/支付宝没有接通时，
// 如果返回一个"看起来成功"的结果，订单会进入服务中但钱从没到账，
// 对账时无法区分"漏单"与"漏洞"。宁可 501，不可假成功。
func TestUnavailableChannelsSaySo(t *testing.T) {
	for _, p := range []Provider{NewWechat(), NewAlipay()} {
		_, err := p.Prepay(context.Background(), PrepayRequest{OrderNo: "OE1", AmountCents: 100})
		require.Errorf(t, err, "%s 尚未接通，Prepay 不该成功", p.Name())
		assert.ErrorIs(t, err, errcode.ErrPayChannelUnavailable, "应回 501 语义的错误码")

		_, err = p.VerifyCallback(context.Background(), CallbackInput{})
		require.Errorf(t, err, "%s 尚未接通，回调不该被放行", p.Name())
		assert.ErrorIs(t, err, errcode.ErrPayChannelUnavailable)
	}
}

// TestAvailableChannelsExcludesUnimplemented 已接通清单不能混进未实现的通道。
//
// 前端按这个清单渲染支付方式。混进去的话，用户会看到一个能点的"微信支付"，
// 点完拿到 501——比一开始就不显示更让人困惑。
func TestAvailableChannelsExcludesUnimplemented(t *testing.T) {
	channels := AvailableChannels()
	assert.Contains(t, channels, "mock")
	assert.Contains(t, channels, "offline")
	assert.NotContains(t, channels, "wechat")
	assert.NotContains(t, channels, "alipay")

	// 清单里的每个通道都必须真的能装配出来。
	for _, name := range channels {
		p, err := New(config.PaymentConfig{Provider: name})
		require.NoErrorf(t, err, "已接通清单里的 %s 装配失败", name)
		assert.Equal(t, name, p.Name())
	}
}

func TestNewRejectsUnknownProvider(t *testing.T) {
	_, err := New(config.PaymentConfig{Provider: "paypal"})
	assert.Error(t, err)
}

// TestFormatYuanIsExact 金额格式化必须精确到分。
//
// 用 float64 中转的写法（float64(cents)/100 再 %.2f）在某些值上会打印出
// 近似结果。这段文字是给用户看转账金额的，差一位就会被转错钱。
func TestFormatYuanIsExact(t *testing.T) {
	cases := map[int64]string{
		1:      "0.01",
		10:     "0.10",
		100:    "1.00",
		199:    "1.99",
		19900:  "199.00",
		19999:  "199.99",
		100000: "1000.00",
		// 这个值用浮点除会得到 0.29000000000000004 那一类结果
		1029: "10.29",
		0:    "0.00",
		-199: "-1.99",
	}
	for cents, want := range cases {
		assert.Equalf(t, want, formatYuan(cents), "%d 分应格式化为 %s", cents, want)
	}
}
