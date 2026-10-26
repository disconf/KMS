package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// ReviewRepo 订单评价数据访问。
type ReviewRepo struct {
	db *gorm.DB
}

// NewReviewRepo 创建 ReviewRepo。
func NewReviewRepo(db *gorm.DB) *ReviewRepo { return &ReviewRepo{db: db} }

// Create 提交一条评价。
//
// order_id 上有唯一索引，"一单一评"这条规则交给索引来守：
// 并发双击提交时也只有一次能插入成功，另一次拿到 ErrReviewDuplicated。
// 用"先查有没有再插"的话，两个请求会同时查到"没有"，然后各插一条。
func (r *ReviewRepo) Create(ctx context.Context, rv *model.Review) error {
	err := r.db.WithContext(ctx).Create(rv).Error
	if err != nil {
		if isDuplicateKeyError(err) {
			return errcode.ErrReviewDuplicated
		}
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建评价失败: %w", err))
	}
	return nil
}

// GetByOrderID 按订单查评价。
func (r *ReviewRepo) GetByOrderID(ctx context.Context, orderID int64) (*model.Review, error) {
	var rv model.Review
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&rv).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &rv, nil
}

// ExistsByOrderID 判断订单是否已评价。
func (r *ReviewRepo) ExistsByOrderID(ctx context.Context, orderID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Review{}).Where("order_id = ?", orderID).
		Count(&count).Error
	if err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("查询评价是否存在失败: %w", err))
	}
	return count > 0, nil
}

// ReviewedIDs 批量判断哪些订单已评价，返回已评价的订单 ID 集合。
//
// 列表页一次要标 20 条订单的"已评价"气泡，逐条 ExistsByOrderID 就是 20 次
// 往返。这里一次 IN 查询取回命中集合，调用方用 map 查即可。
func (r *ReviewRepo) ReviewedIDs(ctx context.Context, orderIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(orderIDs))
	if len(orderIDs) == 0 {
		return out, nil
	}

	var ids []int64
	err := r.db.WithContext(ctx).Model(&model.Review{}).
		Where("order_id IN ?", orderIDs).
		Pluck("order_id", &ids).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("批量查询评价是否存在失败: %w", err))
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// ListByExpert 分页查某专家收到的评价。
func (r *ReviewRepo) ListByExpert(ctx context.Context, expertUserID int64, offset, limit int) ([]model.Review, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Review{}).Where("expert_id = ?", expertUserID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家评价数失败: %w", err))
	}
	if total == 0 {
		return []model.Review{}, 0, nil
	}

	items := make([]model.Review, 0, limit)
	err := q.Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询专家评价失败: %w", err))
	}
	return items, total, nil
}

// ReviewRow 是带评价人展示信息的评价行。
//
// 昵称与头像必须 JOIN 出来：评价列表要显示"谁评的"，
// 而逐条回查用户表就是 20 次往返。
type ReviewRow struct {
	model.Review
	Nickname string
	Avatar   string
}

// ListPublicByExpert 分页查某专家的公开评价（附带评价人昵称头像）。
//
// 与 ListByExpert 的区别只在于多取账号展示信息，排序与分页口径完全一致。
func (r *ReviewRepo) ListPublicByExpert(ctx context.Context, expertUserID int64, offset, limit int) ([]ReviewRow, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Review{}).Where("reviews.expert_id = ?", expertUserID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家评价数失败: %w", err))
	}
	if total == 0 {
		return []ReviewRow{}, 0, nil
	}

	rows := make([]ReviewRow, 0, limit)
	err := q.Select("reviews.*, users.nickname AS nickname, users.avatar AS avatar").
		Joins("LEFT JOIN users ON users.id = reviews.user_id").
		Order("reviews.created_at DESC, reviews.id DESC").
		Offset(offset).Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询专家评价失败: %w", err))
	}
	return rows, total, nil
}

// ScoreDistribution 统计 5..1 星各有多少条。
//
// 用 GROUP BY 而不是把评分全取回来数一遍：专家页首屏就要画分布条，
// 数据量随评价数线性增长，而这个聚合永远是 5 行。
func (r *ReviewRepo) ScoreDistribution(ctx context.Context, expertUserID int64) (map[int]int64, error) {
	var rows []struct {
		Score int
		Count int64
	}
	err := r.db.WithContext(ctx).Model(&model.Review{}).
		Select("score, COUNT(*) AS count").
		Where("expert_id = ?", expertUserID).
		Group("score").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计评分分布失败: %w", err))
	}

	// 固定补齐 1..5：分布条要画五根，缺数据的那档必须是 0 而不是不存在，
	// 否则前端得自己补，两边补一遍总会有一边漏。
	out := map[int]int64{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	for _, row := range rows {
		if row.Score >= 1 && row.Score <= 5 {
			out[row.Score] = row.Count
		}
	}
	return out, nil
}

// StatsOfExpert 汇总某专家的评分数据。
//
// 用 SQL 聚合而不是把评价全取回来在内存里算：专家页每次打开都要展示评分，
// 一个做过几百单的专家会被取回几百条记录，只为算三个数。
func (r *ReviewRepo) StatsOfExpert(ctx context.Context, expertUserID int64) (ReviewStats, error) {
	var row struct {
		Total    int64
		AvgScore *float64
		Good     int64
	}
	// SUM/AVG 在没有行时返回 NULL，直接扫进 int64 会报 "converting NULL"。
	// 用 COALESCE 兜成 0，让"还没收到评价"和"评价全是差评"在返回值上可区分，
	// 而不是靠调用方去猜一个 err。
	err := r.db.WithContext(ctx).Model(&model.Review{}).
		Select("COUNT(*) AS total, AVG(score) AS avg_score, "+
			"COALESCE(SUM(CASE WHEN score >= 4 THEN 1 ELSE 0 END), 0) AS good").
		Where("expert_id = ?", expertUserID).
		Scan(&row).Error
	if err != nil {
		return ReviewStats{}, errcode.ErrDatabase.Wrap(fmt.Errorf("汇总专家评分失败: %w", err))
	}

	stats := ReviewStats{Total: row.Total, GoodCount: row.Good}
	if row.AvgScore != nil {
		stats.AvgScore = *row.AvgScore
	}
	if row.Total > 0 {
		stats.GoodRate = float64(row.Good) / float64(row.Total) * 100
	}
	return stats, nil
}

// Reply 专家回复评价。
func (r *ReviewRepo) Reply(ctx context.Context, orderID int64, reply string, now time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.Review{}).
		Where("order_id = ?", orderID).
		Updates(map[string]any{"reply": reply, "replied_at": now})
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("回复评价失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return errcode.ErrNotFound
	}
	return nil
}

// Appeal 专家对差评提起申诉。
func (r *ReviewRepo) Appeal(ctx context.Context, orderID int64, text string) error {
	res := r.db.WithContext(ctx).Model(&model.Review{}).
		Where("order_id = ?", orderID).
		Updates(map[string]any{"appealed": true, "appeal_text": text})
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("提交评价申诉失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return errcode.ErrNotFound
	}
	return nil
}

// ReviewStats 是专家评分汇总结果。
type ReviewStats struct {
	Total     int64
	AvgScore  float64
	GoodCount int64
	GoodRate  float64
}
