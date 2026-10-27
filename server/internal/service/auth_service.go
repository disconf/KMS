// Package service 承载业务逻辑：编排 repository、缓存与第三方通道。
// handler 只做参数绑定与响应输出，不写业务分支。
package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/cache"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/jwtx"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/tokenstore"
	"github.com/opsexpert/platform/internal/pkg/validate"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/service/mailer"
	"github.com/opsexpert/platform/internal/service/sms"
	"github.com/opsexpert/platform/internal/service/wechat"
)

const (
	// smsCodeLength 验证码位数。
	smsCodeLength = 6
	// maxCodeAttempts 单个验证码允许的最大校验失败次数，超过即作废，防暴力枚举。
	maxCodeAttempts = 5
	// codeFailWindow 失败次数统计窗口。
	codeFailWindow = 10 * time.Minute
	// wechatStateTTL 微信授权 state 有效期。
	wechatStateTTL = 5 * time.Minute
	// defaultNicknamePrefix 未填昵称时的默认昵称前缀。
	defaultNicknamePrefix = "运维用户"
)

// 缓存键。
const (
	keySMSCode     = "sms:code:%s:%s"  // sms:code:<scene>:<phone>
	keySMSSendGap  = "sms:gap:%s"      // sms:gap:<phone>
	keySMSHourly   = "sms:hourly:%s"   // sms:hourly:<phone>
	keySMSDaily    = "sms:daily:%s"    // sms:daily:<phone>
	keySMSFailures = "sms:fail:%s:%s"  // sms:fail:<scene>:<phone>
	keyWechatState = "auth:wxstate:%s" // auth:wxstate:<state>
)

// AuthService 处理注册登录、令牌与个人资料。
type AuthService struct {
	users   *repository.UserRepo
	experts *repository.ExpertRepo

	cache  cache.Cache
	tokens *tokenstore.Store
	jwt    *jwtx.Manager

	sms    sms.Provider
	wechat wechat.Provider
	mail   mailer.Provider

	cfg *config.Config
}

// NewAuthService 创建 AuthService。
func NewAuthService(
	users *repository.UserRepo,
	experts *repository.ExpertRepo,
	c cache.Cache,
	tokens *tokenstore.Store,
	jwtMgr *jwtx.Manager,
	smsProvider sms.Provider,
	wechatProvider wechat.Provider,
	mailProvider mailer.Provider,
	cfg *config.Config,
) *AuthService {
	return &AuthService{
		users:   users,
		experts: experts,
		cache:   c,
		tokens:  tokens,
		jwt:     jwtMgr,
		sms:     smsProvider,
		wechat:  wechatProvider,
		mail:    mailProvider,
		cfg:     cfg,
	}
}

// ============================ 短信验证码 ============================

// SendCode 发送登录验证码，带三重限流（发送间隔 / 小时 / 日）。
func (s *AuthService) SendCode(ctx context.Context, req dto.SendSMSCodeReq) (*dto.SendSMSCodeResp, error) {
	phone := strings.TrimSpace(req.Phone)
	if !validate.Phone(phone) {
		return nil, errcode.ErrPhoneInvalid
	}
	scene := req.Scene
	if scene == "" {
		scene = dto.SceneLogin
	}

	ttl := s.cfg.SMS.CodeTTL.Std()
	interval := s.cfg.SMS.SendInterval.Std()

	// 1) 发送间隔：同一手机号 60 秒内只允许一次
	if _, err := s.cache.Get(ctx, fmt.Sprintf(keySMSSendGap, phone)); err == nil {
		return nil, errcode.ErrSMSSendTooFast.WithMsg(
			fmt.Sprintf("验证码已发送，请 %d 秒后再试", int(interval.Seconds())))
	} else if !errors.Is(err, cache.ErrNotFound) {
		return nil, errcode.ErrCache.Wrap(err)
	}

	// 2) 小时限额
	hourCount, err := s.cache.Incr(ctx, fmt.Sprintf(keySMSHourly, phone), time.Hour)
	if err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}
	if s.cfg.SMS.HourlyLimit > 0 && hourCount > int64(s.cfg.SMS.HourlyLimit) {
		return nil, errcode.ErrTooManyReqs.WithMsg("该手机号 1 小时内验证码发送次数已达上限")
	}

	// 3) 日限额
	dayCount, err := s.cache.Incr(ctx, fmt.Sprintf(keySMSDaily, phone), 24*time.Hour)
	if err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}
	if s.cfg.SMS.DailyLimit > 0 && dayCount > int64(s.cfg.SMS.DailyLimit) {
		return nil, errcode.ErrSMSDailyLimit
	}

	code := idgen.NumericCode(smsCodeLength)
	if err := s.cache.Set(ctx, fmt.Sprintf(keySMSCode, scene, phone), code, ttl); err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}
	if err := s.cache.Set(ctx, fmt.Sprintf(keySMSSendGap, phone), "1", interval); err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}

	if err := s.sms.Send(sms.WithScene(ctx, scene), phone, code, ttl); err != nil {
		// 投递失败必须回滚，否则用户既收不到短信，又被间隔限流锁死 60 秒。
		_ = s.cache.Del(ctx, fmt.Sprintf(keySMSCode, scene, phone), fmt.Sprintf(keySMSSendGap, phone))
		logger.Warn("验证码投递失败", "phone", validate.MaskPhone(phone), "err", err.Error())
		return nil, err
	}

	resp := &dto.SendSMSCodeResp{
		ExpiresIn: int(ttl.Seconds()),
		Interval:  int(interval.Seconds()),
	}
	if s.cfg.SMS.ExposeCode {
		resp.DevCode = code
	}
	return resp, nil
}

// verifyCode 校验验证码，成功后立即作废（一次性使用）。
func (s *AuthService) verifyCode(ctx context.Context, phone, code, scene string) error {
	if !validate.SMSCode(code) {
		return errcode.ErrSMSCodeInvalid
	}

	codeKey := fmt.Sprintf(keySMSCode, scene, phone)
	failKey := fmt.Sprintf(keySMSFailures, scene, phone)

	cached, err := s.cache.Get(ctx, codeKey)
	switch {
	case errors.Is(err, cache.ErrNotFound):
		return errcode.ErrSMSCodeExpired
	case err != nil:
		return errcode.ErrCache.Wrap(err)
	}

	if subtle.ConstantTimeCompare([]byte(cached), []byte(code)) != 1 {
		fails, incrErr := s.cache.Incr(ctx, failKey, codeFailWindow)
		if incrErr != nil {
			return errcode.ErrCache.Wrap(incrErr)
		}
		if fails >= maxCodeAttempts {
			// 失败次数超限：直接废掉该验证码，切断枚举路径。
			_ = s.cache.Del(ctx, codeKey)
			return errcode.ErrSMSCodeInvalid.WithMsg("验证码错误次数过多，请重新获取")
		}
		return errcode.ErrSMSCodeInvalid
	}

	_ = s.cache.Del(ctx, codeKey, failKey)
	return nil
}

// ============================ 登录 ============================

// LoginBySMS 手机号 + 验证码登录，账号不存在时自动注册。
func (s *AuthService) LoginBySMS(ctx context.Context, req dto.LoginSMSReq, ip string) (*dto.AuthResp, error) {
	phone := strings.TrimSpace(req.Phone)
	if !validate.Phone(phone) {
		return nil, errcode.ErrPhoneInvalid
	}
	if err := s.verifyCode(ctx, phone, strings.TrimSpace(req.Code), dto.SceneLogin); err != nil {
		return nil, err
	}

	user, err := s.users.FindByPhone(ctx, phone)
	isNew := false
	switch {
	case err == nil:
		// 已有账号
	case errors.Is(err, errcode.ErrAccountNotFound):
		user, err = s.createUserByPhone(ctx, phone, req.Nickname)
		if err != nil {
			return nil, err
		}
		isNew = true
	default:
		return nil, err
	}

	if !user.IsActive() {
		return nil, errcode.ErrUserDisabled
	}

	resp, err := s.buildAuth(ctx, user, isNew)
	if err != nil {
		return nil, err
	}
	s.touchLogin(ctx, user.ID, ip)
	return resp, nil
}

// WechatAuthURL 生成微信授权地址，并落一份 state 用于回调校验。
func (s *AuthService) WechatAuthURL(ctx context.Context) (*dto.WechatAuthURLResp, error) {
	state := uuid.NewString()
	if err := s.cache.Set(ctx, fmt.Sprintf(keyWechatState, state), "1", wechatStateTTL); err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}
	return &dto.WechatAuthURLResp{URL: s.wechat.AuthorizeURL(state), State: state}, nil
}

// LoginByWechat 微信登录。
//
// 账号匹配优先级：openid 已绑定 → 用该账号；否则若同时带了手机号与验证码 → 绑定到手机号账号；
// 否则以 openid 新建账号（手机号留空，后续可在个人中心补绑）。
func (s *AuthService) LoginByWechat(ctx context.Context, req dto.LoginWechatReq, ip string) (*dto.AuthResp, error) {
	// 非 mock 模式下必须校验 state，防 CSRF。
	if req.State != "" {
		stateKey := fmt.Sprintf(keyWechatState, req.State)
		if _, err := s.cache.Get(ctx, stateKey); err != nil {
			return nil, errcode.ErrWechatAuthFailed.WithMsg("授权状态已失效，请重新发起微信登录")
		}
		_ = s.cache.Del(ctx, stateKey)
	}

	info, err := s.wechat.Exchange(ctx, strings.TrimSpace(req.Code))
	if err != nil {
		return nil, err
	}

	user, err := s.users.FindByOpenID(ctx, info.OpenID)
	isNew := false

	switch {
	case err == nil:
		// 已绑定，直接登录
	case errors.Is(err, errcode.ErrAccountNotFound):
		user, isNew, err = s.bindOrCreateWechatUser(ctx, info, req)
		if err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	if !user.IsActive() {
		return nil, errcode.ErrUserDisabled
	}

	resp, err := s.buildAuth(ctx, user, isNew)
	if err != nil {
		return nil, err
	}
	s.touchLogin(ctx, user.ID, ip)
	return resp, nil
}

func (s *AuthService) bindOrCreateWechatUser(ctx context.Context, info *wechat.UserInfo, req dto.LoginWechatReq) (*model.User, bool, error) {
	phone := strings.TrimSpace(req.Phone)

	// 分支一：带了手机号 + 验证码，说明用户想把微信绑定到已有/新建的手机号账号。
	if phone != "" {
		if !validate.Phone(phone) {
			return nil, false, errcode.ErrPhoneInvalid
		}
		if err := s.verifyCode(ctx, phone, strings.TrimSpace(req.SmsCode), dto.SceneBind); err != nil {
			return nil, false, err
		}

		existing, err := s.users.FindByPhone(ctx, phone)
		if err == nil {
			if existing.WxOpenID != "" && existing.WxOpenID != info.OpenID {
				return nil, false, errcode.ErrWechatAuthFailed.WithMsg("该手机号已绑定其他微信")
			}
			if err := s.users.BindWechat(ctx, existing.ID, info.OpenID, info.UnionID); err != nil {
				return nil, false, err
			}
			existing.WxOpenID = info.OpenID
			return existing, false, nil
		}
		if !errors.Is(err, errcode.ErrAccountNotFound) {
			return nil, false, err
		}

		created, err := s.createUserByPhone(ctx, phone, req.Nickname)
		if err != nil {
			return nil, false, err
		}
		if err := s.users.BindWechat(ctx, created.ID, info.OpenID, info.UnionID); err != nil {
			return nil, false, err
		}
		created.WxOpenID = info.OpenID
		return created, true, nil
	}

	// 分支二：纯微信建号，手机号留空。
	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		nickname = info.Nickname
	}
	if nickname == "" {
		nickname = "微信用户"
	}

	created := &model.User{
		WxOpenID:  info.OpenID,
		WxUnionID: info.UnionID,
		Nickname:  nickname,
		Avatar:    firstNonEmpty(req.Avatar, info.Avatar),
		Role:      model.RoleUser,
		Status:    model.UserStatusActive,
	}
	if err := s.users.Create(ctx, created); err != nil {
		return nil, false, err
	}
	return created, true, nil
}

func (s *AuthService) createUserByPhone(ctx context.Context, phone, nickname string) (*model.User, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		nickname = defaultNicknamePrefix + lastN(phone, 4)
	}

	user := &model.User{
		Phone:    model.StrPtr(phone),
		Nickname: nickname,
		Role:     model.RoleUser,
		Status:   model.UserStatusActive,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// ============================ 令牌 ============================

// Refresh 用刷新令牌换取新的令牌组，并轮换（旧的立即失效）以防重放。
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*dto.AuthResp, error) {
	claims, err := s.jwt.Parse(strings.TrimSpace(refreshToken))
	if err != nil || claims.Type != jwtx.TypeRefresh {
		return nil, errcode.ErrRefreshInvalid
	}

	owner, err := s.tokens.RefreshOwner(ctx, claims.ID)
	if err != nil || owner != claims.UserID {
		// 缓存中查不到说明已被登出或过期，一律按失效处理。
		return nil, errcode.ErrRefreshInvalid
	}

	user, err := s.users.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, errcode.ErrRefreshInvalid
	}
	if !user.IsActive() {
		return nil, errcode.ErrUserDisabled
	}

	_ = s.tokens.DeleteRefresh(ctx, claims.ID)
	return s.buildAuth(ctx, user, false)
}

// Logout 登出：吊销当前访问令牌，并失效传入的刷新令牌。
//
// 访问令牌的吊销标记 TTL 取 access_ttl（而非精确剩余时间），
// 代价是吊销名单多保留最多一个 access_ttl，换来的是无需解析 exp 的简单实现。
func (s *AuthService) Logout(ctx context.Context, identity authctx.Identity, refreshToken string) error {
	if identity.JTI != "" {
		if err := s.tokens.Revoke(ctx, identity.JTI, s.jwt.AccessTTL()); err != nil {
			return errcode.ErrCache.Wrap(err)
		}
	}
	if refreshToken != "" {
		if claims, err := s.jwt.Parse(strings.TrimSpace(refreshToken)); err == nil && claims.Type == jwtx.TypeRefresh {
			_ = s.tokens.DeleteRefresh(ctx, claims.ID)
		}
	}
	return nil
}

func (s *AuthService) buildAuth(ctx context.Context, user *model.User, isNew bool) (*dto.AuthResp, error) {
	expertStatus := ""
	if user.Role == model.RoleExpert {
		expertStatus = s.experts.StatusOf(ctx, user.ID)
	}

	access, err := s.jwt.SignAccess(user.ID, user.Role, expertStatus)
	if err != nil {
		return nil, err
	}
	refresh, err := s.jwt.SignRefresh(user.ID, user.Role, expertStatus)
	if err != nil {
		return nil, err
	}
	if err := s.tokens.SaveRefresh(ctx, refresh.JTI, user.ID, s.jwt.RefreshTTL()); err != nil {
		return nil, errcode.ErrCache.Wrap(err)
	}

	return &dto.AuthResp{
		Token: dto.TokenPair{
			AccessToken:      access.Value,
			TokenType:        "Bearer",
			ExpiresIn:        int(s.jwt.AccessTTL().Seconds()),
			RefreshToken:     refresh.Value,
			RefreshExpiresIn: int(s.jwt.RefreshTTL().Seconds()),
		},
		User:      ToUserProfile(user, expertStatus),
		IsNewUser: isNew,
	}, nil
}

// touchLogin 记录登录信息。失败只记日志，不影响登录结果。
func (s *AuthService) touchLogin(ctx context.Context, userID int64, ip string) {
	if err := s.users.TouchLogin(ctx, userID, ip); err != nil {
		logger.Warn("记录登录信息失败", "user_id", userID, "err", err.Error())
	}
}

// ============================ 个人资料 ============================

// Profile 返回个人资料。
func (s *AuthService) Profile(ctx context.Context, userID int64) (*dto.UserProfile, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	expertStatus := ""
	if user.Role == model.RoleExpert {
		expertStatus = s.experts.StatusOf(ctx, user.ID)
	}
	profile := ToUserProfile(user, expertStatus)
	return &profile, nil
}

// UpdateProfile 局部更新个人资料，未传字段保持不变。
func (s *AuthService) UpdateProfile(ctx context.Context, userID int64, req dto.UpdateProfileReq) (*dto.UserProfile, error) {
	fields := make(map[string]any, 3)

	if req.Nickname != nil {
		nickname := strings.TrimSpace(*req.Nickname)
		minLen, maxLen := dto.NicknameLen()
		// 按「字符数」而非字节数判断，避免中文昵称被误判为超长。
		if n := len([]rune(nickname)); n < minLen || n > maxLen {
			return nil, errcode.ErrNicknameInvalid.WithMsg(
				fmt.Sprintf("昵称长度需为 %d-%d 个字符", minLen, maxLen))
		}
		fields["nickname"] = nickname
	}

	if req.Avatar != nil {
		avatar := strings.TrimSpace(*req.Avatar)
		if len(avatar) > dto.AvatarMaxLen() {
			return nil, errcode.ErrParamInvalid.WithMsg("头像地址过长")
		}
		fields["avatar"] = avatar
	}

	if req.Email != nil {
		// 邮箱是登录主凭据，不能在这里直接改。
		//
		// 放开它等于留了一个"把账号改到别人邮箱"或"改成未验证邮箱绕过校验"的口子。
		// 换绑邮箱应该走"新邮箱收码 → 校验通过 → 写入"的独立流程，
		// 在那条流程做出来之前，这里只受理昵称与头像。
		return nil, errcode.ErrParamInvalid.WithMsg("邮箱暂不支持在个人资料里修改")
	}

	if len(fields) == 0 {
		return s.Profile(ctx, userID)
	}

	if err := s.users.UpdateFields(ctx, userID, fields); err != nil {
		return nil, err
	}
	return s.Profile(ctx, userID)
}

// ============================ 工具 ============================

// ToUserProfile 把模型转换为对外 DTO，敏感字段在此统一脱敏。
func ToUserProfile(user *model.User, expertStatus string) dto.UserProfile {
	return dto.UserProfile{
		ID:              user.ID,
		Phone:           validate.MaskPhone(user.PhoneValue()),
		Nickname:        user.Nickname,
		Avatar:          user.Avatar,
		Email:           user.EmailValue(),
		Role:            user.Role,
		ExpertStatus:    expertStatus,
		CreatedAt:       user.CreatedAt,
		SpeakStatus:     speakStatusOrDefault(user.SpeakStatus),
		SpeakReviewNote: user.SpeakReviewNote,
		SpeakAppliedAt:  user.SpeakAppliedAt,
		CanSpeak:        user.CanSpeak(),
	}
}

// speakStatusOrDefault 老数据里 speak_status 可能是空串（列刚加上时），
// 对外一律呈现 none，避免前端拿到空值后渲染出"未知状态"。
func speakStatusOrDefault(s string) string {
	if s == "" {
		return model.SpeakNone
	}
	return s
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
