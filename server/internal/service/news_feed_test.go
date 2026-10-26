package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 这组测试守两件事：资讯标题必须是中文、版本类型别判反。
//
// 前者是产品红线——站点是中文站，英文 release notes 直接上首页要被人截图吐槽；
// 后者影响读者判断要不要升级，判反了（补丁版说成功能版）比不说更糟。

func TestReleaseTitleIsChinese(t *testing.T) {
	assert.Equal(t, "Kubernetes 发布 v1.37.1", releaseTitle("kubernetes/kubernetes", "v1.37.1"))
	assert.Equal(t, "Grafana 发布 13.2.4", releaseTitle("grafana/grafana", "13.2.4"))
	assert.Equal(t, "OpenResty 发布 v1.27.1.2", releaseTitle("openresty/openresty", "v1.27.1.2"))

	// 不认识的仓库回退 owner/repo，不硬造中文名
	assert.Equal(t, "acme/widget 发布 v0.1.0", releaseTitle("acme/widget", "v0.1.0"))
	// 四种已知仓库全部能落到中文标题
	for _, repo := range []string{
		"kubernetes/kubernetes", "prometheus/prometheus", "grafana/grafana", "openresty/openresty",
	} {
		assert.True(t, hasChinese(releaseTitle(repo, "v1.0.0")), repo+" 的标题应为中文")
	}
}

func TestReleaseSummaryDistinguishesFeatureAndPatch(t *testing.T) {
	assert.Contains(t, releaseSummaryText("kubernetes/kubernetes", "v1.37.0", nil), "功能版本")
	assert.Contains(t, releaseSummaryText("kubernetes/kubernetes", "v1.37.2", nil), "维护版本")
	// 四段版本号（OpenResty 的 nginx 版本 + 自建版本）按维护版本处理
	assert.Contains(t, releaseSummaryText("openresty/openresty", "v1.27.1.2", nil), "维护版本")
	// 摘要里必须出现产品名与版本号，读者扫一眼就知道是什么
	s := releaseSummaryText("prometheus/prometheus", "3.15.0", nil)
	assert.Contains(t, s, "Prometheus")
	assert.Contains(t, s, "3.15.0")
	// 不能把英文原文塞回来
	assert.False(t, strings.Contains(s, "release notes"))
}

func TestIsFeatureRelease(t *testing.T) {
	assert.True(t, isFeatureRelease("v1.37.0"))
	assert.True(t, isFeatureRelease("3.15.0"))
	assert.False(t, isFeatureRelease("v1.37.1"))
	assert.False(t, isFeatureRelease("13.2.4"))
	assert.False(t, isFeatureRelease("v1.27.1.2"))
	// 段数不足或非数字一律按维护版本处理：宁可少说，不要错说
	assert.False(t, isFeatureRelease("v1.37"))
	assert.False(t, isFeatureRelease("latest"))
}

func TestHasChinese(t *testing.T) {
	assert.True(t, hasChinese("Kubernetes 发布 v1.37.1"))
	assert.False(t, hasChinese("Kubernetes v1.37.1 released"))
	// 日文汉字也算汉字，但这只用于「是否放行自动发布」，
	// 混排里只要出现汉字就说明有人处理过，不必额外排除假名。
	assert.True(t, hasChinese("サーバー 監視"))
}
