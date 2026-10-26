package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// NewsRepo 运维资讯订阅源的数据访问，以及抓取结果的落库。
//
// 抓取入库刻意放在仓储层：去重要靠 external_id 的唯一索引，
// 判断「插没插进去」只能看 Create 的返回值。放到 service 里做
// 「先查一次再插」在并发下会漏——两个副本同时抓同一个源就会重复。
type NewsRepo struct {
	db *gorm.DB
}

// NewNewsRepo 创建 NewsRepo。
func NewNewsRepo(db *gorm.DB) *NewsRepo { return &NewsRepo{db: db} }

// ListSources 返回订阅源列表。enabledOnly 为 true 时只返回启用中的。
func (r *NewsRepo) ListSources(ctx context.Context, enabledOnly bool) ([]model.NewsSource, error) {
	q := r.db.WithContext(ctx).Model(&model.NewsSource{})
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}
	items := make([]model.NewsSource, 0, 12)
	if err := q.Order("sort ASC, id ASC").Find(&items).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询资讯订阅源失败: %w", err))
	}
	return items, nil
}

// GetSource 按主键查询订阅源。
func (r *NewsRepo) GetSource(ctx context.Context, id int64) (*model.NewsSource, error) {
	var s model.NewsSource
	if err := r.db.WithContext(ctx).First(&s, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &s, nil
}

// ExistsSourceName 判断来源名称是否重复。
func (r *NewsRepo) ExistsSourceName(ctx context.Context, name string, excludeID int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&model.NewsSource{}).Where("name = ?", name)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("校验订阅源名称失败: %w", err))
	}
	return count > 0, nil
}

// CreateSource 新增订阅源。
func (r *NewsRepo) CreateSource(ctx context.Context, s *model.NewsSource) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建订阅源失败: %w", err))
	}
	return nil
}

// UpdateSourceFields 局部更新订阅源。
func (r *NewsRepo) UpdateSourceFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.NewsSource{}).
		Where("id = ?", id).Updates(fields).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新订阅源 %d 失败: %w", id, err))
	}
	return nil
}

// DeleteSource 删除订阅源。已抓取的文章不受影响——它们已经独立成文，
// 删源只是停止后续抓取。
func (r *NewsRepo) DeleteSource(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&model.NewsSource{}, id).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("删除订阅源 %d 失败: %w", id, err))
	}
	return nil
}

// SaveFeedArticle 写入一条抓取来的文章。
//
// 返回 written=false 表示 external_id 已存在（之前抓过），这是正常情况，
// 不是错误：任务每半小时跑一次，同一个源里的大多数条目必然已经见过。
func (r *NewsRepo) SaveFeedArticle(ctx context.Context, a *model.Article) (written bool, err error) {
	if a.ExternalID == nil || *a.ExternalID == "" {
		return false, fmt.Errorf("抓取文章缺少 external_id，无法去重")
	}
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		if isDuplicateKeyError(err) {
			return false, nil
		}
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("写入抓取文章失败: %w", err))
	}
	return true, nil
}

// SetSourceStatus 记录一次抓取的结果，供后台判断源是否还有效。
func (r *NewsRepo) SetSourceStatus(ctx context.Context, id int64, status, lastErr string, fetched int64) error {
	fields := map[string]any{
		"last_fetched_at": gorm.Expr("CURRENT_TIMESTAMP"),
		"last_status":     status,
		"last_error":      lastErr,
	}
	if fetched > 0 {
		fields["fetched_count"] = gorm.Expr("fetched_count + ?", fetched)
	}
	if err := r.db.WithContext(ctx).Model(&model.NewsSource{}).
		Where("id = ?", id).Updates(fields).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新订阅源状态失败: %w", err))
	}
	return nil
}

// CountPendingNews 统计待采用的资讯条数，用于后台角标。
func (r *NewsRepo) CountPendingNews(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Article{}).
		Where("channel = ? AND status = ?", model.ChannelOpsNews, model.ArticleStatusPending).
		Count(&n).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计待采用资讯失败: %w", err))
	}
	return n, nil
}
