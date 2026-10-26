package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// 本文件实现关键词检索的公共部分：把「用户想找什么」翻译成 SQL 条件，
// 并在「每个词都命中」与「命中任意词」之间逐级放宽。
//
// 为什么需要它：用户搜的是「磁盘满了怎么办」这种完整问句，而库里存的是
// 「容器磁盘空间告警排查」。只拿整句去 LIKE，结果必然是空页面——
// 这不是用户搜错了，是检索没做适配。

// KeywordGroups 是关键词的同义词分组。
//
// 组内是同一概念的不同说法，检索时取 OR；组间是不同概念，
// 严格模式下取 AND（都出现才算命中）。
type KeywordGroups [][]string

// searchColumn 是一张表里参与检索的列及其权重。
//
// Weight 用于排序：标题里出现「重启」的文章，应当排在正文里顺带提一句
// 「重启」的前面。没有权重的话，命中一多的就往后沉。
type searchColumn struct {
	Name   string
	Weight int
}

// Resolve 返回最终使用的分组：显式分组优先，否则把 Keyword 整体当成单组。
//
// 后者是给还没接入查询理解层的调用方兜底——不接也仍然是一套可用的检索，
// 只是不会做同义词展开。
func (g KeywordGroups) Resolve(keyword string) KeywordGroups {
	if len(g) > 0 {
		return g
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		return KeywordGroups{{kw}}
	}
	return nil
}

// maxScoreVariants 限制参与排序计算的同义词个数。
//
// 排序表达式是"每个同义词 × 每列"一个 CASE，全量展开会让 SQL 迅速膨胀。
// 每个概念取前三个说法足够了——它们按「原词优先」排列，
// 后面的近义词本来就只是为了兜底命中，不指望它们决定排序。
const maxScoreVariants = 3

// keywordWhere 生成形如 `(a LIKE ? OR b LIKE ?) AND (a LIKE ? OR b LIKE ?)` 的条件。
//
// columns 一律由调用方以字面量传入——它会被直接拼进 SQL，
// 一旦被请求参数污染就是注入漏洞。
//
// strict 为 true 时组间用 AND：用户输入多个词时，「每个词都出现在同一行」
// 几乎必然比「只命中其中一个」更贴近他要找的东西。
func keywordWhere(columns []searchColumn, groups KeywordGroups, strict bool) (string, []any) {
	parts := make([]string, 0, len(groups))
	args := make([]any, 0, len(groups)*len(columns))

	for _, group := range groups {
		variants := dedupeVariants(group)
		if len(variants) == 0 {
			continue
		}
		ors := make([]string, 0, len(variants)*len(columns))
		for _, v := range variants {
			p := likePattern(v)
			for _, col := range columns {
				ors = append(ors, col.Name+" LIKE ? ESCAPE '\\'")
				args = append(args, p)
			}
		}
		parts = append(parts, "("+strings.Join(ors, " OR ")+")")
	}

	if len(parts) == 0 {
		return "", nil
	}
	joiner := " AND "
	if !strict {
		joiner = " OR "
	}
	return strings.Join(parts, joiner), args
}

// keywordScore 生成相关度表达式：命中列越核心、命中的概念越多，值越大。
// CASE WHEN 在 MySQL 与 SQLite 中语义一致，不需要按方言分叉。
func keywordScore(columns []searchColumn, groups KeywordGroups) (string, []any) {
	terms := make([]string, 0, len(groups))
	args := make([]any, 0, len(groups)*len(columns)*maxScoreVariants)

	for _, group := range groups {
		variants := dedupeVariants(group)
		if len(variants) > maxScoreVariants {
			variants = variants[:maxScoreVariants]
		}
		if len(variants) == 0 {
			continue
		}
		cases := make([]string, 0, len(variants)*len(columns))
		for _, v := range variants {
			p := likePattern(v)
			for _, col := range columns {
				cases = append(cases,
					fmt.Sprintf("WHEN %s LIKE ? ESCAPE '\\' THEN %d", col.Name, col.Weight))
				args = append(args, p)
			}
		}
		terms = append(terms, "(CASE "+strings.Join(cases, " ")+" ELSE 0 END)")
	}

	if len(terms) == 0 {
		return "", nil
	}
	return "(" + strings.Join(terms, " + ") + ")", args
}

// applyKeywordOrder 先按相关度倒序，再接上栏目自己的排序做次级键。
//
// 没有关键词时不插任何表达式，完全交给栏目排序——搜索排序不该影响
// 正常浏览列表的顺序。
func applyKeywordOrder(q *gorm.DB, columns []searchColumn, groups KeywordGroups, secondary string) *gorm.DB {
	score, args := keywordScore(columns, groups)
	if score == "" {
		return q.Order(secondary)
	}
	return q.Order(clause.OrderBy{Expression: clause.Expr{
		SQL:  score + " DESC, " + secondary,
		Vars: args,
	}})
}

// filterByKeyword 给 base 补上关键词条件，并返回补完的查询。
//
// 策略是「先精确、再放宽」：
//  1. 用严格模式探一次总量。命中说明用户的每个词都出现在同一行里，
//     这基本就是他要找的，直接用它。
//  2. 一个都没有时退化为 OR。只走严格模式的话，「磁盘满了怎么办」
//     这类大白话会得到空页面；只走 OR 的话，多词查询又会把
//     只沾一个关键词的无关内容全捞进来。
//
// 探测用独立 Session，避免这一条 Where 泄进后面的真正查询里。
func filterByKeyword(ctx context.Context, base *gorm.DB, columns []searchColumn, groups KeywordGroups, label string) (*gorm.DB, error) {
	if len(groups) == 0 {
		return base, nil
	}
	strictSQL, strictArgs := keywordWhere(columns, groups, true)
	if strictSQL == "" {
		return base, nil
	}

	var probe int64
	err := base.Session(&gorm.Session{}).
		Where(strictSQL, strictArgs...).
		Count(&probe).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("检索%s失败: %w", label, err))
	}
	if probe > 0 {
		return base.Where(strictSQL, strictArgs...), nil
	}

	relaxedSQL, relaxedArgs := keywordWhere(columns, groups, false)
	if relaxedSQL == "" {
		return base, nil
	}
	return base.Where(relaxedSQL, relaxedArgs...), nil
}

// dedupeVariants 去掉空串与重复项，保持首次出现的顺序。
// 同一概念的两个说法若只差大小写，只保留先出现的那个。
func dedupeVariants(in []string) []string {
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

// 各表参与关键词检索的列及其权重。
//
// 文章取标题与正文；工具没有正副标题之分，取名称与简介；
// 资源多给文件名一个高权重——用户常常直接搜「nginx.conf」，
// 那时文件名比标题更说明问题。题库只搜题干与分析，选项文本噪音太大。
var (
	articleSearchColumns = []searchColumn{
		{Name: "title", Weight: 3},
		{Name: "summary", Weight: 2},
		{Name: "content", Weight: 1},
		{Name: "tags", Weight: 1},
	}
	toolSearchColumns = []searchColumn{
		{Name: "name", Weight: 3},
		{Name: "summary", Weight: 2},
		{Name: "content", Weight: 1},
		{Name: "tags", Weight: 1},
	}
	resourceSearchColumns = []searchColumn{
		{Name: "title", Weight: 3},
		{Name: "file_name", Weight: 3},
		{Name: "summary", Weight: 2},
		{Name: "content", Weight: 1},
		{Name: "tags", Weight: 1},
	}
	quizSearchColumns = []searchColumn{
		{Name: "stem", Weight: 3},
		{Name: "analysis", Weight: 1},
		{Name: "tags", Weight: 1},
	}
)
