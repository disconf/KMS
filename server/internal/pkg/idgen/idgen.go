// Package idgen 生成业务编号（订单号、结算单号等）与随机数。
package idgen

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// OrderNo 生成订单号：OE + yyyyMMddHHmmss + 6 位随机数字。
// 例：OE20261014163548123456
func OrderNo() string { return sequenced("OE") }

// SettlementNo 生成结算单号。
func SettlementNo() string { return sequenced("ST") }

// WithdrawalNo 生成提现单号。
func WithdrawalNo() string { return sequenced("WD") }

const (
	orderNoPrefix = "OE"
	// 2 位前缀 + 14 位时间 + 6 位随机。
	orderNoLen = 2 + 14 + 6
)

// ValidOrderNo 校验字符串是否可能是本服务生成的订单号。
//
// 形状判断放在这里而不是调用方：订单号的格式由本包定义，
// 换个前缀或改随机位长度时，校验规则必须跟着一起改，
// 两处各写一份迟早会漂移。
//
// 只证明"形状对"，不证明"订单存在"——存在性由仓储层回答。
// 它的用途是在 handler 里把 id 当订单号这类明显错误的入参及早挡掉。
func ValidOrderNo(s string) bool {
	return hasSequencedShape(s, orderNoPrefix, orderNoLen)
}

func hasSequencedShape(s, prefix string, total int) bool {
	if len(s) != total || !strings.HasPrefix(s, prefix) {
		return false
	}
	for i := len(prefix); i < total; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func sequenced(prefix string) string {
	return fmt.Sprintf("%s%s%s", prefix, time.Now().Format("20060102150405"), randomDigits(6))
}

// RandomDigits 生成 n 位数字字符串（首位允许为 0，长度固定）。
func randomDigits(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	for i := range buf {
		v, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// crypto/rand 失败属于极端情况，退化为时间戳低位，保证不 panic。
			buf[i] = byte('0' + (time.Now().UnixNano()/int64(i+1))%10)
			continue
		}
		buf[i] = byte('0' + v.Int64())
	}
	return string(buf)
}

// NumericCode 生成定长数字验证码（默认 6 位）。
func NumericCode(n int) string { return randomDigits(n) }
