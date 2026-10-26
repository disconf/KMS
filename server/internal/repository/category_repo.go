package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// CategoryRepo 知识库分类数据访问。
type CategoryRepo struct {
	db *gorm.DB
}

// NewCategoryRepo 创建 CategoryRepo。
func NewCategoryRepo(db *gorm.DB) *CategoryRepo { return &CategoryRepo{db: db} }

// List 返回分类列表，按 sort 升序、id 升序排列。
// onlyEnabled 为 true 时只返回启用中的分类（前台口径）。
func (r *CategoryRepo) List(ctx context.Context, onlyEnabled bool) ([]model.ArticleCategory, error) {
	q := r.db.WithContext(ctx).Model(&model.ArticleCategory{})
	if onlyEnabled {
		q = q.Where("enabled = ?", true)
	}
	items := make([]model.ArticleCategory, 0, 8)
	if err := q.Order("sort ASC, id ASC").Find(&items).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询文章分类失败: %w", err))
	}
	return items, nil
}

// GetByID 按主键查询；不存在时返回 errcode.ErrCategoryNotFound。
func (r *CategoryRepo) GetByID(ctx context.Context, id int64) (*model.ArticleCategory, error) {
	var c model.ArticleCategory
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrCategoryNotFound)
	}
	return &c, nil
}

// GetBySlug 按 slug 查询；不存在时返回 errcode.ErrCategoryNotFound。
func (r *CategoryRepo) GetBySlug(ctx context.Context, slug string) (*model.ArticleCategory, error) {
	var c model.ArticleCategory
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&c).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrCategoryNotFound)
	}
	return &c, nil
}

// ExistsName 判断分类名是否已存在（excludeID 用于更新时排除自身）。
func (r *CategoryRepo) ExistsName(ctx context.Context, name string, excludeID int64) (bool, error) {
	return r.exists(ctx, "name", name, excludeID)
}

// ExistsSlug 判断分类 slug 是否已存在。
func (r *CategoryRepo) ExistsSlug(ctx context.Context, slug string, excludeID int64) (bool, error) {
	return r.exists(ctx, "slug", slug, excludeID)
}

func (r *CategoryRepo) exists(ctx context.Context, column, value string, excludeID int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&model.ArticleCategory{}).Where(column+" = ?", value)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("校验分类 %s 唯一性失败: %w", column, err))
	}
	return count > 0, nil
}

// Create 创建分类。
func (r *CategoryRepo) Create(ctx context.Context, c *model.ArticleCategory) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建分类失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新分类。
func (r *CategoryRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.ArticleCategory{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新分类 %d 失败: %w", id, err))
	}
	return nil
}

// Delete 软删除分类。
func (r *CategoryRepo) Delete(ctx context.Context, id int64) error {
	err := r.db.WithContext(ctx).Delete(&model.ArticleCategory{}, id).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("删除分类 %d 失败: %w", id, err))
	}
	return nil
}
