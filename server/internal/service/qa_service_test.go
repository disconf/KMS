package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

// qaFixture 用临时 SQLite 搭真实的 repository → service 链路。
//
// 与 articleFixture 同构：问答的关键规则（一人一答、采纳 CAS、积分幂等）
// 全部长在 SQL 约束上，mock 掉仓储就等于没测。
type qaFixture struct {
	svc    *QaService
	db     *gorm.DB
	asker  int64 // 提问人
	plain  int64 // 普通用户（新账号，回答要进审核）
	expert int64 // 已过审专家（回答免审）
	admin  int64
}

func newQaFixture(t *testing.T) *qaFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "qa.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	ctx := context.Background()
	users := repository.NewUserRepo(db)
	mk := func(phone, nick, role string) int64 {
		u := &model.User{Phone: model.StrPtr(phone), Nickname: nick, Role: role, Status: model.UserStatusActive}
		// 普通用户默认只读（要实名申请才能发言）。这个夹具里的人物都是"已经在
		// 正常使用的老用户"，所以直接给通过状态；"只读被拦住"那一路由
		// auth_email_test.go 走完整的注册→申请→审核链路覆盖，不在这里重复。
		if role == model.RoleUser {
			u.SpeakStatus = model.SpeakApproved
			u.SpeakRealName = nick
			u.SpeakContact = phone
		}
		require.NoError(t, users.Create(ctx, u))
		return u.ID
	}
	f := &qaFixture{
		db:     db,
		admin:  mk("13811110000", "管理员", model.RoleAdmin),
		asker:  mk("13811110001", "提问人", model.RoleUser),
		plain:  mk("13811110002", "新用户", model.RoleUser),
		expert: mk("13811110003", "专家", model.RoleExpert),
	}
	// 专家必须有已过审档案：免审判定看的是 expert_status，不是角色字符串。
	require.NoError(t, db.Create(&model.Expert{
		UserID: f.expert, RealName: "测试专家",
		Status: model.ExpertStatusApproved,
	}).Error)

	auditRepo := repository.NewAuditRepo(db)
	f.svc = NewQaService(
		repository.NewQaRepo(db), repository.NewPointRepo(db),
		users, auditRepo,
		NewArticleService(repository.NewArticleRepo(db), repository.NewCategoryRepo(db), users, nil),
	)
	return f
}

func (f *qaFixture) ask(t *testing.T, userID int64, title string) *dto.QaQuestionItem {
	t.Helper()
	item, err := f.svc.Ask(context.Background(), viewerOf(model.RoleUser, userID),
		dto.QaAskReq{Title: title, Description: "这是一段足够长的正文，用来通过最短长度校验。"})
	require.NoError(t, err)
	return item
}

func viewerOf(role string, id int64) QaViewer {
	v := QaViewer{UserID: id, Role: role}
	// 与 handler 的填法保持一致：普通用户能不能发言看 speak_status，
	// 这里造的普通用户都已经通过实名申请。
	if role == model.RoleUser {
		v.SpeakStatus = model.SpeakApproved
	}
	return v
}

func TestQaNewUserAnswersNeedReview(t *testing.T) {
	f := newQaFixture(t)
	ctx := context.Background()
	q := f.ask(t, f.asker, "新账号回答是否需要审核")

	// 新账号（0 条已发布回答 < 3）：进 pending，游客不可见，不计入回答数。
	ans, err := f.svc.Answer(ctx, viewerOf(model.RoleUser, f.plain), q.ID,
		dto.QaAnswerReq{Content: "新账号的第一条回答，按规则应该先进审核。"})
	require.NoError(t, err)
	assert.Equal(t, model.QaAnswerPending, ans.Status)

	detail, err := f.svc.Detail(ctx, q.ID, QaViewer{})
	require.NoError(t, err)
	assert.Empty(t, detail.Answers, "待审回答对游客不可见")
	assert.Zero(t, detail.AnswerCount, "待审回答不计入回答数")

	// 作者自己能看到，并带审核中标记。
	own, err := f.svc.Detail(ctx, q.ID, viewerOf(model.RoleUser, f.plain))
	require.NoError(t, err)
	require.Len(t, own.Answers, 1)
	assert.Equal(t, model.QaAnswerPending, own.Answers[0].Status)
	assert.Equal(t, ans.ID, own.MyAnswerID)
}

func TestQaExpertAnswersPublishDirectly(t *testing.T) {
	f := newQaFixture(t)
	ctx := context.Background()
	q := f.ask(t, f.asker, "专家回答是否免审直发")

	ans, err := f.svc.Answer(ctx, QaViewer{UserID: f.expert, Role: model.RoleExpert, ExpertStatus: model.ExpertStatusApproved},
		q.ID, dto.QaAnswerReq{Content: "专家的回答直接发布，不该被审核拖住。"})
	require.NoError(t, err)
	assert.Equal(t, model.QaAnswerPublished, ans.Status)
	assert.True(t, ans.IsExpert)

	// 角色是 expert 但档案没过审：按未过审处理，照样进 pending。
	// 这是「空 expertStatus 可绕过入驻审核」那类漏洞的回答侧版本。
	suspect, err := f.svc.Answer(ctx, QaViewer{UserID: f.admin, Role: model.RoleExpert},
		q.ID, dto.QaAnswerReq{Content: "角色是专家但没过审的回答必须进审核，这段足够长。"})
	require.NoError(t, err)
	assert.Equal(t, model.QaAnswerPending, suspect.Status)
}

func TestQaOneAnswerPerUser(t *testing.T) {
	f := newQaFixture(t)
	ctx := context.Background()
	q := f.ask(t, f.asker, "一个人只能回答一次")

	_, err := f.svc.Answer(ctx, viewerOf(model.RoleUser, f.plain), q.ID,
		dto.QaAnswerReq{Content: "第一条回答，这段长度足够。"})
	require.NoError(t, err)

	_, err = f.svc.Answer(ctx, viewerOf(model.RoleUser, f.plain), q.ID,
		dto.QaAnswerReq{Content: "第二条回答应该被唯一索引挡住。"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcode.ErrQaAlreadyAnswered), "应返回一人一答错误: %v", err)

	// 编辑自己的回答不受唯一索引影响。
	detail, _ := f.svc.Detail(ctx, q.ID, viewerOf(model.RoleUser, f.plain))
	require.NotZero(t, detail.MyAnswerID)
	require.NoError(t, f.svc.UpdateAnswer(ctx, viewerOf(model.RoleUser, f.plain),
		detail.MyAnswerID, dto.QaAnswerReq{Content: "编辑后的回答内容，长度也足够通过校验。"}))
}

func TestQaAcceptRulesAndPoints(t *testing.T) {
	f := newQaFixture(t)
	ctx := context.Background()
	q := f.ask(t, f.asker, "采纳的权限与积分")

	ans, err := f.svc.Answer(ctx, viewerOf(model.RoleUser, f.plain), q.ID,
		dto.QaAnswerReq{Content: "回答内容，等待被采纳。这段长度足够。"})
	require.NoError(t, err)

	// 待审不能采纳。
	err = f.svc.Accept(ctx, viewerOf(model.RoleUser, f.asker), q.ID, ans.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcode.ErrQaAnswerNotPublished))

	// 非提问人不能采纳。
	err = f.svc.Accept(ctx, viewerOf(model.RoleUser, f.plain), q.ID, ans.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcode.ErrQaNotQuestionOwner))

	// 管理员过审。
	_, err = f.svc.PendingAnswers(ctx, dto.PageQuery{})
	require.NoError(t, err)
	require.NoError(t, f.svc.ReviewAnswer(ctx, f.admin, model.RoleAdmin, ans.ID,
		dto.QaReviewReq{Action: "approve"}, AuditMeta{}))

	// 采纳成功：状态、标记、积分一次到位。
	require.NoError(t, f.svc.Accept(ctx, viewerOf(model.RoleUser, f.asker), q.ID, ans.ID))

	detail, err := f.svc.Detail(ctx, q.ID, QaViewer{})
	require.NoError(t, err)
	assert.Equal(t, model.QaStatusResolved, detail.Status)
	assert.Equal(t, ans.ID, detail.AcceptedID)
	require.NotEmpty(t, detail.Answers)
	assert.True(t, detail.Answers[0].IsAccepted, "被采纳的回答置顶")

	points, err := f.svc.MyPoints(ctx, viewerOf(model.RoleUser, f.plain))
	require.NoError(t, err)
	assert.Equal(t, int64(10), points.Points, "采纳 +10")
	require.Len(t, points.Recent, 1)
	assert.Equal(t, model.PointActionAcceptAnswer, points.Recent[0].Action)

	// 重复采纳被 CAS 挡住，积分不翻倍。
	err = f.svc.Accept(ctx, viewerOf(model.RoleUser, f.asker), q.ID, ans.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcode.ErrQaAlreadySettled))

	points2, _ := f.svc.MyPoints(ctx, viewerOf(model.RoleUser, f.plain))
	assert.Equal(t, int64(10), points2.Points, "重复采纳不重复记分")
}

func TestQaLikeIdempotentAndSelfLikeNoPoints(t *testing.T) {
	f := newQaFixture(t)
	ctx := context.Background()
	q := f.ask(t, f.asker, "点赞的幂等与自赞")

	ans, err := f.svc.Answer(ctx, QaViewer{UserID: f.expert, Role: model.RoleExpert, ExpertStatus: model.ExpertStatusApproved},
		q.ID, dto.QaAnswerReq{Content: "专家直发的回答，用来测点赞。长度足够。"})
	require.NoError(t, err)

	_, score, err := f.svc.Like(ctx, viewerOf(model.RoleUser, f.plain), ans.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), score)

	// 同一人重复点赞：幂等，不加第二次分。
	_, score, err = f.svc.Like(ctx, viewerOf(model.RoleUser, f.plain), ans.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), score)

	// 提问人也点一下：第二个人，+1。
	_, score, err = f.svc.Like(ctx, viewerOf(model.RoleUser, f.asker), ans.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), score)

	// 作者自己点赞：计数加，积分不加。
	// 注意积分记在「回答作者」头上，不是点赞人头上——所以专家此时的 2 分
	// 来自前两个人的点赞，自赞不该让它变成 3。
	_, score, err = f.svc.Like(ctx, QaViewer{UserID: f.expert, Role: model.RoleExpert, ExpertStatus: model.ExpertStatusApproved}, ans.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), score)

	authorPoints, err := f.svc.MyPoints(ctx, QaViewer{UserID: f.expert, Role: model.RoleExpert, ExpertStatus: model.ExpertStatusApproved})
	require.NoError(t, err)
	assert.Equal(t, int64(2), authorPoints.Points, "两人点赞各 +1，自赞不额外记分")

	// 点赞人不得分：分只记在回答作者身上。
	likerPoints, err := f.svc.MyPoints(ctx, viewerOf(model.RoleUser, f.plain))
	require.NoError(t, err)
	assert.Zero(t, likerPoints.Points, "点赞人不加分")

	// 取消点赞：计数回落，且取消再点不会重复记分（uniq_key 仍在）。
	_, err = f.svc.Unlike(ctx, viewerOf(model.RoleUser, f.plain), ans.ID)
	require.NoError(t, err)
	_, score, err = f.svc.Like(ctx, viewerOf(model.RoleUser, f.plain), ans.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), score)

	authorPoints2, _ := f.svc.MyPoints(ctx, QaViewer{UserID: f.expert, Role: model.RoleExpert, ExpertStatus: model.ExpertStatusApproved})
	assert.Equal(t, int64(2), authorPoints2.Points, "同一人对同一回答只记一次点赞积分")
}

func TestQaAskValidationAndCooldown(t *testing.T) {
	f := newQaFixture(t)
	ctx := context.Background()
	v := viewerOf(model.RoleUser, f.plain)

	_, err := f.svc.Ask(ctx, v, dto.QaAskReq{Title: "短", Description: "这段正文是够长的。"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcode.ErrQaTitleTooShort))

	_, err = f.svc.Ask(ctx, v, dto.QaAskReq{Title: "标题长度合法的提问", Description: "太短"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcode.ErrQaContentTooShort))

	// 冷却：第一条成功后 10 分钟内的第二条被拒。
	first, err := f.svc.Ask(ctx, v, dto.QaAskReq{Title: "第一条合法长度的提问标题", Description: "正文足够长，能通过校验。"})
	require.NoError(t, err)
	require.NotZero(t, first.ID)

	_, err = f.svc.Ask(ctx, v, dto.QaAskReq{Title: "第二条合法长度的提问标题", Description: "正文同样足够长，能通过第一道校验。"})
	require.Error(t, err, "冷却期内再问应被拒")

	// 直接把上一条的时间拨回 11 分钟前，第二条应放行——测的是窗口本身。
	require.NoError(t, f.db.Model(&model.QaQuestion{}).Where("id = ?", first.ID).
		Update("created_at", time.Now().Add(-11*time.Minute)).Error)
	second, err := f.svc.Ask(ctx, v, dto.QaAskReq{Title: "冷却后的第二条提问标题", Description: "正文足够长即可，通过校验。"})
	require.NoError(t, err)
	require.NotZero(t, second.ID)
}

func TestQaBadgeLevels(t *testing.T) {
	assert.Equal(t, "新手", BadgeOf(0))
	assert.Equal(t, "新手", BadgeOf(9))
	assert.Equal(t, "活跃", BadgeOf(10))
	assert.Equal(t, "贡献者", BadgeOf(50))
	assert.Equal(t, "资深贡献者", BadgeOf(200))

	label, next := nextBadgeOf(15)
	assert.Equal(t, "贡献者", label)
	assert.Equal(t, int64(50), next)

	label, next = nextBadgeOf(300)
	assert.Empty(t, label, "已是最高级")
	assert.Zero(t, next)
}

func TestQaDeleteQuestionCascadesAnswers(t *testing.T) {
	f := newQaFixture(t)
	ctx := context.Background()

	// 用已过审专家回答，保证是「已发布」——孤儿统计只有在已发布回答上才看得出来。
	q := f.ask(t, f.asker, "删帖后回答不该继续计入统计")
	ans, err := f.svc.Answer(ctx, QaViewer{UserID: f.expert, Role: model.RoleExpert, ExpertStatus: model.ExpertStatusApproved}, q.ID,
		dto.QaAnswerReq{Content: "这条回答会随问题一起被清理掉，长度足够通过校验。"})
	require.NoError(t, err)
	require.Equal(t, model.QaAnswerPublished, ans.Status)

	before, err := f.svc.List(ctx, repository.QaFilter{}, dto.PageQuery{Page: 1, PageSize: 20}, QaViewer{})
	require.NoError(t, err)
	require.NotNil(t, before.Stats)
	require.Equal(t, int64(1), before.Stats.Answers)

	require.NoError(t, f.svc.DeleteQuestion(ctx, viewerOf(model.RoleUser, f.asker), q.ID))

	after, err := f.svc.List(ctx, repository.QaFilter{}, dto.PageQuery{Page: 1, PageSize: 20}, QaViewer{})
	require.NoError(t, err)
	require.NotNil(t, after.Stats)
	assert.Zero(t, after.Stats.Answers, "问题删了，它的回答不该还计在全局回答数里")

	// 孤儿回答也不能再出现在待审队列（这里用 pending 那条更直观）。
	q2 := f.ask(t, f.asker, "删帖后待审队列不该留下孤儿")
	pend, err := f.svc.Answer(ctx, viewerOf(model.RoleUser, f.plain), q2.ID,
		dto.QaAnswerReq{Content: "新账号的回答先进待审，问题被删后这条也要一起消失。"})
	require.NoError(t, err)
	require.Equal(t, model.QaAnswerPending, pend.Status)

	pendingBefore, err := f.svc.PendingAnswers(ctx, dto.PageQuery{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), pendingBefore.Total)

	require.NoError(t, f.svc.DeleteQuestion(ctx, viewerOf(model.RoleUser, f.asker), q2.ID))

	pendingAfter, err := f.svc.PendingAnswers(ctx, dto.PageQuery{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Zero(t, pendingAfter.Total, "问题已删，待审队列不该留下点进去 404 的行")

	// 免审判定的门槛计数同样要回落，否则删帖多的用户后面会白得免审资格。
	n, err := repository.NewQaRepo(f.db).CountPublishedByUser(ctx, f.expert)
	require.NoError(t, err)
	assert.Zero(t, n)
}
