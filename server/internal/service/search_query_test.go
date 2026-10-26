package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这组测试守的是「用户打一句大白话，能不能翻译成能落地的检索词」。
// 它比端到端冒烟更该先跑：检索不准的根因几乎都在这一层，
// 而要造出「问句 → 命中」的端到端场景，成本比直接断言解析结果高得多。

func planOf(t *testing.T, raw string) SearchPlan {
	t.Helper()
	return PlanSearch(raw)
}

func TestNormalizeSearchTextCollapsesWidthAndPunctuation(t *testing.T) {
	// 全角字母数字必须归一：中文输入法下打出的「Ｋ８Ｓ」若不转半角，
	// 和库里的 k8s 永远匹配不上。
	assert.Equal(t, "k8s 重启", normalizeSearchText("Ｋ８Ｓ，重启"))
	assert.Equal(t, "cpu 使用率", normalizeSearchText("ＣＰＵ　使用率"))
	// 连续标点与空白压成一个空格，避免切出一堆空 token。
	assert.Equal(t, "a b", normalizeSearchText("a ，。、 　b"))
	// 技术名词里的 - _ . + # 是词内字符，不能被当成分隔符切碎。
	assert.Equal(t, "k8s-operator", normalizeSearchText("k8s-operator"))
	assert.Equal(t, "c++", normalizeSearchText("C++"))
}

func TestPlanSearchStripsQuestionWords(t *testing.T) {
	p := planOf(t, "磁盘满了怎么办")

	// 「满了」「怎么办」是纯干扰：真正有检索价值的是「磁盘」。
	// 「满」被当作单字噪音丢掉，避免把一个过宽的词带进 OR。
	assert.Equal(t, []string{"磁盘"}, p.Terms)
	require.False(t, p.IsEmpty())
	assert.Contains(t, p.Expanded, "硬盘", "同义词应当被展开")
	assert.Contains(t, p.Expanded, "disk")
}

func TestPlanSearchStripsLeadingPoliteness(t *testing.T) {
	// 「请问」「我的」这类客套词在真实提问里出现率极高，却不携带任何检索信息。
	p := planOf(t, "请问我的内存为什么一直涨")
	assert.Contains(t, p.Terms, "内存")
	assert.NotContains(t, p.Terms, "请问")
	assert.NotContains(t, p.Terms, "我的")
	assert.NotContains(t, p.Terms, "为什么")
}

func TestPlanSearchSegmentsCompoundHan(t *testing.T) {
	// 「数据库主从延迟」整串去 LIKE 一条都搜不到，必须切成词典里的词。
	p := planOf(t, "数据库主从延迟")
	assert.Contains(t, p.Terms, "数据库")
	assert.Contains(t, p.Terms, "主从")
}

func TestPlanSearchGreedyMatchKeepsLongestWord(t *testing.T) {
	// 最长匹配：词典里同时有「磁盘」和「磁盘空间」，应当优先吃下更长的那个，
	// 否则会切出「磁盘」+「空间」两个组，其中「磁盘」那一组反而更宽、更不准。
	p := planOf(t, "磁盘空间不足")
	assert.Equal(t, []string{"磁盘空间"}, p.Terms)
	// 但展开时仍要把相关说法带上，不能因为切得长就丢掉其它叫法。
	assert.Contains(t, p.Expanded, "硬盘")
	assert.Contains(t, p.Expanded, "disk")
}

func TestPlanSearchKeepsUnknownRunAsWhole(t *testing.T) {
	// 词典没收的词按整串保留——总不能因为没收录就什么都不搜。
	// 这里刻意选一个连子串都命中不了词典的词，否则最长匹配会先吃掉已知的那部分。
	p := planOf(t, "麒麟芯片")
	assert.Equal(t, []string{"麒麟芯片"}, p.Terms)
}

func TestPlanSearchDropsLongUnknownRun(t *testing.T) {
	// 过长的未知串既不可能是关键词，整串 LIKE 也不可能命中，留着只会
	// 让严格模式永远为零、白白把每次检索都降级成宽松模式。
	p := planOf(t, "一二三四五六七八九十")
	assert.True(t, p.IsEmpty(), "超长未知串应当被丢弃")
}

func TestPlanSearchMergesSameSynonymGroup(t *testing.T) {
	// 「k8s」与「集群」指向同一组概念，只应产出一组条件，
	// 否则 SQL 里会出现两遍完全相同的 AND，纯属浪费。
	p := planOf(t, "k8s 集群")
	require.Len(t, p.Groups, 1)
	assert.Contains(t, p.Groups[0], "kubernetes")
}

func TestPlanSearchKeepsDistinctConceptsAsSeparateGroups(t *testing.T) {
	p := planOf(t, "磁盘 内存")
	// 组间是 AND 关系，两个概念必须各占一组，合并了就变成「任一命中」。
	require.Len(t, p.Groups, 2)
}

func TestPlanSearchExpandsEnglishToChinese(t *testing.T) {
	// 用户用英文、库里的内容用中文（或反过来）是常态，
	// 展开方向必须双向打通。
	p := planOf(t, "oom")
	assert.Contains(t, p.Terms, "oom")
	assert.Contains(t, p.Expanded, "内存")
}

func TestPlanSearchDropsEnglishStopwords(t *testing.T) {
	p := planOf(t, "how to fix disk full")
	assert.NotContains(t, p.Terms, "how")
	assert.NotContains(t, p.Terms, "to")
	assert.Contains(t, p.Terms, "disk")
}

func TestPlanSearchEmptyInput(t *testing.T) {
	for _, raw := range []string{"", "   ", "？", "的了吗呢"} {
		p := planOf(t, raw)
		assert.True(t, p.IsEmpty(), "输入 %q 不应产出检索条件", raw)
		assert.Empty(t, p.Terms)
	}
}

func TestPlanSearchSingleCharNoiseIsDropped(t *testing.T) {
	// 单字既是标点噪声（「了」），也可能是过宽的关键词（「满」）。
	// 两种都不该进检索条件。
	p := planOf(t, "满了")
	assert.True(t, p.IsEmpty())
}

func TestPlanSearchExpandedHasNoDuplicate(t *testing.T) {
	p := planOf(t, "磁盘磁盘 硬盘")
	seen := map[string]bool{}
	for _, v := range p.Expanded {
		key := strings.ToLower(v)
		require.False(t, seen[key], "检索词 %q 重复出现", v)
		seen[key] = true
	}
}

func TestPlanSearchSplitsMixedScriptToken(t *testing.T) {
	// 中英混排不能整串当成汉字去查词典——那样英文前缀会被直接跳过，
	// 「cpu占用高」里 cpu 这个有效概念就丢了，用户搜它只会拿到零结果。
	p := planOf(t, "cpu占用高怎么办")
	assert.Contains(t, p.Terms, "cpu")
	assert.NotContains(t, p.Terms, "怎么办")
	// 「占用高」不是词，词典里收了「占用」就能切出来；
	// 不收的话整串保留会是个谁都命中不了的死词。
	assert.Contains(t, p.Terms, "占用")
	// cpu 与占用同一组，组间去重后只剩一个条件——AND 两个等价条件毫无意义。
	require.Len(t, p.Groups, 1)
	// 英文概念同样要展开到中文，否则库里写「负载」「使用率」的内容搜不到。
	assert.Contains(t, p.Expanded, "负载")

	p2 := planOf(t, "k8s集群怎么部署")
	assert.Contains(t, p2.Terms, "k8s")
	assert.Contains(t, p2.Terms, "集群")
	assert.Contains(t, p2.Terms, "部署")
}

func TestSplitByScript(t *testing.T) {
	assert.Equal(t, []string{"cpu", "占用高"}, splitByScript("cpu占用高"))
	assert.Equal(t, []string{"占", "cpu"}, splitByScript("占cpu"))
	// 技术名词里的 - _ . 不是分隔符，整段应当保持完整。
	assert.Equal(t, []string{"k8s-operator"}, splitByScript("k8s-operator"))
	assert.Empty(t, splitByScript(""))
}

func TestSearchGroupsFallsBackToRawKeyword(t *testing.T) {
	// 解析不出词时必须回落到原样匹配整串。若回落到"不过滤"，
	// 一句语气词就会静默返回全站内容。
	groups := SearchGroups("的了吗呢")
	require.Len(t, groups, 1)
	assert.Equal(t, []string{"的了吗呢"}, groups[0])

	// 超过长度上限的未知串同样提不出词，走同一条回落路径。
	groups = SearchGroups("一二三四五六七八九十")
	require.Len(t, groups, 1)
	assert.Equal(t, []string{"一二三四五六七八九十"}, groups[0])

	// 空白输入才是真的没有条件，应当交给调用方按"不筛选"处理。
	assert.Nil(t, SearchGroups("   "))
}

func TestSearchSynonymDictHasNoBlankEntry(t *testing.T) {
	// 词典里混进空串会让切分退化成「匹配到空词」，且永远匹配成功——
	// 这种错误在运行时完全看不出异常，只能靠这里拦住。
	for canon, variants := range searchSynonyms {
		assert.NotEmpty(t, strings.TrimSpace(canon), "规范名不能为空")
		for _, v := range variants {
			assert.NotEmpty(t, strings.TrimSpace(v), "概念 %q 下有空说法", canon)
		}
	}
}

func TestSearchFillersAreSortedByLengthDesc(t *testing.T) {
	// 顺序错了会留下残渣：若「什么」先于「为什么」被替换，
	// 「为什么」会被切成「为」+「么」，「么」还只剩单字。
	for i := 1; i < len(hanFillers); i++ {
		prev := len([]rune(hanFillers[i-1]))
		cur := len([]rune(hanFillers[i]))
		require.GreaterOrEqual(t, prev, cur,
			"hanFillers 必须按长度倒序：%q(%d) 排在 %q(%d) 之前",
			hanFillers[i-1], prev, hanFillers[i], cur)
	}
}
