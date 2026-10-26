// Package payment 抽象支付通道。
//
// 为什么要做成 Provider 接口而不是直接在订单服务里写支付逻辑：
// 支付是唯一"一旦接错就会真的丢钱"的模块。把它挡在一层接口后面，
// 订单服务就只依赖「创建预支付」与「校验回调」两个动作，
// 通道从 mock 换成真实微信支付时，订单状态机一行都不用改。
//
// 当前实现：
//   - mock    ：立即成功，返回可预测的第三方单号。仅供 dev/test。
//   - offline ：线下转账 / 人工确认。Prepay 不下单，等后台标记已支付。
//
// 未实现（保留骨架，**不假装成功**）：
//   - wechat / alipay：真实通道需要商户号、证书与签名验签，见 providers.go 内的说明。
//     在接通之前调用它们会返回 ErrPayChannelUnavailable（501），
//     而不是返回一个"成功"让订单进入服务中——那会造成钱没到账但服务已履约，
//     对账时无法区分是漏单还是漏洞。
package payment

import (
	"context"
	"fmt"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// PrepayResult 是创建预支付的结果。
type PrepayResult struct {
	// ThirdPartyNo 第三方交易号。真实通道是支付平台返回的 prepay_id 或交易流水号。
	ThirdPartyNo string
	// Params 是交给前端唤起支付所需的参数（微信 JSAPI 的 timeStamp/nonceStr/package 等）。
	// 不同通道结构不同，所以用 map 透传，订单服务不解释它的内容。
	Params map[string]string
	// Manual 为 true 表示这个通道不能在线唤起支付，需要用户线下操作
	// （例如 offline 通道要用户先转账，再到订单里等客服确认）。
	Manual bool
	// Instruction 是给用户看的下一步说明，Manual 为 true 时必须有内容。
	Instruction string
}

// CallbackInput 是支付通道回调的原始输入。
type CallbackInput struct {
	// Body 是回调请求体原文。验签必须基于原文，不能基于反序列化后再序列化的结果——
	// 字段顺序与空格都会变，签名对不上。
	Body []byte
	// Headers 是回调请求头，部分通道把签名放在头部。
	Headers map[string]string
}

// CallbackResult 是校验通过后的回调内容。
type CallbackResult struct {
	OrderNo      string
	ThirdPartyNo string
	// Success 为 false 表示这是一条"支付失败"通知，订单不该转已支付。
	Success bool
	// AmountCents 是通道回报的实付金额。订单服务必须与订单金额比对，
	// 不一致说明被改过金额，不能认。
	AmountCents int64
}

// Provider 是支付通道抽象。
type Provider interface {
	// Name 返回通道名，用于启动日志与订单上的 pay_channel 快照。
	Name() string
	// Prepay 创建预支付。
	Prepay(ctx context.Context, req PrepayRequest) (PrepayResult, error)
	// VerifyCallback 校验回调并解析出结果。
	// 校验不通过必须返回 ErrPayCallbackInvalid，绝不能返回一个 Success=true 的结果。
	VerifyCallback(ctx context.Context, in CallbackInput) (CallbackResult, error)
}

// PrepayRequest 是创建预支付所需的订单信息。
type PrepayRequest struct {
	OrderNo     string
	AmountCents int64
	Subject     string
	// NotifyURL 支付结果回调地址。
	NotifyURL string
	// OpenID 微信 JSAPI 支付需要，其他通道可忽略。
	OpenID string
}

// New 按配置装配 Provider。
func New(cfg config.PaymentConfig) (Provider, error) {
	switch cfg.Provider {
	case ChannelMock:
		return NewMock(), nil
	case ChannelOffline:
		return NewOffline(), nil
	case ChannelWechat:
		return NewWechat(), nil
	case ChannelAlipay:
		return NewAlipay(), nil
	default:
		return nil, fmt.Errorf("未知的支付通道: %s", cfg.Provider)
	}
}

// 通道名常量。散落的字符串字面量是拼写错误的温床，尤其是"用户传的通道名"
// 与"provider 的 Name()"必须严格相等这件事——写成常量后编译器会帮忙。
const (
	ChannelMock    = "mock"
	ChannelOffline = "offline"
	ChannelWechat  = "wechat"
	ChannelAlipay  = "alipay"
)

// allChannels 是本服务**认识**的所有通道，也是通道名的唯一真相来源。
//
// 分成两份列表的必要性：
//   - 认识 ≠ 可用。wechat/alipay 的通道名是合法的（请求解析、订单快照、
//     历史数据回读都要认），但当前没有接通，所以不能给用户选。
//   - 如果把"认识"也做成白名单的复制品，那将来接通微信支付时就要同时改
//     三四处的字符串列表，漏掉一处的症状是"用户能选但下单被拒"。
var allChannels = []string{ChannelMock, ChannelOffline, ChannelWechat, ChannelAlipay}

// availableChannels 是已经真正接通、可以给用户选的通道。
//
// 未接通的通道不出现在这个列表里，前端就不会把它渲染成可点的选项；
// 用户看不到，也就不会点了之后再收到一个 501。
var availableChannels = []string{ChannelMock, ChannelOffline}

// AvailableChannels 返回当前可选的通道。
func AvailableChannels() []string {
	out := make([]string, len(availableChannels))
	copy(out, availableChannels)
	return out
}

// IsKnownChannel 判断通道名是否为本服务认识的通道。
//
// 用途是挡掉拼错的通道名。**不能用它判断"能不能用"**——
// 那不归这里管，由 provider 自己回答（未接通会返回 ErrPayChannelUnavailable）。
func IsKnownChannel(name string) bool {
	for _, c := range allChannels {
		if c == name {
			return true
		}
	}
	return false
}

// IsAvailableChannel 判断通道当前是否可选。
func IsAvailableChannel(name string) bool {
	for _, c := range availableChannels {
		if c == name {
			return true
		}
	}
	return false
}

// ChannelLabel 返回通道的中文名，供接口展示。
func ChannelLabel(name string) string { return channelLabel(name) }

// Describe 返回通道说明，用于启动日志。
func Describe(p Provider) string {
	name := p.Name()
	switch name {
	case ChannelMock:
		return "mock（开发环境，下单即支付成功，不产生真实交易）"
	case ChannelOffline:
		return "offline（线下转账，由后台人工确认收款）"
	default:
		return name
	}
}

// unavailable 构造一个"该通道尚未开通"的错误。
func unavailable(name string) error {
	return errcode.ErrPayChannelUnavailable.WithMsg(
		fmt.Sprintf("%s 支付暂未开通，请选择其他支付方式", channelLabel(name)))
}

// fakeCallbackErr 用于"该通道本来就不会有回调"的场景。
//
// 不能因为"这个通道没有回调"就返回 success：那样任何人都能构造一个 POST
// 把自己的订单置为已支付。没有回调的通道，收到回调一律按伪造处理。
func fakeCallbackErr(name string) error {
	return errcode.ErrPayCallbackInvalid.WithMsg(
		fmt.Sprintf("%s 通道不会产生支付回调，该请求来源不可信", channelLabel(name)))
}

func channelLabel(name string) string {
	switch name {
	case ChannelWechat:
		return "微信支付"
	case ChannelAlipay:
		return "支付宝"
	case ChannelOffline:
		return "线下转账"
	case ChannelMock:
		return "模拟支付"
	default:
		return name
	}
}
