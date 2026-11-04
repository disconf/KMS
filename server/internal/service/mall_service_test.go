package service

// 积分商城兑换事务的口径测试。
//
// 兑换是全站唯一「扣积分」的地方，最怕三件事：
// 余额被扣成负数、库存被超卖、取消退款把人刷富。
// 三者的防线都在 CAS 条件更新上，这份测试把每个分支都打一遍。

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

func newMallTestEnv(t *testing.T) (*MallService, *repository.PointRepo, *gorm.DB, context.Context) {
	t.Helper()
	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "mall.db")
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
	svc := NewMallService(repository.NewMallRepo(db), points)
	return svc, points, db, context.Background()
}

// mallUser 建一个指定余额的测试用户。
func mallUser(t *testing.T, db *gorm.DB, points *repository.PointRepo, ctx context.Context, phone string, balance int64) *model.User {
	t.Helper()
	u := &model.User{Phone: model.StrPtr(phone), Nickname: "测试用户" + phone, Role: model.RoleUser, Status: model.UserStatusActive}
	require.NoError(t, db.Create(u).Error)
	if balance > 0 {
		ok, err := points.Award(ctx, &model.PointLog{
			UserID: u.ID, Action: model.PointActionAnswerLiked,
			Points: balance, TargetType: "test", TargetID: 1,
			UniqKey: "mall-test-" + phone,
		})
		require.NoError(t, err)
		require.True(t, ok)
	}
	return u
}

func TestMallRedeemFlow(t *testing.T) {
	svc, points, db, ctx := newMallTestEnv(t)

	u := mallUser(t, db, points, ctx, "13900000102", 100)

	// 商品限量 2 件、单价 30。
	item := &model.MallItem{
		Title: "运维周报模板", Kind: model.MallKindTemplate,
		PointsCost: 30, Stock: 2, Active: true,
	}
	require.NoError(t, db.Create(item).Error)

	// 第一次兑换成功，余额 70。
	resp, err := svc.Redeem(ctx, u.ID, item.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(70), resp.PointsAfter)
	assert.Equal(t, model.MallRedemptionPending, resp.Redemption.Status)

	// 连续兑换把库存买空：第 2 次成功，第 3 次库存不足。
	_, err = svc.Redeem(ctx, u.ID, item.ID)
	require.NoError(t, err)
	_, err = svc.Redeem(ctx, u.ID, item.ID)
	assert.ErrorIs(t, err, errcode.ErrMallStockEmpty)

	// 余额 40：买不起 100 分的商品，且失败不落单、不扣分。
	expensive := &model.MallItem{Title: "大礼包", Kind: model.MallKindBenefit, PointsCost: 100, Unlimited: true, Active: true}
	require.NoError(t, db.Create(expensive).Error)
	_, err = svc.Redeem(ctx, u.ID, expensive.ID)
	assert.ErrorIs(t, err, errcode.ErrMallPointsShort)
	reds, err := svc.MyRedemptions(ctx, u.ID, 10)
	require.NoError(t, err)
	assert.Len(t, reds, 2)
	balance, _ := points.PointsOf(ctx, u.ID)
	assert.Equal(t, int64(40), balance)

	// 下架商品不可兑换。
	require.NoError(t, db.Model(item).Update("active", false).Error)
	_, err = svc.Redeem(ctx, u.ID, item.ID)
	assert.ErrorIs(t, err, errcode.ErrMallItemNotFound)
}

func TestMallAdminCancelRefundsOnce(t *testing.T) {
	svc, points, db, ctx := newMallTestEnv(t)

	u := mallUser(t, db, points, ctx, "13900000103", 50)

	item := &model.MallItem{Title: "不限量权益", Kind: model.MallKindBenefit, PointsCost: 20, Unlimited: true, Active: true}
	require.NoError(t, db.Create(item).Error)

	resp, err := svc.Redeem(ctx, u.ID, item.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(30), resp.PointsAfter)

	// 取消退分一次：回到 50。
	rid := resp.Redemption.ID
	require.NoError(t, svc.AdminCancel(ctx, rid, "发错货了"))
	balance, _ := points.PointsOf(ctx, u.ID)
	assert.Equal(t, int64(50), balance)

	// 已取消的单不能再取消——退分幂等靠 CAS 终态，不是靠唯一索引重放。
	err = svc.AdminCancel(ctx, rid, "再退一次")
	assert.ErrorIs(t, err, errcode.ErrMallRedemptionBad)
	balance, _ = points.PointsOf(ctx, u.ID)
	assert.Equal(t, int64(50), balance, "重复取消不能刷分")

	// 发放后的单也不能取消（终态互不可达）。
	resp2, err := svc.Redeem(ctx, u.ID, item.ID)
	require.NoError(t, err)
	require.NoError(t, svc.AdminFulfill(ctx, resp2.Redemption.ID, "兑换码 ABCD"))
	err = svc.AdminCancel(ctx, resp2.Redemption.ID, "想反悔")
	assert.ErrorIs(t, err, errcode.ErrMallRedemptionBad)
}
