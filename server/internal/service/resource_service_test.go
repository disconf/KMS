package service

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/storage"
	"github.com/opsexpert/platform/internal/repository"
)

type resourceFixture struct {
	svc      *ResourceService
	uploads  *UploadService
	store    storage.Storage
	db       *gorm.DB
	adminID  int64
	userID   int64
	catTools int64
}

func newResourceFixture(t *testing.T) *resourceFixture {
	t.Helper()

	cfg, err := config.Load("")
	require.NoError(t, err, "加载默认配置失败")

	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "res.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err, "初始化测试数据库失败")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	store, err := storage.New(storage.Options{
		Driver:   "local",
		LocalDir: t.TempDir(),
		BaseURL:  "http://localhost:8080/files",
	})
	require.NoError(t, err, "初始化本地存储失败")

	uploads := NewUploadService(store, "test-sign-secret", 50)

	ctx := context.Background()
	users := repository.NewUserRepo(db)

	admin := &model.User{
		Phone: model.StrPtr("13700000001"), Nickname: "运营",
		Role: model.RoleAdmin, Status: model.UserStatusActive,
	}
	require.NoError(t, users.Create(ctx, admin))

	user := &model.User{
		Phone: model.StrPtr("13700000002"), Nickname: "下载的人",
		Role: model.RoleUser, Status: model.UserStatusActive,
	}
	require.NoError(t, users.Create(ctx, user))

	cats := repository.NewCategoryRepo(db)
	cat := &model.ArticleCategory{Name: "运维脚本", Slug: "ops-scripts", Sort: 10, Enabled: true}
	require.NoError(t, cats.Create(ctx, cat))

	return &resourceFixture{
		svc:      NewResourceService(repository.NewResourceRepo(db), cats, uploads),
		uploads:  uploads,
		store:    store,
		db:       db,
		adminID:  admin.ID,
		userID:   user.ID,
		catTools: cat.ID,
	}
}

// putResourceFile 往存储里写一个真实的资源文件，返回它的 key。
// 走的是生产同一个 policy，因此 key 前缀与场景校验行为完全一致。
func (f *resourceFixture) putResourceFile(t *testing.T, content string) string {
	t.Helper()
	policy, ok := storage.LookupPolicy(storage.SceneResourceFile)
	require.True(t, ok, "资源上传场景未注册")

	key := policy.BuildKey("text/plain", idgen.NumericCode(16))
	body := []byte(content)
	require.NoError(t, f.store.Save(context.Background(), key, bytes.NewReader(body), int64(len(body))))
	return key
}

// putForeignFile 写一个"别的场景"的文件，用于验证前缀校验。
func (f *resourceFixture) putForeignFile(t *testing.T, content string) string {
	t.Helper()
	key := "cert/202510/" + idgen.NumericCode(16) + ".txt"
	body := []byte(content)
	require.NoError(t, f.store.Save(context.Background(), key, bytes.NewReader(body), int64(len(body))))
	return key
}

// fileExists 判断存储里是否还有这个 key。
// 用 Open + 立即 Close，别在测试里漏掉句柄——Windows 上未关闭的句柄会让
// t.TempDir 的清理直接失败。
func (f *resourceFixture) fileExists(t *testing.T, key string) bool {
	t.Helper()
	rc, err := f.store.Open(context.Background(), key)
	if err != nil {
		return false
	}
	_ = rc.Close()
	return true
}

func TestResourceCreateMeasuresFileFromDiskNotFromRequest(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	body := "#!/bin/bash\necho nginx check\n"
	key := f.putResourceFile(t, body)

	item, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title:      "Nginx 巡检脚本",
		FileKey:    key,
		FileName:   `C:\tmp\nginx-check.sh`, // 带路径，且带真实扩展名
		Version:    "v1.0.0",
		CategoryID: f.catTools,
		Tags:       []string{"nginx", "脚本"},
		Status:     model.ResourceStatusPublished,
	})
	require.NoError(t, err)

	// 大小与校验和必须来自落盘内容实测，不能采信请求里的值。
	assert.Equal(t, int64(len(body)), item.FileSize, "文件大小应实测")
	assert.Len(t, item.Checksum, 64, "SHA256 十六进制应为 64 字符")
	assert.Equal(t, "nginx-check.sh", item.FileName, "路径成分已被剥掉")
	assert.Equal(t, "sh", item.FileExt, "扩展名取用户原始文件名的，不是存储 key 的 .txt")
	assert.Equal(t, "nginx-巡检脚本", item.Slug, "slug 由标题推导，保留中文、空格转连字符")
	assert.NotNil(t, item.PublishedAt)
}

func TestResourceCreateRejectsFileFromAnotherScene(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	// 资质证书是私有场景。若允许把它挂成公开资源，等于用一个表单字段
	// 绕过了整个私有文件隔离机制。
	foreign := f.putForeignFile(t, "身份证扫描件")
	_, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title:   "偷渡的证书",
		FileKey: foreign,
	})
	assert.ErrorIs(t, err, errcode.ErrResourceFileInvalid)
	assert.Contains(t, err.Error(), "resource_file", "报错要告诉运营该用哪个场景重传")
}

func TestResourceCreateRejectsMissingOrEmptyFile(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	_, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{Title: "没传文件"})
	assert.ErrorIs(t, err, errcode.ErrResourceFileInvalid)

	policy, _ := storage.LookupPolicy(storage.SceneResourceFile)
	ghost := policy.BuildKey("text/plain", idgen.NumericCode(16))
	_, err = f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{Title: "文件不存在", FileKey: ghost})
	assert.ErrorIs(t, err, errcode.ErrResourceFileInvalid)

	empty := f.putResourceFile(t, "")
	_, err = f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{Title: "空文件", FileKey: empty})
	assert.ErrorIs(t, err, errcode.ErrResourceFileInvalid)
}

// createPublishedResource 建一条已发布资源并返回它。
func (f *resourceFixture) createPublishedResource(t *testing.T, title string, needLogin bool) *dto.AdminResourceDetail {
	t.Helper()
	key := f.putResourceFile(t, "#!/bin/bash\necho "+title+"\n")
	item, err := f.svc.Create(context.Background(), f.adminID, dto.ResourceCreateReq{
		Title:     title,
		FileKey:   key,
		FileName:  strings.ReplaceAll(title, " ", "-") + ".sh",
		NeedLogin: needLogin,
		Status:    model.ResourceStatusPublished,
	})
	require.NoError(t, err, "创建资源失败")
	return item
}

func TestResourceDownloadRequiresLoginOnlyWhenFlagged(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	open := f.createPublishedResource(t, "免登录脚本", false)
	locked := f.createPublishedResource(t, "需登录脚本", true)

	// 免登录：游客也能拿到签名地址
	dl, err := f.svc.Download(ctx, open.Slug, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, dl.URL)
	assert.NotZero(t, dl.ExpireAt, "私有文件必须带有效期")
	assert.Equal(t, "免登录脚本.sh", dl.FileName)

	// 需登录：游客被拒
	_, err = f.svc.Download(ctx, locked.Slug, 0)
	assert.ErrorIs(t, err, errcode.ErrResourceLoginNeeded)

	// 登录后放行
	dl, err = f.svc.Download(ctx, locked.Slug, f.userID)
	require.NoError(t, err)
	assert.NotEmpty(t, dl.URL)
	// 私有文件下发的是签名地址，不是裸的静态路径。
	assert.Contains(t, dl.URL, "key=")
	assert.Contains(t, dl.URL, "e=", "地址应带过期时间")
	assert.Contains(t, dl.URL, "s=", "地址应带签名")
}

func TestResourceDownloadIncrementsCountAndHidesDraft(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	item := f.createPublishedResource(t, "计数脚本", false)
	for i := 0; i < 3; i++ {
		_, err := f.svc.Download(ctx, item.Slug, f.userID)
		require.NoError(t, err)
	}

	detail, err := f.svc.ResourceDetail(ctx, item.Slug)
	require.NoError(t, err)
	assert.Equal(t, int64(3), detail.DownloadCount)

	// 草稿：前台详情与下载都不可见
	key := f.putResourceFile(t, "draft")
	draft, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title: "草稿资源", FileKey: key, FileName: "draft.sh",
	})
	require.NoError(t, err)
	_, err = f.svc.ResourceDetail(ctx, draft.Slug)
	assert.ErrorIs(t, err, errcode.ErrResourceNotPublished)
	_, err = f.svc.Download(ctx, draft.Slug, f.userID)
	assert.ErrorIs(t, err, errcode.ErrResourceNotPublished)

	_, total, err := f.svc.ListResources(ctx, dto.ResourceListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total, "草稿不该出现在前台列表")
}

func TestResourceDeleteKeepsFileStillReferencedByOthers(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	// 两个资源页共用同一个脚本包：这在运营里很常见（不同标题、同一下载内容）。
	shared := f.putResourceFile(t, "#!/bin/bash\necho shared\n")
	first, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title: "共用文件 A", FileKey: shared, FileName: "shared.sh",
		Status: model.ResourceStatusPublished,
	})
	require.NoError(t, err)
	second, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title: "共用文件 B", FileKey: shared, FileName: "shared.sh",
		Status: model.ResourceStatusPublished,
	})
	require.NoError(t, err)

	require.NoError(t, f.svc.Delete(ctx, first.ID))

	// 删掉第一个之后，文件必须还在——不能把第二个的资源页弄成死链。
	assert.True(t, f.fileExists(t, shared), "共用文件被误删了")

	// 删掉最后一个引用者，文件才被回收
	require.NoError(t, f.svc.Delete(ctx, second.ID))
	assert.False(t, f.fileExists(t, shared), "没有引用者后文件应被回收")
}

func TestResourceUpdateReplacesFileAndRemeasures(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	item := f.createPublishedResource(t, "待替换", false)
	assert.Equal(t, int64(len("#!/bin/bash\necho 待替换\n")), item.FileSize)

	newBody := "#!/bin/bash\necho 换了一份更长的内容\n"
	newKey := f.putResourceFile(t, newBody)

	updated, err := f.svc.Update(ctx, item.ID, dto.ResourceUpdateReq{
		FileKey:  &newKey,
		FileName: strPtr("replaced.sh"),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(len(newBody)), updated.FileSize, "换文件后必须重新实测大小")
	assert.NotEqual(t, item.Checksum, updated.Checksum)
	assert.Equal(t, "replaced.sh", updated.FileName)
	assert.Equal(t, "sh", updated.FileExt)
	assert.True(t, f.fileExists(t, newKey), "新文件应可读取")

	// 已知行为：Update 换文件时**不回收**旧文件，与 Delete 不同。
	// 这是保守的一侧——回收风险低但有风险（旧 key 可能还被别处引用），
	// 而留下孤儿文件的代价只是占空间，可以靠离线清理兜底。
	// 这里把这个行为钉住，避免哪天被无意改动。
	assert.True(t, f.fileExists(t, item.FileKey),
		"Update 不回收旧文件；若将来改成会回收，请同步确认已加引用检查")
}

func TestResourceFileExtsAggregatesPublishedOnly(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	f.createPublishedResource(t, "脚本一", false)

	conf := f.putResourceFile(t, "server { listen 80; }")
	_, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title: "配置模板", FileKey: conf, FileName: "nginx.conf",
		Status: model.ResourceStatusPublished,
	})
	require.NoError(t, err)

	// 草稿的扩展名不该出现在筛选器里
	zipDraft := f.putResourceFile(t, "draft")
	_, err = f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title: "草稿包", FileKey: zipDraft, FileName: "bundle.zip",
	})
	require.NoError(t, err)

	exts := f.svc.FileExts(ctx)
	assert.ElementsMatch(t, []string{"sh", "conf"}, exts)
	assert.NotContains(t, exts, "zip")
}

func TestResourcePublishOfflineFlow(t *testing.T) {
	f := newResourceFixture(t)
	ctx := context.Background()

	key := f.putResourceFile(t, "x")
	draft, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title: "状态流转", FileKey: key, FileName: "a.sh",
	})
	require.NoError(t, err)

	_, err = f.svc.Publish(ctx, draft.ID)
	require.NoError(t, err)
	_, err = f.svc.Publish(ctx, draft.ID)
	assert.ErrorIs(t, err, errcode.ErrResourceStateInvalid, "重复发布应被拒")

	_, err = f.svc.Offline(ctx, draft.ID)
	require.NoError(t, err)
	_, err = f.svc.Download(ctx, draft.Slug, f.userID)
	assert.ErrorIs(t, err, errcode.ErrResourceNotPublished, "下线后不能再下载")
}

func TestResourceOpenReturnsStoredBytes(t *testing.T) {
	// 这条是给签名下载链路兜底的：签名地址最终要能取回真实文件内容。
	f := newResourceFixture(t)
	ctx := context.Background()

	body := "#!/bin/bash\n# 运维巡检\nset -euo pipefail\n"
	key := f.putResourceFile(t, body)
	item, err := f.svc.Create(ctx, f.adminID, dto.ResourceCreateReq{
		Title: "内容校验", FileKey: key, FileName: "inspect.sh",
		Status: model.ResourceStatusPublished,
	})
	require.NoError(t, err)

	rc, err := f.store.Open(ctx, item.FileKey)
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}
