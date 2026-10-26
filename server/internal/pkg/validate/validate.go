// Package validate 提供跨层复用的基础校验函数。
package validate

import "regexp"

var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

// Phone 校验中国大陆手机号。
func Phone(phone string) bool { return phoneRe.MatchString(phone) }

var smsCodeRe = regexp.MustCompile(`^\d{6}$`)

// SMSCode 校验 6 位数字验证码格式。
func SMSCode(code string) bool { return smsCodeRe.MatchString(code) }

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
