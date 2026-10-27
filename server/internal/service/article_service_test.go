package service

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

type articleFixture struct {
	svc      *ArticleService
	db       *gorm.DB
	adminID  int64
	viewerID int64
	catK8s   int64
	catDB    int64
}

// newArticleFixture 用临时 SQLite 搭建真实的 repository → service 链路。
func newArticleFixture(t *testing.T) *articleFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err, "加载默认配置失败")

	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "kb.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err, "初始化测试数据库失败")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	ctx := context.Background()
	users := repository.NewUserRepo(db)

	admin := &model.User{
		Phone: model.StrPtr("13800000001"), Nickname: "平台管理员",
		Role: model.RoleAdmin, Status: model.UserStatusActive,
	}
	require.NoError(t, users.Create(ctx, admin))

	viewer := &model.User{
		Phone: model.StrPtr("13800000002"), Nickname: "老王",
		Role: model.RoleUser, Status: model.UserStatusActive,
	}
	require.NoError(t, users.Create(ctx, viewer))

	svc := NewArticleService(repository.NewArticleRepo(db), repository.NewCategoryRepo(db), users, nil)

	k8s, err := svc.CreateCategory(ctx, dto.CategoryCreateReq{Name: "K8s 与云原生", Slug: "k8s", Sort: 10})
	require.NoError(t, err)
	dbCat, err := svc.CreateCategory(ctx, dto.CategoryCreateReq{Name: "数据库", Slug: "database", Sort: 20})
	require.NoError(t, err)

	return &articleFixture{
		svc: svc, db: db,
		adminID: admin.ID, viewerID: viewer.ID,
		catK8s: k8s.ID, catDB: dbCat.ID,
	}
}

// createPublished 建一篇已发布文章，减少各用例的样板代码。
func (f *articleFixture) createPublished(t *testing.T, title, slug string, categoryID int64, tags []string) *dto.AdminArticleDetail {
	t.Helper()
	detail, err := f.svc.CreateArticle(context.Background(), f.adminID, dto.ArticleCreateReq{
		Title:      title,
		Slug:       slug,
		Content:    "## 排查步骤\n\n先看状态，再看日志。",
		CategoryID: categoryID,
		Tags:       tags,
		Status:     model.ArticleStatusPublished,
	})
	require.NoError(t, err)
	return detail
}

// setViewCount 直接改库设置阅读数，用于验证排序。
func (f *articleFixture) setViewCount(t *testing.T, id, views int64) {
	t.Helper()
	err := f.db.Model(&model.Article{}).Where("id = ?", id).UpdateColumn("view_count", views).Error
	require.NoError(t, err)
}

// ============================ slug 生成 ============================

func TestSlugIsAutoDerivedAndDeduplicated(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	first, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "K8s Pod 频繁重启排查", Content: "正文", CategoryID: f.catK8s,
	})
	require.NoError(t, err)
	assert.Equal(t, "k8s-pod-频繁重启排查", first.Slug)

	// 同标题再建一篇：slug 必须自动避让，否则唯一索引会直接报错。
	second, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "K8s Pod 频繁重启排查", Content: "正文", CategoryID: f.catK8s,
	})
	require.NoError(t, err)
	assert.Equal(t, "k8s-pod-频繁重启排查-2", second.Slug)
	assert.NotEqual(t, first.Slug, second.Slug)
}

func TestSlugRejectsUnusableExplicitValue(t *testing.T) {
	f := newArticleFixture(t)

	_, err := f.svc.CreateArticle(context.Background(), f.adminID, dto.ArticleCreateReq{
		Title: "正常标题", Slug: "!!!???", Content: "正文",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrSlugInvalid)
}

func TestExplicitSlugConflictIsRejectedNotSilentlyRenamed(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()
	f.createPublished(t, "已有文章", "taken-slug", f.catK8s, nil)

	// 作者显式指定了一个已被占用的 slug：必须报错，不能悄悄改成 taken-slug-2。
	_, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "新文章", Slug: "taken-slug", Content: "正文", CategoryID: f.catK8s,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrArticleSlugTaken)

	// 反之，由标题推导出的 slug 撞名时应当自动避让（上一条用例已覆盖），
	// 这里再确认一次：不改标题、不传 slug 也能正常入库。
	derived, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "已有文章", Content: "正文", CategoryID: f.catK8s,
	})
	require.NoError(t, err)
	assert.NotEqual(t, "taken-slug", derived.Slug)
}

func TestUpdateWithOwnSlugDoesNotConflictWithItself(t *testing.T) {
	f := newArticleFixture(t)
	article := f.createPublished(t, "自冲突校验", "self-conflict", f.catK8s, nil)

	// 只改标题、把原 slug 原样回传，不能被自己判为冲突。
	same := article.Slug
	_, err := f.svc.UpdateArticle(context.Background(), article.ID, dto.ArticleUpdateReq{
		Title: strPtr("自冲突校验（修订）"),
		Slug:  &same,
	})
	require.NoError(t, err)
}

// ============================ 发布与下线 ============================

func TestDraftIsInvisibleUntilPublished(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	draft, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "还没写完的草稿", Slug: "draft-only", Content: "半成品", CategoryID: f.catK8s,
	})
	require.NoError(t, err)
	assert.Equal(t, model.ArticleStatusDraft, draft.Status)
	assert.Nil(t, draft.PublishedAt)

	// 前台按 ID 直取草稿也必须被挡掉，否则草稿内容会被 ID 枚举泄露。
	_, err = f.svc.ArticleDetail(ctx, "draft-only", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrArticleNotPublished)

	_, err = f.svc.ArticleDetail(ctx, strconv.FormatInt(draft.ID, 10), 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrArticleNotPublished)
}

func TestPublishThenOfflineFlow(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	draft, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "发布流程验证", Slug: "publish-flow", Content: "## 正文\n内容",
		CategoryID: f.catK8s, Tags: []string{"Kubernetes", "发布"},
	})
	require.NoError(t, err)

	published, err := f.svc.PublishArticle(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, model.ArticleStatusPublished, published.Status)
	assert.NotNil(t, published.PublishedAt)
	// 发布时应自动补齐 SEO 字段，避免运营手填。
	assert.Equal(t, "发布流程验证", published.SEO.Title)
	assert.Equal(t, "Kubernetes,发布", published.SEO.Keywords)

	// 重复发布要被状态机拦下。
	_, err = f.svc.PublishArticle(ctx, draft.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrArticleStateInvalid)

	offlined, err := f.svc.OfflineArticle(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, model.ArticleStatusOffline, offlined.Status)

	// 下线后前台不可见，但后台仍能读到内容。
	_, err = f.svc.ArticleDetail(ctx, "publish-flow", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrArticleNotPublished)

	_, err = f.svc.PublishArticle(ctx, draft.ID)
	require.NoError(t, err, "已下线文章应可重新发布")
}

func TestPublishRejectsEmptyContent(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	draft, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "正文为空的草稿", Slug: "empty-content",
	})
	require.NoError(t, err)

	_, err = f.svc.PublishArticle(ctx, draft.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrContentInvalid)
}

// ============================ 点赞 ============================

func TestToggleLikeIsPerUserAndIdempotent(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()
	article := f.createPublished(t, "点赞测试", "like-test", f.catK8s, nil)

	first, err := f.svc.ToggleLike(ctx, f.viewerID, "like-test")
	require.NoError(t, err)
	assert.True(t, first.Liked)
	assert.EqualValues(t, 1, first.LikeCount)

	// 同一用户再点一次是取消，而不是加一。
	second, err := f.svc.ToggleLike(ctx, f.viewerID, "like-test")
	require.NoError(t, err)
	assert.False(t, second.Liked)
	assert.EqualValues(t, 0, second.LikeCount)

	// 取消后再点赞：记录必须真的写回去。
	// 这是最容易被漏掉的一条——若点赞表带软删除，被删的行仍占着唯一索引，
	// 再次插入会静默失败，出现"接口返回已点赞、详情页却是未点赞"的诡异状态。
	third, err := f.svc.ToggleLike(ctx, f.viewerID, "like-test")
	require.NoError(t, err)
	assert.True(t, third.Liked)
	assert.EqualValues(t, 1, third.LikeCount)

	liked, err := f.svc.articles.HasLike(ctx, f.viewerID, article.ID)
	require.NoError(t, err)
	assert.True(t, liked, "取消后再点赞必须真的落库")

	// 另一个用户点赞，计数变 2。
	fourth, err := f.svc.ToggleLike(ctx, f.adminID, "like-test")
	require.NoError(t, err)
	assert.EqualValues(t, 2, fourth.LikeCount)
}

func TestArticleDetailReportsViewerLikedState(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()
	f.createPublished(t, "点赞态回显", "liked-state", f.catK8s, nil)

	_, err := f.svc.ToggleLike(ctx, f.viewerID, "liked-state")
	require.NoError(t, err)

	forViewer, err := f.svc.ArticleDetail(ctx, "liked-state", f.viewerID)
	require.NoError(t, err)
	assert.True(t, forViewer.Liked)

	forGuest, err := f.svc.ArticleDetail(ctx, "liked-state", 0)
	require.NoError(t, err)
	assert.False(t, forGuest.Liked)
}

func TestArticleDetailIncrementsViewCount(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()
	article := f.createPublished(t, "阅读数自增", "view-count", f.catK8s, nil)

	first, err := f.svc.ArticleDetail(ctx, "view-count", 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1, first.ViewCount, "首次访问应把阅读数推到 1")

	second, err := f.svc.ArticleDetail(ctx, "view-count", 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, second.ViewCount)

	var stored model.Article
	require.NoError(t, f.db.First(&stored, article.ID).Error)
	assert.EqualValues(t, 2, stored.ViewCount)
}

// ============================ 列表与搜索 ============================

func TestListArticlesFilterByTagAndKeyword(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	f.createPublished(t, "MySQL 主从延迟排查", "mysql-lag", f.catDB, []string{"MySQL", "主从复制"})
	f.createPublished(t, "Redis 内存告警", "redis-memory", f.catDB, []string{"Redis"})
	f.createPublished(t, "K8s Pod 重启", "k8s-restart", f.catK8s, []string{"Kubernetes"})

	byTag, total, err := f.svc.ListArticles(ctx, dto.ArticleListQuery{Tag: "MySQL"})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, "mysql-lag", byTag[0].Slug)

	byKeyword, total, err := f.svc.ListArticles(ctx, dto.ArticleListQuery{Keyword: "重启"})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, "k8s-restart", byKeyword[0].Slug)

	byCategory, catTotal, err := f.svc.ListArticles(ctx, dto.ArticleListQuery{CategorySlug: "database"})
	require.NoError(t, err)
	assert.EqualValues(t, 2, catTotal)
	assert.Len(t, byCategory, 2)

	// 标签前缀不能误命中："MySQL" 不应匹配到 "MySQL8"。
	f.createPublished(t, "MySQL8 新特性", "mysql8-new", f.catDB, []string{"MySQL8"})
	byTag, total, err = f.svc.ListArticles(ctx, dto.ArticleListQuery{Tag: "MySQL"})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "精确标签匹配不应把 MySQL8 算进来")
	assert.Equal(t, "mysql-lag", byTag[0].Slug)
}

func TestListArticlesSortByHot(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	cold := f.createPublished(t, "冷门文章", "cold-one", f.catK8s, nil)
	warm := f.createPublished(t, "中等文章", "warm-one", f.catK8s, nil)
	hot := f.createPublished(t, "热门文章", "hot-one", f.catK8s, nil)

	f.setViewCount(t, cold.ID, 10)
	f.setViewCount(t, warm.ID, 500)
	f.setViewCount(t, hot.ID, 9000)

	items, total, err := f.svc.ListArticles(ctx, dto.ArticleListQuery{Sort: "hot"})
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	assert.Equal(t, "hot-one", items[0].Slug)
	assert.Equal(t, "warm-one", items[1].Slug)
	assert.Equal(t, "cold-one", items[2].Slug)
}

func TestListTagsAggregatesPublishedOnly(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	f.createPublished(t, "文章一", "tag-a", f.catK8s, []string{"Kubernetes", "网络"})
	f.createPublished(t, "文章二", "tag-b", f.catK8s, []string{"Kubernetes"})
	// 草稿的标签不该出现在前台标签墙里。
	_, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "草稿文章", Slug: "tag-draft", Content: "正文", Tags: []string{"草稿标签"},
	})
	require.NoError(t, err)

	tags, err := f.svc.ListTags(ctx)
	require.NoError(t, err)
	require.Len(t, tags, 2)
	assert.Equal(t, "Kubernetes", tags[0].Name, "出现次数多的标签应排前面")
	assert.EqualValues(t, 2, tags[0].Count)
	assert.Equal(t, "网络", tags[1].Name)
}

// ============================ 分类 ============================

func TestCategoryCountsOnlyPublishedForFrontend(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	f.createPublished(t, "已发布文章", "cat-published", f.catK8s, nil)
	_, err := f.svc.CreateArticle(ctx, f.adminID, dto.ArticleCreateReq{
		Title: "草稿文章", Slug: "cat-draft", Content: "正文", CategoryID: f.catK8s,
	})
	require.NoError(t, err)

	front, err := f.svc.ListCategories(ctx, false)
	require.NoError(t, err)
	for _, c := range front {
		if c.ID == f.catK8s {
			assert.EqualValues(t, 1, c.ArticleCount, "前台只统计已发布")
		}
	}

	back, err := f.svc.ListCategories(ctx, true)
	require.NoError(t, err)
	for _, c := range back {
		if c.ID == f.catK8s {
			assert.EqualValues(t, 2, c.ArticleCount, "后台统计全部状态")
		}
	}
}

func TestDeleteCategoryIsBlockedWhileItHasArticles(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	f.createPublished(t, "占用分类的文章", "cat-in-use", f.catK8s, nil)

	err := f.svc.DeleteCategory(ctx, f.catK8s)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrCategoryInUse)

	// 空分类可以删。
	empty, err := f.svc.CreateCategory(ctx, dto.CategoryCreateReq{Name: "临时分类", Slug: "temp-cat"})
	require.NoError(t, err)
	require.NoError(t, f.svc.DeleteCategory(ctx, empty.ID))
}

func TestCreateCategoryRejectsDuplicateNameAndSlug(t *testing.T) {
	f := newArticleFixture(t)
	ctx := context.Background()

	_, err := f.svc.CreateCategory(ctx, dto.CategoryCreateReq{Name: "K8s 与云原生", Slug: "another-slug"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrCategoryNameTaken)

	_, err = f.svc.CreateCategory(ctx, dto.CategoryCreateReq{Name: "全新分类", Slug: "k8s"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrCategorySlugTaken)
}

func TestCategorySlugIsDerivedFromNameWhenOmitted(t *testing.T) {
	f := newArticleFixture(t)

	cat, err := f.svc.CreateCategory(context.Background(), dto.CategoryCreateReq{Name: "可观测性"})
	require.NoError(t, err)
	assert.Equal(t, "可观测性", cat.Slug)
	assert.True(t, cat.Enabled, "未指定时默认启用")
}

// ============================ 纯函数 ============================

func TestSanitizeSlug(t *testing.T) {
	cases := map[string]string{
		"K8s Pod 频繁重启":        "k8s-pod-频繁重启",
		"  --Hello   World--": "hello-world",
		"a/b/c":               "a-b-c",
		"!!!":                 "",
		"":                    "",
		"MySQL8.0 升级":         "mysql8-0-升级",
	}
	for in, want := range cases {
		assert.Equal(t, want, sanitizeSlug(in), "输入 %q", in)
	}
}

func TestNormalizeTagsDedupesAndLimits(t *testing.T) {
	tags, err := normalizeTags([]string{"Kubernetes", " kubernetes ", "", "K8s"})
	require.NoError(t, err)
	assert.Equal(t, []string{"Kubernetes", "K8s"}, []string(tags), "忽略大小写去重并保留首次出现的写法")

	_, maxLen := dto.TagLimits()
	_, err = normalizeTags([]string{strings.Repeat("长", maxLen+1)})
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTagInvalid)

	tooMany := make([]string, 0, 11)
	for i := 0; i < 11; i++ {
		tooMany = append(tooMany, "标签"+strconv.Itoa(i))
	}
	_, err = normalizeTags(tooMany)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTagInvalid)
}

func TestAutoSummaryStripsMarkdownAndTruncates(t *testing.T) {
	summary := autoSummary("# 标题\n\n**粗体** 内容\n")
	assert.Equal(t, "标题 粗体 内容", summary)

	assert.Len(t, []rune(autoSummary(strings.Repeat("字", autoSummaryRunes+50))), autoSummaryRunes)
}

func TestValidateTitleRejectsTooShortAndTooLong(t *testing.T) {
	_, err := validateTitle("短")
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTitleInvalid)

	_, maxLen := dto.ArticleTitleLen()
	_, err = validateTitle(strings.Repeat("长", maxLen+1))
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTitleInvalid)
}

// TestTruncateSlugNeverSplitsRune 回归：按字节截断必须落在字符边界上。
//
// 线上真实故障：资讯抓取用 100 字节截中文标题，恰好切在某个汉字中间，
// 写库时 MySQL 报 1366 Incorrect string value，整条资讯静默丢失。
func TestTruncateSlugNeverSplitsRune(t *testing.T) {
	cases := []string{
		strings.Repeat("汉", 40), // 120 字节，任意切点都可能劈字
		"K8s 集群节点 NotReady 排查思路与实战经验分享记录与复盘",      // 中英混排
		"开源中国资讯：某某项目发布 3.0 版本，带来全新的调度器与可观测能力改进说明", // 真实标题形状
	}
	for _, in := range cases {
		got := truncateSlug(in, 100)
		assert.True(t, utf8.ValidString(got), "截断结果必须是合法 UTF-8：%q", got)
		assert.LessOrEqual(t, len(got), 100, "不得超过字节上限")
		assert.NotEmpty(t, got)
		assert.False(t, strings.HasSuffix(got, "-"), "尾部连字符应被清掉")
	}

	// 中英混排时，切点前的半个汉字被整字丢弃，剩下的内容仍可读。
	got := truncateSlug(strings.Repeat("汉", 34)+"abc", 100)
	assert.Equal(t, 99, len(got), "102 字节截到 100 后回退一个汉字，应为 99 字节")
}

// ============================ 测试辅助 ============================

func strPtr(s string) *string { return &s }
