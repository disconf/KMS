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
	"github.com/opsexpert/platform/internal/pkg/storage"
	"github.com/opsexpert/platform/internal/repository"
)

// reviewFixture 专家主页评价区的测试夹具。
//
// 同样走真实 SQLite：评价区的正确性全压在两条 SQL 上（GROUP BY 分布、
// JOIN users 取昵称），换成假仓储就只剩"字段有没有赋值"可测。
type reviewFixture struct {
	svc      *ExpertService
	db       *gorm.DB
	reviews  *repository.ReviewRepo
	experts  *repository.ExpertRepo
	expertID int64 // 档案 ID（路径参数）
	userID   int64 // 专家用户 ID（reviews.expert_id 存的是它）
	pending  int64 // 未过审专家的档案 ID
}

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err, "加载默认配置失败")

	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "review.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err, "初始化测试数据库失败")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	store, err := storage.New(storage.Options{
		Driver:   "local",
		LocalDir: t.TempDir(),
		BaseURL:  "http://localhost:8080/files",
	})
	require.NoError(t, err)

	ctx := context.Background()
	users := repository.NewUserRepo(db)
	experts := repository.NewExpertRepo(db)

	expertUser := &model.User{Phone: model.StrPtr("13900002001"), Nickname: "陈专家",
		Role: model.RoleExpert, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, expertUser))
	pendingUser := &model.User{Phone: model.StrPtr("13900002002"), Nickname: "待审专家",
		Role: model.RoleUser, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, pendingUser))

	approved := &model.Expert{
		UserID: expertUser.ID, RealName: "陈志远",
		AcceptingOrder: true, Status: model.ExpertStatusApproved,
	}
	require.NoError(t, experts.Create(ctx, approved))
	pendingExpert := &model.Expert{
		UserID: pendingUser.ID, RealName: "李待审",
		Status: model.ExpertStatusPending,
	}
	require.NoError(t, experts.Create(ctx, pendingExpert))

	reviews := repository.NewReviewRepo(db)
	svc := NewExpertService(
		experts, repository.NewSkillRepo(db), users, repository.NewOrderRepo(db),
		repository.NewSettlementRepo(db), repository.NewWithdrawalRepo(db),
		reviews, NewUploadService(store, "test-sign-secret", 20),
	)

	return &reviewFixture{
		svc: svc, db: db, reviews: reviews, experts: experts,
		expertID: approved.ID, userID: expertUser.ID, pending: pendingExpert.ID,
	}
}

// addReview 直接落一条评价。
//
// 不经过下单-完成-评价这条完整链路：那个流程已被订单模块的用例覆盖，
// 这里要验的是"读评价"的口径，造一条最小可用的行就够，
// 也让用例不必为 order_id 唯一索引准备一堆订单。
func (f *reviewFixture) addReview(t *testing.T, idx int, score int, anonymous bool, content, reply string) {
	t.Helper()
	// 用户 ID 由库分配（Base.ID 是自增主键，不能直接写死），
	// 昵称带序号是为了断言时能认出是哪一条。
	u := &model.User{
		Phone: model.StrPtr("13700000" + pad4(idx)), Nickname: "评价人" + pad4(idx),
		Role: model.RoleUser, Status: model.UserStatusActive,
	}
	require.NoError(t, f.db.Create(u).Error)
	require.NoError(t, f.db.Create(&model.Review{
		OrderID: int64(9000 + idx), UserID: u.ID, ExpertID: f.userID,
		Score: score, ProfessionalScore: score, SpeedScore: score, CommunicationScore: score,
		Content: content, Anonymous: anonymous, Recommend: score >= 4, Reply: reply,
	}).Error)
}

func pad4(n int) string {
	s := []byte("0000")
	for i := 3; i >= 0 && n > 0; i-- {
		s[i] = byte('0' + n%10)
		n /= 10
	}
	return string(s)
}

func TestExpertReviewsEmpty(t *testing.T) {
	f := newReviewFixture(t)

	page, err := f.svc.ReviewsOfExpert(context.Background(), f.expertID, dto.PageQuery{Page: 1, PageSize: 5})
	require.NoError(t, err)

	assert.Equal(t, int64(0), page.Summary.Total, "没有评价时总数应为 0")
	assert.Equal(t, 0.0, page.Summary.AvgScore, "没有评价时平均分为 0，前端据此区分新专家")
	assert.Empty(t, page.List)
	// 分布条要画五根，缺的那档必须是 0 而不是不存在。
	require.Len(t, page.Summary.Distribution, 5, "分布应固定 5 档")
	for _, b := range page.Summary.Distribution {
		assert.Equal(t, int64(0), b.Count, "%d 星应为 0 条", b.Score)
	}
	assert.Equal(t, 5, page.Summary.Distribution[0].Score, "第一档应是 5 星")
	assert.Equal(t, 1, page.Summary.Distribution[4].Score, "最后一档应是 1 星")
}

func TestExpertReviewsAnonymousMasked(t *testing.T) {
	f := newReviewFixture(t)
	f.addReview(t, 1, 5, true, "夜里三点还在帮我看日志，专业。", "")
	f.addReview(t, 2, 4, false, "定位得很准。", "感谢反馈")

	page, err := f.svc.ReviewsOfExpert(context.Background(), f.expertID, dto.PageQuery{Page: 1, PageSize: 5})
	require.NoError(t, err)
	require.Len(t, page.List, 2)

	var anon, named *dto.ExpertReviewItem
	for i := range page.List {
		if page.List[i].Anonymous {
			anon = &page.List[i]
		} else {
			named = &page.List[i]
		}
	}
	require.NotNil(t, anon, "应有一条匿名评价")
	require.NotNil(t, named, "应有一条实名评价")

	assert.Equal(t, "匿名用户", anon.Nickname, "匿名评价必须换成匿名昵称")
	assert.Empty(t, anon.Avatar, "匿名连头像一起抹掉，只藏名字等于没匿名")
	assert.Equal(t, "评价人0002", named.Nickname, "实名评价保留昵称")
	assert.Equal(t, "感谢反馈", named.Reply, "专家回复要带出来")
}

func TestExpertReviewsDistributionSumsToTotal(t *testing.T) {
	f := newReviewFixture(t)
	// 5 星两条、4 星一条、3 星一条、1 星一条：分布之和必须等于 5。
	f.addReview(t, 1, 5, false, "好", "")
	f.addReview(t, 2, 5, false, "很好", "")
	f.addReview(t, 3, 4, false, "不错", "")
	f.addReview(t, 4, 3, false, "一般", "")
	f.addReview(t, 5, 1, false, "不满意", "")

	page, err := f.svc.ReviewsOfExpert(context.Background(), f.expertID, dto.PageQuery{Page: 1, PageSize: 10})
	require.NoError(t, err)

	assert.Equal(t, int64(5), page.Summary.Total)
	var sum int64
	for _, b := range page.Summary.Distribution {
		sum += b.Count
	}
	assert.Equal(t, page.Summary.Total, sum, "各档条数之和必须等于评价总数")

	// 平均分 (5+5+4+3+1)/5 = 3.6；好评是 >=4，3/5 = 60%。
	assert.InDelta(t, 3.6, page.Summary.AvgScore, 0.001)
	assert.InDelta(t, 60.0, page.Summary.GoodRate, 0.001)
}

func TestExpertReviewsPagination(t *testing.T) {
	f := newReviewFixture(t)
	for i := 1; i <= 7; i++ {
		f.addReview(t, i, 5, false, "第几条", "")
	}

	first, err := f.svc.ReviewsOfExpert(context.Background(), f.expertID, dto.PageQuery{Page: 1, PageSize: 5})
	require.NoError(t, err)
	assert.Len(t, first.List, 5)
	assert.Equal(t, int64(7), first.Total, "total 是全部条数，不是本页条数")

	second, err := f.svc.ReviewsOfExpert(context.Background(), f.expertID, dto.PageQuery{Page: 2, PageSize: 5})
	require.NoError(t, err)
	assert.Len(t, second.List, 2)

	// 两页不能有重复（同一评价出现两次会让"还有 N 条"永远算不圆）。
	seen := map[int64]bool{}
	for _, r := range append(first.List, second.List...) {
		assert.False(t, seen[r.CreatedAt.UnixNano()], "分页不应重复返回同一条评价")
		seen[r.CreatedAt.UnixNano()] = true
	}
}

func TestExpertReviewsHiddenForUnapproved(t *testing.T) {
	f := newReviewFixture(t)

	_, err := f.svc.ReviewsOfExpert(context.Background(), f.pending, dto.PageQuery{Page: 1, PageSize: 5})
	require.Error(t, err, "未过审的专家不该能通过评价接口被看到")
	assert.ErrorIs(t, err, errcode.ErrExpertNotFound, "应返回 404 语义，不能泄露「存在但未过审」")
}

func TestExpertReviewsUsesUserIDNotProfileID(t *testing.T) {
	f := newReviewFixture(t)
	f.addReview(t, 1, 5, false, "好", "")

	// reviews.expert_id 存用户 ID。若服务里误用档案 ID 查询，这里会拿到 0 条。
	page, err := f.svc.ReviewsOfExpert(context.Background(), f.expertID, dto.PageQuery{Page: 1, PageSize: 5})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Summary.Total, "按档案 ID 查不到东西说明取错了关联列")
	assert.Equal(t, int64(1), page.Total)
}
