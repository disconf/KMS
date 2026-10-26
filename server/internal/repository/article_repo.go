package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// ArticleFilter 是文章列表的查询条件，零值表示该维度不过滤。
type ArticleFilter struct {
	// Status 精确匹配文章状态；空串表示不过滤（后台列表用）。
	Status string
	// Channel 精确匹配所属栏目（linux-tutorial / ops-news）。
	Channel string
	// CategoryID 大于 0 时按分类过滤。
	CategoryID int64
	// Keyword 对标题 / 摘要 / 正文 / 标签做模糊匹配。
	//
	// 已经是同义词分组的调用方只需填 KeywordGroups；这里保留 Keyword 是给
	// 没做查询理解的路径兜底（见 KeywordGroups.Resolve）。
	Keyword string
	// KeywordGroups 是展开后的检索词组，由 service 的查询理解层产出。
	KeywordGroups KeywordGroups
	// Tag 精确匹配单个标签。
	Tag string
	// AuthorID 大于 0 时只查该用户的内容，用于「我的投稿」。
	AuthorID int64
	// Sort 取值 latest（默认，按发布时间倒序）/ hot（按阅读数）/ like（按点赞数）。
	Sort string
}

// ArticleRepo 知识库文章与点赞数据访问。
type ArticleRepo struct {
	db *gorm.DB
}

// NewArticleRepo 创建 ArticleRepo。
func NewArticleRepo(db *gorm.DB) *ArticleRepo { return &ArticleRepo{db: db} }

// ============================ 单篇查询 ============================

// GetByID 按主键查询；不存在时返回 errcode.ErrArticleNotFound。
func (r *ArticleRepo) GetByID(ctx context.Context, id int64) (*model.Article, error) {
	var a model.Article
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrArticleNotFound)
	}
	return &a, nil
}

// GetBySlug 按 slug 查询；不存在时返回 errcode.ErrArticleNotFound。
func (r *ArticleRepo) GetBySlug(ctx context.Context, slug string) (*model.Article, error) {
	var a model.Article
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&a).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrArticleNotFound)
	}
	return &a, nil
}

// SlugExists 判断 slug 是否已被占用（excludeID 用于更新时排除自身）。
func (r *ArticleRepo) SlugExists(ctx context.Context, slug string, excludeID int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&model.Article{}).Where("slug = ?", slug)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("校验文章 slug 失败: %w", err))
	}
	return count > 0, nil
}

// ============================ 列表 ============================

// List 按条件分页查询，返回结果与总数。
func (r *ArticleRepo) List(ctx context.Context, f ArticleFilter, offset, limit int) ([]model.Article, int64, error) {
	groups := f.KeywordGroups.Resolve(f.Keyword)

	totalQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.Article{}), f),
		articleSearchColumns, groups, "文章")
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := totalQ.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计文章数量失败: %w", err))
	}
	if total == 0 {
		return []model.Article{}, 0, nil
	}

	// Count 与 Find 用同一套条件，口径必须一致——否则会出现
	// "显示有 12 条但只能翻到 3 条"这种让人怀疑数据丢了的现象。
	findQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.Article{}), f),
		articleSearchColumns, groups, "文章")
	if err != nil {
		return nil, 0, err
	}

	items := make([]model.Article, 0, limit)
	err = applyKeywordOrder(findQ, articleSearchColumns, groups, orderClause(f.Sort)).
		Offset(offset).Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询文章列表失败: %w", err))
	}
	return items, total, nil
}

// Related 返回同分类下的其他已发布文章，按阅读数倒序，用于详情页推荐位。
func (r *ArticleRepo) Related(ctx context.Context, categoryID, excludeID int64, limit int) ([]model.Article, error) {
	if categoryID <= 0 || limit <= 0 {
		return []model.Article{}, nil
	}
	items := make([]model.Article, 0, limit)
	err := r.db.WithContext(ctx).
		Where("status = ? AND category_id = ? AND id <> ?", model.ArticleStatusPublished, categoryID, excludeID).
		Order("view_count DESC, id DESC").
		Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询相关文章失败: %w", err))
	}
	return items, nil
}

// applyFilter 把过滤条件作用到查询上，List 的 Count 与 Find 共用它以保证口径一致。
//
// 关键词不在这里处理：它需要先探一次严格模式的总量再决定放宽与否，
// 见 List 里的 filterByKeyword。
func (r *ArticleRepo) applyFilter(q *gorm.DB, f ArticleFilter) *gorm.DB {
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Channel != "" {
		// channel 在 MySQL 与 SQLite 中都不是保留字，可直接作为列名使用。
		q = q.Where("channel = ?", f.Channel)
	}
	if f.CategoryID > 0 {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.AuthorID > 0 {
		q = q.Where("author_id = ?", f.AuthorID)
	}
	if tag := strings.TrimSpace(f.Tag); tag != "" {
		// tags 以 JSON 数组文本存储，匹配带引号的完整标签，避免 "MySQL" 命中 "MySQL8"。
		q = q.Where("tags LIKE ? ESCAPE '\\'", likePattern(`"`+tag+`"`))
	}
	return q
}

// orderClause 返回排序语句。所有排序都追加 id 作为末级排序键，
// 否则同一时间发布/同样阅读数的文章在翻页时顺序不稳定，会出现重复或漏项。
func orderClause(sort string) string {
	switch sort {
	case "hot":
		return "view_count DESC, id DESC"
	case "like":
		return "like_count DESC, id DESC"
	case "newest", "":
		// published_at 为空（草稿）时按创建时间兜底，后台列表也能正常排序。
		return "published_at DESC, created_at DESC, id DESC"
	default:
		return "published_at DESC, created_at DESC, id DESC"
	}
}

// ============================ 写入 ============================

// Create 创建文章。
func (r *ArticleRepo) Create(ctx context.Context, a *model.Article) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建文章失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新文章。
func (r *ArticleRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Article{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新文章 %d 失败: %w", id, err))
	}
	return nil
}

// UpdateFieldsIfStatus 按「流转前状态」条件更新，返回是否命中。
//
// 审核是并发场景：两个管理员同时点「通过」，若只做无条件更新，
// 第二次会把 published_at 再刷一遍（发布时间漂移），甚至把刚被驳回的稿件改回发布。
// WHERE status=? 让第二次命中 0 行，由调用方据此回一句「已被其他人处理」。
func (r *ArticleRepo) UpdateFieldsIfStatus(ctx context.Context, id int64, fromStatus string, fields map[string]any) (bool, error) {
	if len(fields) == 0 {
		return false, nil
	}
	res := r.db.WithContext(ctx).Model(&model.Article{}).
		Where("id = ? AND status = ?", id, fromStatus).
		Updates(fields)
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("更新文章 %d 状态失败: %w", id, res.Error))
	}
	return res.RowsAffected > 0, nil
}

// Delete 软删除文章。
func (r *ArticleRepo) Delete(ctx context.Context, id int64) error {
	err := r.db.WithContext(ctx).Delete(&model.Article{}, id).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("删除文章 %d 失败: %w", id, err))
	}
	return nil
}

// IncrementView 阅读数自增。用 SQL 表达式而非"读出再写回"，避免并发下丢计数。
func (r *ArticleRepo) IncrementView(ctx context.Context, id int64) error {
	err := r.db.WithContext(ctx).Model(&model.Article{}).Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新文章阅读数失败: %w", err))
	}
	return nil
}

// IncrementLike 点赞数增减，下限钳制为 0，防止取消点赞并发时出现负数。
func (r *ArticleRepo) IncrementLike(ctx context.Context, id int64, delta int) error {
	err := r.db.WithContext(ctx).Model(&model.Article{}).Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr(
			"CASE WHEN like_count + ? < 0 THEN 0 ELSE like_count + ? END", delta, delta)).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新文章点赞数失败: %w", err))
	}
	return nil
}

// ============================ 聚合统计 ============================

// CountByCategory 统计各分类下的文章数（含草稿，用于分类管理页展示总量）。
func (r *ArticleRepo) CountByCategory(ctx context.Context) (map[int64]int64, error) {
	var rows []struct {
		CategoryID int64
		Total      int64
	}
	err := r.db.WithContext(ctx).Model(&model.Article{}).
		Select("category_id, COUNT(*) AS total").
		Group("category_id").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计分类文章数失败: %w", err))
	}
	counts := make(map[int64]int64, len(rows))
	for _, row := range rows {
		counts[row.CategoryID] = row.Total
	}
	return counts, nil
}

// CountPublishedByCategory 统计各分类下"已发布"的文章数，前台分类导航用它。
func (r *ArticleRepo) CountPublishedByCategory(ctx context.Context) (map[int64]int64, error) {
	var rows []struct {
		CategoryID int64
		Total      int64
	}
	err := r.db.WithContext(ctx).Model(&model.Article{}).
		Select("category_id, COUNT(*) AS total").
		Where("status = ?", model.ArticleStatusPublished).
		Group("category_id").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计分类已发布文章数失败: %w", err))
	}
	counts := make(map[int64]int64, len(rows))
	for _, row := range rows {
		counts[row.CategoryID] = row.Total
	}
	return counts, nil
}

// CountByCategoryID 统计指定分类下的文章数，删除分类前的前置校验用。
func (r *ArticleRepo) CountByCategoryID(ctx context.Context, categoryID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Article{}).
		Where("category_id = ?", categoryID).Count(&count).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计分类 %d 文章数失败: %w", categoryID, err))
	}
	return count, nil
}

// CountPublishedByChannel 统计各栏目的已发布文章数，栏目导航角标用。
func (r *ArticleRepo) CountPublishedByChannel(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		Channel string
		Total   int64
	}
	err := r.db.WithContext(ctx).Model(&model.Article{}).
		Select("channel, COUNT(*) AS total").
		Where("status = ?", model.ArticleStatusPublished).
		Group("channel").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计栏目文章数失败: %w", err))
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Channel] = row.Total
	}
	return counts, nil
}

// TagCounts 聚合标签出现次数。
//
// 标签以 JSON 数组存在文章行内，因此这里把原始 JSON 取回来在内存里统计。
// 数据量级（万级文章）下开销可接受；若后续文章数上十万，
// 应改为独立的 article_tags 关联表或搜索服务，届时只需替换本方法实现。
func (r *ArticleRepo) TagCounts(ctx context.Context, status string) (map[string]int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Article{}).Select("tags")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var raws []string
	if err := q.Pluck("tags", &raws).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询文章标签失败: %w", err))
	}

	counts := make(map[string]int64)
	for _, raw := range raws {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			// 单条脏数据不应让整个标签页挂掉，跳过即可。
			continue
		}
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				counts[tag]++
			}
		}
	}
	return counts, nil
}

// ============================ 点赞 ============================

// HasLike 判断用户是否已点赞该文章。
func (r *ArticleRepo) HasLike(ctx context.Context, userID, articleID int64) (bool, error) {
	if userID <= 0 || articleID <= 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ArticleLike{}).
		Where("user_id = ? AND article_id = ?", userID, articleID).Count(&count).Error
	if err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("查询点赞状态失败: %w", err))
	}
	return count > 0, nil
}

// AddLike 写入点赞记录，返回"本次是否真的新增"。
//
// 依赖唯一索引 + OnConflict DoNothing 实现幂等：并发或重复点击时只有一次
// 会返回 created=true，调用方据此决定要不要给计数加一，从根上避免刷计数。
func (r *ArticleRepo) AddLike(ctx context.Context, userID, articleID int64) (bool, error) {
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.ArticleLike{UserID: userID, ArticleID: articleID})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("写入点赞记录失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// RemoveLike 删除点赞记录，返回"本次是否真的删除了"。
func (r *ArticleRepo) RemoveLike(ctx context.Context, userID, articleID int64) (bool, error) {
	res := r.db.WithContext(ctx).
		Where("user_id = ? AND article_id = ?", userID, articleID).
		Delete(&model.ArticleLike{})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("删除点赞记录失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// likePattern 构造 LIKE 模式串，转义通配符。
// 不转义的话，用户搜索 "100%" 会因为 % 变成通配符而命中全部文章。
func likePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return "%" + s + "%"
}
