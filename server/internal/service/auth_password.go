package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/cache"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/hash"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/validate"
)

// 口令登录的限流参数。
//
// 短信验证码有 5 次失败即作废的机制，口令没有"作废"这个动作，
// 只能靠时间窗兜住爆破：15 分钟内错 5 次就锁死这个手机号的密码登录，
// 期间无论口令对不对都进不去。锁定按手机号计而不是按 IP——
// 换 IP 是攻击者最容易做到的事，成本几乎为零。
const (
	keyPasswordFailures = "auth:pwfail:%s" // auth:pwfail:<phone>
	maxPasswordAttempts = 5
	passwordFailWindow  = 15 * time.Minute
	minPasswordLen      = 8
	maxPasswordLen      = 64
)

// LoginByPassword 手机号 + 口令登录。
//
// 只在账号已设置口令时可用；没有口令的账号（绝大多数普通用户）一律走验证码。
// 所有失败分支返回同一条 ErrPasswordInvalid：手机号不存在、账号无口令、
// 口令错误三种情况在外层看来必须一模一样，否则就成了账号枚举接口。
func (s *AuthService) LoginByPassword(ctx context.Context, req dto.LoginPasswordReq, ip string) (*dto.AuthResp, error) {
	phone := strings.TrimSpace(req.Phone)
	if !validate.Phone(phone) {
		return nil, errcode.ErrPhoneInvalid
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		return nil, errcode.ErrPasswordInvalid
	}

	failKey := fmt.Sprintf(keyPasswordFailures, phone)
	locked, err := s.passwordLocked(ctx, failKey)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, errcode.ErrLoginLocked
	}

	user, err := s.users.FindByPhone(ctx, phone)
	if err != nil {
		if errors.Is(err, errcode.ErrAccountNotFound) {
			return nil, s.passwordFailure(ctx, failKey)
		}
		return nil, err
	}
	if user.PasswordHash == "" || !hash.Verify(user.PasswordHash, password) {
		return nil, s.passwordFailure(ctx, failKey)
	}
	if !user.IsActive() {
		return nil, errcode.ErrUserDisabled
	}

	// 登录成功即清零：否则攒够 5 次历史失败的人，某天会被莫名锁在门外。
	if err := s.cache.Del(ctx, failKey); err != nil {
		logger.Warn("清除密码失败计数失败", "phone", validate.MaskPhone(phone), "err", err.Error())
	}

	resp, err := s.buildAuth(ctx, user, false)
	if err != nil {
		return nil, err
	}
	s.touchLogin(ctx, user.ID, ip)
	return resp, nil
}

// ChangePassword 修改当前账号的登录口令。
//
// 已设置口令的账号必须先给出原口令；从未设置过口令的账号（短信注册的普通
// 用户、以及首次配置的后台账号）允许直接设置——能带着有效令牌调到这里，
// 说明身份已经验证过了，再要一次原口令只会把"首次设置"这条路堵死。
func (s *AuthService) ChangePassword(ctx context.Context, userID int64, req dto.ChangePasswordReq) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.PasswordHash != "" && !hash.Verify(user.PasswordHash, req.OldPassword) {
		return errcode.ErrPasswordInvalid.WithMsg("原密码不正确")
	}
	if user.PasswordHash != "" && req.OldPassword == req.NewPassword {
		return errcode.ErrParamInvalid.WithMsg("新密码不能与原密码相同")
	}
	if err := CheckPasswordStrength(req.NewPassword); err != nil {
		return err
	}

	encoded, err := hash.Hash(req.NewPassword)
	if err != nil {
		return err
	}
	if err := s.users.UpdateFields(ctx, userID, map[string]any{"password_hash": encoded}); err != nil {
		return err
	}
	logger.Info("账号密码已更新", "user_id", userID)
	return nil
}

// CheckPasswordStrength 校验口令强度：长度 8-64，且字母与数字都要有。
//
// 不引入"必须含符号"这类规则：它换来的是用户把 ! 加在末尾然后写在便签上。
// 长度加两类字符是最小可用的底线，够挡住 123456 与纯生日。
func CheckPasswordStrength(password string) error {
	password = strings.TrimSpace(password)
	if len([]rune(password)) < minPasswordLen || len(password) > maxPasswordLen {
		return errcode.ErrPasswordTooWeak
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errcode.ErrPasswordTooWeak
	}
	return nil
}

// passwordLocked 判断该手机号是否已被锁定。
func (s *AuthService) passwordLocked(ctx context.Context, key string) (bool, error) {
	raw, err := s.cache.Get(ctx, key)
	switch {
	case errors.Is(err, cache.ErrNotFound):
		return false, nil
	case err != nil:
		return false, errcode.ErrCache.Wrap(err)
	}
	n, convErr := strconv.Atoi(raw)
	if convErr != nil {
		return false, nil
	}
	return n >= maxPasswordAttempts, nil
}

// passwordFailure 记一次失败，并返回该给调用方的错误。
//
// 刚好踩到第 5 次就立刻告知已锁定：本次仍然算失败，让用户知道"再试也没用"，
// 而不是等第 6 次请求才发现——后者看起来像服务端在消耗他的请求。
// 写缓存失败只记日志不改变返回值：计数丢了顶多少锁一次，
// 为此让用户看到 500 更糟。
func (s *AuthService) passwordFailure(ctx context.Context, key string) error {
	n, err := s.cache.Incr(ctx, key, passwordFailWindow)
	if err != nil {
		logger.Warn("记录密码失败次数失败", "err", err.Error())
		return errcode.ErrPasswordInvalid
	}
	if n >= maxPasswordAttempts {
		return errcode.ErrLoginLocked
	}
	return errcode.ErrPasswordInvalid
}
