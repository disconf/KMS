package service

import (
	"context"
	"fmt"
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

// relatedToolLimit 详情页推荐位条数。
const relatedToolLimit = 6

// maxGuidesPerTool 一个工具最多关联的教程条数。
// 四类各留 6 个位置：再多说明该拆成独立专题，而不是继续堆在工具页里。
const maxGuidesPerTool = 24

// ToolService 开源工具推荐业务逻辑。
type ToolService struct {
	tools      *repository.ToolRepo
	categories *repository.CategoryRepo
}

// NewToolService 创建 ToolService。
func NewToolService(tools *repository.ToolRepo, categories *repository.CategoryRepo) *ToolService {
	return &ToolService{tools: tools, categories: categories}
}

// ============================ 前台 ============================

// ListTools 查询已发布工具列表。
func (s *ToolService) ListTools(ctx context.Context, q dto.ToolListQuery) ([]dto.ToolItem, int64, error) {
	filter := repository.ToolFilter{
		Status:        model.ToolStatusPublished,
		CategoryID:    q.CategoryID,
		KeywordGroups: SearchGroups(q.Keyword),
		Language:      strings.TrimSpace(q.Language),
		License:       strings.TrimSpace(q.License),
		Tag:           q.Tag,
		FeaturedOnly:  q.FeaturedOnly,
		MinStars:      q.MinStars,
		Sort:          q.Sort,
	}
	if filter.CategoryID <= 0 && strings.TrimSpace(q.CategorySlug) != "" {
		cat, err := s.categories.GetBySlug(ctx, strings.TrimSpace(q.CategorySlug))
		if err != nil {
			return nil, 0, err
		}
		filter.CategoryID = cat.ID
	}

	rows, total, err := s.tools.List(ctx, filter, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items, err := s.toItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ToolDetail 按 slug 或 ID 查询工具详情。
func (s *ToolService) ToolDetail(ctx context.Context, key string) (*dto.ToolDetail, error) {
	tool, err := s.resolveTool(ctx, key)
	if err != nil {
		return nil, err
	}
	if !tool.IsPublished() {
		return nil, errcode.ErrToolNotPublished
	}

	// 浏览数自增失败不该让用户看不到内容。
	if err := s.tools.IncrementView(ctx, tool.ID); err != nil {
		logger.Warn("工具浏览数自增失败", "tool_id", tool.ID, "err", err.Error())
	} else {
		tool.ViewCount++
	}

	items, err := s.toItems(ctx, []model.Tool{*tool})
	if err != nil {
		return nil, err
	}

	related, _ := s.relatedItems(ctx, tool)
	guides, err := s.guidesOf(ctx, tool.ID)
	if err != nil {
		// 教程关联读不出来不该让整个详情页 500：正文与仓库地址才是主体，
		// 少一块「配套教程」只是少一个入口。
		logger.Warn("读取工具教程失败", "tool_id", tool.ID, "err", err.Error())
		guides = []dto.ToolGuideItem{}
	}
	return &dto.ToolDetail{
		ToolItem:     items[0],
		Content:      tool.Content,
		SEO:          toolSEO(tool),
		StarsUpdated: tool.StarsUpdated,
		Related:      related,
		Guides:       guides,
	}, nil
}

// guidesOf 把教程关联转成出参。
//
// 指向站内文章时，标题与摘要优先取文章当前的值而不是关联里存的快照：
// 文章标题改过之后，工具页还挂着旧标题会让人以为点错了。
func (s *ToolService) guidesOf(ctx context.Context, toolID int64) ([]dto.ToolGuideItem, error) {
	rows, err := s.tools.ListGuidesByTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ToolGuideItem, 0, len(rows))
	for _, r := range rows {
		item := dto.ToolGuideItem{
			ID:          r.ID,
			Kind:        r.Kind,
			Title:       r.Title,
			URL:         r.URL,
			ArticleID:   r.ArticleID,
			ArticleSlug: r.ArticleSlug,
			Sort:        r.Sort,
		}
		if r.ArticleID > 0 {
			if r.ArticleTitle != "" {
				item.Title = r.ArticleTitle
				item.Summary = r.ArticleSummary
			}
			// 文章被删或下线后关联还在，这里跳过而不是展示一个死链。
			if r.ArticleSlug == "" {
				continue
			}
		}
		if item.Title == "" && item.URL == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// GuidesForAdmin 后台读取教程清单。
//
// 与前台 guidesOf 的关键差别：这里**不过滤**失效条目。
// 关联的文章被下线后，前台不该展示死链，但后台必须能看到它，
// 否则运营既改不了也删不掉，只能看着那条配置一直不生效。
func (s *ToolService) GuidesForAdmin(ctx context.Context, toolID int64) ([]dto.ToolGuideItem, error) {
	if _, err := s.tools.GetByID(ctx, toolID); err != nil {
		return nil, err
	}
	rows, err := s.tools.ListGuidesByTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ToolGuideItem, 0, len(rows))
	for _, r := range rows {
		title := r.Title
		summary := ""
		if r.ArticleID > 0 {
			if r.ArticleTitle != "" {
				title = r.ArticleTitle
				summary = r.ArticleSummary
			}
		}
		out = append(out, dto.ToolGuideItem{
			ID:          r.ID,
			Kind:        r.Kind,
			Title:       title,
			URL:         r.URL,
			ArticleID:   r.ArticleID,
			ArticleSlug: r.ArticleSlug,
			Summary:     summary,
			Sort:        r.Sort,
		})
	}
	return out, nil
}

// SetGuides 整体替换工具的教程清单。
func (s *ToolService) SetGuides(ctx context.Context, toolID int64, req dto.ToolGuidesReq) ([]dto.ToolGuideItem, error) {
	if _, err := s.tools.GetByID(ctx, toolID); err != nil {
		return nil, err
	}
	if len(req.Guides) > maxGuidesPerTool {
		return nil, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("一个工具最多关联 %d 篇教程", maxGuidesPerTool))
	}

	prepared := make([]model.ToolGuide, 0, len(req.Guides))
	for i, g := range req.Guides {
		if !model.IsValidToolGuideKind(g.Kind) {
			return nil, errcode.ErrParamInvalid.WithMsg("第 " + strconv.Itoa(i+1) + " 条教程的类型不合法")
		}
		title := strings.TrimSpace(g.Title)
		url := strings.TrimSpace(g.URL)

		if g.ArticleID > 0 {
			ref, err := s.tools.GetArticleRef(ctx, g.ArticleID)
			if err != nil {
				return nil, errcode.ErrParamInvalid.WithMsg(
					"第 " + strconv.Itoa(i+1) + " 条教程关联的文章不存在")
			}
			// 只接受已发布的文章：把草稿挂到公开展示的工具页上，
			// 等于绕过草稿保护把它暴露出去。
			if ref.Status != model.ArticleStatusPublished {
				return nil, errcode.ErrParamInvalid.WithMsg(
					"第 " + strconv.Itoa(i+1) + " 条教程关联的文章尚未发布")
			}
			if title == "" {
				title = ref.Title
			}
			url = ""
		} else {
			if url == "" {
				return nil, errcode.ErrParamInvalid.WithMsg(
					"第 " + strconv.Itoa(i+1) + " 条教程需要关联站内文章或填写外部链接")
			}
			normalized, err := normalizeURL(url, 512, "教程链接")
			if err != nil {
				return nil, errcode.ErrParamInvalid.WithMsg(
					"第 " + strconv.Itoa(i+1) + " 条教程的链接不合法，只接受 http/https")
			}
			url = normalized
			if title == "" {
				title = url
			}
		}

		prepared = append(prepared, model.ToolGuide{
			Kind:      g.Kind,
			ArticleID: g.ArticleID,
			Title:     truncateRunes(title, 200),
			URL:       url,
			Sort:      g.Sort,
		})
	}

	if err := s.tools.ReplaceGuides(ctx, toolID, prepared); err != nil {
		return nil, err
	}
	return s.guidesOf(ctx, toolID)
}

// Filters 返回工具筛选器的可选值。
//
// 从已发布数据里聚合，而不是写一份静态字典：运维工具的语言分布变化很快，
// 静态字典迟早会出现"筛选项里有但一个工具都没有"的死选项。
func (s *ToolService) Filters(ctx context.Context) map[string][]string {
	out := map[string][]string{
		"languages":   {},
		"licenses":    {},
		"deployModes": dto.ToolDeployModes(),
	}
	for _, col := range []string{"language", "license"} {
		values, err := s.tools.Distinct(ctx, col)
		if err != nil {
			// 筛选项是辅助功能，取不到就返回空列表，不影响列表本身。
			logger.Warn("查询工具筛选值失败", "column", col, "err", err.Error())
			continue
		}
		out[col+"s"] = values
	}
	return out
}

// ============================ 后台 ============================

// AdminList 后台工具列表，可查全部状态。
func (s *ToolService) AdminList(ctx context.Context, q dto.AdminToolListQuery) ([]dto.AdminToolItem, int64, error) {
	rows, total, err := s.tools.List(ctx, repository.ToolFilter{
		Status:        q.Status,
		CategoryID:    q.CategoryID,
		KeywordGroups: SearchGroups(q.Keyword),
		Sort:          "sort",
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}

	base, err := s.toItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.AdminToolItem, 0, len(base))
	for i := range base {
		items = append(items, dto.AdminToolItem{
			ToolItem:  base[i],
			Status:    rows[i].Status,
			Sort:      rows[i].Sort,
			AuthorID:  rows[i].AuthorID,
			CreatedAt: rows[i].CreatedAt,
			UpdatedAt: rows[i].UpdatedAt,
		})
	}
	return items, total, nil
}

// AdminGet 后台读取工具详情（含草稿）。
func (s *ToolService) AdminGet(ctx context.Context, id int64) (*dto.AdminToolDetail, error) {
	tool, err := s.tools.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toAdminDetail(ctx, tool)
}

// Create 新建工具。
func (s *ToolService) Create(ctx context.Context, adminID int64, req dto.ToolCreateReq) (*dto.AdminToolDetail, error) {
	fields, err := s.prepareFields(ctx, toolInput{
		Name:        req.Name,
		Summary:     req.Summary,
		Content:     req.Content,
		Logo:        req.Logo,
		RepoURL:     req.RepoURL,
		HomepageURL: req.HomepageURL,
		License:     req.License,
		Language:    req.Language,
		Stars:       req.Stars,
		ForkCount:   req.ForkCount,
		DeployModes: req.DeployModes,
		Tags:        req.Tags,
		CategoryID:  req.CategoryID,
		SEOTitle:    req.SEOTitle,
		SEODesc:     req.SEODescription,
		SEOKeywords: req.SEOKeywords,
	})
	if err != nil {
		return nil, err
	}

	slug, err := s.resolveSlug(ctx, req.Slug, fields.name, 0)
	if err != nil {
		return nil, err
	}

	status := model.ToolStatusDraft
	if req.Status == model.ToolStatusPublished {
		status = model.ToolStatusPublished
	}
	if status == model.ToolStatusPublished && strings.TrimSpace(fields.content) == "" {
		return nil, errcode.ErrContentInvalid.WithMsg("详细介绍为空，无法直接发布")
	}

	now := time.Now()
	tool := &model.Tool{
		Name:           fields.name,
		Slug:           slug,
		Summary:        fields.summary,
		Content:        fields.content,
		Logo:           fields.logo,
		RepoURL:        fields.repoURL,
		HomepageURL:    fields.homepageURL,
		License:        fields.license,
		Language:       fields.language,
		Stars:          fields.stars,
		ForkCount:      fields.forkCount,
		StarsUpdated:   &now,
		DeployModes:    fields.deployModes,
		Tags:           fields.tags,
		CategoryID:     fields.categoryID,
		SEOTitle:       fields.seo.title,
		SEODescription: fields.seo.description,
		SEOKeywords:    fields.seo.keywords,
		Featured:       req.Featured,
		Sort:           req.Sort,
		Status:         status,
		AuthorID:       adminID,
	}
	if tool.Summary == "" {
		tool.Summary = autoSummary(fields.content)
	}
	if status == model.ToolStatusPublished {
		tool.PublishedAt = &now
	}

	if err := s.tools.Create(ctx, tool); err != nil {
		return nil, err
	}
	return s.toAdminDetail(ctx, tool)
}

// Update 局部更新工具，未传字段保持不变。
func (s *ToolService) Update(ctx context.Context, id int64, req dto.ToolUpdateReq) (*dto.AdminToolDetail, error) {
	tool, err := s.tools.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any, 20)

	if req.Name != nil {
		name, err := validateText(*req.Name, 1, toolLimits().name, errcode.ErrTitleInvalid, "工具名称")
		if err != nil {
			return nil, err
		}
		updates["name"] = name
	}
	if req.Slug != nil {
		slug, err := s.resolveSlug(ctx, *req.Slug, tool.Name, id)
		if err != nil {
			return nil, err
		}
		updates["slug"] = slug
	}
	if req.Summary != nil {
		summary, err := validateOptionalText(*req.Summary, toolLimits().summary, errcode.ErrParamInvalid, "简介")
		if err != nil {
			return nil, err
		}
		updates["summary"] = summary
	}
	if req.Content != nil {
		content, err := validateContentText(*req.Content, toolLimits().content, errcode.ErrContentInvalid, "详细介绍")
		if err != nil {
			return nil, err
		}
		updates["content"] = content
	}
	if req.Logo != nil {
		logo, err := validateOptionalText(*req.Logo, dto.AvatarMaxLen(), errcode.ErrParamInvalid, "Logo 地址")
		if err != nil {
			return nil, err
		}
		updates["logo"] = logo
	}
	if req.RepoURL != nil {
		link, err := normalizeURL(*req.RepoURL, toolLimits().url, "仓库地址")
		if err != nil {
			return nil, err
		}
		updates["repo_url"] = link
	}
	if req.HomepageURL != nil {
		link, err := normalizeURL(*req.HomepageURL, toolLimits().url, "官网地址")
		if err != nil {
			return nil, err
		}
		updates["homepage_url"] = link
	}
	if req.License != nil {
		license, err := validateOptionalText(*req.License, toolLimits().license, errcode.ErrParamInvalid, "开源协议")
		if err != nil {
			return nil, err
		}
		updates["license"] = license
	}
	if req.Language != nil {
		language, err := validateOptionalText(*req.Language, toolLimits().language, errcode.ErrParamInvalid, "实现语言")
		if err != nil {
			return nil, err
		}
		updates["language"] = language
	}
	if req.Stars != nil {
		updates["stars"] = clampMin(*req.Stars)
		updates["stars_updated"] = time.Now()
	}
	if req.ForkCount != nil {
		updates["fork_count"] = clampMin(*req.ForkCount)
	}
	if req.DeployModes != nil {
		modes, err := normalizeDeployModes(*req.DeployModes)
		if err != nil {
			return nil, err
		}
		updates["deploy_modes"] = modes
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
	if req.SEOTitle != nil || req.SEODescription != nil || req.SEOKeywords != nil {
		seo, err := validateSEO(
			derefString(req.SEOTitle, tool.SEOTitle),
			derefString(req.SEODescription, tool.SEODescription),
			derefString(req.SEOKeywords, tool.SEOKeywords),
		)
		if err != nil {
			return nil, err
		}
		updates["seo_title"] = seo.title
		updates["seo_description"] = seo.description
		updates["seo_keywords"] = seo.keywords
	}
	if req.Featured != nil {
		updates["featured"] = *req.Featured
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}

	if err := s.tools.UpdateFields(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// Delete 删除工具。
func (s *ToolService) Delete(ctx context.Context, id int64) error {
	if _, err := s.tools.GetByID(ctx, id); err != nil {
		return err
	}
	return s.tools.Delete(ctx, id)
}

// Publish 发布工具（草稿 / 已下线 → 已发布）。
func (s *ToolService) Publish(ctx context.Context, id int64) (*dto.AdminToolDetail, error) {
	tool, err := s.tools.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tool.Status == model.ToolStatusPublished {
		return nil, errcode.ErrToolStateInvalid.WithMsg("该工具已是发布状态")
	}
	if strings.TrimSpace(tool.Content) == "" {
		return nil, errcode.ErrContentInvalid.WithMsg("详细介绍为空，无法发布")
	}

	updates := map[string]any{"status": model.ToolStatusPublished}
	if tool.PublishedAt == nil {
		now := time.Now()
		updates["published_at"] = now
	}
	if err := s.tools.UpdateFields(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// Offline 下线工具。
func (s *ToolService) Offline(ctx context.Context, id int64) (*dto.AdminToolDetail, error) {
	tool, err := s.tools.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tool.Status != model.ToolStatusPublished {
		return nil, errcode.ErrToolStateInvalid.WithMsg("只有已发布的工具可以下线")
	}
	if err := s.tools.UpdateFields(ctx, id, map[string]any{"status": model.ToolStatusOffline}); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// ============================ 内部 ============================

// toolInput 是新建时的字段集合，避免 Create 里传十几个参数。
type toolInput struct {
	Name        string
	Summary     string
	Content     string
	Logo        string
	RepoURL     string
	HomepageURL string
	License     string
	Language    string
	Stars       int64
	ForkCount   int64
	DeployModes []string
	Tags        []string
	CategoryID  int64
	SEOTitle    string
	SEODesc     string
	SEOKeywords string
}

// preparedTool 是校验与规范化之后的字段集合。
type preparedTool struct {
	name        string
	summary     string
	content     string
	logo        string
	repoURL     string
	homepageURL string
	license     string
	language    string
	stars       int64
	forkCount   int64
	deployModes model.StringList
	tags        model.StringList
	categoryID  int64
	seo         seoFields
}

func (s *ToolService) prepareFields(ctx context.Context, in toolInput) (preparedTool, error) {
	var out preparedTool
	var err error

	if out.name, err = validateText(in.Name, 1, toolLimits().name, errcode.ErrTitleInvalid, "工具名称"); err != nil {
		return out, err
	}
	if out.summary, err = validateOptionalText(in.Summary, toolLimits().summary, errcode.ErrParamInvalid, "简介"); err != nil {
		return out, err
	}
	if out.content, err = validateContentText(in.Content, toolLimits().content, errcode.ErrContentInvalid, "详细介绍"); err != nil {
		return out, err
	}
	if out.logo, err = validateOptionalText(in.Logo, dto.AvatarMaxLen(), errcode.ErrParamInvalid, "Logo 地址"); err != nil {
		return out, err
	}
	if out.repoURL, err = normalizeURL(in.RepoURL, toolLimits().url, "仓库地址"); err != nil {
		return out, err
	}
	if out.homepageURL, err = normalizeURL(in.HomepageURL, toolLimits().url, "官网地址"); err != nil {
		return out, err
	}
	if out.license, err = validateOptionalText(in.License, toolLimits().license, errcode.ErrParamInvalid, "开源协议"); err != nil {
		return out, err
	}
	if out.language, err = validateOptionalText(in.Language, toolLimits().language, errcode.ErrParamInvalid, "实现语言"); err != nil {
		return out, err
	}
	if in.Stars < 0 || in.ForkCount < 0 {
		return out, errcode.ErrParamInvalid.WithMsg("Star / Fork 数不能为负数")
	}
	out.stars, out.forkCount = in.Stars, in.ForkCount

	if out.deployModes, err = normalizeDeployModes(in.DeployModes); err != nil {
		return out, err
	}
	if out.tags, err = normalizeContentTags(in.Tags); err != nil {
		return out, err
	}
	if err := s.checkCategory(ctx, in.CategoryID); err != nil {
		return out, err
	}
	out.categoryID = in.CategoryID

	if out.seo, err = validateSEO(in.SEOTitle, in.SEODesc, in.SEOKeywords); err != nil {
		return out, err
	}
	return out, nil
}

func (s *ToolService) checkCategory(ctx context.Context, categoryID int64) error {
	if categoryID <= 0 {
		return nil
	}
	_, err := s.categories.GetByID(ctx, categoryID)
	return err
}

func (s *ToolService) resolveTool(ctx context.Context, key string) (*model.Tool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errcode.ErrToolNotFound
	}
	if isNumericKey(key) {
		if id, err := strconv.ParseInt(key, 10, 64); err == nil && id > 0 {
			return s.tools.GetByID(ctx, id)
		}
	}
	return s.tools.GetBySlug(ctx, key)
}

// resolveSlug 生成唯一 slug。用户传了就按其规整，没传就用名称生成。
func (s *ToolService) resolveSlug(ctx context.Context, raw, fallbackName string, excludeID int64) (string, error) {
	base := truncateSlug(sanitizeSlug(firstNonEmpty(raw, fallbackName)), toolLimits().slug)
	if base == "" {
		base = "tool-" + idgen.NumericCode(8)
	}

	for i := 0; i < slugMaxAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		taken, err := s.tools.SlugExists(ctx, candidate, excludeID)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return fmt.Sprintf("%s-%s", base, idgen.NumericCode(4)), nil
}

// toItems 批量转换为 DTO，一次性解析分类名，避免 N+1 查询。
func (s *ToolService) toItems(ctx context.Context, rows []model.Tool) ([]dto.ToolItem, error) {
	items := make([]dto.ToolItem, 0, len(rows))
	if len(rows) == 0 {
		return items, nil
	}

	cats, err := s.categoryIndex(ctx)
	if err != nil {
		// 分类名只影响展示，查不到就留空，不阻断列表。
		logger.Warn("批量查询工具分类失败，分类名将留空", "err", err.Error())
		cats = map[int64]model.ArticleCategory{}
	}
	for i := range rows {
		items = append(items, buildToolItem(rows[i], cats))
	}
	return items, nil
}

func (s *ToolService) categoryIndex(ctx context.Context) (map[int64]model.ArticleCategory, error) {
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

func (s *ToolService) relatedItems(ctx context.Context, tool *model.Tool) ([]dto.ToolItem, error) {
	rows, err := s.tools.Related(ctx, tool.CategoryID, tool.ID, relatedToolLimit)
	if err != nil {
		logger.Warn("查询相关工具失败", "tool_id", tool.ID, "err", err.Error())
		return []dto.ToolItem{}, nil
	}
	return s.toItems(ctx, rows)
}

func (s *ToolService) toAdminDetail(ctx context.Context, tool *model.Tool) (*dto.AdminToolDetail, error) {
	base, err := s.toItems(ctx, []model.Tool{*tool})
	if err != nil {
		return nil, err
	}
	return &dto.AdminToolDetail{
		AdminToolItem: dto.AdminToolItem{
			ToolItem:  base[0],
			Status:    tool.Status,
			Sort:      tool.Sort,
			AuthorID:  tool.AuthorID,
			CreatedAt: tool.CreatedAt,
			UpdatedAt: tool.UpdatedAt,
		},
		Content: tool.Content,
		SEO:     toolSEO(tool),
	}, nil
}

func buildToolItem(t model.Tool, cats map[int64]model.ArticleCategory) dto.ToolItem {
	item := dto.ToolItem{
		ID:          t.ID,
		Name:        t.Name,
		Slug:        t.Slug,
		Summary:     t.Summary,
		Logo:        t.Logo,
		RepoURL:     t.RepoURL,
		HomepageURL: t.HomepageURL,
		License:     t.License,
		Language:    t.Language,
		Stars:       t.Stars,
		ForkCount:   t.ForkCount,
		DeployModes: []string(t.DeployModes),
		Tags:        []string(t.Tags),
		CategoryID:  t.CategoryID,
		Featured:    t.Featured,
		ViewCount:   t.ViewCount,
		PublishedAt: t.PublishedAt,
	}
	// 输出空数组而非 null，前端可以少写一个判空分支。
	if item.Tags == nil {
		item.Tags = []string{}
	}
	if item.DeployModes == nil {
		item.DeployModes = []string{}
	}
	if cat, ok := cats[t.CategoryID]; ok {
		item.CategoryName = cat.Name
		item.CategorySlug = cat.Slug
	}
	return item
}

func toolSEO(t *model.Tool) dto.SEOInfo {
	info := dto.SEOInfo{Title: t.SEOTitle, Description: t.SEODescription, Keywords: t.SEOKeywords}
	// SEO 字段留空时用基础字段兜底，避免前端输出空的 meta 标签。
	if info.Title == "" {
		info.Title = t.Name
	}
	if info.Description == "" {
		info.Description = t.Summary
	}
	if info.Keywords == "" {
		info.Keywords = strings.Join(t.Tags, ",")
	}
	return info
}

// toolLimits 把 dto 的长度约束取回来，避免每次调用都写一长串返回值。
type toolLimitSet struct {
	name, slug, summary, content, url, license, language int
}

func toolLimits() toolLimitSet {
	name, slug, summary, content, url, license, language := dto.ToolLimits()
	return toolLimitSet{name, slug, summary, content, url, license, language}
}
