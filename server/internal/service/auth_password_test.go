package service

import "testing"

// 口令强度是"能不能设成密码"的唯一闸门，规则松一格就等于允许 Admin@12345
// 这种人人皆知的初始口令继续用下去，所以这里逐条钉住。
func TestCheckPasswordStrength(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"八位字母加数字", "Abcd1234", false},
		{"长口令带符号", "N3w-Strong-Pass", false},
		{"纯字母", "abcdefgh", true},
		{"纯数字", "12345678", true},
		{"七位太短", "Abc1234", true},
		{"空串", "", true},
		{"六位旧默认口令", "Admin1", true},
		{"前后空格视为同一口令", "  Abcd1234  ", false},
	}

	for _, c := range cases {
		err := CheckPasswordStrength(c.password)
		if c.wantErr && err == nil {
			t.Fatalf("%s: 期望拒绝 %q，实际通过", c.name, c.password)
		}
		if !c.wantErr && err != nil {
			t.Fatalf("%s: 期望通过 %q，实际拒绝：%v", c.name, c.password, err)
		}
	}
}
