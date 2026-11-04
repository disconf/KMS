package service

// 贡献排行榜的口径测试。
//
// 排行榜最容易坏的不是排序本身，而是「榜单口径」与「我的名次口径」
// 各改各的：一处过滤了封禁用户另一处没过滤，就会出现
// "榜上第 7 名，我的卡片写着第 8 名"这种说不清的错位。
// 这份测试把两个口径放在一起验，谁单独走岔都会在这里红。

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/repository"
)

func TestQaLeaderboard(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "board.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	points := repository.NewPointRepo(db)
	svc := NewQaService(
		repository.NewQaRepo(db), points,
		repository.NewUserRepo(db), repository.NewAuditRepo(db),
		NewArticleService(repository.NewArticleRepo(db), repository.NewCategoryRepo(db),
			repository.NewUserRepo(db), nil),
	)
	ctx := context.Background()

	// 注册先后被用来打破同分平局，所以这里的创建顺序本身就是断言的一部分。
	mkUser := func(phone, nick, status string) int64 {
		u := &model.User{Phone: model.StrPtr(phone), Nickname: nick, Role: model.RoleUser, Status: status}
		require.NoError(t, db.Create(u).Error)
		return u.ID
	}
	sameFirst := mkUser("13900000001", "同分先来的", model.UserStatusActive)
	sameSecond := mkUser("13900000002", "同分后来的", model.UserStatusActive)
	banned := mkUser("13900000003", "被封禁的", model.UserStatusDisabled)

	award := func(userID int64, n int64) {
		ok, err := points.Award(ctx, &model.PointLog{
			UserID: userID, Action: model.PointActionAnswerLiked,
			Points: n, TargetType: "answer", TargetID: 1,
			// 每人一个独立幂等键，不撞唯一索引。
			UniqKey: fmt.Sprintf("test-board-%d", userID),
		})
		require.NoError(t, err)
		require.True(t, ok)
	}

	// 三位活跃用户：200 / 50 / 10，再加一名踩线 1 分的。
	mkScored := func(phone, nick string, pts int64) int64 {
		id := mkUser(phone, nick, model.UserStatusActive)
		award(id, pts)
		return id
	}
	top := mkScored("13900000004", "资深甲", 200)
	mid := mkScored("13900000005", "贡献乙", 50)
	low := mkScored("13900000006", "活跃丙", 10)
	mkScored("13900000007", "踩线丁", 1)

	// 封禁用户 +40：分数在，但永远不该出现在榜单上。
	award(banned, 40)
	// 两个同分用户各 +12，凑成 13 分平局，验证平局按注册先后排序。
	award(sameFirst, 12)
	award(sameSecond, 12)

	board, err := svc.Leaderboard(ctx, QaViewer{}, LeaderboardLimit)
	require.NoError(t, err)

	got := make([]int64, 0, len(board.Items))
	for _, it := range board.Items {
		got = append(got, it.UserID)
	}
	// 期望：200 > 50 > 13(先) > 13(后) > 10 > 1。
	// 封禁用户的 40 分被排除——他不在榜上，但分数还在。
	assert.Equal(t, []int64{top, mid, sameFirst, sameSecond, low}, got[:5])
	assert.Len(t, board.Items, 6, "六名活跃用户全部上榜")
	assert.Equal(t, int64(6), board.Total, "封禁的不算、1 分的算")

	// 名次与榜单口径一致：榜上第一名查出来也是第 1。
	rank, pts, err := points.RankOf(ctx, top)
	require.NoError(t, err)
	assert.Equal(t, 1, rank)
	assert.Equal(t, int64(200), pts)

	// 同分的两人：注册早的名次在前，且名次等于榜单位置。
	rankFirst, _, err := points.RankOf(ctx, sameFirst)
	require.NoError(t, err)
	rankSecond, _, err := points.RankOf(ctx, sameSecond)
	require.NoError(t, err)
	assert.Equal(t, 3, rankFirst)
	assert.Equal(t, 4, rankSecond)

	// 封禁用户：有 40 分也不上榜，但分数如实保留。
	rankBanned, ptsBanned, err := points.RankOf(ctx, banned)
	require.NoError(t, err)
	assert.Equal(t, 0, rankBanned, "封禁用户不该有名次")
	assert.Equal(t, int64(40), ptsBanned, "封禁保留分数但不上榜")

	// 游客视角：myRank 恒为 0，前端据此隐藏"我的名次"卡。
	guestBoard, err := svc.Leaderboard(ctx, QaViewer{}, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, guestBoard.MyRank)
}
