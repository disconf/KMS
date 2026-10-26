package service

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

const (
	// relatedResourceLimit 详情页推荐位条数。
	relatedResourceLimit = 6
	// maxLinksPerResource 一个资源最多几条外链。
	// 官网 + 国内镜像 + 包管理器 + 容器镜像，四条基本够；再多说明该拆成多个资源。
	maxLinksPerResource = 8
	// resourceScene 资源文件必须来自这个上传场景。
	resourceScene = "resource_file"
	// fileNameMaxLen 展示用文件名的长度上限。
	fileNameMaxLen = 255
	// fileExtMaxLen 扩展名长度上限。
	fileExtMaxLen = 16
)

// ResourceService 资源下载业务逻辑。
type ResourceService struct {
	resources  *repository.ResourceRepo
	categories *repository.CategoryRepo
	uploads    *UploadService
}

// NewResourceService 创建 ResourceService。
func NewResourceService(
	resources *repository.ResourceRepo,
	categories *repository.CategoryRepo,
	uploads *UploadService,
) *ResourceService {
	return &ResourceService{resources: resources, categories: categories, uploads: uploads}
}

// ============================ 前台 ============================

// ListResources 查询已发布资源列表。
func (s *ResourceService) ListResources(ctx context.Context, q dto.ResourceListQuery) ([]dto.ResourceItem, int64, error) {
	filter := repository.ResourceFilter{
		Status:        model.ResourceStatusPublished,
		CategoryID:    q.CategoryID,
		KeywordGroups: SearchGroups(q.Keyword),
		Tag:           q.Tag,
		FileExt:       q.FileExt,
		Sort:          q.Sort,
	}
	if filter.CategoryID <= 0 && strings.TrimSpace(q.CategorySlug) != "" {
		cat, err := s.categories.GetBySlug(ctx, strings.TrimSpace(q.CategorySlug))
		if err != nil {
			return nil, 0, err
		}
		filter.CategoryID = cat.ID
	}

	rows, total, err := s.resources.List(ctx, filter, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items, err := s.toItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ResourceDetail 按 slug 或 ID 查询资源详情。
func (s *ResourceService) ResourceDetail(ctx context.Context, key string) (*dto.ResourceDetail, error) {
	res, err := s.resolveResource(ctx, key)
	if err != nil {
		return nil, err
	}
	if !res.IsPublished() {
		return nil, errcode.ErrResourceNotPublished
	}

	if err := s.resources.IncrementView(ctx, res.ID); err != nil {
		logger.Warn("资源浏览数自增失败", "resource_id", res.ID, "err", err.Error())
	} else {
		res.ViewCount++
	}

	items, err := s.toItems(ctx, []model.Resource{*res})
	if err != nil {
		return nil, err
	}

	related, _ := s.relatedItems(ctx, res)
	links := make([]dto.ResourceLinkItem, 0, 3)
	if rows, err := s.resources.ListLinksByResource(ctx, res.ID); err != nil {
		// 链接读不出来不该让详情页 500：托管文件与使用说明才是主体。
		logger.Warn("读取资源下载链接失败", "resource_id", res.ID, "err", err.Error())
	} else {
		for _, row := range rows {
			links = append(links, dto.ResourceLinkItem{
				ID: row.ID, Label: row.Label, URL: row.URL,
				Kind: row.Kind, Note: row.Note, Sort: row.Sort,
			})
		}
	}

	return &dto.ResourceDetail{
		ResourceItem: items[0],
		Content:      res.Content,
		Links:        links,
		Related:      related,
	}, nil
}

// SetLinks 整体替换资源的外部下载链接。
func (s *ResourceService) SetLinks(ctx context.Context, resourceID int64, req dto.ResourceLinksReq) ([]dto.ResourceLinkItem, error) {
	if _, err := s.resources.GetByID(ctx, resourceID); err != nil {
		return nil, err
	}
	if len(req.Links) > maxLinksPerResource {
		return nil, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("一个资源最多 %d 条下载链接", maxLinksPerResource))
	}

	prepared := make([]model.ResourceLink, 0, len(req.Links))
	for i, l := range req.Links {
		url, err := normalizeURL(l.URL, 512, "下载链接")
		if err != nil {
			return nil, errcode.ErrParamInvalid.WithMsg(
				"第 " + strconv.Itoa(i+1) + " 条链接不合法，只接受 http/https")
		}
		if url == "" {
			return nil, errcode.ErrParamInvalid.WithMsg(
				"第 " + strconv.Itoa(i+1) + " 条链接不能为空")
		}
		label := strings.TrimSpace(l.Label)
		if label == "" {
			// 标签留空时用地址兜底，总比列表里出现一行空白可点区域强。
			label = url
		}
		prepared = append(prepared, model.ResourceLink{
			Label: truncateRunes(label, 120),
			URL:   url,
			Kind:  normalizeLinkKind(l.Kind),
			Note:  truncateRunes(strings.TrimSpace(l.Note), 255),
			Sort:  l.Sort,
		})
	}

	if err := s.resources.ReplaceLinks(ctx, resourceID, prepared); err != nil {
		return nil, err
	}
	return s.linksOf(ctx, resourceID)
}

// LinksForAdmin 后台读取链接清单。
func (s *ResourceService) LinksForAdmin(ctx context.Context, resourceID int64) ([]dto.ResourceLinkItem, error) {
	if _, err := s.resources.GetByID(ctx, resourceID); err != nil {
		return nil, err
	}
	return s.linksOf(ctx, resourceID)
}

func (s *ResourceService) linksOf(ctx context.Context, resourceID int64) ([]dto.ResourceLinkItem, error) {
	rows, err := s.resources.ListLinksByResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ResourceLinkItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.ResourceLinkItem{
			ID: row.ID, Label: row.Label, URL: row.URL,
			Kind: row.Kind, Note: row.Note, Sort: row.Sort,
		})
	}
	return out, nil
}

// normalizeLinkKind 把不认识的类型归到 other，而不是报错：
// 链接类型只影响图标与排序，为它挡住整条链接不值得。
func normalizeLinkKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case model.ResourceLinkOfficial, model.ResourceLinkMirror, model.ResourceLinkPackage,
		model.ResourceLinkRepo, model.ResourceLinkImage:
		return strings.TrimSpace(kind)
	default:
		return model.ResourceLinkOther
	}
}

// Download 签发下载凭证。
//
// 下载地址是现场签发的短时有效链接，而不是入库时存好的直链：
// 直链一旦泄露就是永久有效的免费下载入口，运营也失去了"撤回资源"的能力。
func (s *ResourceService) Download(ctx context.Context, key string, userID int64) (*dto.ResourceDownload, error) {
	res, err := s.resolveResource(ctx, key)
	if err != nil {
		return nil, err
	}
	if !res.IsPublished() {
		return nil, errcode.ErrResourceNotPublished
	}
	if res.NeedLogin && userID <= 0 {
		return nil, errcode.ErrResourceLoginNeeded
	}

	// 只给外链的资源（安装包动辄上百 MB，平台不自托管）没有可签发的文件。
	// 这里必须单独拦一句：落到下面的空 key 分支会回「请联系管理员」，
	// 而用户真正该做的是去点页面上的下载链接，那句话把人引向了错误的方向。
	if !res.HasFile() {
		return nil, errcode.ErrResourceFileInvalid.WithMsg(
			"这份资源只提供外部下载地址，请在资源页点击对应链接获取")
	}

	url, expireAt, _ := s.uploads.AssignURL(res.FileKey)
	if url == "" {
		return nil, errcode.ErrResourceFileInvalid.WithMsg("资源文件不可用，请联系管理员")
	}

	// 计数失败不该让用户下不到东西，只记日志。
	if err := s.resources.IncrementDownload(ctx, res.ID); err != nil {
		logger.Warn("资源下载数自增失败", "resource_id", res.ID, "err", err.Error())
	}

	return &dto.ResourceDownload{
		URL:      url,
		ExpireAt: expireAt,
		FileName: res.ReadableName(),
		FileSize: res.FileSize,
		Checksum: res.Checksum,
		Version:  res.Version,
	}, nil
}

// FileExts 返回可选的文件类型筛选项。
func (s *ResourceService) FileExts(ctx context.Context) []string {
	exts, err := s.resources.DistinctFileExts(ctx)
	if err != nil {
		logger.Warn("查询资源文件类型失败", "err", err.Error())
		return []string{}
	}
	return exts
}

// ============================ 后台 ============================

// AdminList 后台资源列表，可查全部状态。
func (s *ResourceService) AdminList(ctx context.Context, q dto.AdminResourceListQuery) ([]dto.AdminResourceItem, int64, error) {
	rows, total, err := s.resources.List(ctx, repository.ResourceFilter{
		Status:        q.Status,
		CategoryID:    q.CategoryID,
		KeywordGroups: SearchGroups(q.Keyword),
		Sort:          "latest",
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}

	base, err := s.toItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.AdminResourceItem, 0, len(base))
	for i := range base {
		items = append(items, dto.AdminResourceItem{
			ResourceItem: base[i],
			Status:       rows[i].Status,
			AuthorID:     rows[i].AuthorID,
			CreatedAt:    rows[i].CreatedAt,
			UpdatedAt:    rows[i].UpdatedAt,
		})
	}
	return items, total, nil
}

// AdminGet 后台读取资源详情（含 FileKey，便于替换文件）。
func (s *ResourceService) AdminGet(ctx context.Context, id int64) (*dto.AdminResourceDetail, error) {
	res, err := s.resources.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toAdminDetail(ctx, res)
}

// Create 新建资源。
func (s *ResourceService) Create(ctx context.Context, adminID int64, req dto.ResourceCreateReq) (*dto.AdminResourceDetail, error) {
	title, err := validateText(req.Title, 1, resourceLimits().title, errcode.ErrTitleInvalid, "资源标题")
	if err != nil {
		return nil, err
	}
	summary, err := validateOptionalText(req.Summary, resourceLimits().summary, errcode.ErrParamInvalid, "简介")
	if err != nil {
		return nil, err
	}
	content, err := validateContentText(req.Content, resourceLimits().content, errcode.ErrContentInvalid, "使用说明")
	if err != nil {
		return nil, err
	}
	version, err := validateOptionalText(req.Version, resourceLimits().version, errcode.ErrParamInvalid, "版本号")
	if err != nil {
		return nil, err
	}
	compat, err := validateOptionalText(req.CompatibleWith, resourceLimits().compat, errcode.ErrParamInvalid, "适用环境")
	if err != nil {
		return nil, err
	}
	tags, err := normalizeContentTags(req.Tags)
	if err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, req.CategoryID); err != nil {
		return nil, err
	}

	file, err := s.inspectFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}
	fileName, fileExt := normalizeResourceFileName(req.FileName, file)

	slug, err := s.resolveSlug(ctx, req.Slug, title, 0)
	if err != nil {
		return nil, err
	}

	status := model.ResourceStatusDraft
	if req.Status == model.ResourceStatusPublished {
		status = model.ResourceStatusPublished
	}

	now := time.Now()
	res := &model.Resource{
		Title:          title,
		Slug:           slug,
		Summary:        summary,
		Content:        content,
		Cover:          strings.TrimSpace(req.Cover),
		FileKey:        file.Key,
		FileName:       fileName,
		FileSize:       file.Size,
		FileExt:        fileExt,
		Checksum:       file.SHA256,
		Version:        version,
		CompatibleWith: compat,
		CategoryID:     req.CategoryID,
		Tags:           tags,
		NeedLogin:      req.NeedLogin,
		Status:         status,
		AuthorID:       adminID,
	}
	if res.Summary == "" {
		res.Summary = autoSummary(content)
	}
	if status == model.ResourceStatusPublished {
		res.PublishedAt = &now
	}

	if err := s.resources.Create(ctx, res); err != nil {
		return nil, err
	}
	return s.toAdminDetail(ctx, res)
}

// Update 局部更新资源，未传字段保持不变。
func (s *ResourceService) Update(ctx context.Context, id int64, req dto.ResourceUpdateReq) (*dto.AdminResourceDetail, error) {
	res, err := s.resources.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any, 16)

	if req.Title != nil {
		title, err := validateText(*req.Title, 1, resourceLimits().title, errcode.ErrTitleInvalid, "资源标题")
		if err != nil {
			return nil, err
		}
		updates["title"] = title
	}
	if req.Slug != nil {
		slug, err := s.resolveSlug(ctx, *req.Slug, res.Title, id)
		if err != nil {
			return nil, err
		}
		updates["slug"] = slug
	}
	if req.Summary != nil {
		summary, err := validateOptionalText(*req.Summary, resourceLimits().summary, errcode.ErrParamInvalid, "简介")
		if err != nil {
			return nil, err
		}
		updates["summary"] = summary
	}
	if req.Content != nil {
		content, err := validateContentText(*req.Content, resourceLimits().content, errcode.ErrContentInvalid, "使用说明")
		if err != nil {
			return nil, err
		}
		updates["content"] = content
	}
	if req.Cover != nil {
		cover, err := validateOptionalText(*req.Cover, dto.AvatarMaxLen(), errcode.ErrParamInvalid, "封面地址")
		if err != nil {
			return nil, err
		}
		updates["cover"] = cover
	}
	if req.Version != nil {
		version, err := validateOptionalText(*req.Version, resourceLimits().version, errcode.ErrParamInvalid, "版本号")
		if err != nil {
			return nil, err
		}
		updates["version"] = version
	}
	if req.CompatibleWith != nil {
		compat, err := validateOptionalText(*req.CompatibleWith, resourceLimits().compat, errcode.ErrParamInvalid, "适用环境")
		if err != nil {
			return nil, err
		}
		updates["compatible_with"] = compat
	}
	if req.Tags != nil {
		tags, err := normalizeContentTags(*req.Tags)
		if err != nil {
			return nil, err
		}
		updates["tags"] = tags
	}
	if req.CategoryID != nil {
		if err := s.checkCategory(ctx, *req.CategoryID); err != nil {
			return nil, err
		}
		updates["category_id"] = *req.CategoryID
	}

	// 换文件时大小与校验和必须跟着重算，否则页面展示的大小和实际文件对不上，
	// 校验和也会失效——那比不提供校验和更糟，用户会以为文件被篡改了。
	if req.FileKey != nil {
		file, err := s.inspectFile(ctx, *req.FileKey)
		if err != nil {
			return nil, err
		}
		fileName, fileExt := normalizeResourceFileName(derefString(req.FileName, ""), file)
		updates["file_key"] = file.Key
		updates["file_size"] = file.Size
		updates["checksum"] = file.SHA256
		if fileName != "" {
			updates["file_name"] = fileName
		}
		updates["file_ext"] = fileExt
	} else if req.FileName != nil {
		name := sanitizeFileName(*req.FileName)
		updates["file_name"] = name
	}

	if req.NeedLogin != nil {
		updates["need_login"] = *req.NeedLogin
	}

	if err := s.resources.UpdateFields(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// Delete 删除资源，并尝试回收不再被引用的文件。
func (s *ResourceService) Delete(ctx context.Context, id int64) error {
	res, err := s.resources.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.resources.Delete(ctx, id); err != nil {
		return err
	}

	// 只有当没有别的资源还引用这个文件时，才真的去存储里删。
	// 否则两个资源页共用一个脚本包时，删其中一个会把另一个的文件也删没。
	if strings.TrimSpace(res.FileKey) == "" {
		return nil
	}
	stillUsed, err := s.resources.ExtExists(ctx, res.FileKey, id)
	if err != nil {
		logger.Warn("检查资源文件引用失败，跳过物理删除", "resource_id", id, "err", err.Error())
		return nil
	}
	if stillUsed {
		return nil
	}
	if err := s.uploads.Delete(ctx, res.FileKey); err != nil {
		// 文件删除失败不影响资源删除本身，留日志供后续清理。
		logger.Warn("删除资源文件失败，需人工清理", "resource_id", id, "file_key", res.FileKey, "err", err.Error())
	}
	return nil
}

// Publish 发布资源。
func (s *ResourceService) Publish(ctx context.Context, id int64) (*dto.AdminResourceDetail, error) {
	res, err := s.resources.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if res.Status == model.ResourceStatusPublished {
		return nil, errcode.ErrResourceStateInvalid.WithMsg("该资源已是发布状态")
	}
	// 发布前必须确认文件确实还在：资源页最忌讳的形态是"能点下载但下不到"。
	if strings.TrimSpace(res.FileKey) == "" {
		return nil, errcode.ErrResourceFileInvalid.WithMsg("请先上传资源文件")
	}
	if _, err := s.uploads.Inspect(ctx, res.FileKey); err != nil {
		return nil, errcode.ErrResourceFileInvalid.WithMsg("资源文件已丢失，请重新上传后再发布")
	}

	updates := map[string]any{"status": model.ResourceStatusPublished}
	if res.PublishedAt == nil {
		updates["published_at"] = time.Now()
	}
	if err := s.resources.UpdateFields(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// Offline 下线资源。
func (s *ResourceService) Offline(ctx context.Context, id int64) (*dto.AdminResourceDetail, error) {
	res, err := s.resources.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if res.Status != model.ResourceStatusPublished {
		return nil, errcode.ErrResourceStateInvalid.WithMsg("只有已发布的资源可以下线")
	}
	if err := s.resources.UpdateFields(ctx, id, map[string]any{"status": model.ResourceStatusOffline}); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// ============================ 内部 ============================

// inspectFile 校验文件来自资源场景并读取实测元信息。
func (s *ResourceService) inspectFile(ctx context.Context, key string) (*UploadedFile, error) {
	raw := strings.TrimSpace(key)
	if raw == "" {
		return nil, errcode.ErrResourceFileInvalid.WithMsg("请先上传资源文件")
	}
	if !HasPrefix(raw, resourceScene) {
		return nil, errcode.ErrResourceFileInvalid.WithMsg(
			"文件不是通过资源上传场景获得的，请使用 scene=" + resourceScene + " 重新上传")
	}
	file, err := s.uploads.Inspect(ctx, raw)
	if err != nil {
		return nil, errcode.ErrResourceFileInvalid.WithMsg("资源文件读取失败，请重新上传")
	}
	if file.Size <= 0 {
		return nil, errcode.ErrResourceFileInvalid.WithMsg("资源文件为空")
	}
	return file, nil
}

// normalizeResourceFileName 规整展示用文件名，并推导扩展名。
//
// 扩展名优先取用户原始文件名的：存储 key 里的扩展名是服务端按识别出的
// MIME 生成的（deploy.sh 会被存成 .txt），拿它当扩展名会让筛选器里
// 全是 txt，用户根本找不到自己想要的脚本。
func normalizeResourceFileName(raw string, file *UploadedFile) (string, string) {
	name := sanitizeFileName(raw)
	if name == "" {
		name = path.Base(file.Key)
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	if ext == "" || len(ext) > fileExtMaxLen {
		// 原始文件名缺失或扩展名异常时，退回服务端识别出的类型。
		return name, file.Ext
	}
	return name, ext
}

// sanitizeFileName 去掉路径成分与控制字符，只保留展示用的裸文件名。
// 前端会把它放进下载框与列表，带上路径分隔符只会在各个平台上显示成怪东西，
// 更严重的是某些客户端会把带路径的文件名当作目录写入。
func sanitizeFileName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	// 同时处理两种分隔符：上传可能来自 Windows 客户端。
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	if runes := []rune(name); len(runes) > fileNameMaxLen {
		name = string(runes[:fileNameMaxLen])
	}
	return name
}

func (s *ResourceService) checkCategory(ctx context.Context, categoryID int64) error {
	if categoryID <= 0 {
		return nil
	}
	_, err := s.categories.GetByID(ctx, categoryID)
	return err
}

func (s *ResourceService) resolveResource(ctx context.Context, key string) (*model.Resource, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errcode.ErrResourceNotFound
	}
	if isNumericKey(key) {
		if id, err := strconv.ParseInt(key, 10, 64); err == nil && id > 0 {
			return s.resources.GetByID(ctx, id)
		}
	}
	return s.resources.GetBySlug(ctx, key)
}

func (s *ResourceService) resolveSlug(ctx context.Context, raw, fallbackTitle string, excludeID int64) (string, error) {
	base := truncateSlug(sanitizeSlug(firstNonEmpty(raw, fallbackTitle)), resourceLimits().slug)
	if base == "" {
		base = "res-" + idgen.NumericCode(8)
	}

	for i := 0; i < slugMaxAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		taken, err := s.resources.SlugExists(ctx, candidate, excludeID)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return fmt.Sprintf("%s-%s", base, idgen.NumericCode(4)), nil
}

func (s *ResourceService) toItems(ctx context.Context, rows []model.Resource) ([]dto.ResourceItem, error) {
	items := make([]dto.ResourceItem, 0, len(rows))
	if len(rows) == 0 {
		return items, nil
	}

	cats, err := s.categoryIndex(ctx)
	if err != nil {
		logger.Warn("批量查询资源分类失败，分类名将留空", "err", err.Error())
		cats = map[int64]model.ArticleCategory{}
	}
	for i := range rows {
		items = append(items, buildResourceItem(rows[i], cats))
	}
	return items, nil
}

func (s *ResourceService) categoryIndex(ctx context.Context) (map[int64]model.ArticleCategory, error) {
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

func (s *ResourceService) relatedItems(ctx context.Context, res *model.Resource) ([]dto.ResourceItem, error) {
	rows, err := s.resources.Related(ctx, res.CategoryID, res.ID, relatedResourceLimit)
	if err != nil {
		logger.Warn("查询相关资源失败", "resource_id", res.ID, "err", err.Error())
		return []dto.ResourceItem{}, nil
	}
	return s.toItems(ctx, rows)
}

func (s *ResourceService) toAdminDetail(ctx context.Context, res *model.Resource) (*dto.AdminResourceDetail, error) {
	base, err := s.toItems(ctx, []model.Resource{*res})
	if err != nil {
		return nil, err
	}
	return &dto.AdminResourceDetail{
		AdminResourceItem: dto.AdminResourceItem{
			ResourceItem: base[0],
			Status:       res.Status,
			AuthorID:     res.AuthorID,
			CreatedAt:    res.CreatedAt,
			UpdatedAt:    res.UpdatedAt,
		},
		Content: res.Content,
		FileKey: res.FileKey,
	}, nil
}

func buildResourceItem(r model.Resource, cats map[int64]model.ArticleCategory) dto.ResourceItem {
	item := dto.ResourceItem{
		ID:             r.ID,
		Title:          r.Title,
		Slug:           r.Slug,
		Summary:        r.Summary,
		Cover:          r.Cover,
		FileName:       r.ReadableName(),
		FileSize:       r.FileSize,
		FileExt:        r.FileExt,
		Checksum:       r.Checksum,
		Version:        r.Version,
		CompatibleWith: r.CompatibleWith,
		CategoryID:     r.CategoryID,
		Tags:           []string(r.Tags),
		NeedLogin:      r.NeedLogin,
		HasFile:        r.HasFile(),
		DownloadCount:  r.DownloadCount,
		ViewCount:      r.ViewCount,
		PublishedAt:    r.PublishedAt,
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	if cat, ok := cats[r.CategoryID]; ok {
		item.CategoryName = cat.Name
		item.CategorySlug = cat.Slug
	}
	return item
}

type resourceLimitSet struct {
	title, slug, summary, content, version, compat int
}

func resourceLimits() resourceLimitSet {
	title, slug, summary, content, version, compat := dto.ResourceLimits()
	return resourceLimitSet{title, slug, summary, content, version, compat}
}
