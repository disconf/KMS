package service

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/cache"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/jwtx"
	"github.com/opsexpert/platform/internal/pkg/tokenstore"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/service/mailer"
	"github.com/opsexpert/platform/internal/service/sms"
	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/service/wechat"
)

const (
	testPhone  = "13800001111"
	testPhone2 = "13900002222"
)

type authFixture struct {
	svc    *AuthService
	cfg    *config.Config
	cache  cache.Cache
	jwt    *jwtx.Manager
	tokens *tokenstore.Store
	// db 供需要直接拼仓储的测试用（如发言申请），避免再造一套 fixture。
	db *gorm.DB
}

// newAuthFixture 用临时 SQLite + 内存缓存 + mock 通道搭建一套完整的认证环境。
// 这样测试覆盖的是真实的 repository → service 链路，而不是打桩后的假链路。
func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err, "加载默认配置失败")

	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "test.db")
	cfg.Database.AutoMigrate = true
	// 打开验证码回传，测试才能拿到 devCode 完成登录闭环。
	cfg.SMS.ExposeCode = true
	cfg.SMS.Provider = "mock"
	// 邮件同理：mock 通道 + 回传验证码，邮箱注册链路才可测。
	cfg.Mail.ExposeCode = true
	cfg.Mail.Provider = "mock"
	cfg.Redis.Addr = "" // 强制走内存缓存，测试不依赖外部 Redis

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err, "初始化测试数据库失败")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	memCache := cache.NewMemory()
	t.Cleanup(func() { _ = memCache.Close() })

	jwtMgr := jwtx.NewManager(
		cfg.JWT.Secret, cfg.JWT.Issuer,
		cfg.JWT.AccessTTL.Std(), cfg.JWT.RefreshTTL.Std(),
	)
	tokens := tokenstore.New(memCache)

	smsProvider, err := sms.New(cfg.SMS)
	require.NoError(t, err)
	mailProvider, err := mailer.New(cfg.Mail)
	require.NoError(t, err)

	svc := NewAuthService(
		repository.NewUserRepo(db),
		repository.NewExpertRepo(db),
		memCache, tokens, jwtMgr,
		smsProvider, wechat.New(cfg.Wechat), mailProvider, cfg,
	)

	return &authFixture{svc: svc, cfg: cfg, cache: memCache, jwt: jwtMgr, tokens: tokens, db: db}
}

// sendCode 走真实发送流程并取回验证码。
func (f *authFixture) sendCode(t *testing.T, phone, scene string) string {
	t.Helper()
	resp, err := f.svc.SendCode(context.Background(), dto.SendSMSCodeReq{Phone: phone, Scene: scene})
	require.NoError(t, err)
	require.NotEmpty(t, resp.DevCode, "开发环境应回传验证码")
	assert.Equal(t, int(f.cfg.SMS.CodeTTL.Std().Seconds()), resp.ExpiresIn)
	assert.Equal(t, int(f.cfg.SMS.SendInterval.Std().Seconds()), resp.Interval)
	return resp.DevCode
}

// clearSendGap 清掉发送间隔，便于在同一测试里连续获取验证码。
func (f *authFixture) clearSendGap(t *testing.T, phone string) {
	t.Helper()
	require.NoError(t, f.cache.Del(context.Background(), fmt.Sprintf(keySMSSendGap, phone)))
}

func TestSendCodeRejectsInvalidPhone(t *testing.T) {
	f := newAuthFixture(t)

	cases := []string{"", "12345", "23800001111", "1380000111", "138000011112"}
	for _, phone := range cases {
		_, err := f.svc.SendCode(context.Background(), dto.SendSMSCodeReq{Phone: phone})
		require.Error(t, err, "手机号 %q 应被拒绝", phone)
		assert.ErrorIs(t, err, errcode.ErrPhoneInvalid, "手机号 %q", phone)
	}
}

func TestSendCodeIsRateLimited(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	_, err := f.svc.SendCode(ctx, dto.SendSMSCodeReq{Phone: testPhone})
	require.NoError(t, err)

	// 立即再发一次，应被发送间隔挡住。
	_, err = f.svc.SendCode(ctx, dto.SendSMSCodeReq{Phone: testPhone})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrSMSSendTooFast)
}

func TestSendCodeHitsDailyLimit(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	// 关掉小时限额，否则会先撞上小时上限，测不到日限额这条分支。
	f.cfg.SMS.HourlyLimit = 0

	// 日限额默认 20，逐次清掉 60 秒间隔来触顶。
	for i := 0; i < f.cfg.SMS.DailyLimit; i++ {
		f.clearSendGap(t, testPhone)
		_, err := f.svc.SendCode(ctx, dto.SendSMSCodeReq{Phone: testPhone})
		require.NoError(t, err, "第 %d 次发送不应失败", i+1)
	}

	f.clearSendGap(t, testPhone)
	_, err := f.svc.SendCode(ctx, dto.SendSMSCodeReq{Phone: testPhone})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrSMSDailyLimit)
}

func TestSendCodeHitsHourlyLimit(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	limit := f.cfg.SMS.HourlyLimit
	require.Positive(t, limit)

	for i := 0; i < limit; i++ {
		f.clearSendGap(t, testPhone)
		_, err := f.svc.SendCode(ctx, dto.SendSMSCodeReq{Phone: testPhone})
		require.NoError(t, err, "第 %d 次发送不应失败", i+1)
	}

	f.clearSendGap(t, testPhone)
	_, err := f.svc.SendCode(ctx, dto.SendSMSCodeReq{Phone: testPhone})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTooManyReqs)
}

// wrongCode 返回一个确定不等于真实验证码的 6 位数字，避免测试偶发失败。
func wrongCode(real string) string {
	if real != "000000" {
		return "000000"
	}
	return "111111"
}

func TestLoginBySMSSuccessCreatesUser(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)

	resp, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.NoError(t, err)

	assert.True(t, resp.IsNewUser, "首次登录应标记为新用户")
	assert.Equal(t, model.RoleUser, resp.User.Role)
	assert.Equal(t, "138****1111", resp.User.Phone, "手机号必须脱敏返回")
	assert.NotEmpty(t, resp.Token.AccessToken)
	assert.NotEmpty(t, resp.Token.RefreshToken)
	assert.Equal(t, "Bearer", resp.Token.TokenType)
	assert.Equal(t, int(f.cfg.JWT.AccessTTL.Std().Seconds()), resp.Token.ExpiresIn)

	// 令牌应能被真正解析，并且与用户对上。
	claims, err := f.jwt.Parse(resp.Token.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, resp.User.ID, claims.UserID)
	assert.Equal(t, jwtx.TypeAccess, claims.Type)
}

func TestLoginBySMSSecondTimeIsNotNewUser(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	first, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.NoError(t, err)
	require.True(t, first.IsNewUser)

	f.clearSendGap(t, testPhone)
	code2 := f.sendCode(t, testPhone, dto.SceneLogin)
	second, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code2}, "127.0.0.1")
	require.NoError(t, err)

	assert.False(t, second.IsNewUser, "同一手机号第二次登录不应再建号")
	assert.Equal(t, first.User.ID, second.User.ID, "必须是同一个账号")
}

func TestLoginBySMSWithWrongCode(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	real := f.sendCode(t, testPhone, dto.SceneLogin)

	_, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: wrongCode(real)}, "127.0.0.1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrSMSCodeInvalid)
}

func TestVerifyCodeInvalidatedAfterTooManyAttempts(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	bad := wrongCode(code)

	// 连续错到阈值，验证码应被直接作废，切断枚举路径。
	for i := 0; i < maxCodeAttempts; i++ {
		_, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: bad}, "127.0.0.1")
		require.Error(t, err)
	}

	// 此时即使拿着正确的验证码也无法登录。
	_, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrSMSCodeExpired)
}

func TestVerifyCodeIsSingleUse(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	_, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.NoError(t, err)

	// 同一个验证码不能复用。
	_, err = f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrSMSCodeExpired)
}

func TestRefreshRotatesToken(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	login, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.NoError(t, err)

	refreshed, err := f.svc.Refresh(ctx, login.Token.RefreshToken)
	require.NoError(t, err)
	assert.NotEqual(t, login.Token.AccessToken, refreshed.Token.AccessToken)
	assert.False(t, refreshed.IsNewUser)

	// 旧的刷新令牌必须立即失效，杜绝重放。
	_, err = f.svc.Refresh(ctx, login.Token.RefreshToken)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrRefreshInvalid)
}

func TestRefreshRejectsAccessToken(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	login, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.NoError(t, err)

	// 拿访问令牌去刷新必须被拒绝，否则等于绕过了令牌分层。
	_, err = f.svc.Refresh(ctx, login.Token.AccessToken)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrRefreshInvalid)
}

func TestLogoutRevokesAccessTokenAndRefreshToken(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	login, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.NoError(t, err)

	claims, err := f.jwt.Parse(login.Token.AccessToken)
	require.NoError(t, err)
	require.False(t, f.tokens.IsRevoked(ctx, claims.ID), "登出前不应处于吊销状态")

	identity := authctx.Identity{UserID: claims.UserID, Role: claims.Role, JTI: claims.ID}
	require.NoError(t, f.svc.Logout(ctx, identity, login.Token.RefreshToken))

	assert.True(t, f.tokens.IsRevoked(ctx, claims.ID), "登出后访问令牌应被吊销")

	_, err = f.svc.Refresh(ctx, login.Token.RefreshToken)
	require.Error(t, err, "登出后刷新令牌也必须失效")
	assert.ErrorIs(t, err, errcode.ErrRefreshInvalid)
}

func TestProfileMaskedAndUpdate(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	login, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code, Nickname: "老王"}, "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, "老王", login.User.Nickname, "首次注册应使用传入昵称")

	profile, err := f.svc.Profile(ctx, login.User.ID)
	require.NoError(t, err)
	assert.Equal(t, "老王", profile.Nickname)
	assert.Equal(t, "138****1111", profile.Phone)

	nickname := "  运维老王  "
	email := "laowang@example.com"
	// 邮箱不能在资料里直接改：这条路径没有验证码，能改等于能抢注别人的邮箱。
	// 邮箱的唯一变更入口是注册时的验证（见 auth_email.go）。
	_, err = f.svc.UpdateProfile(ctx, login.User.ID, dto.UpdateProfileReq{
		Nickname: &nickname,
		Email:    &email,
	})
	require.ErrorIs(t, err, errcode.ErrParamInvalid, "资料页改邮箱必须被拒绝")

	updated, err := f.svc.UpdateProfile(ctx, login.User.ID, dto.UpdateProfileReq{Nickname: &nickname})
	require.NoError(t, err)
	assert.Equal(t, "运维老王", updated.Nickname, "昵称首尾空格应被裁剪")
	assert.Equal(t, "", updated.Email, "没验证过的邮箱不该出现在资料里")
	assert.Equal(t, "138****1111", updated.Phone, "未传手机号时不应被改动")
}

func TestUpdateProfileValidatesNickname(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	code := f.sendCode(t, testPhone, dto.SceneLogin)
	login, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone, Code: code}, "127.0.0.1")
	require.NoError(t, err)

	tooShort := "王"
	_, err = f.svc.UpdateProfile(ctx, login.User.ID, dto.UpdateProfileReq{Nickname: &tooShort})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrNicknameInvalid)

	// 中文昵称按字符数计算：20 个汉字应当通过（若按字节数会被误判为超长）。
	exactly20 := "一二三四五六七八九十一二三四五六七八九十"
	assert.Len(t, []rune(exactly20), 20)
	_, err = f.svc.UpdateProfile(ctx, login.User.ID, dto.UpdateProfileReq{Nickname: &exactly20})
	require.NoError(t, err, "20 个汉字的昵称应被接受")

	tooLong := exactly20 + "超"
	_, err = f.svc.UpdateProfile(ctx, login.User.ID, dto.UpdateProfileReq{Nickname: &tooLong})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrNicknameInvalid)
}

func TestLoginByWechatMockCreatesAndReusesAccount(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	const openID = "wx_openid_001"
	code := "mock:" + openID

	first, err := f.svc.LoginByWechat(ctx, dto.LoginWechatReq{Code: code}, "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, first.IsNewUser)
	assert.Equal(t, "微信用户", first.User.Nickname)
	assert.Empty(t, first.User.Phone, "纯微信建号时手机号应为空且不报错")

	// 同一个 openid 再登录，应复用账号而不是新建（这是 NULL 手机号 + 唯一索引的关键验证点）。
	second, err := f.svc.LoginByWechat(ctx, dto.LoginWechatReq{Code: code}, "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, second.IsNewUser)
	assert.Equal(t, first.User.ID, second.User.ID)
}

func TestLoginByWechatBindsPhoneAccount(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	// 先用手机号建号
	code := f.sendCode(t, testPhone2, dto.SceneLogin)
	phoneLogin, err := f.svc.LoginBySMS(ctx, dto.LoginSMSReq{Phone: testPhone2, Code: code}, "127.0.0.1")
	require.NoError(t, err)

	// 再用微信 + 手机号验证码登录，应绑定到同一账号
	f.clearSendGap(t, testPhone2)
	bindCode := f.sendCode(t, testPhone2, dto.SceneBind)
	wechatLogin, err := f.svc.LoginByWechat(ctx, dto.LoginWechatReq{
		Code:    "mock:wx_openid_002",
		Phone:   testPhone2,
		SmsCode: bindCode,
	}, "127.0.0.1")
	require.NoError(t, err)

	assert.Equal(t, phoneLogin.User.ID, wechatLogin.User.ID, "微信应绑定到已有手机号账号，而不是新建")
	assert.False(t, wechatLogin.IsNewUser)
}

func TestLoginByWechatRejectsBadCodeInMockMode(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	_, err := f.svc.LoginByWechat(ctx, dto.LoginWechatReq{Code: "not-a-mock-code"}, "127.0.0.1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrWechatAuthFailed)
}

func TestWechatAuthURLRegistersState(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	resp, err := f.svc.WechatAuthURL(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.State)
	assert.Contains(t, resp.URL, resp.State)

	// state 应已落缓存，供回调校验。
	_, err = f.cache.Get(ctx, fmt.Sprintf(keyWechatState, resp.State))
	require.NoError(t, err)
}

func TestLoginByWechatRejectsExpiredState(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()

	_, err := f.svc.LoginByWechat(ctx, dto.LoginWechatReq{
		Code:  "mock:wx_openid_003",
		State: "never-issued-state",
	}, "127.0.0.1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrWechatAuthFailed)
}
