package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/logger"
)

// 本文件是「系统教程集」的种子：把站内文章串成一条有顺序的学习路径。
//
// 与 toolGuideSeeds 的分工：
//   - guide 是「这个工具有哪些值得看的」，条目之间无序，四类分栏；
//   - book 是「我该怎么系统学会它」，章节有序、章下分节。
//
// 内容形态：教程集的每一节都指向**本站文章**（<tool>-install / -production / -commands
// 三篇，见 seed_tool_articles.go），不再挂官网外链——读者点进去就在本站读完，
// 官方文档作为延伸阅读由文章末尾的参考链接承担。
//
// 章节结构固定为六章（toolBookChapters 统一生成）：
//   软件介绍 / 多种部署方式 / 配置参数详解 / 生产实战部署 / 常用命令速查 / 官方文档导读

type toolBookSectionSeed struct {
	Title       string
	ArticleSlug string
	Summary     string
}

type toolBookChapterSeed struct {
	Title    string
	Sections []toolBookSectionSeed
}

type toolBookSeed struct {
	ToolSlug string
	Title    string
	Summary  string
	Level    string
	Sort     int
	Chapters []toolBookChapterSeed
}

// toolBookChapters 按统一的六章结构生成章节种子，
// 避免 15 本书写 90 段几乎一样的结构代码。
// 六节的正文分别是 <toolSlug> 的 intro / install / config / production /
// commands / docs 六篇站内文章，节导语直接取文章元数据里的摘要，
// 保证「目录导语 = 文章摘要」不出现两份文案漂移。
// 文章标题用 toolName 拼出来，与 data/articles 里的正文一一对应。
func toolBookChapters(toolSlug, toolName string) []toolBookChapterSeed {
	summary := func(suffix string) string {
		for _, m := range append(append([]toolArticleMeta{}, toolArticleMetas...), extraToolArticleMetas...) {
			if m.articleSeed.Slug == toolSlug+"-"+suffix {
				return m.articleSeed.Summary
			}
		}
		// 元数据缺失说明注册表漏登了这篇文章，宁可空导语也不 panic：
		// 章节 build 有文章标题兜底，导语只是锦上添花。
		return ""
	}
	chapter := func(no int, label, title, suffix string) toolBookChapterSeed {
		return toolBookChapterSeed{
			Title: fmt.Sprintf("第%s章 %s", chineseNum(no), label),
			Sections: []toolBookSectionSeed{{
				Title: title, ArticleSlug: toolSlug + "-" + suffix, Summary: summary(suffix),
			}},
		}
	}
	return []toolBookChapterSeed{
		chapter(1, "软件介绍", toolName+" 软件介绍与核心概念", "intro"),
		chapter(2, "多种部署方式", toolName+" 安装与多种部署方式", "install"),
		chapter(3, "配置参数详解", toolName+" 配置参数详解", "config"),
		chapter(4, "生产实战部署", toolName+" 生产环境实战部署", "production"),
		chapter(5, "常用命令速查", toolName+" 常用命令速查", "commands"),
		chapter(6, "官方文档导读", toolName+" 官方文档导读", "docs"),
	}
}

// chineseNum 把 1-10 转成汉字序号，仅用于章节标题。
func chineseNum(n int) string {
	nums := []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九", "十"}
	if n >= 0 && n <= 10 {
		return nums[n]
	}
	return fmt.Sprint(n)
}

var toolBookSeeds = []toolBookSeed{
	{
		ToolSlug: "openresty",
		Title:    "Nginx / OpenResty 从装到集群",
		Summary: "从认识它讲到部署、参数、生产与命令，最后教你读官方文档：" +
			"六章走完，一台机器上的 Nginx 就变成扛得住流量的入口层。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     10,
		Chapters: toolBookChapters("openresty", "OpenResty / Nginx"),
	},
}

// seedToolBooks 写入教程集与章节，幂等。
//
// 幂等键是「该工具下是否已有同名教程集」而不是「是否已有任何教程集」：
// 运营可以给同一个工具再建一本进阶篇，用总数判断会让新增的那本永远种不进来。
//
// 站内化迁移：旧版种子每一节挂官网外链（external_url 非空）。种子改成站内文章后，
// 老库里那批章节要整体重建——章节表不嵌 Base，物理删除重插即可。
// 判据是「存在外链节」，所以运营在后台手工建的纯文章目录不会被误伤。
//
// 种子只写开发/测试库，且失败一律只 warn 不中断：
// 教程集是展示层内容，种不进去不该让整个服务起不来。
func seedToolBooks(ctx context.Context, db *gorm.DB) error {
	adminID := findAdminID(ctx, db)

	// 三批教程集合并处理：openresty 那本在 toolBookSeeds，
	// 其余工具的在 seed_toolbook_more.go 的 extraToolBookSeeds，
	// 自研 CMDB 平台的在 seed_tools_cmdb.go 的 cmdbToolBookSeeds。
	books := make([]toolBookSeed, 0, len(toolBookSeeds)+len(extraToolBookSeeds)+len(cmdbToolBookSeeds))
	books = append(books, toolBookSeeds...)
	books = append(books, extraToolBookSeeds...)
	books = append(books, cmdbToolBookSeeds...)

	for _, s := range books {
		var tool model.Tool
		if err := db.WithContext(ctx).Where("slug = ?", s.ToolSlug).First(&tool).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				logger.Warn("教程集种子的工具不存在，跳过", "tool", s.ToolSlug)
				continue
			}
			return fmt.Errorf("查询工具 %s 失败: %w", s.ToolSlug, err)
		}

		var existing model.ToolBook
		err := db.WithContext(ctx).
			Where("tool_id = ? AND title = ?", tool.ID, s.Title).First(&existing).Error
		switch {
		case err == nil:
			// 已存在：目录需要重建的情况有两种——
			//   1) 外链迁移：老版本每节挂官网 external_url；
			//   2) 结构升级：种子新加了章/节（比如从三章扩成六章），
			//      老目录里缺新节的标题。
			// 两种都用「期望的节标题是否齐全」判断，判据一致，实现也一致。
			var sectionTitles []string
			if err := db.WithContext(ctx).Model(&model.ToolBookChapter{}).
				Where("book_id = ? AND parent_id > 0", existing.ID).
				Pluck("title", &sectionTitles).Error; err != nil {
				return fmt.Errorf("读取教程集 %s 的节标题失败: %w", s.Title, err)
			}
			have := make(map[string]bool, len(sectionTitles))
			external := false
			for _, t := range sectionTitles {
				have[t] = true
			}
			var externalCount int64
			if err := db.WithContext(ctx).Model(&model.ToolBookChapter{}).
				Where("book_id = ? AND external_url <> ''", existing.ID).
				Count(&externalCount).Error; err != nil {
				return fmt.Errorf("统计教程集 %s 的外链节失败: %w", s.Title, err)
			}
			external = externalCount > 0

			missing := false
			for _, ch := range s.Chapters {
				for _, sec := range ch.Sections {
					if !have[sec.Title] {
						missing = true
						break
					}
				}
			}
			if !missing && !external {
				continue
			}
			if err := db.WithContext(ctx).
				Where("book_id = ?", existing.ID).
				Delete(&model.ToolBookChapter{}).Error; err != nil {
				return fmt.Errorf("清理教程集 %s 的旧目录失败: %w", s.Title, err)
			}
			logger.Info("教程集目录重建为最新结构", "book", s.Title)
		case errors.Is(err, gorm.ErrRecordNotFound):
			now := time.Now()
			existing = model.ToolBook{
				ToolID:  tool.ID,
				Title:   s.Title,
				Summary: s.Summary,
				Level:   s.Level,
				// 状态取值与 service 保持一致：教程集复用内容五状态，
				// 但常量仍借用 ToolStatus* 这一组（service 里也是这么用的）。
				Status:      model.ToolStatusPublished,
				Sort:        s.Sort,
				AuthorID:    adminID,
				PublishedAt: &now,
			}
			if err := db.WithContext(ctx).Create(&existing).Error; err != nil {
				return fmt.Errorf("写入教程集 %s 失败: %w", s.Title, err)
			}
		default:
			return fmt.Errorf("查询教程集 %s 失败: %w", s.Title, err)
		}

		// 先章后节：节要挂父节点自增 ID，只能在章落库之后插。
		for ci, ch := range s.Chapters {
			chapter := model.ToolBookChapter{
				BookID: existing.ID,
				Title:  ch.Title,
				Sort:   (ci + 1) * 10,
			}
			if err := db.WithContext(ctx).Create(&chapter).Error; err != nil {
				return fmt.Errorf("写入教程集 %s 的章 %q 失败: %w", s.Title, ch.Title, err)
			}
			for si, sec := range ch.Sections {
				row := model.ToolBookChapter{
					BookID:   existing.ID,
					ParentID: chapter.ID,
					Title:    sec.Title,
					Summary:  sec.Summary,
					Sort:     (si + 1) * 10,
				}
				if sec.ArticleSlug != "" {
					var article model.Article
					err := db.WithContext(ctx).Where("slug = ?", sec.ArticleSlug).First(&article).Error
					if err != nil {
						// 文章种子没写成功时不要让整条链路失败，
						// 保留标题去掉链接——与 SetOutline 处理失效 slug 的口径一致。
						logger.Warn("教程集章节关联的文章不存在，该节将只有标题",
							"slug", sec.ArticleSlug, "book", s.Title)
					} else {
						row.ArticleID = article.ID
					}
				}
				if err := db.WithContext(ctx).Create(&row).Error; err != nil {
					return fmt.Errorf("写入教程集 %s 的节 %q 失败: %w", s.Title, sec.Title, err)
				}
			}
		}
	}
	return nil
}
