package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// ResourceFilter 是资源列表的查询条件。
type ResourceFilter struct {
	Status     string
	CategoryID int64
	Keyword    string
	// KeywordGroups 是展开后的检索词组，由 service 的查询理解层产出。
	KeywordGroups KeywordGroups
	Tag           string
	FileExt       string
	// Sort 取值 latest（默认）/ hot（按浏览数）/ download（按下载数）。
	Sort string
}

// ResourceRepo 资源下载数据访问。
type ResourceRepo struct {
	db *gorm.DB
}

// NewResourceRepo 创建 ResourceRepo。
func NewResourceRepo(db *gorm.DB) *ResourceRepo { return &ResourceRepo{db: db} }

// GetByID 按主键查询；不存在时返回 errcode.ErrResourceNotFound。
func (r *ResourceRepo) GetByID(ctx context.Context, id int64) (*model.Resource, error) {
	var res model.Resource
	if err := r.db.WithContext(ctx).First(&res, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrResourceNotFound)
	}
	return &res, nil
}

// GetBySlug 按 slug 查询。
func (r *ResourceRepo) GetBySlug(ctx context.Context, slug string) (*model.Resource, error) {
	var res model.Resource
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&res).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrResourceNotFound)
	}
	return &res, nil
}

// SlugExists 判断 slug 是否已被占用。
func (r *ResourceRepo) SlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	return existsBy(ctx, r.db, &model.Resource{}, "slug", slug, excludeID, "资源 slug")
}

// List 按条件分页查询。
func (r *ResourceRepo) List(ctx context.Context, f ResourceFilter, offset, limit int) ([]model.Resource, int64, error) {
	groups := f.KeywordGroups.Resolve(f.Keyword)

	totalQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.Resource{}), f),
		resourceSearchColumns, groups, "资源")
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := totalQ.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计资源数量失败: %w", err))
	}
	if total == 0 {
		return []model.Resource{}, 0, nil
	}

	findQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.Resource{}), f),
		resourceSearchColumns, groups, "资源")
	if err != nil {
		return nil, 0, err
	}

	items := make([]model.Resource, 0, limit)
	err = applyKeywordOrder(findQ, resourceSearchColumns, groups, resourceOrderClause(f.Sort)).
		Offset(offset).Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询资源列表失败: %w", err))
	}
	return items, total, nil
}

// Related 返回同分类下的其他已发布资源。
func (r *ResourceRepo) Related(ctx context.Context, categoryID, excludeID int64, limit int) ([]model.Resource, error) {
	if categoryID <= 0 || limit <= 0 {
		return []model.Resource{}, nil
	}
	items := make([]model.Resource, 0, limit)
	err := r.db.WithContext(ctx).
		Where("status = ? AND category_id = ? AND id <> ?", model.ResourceStatusPublished, categoryID, excludeID).
		Order("download_count DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询相关资源失败: %w", err))
	}
	return items, nil
}

// CountPublished 统计已发布资源数。
func (r *ResourceRepo) CountPublished(ctx context.Context) (int64, error) {
	return countPublished(ctx, r.db, &model.Resource{}, model.ResourceStatusPublished, "资源")
}

// DistinctFileExts 返回已发布资源的文件扩展名列表，用于前端筛选器。
func (r *ResourceRepo) DistinctFileExts(ctx context.Context) ([]string, error) {
	return distinctValues(ctx, r.db, &model.Resource{}, "file_ext", model.ResourceStatusPublished, "资源")
}

// ExtExists 判断除 excludeID 外，是否还有资源引用同一个存储 key。
//
// 用途：替换 / 删除资源时，只有当没有任何资源再引用这个文件，
// 才可以真的去存储里删掉它。否则两个资源页共用一个脚本包时，
// 删其中一个会把另一个的文件也删没。
func (r *ResourceRepo) ExtExists(ctx context.Context, fileKey string, excludeID int64) (bool, error) {
	if strings.TrimSpace(fileKey) == "" {
		return false, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Resource{}).Where("file_key = ?", fileKey)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("查询文件引用失败: %w", err))
	}
	return count > 0, nil
}

// applyFilter 把过滤条件作用到查询上。
// 关键词由 List 里的 filterByKeyword 处理（需要先探测再决定放宽与否）。
func (r *ResourceRepo) applyFilter(q *gorm.DB, f ResourceFilter) *gorm.DB {
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CategoryID > 0 {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.FileExt != "" {
		// 统一按小写比对：入库时已规范为小写，避免 .SH 与 .sh 各建一套筛选项。
		q = q.Where("file_ext = ?", strings.ToLower(strings.TrimPrefix(f.FileExt, ".")))
	}
	if tag := strings.TrimSpace(f.Tag); tag != "" {
		q = q.Where("tags LIKE ? ESCAPE '\\'", likePattern(`"`+tag+`"`))
	}
	return q
}

// resourceOrderClause 返回排序语句。
// 默认 latest 而非 download：新上传的资源需要曝光，
// 否则下载榜头部会被几个老资源长期占住，新内容永远起不来。
func resourceOrderClause(sort string) string {
	switch sort {
	case "download":
		return "download_count DESC, id DESC"
	case "hot":
		return "view_count DESC, id DESC"
	default:
		return "published_at DESC, created_at DESC, id DESC"
	}
}

// Create 创建资源。
func (r *ResourceRepo) Create(ctx context.Context, res *model.Resource) error {
	if err := r.db.WithContext(ctx).Create(res).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建资源失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新资源。
func (r *ResourceRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	return updateFieldsByID(ctx, r.db, &model.Resource{}, id, fields, "资源")
}

// Delete 软删除资源。
func (r *ResourceRepo) Delete(ctx context.Context, id int64) error {
	return softDeleteByID(ctx, r.db, &model.Resource{}, id, "资源")
}

// IncrementDownload 下载数自增。
func (r *ResourceRepo) IncrementDownload(ctx context.Context, id int64) error {
	return incrementColumn(ctx, r.db, &model.Resource{}, id, "download_count", 1, "资源下载数")
}

// IncrementView 浏览数自增。
func (r *ResourceRepo) IncrementView(ctx context.Context, id int64) error {
	return incrementColumn(ctx, r.db, &model.Resource{}, id, "view_count", 1, "资源浏览数")
}

// CountByCategoryID 统计分类下的资源数，删除分类前的前置校验用。
func (r *ResourceRepo) CountByCategoryID(ctx context.Context, categoryID int64) (int64, error) {
	return countByCategoryID(ctx, r.db, &model.Resource{}, categoryID, "资源")
}

// CountByCategory 统计各分类下的资源数。
func (r *ResourceRepo) CountByCategory(ctx context.Context) (map[int64]int64, error) {
	return countGroupByCategory(ctx, r.db, &model.Resource{}, "", "资源")
}

// ============================ 外链下载 ============================

// ListLinksByResource 返回某个资源的全部外部下载链接。
func (r *ResourceRepo) ListLinksByResource(ctx context.Context, resourceID int64) ([]model.ResourceLink, error) {
	rows := make([]model.ResourceLink, 0, 4)
	err := r.db.WithContext(ctx).
		Where("resource_id = ?", resourceID).
		Order("sort ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询资源下载链接失败: %w", err))
	}
	return rows, nil
}

// ReplaceLinks 整体替换某资源的下载链接。
//
// 与工具教程同理：链接是资源的组成部分，编辑页一次提交完整清单，
// 事务内先删后插，避免出现"删完了但插入失败"的半截状态。
func (r *ResourceRepo) ReplaceLinks(ctx context.Context, resourceID int64, links []model.ResourceLink) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_id = ?", resourceID).Delete(&model.ResourceLink{}).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("清理旧下载链接失败: %w", err))
		}
		if len(links) == 0 {
			return nil
		}
		for i := range links {
			links[i].ResourceID = resourceID
		}
		if err := tx.Create(&links).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("写入下载链接失败: %w", err))
		}
		return nil
	})
}
