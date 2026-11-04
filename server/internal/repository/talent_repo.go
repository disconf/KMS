package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
)

// TalentRepo 岗位对接一期的「人才名片」统计仓储。
//
// 名片要展示的是社区贡献，不是订单成绩——订单数据专家详情里已有，
// 这里补的是另一半：这个人在社区里留下了多少真实痕迹。三块数据：
// 回答（问答广场）、文章（教程/复盘等）、积分与榜单名次。
// 全部是只读聚合查询，一条名片一次取齐，避免前端串行调三个接口。
type TalentRepo struct {
	db     *gorm.DB
	points *PointRepo
}

// NewTalentRepo 创建仓储。points 复用全局实例（无状态，共享即可）。
func NewTalentRepo(db *gorm.DB, points *PointRepo) *TalentRepo {
	return &TalentRepo{db: db, points: points}
}

// TalentContribution 一个用户的社区贡献汇总。
type TalentContribution struct {
	// Points 贡献积分；Rank 榜上名次（0 = 未上榜）；RankedUsers 榜上总人数。
	Points      int64 `json:"points"`
	Rank        int   `json:"rank"`
	RankedUsers int64 `json:"rankedUsers"`
	// Answers 已发布的回答数；AcceptedAnswers 被采纳数；AdoptionRate 采纳率（%，保留一位由前端取整）。
	Answers         int64   `json:"answers"`
	AcceptedAnswers int64   `json:"acceptedAnswers"`
	AdoptionRate    float64 `json:"adoptionRate"`
	// Articles 已发布文章数；ArticleViews 累计阅读。
	Articles     int64 `json:"articles"`
	ArticleViews int64 `json:"articleViews"`
}

// ContributionOf 取一个用户的社区贡献。任何一块查不到都按 0 处理——
// 名片是展示页，缺一块数据不应该 500 整张卡。
func (r *TalentRepo) ContributionOf(ctx context.Context, userID int64) (*TalentContribution, error) {
	tc := &TalentContribution{}

	// 回答统计：只数 published，待审/被拒的回答不能给名片充数。
	var ans struct {
		Total    int64
		Accepted int64
	}
	err := r.db.WithContext(ctx).Model(&model.QaAnswer{}).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN is_accepted THEN 1 ELSE 0 END), 0) AS accepted").
		Where("user_id = ? AND status = ?", userID, "published").
		Take(&ans).Error
	if err != nil {
		return nil, err
	}
	tc.Answers = ans.Total
	tc.AcceptedAnswers = ans.Accepted
	if ans.Total > 0 {
		// 采纳率保留整数百分比即可——名片上的数字是给人扫一眼的，不是对账的。
		tc.AdoptionRate = float64(ans.Accepted) * 100 / float64(ans.Total)
	}

	// 文章统计：同理只数已发布，阅读数是贡献浓度的旁证。
	var art struct {
		Total int64
		Views int64
	}
	err = r.db.WithContext(ctx).Model(&model.Article{}).
		Select("COUNT(*) AS total, COALESCE(SUM(view_count), 0) AS views").
		Where("author_id = ? AND status = ?", userID, model.ArticleStatusPublished).
		Take(&art).Error
	if err != nil {
		return nil, err
	}
	tc.Articles = art.Total
	tc.ArticleViews = art.Views

	// 积分与名次：RankOf 返回 rank=0 表示未上榜，前端按「未上榜」渲染。
	rank, points, err := r.points.RankOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	tc.Rank = rank
	tc.Points = points
	if ranked, err := r.points.RankedCount(ctx); err == nil {
		tc.RankedUsers = ranked
	}

	return tc, nil
}
