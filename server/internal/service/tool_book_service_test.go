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

type bookFixture struct {
	svc   *ToolBookService
	db    *gorm.DB
	tool  *model.Tool
	admin int64
}

func newBookFixture(t *testing.T) *bookFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "book.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	admin := &model.User{
		Phone: model.StrPtr("13700000001"), Nickname: "运营",
		Role: model.RoleAdmin, Status: model.UserStatusActive,
	}
	require.NoError(t, db.Create(admin).Error)

	tool := &model.Tool{
		Name: "Nginx", Slug: "nginx", Summary: "反向代理与 Web 服务器",
		Content: "详细介绍", Status: model.ToolStatusPublished,
	}
	require.NoError(t, db.Create(tool).Error)

	pub := &model.Article{
		Title: "Nginx 编译安装", Slug: "nginx-compile", Summary: "从源码编译",
		Content: "正文", Channel: model.ChannelLinuxTutorial, Status: model.ArticleStatusPublished,
	}
	require.NoError(t, db.Create(pub).Error)

	draft := &model.Article{
		Title: "Nginx 调优(草稿)", Slug: "nginx-tuning", Summary: "还没写完",
		Content: "正文", Channel: model.ChannelLinuxTutorial, Status: model.ArticleStatusDraft,
	}
	require.NoError(t, db.Create(draft).Error)

	return &bookFixture{
		svc:   NewToolBookService(repository.NewToolBookRepo(db), repository.NewToolRepo(db)),
		db:    db,
		tool:  tool,
		admin: admin.ID,
	}
}

// createBook 建一本空书（草稿）。
func (f *bookFixture) createBook(t *testing.T, title string) *dto.ToolBookItem {
	t.Helper()
	item, err := f.svc.CreateBook(context.Background(), f.admin, f.tool.ID, dto.ToolBookCreateReq{
		Title: title, Level: model.ToolBookLevelBeginner,
	})
	require.NoError(t, err)
	return item
}

func TestParseChapterOutlineAcceptsCommonShapes(t *testing.T) {
	raw := `# 第一章 安装
## 1.1 编译安装 | nginx-compile | 先看这里
- 1.2 目录结构 | nginx-compile
* 1.3 最小配置
# 第二章 反向代理｜全角分隔符
+ 2.1 upstream
这一行是随手写的备注
### 三级标题不要

# 第三章 排错
`
	specs, warnings := parseChapterOutline(raw)

	require.Len(t, specs, 7, "合法行应全部解析出来")
	assert.True(t, specs[0].isChapter)
	assert.Equal(t, "第一章 安装", specs[0].title)
	assert.False(t, specs[1].isChapter)
	assert.Equal(t, "1.1 编译安装", specs[1].title)
	assert.Equal(t, "nginx-compile", specs[1].ref)
	assert.Equal(t, "先看这里", specs[1].summary)
	// 减号、星号、加号都当「节」
	assert.Equal(t, "1.2 目录结构", specs[2].title)
	assert.Equal(t, "1.3 最小配置", specs[3].title)
	// 全角竖线也要能切
	assert.True(t, specs[4].isChapter)
	assert.Equal(t, "第二章 反向代理", specs[4].title)
	assert.Equal(t, "2.1 upstream", specs[5].title)

	// 两行被忽略：一行不是章节、一行是三级标题
	require.Len(t, warnings, 2)
	assert.Contains(t, warnings[0], "第 7 行")
	assert.Contains(t, warnings[1], "两级")
}

func TestSetOutlineBuildsTwoLevelTree(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	book := f.createBook(t, "Nginx 从入门到集群")

	res, err := f.svc.SetOutline(ctx, book.ID, `# 第一章 安装
## 编译安装 | nginx-compile
## 目录结构
# 第二章 调优
## 参数调优 | nginx-tuning
`)
	require.NoError(t, err)
	assert.Empty(t, res.Warnings)

	require.Len(t, res.Chapters, 2)
	assert.Equal(t, "第一章 安装", res.Chapters[0].Title)
	require.Len(t, res.Chapters[0].Children, 2)
	// 标题取文章的当前标题，不是大纲里写的「编译安装」——
	// 作者改了标题，目录必须跟着变，否则两处对不上。
	assert.Equal(t, "Nginx 编译安装", res.Chapters[0].Children[0].Title)
	assert.Equal(t, "nginx-compile", res.Chapters[0].Children[0].ArticleSlug)
	// 没有链接的节点保留标题，只是点不动
	assert.Equal(t, "目录结构", res.Chapters[0].Children[1].Title)
	assert.Empty(t, res.Chapters[0].Children[1].ArticleSlug)

	// 回读一次，确认真的落库了而不是只在返回值里
	doc, err := f.svc.Outline(ctx, book.ID)
	require.NoError(t, err)
	assert.Contains(t, doc.Outline, "# 第一章 安装")
	assert.Contains(t, doc.Outline, "## Nginx 编译安装 | nginx-compile")
	assert.Equal(t, 5, doc.Count, "2 章 + 3 节")
}

func TestBookFrontHidesUnpublishedChapterButAdminSeesIt(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	book := f.createBook(t, "Nginx 学习路径")

	_, err := f.svc.SetOutline(ctx, book.ID, `# 第一章 安装
## 编译安装 | nginx-compile
## 调优(草稿) | nginx-tuning
`)
	require.NoError(t, err)

	// 草稿书在前台不可见
	_, err = f.svc.BookDetail(ctx, book.ID)
	assert.ErrorIs(t, err, errcode.ErrToolBookNotFound, "草稿教程集不该被前台读到")

	// 发布后：挂着草稿文章的节点被跳过
	_, err = f.svc.PublishBook(ctx, book.ID)
	require.NoError(t, err)

	detail, err := f.svc.BookDetail(ctx, book.ID)
	require.NoError(t, err)
	require.Len(t, detail.Chapters, 1)
	assert.Len(t, detail.Chapters[0].Children, 1, "草稿文章那一节应被跳过")
	assert.Equal(t, "nginx-compile", detail.Chapters[0].Children[0].ArticleSlug)

	// 后台必须看得见那一条，否则运营既改不了也删不掉
	adminItem, err := f.svc.AdminBook(ctx, book.ID)
	require.NoError(t, err)
	require.Len(t, adminItem.Chapters, 1)
	require.Len(t, adminItem.Chapters[0].Children, 2)
	assert.True(t, adminItem.Chapters[0].Children[1].Broken, "失效节点要在后台标红")
}

func TestBookCannotPublishWithoutChapters(t *testing.T) {
	f := newBookFixture(t)
	book := f.createBook(t, "空目录的书")

	_, err := f.svc.PublishBook(context.Background(), book.ID)
	assert.ErrorIs(t, err, errcode.ErrToolBookStateInvalid)
	assert.Contains(t, err.Error(), "目录为空")

	// 空大纲 = 清空目录，合法但不是发布状态
	res, err := f.svc.SetOutline(context.Background(), book.ID, "  \n\n")
	require.NoError(t, err)
	assert.Zero(t, res.Count)
}

func TestBookDropsSectionBeforeAnyChapter(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	book := f.createBook(t, "先写了节再写章")

	res, err := f.svc.SetOutline(ctx, book.ID, `## 孤零零的一节
# 第一章 安装
## 编译安装 | nginx-compile
`)
	require.NoError(t, err)
	assert.Len(t, res.Chapters, 1, "章之前的节无处安放，应被丢弃")
	assert.Equal(t, 2, res.Count)
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "任何章之前")
}

func TestBookDeleteRemovesChapters(t *testing.T) {
	f := newBookFixture(t)
	ctx := context.Background()
	book := f.createBook(t, "会被删掉的书")
	_, err := f.svc.SetOutline(ctx, book.ID, "# 第一章\n## 节 | nginx-compile\n")
	require.NoError(t, err)

	require.NoError(t, f.svc.DeleteBook(ctx, book.ID))

	var n int64
	require.NoError(t, f.db.Model(&model.ToolBookChapter{}).Where("book_id = ?", book.ID).Count(&n).Error)
	assert.Zero(t, n, "章节是纯目录数据，应随书一起物理清理")

	_, err = f.svc.AdminBook(ctx, book.ID)
	assert.ErrorIs(t, err, errcode.ErrToolBookNotFound)
}
