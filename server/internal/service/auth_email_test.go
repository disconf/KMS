package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

const testEmail = "tester@example.com"

// sendMailCode 走真实发送流程（mock 通道）取回验证码。
//
// 先清掉"发送间隔"键：同一邮箱 60 秒内只能发一次是刻意的产品限制，
// 但测试里连续注册/登录会被它挡住，清掉间隔只影响计时不影响验证码本身。
func (f *authFixture) sendMailCode(t *testing.T, email, scene string) string {
	t.Helper()
	require.NoError(t, f.cache.Del(context.Background(), fmt.Sprintf(keyMailGap, strings.ToLower(email))))
	resp, err := f.svc.SendEmailCode(context.Background(), dto.SendEmailCodeReq{Email: email, Scene: scene})
	require.NoError(t, err)
	require.NotEmpty(t, resp.DevCode, "开发环境应回传验证码")
	return resp.DevCode
}

// registerByEmail 完整走一遍注册，返回登录响应。
func (f *authFixture) registerByEmail(t *testing.T, email, password string) *dto.AuthResp {
	t.Helper()
	code := f.sendMailCode(t, email, dto.SceneRegister)
	resp, err := f.svc.RegisterByEmail(context.Background(), dto.RegisterEmailReq{
		Email: email, Code: code, Password: password,
	}, "127.0.0.1")
	require.NoError(t, err)
	return resp
}

// TestRegisterByEmailCreatesReadOnlyUser 注册成功，但账号默认只读。
//
// 这是这个功能最核心的一条契约：公开注册 + 最小权限。
// 注册接口一旦被刷，能发内容的账号就是垃圾内容的入口。
func TestRegisterByEmailCreatesReadOnlyUser(t *testing.T) {
	f := newAuthFixture(t)
	resp := f.registerByEmail(t, testEmail, "MyPass1234")

	require.NotEmpty(t, resp.Token.AccessToken)
	assert.True(t, resp.IsNewUser, "首次注册应标记为新用户")

	user, err := f.svc.users.FindByEmail(context.Background(), testEmail)
	require.NoError(t, err)
	assert.Equal(t, model.RoleUser, user.Role, "自助注册只能是普通用户")
	assert.Equal(t, model.SpeakNone, user.SpeakStatus, "注册后默认只读")
	assert.False(t, user.CanSpeak(), "只读账号不能发言")
	assert.True(t, user.EmailVerified(), "验证码注册即完成邮箱验证")
	assert.NotEmpty(t, user.Nickname, "昵称应回落到邮箱前缀")
}

// TestRegisterByEmailRejectsTakenEmail 同一邮箱不能注册两次。
func TestRegisterByEmailRejectsTakenEmail(t *testing.T) {
	f := newAuthFixture(t)
	f.registerByEmail(t, testEmail, "MyPass1234")

	// 发码阶段就该拦住，而不是让用户白填一遍表单。
	_, err := f.svc.SendEmailCode(context.Background(), dto.SendEmailCodeReq{
		Email: testEmail, Scene: dto.SceneRegister,
	})
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrEmailTaken), "已注册邮箱应返回 ErrEmailTaken，实际 %v", err)
}

// TestRegisterByEmailRejectsWrongCode 验证码错误不能建号。
func TestRegisterByEmailRejectsWrongCode(t *testing.T) {
	f := newAuthFixture(t)
	f.sendMailCode(t, testEmail, dto.SceneRegister)

	_, err := f.svc.RegisterByEmail(context.Background(), dto.RegisterEmailReq{
		Email: testEmail, Code: "000000", Password: "MyPass1234",
	}, "127.0.0.1")
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrEmailCodeInvalid), "应返回验证码错误，实际 %v", err)

	_, findErr := f.svc.users.FindByEmail(context.Background(), testEmail)
	assert.Error(t, findErr, "验证码错误时不应创建账号")
}

// TestRegisterByEmailRejectsWeakPassword 弱口令挡在注册这一步。
func TestRegisterByEmailRejectsWeakPassword(t *testing.T) {
	f := newAuthFixture(t)
	code := f.sendMailCode(t, testEmail, dto.SceneRegister)

	_, err := f.svc.RegisterByEmail(context.Background(), dto.RegisterEmailReq{
		Email: testEmail, Code: code, Password: "12345678",
	}, "127.0.0.1")
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrPasswordTooWeak))
}

// TestLoginByEmailPassword 注册后可用邮箱 + 密码登录。
func TestLoginByEmailPassword(t *testing.T) {
	f := newAuthFixture(t)
	f.registerByEmail(t, testEmail, "MyPass1234")

	resp, err := f.svc.LoginByEmail(context.Background(), dto.LoginEmailReq{
		Email: testEmail, Password: "MyPass1234",
	}, "127.0.0.1")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token.AccessToken)
	assert.Equal(t, model.SpeakNone, resp.User.SpeakStatus)
	assert.False(t, resp.User.CanSpeak)
}

// TestLoginByEmailPasswordCaseInsensitive 邮箱大小写不敏感。
//
// 线上 MySQL 的排序规则本来就不区分大小写，但 SQLite 区分。
// 这条测试守住"代码里统一小写"这个约定，否则会出现本地能登、线上登不上。
func TestLoginByEmailPasswordCaseInsensitive(t *testing.T) {
	f := newAuthFixture(t)
	f.registerByEmail(t, testEmail, "MyPass1234")

	_, err := f.svc.LoginByEmail(context.Background(), dto.LoginEmailReq{
		Email: strings.ToUpper(testEmail), Password: "MyPass1234",
	}, "127.0.0.1")
	require.NoError(t, err)
}

// TestLoginByEmailCode 邮箱 + 验证码登录。
func TestLoginByEmailCode(t *testing.T) {
	f := newAuthFixture(t)
	f.registerByEmail(t, testEmail, "MyPass1234")

	code := f.sendMailCode(t, testEmail, dto.SceneLogin)
	resp, err := f.svc.LoginByEmailCode(context.Background(), dto.LoginEmailCodeReq{
		Email: testEmail, Code: code,
	}, "127.0.0.1")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token.AccessToken)

	// 验证码是一次性的：同一串再用一次必须失败。
	_, err = f.svc.LoginByEmailCode(context.Background(), dto.LoginEmailCodeReq{
		Email: testEmail, Code: code,
	}, "127.0.0.1")
	require.Error(t, err, "验证码不能重复使用")
}

// TestSendEmailCodeRateLimited 同一邮箱 60 秒内不能连发。
func TestSendEmailCodeRateLimited(t *testing.T) {
	f := newAuthFixture(t)
	f.sendMailCode(t, testEmail, dto.SceneRegister)

	_, err := f.svc.SendEmailCode(context.Background(), dto.SendEmailCodeReq{
		Email: testEmail, Scene: dto.SceneRegister,
	})
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrEmailSendTooFast), "应触发发送间隔限制，实际 %v", err)
}

// TestSendEmailCodeNotConfigured 通道没配好时明确报错，而不是假装发出去。
func TestSendEmailCodeNotConfigured(t *testing.T) {
	f := newAuthFixture(t)
	// smtp 但缺少 host/from：Configured() 为 false。
	f.svc.cfg.Mail.Provider = "smtp"
	f.svc.cfg.Mail.SMTP.Host = ""
	f.svc.cfg.Mail.SMTP.From = ""

	_, err := f.svc.SendEmailCode(context.Background(), dto.SendEmailCodeReq{
		Email: "someone@example.com", Scene: dto.SceneRegister,
	})
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrMailNotConfigured), "应返回通道未配置，实际 %v", err)
}

// TestSpeakApplyFlow 只读 → 申请 → 审核通过 → 可发言。
func TestSpeakApplyFlow(t *testing.T) {
	f := newAuthFixture(t)
	f.registerByEmail(t, testEmail, "MyPass1234")

	ctx := context.Background()
	user, err := f.svc.users.FindByEmail(ctx, testEmail)
	require.NoError(t, err)

	speak := NewSpeakService(f.svc.users, repository.NewAuditRepo(f.db))

	// 1) 提交申请
	state, err := speak.Apply(ctx, user.ID, dto.SpeakApplyReq{
		RealName: "张伟", Contact: "13800000000", Note: "想做运维问答",
	})
	require.NoError(t, err)
	assert.Equal(t, model.SpeakPending, state.Status)
	assert.False(t, state.CanSpeak, "待审期间仍然只读")

	// 2) 重复提交要被挡住，否则后台队列会被同一个人刷屏
	_, err = speak.Apply(ctx, user.ID, dto.SpeakApplyReq{
		RealName: "张伟", Contact: "13800000000",
	})
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrSpeakApplyDup))

	// 3) 审核通过
	err = speak.Review(ctx, 1, model.RoleAdmin, user.ID, dto.AdminSpeakReviewReq{Approve: true}, AuditMeta{})
	require.NoError(t, err)

	updated, err := f.svc.users.FindByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, model.SpeakApproved, updated.SpeakStatus)
	assert.True(t, updated.CanSpeak(), "审核通过后应可发言")

	// 4) 同一条申请不能再审一次
	err = speak.Review(ctx, 1, model.RoleAdmin, user.ID, dto.AdminSpeakReviewReq{Approve: false, Note: "重复"}, AuditMeta{})
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrSpeakApplyState))
}

// TestSpeakReviewRequiresReasonOnReject 驳回必须填原因。
func TestSpeakReviewRequiresReasonOnReject(t *testing.T) {
	f := newAuthFixture(t)
	f.registerByEmail(t, testEmail, "MyPass1234")
	user, err := f.svc.users.FindByEmail(context.Background(), testEmail)
	require.NoError(t, err)

	speak := NewSpeakService(f.svc.users, repository.NewAuditRepo(f.db))
	_, err = speak.Apply(context.Background(), user.ID, dto.SpeakApplyReq{
		RealName: "张伟", Contact: "13800000000",
	})
	require.NoError(t, err)

	err = speak.Review(context.Background(), 1, model.RoleAdmin, user.ID,
		dto.AdminSpeakReviewReq{Approve: false}, AuditMeta{})
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrAuditRemarkRequired))
}

// TestReadOnlyUserCannotAsk 只读账号发不了问题。
func TestReadOnlyUserCannotAsk(t *testing.T) {
	f := newAuthFixture(t)
	f.registerByEmail(t, testEmail, "MyPass1234")
	user, err := f.svc.users.FindByEmail(context.Background(), testEmail)
	require.NoError(t, err)

	viewer := QaViewer{UserID: user.ID, Role: user.Role, SpeakStatus: user.SpeakStatus}
	assert.False(t, viewer.CanSpeak())
	assert.False(t, viewer.CanPublishDirectly(), "只读账号也不可能免审直发")

	qaSvc := NewQaService(
		repository.NewQaRepo(f.db),
		repository.NewPointRepo(f.db),
		f.svc.users,
		repository.NewAuditRepo(f.db),
		nil,
	)
	_, err = qaSvc.Ask(context.Background(), viewer, dto.QaAskReq{
		Title: "这个报错怎么排查", Description: "重启之后一直报 connection refused，想确认下排查顺序。",
	})
	require.Error(t, err)
	assert.True(t, errIs(err, errcode.ErrSpeakNotAllowed), "只读账号应被拒绝，实际 %v", err)
}

// errIs 是 errors.Is 的薄封装，便于在断言里同时看原始错误。
func errIs(err, target error) bool { return errors.Is(err, target) }
