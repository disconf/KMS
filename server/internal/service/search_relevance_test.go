package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
)

// 这组测试跑真实 SQLite，守的是「检索能不能真的把内容找出来」。
// 纯函数的解析测试只能证明词切对了，证明不了它落到 SQL 上还成立——
// 中间隔着列名、LIKE 转义、AND/OR 组合与排序表达式。

// createArticle 建一篇已发布文章，正文由用例指定。
func (f *articleFixture) createArticle(t *testing.T, title, slug, content string, tags []string) {
	t.Helper()
	_, err := f.svc.CreateArticle(context.Background(), f.adminID, dto.ArticleCreateReq{
		Title:      title,
		Slug:       slug,
		Content:    content,
		CategoryID: f.catK8s,
		Tags:       tags,
		Status:     model.ArticleStatusPublished,
	})
	require.NoError(t, err, "建文章失败: %s", title)
}

func (f *articleFixture) search(t *testing.T, keyword string) []dto.ArticleListItem {
	t.Helper()
	items, _, err := f.svc.ListArticles(context.Background(), dto.ArticleListQuery{
		PageQuery: dto.PageQuery{Page: 1, PageSize: 20},
		Keyword:   keyword,
	})
	require.NoError(t, err)
	return items
}

func TestSearchQuestionFindsBodyMatch(t *testing.T) {
	f := newArticleFixture(t)
	f.createArticle(t, "MySQL 主从延迟排查",
		"mysql-repl-lag", "常见原因之一是磁盘 I/O 打满，写入跟不上。", []string{"MySQL"})

	// 完整问句里只有「磁盘」是有检索价值的，其余都是疑问词。
	// 改之前这里是一条都搜不到的——正是要修的那个症状。
	items := f.search(t, "磁盘满了怎么办")
	require.Len(t, items, 1)
	assert.Equal(t, "MySQL 主从延迟排查", items[0].Title)
}

func TestSearchExpandsSynonymsToFindContent(t *testing.T) {
	f := newArticleFixture(t)
	// 正文里只有英文 disk，用户却按中文「硬盘」提问——同义词必须打通。
	f.createArticle(t, "磁盘告警处理", "disk-alert",
		"先看 disk 使用率，再定位大文件。", []string{"运维"})

	items := f.search(t, "硬盘")
	require.Len(t, items, 1)
	assert.Equal(t, "磁盘告警处理", items[0].Title)
}

func TestSearchTitleMatchRanksFirst(t *testing.T) {
	f := newArticleFixture(t)
	// 两篇都命中「重启」，但一篇是主题、一篇只是正文顺带提一句。
	f.createArticle(t, "K8s Pod 频繁重启排查手册", "pod-restart",
		"先看事件，再看上一次的日志。", []string{"Kubernetes"})
	f.createArticle(t, "DNS 解析超时处理", "dns-timeout",
		"排查步骤：先看日志，必要时重启服务。", []string{"DNS"})

	items := f.search(t, "重启")
	require.Len(t, items, 2)
	assert.Equal(t, "K8s Pod 频繁重启排查手册", items[0].Title,
		"标题命中应当排在正文顺带提及之前")
}

func TestSearchAllConceptsMustMatchFirst(t *testing.T) {
	f := newArticleFixture(t)
	// A 同时提到磁盘与内存，B 只提磁盘。
	f.createArticle(t, "磁盘空间告警", "disk-space",
		"磁盘空间接近 100%，同时内存使用率也偏高。", []string{"运维"})
	f.createArticle(t, "inode 用尽", "inode-exhausted",
		"磁盘还有余量但 inode 用尽。", []string{"运维"})

	// 严格模式应当先命中 A（两个概念都出现），此时不应把只沾一个词的 B 也带出来。
	items := f.search(t, "磁盘 内存")
	require.Len(t, items, 1)
	assert.Equal(t, "磁盘空间告警", items[0].Title)
}

func TestSearchFallsBackToAnyConcept(t *testing.T) {
	f := newArticleFixture(t)
	// 只提磁盘，不提内存——严格模式下必然零结果，必须能放宽到 OR。
	f.createArticle(t, "磁盘写满排查", "disk-full",
		"用 df 看使用率，再找大文件。", []string{"运维"})

	items := f.search(t, "磁盘 内存")
	require.Len(t, items, 1, "严格模式无结果时应放宽为 OR")
	assert.Equal(t, "磁盘写满排查", items[0].Title)
}

func TestSearchMatchesTagOnly(t *testing.T) {
	f := newArticleFixture(t)
	// 标题与正文都不含「k8s」，只有标签带 Kubernetes。
	// 标签是运营在维护的话题词，不该被排除在检索范围之外。
	f.createArticle(t, "Pod 调度策略详解", "pod-scheduling",
		"调度器按资源请求与亲和性选择节点。", []string{"Kubernetes"})

	items := f.search(t, "k8s")
	require.Len(t, items, 1)
	assert.Equal(t, "Pod 调度策略详解", items[0].Title)
}

func TestSearchWithoutKeywordReturnsAll(t *testing.T) {
	f := newArticleFixture(t)
	f.createArticle(t, "甲文章", "jia", "正文甲。", nil)
	f.createArticle(t, "乙文章", "yi", "正文乙。", nil)

	items := f.search(t, "")
	assert.Len(t, items, 2, "不传关键词时不该附加任何过滤")
}

func TestSearchNoMatchReturnsEmpty(t *testing.T) {
	f := newArticleFixture(t)
	f.createArticle(t, "甲文章", "jia", "正文甲。", nil)

	items := f.search(t, "完全不相关的词xyz")
	assert.Empty(t, items)
}

func TestSearchUnparsableQueryReturnsNothing(t *testing.T) {
	f := newArticleFixture(t)
	f.createArticle(t, "甲文章", "jia", "正文甲。", nil)

	// 解析不出检索词时若直接"不过滤"，用户搜一句语气词会拿到全站内容——
	// 这比搜不到更让人困惑：他会以为搜索坏了。
	assert.Empty(t, f.search(t, "的了吗呢"))
	assert.Empty(t, f.search(t, "？？？"))
}
