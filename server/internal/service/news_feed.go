package service

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// 本文件只做「把外部订阅源解析成统一条目」，不碰数据库、不碰业务。
// 拆出来的理由很实际：解析逻辑要靠单元测试守住（各家 feed 的字段五花八门），
// 而抓取逻辑要靠端到端验。混在一起后，测解析得先准备一个数据库。

// FeedItem 是订阅源里的一条条目，已归一化成入库需要的形状。
type FeedItem struct {
	// ExternalID 源内唯一标识：RSS 的 guid、Atom 的 id、GitHub release 的 id。
	// 去重全靠它，所以为空时宁可不入库也不能编一个——编出来的 ID 每次都不一样，
	// 同一篇文章会被反复写进去。
	ExternalID  string
	Title       string
	Link        string
	Summary     string
	PublishedAt *time.Time
}

// feedLink 兼容 RSS 的 <link>文本</link> 与 Atom 的 <link href="..."/>。
type feedLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Text string `xml:",chardata"`
}

func (l feedLink) value() string {
	if v := strings.TrimSpace(l.Text); v != "" {
		return v
	}
	return strings.TrimSpace(l.Href)
}

// rssItemXML 是 RSS 2.0 的 item。
type rssItemXML struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	// content:encoded 带命名空间，按 local name 匹配即可。
	Content string `xml:"encoded"`
	PubDate string `xml:"pubDate"`
	GUID    string `xml:"guid"`
}

// atomEntryXML 是 Atom 的 entry。
type atomEntryXML struct {
	Title     string     `xml:"title"`
	Links     []feedLink `xml:"link"`
	ID        string     `xml:"id"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
}

// feedXML 一个结构同时接住 RSS 2.0 与 Atom。
//
// 不用先判断是哪种格式：两者的根节点不同，另一个分支自然为空。
// 判断格式再分别解析会多一层分支，而收益只是省几个字节的反序列化。
type feedXML struct {
	XMLName xml.Name
	Channel struct {
		Title string       `xml:"title"`
		Items []rssItemXML `xml:"item"`
	} `xml:"channel"`
	Entries []atomEntryXML `xml:"entry"`
}

// ParseFeed 解析 RSS 2.0 / Atom 订阅内容。
//
// 返回的条目已按发布时间倒序（源里顺序不一定可靠），并丢弃没有标题或没有链接的条目：
// 没有链接的条目无法转到原文，留在站内等于把别人的内容抄一半。
func ParseFeed(r io.Reader) ([]FeedItem, error) {
	dec := xml.NewDecoder(r)
	// 不少国内源仍声明 gb2312/gbk。encoding/xml 默认只认 UTF-8，
	// 不挂 CharsetReader 会直接报 "unsupported charset"。
	dec.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		return charset.NewReaderLabel(label, input)
	}

	var doc feedXML
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析订阅内容失败: %w", err)
	}

	items := make([]FeedItem, 0, len(doc.Channel.Items)+len(doc.Entries))

	for _, it := range doc.Channel.Items {
		item := FeedItem{
			ExternalID:  strings.TrimSpace(it.GUID),
			Title:       plainText(it.Title, 200),
			Link:        strings.TrimSpace(it.Link),
			Summary:     plainText(pickFirst(it.Description, it.Content), 400),
			PublishedAt: parseFeedTime(it.PubDate),
		}
		if item.Title == "" || item.Link == "" {
			continue
		}
		// 没有 guid 的老式 RSS 用链接兜底：链接在同一个源里也基本唯一。
		if item.ExternalID == "" {
			item.ExternalID = item.Link
		}
		items = append(items, item)
	}

	for _, en := range doc.Entries {
		link := ""
		for _, l := range en.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.value()
				if link != "" {
					break
				}
			}
		}
		item := FeedItem{
			ExternalID:  strings.TrimSpace(en.ID),
			Title:       plainText(en.Title, 200),
			Link:        link,
			Summary:     plainText(pickFirst(en.Summary, en.Content), 400),
			PublishedAt: parseFeedTime(pickFirst(en.Published, en.Updated)),
		}
		if item.Title == "" || item.Link == "" {
			continue
		}
		if item.ExternalID == "" {
			item.ExternalID = item.Link
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("订阅内容里没有可用的条目（既不是 RSS 也不是 Atom？）")
	}

	// 按发布时间倒序，没有时间的排最后。抓取只取前 N 条，
	// 顺序错了会一直在处理陈年旧闻，新版本反而漏掉。
	sortByPublishedDesc(items)
	return items, nil
}

// githubRelease 是 GitHub Releases API 的响应条目（只声明用到的字段）。
type githubRelease struct {
	ID          int64  `json:"id"`
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

// ParseGitHubReleases 解析 GitHub Releases API 的响应。
//
// 跳过 draft 与 prerelease：版本号带 -rc/-beta 的预发布不该出现在资讯里，
// 运维看到「v1.30.0-rc.1 发布」跑去升级是要出事的。
//
// 标题与摘要在这里就写成中文：站点是中文站，自动采集不能把英文 release notes
// 直接摆到前台。原文地址仍然保留，需要看逐条 commit 的人点进去即可。
func ParseGitHubReleases(repo string, r io.Reader) ([]FeedItem, error) {
	var rows []githubRelease
	if err := json.NewDecoder(io.LimitReader(r, 4<<20)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("解析 GitHub Releases 失败: %w", err)
	}

	items := make([]FeedItem, 0, len(rows))
	for _, rel := range rows {
		if rel.Draft || rel.Prerelease {
			continue
		}
		tag := plainText(strings.TrimSpace(rel.TagName), 60)
		if tag == "" || rel.HTMLURL == "" {
			continue
		}
		at := parseFeedTime(rel.PublishedAt)
		items = append(items, FeedItem{
			// 加 repo 前缀：不同仓库的 release id 理论上可能重复，
			// 而 external_id 是全表唯一索引。
			ExternalID:  fmt.Sprintf("github:%s:%d", repo, rel.ID),
			Title:       releaseTitle(repo, tag),
			Link:        rel.HTMLURL,
			Summary:     releaseSummaryText(repo, tag, at),
			PublishedAt: at,
		})
	}
	return items, nil
}

// releaseProjectName 把仓库名换成站点惯用的产品名。
//
// 只映射有公认中文写法的产品，不认识的一律回退 owner/repo：
// 猜一个名字出来（比如把 grafana/grafana 写成「格拉法纳」）比显示仓库名更糟。
var releaseProjectName = map[string]string{
	"kubernetes/kubernetes":    "Kubernetes",
	"prometheus/prometheus":    "Prometheus",
	"grafana/grafana":          "Grafana",
	"openresty/openresty":      "OpenResty",
	"nginx/nginx":              "Nginx",
	"redis/redis":              "Redis",
	"mysql/mysql-server":       "MySQL",
	"elastic/elasticsearch":    "Elasticsearch",
	"moby/moby":                "Docker",
	"prometheus/node_exporter": "node_exporter",
	"hashicorp/terraform":      "Terraform",
	"ansible/ansible":          "Ansible",
	"argoproj/argo-cd":         "Argo CD",
	"grafana/loki":             "Loki",
	"jenkinsci/jenkins":        "Jenkins",
}

// releaseTitle 生成中文标题，形如「Kubernetes 发布 v1.37.1」。
func releaseTitle(repo, tag string) string {
	name := repo
	if n, ok := releaseProjectName[repo]; ok {
		name = n
	}
	return plainText(fmt.Sprintf("%s 发布 %s", name, tag), 120)
}

// releaseSummaryText 生成中文摘要。
//
// 刻意不搬运、也不机翻 release notes：版本条目的价值是「有新版本了」这个信号，
// 逐条改动让读者去发布页看原文；半机翻的技术描述反而会误导。
// 能给出的确定信息只有版本号与版本类型，那就只说这两件事。
func releaseSummaryText(repo, tag string, at *time.Time) string {
	name := repo
	if n, ok := releaseProjectName[repo]; ok {
		name = n
	}
	kind := "维护版本，以缺陷修复与安全更新为主"
	if isFeatureRelease(tag) {
		kind = "功能版本，通常包含新特性与行为变更，升级前先看兼容性说明"
	}
	if at != nil {
		return fmt.Sprintf("%s %s 已发布（%s），属于%s。完整变更记录与各平台安装包见发布页。",
			name, tag, at.Format("2006-01-02"), kind)
	}
	return fmt.Sprintf("%s %s 已发布，属于%s。完整变更记录与各平台安装包见发布页。", name, tag, kind)
}

// isFeatureRelease 判断是否功能版本：minor 之后的段位全为 0 才算。
//
// v1.37.0 / 3.15.0 → 功能版本；v1.37.1 / 13.2.4 / v1.27.1.2 → 维护版本。
// 只按「数字段」判断，非数字后缀（-rc.1 之类）已经在解析入口被过滤掉了。
func isFeatureRelease(tag string) bool {
	v := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return false
	}
	for _, p := range parts[2:] {
		n := 0
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil {
			return false
		}
		if n != 0 {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- 内部 --

var htmlTagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// plainText 把 HTML 片段压成单行纯文本。
//
// 两遍去标签是必要的：先脱原标签，反转义实体后可能又冒出标签
// （`&lt;script&gt;` 是转义后的写法），只做一遍就会把它当正文留下。
func plainText(s string, limit int) string {
	if s == "" {
		return ""
	}
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, limit)
}

func pickFirst(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// feedTimeLayouts 覆盖各家用过的写法。
// 顺序有意义：带时区的先试，最后才试不带时区的——
// 不带时区的时间会被当成本地时间，早试会得到错的时刻。
var feedTimeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func parseFeedTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range feedTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}

func sortByPublishedDesc(items []FeedItem) {
	// 手写插入排序：条数只有几十，且要「无时间的排最后」这一条额外规则，
	// 用 sort.Slice 还得再写一个三值比较，可读性反而更差。
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && publishedBefore(items[j-1], items[j]); j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}

// publishedBefore 判断 a 是否应当排在 b 之后（即 a 更旧）。
// 没有时间的条目一律视为最旧，沉到最后。
func publishedBefore(a, b FeedItem) bool {
	if a.PublishedAt == nil {
		return true
	}
	if b.PublishedAt == nil {
		return false
	}
	return a.PublishedAt.Before(*b.PublishedAt)
}

// feedHTTPClient 是抓取用的客户端。
//
// 超时必须显式设：默认的 http.Client 没有超时，
// 一个不响应的源会把整个抓取任务拖到 job 超时，后面所有源都轮不到。
var feedHTTPClient = &http.Client{Timeout: 12 * time.Second}

// feedUserAgent 如实说明自己是谁。伪装成浏览器抓取既没必要也不礼貌，
// 多数技术站会按 UA 提供 feed 的缓存友好版本。
const feedUserAgent = "OpsExpertBot/1.0 (+https://github.com/opsexpert; ops content aggregation)"
