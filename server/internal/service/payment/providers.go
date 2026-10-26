package payment

import (
	"context"
)

// 本文件保留真实支付通道的骨架。
//
// 为什么留空而不是随便实现一个：微信支付与支付宝的签名算法、证书管理、
// 回调验签都有明确规范，凭印象写一个"看起来对"的版本，风险不是跑不通，
// 而是**在某些输入下验签通过**——那等于把"支付成功"的判断权交给了攻击者。
//
// 所以这里的每个方法都返回明确的"暂未开通"（501），
// 而不是返回一个假成功。接通时需要补的是：
//
//	微信支付（JSAPI / Native）
//	  1. 商户号 mchid、APIv3 密钥、商户私钥与证书序列号
//	  2. Prepay：POST https://api.mch.weixin.qq.com/v3/pay/transactions/jsapi
//	     用商户私钥签 Authorization 头（WECHATPAY2-SHA256-RSA2048）
//	  3. 回调验签：用微信平台证书验 Wechatpay-Signature，并用 AES-GCM 解密 resource
//	  4. 金额字段是「分」，与本地口径一致，不需要换算
//
//	支付宝（手机网站支付 / 当面付）
//	  1. app_id、应用私钥、支付宝公钥
//	  2. Prepay：alipay.trade.page.pay，参数按字典序拼接后 RSA2 签名
//	  3. 回调验签：对去掉 sign / sign_type 的参数按字典序重拼后验签
//	  4. 金额字段是「元」且为字符串，必须按字符解析成分，不能用 float 中转

// Wechat 是微信支付通道（未接通）。
type Wechat struct{}

// NewWechat 创建微信支付通道。
func NewWechat() *Wechat { return &Wechat{} }

// Name 返回通道名。
func (w *Wechat) Name() string { return "wechat" }

// Prepay 尚未接通，返回"暂未开通"。
func (w *Wechat) Prepay(_ context.Context, _ PrepayRequest) (PrepayResult, error) {
	return PrepayResult{}, unavailable(w.Name())
}

// VerifyCallback 尚未接通。回调验签在接通前一律视为不可信。
func (w *Wechat) VerifyCallback(_ context.Context, _ CallbackInput) (CallbackResult, error) {
	return CallbackResult{}, unavailable(w.Name())
}

// Alipay 是支付宝通道（未接通）。
type Alipay struct{}

// NewAlipay 创建支付宝通道。
func NewAlipay() *Alipay { return &Alipay{} }

// Name 返回通道名。
func (a *Alipay) Name() string { return "alipay" }

// Prepay 尚未接通，返回"暂未开通"。
func (a *Alipay) Prepay(_ context.Context, _ PrepayRequest) (PrepayResult, error) {
	return PrepayResult{}, unavailable(a.Name())
}

// VerifyCallback 尚未接通。
func (a *Alipay) VerifyCallback(_ context.Context, _ CallbackInput) (CallbackResult, error) {
	return CallbackResult{}, unavailable(a.Name())
}
