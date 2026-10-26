package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// ToolBookRepo 教程集数据访问：书 + 章节。
type ToolBookRepo struct {
	db *gorm.DB
}

// NewToolBookRepo 创建 ToolBookRepo。
func NewToolBookRepo(db *gorm.DB) *ToolBookRepo { return &ToolBookRepo{db: db} }

// GetBook 按主键查询教程集；不存在时返回 errcode.ErrToolBookNotFound。
func (r *ToolBookRepo) GetBook(ctx context.Context, id int64) (*model.ToolBook, error) {
	var b model.ToolBook
	err := r.db.WithContext(ctx).First(&b, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrToolBookNotFound
		}
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询教程集 %d 失败: %w", id, err))
	}
	return &b, nil
}

// BooksByTool 返回一个工具下的全部教程集（含草稿，由 service 决定是否过滤）。
//
// 后台要看到草稿与已下线的书，前台只展示已发布的——两种视图共用这一条查询，
// 免得两处条件慢慢走岔。
func (r *ToolBookRepo) BooksByTool(ctx context.Context, toolID int64) ([]model.ToolBook, error) {
	rows := make([]model.ToolBook, 0, 4)
	err := r.db.WithContext(ctx).
		Where("tool_id = ?", toolID).
		Order("sort DESC, id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询工具的教程集失败: %w", err))
	}
	return rows, nil
}

// CountBooksByTool 统计工具下的教程集数量，删除工具前的前置校验用。
func (r *ToolBookRepo) CountBooksByTool(ctx context.Context, toolID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ToolBook{}).Where("tool_id = ?", toolID).Count(&n).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计工具教程集失败: %w", err))
	}
	return n, nil
}

// ChapterRow 是章节 + 正文文章的联查结果。
//
// 不直接复用 model.ToolBookChapter：目录里一条节点要显示的是**文章标题**
// （作者改过标题，目录得跟着变），而不是当初录入时存的字符串。
// 同时把文章状态带出来，前台据此跳过死链、后台据此标红。
type ChapterRow struct {
	ID          int64
	BookID      int64
	ParentID    int64
	Title       string
	ArticleID   int64
	ExternalURL string
	Summary     string
	Sort        int

	ArticleTitle   string
	ArticleSlug    string
	ArticleStatus  string
	ArticleSummary string
}

// ChapterRows 批量取多本书的章节（已联好文章信息）。
//
// 一次查完而不是每本查一次：工具详情页可能同时展示三本书，
// 逐本查会变成 3 次往返；而「按 book_id IN (...)」一条就够，
// 章节总量本来就不大（单本上限 60 个节点）。
func (r *ToolBookRepo) ChapterRows(ctx context.Context, bookIDs []int64) ([]ChapterRow, error) {
	rows := make([]ChapterRow, 0, 16)
	if len(bookIDs) == 0 {
		return rows, nil
	}
	err := r.db.WithContext(ctx).
		Table("tool_book_chapters AS c").
		Select("c.id, c.book_id, c.parent_id, c.title, c.article_id, c.external_url, c.summary, c.sort, "+
			"COALESCE(a.title,'') AS article_title, COALESCE(a.slug,'') AS article_slug, "+
			"COALESCE(a.status,'') AS article_status, COALESCE(a.summary,'') AS article_summary").
		Joins("LEFT JOIN articles AS a ON a.id = c.article_id AND a.deleted_at IS NULL").
		Where("c.book_id IN ?", bookIDs).
		Order("c.book_id ASC, c.parent_id ASC, c.sort ASC, c.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询教程集章节失败: %w", err))
	}
	return rows, nil
}

// ArticleRefBySlug 按 slug 查文章最简引用（大纲里写的是 slug，不是 ID）。
//
// 为什么不要求运营填 ID：文章 ID 在后台任何页面上都不显示，
// 而 slug 就在文章详情页的 URL 里（#/article/nginx-install），复制一眼就成。
func (r *ToolBookRepo) ArticleRefBySlug(ctx context.Context, slug string) (*ArticleRef, error) {
	var ref ArticleRef
	err := r.db.WithContext(ctx).Model(&model.Article{}).
		Select("id, slug, title, status").
		Where("slug = ?", slug).First(&ref).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("按 slug 查询文章失败: %w", err))
	}
	return &ref, nil
}

// CreateBook 新建教程集。
func (r *ToolBookRepo) CreateBook(ctx context.Context, b *model.ToolBook) error {
	if err := r.db.WithContext(ctx).Create(b).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建教程集失败: %w", err))
	}
	return nil
}

// UpdateBookFields 按字段名局部更新教程集。
func (r *ToolBookRepo) UpdateBookFields(ctx context.Context, id int64, fields map[string]any) error {
	return updateFieldsByID(ctx, r.db, &model.ToolBook{}, id, fields, "教程集")
}

// DeleteBook 删除教程集。
//
// 书是 Base 模型（软删除），章节是纯目录数据（物理删除）：
// 章节表不嵌 Base，软删会留下永远躺在表里、每次查询都要带 deleted_at
// 条件的幽灵行——与 ToolGuide 的处理一致。
func (r *ToolBookRepo) DeleteBook(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("book_id = ?", id).Delete(&model.ToolBookChapter{}).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("删除教程集章节失败: %w", err))
		}
		if err := tx.Delete(&model.ToolBook{}, id).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("删除教程集失败: %w", err))
		}
		return nil
	})
}

// ChapterNode 是一棵两级的目录节点：一个章 + 它下面的节。
//
// 用树而不是平铺的 parentID：章节是「先删后插」整体替换的，
// 插入前子节点还不知道父节点的自增 ID——平铺结构要么让 service
// 先插章再插节（两次往返、两次错误处理），要么在 SQL 里做 ID 回填。
// 树结构把这件事压到一次事务里。
type ChapterNode struct {
	Chapter  model.ToolBookChapter
	Children []model.ToolBookChapter
}

// ReplaceChapterTree 用一棵新的目录树整体替换这本书的章节。
//
// 步骤：清旧 → 逐章插入拿 ID → 子节点挂上父 ID 再插入，全程一个事务。
// 事务的意义不只是"失败回滚"：读到的目录永远是完整的旧树或完整的新树，
// 不会出现「章已经换了、节还是上一版」的中间态。
func (r *ToolBookRepo) ReplaceChapterTree(ctx context.Context, bookID int64, nodes []ChapterNode) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("book_id = ?", bookID).Delete(&model.ToolBookChapter{}).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("清理旧章节失败: %w", err))
		}
		for i := range nodes {
			chapter := nodes[i].Chapter
			chapter.BookID = bookID
			chapter.ParentID = 0
			if err := tx.Create(&chapter).Error; err != nil {
				return errcode.ErrDatabase.Wrap(fmt.Errorf("写入章「%s」失败: %w", chapter.Title, err))
			}
			if len(nodes[i].Children) == 0 {
				continue
			}
			children := make([]model.ToolBookChapter, len(nodes[i].Children))
			copy(children, nodes[i].Children)
			for j := range children {
				children[j].BookID = bookID
				children[j].ParentID = chapter.ID
			}
			if err := tx.Create(&children).Error; err != nil {
				return errcode.ErrDatabase.Wrap(fmt.Errorf("写入章「%s」的小节失败: %w", chapter.Title, err))
			}
		}
		return nil
	})
}
