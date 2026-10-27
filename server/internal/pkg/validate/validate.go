// Package validate 提供跨层复用的基础校验函数。
package validate

import "regexp"

var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

// Phone 校验中国大陆手机号。
func Phone(phone string) bool { return phoneRe.MatchString(phone) }

var smsCodeRe = regexp.MustCompile(`^\d{6}$`)

// SMSCode 校验 6 位数字验证码格式。
func SMSCode(code string) bool { return smsCodeRe.MatchString(code) }

// emailRe 不做完整的 RFC 5322 校验，只挡住明显不是邮箱的输入。
//
// 理由：验证码邮件本身就是最强的校验——地址写错就收不到信，
// 收不到就注册不了。正则收得越紧，误伤的正常地址越多
// （比如带 + 号、带中文域名的地址），得不偿失。
var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

// Email 校验邮箱格式，并限制长度上限 128（与库字段一致）。
func Email(email string) bool {
	if len(email) > 128 {
		return false
	}
	return emailRe.MatchString(email)
}

// MaskPhone 手机号脱敏：138****8000。
func MaskPhone(phone string) string {
	if len(phone) < 7 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

// MaskIDCard 身份证号脱敏：只保留前 4 位与后 4 位。
func MaskIDCard(id string) string {
	if len(id) <= 8 {
		return ""
	}
	return id[:4] + "**********" + id[len(id)-4:]
}

// MaskAccountNo 收款账号脱敏：只保留后 4 位。
func MaskAccountNo(no string) string {
	if len(no) <= 4 {
		return "****"
	}
	return "****" + no[len(no)-4:]
}
