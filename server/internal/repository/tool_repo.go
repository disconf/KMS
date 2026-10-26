package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// ToolFilter 是工具列表的查询条件，零值表示该维度不过滤。
type ToolFilter struct {
	// Status 精确匹配状态；空串表示不过滤（后台列表用）。
	Status string
	// CategoryID 大于 0 时按分类过滤。
	CategoryID int64
	// Keyword 对名称 / 简介 / 正文 / 标签做模糊匹配。
	// 已经做过查询理解的调用方填 KeywordGroups 即可。
	Keyword       string
	KeywordGroups KeywordGroups
	// Language / License 精确匹配，走索引列。
	Language string
	License  string
	Tag      string
	// FeaturedOnly 只看精选。
	FeaturedOnly bool
	MinStars     int64
	// Sort 取值 latest（默认）/ hot（按浏览数）/ stars / sort（人工权重）。
	Sort string
}

// ToolRepo 开源工具数据访问。
type ToolRepo struct {
	db *gorm.DB
}

// NewToolRepo 创建 ToolRepo。
func NewToolRepo(db *gorm.DB) *ToolRepo { return &ToolRepo{db: db} }

// GetByID 按主键查询；不存在时返回 errcode.ErrToolNotFound。
func (r *ToolRepo) GetByID(ctx context.Context, id int64) (*model.Tool, error) {
	var t model.Tool
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrToolNotFound)
	}
	return &t, nil
}

// GetBySlug 按 slug 查询；不存在时返回 errcode.ErrToolNotFound。
func (r *ToolRepo) GetBySlug(ctx context.Context, slug string) (*model.Tool, error) {
	var t model.Tool
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&t).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrToolNotFound)
	}
	return &t, nil
}

// SlugExists 判断 slug 是否已被占用。
func (r *ToolRepo) SlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	return existsBy(ctx, r.db, &model.Tool{}, "slug", slug, excludeID, "工具 slug")
}

// List 按条件分页查询。
func (r *ToolRepo) List(ctx context.Context, f ToolFilter, offset, limit int) ([]model.Tool, int64, error) {
	groups := f.KeywordGroups.Resolve(f.Keyword)

	totalQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.Tool{}), f),
		toolSearchColumns, groups, "工具")
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := totalQ.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计工具数量失败: %w", err))
	}
	if total == 0 {
		return []model.Tool{}, 0, nil
	}

	findQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.Tool{}), f),
		toolSearchColumns, groups, "工具")
	if err != nil {
		return nil, 0, err
	}

	items := make([]model.Tool, 0, limit)
	err = applyKeywordOrder(findQ, toolSearchColumns, groups, toolOrderClause(f.Sort)).
		Offset(offset).Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询工具列表失败: %w", err))
	}
	return items, total, nil
}

// Related 返回同分类下的其他已发布工具，用于详情页推荐位。
func (r *ToolRepo) Related(ctx context.Context, categoryID, excludeID int64, limit int) ([]model.Tool, error) {
	if categoryID <= 0 || limit <= 0 {
		return []model.Tool{}, nil
	}
	items := make([]model.Tool, 0, limit)
	err := r.db.WithContext(ctx).
		Where("status = ? AND category_id = ? AND id <> ?", model.ToolStatusPublished, categoryID, excludeID).
		Order("stars DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询相关工具失败: %w", err))
	}
	return items, nil
}

// CountPublished 统计已发布的工具数，栏目导航角标用。
func (r *ToolRepo) CountPublished(ctx context.Context) (int64, error) {
	return countPublished(ctx, r.db, &model.Tool{}, model.ToolStatusPublished, "工具")
}

// Distinct 返回某个字段的全部去重取值，用于生成筛选项（语言、协议列表）。
//
// 只取已发布数据的取值：草稿里随手填的 "golang" 不该出现在前端的筛选器里。
func (r *ToolRepo) Distinct(ctx context.Context, column string) ([]string, error) {
	return distinctValues(ctx, r.db, &model.Tool{}, column, model.ToolStatusPublished, "工具")
}

// applyFilter 把过滤条件作用到查询上，Count 与 Find 共用以保证口径一致。
// 关键词由 List 里的 filterByKeyword 处理（需要先探测再决定放宽与否）。
func (r *ToolRepo) applyFilter(q *gorm.DB, f ToolFilter) *gorm.DB {
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CategoryID > 0 {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.Language != "" {
		q = q.Where("language = ?", f.Language)
	}
	if f.License != "" {
		q = q.Where("license = ?", f.License)
	}
	if f.FeaturedOnly {
		q = q.Where("featured = ?", true)
	}
	if f.MinStars > 0 {
		q = q.Where("stars >= ?", f.MinStars)
	}
	if tag := strings.TrimSpace(f.Tag); tag != "" {
		q = q.Where("tags LIKE ? ESCAPE '\\'", likePattern(`"`+tag+`"`))
	}
	return q
}

// toolOrderClause 返回排序语句。
//
// 默认排序刻意不是"按 star 数"：star 高的工具未必适合这个平台的使用场景，
// 人工权重 sort 排前面、再按 star 兜底，运营才有办法把真正推荐的顶上去。
func toolOrderClause(sort string) string {
	switch sort {
	case "stars":
		return "stars DESC, id DESC"
	case "hot":
		return "view_count DESC, id DESC"
	case "sort":
		return "sort DESC, stars DESC, id DESC"
	default:
		return "sort DESC, published_at DESC, id DESC"
	}
}

// ============================ 写入 ============================

// Create 创建工具。
func (r *ToolRepo) Create(ctx context.Context, t *model.Tool) error {
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建工具失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新工具。
func (r *ToolRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	return updateFieldsByID(ctx, r.db, &model.Tool{}, id, fields, "工具")
}

// Delete 软删除工具。
func (r *ToolRepo) Delete(ctx context.Context, id int64) error {
	return softDeleteByID(ctx, r.db, &model.Tool{}, id, "工具")
}

// IncrementView 浏览数自增。
func (r *ToolRepo) IncrementView(ctx context.Context, id int64) error {
	return incrementColumn(ctx, r.db, &model.Tool{}, id, "view_count", 1, "工具浏览数")
}

// ============================ 四类实战教程 ============================

// ToolGuideRow 是工具教程关联 + 文章摘要信息。
//
// 用 LEFT JOIN 而不是在 service 里再查一次文章：教程条目最多十来条，
// 逐条查文章会变成 N+1；而这条查询本来就要走 tool_id 索引，多 join 一张表几乎无成本。
type ToolGuideRow struct {
	model.ToolGuide
	ArticleTitle   string
	ArticleSlug    string
	ArticleSummary string
}

// ListGuidesByTool 返回某个工具的全部教程关联，按类型与排序权重排列。
func (r *ToolRepo) ListGuidesByTool(ctx context.Context, toolID int64) ([]ToolGuideRow, error) {
	rows := make([]ToolGuideRow, 0, 8)
	err := r.db.WithContext(ctx).
		Table("tool_guides AS g").
		Select("g.id, g.tool_id, g.kind, g.article_id, g.title, g.url, g.sort, "+
			"a.title AS article_title, a.slug AS article_slug, a.summary AS article_summary").
		Joins("LEFT JOIN articles AS a ON a.id = g.article_id AND a.deleted_at IS NULL").
		Where("g.tool_id = ?", toolID).
		Order("g.kind ASC, g.sort ASC, g.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询工具教程失败: %w", err))
	}
	return rows, nil
}

// ReplaceGuides 整体替换某个工具的教程关联。
//
// 整体替换而不是逐条增删：教程是工具的「配置」而不是独立内容，
// 后台编辑页一次性提交完整的四类清单，逐条比对反而要处理一堆
// "先增后删同一个 id" 的边界。放在事务里，避免删完插入失败留下空清单。
func (r *ToolRepo) ReplaceGuides(ctx context.Context, toolID int64, guides []model.ToolGuide) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tool_id = ?", toolID).Delete(&model.ToolGuide{}).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("清理旧教程关联失败: %w", err))
		}
		if len(guides) == 0 {
			return nil
		}
		for i := range guides {
			guides[i].ToolID = toolID
		}
		if err := tx.Create(&guides).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("写入教程关联失败: %w", err))
		}
		return nil
	})
}

// CountGuidesByTool 批量统计各工具的教程条数，用于列表页显示「含 N 篇教程」。
func (r *ToolRepo) CountGuidesByTool(ctx context.Context, toolIDs []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(toolIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ToolID int64
		Count  int64
	}
	err := r.db.WithContext(ctx).Model(&model.ToolGuide{}).
		Select("tool_id AS tool_id, COUNT(*) AS count").
		Where("tool_id IN ?", toolIDs).
		Group("tool_id").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计工具教程数失败: %w", err))
	}
	for _, row := range rows {
		out[row.ToolID] = row.Count
	}
	return out, nil
}

// ArticleRef 是站内文章的最简引用，用于校验教程关联。
type ArticleRef struct {
	ID     int64
	Slug   string
	Title  string
	Status string
}

// GetArticleRef 按 ID 查文章的 slug / 标题 / 状态。
//
// 放在工具仓储里是为了避免 service 之间互相依赖：
// ToolService 只需要「这篇文章存不存在、发布没有」，
// 为此注入整个 ArticleService 会把两个模块绑死。
func (r *ToolRepo) GetArticleRef(ctx context.Context, id int64) (*ArticleRef, error) {
	var ref ArticleRef
	err := r.db.WithContext(ctx).Model(&model.Article{}).
		Select("id, slug, title, status").
		Where("id = ?", id).First(&ref).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &ref, nil
}
