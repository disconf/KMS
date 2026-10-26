package service

import (
	"context"
	"path/filepath"
	"testing"

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

// ============================ 纯函数：判分与规范化 ============================

func TestJudgeAnswerIsSetEquality(t *testing.T) {
	multi := &model.QuizQuestion{
		Type:    model.QuizTypeMultiple,
		Options: model.QuizOptionList{{Key: "A"}, {Key: "B"}, {Key: "C"}},
		Correct: model.StringList{"A", "B"},
	}

	assert.True(t, judgeAnswer(multi, model.StringList{"A", "B"}))
	assert.True(t, judgeAnswer(multi, model.StringList{"B", "A"}), "顺序不同不影响判分")

	// 漏选不给部分分：运维场景里的多选往往就是操作规程，漏一步就是事故。
	assert.False(t, judgeAnswer(multi, model.StringList{"A"}), "漏选不算对")
	assert.False(t, judgeAnswer(multi, model.StringList{"A", "B", "C"}), "多选不算对")
	assert.False(t, judgeAnswer(multi, model.StringList{"A", "C"}))

	single := &model.QuizQuestion{
		Type:    model.QuizTypeSingle,
		Options: model.QuizOptionList{{Key: "A"}, {Key: "B"}},
		Correct: model.StringList{"A"},
	}
	assert.True(t, judgeAnswer(single, model.StringList{"A"}))
	assert.False(t, judgeAnswer(single, model.StringList{"B"}))
	assert.False(t, judgeAnswer(single, nil))
}

func TestNormalizeChosenRejectsUnknownAndMultiOnSingle(t *testing.T) {
	q := &model.QuizQuestion{
		Type:    model.QuizTypeSingle,
		Options: model.QuizOptionList{{Key: "A"}, {Key: "B"}, {Key: "C"}},
		Correct: model.StringList{"A"},
	}

	out, err := normalizeChosen([]string{" b "}, q)
	require.NoError(t, err)
	assert.Equal(t, []string{"B"}, []string(out), "小写与空白都被规整")

	_, err = normalizeChosen([]string{"Z"}, q)
	assert.ErrorIs(t, err, errcode.ErrQuizAnswerInvalid, "不存在的选项必须拒绝")

	_, err = normalizeChosen([]string{"A", "B"}, q)
	assert.ErrorIs(t, err, errcode.ErrQuizAnswerInvalid, "单选题传两个答案必须拒绝")

	_, err = normalizeChosen(nil, q)
	assert.ErrorIs(t, err, errcode.ErrQuizAnswerInvalid)

	_, err = normalizeChosen([]string{"-"}, q)
	assert.ErrorIs(t, err, errcode.ErrQuizAnswerInvalid, "规整后为空的答案应视为未作答")

	// 多选题可以传多个，且会去重排序
	mq := &model.QuizQuestion{
		Type:    model.QuizTypeMultiple,
		Options: model.QuizOptionList{{Key: "A"}, {Key: "B"}, {Key: "C"}},
	}
	out, err = normalizeChosen([]string{"C", "A", "a"}, mq)
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "C"}, []string(out))
}

func TestDedupeSubmitItemsKeepsFirstOccurrence(t *testing.T) {
	out, err := dedupeSubmitItems([]dto.QuizSubmitItem{
		{QuestionID: 3, Chosen: []string{"A"}},
		{QuestionID: 1, Chosen: []string{"B"}},
		{QuestionID: 3, Chosen: []string{"C"}},
	})
	require.NoError(t, err)
	require.Len(t, out, 2, "同一题的重复项应去掉")
	// 保持提交顺序，且保留的是**首次**作答：若保最后一次，前端重复渲染一遍
	// 就能把答案改掉，判分结果不再可信。
	assert.Equal(t, int64(3), out[0].QuestionID)
	assert.Equal(t, []string{"A"}, out[0].Chosen)
	assert.Equal(t, int64(1), out[1].QuestionID)
	assert.Equal(t, []string{"B"}, out[1].Chosen)

	_, err = dedupeSubmitItems([]dto.QuizSubmitItem{{QuestionID: 0, Chosen: []string{"A"}}})
	assert.ErrorIs(t, err, errcode.ErrQuizAnswerInvalid)
}

func TestNormalizeQuizTypeAndDifficulty(t *testing.T) {
	tp, err := normalizeQuizType("")
	require.NoError(t, err)
	assert.Equal(t, model.QuizTypeSingle, tp, "省略题型按单选处理")

	tp, err = normalizeQuizType(" MULTIPLE ")
	require.NoError(t, err)
	assert.Equal(t, model.QuizTypeMultiple, tp)

	_, err = normalizeQuizType("essay")
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)

	d, err := normalizeDifficulty("")
	require.NoError(t, err)
	assert.Equal(t, model.QuizDifficultyMedium, d)

	_, err = normalizeDifficulty("very-hard")
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)
}

func TestNormalizeOptionsEnforcesRulesPerType(t *testing.T) {
	single := []dto.QuizOptionInput{{Key: "A", Text: "甲"}, {Key: "B", Text: "乙"}}

	t.Run("单选题只能有一个正确答案", func(t *testing.T) {
		_, _, err := normalizeOptions(model.QuizTypeSingle, single, []string{"A", "B"})
		assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid)
	})

	t.Run("答案必须指向已声明的选项", func(t *testing.T) {
		_, _, err := normalizeOptions(model.QuizTypeSingle, single, []string{"Z"})
		assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid)
	})

	t.Run("选项标识不能重复", func(t *testing.T) {
		dup := []dto.QuizOptionInput{{Key: "A", Text: "甲"}, {Key: "a", Text: "乙"}}
		_, _, err := normalizeOptions(model.QuizTypeSingle, dup, []string{"A"})
		assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid)
	})

	t.Run("选项数量不足被拒", func(t *testing.T) {
		_, _, err := normalizeOptions(model.QuizTypeSingle, single[:1], []string{"A"})
		assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid)
	})

	t.Run("多选题答案不能覆盖全部选项", func(t *testing.T) {
		_, _, err := normalizeOptions(model.QuizTypeMultiple, single, []string{"A", "B"})
		assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid,
			"全选即正确答案会让题目失去区分度")
	})

	t.Run("多选题答案排序后入库", func(t *testing.T) {
		three := []dto.QuizOptionInput{
			{Key: "A", Text: "甲"}, {Key: "B", Text: "乙"}, {Key: "C", Text: "丙"},
		}
		_, correct, err := normalizeOptions(model.QuizTypeMultiple, three, []string{"c", "a"})
		require.NoError(t, err)
		assert.Equal(t, []string{"A", "C"}, []string(correct))
	})

	t.Run("判断题选项固定为正确与错误", func(t *testing.T) {
		opts, correct, err := normalizeOptions(model.QuizTypeJudge, nil, []string{"A"})
		require.NoError(t, err)
		require.Len(t, opts, 2)
		assert.Equal(t, "正确", opts[0].Text)
		assert.Equal(t, "错误", opts[1].Text)
		assert.Equal(t, []string{"A"}, []string(correct))

		// 判断题答案也必须是恰好一个
		_, _, err = normalizeOptions(model.QuizTypeJudge, nil, []string{"A", "B"})
		assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid)
	})

	t.Run("判断题不接受三个以上选项", func(t *testing.T) {
		three := []dto.QuizOptionInput{{Key: "A"}, {Key: "B"}, {Key: "C"}}
		_, _, err := normalizeOptions(model.QuizTypeJudge, three, []string{"A"})
		assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid)
	})
}

// ============================ 落库链路 ============================

type quizFixture struct {
	svc      *QuizService
	db       *gorm.DB
	adminID  int64
	userID   int64
	catK8s   int64
	catLinux int64
}

// newQuizFixture 用临时 SQLite 搭出真实的 repository → service 链路。
func newQuizFixture(t *testing.T) *quizFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err, "加载默认配置失败")

	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "quiz.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err, "初始化测试数据库失败")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	ctx := context.Background()
	users := repository.NewUserRepo(db)

	admin := &model.User{
		Phone: model.StrPtr("13900000001"), Nickname: "出题人",
		Role: model.RoleAdmin, Status: model.UserStatusActive,
	}
	require.NoError(t, users.Create(ctx, admin))

	user := &model.User{
		Phone: model.StrPtr("13900000002"), Nickname: "考生",
		Role: model.RoleUser, Status: model.UserStatusActive,
	}
	require.NoError(t, users.Create(ctx, user))

	cats := repository.NewCategoryRepo(db)
	courses := []*model.ArticleCategory{
		{Name: "K8s 与云原生", Slug: "k8s", Sort: 10, Enabled: true},
		{Name: "Linux 基础", Slug: "linux-basic", Sort: 20, Enabled: true},
	}
	for _, c := range courses {
		require.NoError(t, cats.Create(ctx, c))
	}

	svc := NewQuizService(repository.NewQuizRepo(db), cats, nil)

	return &quizFixture{
		svc: svc, db: db,
		adminID: admin.ID, userID: user.ID,
		catK8s: courses[0].ID, catLinux: courses[1].ID,
	}
}

// createPublished 录入一道已发布题目。
func (f *quizFixture) createPublished(t *testing.T, req dto.QuizQuestionCreateReq) *dto.AdminQuizItem {
	t.Helper()
	req.Status = model.QuizStatusPublished
	item, err := f.svc.Create(context.Background(), f.adminID, req)
	require.NoError(t, err, "录入题目失败")
	return item
}

// singleChoice 一道最简单的单选题：正确答案 A。
func singleChoice(stem string) dto.QuizQuestionCreateReq {
	return dto.QuizQuestionCreateReq{
		Stem: stem,
		Type: model.QuizTypeSingle,
		Options: []dto.QuizOptionInput{
			{Key: "A", Text: "正确答案"},
			{Key: "B", Text: "干扰项"},
			{Key: "C", Text: "另一个干扰项"},
		},
		Correct:    []string{"A"},
		Analysis:   "解析：选 A。",
		Difficulty: model.QuizDifficultyEasy,
	}
}

func TestQuizCreateRejectsCoveringAllOptions(t *testing.T) {
	f := newQuizFixture(t)
	_, err := f.svc.Create(context.Background(), f.adminID, dto.QuizQuestionCreateReq{
		Stem: "以下哪些操作是安全的？",
		Type: model.QuizTypeMultiple,
		Options: []dto.QuizOptionInput{
			{Key: "A", Text: "甲"}, {Key: "B", Text: "乙"}, {Key: "C", Text: "丙"},
		},
		Correct: []string{"A", "B", "C"},
	})
	assert.ErrorIs(t, err, errcode.ErrQuizOptionInvalid)
}

func TestQuizCreateJudgeFillsFixedOptions(t *testing.T) {
	f := newQuizFixture(t)

	// 判断题可以不传 options：选项固定是「正确 / 错误」，由服务端填。
	// 调用方自己传"对/错""是/否""True/False"会让统计与复习都乱掉。
	item := f.createPublished(t, dto.QuizQuestionCreateReq{
		Stem:     "判断：Kubernetes 中 Pod 是最小的可调度单元。",
		Type:     model.QuizTypeJudge,
		Correct:  []string{"A"},
		Analysis: "正确。Pod 是最小的可部署与调度单元。",
	})
	require.Len(t, item.Options, 2)
	assert.Equal(t, "A", item.Options[0].Key)
	assert.Equal(t, "正确", item.Options[0].Text)
	assert.Equal(t, "B", item.Options[1].Key)
	assert.Equal(t, "错误", item.Options[1].Text)
	assert.Equal(t, []string{"A"}, item.Correct)
}

func TestQuizDraftIsInvisibleToFrontend(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()

	draft, err := f.svc.Create(ctx, f.adminID, dto.QuizQuestionCreateReq{
		Stem:    "草稿题",
		Type:    model.QuizTypeSingle,
		Options: []dto.QuizOptionInput{{Key: "A", Text: "甲"}, {Key: "B", Text: "乙"}},
		Correct: []string{"A"},
		Status:  model.QuizStatusDraft,
	})
	require.NoError(t, err)

	items, total, err := f.svc.ListQuestions(ctx, dto.QuizListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total, "草稿不该出现在前台列表")
	assert.Empty(t, items)

	// 抽题同样抽不到
	_, err = f.svc.Practice(ctx, dto.QuizPracticeQuery{Count: 5})
	assert.ErrorIs(t, err, errcode.ErrQuizPoolEmpty)

	// 发布后可见
	_, err = f.svc.Publish(ctx, draft.ID)
	require.NoError(t, err)
	_, total, err = f.svc.ListQuestions(ctx, dto.QuizListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

func TestQuizAnswerRevealsSolutionAndMarksFresh(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()
	q := f.createPublished(t, singleChoice("Pod 一直 Pending 先看什么？"))

	// 答错也要给出正确答案与解析——题库的价值是让人学会。
	wrong, err := f.svc.Answer(ctx, f.userID, q.ID, dto.QuizAnswerReq{Chosen: []string{"B"}})
	require.NoError(t, err)
	assert.False(t, wrong.Correct)
	assert.Equal(t, []string{"A"}, wrong.CorrectOptions)
	assert.Equal(t, "解析：选 A。", wrong.Analysis)
	assert.True(t, wrong.Fresh, "首次作答应标记 fresh")

	// 再答一次：正确答案不变
	right, err := f.svc.Answer(ctx, f.userID, q.ID, dto.QuizAnswerReq{Chosen: []string{"A"}})
	require.NoError(t, err)
	assert.True(t, right.Correct)
	assert.False(t, right.Fresh, "重复作答不应再标记 fresh")
}

func TestQuizStatsCountsEachQuestionOnce(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()
	q1 := f.createPublished(t, singleChoice("题目一"))
	q2 := f.createPublished(t, singleChoice("题目二"))

	// 同一道题刷三遍：一次答对，统计里只应算一题一次。
	for i := 0; i < 3; i++ {
		_, err := f.svc.Answer(ctx, f.userID, q1.ID, dto.QuizAnswerReq{Chosen: []string{"A"}})
		require.NoError(t, err)
	}
	_, err := f.svc.Answer(ctx, f.userID, q2.ID, dto.QuizAnswerReq{Chosen: []string{"B"}})
	require.NoError(t, err)

	stats, err := f.svc.Stats(ctx, f.userID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalAnswered, "刷三遍同一题也只算一题")
	assert.Equal(t, int64(1), stats.TotalCorrect)
	assert.Equal(t, int64(1), stats.WrongCount)
	assert.InDelta(t, 50, stats.Accuracy, 0.01)
	assert.Equal(t, int64(0), stats.AttemptCount, "单题作答不产生整卷成绩")
}

func TestQuizSubmitScoresWholeAttempt(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()
	q1 := f.createPublished(t, singleChoice("题一"))
	q2 := f.createPublished(t, singleChoice("题二"))
	q3 := f.createPublished(t, singleChoice("题三"))

	result, err := f.svc.Submit(ctx, f.userID, dto.QuizSubmitReq{
		DurationSec: 120,
		Items: []dto.QuizSubmitItem{
			{QuestionID: q1.ID, Chosen: []string{"A"}}, // 对
			{QuestionID: q2.ID, Chosen: []string{"A"}}, // 对
			{QuestionID: q3.ID, Chosen: []string{"C"}}, // 错
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.QuestionCount)
	assert.Equal(t, int64(2), result.CorrectCount)
	assert.Equal(t, 67, result.Score, "2/3 四舍五入为 67")
	assert.Len(t, result.Items, 3)

	// 同一题在单次提交里重复出现只算一次
	deduped, err := f.svc.Submit(ctx, f.userID, dto.QuizSubmitReq{
		Items: []dto.QuizSubmitItem{
			{QuestionID: q1.ID, Chosen: []string{"A"}},
			{QuestionID: q1.ID, Chosen: []string{"B"}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), deduped.QuestionCount)
	assert.Equal(t, 100, deduped.Score)

	attempts, err := f.svc.Attempts(ctx, f.userID)
	require.NoError(t, err)
	assert.Len(t, attempts, 2)
}

func TestQuizWrongBookTracksLatestAnswerOnly(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()
	q := f.createPublished(t, singleChoice("这道题先错后对"))

	// 先答错 → 进错题本
	_, err := f.svc.Answer(ctx, f.userID, q.ID, dto.QuizAnswerReq{Chosen: []string{"B"}})
	require.NoError(t, err)

	items, total, err := f.svc.WrongBook(ctx, f.userID, dto.QuizWrongQuery{})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, []string{"B"}, items[0].Chosen)
	assert.Equal(t, []string{"A"}, items[0].CorrectOptions)
	// 错题本自带选项与解析：点进去复习不该再跳一次详情。
	assert.Len(t, items[0].Options, 3)
	assert.Equal(t, "解析：选 A。", items[0].Analysis)

	// 改对之后应从错题本消失——口径是"最近一次仍答错"，不是"曾经答错过"。
	_, err = f.svc.Answer(ctx, f.userID, q.ID, dto.QuizAnswerReq{Chosen: []string{"A"}})
	require.NoError(t, err)

	_, total, err = f.svc.WrongBook(ctx, f.userID, dto.QuizWrongQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total, "答对之后不该再挂在错题本里")
}

func TestQuizPracticeRespectsFiltersAndPoolSize(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()

	easy := singleChoice("简单题")
	easy.CategoryID = f.catK8s
	easy.Difficulty = model.QuizDifficultyEasy
	f.createPublished(t, easy)

	hard := singleChoice("难题")
	hard.CategoryID = f.catLinux
	hard.Difficulty = model.QuizDifficultyHard
	f.createPublished(t, hard)

	// count 超出题库总量时按实际数量返回，不报错
	items, err := f.svc.Practice(ctx, dto.QuizPracticeQuery{Count: 50})
	require.NoError(t, err)
	assert.Len(t, items, 2)

	items, err = f.svc.Practice(ctx, dto.QuizPracticeQuery{Count: 10, Difficulty: model.QuizDifficultyHard})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "难题", items[0].Stem)

	items, err = f.svc.Practice(ctx, dto.QuizPracticeQuery{Count: 10, CategoryID: f.catK8s})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "简单题", items[0].Stem)

	// 前台出参不含答案与解析
	assert.Empty(t, items[0].Tags)
}

func TestQuizAnswerRejectsQuestionThatIsNotPublished(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()

	draft, err := f.svc.Create(ctx, f.adminID, dto.QuizQuestionCreateReq{
		Stem:    "未发布的题",
		Type:    model.QuizTypeSingle,
		Options: []dto.QuizOptionInput{{Key: "A", Text: "甲"}, {Key: "B", Text: "乙"}},
		Correct: []string{"A"},
	})
	require.NoError(t, err)

	_, err = f.svc.Answer(ctx, f.userID, draft.ID, dto.QuizAnswerReq{Chosen: []string{"A"}})
	assert.ErrorIs(t, err, errcode.ErrQuizNotPublished)
}

func TestQuizPublishThenOfflineFlow(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()

	q, err := f.svc.Create(ctx, f.adminID, dto.QuizQuestionCreateReq{
		Stem:    "状态流转",
		Type:    model.QuizTypeSingle,
		Options: []dto.QuizOptionInput{{Key: "A", Text: "甲"}, {Key: "B", Text: "乙"}},
		Correct: []string{"A"},
	})
	require.NoError(t, err)

	_, err = f.svc.Publish(ctx, q.ID)
	require.NoError(t, err)
	_, err = f.svc.Publish(ctx, q.ID)
	assert.ErrorIs(t, err, errcode.ErrQuizStateInvalid, "重复发布应被拒")

	_, err = f.svc.Offline(ctx, q.ID)
	require.NoError(t, err)

	_, err = f.svc.Answer(ctx, f.userID, q.ID, dto.QuizAnswerReq{Chosen: []string{"A"}})
	assert.ErrorIs(t, err, errcode.ErrQuizNotPublished, "下线后不能再作答")

	// 下线是可逆的：内容只是下架，不是删除，改一改还能重新上。
	_, err = f.svc.Publish(ctx, q.ID)
	require.NoError(t, err)
	_, err = f.svc.Answer(ctx, f.userID, q.ID, dto.QuizAnswerReq{Chosen: []string{"A"}})
	require.NoError(t, err)

	_, err = f.svc.Offline(ctx, q.ID)
	require.NoError(t, err)
	_, err = f.svc.Offline(ctx, q.ID)
	assert.ErrorIs(t, err, errcode.ErrQuizStateInvalid, "已下线的题不能再下线")
}

// ============================ 批量导入 ============================

func TestQuizImportIsAllOrNothing(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()

	good := singleChoice("以下哪个命令查看磁盘使用率？")
	bad := singleChoice("这道题的选项只有一个")
	bad.Options = []dto.QuizOptionInput{{Key: "A", Text: "只有一个选项"}}

	res, err := f.svc.Import(ctx, f.adminID, dto.QuizQuestionImportReq{
		Items: []dto.QuizQuestionCreateReq{good, bad, singleChoice("第三道合法题目")},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Total)
	assert.Zero(t, res.Imported, "有任何一行不合法就一条都不能落库")
	require.Len(t, res.Errors, 1)
	assert.Equal(t, 2, res.Errors[0].Row, "错误要带行号，用户才知道改哪一行")

	var n int64
	require.NoError(t, f.db.Model(&model.QuizQuestion{}).Count(&n).Error)
	assert.Zero(t, n, "校验失败时库里不该有任何新题（半批入库最难收拾）")

	// 改好之后再传，三条全部落库，且都是草稿（导入的题没人逐条看过）
	ok := f.svcImportOK(t, []dto.QuizQuestionCreateReq{good, singleChoice("第二道"), singleChoice("第三道")})
	assert.Equal(t, 3, ok.Imported)
	assert.Empty(t, ok.Errors)

	var drafts int64
	require.NoError(t, f.db.Model(&model.QuizQuestion{}).
		Where("status = ?", model.QuizStatusDraft).Count(&drafts).Error)
	assert.EqualValues(t, 3, drafts, "批量导入一律先落草稿，确认后再发布")
}

// svcImportOK 导入一批合法题目，断言无错误行。
func (f *quizFixture) svcImportOK(t *testing.T, items []dto.QuizQuestionCreateReq) *dto.QuizImportResult {
	t.Helper()
	res, err := f.svc.Import(context.Background(), f.adminID, dto.QuizQuestionImportReq{Items: items})
	require.NoError(t, err)
	require.Empty(t, res.Errors)
	return res
}

func TestQuizImportRejectsUnknownCategory(t *testing.T) {
	f := newQuizFixture(t)

	req := singleChoice("分类填错的题")
	req.CategoryID = 99999
	res, err := f.svc.Import(context.Background(), f.adminID, dto.QuizQuestionImportReq{
		Items: []dto.QuizQuestionCreateReq{req},
	})
	require.NoError(t, err)
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0].Message, "分类")
}

// ============================ 题目投稿 ============================

func TestQuizContributeRequiresAnalysisAndStaysPending(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()

	noAnalysis := singleChoice("没有解析的题目")
	noAnalysis.Analysis = ""
	_, err := f.svc.Contribute(ctx, f.userID, dto.QuizContributeReq{
		Stem: noAnalysis.Stem, Type: noAnalysis.Type, Options: noAnalysis.Options,
		Correct: noAnalysis.Correct, Analysis: noAnalysis.Analysis,
	}, false)
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)
	assert.Contains(t, err.Error(), "解析", "没有解析的题，答错的人也不知道错在哪")

	withAnalysis := singleChoice("投稿的合法题目")
	resp, err := f.svc.Contribute(ctx, f.userID, dto.QuizContributeReq{
		Stem: withAnalysis.Stem, Type: withAnalysis.Type, Options: withAnalysis.Options,
		Correct: withAnalysis.Correct, Analysis: withAnalysis.Analysis,
		Difficulty: withAnalysis.Difficulty, CategoryID: f.catK8s,
	}, false)
	require.NoError(t, err)
	assert.Equal(t, model.QuizStatusPending, resp.Status, "投稿固定进待审，不接受调用方指定状态")

	saved, err := f.svc.AdminGet(ctx, resp.ID)
	require.NoError(t, err)
	assert.EqualValues(t, f.userID, saved.AuthorID, "作者固定取登录态，不收客户端传的 ID")

	// 待审的题不能出现在前台
	_, total, err := f.svc.ListQuestions(ctx, dto.QuizListQuery{})
	require.NoError(t, err)
	assert.Zero(t, total)

	// 专家直发：免审，但作者仍是他自己
	direct := singleChoice("专家直发的题目")
	directResp, err := f.svc.Contribute(ctx, f.adminID, dto.QuizContributeReq{
		Stem: direct.Stem, Type: direct.Type, Options: direct.Options,
		Correct: direct.Correct, Analysis: direct.Analysis,
	}, true)
	require.NoError(t, err)
	assert.Equal(t, model.QuizStatusPublished, directResp.Status)
}

func TestQuizReviewRequiresNoteOnRejectAndIsCAS(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()

	item := singleChoice("待审的题目")
	resp, err := f.svc.Contribute(ctx, f.userID, dto.QuizContributeReq{
		Stem: item.Stem, Type: item.Type, Options: item.Options,
		Correct: item.Correct, Analysis: item.Analysis,
	}, false)
	require.NoError(t, err)

	_, err = f.svc.ReviewQuestion(ctx, resp.ID, f.adminID, dto.QuizReviewReq{Action: "reject"})
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)
	assert.Contains(t, err.Error(), "原因")

	passed, err := f.svc.ReviewQuestion(ctx, resp.ID, f.adminID, dto.QuizReviewReq{Action: "pass"})
	require.NoError(t, err)
	assert.Equal(t, model.QuizStatusPublished, passed.Status)
	assert.NotNil(t, passed.PublishedAt)
	assert.EqualValues(t, f.adminID, passed.ReviewerID)

	// 第二次审核被拒。单线程下先命中"状态不是 pending"的前置检查；
	// 真正兜住并发双审的是 UpdateFieldsIfStatus 的 CAS（两个请求同时通过
	// 前置检查时，第二个会命中 0 行并改报"已被其他人处理"）——
	// 那条分支要靠并发才能触发，此处只锁住"不能重复审"这个结果。
	_, err = f.svc.ReviewQuestion(ctx, resp.ID, f.adminID, dto.QuizReviewReq{Action: "pass"})
	assert.ErrorIs(t, err, errcode.ErrQuizStateInvalid)
	assert.Contains(t, err.Error(), "待审核")
}

func TestQuizReviewAwardsPointsOnce(t *testing.T) {
	f := newQuizFixture(t)
	ctx := context.Background()
	f.svc.points = repository.NewPointRepo(f.db)

	item := singleChoice("过审后应当记分的题目")
	resp, err := f.svc.Contribute(ctx, f.userID, dto.QuizContributeReq{
		Stem: item.Stem, Type: item.Type, Options: item.Options,
		Correct: item.Correct, Analysis: item.Analysis,
	}, false)
	require.NoError(t, err)

	_, err = f.svc.ReviewQuestion(ctx, resp.ID, f.adminID, dto.QuizReviewReq{Action: "pass"})
	require.NoError(t, err)

	points, err := f.svc.points.PointsOf(ctx, f.userID)
	require.NoError(t, err)
	assert.EqualValues(t, pointsQuizPublished, points)
}
