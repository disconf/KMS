package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

// minFetchInterval 是两次真实抓取之间的最小间隔。
//
// 调度器的 interval 是「多久唤醒一次这个任务」，抓取节奏是「多久真的去拉一次源」，
// 两者语义不同：订单相关任务需要分钟级扫描，而资讯源二十分钟一次就够，
// 更频繁只会消耗对方站点带宽，还可能被限流封 UA。
// 所以任务每次被唤醒都先问一句「到点了吗」，这个判断只写在这一处。
const minFetchInterval = 20 * time.Minute

// NewsService 负责运维资讯的抓取与订阅源管理。
//
// 抓取的产品判断：自动抓取解决的是「不用人去找」，不是「不用人看」。
// 所以默认抓到的东西进 pending 待采用池，后台过一眼再上线；
// 只有对特别信任的官方源才把 autoPublish 打开。
// 直接把第三方标题推上首页，一次翻车就要靠删稿挽回，而资讯的价值周期只有几天。
type NewsService struct {
	news       *repository.NewsRepo
	categories *repository.CategoryRepo
	// perSourceLimit 每个源每轮最多采用的条数。
	// 首次接入一个活跃源时可能一次吐出上百条历史条目，
	// 全量灌进待审池等于把审核队列淹掉，反而没人看了。
	perSourceLimit int

	mu        sync.Mutex
	lastFetch time.Time
}

// NewNewsService 创建 NewsService。
func NewNewsService(news *repository.NewsRepo, categories *repository.CategoryRepo) *NewsService {
	return &NewsService{news: news, categories: categories, perSourceLimit: 8}
}

// ============================ 抓取 ============================

// FetchResult 是单个源的一轮抓取结果。
type FetchResult struct {
	SourceID int64
	Name     string
	Written  int
	Err      error
}

// FetchAll 抓取所有启用的订阅源。
//
// 单个源失败不影响其他源：资讯源里总有几个会挂（改域名、临时 502、被墙），
// 一个失败就让整轮任务红掉，等于把「部分可用」误报成「全不可用」。
// 失败信息落到订阅源的 last_error 上，后台一眼能看到坏的是哪个。
func (s *NewsService) FetchAll(ctx context.Context) (int, error) {
	// 先节流再看有没有源：判断成本与源数量无关，放在最前面能省掉一次查询。
	s.mu.Lock()
	if !s.lastFetch.IsZero() && time.Since(s.lastFetch) < minFetchInterval {
		s.mu.Unlock()
		return 0, nil
	}
	s.lastFetch = time.Now()
	s.mu.Unlock()

	sources, err := s.news.ListSources(ctx, true)
	if err != nil {
		return 0, err
	}
	if len(sources) == 0 {
		return 0, nil
	}

	total := 0
	for _, src := range sources {
		res := s.FetchSource(ctx, &src)
		if res.Err != nil {
			logger.Warn("资讯源抓取失败", "source", res.Name, "err", res.Err.Error())
			continue
		}
		total += res.Written
	}
	logger.Info("资讯抓取完成", "sources", len(sources), "written", total)
	return total, nil
}

// FetchSource 抓取单个源并落库。
func (s *NewsService) FetchSource(ctx context.Context, src *model.NewsSource) FetchResult {
	res := FetchResult{SourceID: src.ID, Name: src.Name}

	items, err := s.pull(ctx, src)
	if err != nil {
		res.Err = err
		// 状态写失败也不能盖掉真正的抓取错误，否则后台看到的永远是「数据库错误」。
		if e := s.news.SetSourceStatus(ctx, src.ID, "error", truncateRunes(err.Error(), 480), 0); e != nil {
			logger.Warn("记录资讯源失败状态时出错", "source", src.Name, "err", e.Error())
		}
		return res
	}

	now := time.Now()
	written := 0
	for i, item := range items {
		if i >= s.perSourceLimit {
			break
		}
		article := s.buildArticle(src, item, now)
		ok, err := s.news.SaveFeedArticle(ctx, article)
		if err != nil {
			res.Err = err
			_ = s.news.SetSourceStatus(ctx, src.ID, "error", truncateRunes(err.Error(), 480), int64(written))
			return res
		}
		if ok {
			written++
		}
	}

	res.Written = written
	if err := s.news.SetSourceStatus(ctx, src.ID, "ok", "", int64(written)); err != nil {
		res.Err = err
	}
	return res
}

// pull 按源类型取回并解析条目。
func (s *NewsService) pull(ctx context.Context, src *model.NewsSource) ([]FeedItem, error) {
	url := strings.TrimSpace(src.URL)
	if url == "" {
		return nil, fmt.Errorf("订阅源未配置地址")
	}

	switch src.Kind {
	case model.NewsSourceGitHub:
		// 允许运营直接填 https://github.com/owner/repo，这里归一成 owner/repo。
		repo := normalizeRepo(url)
		if repo == "" {
			return nil, fmt.Errorf("GitHub 源地址应形如 owner/repo")
		}
		api := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=10", repo)
		body, err := s.get(ctx, api, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}
		defer body.Close()
		return ParseGitHubReleases(repo, body)

	default: // 未识别的类型按 RSS 处理，比直接报错更宽容
		body, err := s.get(ctx, url, "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")
		if err != nil {
			return nil, err
		}
		defer body.Close()
		return ParseFeed(body)
	}
}

func (s *NewsService) get(ctx context.Context, url, accept string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", feedUserAgent)
	req.Header.Set("Accept", accept)

	resp, err := feedHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("请求 %s 返回 HTTP %d", url, resp.StatusCode)
	}
	// 限制读取体积：源被劫持或配置错指向了一个大文件时，
	// 不设上限会直接把进程内存吃满。
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, 4<<20), resp.Body}, nil
}

// buildArticle 把一条订阅条目转成待审文章。
func (s *NewsService) buildArticle(src *model.NewsSource, item FeedItem, now time.Time) *model.Article {
	extID := item.ExternalID
	summary := item.Summary
	if summary == "" {
		summary = item.Title
	}

	content := summary + "\n\n---\n\n" +
		fmt.Sprintf("> 本条由「%s」自动采集，仅保留摘要与原文链接，版权归原作者所有。\n", src.Name) +
		fmt.Sprintf("> 原文：[%s](%s)", item.Title, item.Link)

	a := &model.Article{
		Title:      item.Title,
		Slug:       feedSlug(item),
		Summary:    truncateRunes(summary, 300),
		Content:    content,
		CategoryID: src.CategoryID,
		Tags:       src.Tags,
		Channel:    model.ChannelOpsNews,
		Source:     model.ArticleSourceFeed,
		SourceName: src.Name,
		SourceURL:  item.Link,
		ExternalID: &extID,
		Status:     model.ArticleStatusPending,
	}
	// 自动发布只放行中文条目。
	//
	// 站点是对外的中文站，autoPublish 的本意是「官方源不用人看」，
	// 不是「英文原文直接上首页」。英文条目标题要么是长句、要么是 release notes，
	// 前台一屏英文标题既不像中文站，也没人愿意点。非中文一律进待审池，
	// 由运营决定是编译成中文还是放弃——这条判断放在唯一的入库口，
	// 以后再加英文源也不会绕过。
	if src.AutoPublish && hasChinese(a.Title) {
		a.Status = model.ArticleStatusPublished
		// 只有真正上线才记发布时间：待审状态下写发布时间，
		// 列表按时间排序时会把还没过审的东西排到最前面。
		published := now
		if item.PublishedAt != nil {
			published = *item.PublishedAt
		}
		a.PublishedAt = &published
	}
	return a
}

// hasChinese 判断字符串里是否有汉字。
//
// 用「有汉字」而不是「不是纯英文」：日文/韩文同样是非中文，
// 放行它们等于把另一种看不懂的语言搬上首页。
func hasChinese(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// feedSlug 由标题派生 slug，并挂上 external_id 的短哈希。
//
// 后缀不是装饰：slug 与 external_id 都是唯一索引，
// 不同源里同名文章（比如两个站都发「Kubernetes 1.30 发布」）撞 slug 时，
// 插入会被判成「已存在」而静默丢掉——那正是我们最想收录的重复报道。
func feedSlug(item FeedItem) string {
	base := truncateSlug(sanitizeSlug(item.Title), 100)
	if base == "" {
		base = "news"
	}
	sum := sha1.Sum([]byte(item.ExternalID))
	return base + "-" + hex.EncodeToString(sum[:3])
}

// normalizeRepo 把各种 GitHub 地址写法归一成 owner/repo。
func normalizeRepo(raw string) string {
	v := strings.TrimSpace(raw)
	v = strings.TrimPrefix(v, "https://github.com/")
	v = strings.TrimPrefix(v, "http://github.com/")
	v = strings.TrimSuffix(v, "/")
	v = strings.TrimSuffix(v, ".git")
	parts := strings.Split(v, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// ============================ 订阅源管理 ============================

// AdminListSources 后台订阅源列表。
func (s *NewsService) AdminListSources(ctx context.Context) ([]dto.NewsSourceItem, error) {
	rows, err := s.news.ListSources(ctx, false)
	if err != nil {
		return nil, err
	}
	pending, err := s.news.CountPendingNews(ctx)
	if err != nil {
		pending = 0
	}
	out := make([]dto.NewsSourceItem, 0, len(rows))
	for i := range rows {
		out = append(out, dto.NewsSourceItem{
			ID:            rows[i].ID,
			Name:          rows[i].Name,
			URL:           rows[i].URL,
			Kind:          rows[i].Kind,
			Enabled:       rows[i].Enabled,
			AutoPublish:   rows[i].AutoPublish,
			LastFetchedAt: rows[i].LastFetchedAt,
			LastStatus:    rows[i].LastStatus,
			LastError:     rows[i].LastError,
			FetchedCount:  rows[i].FetchedCount,
			// 同一个数字挂在每个源上有点冗余，但前端拿到就能直接显示角标，
			// 省掉一次单独的统计请求。
			PendingTotal: pending,
		})
	}
	return out, nil
}

// CreateSource 新增订阅源。
func (s *NewsService) CreateSource(ctx context.Context, req dto.NewsSourceReq) (*dto.NewsSourceItem, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errcode.ErrParamInvalid.WithMsg("来源名称不能为空")
	}
	if err := validateSourceKind(req.Kind); err != nil {
		return nil, err
	}
	taken, err := s.news.ExistsSourceName(ctx, name, 0)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, errcode.ErrParamInvalid.WithMsg("来源名称 " + name + " 已存在")
	}

	// 没显式传 enabled 时默认启用：加一个源的目的就是要抓它，
	// 建完还得再点一次「启用」纯属多余。
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	src := &model.NewsSource{
		Name:        name,
		URL:         strings.TrimSpace(req.URL),
		Kind:        normalizeSourceKind(req.Kind),
		CategoryID:  req.CategoryID,
		Tags:        model.StringList(req.Tags),
		Enabled:     enabled,
		AutoPublish: req.AutoPublish,
		Sort:        req.Sort,
	}
	if err := s.news.CreateSource(ctx, src); err != nil {
		return nil, err
	}
	items, err := s.AdminListSources(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == src.ID {
			return &items[i], nil
		}
	}
	return nil, errcode.ErrNotFound
}

// UpdateSource 更新订阅源。
func (s *NewsService) UpdateSource(ctx context.Context, id int64, req dto.NewsSourceUpdateReq) (*dto.NewsSourceItem, error) {
	if _, err := s.news.GetSource(ctx, id); err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, errcode.ErrParamInvalid.WithMsg("来源名称不能为空")
		}
		taken, err := s.news.ExistsSourceName(ctx, name, id)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, errcode.ErrParamInvalid.WithMsg("来源名称 " + name + " 已存在")
		}
		fields["name"] = name
	}
	if req.URL != nil {
		fields["url"] = strings.TrimSpace(*req.URL)
	}
	if req.Kind != nil {
		if err := validateSourceKind(*req.Kind); err != nil {
			return nil, err
		}
		fields["kind"] = normalizeSourceKind(*req.Kind)
	}
	if req.CategoryID != nil {
		if *req.CategoryID > 0 {
			if _, err := s.categories.GetByID(ctx, *req.CategoryID); err != nil {
				return nil, err
			}
		}
		fields["category_id"] = *req.CategoryID
	}
	if req.Tags != nil {
		fields["tags"] = model.StringList(*req.Tags)
	}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
	}
	if req.AutoPublish != nil {
		fields["auto_publish"] = *req.AutoPublish
	}
	if req.Sort != nil {
		fields["sort"] = *req.Sort
	}
	if err := s.news.UpdateSourceFields(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.sourceItem(ctx, id)
}

// DeleteSource 删除订阅源。
func (s *NewsService) DeleteSource(ctx context.Context, id int64) error {
	if _, err := s.news.GetSource(ctx, id); err != nil {
		return err
	}
	return s.news.DeleteSource(ctx, id)
}

// FetchNow 手动触发一次抓取，供后台「立即抓取」按钮使用。
func (s *NewsService) FetchNow(ctx context.Context, id int64) (*dto.FetchResultItem, error) {
	src, err := s.news.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	res := s.FetchSource(ctx, src)
	out := &dto.FetchResultItem{SourceID: res.SourceID, Name: res.Name, Written: res.Written}
	if res.Err != nil {
		out.Error = res.Err.Error()
	}
	return out, nil
}

func (s *NewsService) sourceItem(ctx context.Context, id int64) (*dto.NewsSourceItem, error) {
	items, err := s.AdminListSources(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, errcode.ErrNotFound
}

func normalizeSourceKind(kind string) string {
	if strings.TrimSpace(kind) == model.NewsSourceGitHub {
		return model.NewsSourceGitHub
	}
	return model.NewsSourceRSS
}

func validateSourceKind(kind string) error {
	switch strings.TrimSpace(kind) {
	case "", model.NewsSourceRSS, model.NewsSourceGitHub:
		return nil
	default:
		return errcode.ErrParamInvalid.WithMsg("订阅源类型只能是 rss 或 github")
	}
}
