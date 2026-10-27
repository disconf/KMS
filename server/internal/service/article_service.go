package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

const (
	// relatedArticleLimit 详情页相关文章条数。
	relatedArticleLimit = 5
	// slugMaxAttempts slug 冲突时最多尝试的序号后缀次数，超过则追加随机串。
	slugMaxAttempts = 20
	// autoSummaryRunes 未填摘要时自动截取的字符数。
	autoSummaryRunes = 120
	// tagListLimit 标签接口最多返回的标签数。
	tagListLimit = 50
)

// ArticleService 知识库业务逻辑：文章、分类与点赞。
type ArticleService struct {
	articles   *repository.ArticleRepo
	categories *repository.CategoryRepo
	users      *repository.UserRepo
	// points 可为 nil（测试里常不传）：积分是荣誉系统，缺了它文章照常发布。
	points *repository.PointRepo
}

// NewArticleService 创建 ArticleService。
func NewArticleService(
	articles *repository.ArticleRepo,
	categories *repository.CategoryRepo,
	users *repository.UserRepo,
	points *repository.PointRepo,
) *ArticleService {
	return &ArticleService{articles: articles, categories: categories, users: users, points: points}
}

// ============================ 前台：分类与标签 ============================

// ListCategories 返回分类列表。includeDisabled 为 true 时含停用分类（后台口径），
// 且文章数统计口径随之切换为"全部文章"。
func (s *ArticleService) ListCategories(ctx context.Context, includeDisabled bool) ([]dto.CategoryItem, error) {
	cats, err := s.categories.List(ctx, !includeDisabled)
	if err != nil {
		return nil, err
	}

	var counts map[int64]int64
	if includeDisabled {
		counts, err = s.articles.CountByCategory(ctx)
	} else {
		counts, err = s.articles.CountPublishedByCategory(ctx)
	}
	if err != nil {
		return nil, err
	}

	items := make([]dto.CategoryItem, 0, len(cats))
	for i := range cats {
		items = append(items, dto.CategoryItem{
			ID:           cats[i].ID,
			Name:         cats[i].Name,
			Slug:         cats[i].Slug,
			Icon:         cats[i].Icon,
			Sort:         cats[i].Sort,
			Enabled:      cats[i].Enabled,
			ArticleCount: counts[cats[i].ID],
		})
	}
	return items, nil
}

// ListTags 返回已发布文章中出现过的标签，按出现次数倒序。
func (s *ArticleService) ListTags(ctx context.Context) ([]dto.TagItem, error) {
	counts, err := s.articles.TagCounts(ctx, model.ArticleStatusPublished)
	if err != nil {
		return nil, err
	}

	items := make([]dto.TagItem, 0, len(counts))
	for name, count := range counts {
		items = append(items, dto.TagItem{Name: name, Count: count})
	}
	// 次数相同时按名称排序，保证同一份数据每次返回顺序一致（前端可缓存）。
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Name < items[j].Name
	})
	if len(items) > tagListLimit {
		items = items[:tagListLimit]
	}
	return items, nil
}

// ============================ 前台：文章 ============================

// ListArticles 查询已发布文章列表。
func (s *ArticleService) ListArticles(ctx context.Context, q dto.ArticleListQuery) ([]dto.ArticleListItem, int64, error) {
	channel, err := resolveArticleChannelFilter(q.Channel)
	if err != nil {
		return nil, 0, err
	}

	filter := repository.ArticleFilter{
		Status:        model.ArticleStatusPublished,
		Channel:       channel,
		KeywordGroups: SearchGroups(q.Keyword),
		Tag:           q.Tag,
		Sort:          q.Sort,
	}

	switch {
	case q.CategoryID > 0:
		filter.CategoryID = q.CategoryID
	case strings.TrimSpace(q.CategorySlug) != "":
		cat, err := s.categories.GetBySlug(ctx, strings.TrimSpace(q.CategorySlug))
		if err != nil {
			return nil, 0, err
		}
		filter.CategoryID = cat.ID
	}

	rows, total, err := s.articles.List(ctx, filter, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items, err := s.toListItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ArticleDetail 按 slug 或 ID 查询文章详情。
//
// key 为纯数字时按 ID 查，否则按 slug 查——前台 SEO 用 slug，
// 而前端从列表拿到的 ID 也能直接拼详情页，省掉一次 slug 映射。
func (s *ArticleService) ArticleDetail(ctx context.Context, key string, viewerID int64) (*dto.ArticleDetail, error) {
	article, err := s.resolveArticle(ctx, key)
	if err != nil {
		return nil, err
	}
	if !article.IsPublished() {
		return nil, errcode.ErrArticleNotPublished
	}

	// 阅读数自增失败不该让用户读不到文章，因此只记日志。
	if err := s.articles.IncrementView(ctx, article.ID); err != nil {
		logger.Warn("文章阅读数自增失败", "article_id", article.ID, "err", err.Error())
	} else {
		article.ViewCount++
	}

	base, err := s.toListItems(ctx, []model.Article{*article})
	if err != nil {
		return nil, err
	}

	related, err := s.toListItems(ctx, s.relatedRows(ctx, article))
	if err != nil {
		return nil, err
	}

	liked := false
	if viewerID > 0 {
		liked, err = s.articles.HasLike(ctx, viewerID, article.ID)
		if err != nil {
			logger.Warn("查询点赞状态失败", "article_id", article.ID, "user_id", viewerID, "err", err.Error())
			liked = false
		}
	}

	return &dto.ArticleDetail{
		ArticleListItem: base[0],
		Content:         article.Content,
		SEO:             seoOf(article),
		Source:          article.Source,
		Liked:           liked,
		Related:         related,
	}, nil
}

// ToggleLike 点赞 / 取消点赞。返回最新点赞态与点赞数，前端直接用于回显。
func (s *ArticleService) ToggleLike(ctx context.Context, userID int64, key string) (*dto.LikeResult, error) {
	article, err := s.resolveArticle(ctx, key)
	if err != nil {
		return nil, err
	}
	if !article.IsPublished() {
		return nil, errcode.ErrArticleNotPublished
	}

	liked, err := s.articles.HasLike(ctx, userID, article.ID)
	if err != nil {
		return nil, err
	}

	if liked {
		removed, err := s.articles.RemoveLike(ctx, userID, article.ID)
		if err != nil {
			return nil, err
		}
		if !removed {
			// 并发下已被其他请求删除，计数不必再减。
			return &dto.LikeResult{Liked: false, LikeCount: article.LikeCount}, nil
		}
		if err := s.articles.IncrementLike(ctx, article.ID, -1); err != nil {
			logger.Warn("点赞数自减失败", "article_id", article.ID, "err", err.Error())
		}
		return &dto.LikeResult{Liked: false, LikeCount: clampMin(article.LikeCount - 1)}, nil
	}

	created, err := s.articles.AddLike(ctx, userID, article.ID)
	if err != nil {
		return nil, err
	}
	if !created {
		// 并发重复提交：记录已存在，计数不要重复加。
		return &dto.LikeResult{Liked: true, LikeCount: article.LikeCount}, nil
	}
	if err := s.articles.IncrementLike(ctx, article.ID, 1); err != nil {
		logger.Warn("点赞数自增失败", "article_id", article.ID, "err", err.Error())
	}
	return &dto.LikeResult{Liked: true, LikeCount: article.LikeCount + 1}, nil
}

// ============================ 后台：文章 ============================

// AdminListArticles 后台文章列表，可查全部状态。
func (s *ArticleService) AdminListArticles(ctx context.Context, q dto.AdminArticleListQuery) ([]dto.AdminArticleItem, int64, error) {
	channel, err := resolveArticleChannelFilter(q.Channel)
	if err != nil {
		return nil, 0, err
	}

	rows, total, err := s.articles.List(ctx, repository.ArticleFilter{
		Status:        q.Status,
		Channel:       channel,
		CategoryID:    q.CategoryID,
		KeywordGroups: SearchGroups(q.Keyword),
		Sort:          "newest",
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}

	base, err := s.toListItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.AdminArticleItem, 0, len(base))
	for i := range base {
		items = append(items, dto.AdminArticleItem{
			ArticleListItem: base[i],
			Status:          rows[i].Status,
			AuthorID:        rows[i].AuthorID,
			// 审核痕迹要跟着列表一起下发：待审队列里两个管理员同时看同一篇时，
			// 后点的人得知道前一位已经驳回过一次，以及驳回理由是什么。
			ReviewNote: rows[i].ReviewNote,
			ReviewerID: rows[i].ReviewerID,
			ReviewedAt: rows[i].ReviewedAt,
		})
	}
	return items, total, nil
}

// AdminGetArticle 后台读取文章详情（含草稿与已下线内容）。
func (s *ArticleService) AdminGetArticle(ctx context.Context, id int64) (*dto.AdminArticleDetail, error) {
	article, err := s.articles.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toAdminDetail(ctx, article)
}

// CreateArticle 新建文章。status 省略即存草稿，避免误发。
func (s *ArticleService) CreateArticle(ctx context.Context, adminID int64, req dto.ArticleCreateReq) (*dto.AdminArticleDetail, error) {
	title, err := validateTitle(req.Title)
	if err != nil {
		return nil, err
	}
	summary, err := validateSummary(req.Summary)
	if err != nil {
		return nil, err
	}
	content, err := validateContent(req.Content)
	if err != nil {
		return nil, err
	}
	tags, err := normalizeTags(req.Tags)
	if err != nil {
		return nil, err
	}
	seo, err := validateSEO(req.SEOTitle, req.SEODescription, req.SEOKeywords)
	if err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, req.CategoryID); err != nil {
		return nil, err
	}
	channel, err := resolveArticleChannel(req.Channel)
	if err != nil {
		return nil, err
	}
	slug, err := s.resolveArticleSlug(ctx, req.Slug, title, 0)
	if err != nil {
		return nil, err
	}

	status := model.ArticleStatusDraft
	if req.Status == model.ArticleStatusPublished {
		status = model.ArticleStatusPublished
	}
	if status == model.ArticleStatusPublished && strings.TrimSpace(content) == "" {
		return nil, errcode.ErrContentInvalid.WithMsg("正文为空，无法直接发布")
	}

	article := &model.Article{
		Title:          title,
		Slug:           slug,
		Summary:        summary,
		Cover:          strings.TrimSpace(req.Cover),
		Content:        content,
		CategoryID:     req.CategoryID,
		Tags:           tags,
		Channel:        channel,
		SEOTitle:       seo.title,
		SEODescription: seo.description,
		SEOKeywords:    seo.keywords,
		Source:         model.ArticleSourceManual,
		Status:         status,
		AuthorID:       adminID,
	}
	if article.Summary == "" {
		article.Summary = autoSummary(content)
	}
	if status == model.ArticleStatusPublished {
		now := time.Now()
		article.PublishedAt = &now
	}

	if err := s.articles.Create(ctx, article); err != nil {
		return nil, err
	}
	return s.toAdminDetail(ctx, article)
}

// UpdateArticle 局部更新文章，未传字段保持不变。
func (s *ArticleService) UpdateArticle(ctx context.Context, id int64, req dto.ArticleUpdateReq) (*dto.AdminArticleDetail, error) {
	article, err := s.articles.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]any, 10)

	if req.Title != nil {
		title, err := validateTitle(*req.Title)
		if err != nil {
			return nil, err
		}
		fields["title"] = title
	}

	if req.Slug != nil {
		// 更新时把 excludeID 传进去，否则"不改 slug 只改标题"会被自己判为冲突。
		slug, err := s.resolveArticleSlug(ctx, *req.Slug, article.Title, id)
		if err != nil {
			return nil, err
		}
		fields["slug"] = slug
	}

	if req.Summary != nil {
		summary, err := validateSummary(*req.Summary)
		if err != nil {
			return nil, err
		}
		fields["summary"] = summary
	}

	if req.Cover != nil {
		cover := strings.TrimSpace(*req.Cover)
		if err := checkCoverLen(cover); err != nil {
			return nil, err
		}
		fields["cover"] = cover
	}

	if req.Content != nil {
		content, err := validateContent(*req.Content)
		if err != nil {
			return nil, err
		}
		fields["content"] = content
		// 正文变了但没单独给摘要、原摘要又是自动生成的空值，就重新生成一次。
		if req.Summary == nil && strings.TrimSpace(article.Summary) == "" {
			fields["summary"] = autoSummary(content)
		}
	}

	if req.CategoryID != nil {
		if err := s.checkCategory(ctx, *req.CategoryID); err != nil {
			return nil, err
		}
		fields["category_id"] = *req.CategoryID
	}

	if req.Tags != nil {
		tags, err := normalizeTags(*req.Tags)
		if err != nil {
			return nil, err
		}
		fields["tags"] = tags
	}

	if req.Channel != nil {
		channel, err := resolveArticleChannel(*req.Channel)
		if err != nil {
			return nil, err
		}
		fields["channel"] = channel
	}

	if req.SEOTitle != nil || req.SEODescription != nil || req.SEOKeywords != nil {
		seo, err := validateSEO(
			derefString(req.SEOTitle, article.SEOTitle),
			derefString(req.SEODescription, article.SEODescription),
			derefString(req.SEOKeywords, article.SEOKeywords),
		)
		if err != nil {
			return nil, err
		}
		fields["seo_title"] = seo.title
		fields["seo_description"] = seo.description
		fields["seo_keywords"] = seo.keywords
	}

	if len(fields) == 0 {
		return s.toAdminDetail(ctx, article)
	}
	if err := s.articles.UpdateFields(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.AdminGetArticle(ctx, id)
}

// PublishArticle 发布文章（草稿 / 已下线 → 已发布）。
func (s *ArticleService) PublishArticle(ctx context.Context, id int64) (*dto.AdminArticleDetail, error) {
	article, err := s.articles.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if article.IsPublished() {
		return nil, errcode.ErrArticleStateInvalid.WithMsg("文章已是发布状态")
	}
	if strings.TrimSpace(article.Content) == "" {
		return nil, errcode.ErrContentInvalid.WithMsg("正文为空，无法发布")
	}

	fields := map[string]any{"status": model.ArticleStatusPublished}
	if article.PublishedAt == nil {
		fields["published_at"] = time.Now()
	}
	// 发布时把缺失的 SEO 字段补齐：手工填一遍标题和标签对运营是纯负担。
	seo := seoOf(article)
	if strings.TrimSpace(article.SEOTitle) == "" {
		fields["seo_title"] = seo.Title
	}
	if strings.TrimSpace(article.SEODescription) == "" {
		fields["seo_description"] = seo.Description
	}
	if strings.TrimSpace(article.SEOKeywords) == "" {
		fields["seo_keywords"] = seo.Keywords
	}

	if err := s.articles.UpdateFields(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.AdminGetArticle(ctx, id)
}

// OfflineArticle 下线已发布文章，保留内容以便后续重新发布。
func (s *ArticleService) OfflineArticle(ctx context.Context, id int64) (*dto.AdminArticleDetail, error) {
	article, err := s.articles.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !article.IsPublished() {
		return nil, errcode.ErrArticleStateInvalid.WithMsg("仅已发布的文章可以下线")
	}
	if err := s.articles.UpdateFields(ctx, id, map[string]any{"status": model.ArticleStatusOffline}); err != nil {
		return nil, err
	}
	return s.AdminGetArticle(ctx, id)
}

// DeleteArticle 删除文章（软删除，可通过数据库恢复）。
func (s *ArticleService) DeleteArticle(ctx context.Context, id int64) error {
	if _, err := s.articles.GetByID(ctx, id); err != nil {
		return err
	}
	return s.articles.Delete(ctx, id)
}

// ============================ 后台：分类 ============================

// CreateCategory 新建分类。
func (s *ArticleService) CreateCategory(ctx context.Context, req dto.CategoryCreateReq) (*dto.CategoryItem, error) {
	name, err := validateCategoryName(req.Name)
	if err != nil {
		return nil, err
	}
	if err := checkIconLen(req.Icon); err != nil {
		return nil, err
	}

	if exists, err := s.categories.ExistsName(ctx, name, 0); err != nil {
		return nil, err
	} else if exists {
		return nil, errcode.ErrCategoryNameTaken
	}

	slug, err := s.resolveCategorySlug(ctx, req.Slug, name, 0)
	if err != nil {
		return nil, err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	cat := &model.ArticleCategory{
		Name:    name,
		Slug:    slug,
		Icon:    strings.TrimSpace(req.Icon),
		Sort:    req.Sort,
		Enabled: enabled,
	}
	if err := s.categories.Create(ctx, cat); err != nil {
		return nil, err
	}
	return &dto.CategoryItem{
		ID: cat.ID, Name: cat.Name, Slug: cat.Slug, Icon: cat.Icon,
		Sort: cat.Sort, Enabled: cat.Enabled, ArticleCount: 0,
	}, nil
}

// UpdateCategory 局部更新分类。
func (s *ArticleService) UpdateCategory(ctx context.Context, id int64, req dto.CategoryUpdateReq) (*dto.CategoryItem, error) {
	cat, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	fields := make(map[string]any, 5)

	if req.Name != nil {
		name, err := validateCategoryName(*req.Name)
		if err != nil {
			return nil, err
		}
		if exists, err := s.categories.ExistsName(ctx, name, id); err != nil {
			return nil, err
		} else if exists {
			return nil, errcode.ErrCategoryNameTaken
		}
		fields["name"] = name
	}

	if req.Slug != nil {
		slug, err := s.resolveCategorySlug(ctx, *req.Slug, derefString(req.Name, cat.Name), id)
		if err != nil {
			return nil, err
		}
		fields["slug"] = slug
	}

	if req.Icon != nil {
		if err := checkIconLen(*req.Icon); err != nil {
			return nil, err
		}
		fields["icon"] = strings.TrimSpace(*req.Icon)
	}

	if req.Sort != nil {
		fields["sort"] = *req.Sort
	}

	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
	}

	if len(fields) == 0 {
		return s.categoryItem(ctx, cat), nil
	}
	if err := s.categories.UpdateFields(ctx, id, fields); err != nil {
		return nil, err
	}

	updated, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.categoryItem(ctx, updated), nil
}

// DeleteCategory 删除分类。分类下仍有文章时拒绝删除，
// 否则那些文章会集体变成"无分类"，前台导航与 SEO 面包屑都会断掉。
func (s *ArticleService) DeleteCategory(ctx context.Context, id int64) error {
	if _, err := s.categories.GetByID(ctx, id); err != nil {
		return err
	}
	count, err := s.articles.CountByCategoryID(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return errcode.ErrCategoryInUse.WithMsg(
			fmt.Sprintf("该分类下还有 %d 篇文章，请先移出或删除文章", count))
	}
	return s.categories.Delete(ctx, id)
}

func (s *ArticleService) categoryItem(ctx context.Context, cat *model.ArticleCategory) *dto.CategoryItem {
	counts, err := s.articles.CountByCategory(ctx)
	if err != nil {
		logger.Warn("统计分类文章数失败", "category_id", cat.ID, "err", err.Error())
		counts = map[int64]int64{}
	}
	return &dto.CategoryItem{
		ID: cat.ID, Name: cat.Name, Slug: cat.Slug, Icon: cat.Icon,
		Sort: cat.Sort, Enabled: cat.Enabled, ArticleCount: counts[cat.ID],
	}
}

// ============================ 内部工具 ============================

// resolveArticle 解析文章定位串：纯数字按 ID，其余按 slug。
func (s *ArticleService) resolveArticle(ctx context.Context, key string) (*model.Article, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errcode.ErrArticleNotFound
	}
	if id, err := strconv.ParseInt(key, 10, 64); err == nil && id > 0 {
		return s.articles.GetByID(ctx, id)
	}
	return s.articles.GetBySlug(ctx, key)
}

// relatedRows 取相关文章；失败时降级为空列表，不影响详情页打开。
func (s *ArticleService) relatedRows(ctx context.Context, article *model.Article) []model.Article {
	rows, err := s.articles.Related(ctx, article.CategoryID, article.ID, relatedArticleLimit)
	if err != nil {
		logger.Warn("查询相关文章失败", "article_id", article.ID, "err", err.Error())
		return nil
	}
	return rows
}

// toListItems 把模型批量转换为列表 DTO，一次性解析分类名与作者名，避免 N+1 查询。
func (s *ArticleService) toListItems(ctx context.Context, rows []model.Article) ([]dto.ArticleListItem, error) {
	items := make([]dto.ArticleListItem, 0, len(rows))
	if len(rows) == 0 {
		return items, nil
	}

	cats, err := s.categoryIndex(ctx)
	if err != nil {
		return nil, err
	}

	authorIDs := make([]int64, 0, len(rows))
	for i := range rows {
		authorIDs = append(authorIDs, rows[i].AuthorID)
	}
	authors, err := s.users.FindByIDs(ctx, authorIDs)
	if err != nil {
		// 作者名只影响展示，查不到就把名字留空，不阻断列表返回。
		logger.Warn("批量查询文章作者失败，作者名将留空", "err", err.Error())
		authors = map[int64]*model.User{}
	}

	for i := range rows {
		items = append(items, toListItem(rows[i], cats, authors))
	}
	return items, nil
}

func (s *ArticleService) categoryIndex(ctx context.Context) (map[int64]model.ArticleCategory, error) {
	cats, err := s.categories.List(ctx, false)
	if err != nil {
		return nil, err
	}
	index := make(map[int64]model.ArticleCategory, len(cats))
	for _, c := range cats {
		index[c.ID] = c
	}
	return index, nil
}

func toListItem(a model.Article, cats map[int64]model.ArticleCategory, authors map[int64]*model.User) dto.ArticleListItem {
	item := dto.ArticleListItem{
		ID:          a.ID,
		Title:       a.Title,
		Slug:        a.Slug,
		Summary:     a.Summary,
		Cover:       a.Cover,
		Channel:     a.Channel,
		CategoryID:  a.CategoryID,
		Tags:        []string(a.Tags),
		ViewCount:   a.ViewCount,
		LikeCount:   a.LikeCount,
		PublishedAt: a.PublishedAt,
		// 资讯栏目的条目来自订阅源，列表上必须能看出「这条不是我写的」。
		SourceName: a.SourceName,
		SourceURL:  a.SourceURL,
	}
	if item.Tags == nil {
		// 输出空数组而非 null，前端可以少写一个判空分支。
		item.Tags = []string{}
	}
	if cat, ok := cats[a.CategoryID]; ok {
		item.CategoryName = cat.Name
		item.CategorySlug = cat.Slug
	}
	if u, ok := authors[a.AuthorID]; ok && u != nil {
		item.AuthorName = u.Nickname
	}
	return item
}

func (s *ArticleService) toAdminDetail(ctx context.Context, article *model.Article) (*dto.AdminArticleDetail, error) {
	base, err := s.toListItems(ctx, []model.Article{*article})
	if err != nil {
		return nil, err
	}
	return &dto.AdminArticleDetail{
		AdminArticleItem: dto.AdminArticleItem{
			ArticleListItem: base[0],
			Status:          article.Status,
			AuthorID:        article.AuthorID,
			// 审核痕迹必须回填：后台详情页要显示「谁在什么时候以什么理由驳回」，
			// 否则审核记录写进了库却永远没人看得到。
			ReviewNote: article.ReviewNote,
			ReviewerID: article.ReviewerID,
			ReviewedAt: article.ReviewedAt,
		},
		Content: article.Content,
		SEO:     seoOf(article),
		Source:  article.Source,
	}, nil
}

// seoOf 组装 SEO 字段，缺项时用标题 / 摘要 / 标签兜底，
// 保证前台永远能输出一份完整的结构化数据。
func seoOf(a *model.Article) dto.SEOInfo {
	info := dto.SEOInfo{
		Title:       strings.TrimSpace(a.SEOTitle),
		Description: strings.TrimSpace(a.SEODescription),
		Keywords:    strings.TrimSpace(a.SEOKeywords),
	}
	if info.Title == "" {
		info.Title = a.Title
	}
	if info.Description == "" {
		info.Description = a.Summary
	}
	if info.Description == "" {
		info.Description = autoSummary(a.Content)
	}
	if info.Keywords == "" {
		info.Keywords = strings.Join(a.Tags, ",")
	}
	return info
}

// resolveArticleSlug 生成唯一的文章 slug。
//
// 两条路径刻意区别对待：
//   - 显式传入 slug：冲突或缺省时直接报错。静默改写成 `-2` 会让作者以为
//     自己指定的地址生效了、其实没有——对已经对外发过的链接是灾难。
//   - 由标题推导 slug：冲突时自动避让 `-2` / `-3`，因为作者本来就没指定。
func (s *ArticleService) resolveArticleSlug(ctx context.Context, provided, title string, excludeID int64) (string, error) {
	if explicit := strings.TrimSpace(provided); explicit != "" {
		slug := sanitizeSlug(explicit)
		if slug == "" || len(slug) > dto.ArticleSlugMaxLen() {
			return "", errcode.ErrSlugInvalid.WithMsg(
				fmt.Sprintf("URL 标识只能包含字母、数字、汉字与连字符，且不超过 %d 个字符", dto.ArticleSlugMaxLen()))
		}
		taken, err := s.articles.SlugExists(ctx, slug, excludeID)
		if err != nil {
			return "", err
		}
		if taken {
			return "", errcode.ErrArticleSlugTaken.WithMsg("URL 标识 " + slug + " 已被占用，请换一个")
		}
		return slug, nil
	}

	base := truncateSlug(sanitizeSlug(title), dto.ArticleSlugMaxLen())
	if base == "" {
		base = "a" + idgen.NumericCode(10)
	}

	for i := 0; i < slugMaxAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		taken, err := s.articles.SlugExists(ctx, candidate, excludeID)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	// 极端情况（同标题被连续创建 20 次）直接挂随机后缀，保证一定能写入。
	return fmt.Sprintf("%s-%s", base, idgen.NumericCode(6)), nil
}

func (s *ArticleService) resolveCategorySlug(ctx context.Context, provided, name string, excludeID int64) (string, error) {
	if explicit := strings.TrimSpace(provided); explicit != "" {
		slug := sanitizeSlug(explicit)
		if slug == "" || len(slug) > dto.CategorySlugMaxLen() {
			return "", errcode.ErrSlugInvalid.WithMsg(
				fmt.Sprintf("分类标识只能包含字母、数字、汉字与连字符，且不超过 %d 个字符", dto.CategorySlugMaxLen()))
		}
		taken, err := s.categories.ExistsSlug(ctx, slug, excludeID)
		if err != nil {
			return "", err
		}
		if taken {
			return "", errcode.ErrCategorySlugTaken
		}
		return slug, nil
	}

	base := truncateSlug(sanitizeSlug(name), dto.CategorySlugMaxLen())
	if base == "" {
		base = "c" + idgen.NumericCode(8)
	}

	for i := 0; i < slugMaxAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		taken, err := s.categories.ExistsSlug(ctx, candidate, excludeID)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return fmt.Sprintf("%s-%s", base, idgen.NumericCode(4)), nil
}

// truncateSlug 按字节上限截断并清掉尾部残留的连字符。
//
// maxLen 是**字节**上限（对齐 MySQL varchar 的长度语义），但汉字是 3 字节，
// 直接 slug[:maxLen] 会把最后一个字劈成半个 UTF-8 序列——写库时 MySQL 直接报
// 1366 Incorrect string value。线上资讯抓取就是这么挂的：中文标题按 100 字节
// 切，恰好切在某个字的中间，于是整条资讯写不进去。
// 所以先退到字符边界再截。
func truncateSlug(slug string, maxLen int) string {
	if len(slug) <= maxLen {
		return slug
	}
	cut := maxLen
	for cut > 0 && !utf8.RuneStart(slug[cut]) {
		cut--
	}
	return strings.Trim(slug[:cut], "-")
}

// checkCategory 校验分类存在（0 表示不指定分类，允许留空）。
func (s *ArticleService) checkCategory(ctx context.Context, categoryID int64) error {
	if categoryID <= 0 {
		return nil
	}
	_, err := s.categories.GetByID(ctx, categoryID)
	return err
}

// sanitizeSlug 把任意文本规整为 URL 友好的 slug。
//
// 汉字保留：现代浏览器会把它们百分号编码，可读性远好于无意义的随机串；
// 其余字符统一折叠为单个连字符，首尾连字符裁掉，ASCII 大写转小写。
func sanitizeSlug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', unicode.Is(unicode.Han, r):
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// autoSummary 从正文自动截取摘要，去掉常见 Markdown 标记与多余空白。
func autoSummary(content string) string {
	replacer := strings.NewReplacer(
		"#", " ", "*", " ", "`", " ", ">", " ",
		"\n", " ", "\r", " ", "\t", " ", "|", " ",
	)
	text := strings.Join(strings.Fields(replacer.Replace(content)), " ")
	runes := []rune(text)
	if len(runes) > autoSummaryRunes {
		runes = runes[:autoSummaryRunes]
	}
	return string(runes)
}

func validateTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	minLen, maxLen := dto.ArticleTitleLen()
	if n := len([]rune(title)); n < minLen || n > maxLen {
		return "", errcode.ErrTitleInvalid.WithMsg(
			fmt.Sprintf("标题长度需为 %d-%d 个字符", minLen, maxLen))
	}
	return title, nil
}

func validateSummary(raw string) (string, error) {
	summary := strings.TrimSpace(raw)
	summaryMax, _, _, _, _, _ := dto.ArticleLimits()
	if len([]rune(summary)) > summaryMax {
		return "", errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("摘要不超过 %d 个字符", summaryMax))
	}
	return summary, nil
}

// validateContent 校验正文。空白正文是合法输入（允许先存半成品草稿），
// 是否允许发布由发布链路单独判断。
func validateContent(raw string) (string, error) {
	content := strings.TrimRight(raw, " \t\r\n")
	_, _, contentMax, _, _, _ := dto.ArticleLimits()
	if len([]rune(content)) > contentMax {
		return "", errcode.ErrContentInvalid.WithMsg(
			fmt.Sprintf("正文不超过 %d 个字符", contentMax))
	}
	return content, nil
}

func checkCoverLen(cover string) error {
	_, coverMax, _, _, _, _ := dto.ArticleLimits()
	if len(cover) > coverMax {
		return errcode.ErrParamInvalid.WithMsg("封面地址过长")
	}
	return nil
}

type seoFields struct {
	title, description, keywords string
}

func validateSEO(title, description, keywords string) (seoFields, error) {
	_, _, _, titleMax, descMax, kwMax := dto.ArticleLimits()
	out := seoFields{
		title:       strings.TrimSpace(title),
		description: strings.TrimSpace(description),
		keywords:    strings.TrimSpace(keywords),
	}
	switch {
	case len([]rune(out.title)) > titleMax:
		return out, errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("SEO 标题不超过 %d 个字符", titleMax))
	case len([]rune(out.description)) > descMax:
		return out, errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("SEO 描述不超过 %d 个字符", descMax))
	case len([]rune(out.keywords)) > kwMax:
		return out, errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("SEO 关键词不超过 %d 个字符", kwMax))
	}
	return out, nil
}

// normalizeTags 去空、去重（忽略大小写）、限个数与长度。
// 收敛到 normalizeTagsLimit，与工具 / 题库 / 资源共用同一套去重规则。
func normalizeTags(raw []string) (model.StringList, error) {
	maxCount, maxLen := dto.TagLimits()
	return normalizeTagsLimit(raw, maxCount, maxLen)
}

func validateCategoryName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	nameMax, _, _ := dto.CategoryLimits()
	if name == "" || len([]rune(name)) > nameMax {
		return "", errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("分类名称需为 1-%d 个字符", nameMax))
	}
	return name, nil
}

func checkIconLen(icon string) error {
	_, _, iconMax := dto.CategoryLimits()
	if len(icon) > iconMax {
		return errcode.ErrParamInvalid.WithMsg("分类图标地址过长")
	}
	return nil
}

func derefString(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

func clampMin(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

// ============================ 投稿与审核 ============================

// Contribute 用户投稿文章。
//
// 状态默认 pending、作者固定取登录态：这两个值一旦让调用方传，
// 就有了「自己发布」和「冒名投稿」两个后门，而它们恰好是最容易被利用的。
//
// directPublish 由 handler 依据「已过审专家 / 管理员」判定后传入。
// 它不是权限本身，只是把判定结果带下来的一个布尔——真正的判定只在一处，
// 免得 service 里再拼一遍角色条件、两边慢慢走岔。
func (s *ArticleService) Contribute(ctx context.Context, userID int64, req dto.ArticleContributeReq, directPublish bool) (*dto.AdminArticleDetail, error) {
	title, err := validateTitle(req.Title)
	if err != nil {
		return nil, err
	}
	summary, err := validateSummary(req.Summary)
	if err != nil {
		return nil, err
	}
	content, err := validateContent(req.Content)
	if err != nil {
		return nil, err
	}
	tags, err := normalizeTags(req.Tags)
	if err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, req.CategoryID); err != nil {
		return nil, err
	}
	channel, err := resolveArticleChannel(req.Channel)
	if err != nil {
		return nil, err
	}
	// 资讯是靠订阅源抓取 + 运营挑选的栏目，开放投稿等于让任何人都能往
	// "行业动态"里塞软文，且无法与抓取内容区分。教程类才接受投稿。
	if channel == model.ChannelOpsNews {
		return nil, errcode.ErrParamInvalid.WithMsg("运维资讯由平台运营维护，暂不开放投稿")
	}
	slug, err := s.resolveArticleSlug(ctx, "", title, 0)
	if err != nil {
		return nil, err
	}

	status := model.ArticleStatusPending
	if directPublish {
		// 已过审专家直发：他们写的内容由入驻审核与历史订单评价背书。
		// 仍然保留事后处置能力（下线/驳回），只是不必排队等审。
		status = model.ArticleStatusPublished
	}

	article := &model.Article{
		Title:      title,
		Slug:       slug,
		Summary:    summary,
		Cover:      strings.TrimSpace(req.Cover),
		Content:    content,
		CategoryID: req.CategoryID,
		Tags:       tags,
		Channel:    channel,
		Source:     model.ArticleSourceManual,
		Status:     status,
		AuthorID:   userID,
	}
	if article.Summary == "" {
		article.Summary = autoSummary(content)
	}
	if status == model.ArticleStatusPublished {
		now := time.Now()
		article.PublishedAt = &now
	}
	if err := s.articles.Create(ctx, article); err != nil {
		return nil, err
	}
	// 直发的也要记分：否则「专家直发」比「投稿过审」少 20 分，
	// 等于惩罚效率高的人。
	if status == model.ArticleStatusPublished {
		s.awardPublishPoints(ctx, article.ID, userID)
	}
	return s.toAdminDetail(ctx, article)
}

// awardPublishPoints 记「教程发布」积分。points 为 nil（测试）时静默跳过。
func (s *ArticleService) awardPublishPoints(ctx context.Context, articleID, userID int64) {
	if s.points == nil || userID <= 0 {
		return
	}
	if _, err := s.points.Award(ctx, &model.PointLog{
		UserID: userID, Action: model.PointActionArticlePass,
		Points: 20, TargetType: "article", TargetID: articleID,
		UniqKey: fmt.Sprintf("article.publish:%d", articleID),
	}); err != nil {
		logger.Warn("记录教程积分失败", "article_id", articleID, "err", err.Error())
	}
}

// CreateDraft 由后台直接建草稿（问答精选转教程等）。
//
// 与 Contribute 的区别只有两点：状态是 draft（不发 pending，因为操作者本身就是运营）、
// 作者可以是管理员。其余校验与 slug 生成完全复用——两处各写一份，
// 迟早会出现"投稿的文章 slug 会自动去重、转出来的不会"这种不对称。
func (s *ArticleService) CreateDraft(
	ctx context.Context, authorID int64,
	title, summary, content, channel string, categoryID int64,
) (*model.Article, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errcode.ErrTitleInvalid.WithMsg("请填写标题")
	}
	if n := runeLen(title); n > 200 {
		return nil, errcode.ErrTitleInvalid.WithMsg("标题不能超过 200 个字")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errcode.ErrParamInvalid.WithMsg("正文不能为空")
	}
	if err := s.checkCategory(ctx, categoryID); err != nil {
		return nil, err
	}
	if channel == "" {
		channel = model.ChannelLinuxTutorial
	}
	slug, err := s.resolveArticleSlug(ctx, "", title, 0)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(summary) == "" {
		summary = autoSummary(content)
	}

	article := &model.Article{
		Title: title, Slug: slug, Summary: summary, Content: content,
		CategoryID: categoryID, Channel: channel,
		Source: model.ArticleSourceManual, Status: model.ArticleStatusDraft,
		AuthorID: authorID,
	}
	if err := s.articles.Create(ctx, article); err != nil {
		return nil, err
	}
	return article, nil
}

// MyContributions 我的投稿列表，含审核状态与驳回原因。
func (s *ArticleService) MyContributions(ctx context.Context, userID int64, q dto.PageQuery) ([]dto.MyContributionItem, int64, error) {
	q.Normalize()
	rows, total, err := s.articles.List(ctx, repository.ArticleFilter{
		AuthorID: userID,
		Sort:     "latest",
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items := make([]dto.MyContributionItem, 0, len(rows))
	for i := range rows {
		items = append(items, dto.MyContributionItem{
			Kind:        "article",
			ID:          rows[i].ID,
			Title:       rows[i].Title,
			Slug:        rows[i].Slug,
			Channel:     rows[i].Channel,
			Status:      rows[i].Status,
			ReviewNote:  rows[i].ReviewNote,
			ReviewedAt:  rows[i].ReviewedAt,
			CreatedAt:   rows[i].CreatedAt,
			PublishedAt: rows[i].PublishedAt,
			ViewCount:   rows[i].ViewCount,
		})
	}
	return items, total, nil
}

// ReviewArticle 审核投稿：通过则发布，驳回必须留原因。
//
// 只接受 pending 状态的内容，且用条件更新（WHERE status='pending'）：
// 两个管理员同时点「通过」时，第二次会命中 0 行而不是把发布时间再刷一遍。
func (s *ArticleService) ReviewArticle(ctx context.Context, reviewerID, id int64, req dto.ArticleReviewReq) (*dto.AdminArticleDetail, error) {
	article, err := s.articles.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if article.Status != model.ArticleStatusPending {
		return nil, errcode.ErrParamInvalid.WithMsg(
			"只有待审核的内容可以审核，当前状态：" + article.StatusLabel())
	}

	note := strings.TrimSpace(req.Note)
	now := time.Now()
	fields := map[string]any{
		"reviewer_id": reviewerID,
		"reviewed_at": now,
		"review_note": truncateRunes(note, 500),
	}
	switch req.Action {
	case "approve":
		fields["status"] = model.ArticleStatusPublished
		fields["published_at"] = now
	case "reject":
		// 驳回不给原因，投稿人只能猜。这不是审核流程，是踢皮球。
		if note == "" {
			return nil, errcode.ErrParamInvalid.WithMsg("驳回需要填写原因")
		}
		fields["status"] = model.ArticleStatusRejected
	default:
		return nil, errcode.ErrParamInvalid.WithMsg("审核动作只能是 approve 或 reject")
	}

	ok, err := s.articles.UpdateFieldsIfStatus(ctx, id, model.ArticleStatusPending, fields)
	if err != nil {
		return nil, err
	}
	if !ok {
		// 命中 0 行说明审核期间已被别人处理过。必须报错而不是静默成功——
		// 否则管理员会以为自己通过了，实际稿件状态是别人改的另一个值。
		return nil, errcode.ErrParamInvalid.WithMsg("这篇内容已被其他人处理，请刷新后再看")
	}
	updated, err := s.articles.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 过审发布给作者记分。CAS 已经保证只有第一次 approve 会走到这里，
	// 积分侧还有唯一键兜底——两层都挡着，重复审核不会重复记功。
	if req.Action == "approve" {
		s.awardPublishPoints(ctx, id, article.AuthorID)
	}
	return s.toAdminDetail(ctx, updated)
}
