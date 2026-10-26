package service

import (
	"context"
	"path/filepath"
	"testing"

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

type toolFixture struct {
	svc       *ToolService
	db        *gorm.DB
	adminID   int64
	catObs    int64
	catDeploy int64
}

func newToolFixture(t *testing.T) *toolFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err, "加载默认配置失败")

	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "tool.db")
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
		Phone: model.StrPtr("13600000001"), Nickname: "运营",
		Role: model.RoleAdmin, Status: model.UserStatusActive,
	}
	require.NoError(t, users.Create(ctx, admin))

	cats := repository.NewCategoryRepo(db)
	observe := &model.ArticleCategory{Name: "可观测性", Slug: "observability", Sort: 10, Enabled: true}
	require.NoError(t, cats.Create(ctx, observe))
	deploy := &model.ArticleCategory{Name: "自动化与脚本", Slug: "automation", Sort: 20, Enabled: true}
	require.NoError(t, cats.Create(ctx, deploy))

	return &toolFixture{
		svc: NewToolService(repository.NewToolRepo(db), cats), db: db,
		adminID: admin.ID, catObs: observe.ID, catDeploy: deploy.ID,
	}
}

// createTool 录入一个工具。不传的字段走默认值，减少各用例的样板代码。
//
// 会自动补一段详细介绍：工具只有草稿能空着正文，一旦要发布就必须有正文
// （与文章同一条规则），否则用例里每个都要手写一段占位文字。
func (f *toolFixture) createTool(t *testing.T, req dto.ToolCreateReq) *dto.AdminToolDetail {
	t.Helper()
	if req.Content == "" {
		req.Content = "## " + req.Name + "\n\n这是一段用于测试的详细介绍。"
	}
	item, err := f.svc.Create(context.Background(), f.adminID, req)
	require.NoError(t, err, "录入工具失败")
	return item
}

func TestToolCreateRejectsNonHTTPURL(t *testing.T) {
	f := newToolFixture(t)

	// 运营录入的链接会被渲染到前台详情页，允许 javascript: 就是开了个存储型 XSS 的口子。
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,<script>", "file:///etc/passwd", "github.com/foo"} {
		t.Run(bad, func(t *testing.T) {
			_, err := f.svc.Create(context.Background(), f.adminID, dto.ToolCreateReq{
				Name: "恶意工具", RepoURL: bad,
			})
			assert.ErrorIs(t, err, errcode.ErrToolURLInvalid)
		})
	}

	// 官网地址同样校验
	_, err := f.svc.Create(context.Background(), f.adminID, dto.ToolCreateReq{
		Name: "恶意官网", HomepageURL: "javascript:void(0)",
	})
	assert.ErrorIs(t, err, errcode.ErrToolURLInvalid)
}

func TestToolCreateRejectsUnknownDeployMode(t *testing.T) {
	f := newToolFixture(t)

	_, err := f.svc.Create(context.Background(), f.adminID, dto.ToolCreateReq{
		Name: "写错部署方式的工具", DeployModes: []string{"k8s"},
	})
	assert.ErrorIs(t, err, errcode.ErrToolDeployMode)
	assert.Contains(t, err.Error(), "k8s-operator",
		"报错只说不支持还不够，应让运营看到该填什么")

	// 白名单内的取值会被转小写并去重
	item := f.createTool(t, dto.ToolCreateReq{
		Name: "正常工具", DeployModes: []string{"Docker", " HELM ", "docker"},
		Status: model.ToolStatusPublished,
	})
	assert.Equal(t, []string{"docker", "helm"}, []string(item.DeployModes))
}

func TestToolCreateRejectsNegativeStars(t *testing.T) {
	f := newToolFixture(t)
	_, err := f.svc.Create(context.Background(), f.adminID, dto.ToolCreateReq{
		Name: "负 Star", Stars: -1,
	})
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)
}

func TestToolDraftIsInvisibleAndDetailBySlugOrID(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	draft := f.createTool(t, dto.ToolCreateReq{
		Name: "草稿工具", Summary: "还没写完", CategoryID: f.catObs,
		Status: model.ToolStatusDraft,
	})

	// 草稿：前台列表看不到
	_, total, err := f.svc.ListTools(ctx, dto.ToolListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)

	// 草稿：详情用 slug 与数字 ID 两条路都拿不到，避免通过 ID 枚举泄露
	_, err = f.svc.ToolDetail(ctx, draft.Slug)
	assert.ErrorIs(t, err, errcode.ErrToolNotPublished)
	_, err = f.svc.ToolDetail(ctx, "1")
	assert.ErrorIs(t, err, errcode.ErrToolNotPublished)

	_, err = f.svc.Publish(ctx, draft.ID)
	require.NoError(t, err)

	bySlug, err := f.svc.ToolDetail(ctx, draft.Slug)
	require.NoError(t, err)
	assert.Equal(t, "草稿工具", bySlug.Name)

	byID, err := f.svc.ToolDetail(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, bySlug.ID, byID.ID, "slug 与 ID 应指向同一条记录")
}

func TestToolDetailIncrementsViewCount(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	item := f.createTool(t, dto.ToolCreateReq{
		Name: "计数工具", Status: model.ToolStatusPublished,
	})

	first, err := f.svc.ToolDetail(ctx, item.Slug)
	require.NoError(t, err)
	assert.Equal(t, int64(1), first.ViewCount)

	second, err := f.svc.ToolDetail(ctx, item.Slug)
	require.NoError(t, err)
	assert.Equal(t, int64(2), second.ViewCount)
}

func TestToolListFiltersByLanguageLicenseAndStars(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	base := func(name, lang, license string, stars int64) dto.ToolCreateReq {
		return dto.ToolCreateReq{
			Name: name, Language: lang, License: license, Stars: stars,
			RepoURL: "https://github.com/example/" + name,
			Content: "## " + name + "\n\n用于测试筛选的详细介绍。",
			Status:  model.ToolStatusPublished,
		}
	}
	f.createTool(t, base("Prometheus", "Go", "Apache-2.0", 56000))
	f.createTool(t, base("Grafana", "TypeScript", "AGPL-3.0", 62000))
	f.createTool(t, base("小工具", "Go", "MIT", 120))

	// 语言精确匹配
	items, total, err := f.svc.ListTools(ctx, dto.ToolListQuery{Language: "Go"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 2)

	// 协议精确匹配
	_, total, err = f.svc.ListTools(ctx, dto.ToolListQuery{License: "MIT"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)

	// Star 下限
	items, total, err = f.svc.ListTools(ctx, dto.ToolListQuery{MinStars: 10000})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	// 按 Star 降序
	items, _, err = f.svc.ListTools(ctx, dto.ToolListQuery{Sort: "stars"})
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "Grafana", items[0].Name)

	// 关键词命中名称
	_, total, err = f.svc.ListTools(ctx, dto.ToolListQuery{Keyword: "Graf"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

func TestToolFiltersAggregatesFromPublishedOnly(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	f.createTool(t, dto.ToolCreateReq{
		Name: "已发布", Language: "Go", License: "Apache-2.0",
		Content: "已发布工具的正文。",
		Status:  model.ToolStatusPublished,
	})
	f.createTool(t, dto.ToolCreateReq{
		Name: "草稿", Language: "Rust", License: "MIT",
		Content: "草稿工具的正文。",
		Status:  model.ToolStatusDraft,
	})

	out := f.svc.Filters(ctx)
	// 草稿的语言不该出现在筛选器里，否则会出现"选了却一个工具都没有"的死选项。
	assert.Equal(t, []string{"Go"}, out["languages"])
	assert.Equal(t, []string{"Apache-2.0"}, out["licenses"])
	assert.Equal(t, dto.ToolDeployModes(), out["deployModes"], "部署方式是白名单，与数据无关")
}

func TestToolPublishOfflineFlow(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	item := f.createTool(t, dto.ToolCreateReq{Name: "状态流转工具"})

	_, err := f.svc.Publish(ctx, item.ID)
	require.NoError(t, err)
	_, err = f.svc.Publish(ctx, item.ID)
	assert.ErrorIs(t, err, errcode.ErrToolStateInvalid, "重复发布应被拒")

	_, err = f.svc.Offline(ctx, item.ID)
	require.NoError(t, err)
	_, err = f.svc.ToolDetail(ctx, item.Slug)
	assert.ErrorIs(t, err, errcode.ErrToolNotPublished, "下线后前台不可见")

	_, err = f.svc.Publish(ctx, item.ID)
	require.NoError(t, err, "下线是可逆的")
}

func TestToolSlugIsDerivedAndDeduplicated(t *testing.T) {
	f := newToolFixture(t)

	first := f.createTool(t, dto.ToolCreateReq{Name: "Nginx Exporter"})
	assert.Equal(t, "nginx-exporter", first.Slug)

	second := f.createTool(t, dto.ToolCreateReq{Name: "Nginx Exporter"})
	assert.NotEqual(t, first.Slug, second.Slug, "同名工具应自动避让而不是撞唯一索引")
	assert.Contains(t, second.Slug, "nginx-exporter")
}

func TestToolUpdateKeepsUntouchedFields(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	item := f.createTool(t, dto.ToolCreateReq{
		Name: "局部更新", Summary: "原始简介", Language: "Go",
		CategoryID: f.catObs, Status: model.ToolStatusPublished,
	})

	updated, err := f.svc.Update(ctx, item.ID, dto.ToolUpdateReq{
		Summary: strPtr("改过的简介"),
	})
	require.NoError(t, err)
	assert.Equal(t, "改过的简介", updated.Summary)
	// 没传的字段必须原样保留
	assert.Equal(t, "局部更新", updated.Name)
	assert.Equal(t, "Go", updated.Language)
	assert.Equal(t, f.catObs, updated.CategoryID)

	// 传了非法链接一样要拦
	_, err = f.svc.Update(ctx, item.ID, dto.ToolUpdateReq{
		RepoURL: strPtr("javascript:alert(1)"),
	})
	assert.ErrorIs(t, err, errcode.ErrToolURLInvalid)
}

func TestToolCreateRejectsUnknownCategory(t *testing.T) {
	f := newToolFixture(t)
	_, err := f.svc.Create(context.Background(), f.adminID, dto.ToolCreateReq{
		Name: "挂到不存在的分类", CategoryID: 99999,
	})
	assert.ErrorIs(t, err, errcode.ErrCategoryNotFound)
}
