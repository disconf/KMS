package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

const (
	// toolBookMaxChapters 单本教程集的章数上限。
	toolBookMaxChapters = 20
	// toolBookMaxNodes 单本教程集的节点总数上限（章 + 节）。
	toolBookMaxNodes = 60
)

// ToolBookService 教程集：把站内文章按章节组织成「从零基础到深入」的学习路径。
//
// 它只做目录组织，正文仍然是 articles 里的文章。这条边界是刻意的：
// 一旦教程集开始存正文，编辑器、图片上传、审核流、SEO、版本回滚
// 全都要再做一遍，而它们会立刻落后于主编辑器。
type ToolBookService struct {
	books *repository.ToolBookRepo
	tools *repository.ToolRepo
}

// NewToolBookService 创建 ToolBookService。
func NewToolBookService(books *repository.ToolBookRepo, tools *repository.ToolRepo) *ToolBookService {
	return &ToolBookService{books: books, tools: tools}
}

// ============================ 前台 ============================

// BooksOf 返回工具详情页要展示的教程集（只含已发布的书）。
//
// 失效节点（正文文章被删或下线）在这里被整条跳过：目录里挂一个点进去
// 404 的链接，比这块内容不存在更伤——用户会以为整个教程集坏了。
func (s *ToolBookService) BooksOf(ctx context.Context, toolID int64) ([]dto.ToolBookItem, error) {
	books, err := s.books.BooksByTool(ctx, toolID)
	if err != nil {
		return nil, err
	}

	visible := make([]model.ToolBook, 0, len(books))
	for _, b := range books {
		if b.Status == model.ToolStatusPublished {
			visible = append(visible, b)
		}
	}
	if len(visible) == 0 {
		return []dto.ToolBookItem{}, nil
	}

	ids := make([]int64, 0, len(visible))
	for _, b := range visible {
		ids = append(ids, b.ID)
	}
	rows, err := s.books.ChapterRows(ctx, ids)
	if err != nil {
		return nil, err
	}

	grouped := groupChapterRows(rows)
	out := make([]dto.ToolBookItem, 0, len(visible))
	for _, b := range visible {
		tree, count := buildChapterTree(grouped[b.ID], false)
		// 目录被清空（或全部失效）的教程集不展示：点进去只有一张封面。
		if count == 0 {
			continue
		}
		item := toBookItem(b, count)
		// 工具详情页直接渲染整棵树：书通常只有两三本、节点几十个，
		// 一次带回来比"点开再请求"少一轮往返，也避免目录在展开时闪一下。
		item.Chapters = tree
		out = append(out, item)
	}
	return out, nil
}

// BookDetail 教程集详情（前台）。草稿与已下线的书一律 404。
func (s *ToolBookService) BookDetail(ctx context.Context, bookID int64) (*dto.ToolBookDetail, error) {
	book, err := s.books.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}
	if book.Status != model.ToolStatusPublished {
		return nil, errcode.ErrToolBookNotFound
	}
	tool, err := s.tools.GetByID(ctx, book.ToolID)
	if err != nil {
		return nil, err
	}
	rows, err := s.books.ChapterRows(ctx, []int64{book.ID})
	if err != nil {
		return nil, err
	}
	tree, count := buildChapterTree(rows, false)

	item := toBookItem(*book, count)
	item.Chapters = tree
	detail := &dto.ToolBookDetail{
		ToolBookItem: item,
		ToolName:     tool.Name,
		ToolSlug:     tool.Slug,
	}
	return detail, nil
}

// ============================ 后台 ============================

// AdminBooks 后台读取一个工具下的全部教程集（含草稿与失效节点）。
//
// 与前台的关键差别：失效节点**保留并标 Broken**。前台不展示死链是对的，
// 但后台必须看得见——否则运营既改不了也删不掉，只能看着那条配置一直不生效。
func (s *ToolBookService) AdminBooks(ctx context.Context, toolID int64) ([]dto.ToolBookItem, error) {
	if _, err := s.tools.GetByID(ctx, toolID); err != nil {
		return nil, err
	}
	books, err := s.books.BooksByTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	if len(books) == 0 {
		return []dto.ToolBookItem{}, nil
	}
	ids := make([]int64, 0, len(books))
	for _, b := range books {
		ids = append(ids, b.ID)
	}
	rows, err := s.books.ChapterRows(ctx, ids)
	if err != nil {
		return nil, err
	}
	grouped := groupChapterRows(rows)

	out := make([]dto.ToolBookItem, 0, len(books))
	for _, b := range books {
		tree, count := buildChapterTree(grouped[b.ID], true)
		item := toBookItem(b, count)
		item.Chapters = tree
		out = append(out, item)
	}
	return out, nil
}

// AdminBook 后台读取单本教程集（含章节树）。
func (s *ToolBookService) AdminBook(ctx context.Context, bookID int64) (*dto.ToolBookItem, error) {
	book, err := s.books.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}
	rows, err := s.books.ChapterRows(ctx, []int64{book.ID})
	if err != nil {
		return nil, err
	}
	tree, count := buildChapterTree(rows, true)
	item := toBookItem(*book, count)
	item.Chapters = tree
	return &item, nil
}

// CreateBook 新建教程集（固定草稿）。
func (s *ToolBookService) CreateBook(
	ctx context.Context, authorID, toolID int64, req dto.ToolBookCreateReq,
) (*dto.ToolBookItem, error) {
	if _, err := s.tools.GetByID(ctx, toolID); err != nil {
		return nil, err
	}
	title, err := validateText(req.Title, 2, 200, errcode.ErrTitleInvalid, "教程集标题")
	if err != nil {
		return nil, err
	}
	summary, err := validateOptionalText(req.Summary, 500, errcode.ErrParamInvalid, "简介")
	if err != nil {
		return nil, err
	}
	level, err := normalizeBookLevel(req.Level)
	if err != nil {
		return nil, err
	}
	cover, err := normalizeURL(req.Cover, 512, "封面地址")
	if err != nil {
		return nil, err
	}

	b := &model.ToolBook{
		ToolID: toolID, Title: title, Summary: summary, Cover: cover, Level: level,
		Sort: req.Sort, Status: model.ToolStatusDraft, AuthorID: authorID,
	}
	if err := s.books.CreateBook(ctx, b); err != nil {
		return nil, err
	}
	return s.AdminBook(ctx, b.ID)
}

// UpdateBook 局部更新教程集。状态不在这里改，走发布 / 下线。
func (s *ToolBookService) UpdateBook(ctx context.Context, bookID int64, req dto.ToolBookUpdateReq) (*dto.ToolBookItem, error) {
	book, err := s.books.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}
	updates := make(map[string]any, 5)
	if req.Title != nil {
		title, err := validateText(*req.Title, 2, 200, errcode.ErrTitleInvalid, "教程集标题")
		if err != nil {
			return nil, err
		}
		updates["title"] = title
	}
	if req.Summary != nil {
		summary, err := validateOptionalText(*req.Summary, 500, errcode.ErrParamInvalid, "简介")
		if err != nil {
			return nil, err
		}
		updates["summary"] = summary
	}
	if req.Cover != nil {
		cover, err := normalizeURL(*req.Cover, 512, "封面地址")
		if err != nil {
			return nil, err
		}
		updates["cover"] = cover
	}
	if req.Level != nil {
		level, err := normalizeBookLevel(*req.Level)
		if err != nil {
			return nil, err
		}
		updates["level"] = level
	}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if err := s.books.UpdateBookFields(ctx, book.ID, updates); err != nil {
		return nil, err
	}
	return s.AdminBook(ctx, book.ID)
}

// DeleteBook 删除教程集（章节一并物理清理）。
func (s *ToolBookService) DeleteBook(ctx context.Context, bookID int64) error {
	if _, err := s.books.GetBook(ctx, bookID); err != nil {
		return err
	}
	return s.books.DeleteBook(ctx, bookID)
}

// PublishBook 发布教程集。
//
// 发布前要求目录非空：空目录的"教程集"对用户毫无价值，
// 而它一旦挂在工具详情页，就会挤掉真正的推荐位。
func (s *ToolBookService) PublishBook(ctx context.Context, bookID int64) (*dto.ToolBookItem, error) {
	book, err := s.books.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}
	if book.Status == model.ToolStatusPublished {
		return nil, errcode.ErrToolBookStateInvalid.WithMsg("这本教程集已经是发布状态")
	}
	rows, err := s.books.ChapterRows(ctx, []int64{book.ID})
	if err != nil {
		return nil, err
	}
	if _, count := buildChapterTree(rows, false); count == 0 {
		return nil, errcode.ErrToolBookStateInvalid.WithMsg("目录为空，请先填写章节大纲再发布")
	}
	updates := map[string]any{"status": model.ToolStatusPublished}
	if book.PublishedAt == nil {
		updates["published_at"] = time.Now()
	}
	if err := s.books.UpdateBookFields(ctx, book.ID, updates); err != nil {
		return nil, err
	}
	return s.AdminBook(ctx, book.ID)
}

// OfflineBook 下线教程集。
func (s *ToolBookService) OfflineBook(ctx context.Context, bookID int64) (*dto.ToolBookItem, error) {
	book, err := s.books.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}
	if book.Status != model.ToolStatusPublished {
		return nil, errcode.ErrToolBookStateInvalid.WithMsg("只有已发布的教程集可以下线")
	}
	if err := s.books.UpdateBookFields(ctx, book.ID, map[string]any{"status": model.ToolStatusOffline}); err != nil {
		return nil, err
	}
	return s.AdminBook(ctx, book.ID)
}

// SetOutline 用大纲文本整体替换目录。
func (s *ToolBookService) SetOutline(
	ctx context.Context, bookID int64, raw string,
) (*dto.ToolChapterOutlineResult, error) {
	book, err := s.books.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}

	specs, warnings := parseChapterOutline(raw)

	chapterCount := 0
	for _, sp := range specs {
		if sp.isChapter {
			chapterCount++
		}
	}
	if chapterCount > toolBookMaxChapters {
		return nil, errcode.ErrToolChapterInvalid.WithMsg(
			fmt.Sprintf("单本教程集最多 %d 章，请拆成两本", toolBookMaxChapters))
	}
	if len(specs) > toolBookMaxNodes {
		return nil, errcode.ErrToolChapterInvalid.WithMsg(
			fmt.Sprintf("单本教程集最多 %d 个节点（章 + 节），请拆成两本", toolBookMaxNodes))
	}

	// 按「节挂在它前面最近的章下」组装成树。出现在任何章之前的节直接丢弃：
	// 保留它会变成一条无处安放的孤儿，目录里看不见、后台却占着一行。
	nodes := make([]repository.ChapterNode, 0, chapterCount)
	cur := -1
	for _, sp := range specs {
		ch := model.ToolBookChapter{Title: sp.title, Summary: sp.summary}
		if sp.ref != "" {
			// 某个 slug 找不到文章不该让整份大纲保存失败——那会逼运营
			// 在几十行里找出写错的那一个。降级成"保留标题、去掉链接"并提示。
			articleID, link, warn := s.resolveChapterRef(ctx, sp.ref)
			if warn != "" {
				warnings = append(warnings, warn)
			}
			ch.ArticleID = articleID
			ch.ExternalURL = link
		}
		if sp.isChapter {
			nodes = append(nodes, repository.ChapterNode{Chapter: ch})
			cur = len(nodes) - 1
			continue
		}
		if cur < 0 {
			warnings = append(warnings, fmt.Sprintf("「%s」出现在任何章之前，已忽略", sp.title))
			continue
		}
		ch.Sort = len(nodes[cur].Children)
		nodes[cur].Children = append(nodes[cur].Children, ch)
	}
	for i := range nodes {
		nodes[i].Chapter.Sort = i
	}

	if err := s.books.ReplaceChapterTree(ctx, book.ID, nodes); err != nil {
		return nil, err
	}

	rows, err := s.books.ChapterRows(ctx, []int64{book.ID})
	if err != nil {
		return nil, err
	}
	tree, count := buildChapterTree(rows, true)
	return &dto.ToolChapterOutlineResult{Chapters: tree, Count: count, Warnings: warnings}, nil
}

// Outline 把现有目录导出成大纲文本，供后台编辑页回填。
//
// 有了它，运营改一行再保存就行，不必重新敲一遍整份大纲——
// 而没有回填的文本框会让人以为"这书还没有目录"。
func (s *ToolBookService) Outline(ctx context.Context, bookID int64) (*dto.ToolChapterOutlineDoc, error) {
	book, err := s.books.GetBook(ctx, bookID)
	if err != nil {
		return nil, err
	}
	rows, err := s.books.ChapterRows(ctx, []int64{book.ID})
	if err != nil {
		return nil, err
	}
	tree, count := buildChapterTree(rows, true)

	var b strings.Builder
	for _, ch := range tree {
		b.WriteString("# ")
		b.WriteString(ch.Title)
		b.WriteString("\n")
		for _, sub := range ch.Children {
			b.WriteString("## ")
			b.WriteString(sub.Title)
			if ref := sub.ArticleSlug; ref != "" {
				b.WriteString(" | ")
				b.WriteString(ref)
			} else if sub.URL != "" {
				b.WriteString(" | ")
				b.WriteString(sub.URL)
			}
			if sub.Summary != "" {
				b.WriteString(" | ")
				b.WriteString(sub.Summary)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return &dto.ToolChapterOutlineDoc{Outline: b.String(), Count: count}, nil
}

// ============================ 内部 ============================

// outlineSpec 是大纲里的一行解析结果。
type outlineSpec struct {
	isChapter bool
	title     string
	ref       string
	summary   string
}

// parseChapterOutline 解析大纲文本。
//
// 格式（三种行首标记都认，照顾不同人的书写习惯）：
//
//	# 章标题
//	## 节标题 | 文章 slug 或外链 | 导语
//	- 节标题（用减号写大纲是最常见的习惯，等价于 ##）
//
// 解析不碰数据库：链接解析由 resolveChapterRef 做，两者分开才好单测
// 「缩进/空行/全角竖线」这类纯文本问题。
func parseChapterOutline(raw string) ([]outlineSpec, []string) {
	specs := make([]outlineSpec, 0, 12)
	warnings := make([]string, 0)

	text := strings.ReplaceAll(raw, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for i, line := range strings.Split(text, "\n") {
		lineno := i + 1
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}

		isChapter := false
		switch {
		// 先判两个 # 再判一个，反了的话 "## 节" 会被当成章。
		case strings.HasPrefix(t, "## "):
			t = strings.TrimSpace(strings.TrimPrefix(t, "## "))
		case strings.HasPrefix(t, "# "):
			isChapter = true
			t = strings.TrimSpace(strings.TrimPrefix(t, "# "))
		case strings.HasPrefix(t, "- "), strings.HasPrefix(t, "* "), strings.HasPrefix(t, "+ "):
			t = strings.TrimSpace(t[2:])
		case strings.HasPrefix(t, "#"):
			// "#章标题"（漏了空格）也认，但 "###" 之类三级标记说明这人想写三层，
			// 二级封顶下只能提示他改。
			if strings.HasPrefix(t, "###") {
				warnings = append(warnings, fmt.Sprintf("第 %d 行：目录只有两级，三级标题已忽略", lineno))
				continue
			}
			isChapter = true
			t = strings.TrimSpace(strings.TrimLeft(t, "#"))
		default:
			warnings = append(warnings, fmt.Sprintf("第 %d 行不像章节，已忽略：%s", lineno, truncateRunes(t, 24)))
			continue
		}

		parts := splitOutlineParts(t)
		title := strings.TrimSpace(parts[0])
		if title == "" {
			warnings = append(warnings, fmt.Sprintf("第 %d 行没有标题，已忽略", lineno))
			continue
		}
		if runeLen(title) > 200 {
			return nil, []string{fmt.Sprintf("第 %d 行的标题超过 200 字", lineno)}
		}

		sp := outlineSpec{isChapter: isChapter, title: title}
		if len(parts) > 1 {
			sp.ref = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			sp.summary = strings.TrimSpace(parts[2])
			if runeLen(sp.summary) > 500 {
				sp.summary = truncateRunes(sp.summary, 500)
			}
		}
		specs = append(specs, sp)
	}
	return specs, warnings
}

// splitOutlineParts 按竖线切分标题 / 链接 / 导语。
//
// 全角竖线一并接受：中文输入法下打出来的就是「｜」，
// 要求用户切到半角才能保存是纯粹的折磨。
func splitOutlineParts(s string) []string {
	s = strings.ReplaceAll(s, "｜", "|")
	return strings.Split(s, "|")
}

// resolveChapterRef 把一行里的链接解析成站内文章或外链。
//
// 返回值三选一：articleID>0 / link 非空 / warn 非空（此时两个都为空，
// 调用方保留标题、去掉链接）。warn 不阻断保存，这是刻意的。
func (s *ToolBookService) resolveChapterRef(ctx context.Context, ref string) (int64, string, string) {
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		link, err := normalizeURL(ref, 512, "外链")
		if err != nil {
			return 0, "", "外链地址不合法，已去掉链接：" + truncateRunes(ref, 40)
		}
		return 0, link, ""
	}
	if strings.ContainsAny(ref, " /?#") {
		return 0, "", fmt.Sprintf("「%s」既不是站内文章 slug 也不是 http(s) 外链，已去掉链接", truncateRunes(ref, 40))
	}
	published, err := s.books.ArticleRefBySlug(ctx, ref)
	if err != nil {
		return 0, "", fmt.Sprintf("找不到 slug 为「%s」的文章，已保留标题但去掉链接", ref)
	}
	if published.Status != model.ArticleStatusPublished {
		// 挂草稿文章不报错：运营常常先排好目录、再逐篇成稿。
		// 前台会跳过它，后台标红提醒。
		logger.Warn("教程集引用了未发布文章", "slug", ref, "status", published.Status)
	}
	return published.ID, "", ""
}

// groupChapterRows 按书分组。
func groupChapterRows(rows []repository.ChapterRow) map[int64][]repository.ChapterRow {
	grouped := make(map[int64][]repository.ChapterRow, 4)
	for _, r := range rows {
		grouped[r.BookID] = append(grouped[r.BookID], r)
	}
	return grouped
}

// buildChapterTree 把平铺的章节行组装成两级树，并返回**可见节点数**。
//
// forAdmin 决定失效节点怎么处理：
//   - 前台（false）：正文文章被删或下线的节点整条跳过——目录里挂一个
//     点进去 404 的链接，比这块内容不存在更伤；
//   - 后台（true）：保留并标 Broken。看不到就改不了，运营只能看着
//     一条配置永远不生效。
func buildChapterTree(rows []repository.ChapterRow, forAdmin bool) ([]dto.ToolChapterItem, int) {
	children := make(map[int64][]repository.ChapterRow, len(rows))
	chapters := make([]repository.ChapterRow, 0, len(rows)/3+1)
	for _, r := range rows {
		if r.ParentID == 0 {
			chapters = append(chapters, r)
			continue
		}
		children[r.ParentID] = append(children[r.ParentID], r)
	}
	sort.SliceStable(chapters, func(i, j int) bool {
		if chapters[i].Sort != chapters[j].Sort {
			return chapters[i].Sort < chapters[j].Sort
		}
		return chapters[i].ID < chapters[j].ID
	})

	out := make([]dto.ToolChapterItem, 0, len(chapters))
	count := 0
	for _, ch := range chapters {
		item, ok := chapterItem(ch, forAdmin)
		if !ok {
			continue
		}
		subs := make([]dto.ToolChapterItem, 0, len(children[ch.ID]))
		kids := children[ch.ID]
		sort.SliceStable(kids, func(i, j int) bool {
			if kids[i].Sort != kids[j].Sort {
				return kids[i].Sort < kids[j].Sort
			}
			return kids[i].ID < kids[j].ID
		})
		for _, sub := range kids {
			if it, ok2 := chapterItem(sub, forAdmin); ok2 {
				subs = append(subs, it)
			}
		}
		// 前台：章自己没正文、子节点又全失效 → 整章不展示（空标题没有意义）。
		if !forAdmin && item.ArticleID == 0 && item.URL == "" && len(subs) == 0 {
			continue
		}
		item.Children = subs
		count += 1 + len(subs)
		out = append(out, item)
	}
	return out, count
}

// chapterItem 把一行转成目录节点。第二个返回值表示是否展示。
func chapterItem(r repository.ChapterRow, forAdmin bool) (dto.ToolChapterItem, bool) {
	item := dto.ToolChapterItem{
		ID: r.ID, Title: r.Title, Sort: r.Sort, Summary: r.Summary,
	}
	if r.ArticleID > 0 {
		item.ArticleID = r.ArticleID
		visible := r.ArticleSlug != "" && r.ArticleStatus == model.ArticleStatusPublished
		if visible {
			item.ArticleSlug = r.ArticleSlug
			// 标题取文章的当前标题，而不是录入目录时的快照：
			// 作者改了标题，目录该跟着变，否则两处对不上。
			item.Title = r.ArticleTitle
			if item.Summary == "" {
				item.Summary = r.ArticleSummary
			}
			return item, true
		}
		if forAdmin {
			item.Broken = true
			if item.Title == "" {
				item.Title = r.ArticleTitle
			}
			return item, true
		}
		return item, false
	}
	if r.ExternalURL != "" {
		item.URL = r.ExternalURL
		return item, true
	}
	// 没有任何链接：仍然展示标题。目录结构本身就是内容，
	// 「第 3 章 排错」这种标题对读者有价值，只是点不动。
	return item, true
}

// toBookItem 把模型转成列表项。
func toBookItem(b model.ToolBook, count int) dto.ToolBookItem {
	return dto.ToolBookItem{
		ID: b.ID, ToolID: b.ToolID, Title: b.Title, Summary: b.Summary, Cover: b.Cover,
		Level: b.Level, Status: b.Status, Sort: b.Sort,
		ChapterCount: count, PublishedAt: b.PublishedAt,
	}
}

// normalizeBookLevel 校验难度，空串按入门处理。
func normalizeBookLevel(raw string) (string, error) {
	level := strings.ToLower(strings.TrimSpace(raw))
	if level == "" {
		return model.ToolBookLevelBeginner, nil
	}
	if !model.IsValidToolBookLevel(level) {
		return "", errcode.ErrParamInvalid.WithMsg("难度只能是 beginner / advanced / expert")
	}
	return level, nil
}
