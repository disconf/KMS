package service

import (
	"sort"
	"strings"
	"unicode"

	"github.com/opsexpert/platform/internal/repository"
)

// ============================ 查询理解 ============================
//
// 用户搜的是「磁盘满了怎么办」，库里存的是「容器磁盘空间告警排查」。
// 直接拿整句去 LIKE 必然空页——这不是用户搜错了，是检索没适配。
// 这一层负责把大白话翻译成能落地的检索词：
//
//	① 归一化（全角转半角、统一大小写、标点转空格）
//	② 去掉疑问词与语气词（「怎么办」「如何」「我的」）
//	③ 用领域词典对连续汉字做最长匹配切分
//	④ 同义词展开（磁盘 → 硬盘 / disk / df）
//
// 产出交给仓储层，由它决定组间取 AND 还是 OR。

// SearchPlan 是一次检索的完整意图。
type SearchPlan struct {
	// Raw 原始输入，用于回显。
	Raw string
	// Terms 是识别出的有效概念，用于告诉用户「我按这几个词帮你找」。
	Terms []string
	// Expanded 是实际参与检索的全部词（含同义词）。
	Expanded []string
	// Groups 是给仓储层的同义词分组：组内 OR，组间严格模式取 AND。
	Groups repository.KeywordGroups
}

// IsEmpty 表示这次检索没提取出任何可用信息（输入全是语气词，或根本没输入）。
// 调用方据此决定是"返回空结果"还是"提示用户换个说法"。
func (p SearchPlan) IsEmpty() bool { return len(p.Groups) == 0 }

// minTermRunes 是单个检索词的最小长度。
// 单字词（「满」「了」「的」）的区分度太低，放进去只会把结果稀释成噪音。
const minTermRunes = 2

// maxUndictRunes 是词典未收录时保留整串的上限。
// 超过这个长度的未知串既不可能是关键词，整串 LIKE 也不可能命中，直接丢弃。
const maxUndictRunes = 8

// SearchGroups 把用户的原始输入解析成检索词组，供仓储层直接使用。
//
// 解析不出任何有效词时，回落到「原样匹配整串」而不是「不过滤」。
// 后者是个很隐蔽的坑：一句全是语气词、或超过长度上限的输入会让检索条件为空，
// 于是静默返回全站内容——用户看到的是「搜什么都出一大堆」，比搜不到更让人困惑。
// 回落之后这类输入得到的是零结果，语义上才是对的。
func SearchGroups(keyword string) repository.KeywordGroups {
	if strings.TrimSpace(keyword) == "" {
		return nil
	}
	if groups := PlanSearch(keyword).Groups; len(groups) > 0 {
		return groups
	}
	return repository.KeywordGroups{{strings.TrimSpace(keyword)}}
}

// PlanSearch 解析一次用户检索。
func PlanSearch(raw string) SearchPlan {
	plan := SearchPlan{Raw: strings.TrimSpace(raw)}

	text := normalizeSearchText(raw)
	if text == "" {
		return plan
	}

	terms := extractTerms(text)
	plan.Terms = terms
	plan.Expanded = []string{}
	plan.Groups = make(repository.KeywordGroups, 0, len(terms))

	seen := map[string]bool{}
	groupKeys := map[string]bool{}
	for _, t := range terms {
		group := dedupeWords(expandTerm(t))
		if len(group) == 0 {
			continue
		}
		// 两个词展开成同一组时只保留一组：「k8s 集群」都会落到「集群」这一组，
		// 重复 AND 一遍完全等价，却让 SQL 白白变长一倍。
		key := groupKey(group)
		if groupKeys[key] {
			continue
		}
		groupKeys[key] = true
		plan.Groups = append(plan.Groups, group)
		for _, v := range group {
			key := strings.ToLower(v)
			if seen[key] {
				continue
			}
			seen[key] = true
			plan.Expanded = append(plan.Expanded, v)
		}
	}
	return plan
}

// groupKey 把一组同义词压成可比较的键（忽略顺序）。
func groupKey(group []string) string {
	keys := make([]string, 0, len(group))
	for _, v := range group {
		keys = append(keys, strings.ToLower(v))
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x00")
}

/* ------------------------------------------------------------ 归一化 -- */

// normalizeSearchText 统一大小写与全角字符，把标点替换为空格。
//
// 全角必须处理：中文输入法下打出的「Ｋ８Ｓ」「｜」「（）」很常见，
// 不归一化就和库里的半角形式永远匹配不上。
func normalizeSearchText(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'Ａ' && r <= 'Ｚ':
			b.WriteRune(r - 'Ａ' + 'a')
		case r >= 'ａ' && r <= 'ｚ':
			b.WriteRune(r - 'ａ' + 'a')
		case r >= '０' && r <= '９':
			b.WriteRune(r - '０' + '0')
		case r == '　':
			b.WriteByte(' ')
		case isWordRune(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteByte(' ')
		}
	}
	// Fields 顺带把连续空白压成一个。
	return strings.Join(strings.Fields(b.String()), " ")
}

// isWordRune 判断字符是否算作"词内字符"。
//
// 放开 + # . - _ 是为了让 c++ / c# / k8s.io / k8s-operator / node_exporter
// 这类技术名词保持完整——在运维语境里它们是一个词，切开就什么都不剩了。
func isWordRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '+', '#', '.', '-', '_':
		return true
	}
	return false
}

/* ---------------------------------------------------------- 词提取 -- */

// extractTerms 从归一化文本里挑出有效概念。
func extractTerms(text string) []string {
	out := make([]string, 0, 4)
	seen := map[string]bool{}

	push := func(t string) {
		t = strings.TrimSpace(t)
		if len([]rune(t)) < minTermRunes {
			return
		}
		key := strings.ToLower(t)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, t)
	}

	for _, token := range strings.Fields(text) {
		for _, part := range splitByScript(token) {
			if !hasHan(part) {
				if !latinStopwords[part] {
					push(part)
				}
				continue
			}
			for _, seg := range segmentHan(stripFillers(part)) {
				push(seg)
			}
		}
	}
	return out
}

// splitByScript 把 token 按「汉字 / 非汉字」切成若干段。
//
// 中英混排是常态：「cpu占用高」「k8s集群怎么部署」都是一串里两种文字。
// 整段按汉字去查词典，英文前缀会被直接跳过——「cpu占用高」里 cpu 明明是个
// 有效概念却提不出来，用户搜它只能得到零结果；整段按英文处理又会把中文全丢掉。
// 切开各按各的规则走才对。
func splitByScript(s string) []string {
	out := make([]string, 0, 2)
	var cur strings.Builder
	curHan := false

	for _, r := range s {
		isHan := unicode.Is(unicode.Han, r)
		if cur.Len() > 0 && isHan != curHan {
			out = append(out, cur.String())
			cur.Reset()
		}
		curHan = isHan
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// stripFillers 去掉疑问词与语气词。
//
// hanFillers 已按长度倒序排好（init 里做的）：必须先替换「为什么」再替换
// 「什么」，否则「为什么」会被切成「为」+「么」，反而留下残渣。
func stripFillers(s string) string {
	for _, f := range hanFillers {
		s = strings.ReplaceAll(s, f, "")
	}
	return s
}

// segmentHan 用领域词典对连续汉字做最长匹配切分。
//
// 不做切分的后果很直观：「磁盘空间不足」整串 LIKE 一条也搜不到；
// 按字切又会切出「间不」这种没有检索意义的组合。词典里没收录的串
// 按整串保留（够短的话），总不能因为没收词就什么都不搜。
func segmentHan(s string) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}

	out := make([]string, 0, 4)
	matched := false
	for i := 0; i < len(runes); {
		hit := 0
		limit := len(runes) - i
		if limit > hanDictMaxLen {
			limit = hanDictMaxLen
		}
		for n := limit; n >= minTermRunes; n-- {
			if hanDict[string(runes[i:i+n])] {
				hit = n
				break
			}
		}
		if hit == 0 {
			i++
			continue
		}
		out = append(out, string(runes[i:i+hit]))
		matched = true
		i += hit
	}

	if matched {
		return out
	}
	if len(runes) <= maxUndictRunes {
		return []string{s}
	}
	return nil
}

// expandTerm 返回一个概念的全部说法；词典里没有就原样返回。
func expandTerm(term string) []string {
	if group, ok := expandIndex[strings.ToLower(term)]; ok {
		return group
	}
	return []string{term}
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// dedupeWords 去空、去重（忽略大小写），保持首次出现的顺序。
func dedupeWords(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

/* -------------------------------------------------------- 词典 -- */

// hanFillers 是要丢掉的疑问词与语气词。
//
// 这类词在自然语言问句里出现率极高，但完全不携带检索信息：
// 「磁盘满了怎么办」里真正有用的是「磁盘」，「满了」「怎么办」都是干扰。
var hanFillers = []string{
	"什么时候", "怎么样", "为什么", "怎么办", "怎么弄", "如何解决", "是什么",
	"请问", "请教", "帮忙", "帮我", "我们", "我的", "我想", "我要", "能否",
	"是不是", "有没有", "哪些", "哪个", "什么", "怎么", "如何", "怎样", "为何",
	"应该", "需要", "可以", "一直", "经常", "突然", "有时候", "问题", "情况",
	"时候", "现在", "一下", "的话", "处理", "解决",
	"吗", "呢", "吧", "呀", "啊", "了", "的",
}

// latinStopwords 是英文停用词。
// 用户混着打英文问句（"how to fix disk full"）时，「how」「to」不该进检索条件。
var latinStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"of": true, "to": true, "in": true, "on": true, "at": true, "for": true,
	"and": true, "or": true, "not": true, "how": true, "what": true, "why": true,
	"with": true, "my": true, "me": true, "you": true, "it": true, "this": true,
	"that": true, "do": true, "does": true, "can": true, "should": true,
	"help": true, "please": true, "fix": true,
}

// searchSynonyms 是运维领域的概念词典。
//
// key 是概念的规范名，value 是它在用户口中的各种说法。
// 这张表同时承担两个职责：同义词展开，以及汉字切分的词表——
// 所以补词的时候顺手把常见别名一起写进去，检索和切分都会跟着变准。
// 增补只需加一行，不用改任何逻辑。
var searchSynonyms = map[string][]string{
	"磁盘": {"磁盘", "硬盘", "磁盘空间", "空间不足", "盘满", "disk", "df"},
	// 「占用」必须收进这一组：用户说的是「cpu 占用高」，「占用高」不是词，
	// 不收词就只能整串保留，而整串 LIKE 一条也命中不了。
	// 收进来之后它和 cpu 落到同一组，组间去重又会把两个词并成一个条件。
	"CPU":  {"cpu", "处理器", "负载", "load", "使用率", "占用"},
	"内存":   {"内存", "memory", "ram", "oom", "内存溢出", "内存泄漏"},
	"进程":   {"进程", "process", "ps", "僵尸进程"},
	"日志":   {"日志", "log", "logs", "日志分析"},
	"网络":   {"网络", "network", "丢包", "网络延迟", "不通"},
	"端口":   {"端口", "port", "端口占用"},
	"连接":   {"连接", "connection", "连接数", "连接池", "timeout", "超时"},
	"容器":   {"容器", "container", "docker"},
	"镜像":   {"镜像", "image", "dockerfile"},
	"集群":   {"集群", "cluster", "k8s", "kubernetes", "kubectl"},
	"节点":   {"节点", "node", "主机", "服务器", "机器"},
	"证书":   {"证书", "cert", "certificate", "tls", "ssl", "https"},
	"备份":   {"备份", "backup", "恢复", "还原", "restore"},
	"权限":   {"权限", "permission", "chmod", "chown", "acl"},
	"性能":   {"性能", "调优", "优化", "performance"},
	"监控":   {"监控", "monitoring", "prometheus", "grafana", "指标", "告警", "报警"},
	"数据库":  {"数据库", "database", "mysql", "postgres", "sql", "db"},
	"主从":   {"主从", "复制", "replication", "主从同步", "数据同步"},
	"慢查询":  {"慢查询", "慢sql", "慢日志"},
	"部署":   {"部署", "deploy", "发布", "上线", "回滚"},
	"配置":   {"配置", "config", "配置项", "参数"},
	"排查":   {"排查", "排障", "故障", "诊断", "troubleshoot", "异常", "报错", "错误"},
	"重启":   {"重启", "restart", "重新启动", "恢复运行"},
	"启动":   {"启动", "start", "启动失败", "起不来"},
	"安全":   {"安全", "security", "加固", "漏洞", "cve"},
	"扩容":   {"扩容", "扩缩容", "scale", "副本"},
	"限流":   {"限流", "熔断", "降级", "ratelimit"},
	"链路":   {"链路", "trace", "tracing", "调用链"},
	"脚本":   {"脚本", "shell", "bash", "script"},
	"定时":   {"定时", "cron", "计划任务", "定时任务"},
	"缓存":   {"缓存", "cache", "redis", "memcached"},
	"队列":   {"队列", "queue", "kafka", "消息"},
	"网关":   {"网关", "gateway", "nginx", "ingress", "反向代理"},
	"文件":   {"文件", "file", "inode", "句柄"},
	"系统":   {"系统", "system", "内核", "kernel", "linux"},
	"磁盘IO": {"磁盘io", "io", "iowait", "读写"},
}

var (
	// expandIndex 是"某个说法 → 该概念的全部说法"的索引。
	expandIndex = map[string][]string{}
	// hanDict 是切分用的汉字词表，由上面的同义词表推导而来。
	hanDict = map[string]bool{}
	// hanDictMaxLen 是最长词的长度，决定最长匹配的窗口起点。
	hanDictMaxLen = minTermRunes
)

func init() {
	// 先按长度倒序，保证「为什么」先于「什么」被替换掉。
	sort.SliceStable(hanFillers, func(i, j int) bool {
		return len([]rune(hanFillers[i])) > len([]rune(hanFillers[j]))
	})

	for _, variants := range searchSynonyms {
		group := dedupeWords(variants)
		for _, v := range group {
			key := strings.ToLower(v)
			// 同一个说法可能出现在多个概念下（例如「延迟」），把两组并起来，
			// 否则查到一个就停会让检索面凭空变窄。
			expandIndex[key] = dedupeWords(append(expandIndex[key], group...))
			if hasHan(v) {
				hanDict[v] = true
			}
		}
	}

	for w := range hanDict {
		if n := len([]rune(w)); n > hanDictMaxLen {
			hanDictMaxLen = n
		}
	}
}
