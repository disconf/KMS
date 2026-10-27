package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/cache"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/hash"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/validate"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/service/mailer"
)

// 邮箱验证码的缓存键与限流参数。
//
// 额度比短信宽松一点（邮件成本低、送达慢，用户重试更频繁），
// 但发送间隔仍是 60 秒：真正防的是脚本，不是手抖。
const (
	keyMailCode     = "mail:code:%s:%s" // mail:code:<scene>:<email>
	keyMailGap      = "mail:gap:%s"     // mail:gap:<email>
	keyMailHourly   = "mail:hourly:%s"  // mail:hourly:<email>
	keyMailDaily    = "mail:daily:%s"   // mail:daily:<email>
	keyMailFailures = "mail:fail:%s:%s" // mail:fail:<scene>:<email>
	keyMailPW       = "auth:emfail:%s"  // auth:emfail:<email> 密码登录失败计数
)

// 邮箱验证码校验的失败上限，与短信一致。
const maxMailCodeAttempts = 5

// SendEmailCode 发送邮箱验证码，带三重限流（发送间隔 / 小时 / 日）。
//
// 和短信的差别：短信通道没接通时生产直接起不来，邮件不是。
// 所以这里先判断通道是否可用，不可用直接给 501，
// 而不是假装发送成功让用户干等一封永远不会来的邮件。
func (s *AuthService) SendEmailCode(ctx context.Context, req dto.SendEmailCodeReq) (*dto.SendSMSCodeResp, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, err
	}
	scene := strings.TrimSpace(req.Scene)
	if scene == "" {
		scene = dto.SceneLogin
	}
	if !dto.ValidMailScene(scene) {
		return nil, errcode.ErrParamInvalid.WithMsg("不支持的验证码场景")
	}
	if !s.mailReady() {
		return nil, errcode.ErrMailNotConfigured
	}

	// 注册场景的注册开关与占用检查放在发码之前：
	// 已经注册过的邮箱再去收一封注册码毫无意义，
	// 而且白消耗一次发信额度。
	if scene == dto.SceneRegister {
		if !s.cfg.RegistrationEnabled() {
			return nil, errcode.ErrRegisterDisabled
		}
		if _, err := s.users.FindByEmail(ctx, email); err == nil {
			return nil, errcode.ErrEmailTaken
		} else if !errors.Is(err, errcode.ErrAccountNotFound) {
			return nil, err
		}
	}

	ttl := s.cfg.Mail.CodeTTL.Std()
	interval := s.cfg.Mail.SendInterval.Std()

	// 1) 发送间隔
	if _, err := s.cache.Get(ctx, fmt.Sprintf(keyMailGap, email)); err == nil {
		return nil, errcode.ErrEmailSendTooFast.WithMsg(
			fmt.Sprintf("验证码已发送，请 %d 秒后再试", int(interval.Seconds())))
	} else if !errors.Is(err, cache.ErrNotFound) {
		return nil, errcode.ErrCache.Wrap(err)
	}

	// 2) 小时限额
	hourCount, err := s.cache.Incr(ctx, fmt.Sprintf(keyMailHourly, email), time.Hour)
	if err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}
	if limit := s.cfg.Mail.HourlyLimit; limit > 0 && hourCount > int64(limit) {
		return nil, errcode.ErrEmailSendTooFast.WithMsg(
			fmt.Sprintf("同一邮箱每小时最多发送 %d 封，请稍后再试", limit))
	}

	// 3) 日限额
	dayCount, err := s.cache.Incr(ctx, fmt.Sprintf(keyMailDaily, email), 24*time.Hour)
	if err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}
	if limit := s.cfg.Mail.DailyLimit; limit > 0 && dayCount > int64(limit) {
		return nil, errcode.ErrEmailDailyLimit
	}

	code, err := randomDigits(smsCodeLength)
	if err != nil {
		return nil, errcode.ErrInternal.Wrap(err)
	}
	if err := s.cache.Set(ctx, fmt.Sprintf(keyMailCode, scene, email), code, ttl); err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}
	// 验证码写成功后才占用发送间隔：先占间隔再写码，写失败会让用户在 60 秒内无法重试。
	if err := s.cache.Set(ctx, fmt.Sprintf(keyMailGap, email), "1", interval); err != nil {
		logger.Warn("写入邮件发送间隔失败", "err", err.Error())
	}

	msg := mailer.VerifyCodeMessage(email, code, scene, ttl)
	if err := s.mail.Send(ctx, msg); err != nil {
		// 发信失败要把刚写的验证码删掉，否则这一分钟的重试会一直撞"已发送"，
		// 而用户手里根本没有可用的码。
		_ = s.cache.Del(ctx, fmt.Sprintf(keyMailCode, scene, email))
		_ = s.cache.Del(ctx, fmt.Sprintf(keyMailGap, email))
		logger.Error("发送邮箱验证码失败", "email", model.MaskEmail(email), "err", err.Error())
		return nil, errcode.From(err)
	}

	resp := &dto.SendSMSCodeResp{
		ExpiresIn: int(ttl.Seconds()),
		Interval:  int(interval.Seconds()),
	}
	if s.cfg.Mail.ExposeCode && !s.cfg.IsProd() {
		resp.DevCode = code
	}
	return resp, nil
}

// RegisterByEmail 邮箱 + 验证码 + 密码注册。
//
// 注册出来的是**只读账号**：role=user、speak_status=none。
// 能浏览、能收藏、能看自己的资料，但任何"发布"动作都会被
// ErrSpeakNotAllowed 挡住，直到实名申请通过。
// 这是刻意的门槛——公开注册的站点，最贵的不是注册量，是清垃圾内容的工。
func (s *AuthService) RegisterByEmail(ctx context.Context, req dto.RegisterEmailReq, ip string) (*dto.AuthResp, error) {
	if !s.cfg.RegistrationEnabled() {
		return nil, errcode.ErrRegisterDisabled
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if err := CheckPasswordStrength(req.Password); err != nil {
		return nil, err
	}
	if err := s.verifyMailCode(ctx, email, req.Code, dto.SceneRegister); err != nil {
		return nil, err
	}

	// 验证码校验通过后才知道"这个邮箱确实归他"，此时再查占用，
	// 避免把"邮箱是否已注册"暴露给没有邮箱访问权的人。
	if _, err := s.users.FindByEmail(ctx, email); err == nil {
		return nil, errcode.ErrEmailTaken
	} else if !errors.Is(err, errcode.ErrAccountNotFound) {
		return nil, err
	}

	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		// 没填昵称就用邮箱前缀，比"运维用户12345"更有辨识度，
		// 也省掉一次"我的昵称怎么是一串数字"的困惑。
		nickname = defaultNicknameFromEmail(email)
	}
	minLen, maxLen := dto.NicknameLen()
	if n := len([]rune(nickname)); n < minLen || n > maxLen {
		return nil, errcode.ErrNicknameInvalid.WithMsg(
			fmt.Sprintf("昵称长度需为 %d-%d 个字符", minLen, maxLen))
	}

	encoded, err := hash.Hash(strings.TrimSpace(req.Password))
	if err != nil {
		return nil, errcode.ErrInternal.Wrap(err)
	}

	now := time.Now()
	addr := email
	user := &model.User{
		Email:           &addr,
		EmailVerifiedAt: &now,
		Nickname:        nickname,
		Role:            model.RoleUser,
		Status:          model.UserStatusActive,
		SpeakStatus:     model.SpeakNone,
		PasswordHash:    encoded,
	}
	if err := s.users.Create(ctx, user); err != nil {
		// 唯一索引是最后一道闸：并发注册同一邮箱时，
		// 上面的 FindByEmail 会双双通过，只有这里能挡住。
		if repository.IsDuplicateKey(err) {
			return nil, errcode.ErrEmailTaken
		}
		return nil, err
	}

	logger.Info("邮箱注册成功", "user_id", user.ID, "email", user.MaskedEmail())

	resp, err := s.buildAuth(ctx, user, true)
	if err != nil {
		return nil, err
	}
	s.touchLogin(ctx, user.ID, ip)
	return resp, nil
}

// LoginByEmail 邮箱 + 密码登录。
//
// 失败分支全部返回同一条 ErrPasswordInvalid，理由与手机号口令登录一致：
// 分开提示等于把"这个邮箱有没有账号"送给撞库脚本。
func (s *AuthService) LoginByEmail(ctx context.Context, req dto.LoginEmailReq, ip string) (*dto.AuthResp, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, errcode.ErrPasswordInvalid
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		return nil, errcode.ErrPasswordInvalid
	}

	failKey := fmt.Sprintf(keyMailPW, email)
	locked, err := s.passwordLocked(ctx, failKey)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, errcode.ErrLoginLocked
	}

	user, err := s.users.FindByEmail(ctx, email)
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
	if !user.EmailVerified() {
		return nil, errcode.ErrEmailNotVerified
	}

	if err := s.cache.Del(ctx, failKey); err != nil {
		logger.Warn("清除邮箱密码失败计数失败", "email", user.MaskedEmail(), "err", err.Error())
	}

	resp, err := s.buildAuth(ctx, user, false)
	if err != nil {
		return nil, err
	}
	s.touchLogin(ctx, user.ID, ip)
	return resp, nil
}

// LoginByEmailCode 邮箱 + 验证码登录。
//
// 这里可以放心地告诉用户"该邮箱尚未注册"：能收到验证码说明
// 邮箱确实归他所有，不存在枚举价值，反而少一次"为什么进不去"的客服。
func (s *AuthService) LoginByEmailCode(ctx context.Context, req dto.LoginEmailCodeReq, ip string) (*dto.AuthResp, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if err := s.verifyMailCode(ctx, email, req.Code, dto.SceneLogin); err != nil {
		return nil, err
	}

	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, errcode.ErrAccountNotFound) {
			return nil, errcode.ErrAccountNotFound.WithMsg("该邮箱尚未注册，请先注册")
		}
		return nil, err
	}
	if !user.IsActive() {
		return nil, errcode.ErrUserDisabled
	}

	// 验证码登录顺带把"已验证"补上：能收到码就是最好的验证，
	// 老数据里 2026-10 之前用邮箱注册但没走验证流程的账号因此能自然转正。
	if !user.EmailVerified() {
		now := time.Now()
		if err := s.users.UpdateFields(ctx, user.ID, map[string]any{"email_verified_at": now}); err != nil {
			logger.Warn("补写邮箱验证时间失败", "user_id", user.ID, "err", err.Error())
		}
		user.EmailVerifiedAt = &now
	}

	resp, err := s.buildAuth(ctx, user, false)
	if err != nil {
		return nil, err
	}
	s.touchLogin(ctx, user.ID, ip)
	return resp, nil
}

// verifyMailCode 校验邮箱验证码，成功后立即作废（一次性使用）。
func (s *AuthService) verifyMailCode(ctx context.Context, email, code, scene string) error {
	code = strings.TrimSpace(code)
	if !validate.SMSCode(code) {
		return errcode.ErrEmailCodeInvalid
	}
	key := fmt.Sprintf(keyMailCode, scene, email)
	failKey := fmt.Sprintf(keyMailFailures, scene, email)

	raw, err := s.cache.Get(ctx, key)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return errcode.ErrSMSCodeExpired.WithMsg("验证码已过期，请重新获取")
		}
		return errcode.ErrCache.Wrap(err)
	}
	if subtle.ConstantTimeCompare([]byte(raw), []byte(code)) == 1 {
		_ = s.cache.Del(ctx, key)
		_ = s.cache.Del(ctx, failKey)
		return nil
	}

	n, incErr := s.cache.Incr(ctx, failKey, codeFailWindow)
	if incErr != nil {
		logger.Warn("记录邮箱验证码失败次数失败", "err", incErr.Error())
		return errcode.ErrEmailCodeInvalid
	}
	if n >= maxMailCodeAttempts {
		// 试错到上限就作废，逼攻击者重新收码（而收码又受发送间隔约束）。
		_ = s.cache.Del(ctx, key)
		return errcode.ErrEmailCodeInvalid.WithMsg("验证码错误次数过多，请重新获取")
	}
	return errcode.ErrEmailCodeInvalid
}

// mailReady 判断邮件通道是否可用。
//
// 生产环境用 mock 一律视为不可用：mock 只把验证码写进日志，
// 线上放行等于"用户永远收不到码，但接口说发送成功"。
func (s *AuthService) mailReady() bool {
	if s.mail == nil || !s.cfg.Mail.Configured() {
		return false
	}
	if s.cfg.IsProd() && s.cfg.Mail.Provider == "mock" {
		return false
	}
	return true
}

// normalizeEmail 归一化并校验邮箱：统一小写去空格。
func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if !validate.Email(email) {
		return "", errcode.ErrEmailInvalid
	}
	return email, nil
}

// defaultNicknameFromEmail 取邮箱前缀作昵称，去掉过短的退化情况。
func defaultNicknameFromEmail(email string) string {
	name := email
	if at := strings.IndexByte(email, '@'); at > 0 {
		name = email[:at]
	}
	// 昵称有 2 字符下限，单字符前缀补成"用户+前缀"而不是直接报错。
	if len([]rune(name)) < 2 {
		name = "用户" + name
	}
	if len([]rune(name)) > 20 {
		name = string([]rune(name)[:20])
	}
	return name
}

// randomDigits 生成指定位数的数字验证码，用 crypto/rand 而非 math/rand：
// 验证码是唯一凭证，可预测的伪随机等于没有。
func randomDigits(n int) (string, error) {
	var b strings.Builder
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteString(strconv.FormatInt(v.Int64(), 10))
	}
	return b.String(), nil
}
