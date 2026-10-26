package model

import "time"

// 教程集难度。三档而不是"入门/中级/高级/专家"四档：
// 运维学习路径的实际分水岭只有三个——能装起来、能接进业务、能上规模扛住。
const (
	ToolBookLevelBeginner = "beginner" // 入门
	ToolBookLevelAdvanced = "advanced" // 进阶
	ToolBookLevelExpert   = "expert"   // 深入
)

// ToolBookLevels 返回三档难度的稳定顺序。
func ToolBookLevels() []string {
	return []string{ToolBookLevelBeginner, ToolBookLevelAdvanced, ToolBookLevelExpert}
}

// IsValidToolBookLevel 判断难度是否合法。
func IsValidToolBookLevel(level string) bool {
	for _, l := range ToolBookLevels() {
		if l == level {
			return true
		}
	}
	return false
}

// ToolBook 是一本「系统教程」：把若干篇文章按章节组织成从零基础到深入的路径。
//
// 为什么不把章节正文直接存进来：正文就是 articles 里的文章，编辑器、图片上传、
// 审核流、版本回滚、SEO 全都已经在那套里。教程集只做**目录组织**这一件事，
// 存正文等于再造一个编辑器，而它立刻会落后于主编辑器。
//
// 与 ToolGuide（平铺的"实战教程"推荐）的区别：
//   - ToolGuide 回答「这个工具有哪些值得看的文章」，条目之间没有顺序；
//   - ToolBook 回答「我该怎么系统地学会它」，章节有严格顺序与层级。
//
// 两者在工具详情页分两块展示，不互相替代。
type ToolBook struct {
	Base

	ToolID int64 `gorm:"index;not null;comment:所属工具 ID" json:"toolId"`

	Title   string `gorm:"size:200;index;comment:教程集标题" json:"title"`
	Summary string `gorm:"size:500;comment:一句话简介" json:"summary"`
	Cover   string `gorm:"size:512;comment:封面图地址" json:"cover"`
	// Level 取值见 ToolBookLevel* 常量。
	Level string `gorm:"size:16;index;default:beginner;comment:难度 beginner|advanced|expert" json:"level"`

	// Status 沿用内容五状态：教程集也是要被审核/下线的对象。
	Status string `gorm:"size:16;index;default:draft;comment:状态" json:"status"`
	// Sort 越大越靠前，同一工具下的多本书按此排序。
	Sort int `gorm:"default:0;comment:排序权重" json:"sort"`

	AuthorID    int64      `gorm:"index;comment:创建人用户 ID" json:"authorId"`
	PublishedAt *time.Time `gorm:"comment:发布时间" json:"publishedAt"`
}

// TableName 固定表名。
func (ToolBook) TableName() string { return "tool_books" }

// ToolBookChapter 教程集里的一个节点。
//
// 两级封顶：ParentID 为 0 是「章」，非 0 是「节」，且节的父级必须也是章。
// 不做无限层级，原因是运维教程的实际情况——「第一章 / 1.2 节」已经够用，
// 再往下钻的教程几乎都没人读完；而无限层级会让目录渲染、拖拽排序、
// 面包屑三处同时变复杂，维护成本立刻翻倍。
//
// 刻意不嵌入 Base（不软删除）：章节是整体替换的目录结构，
// 软删除会让被删掉的节点一直占着行，而目录查询每次都要带 deleted_at 条件。
// 与 ToolGuide / ArticleLike 是同一类处理。
type ToolBookChapter struct {
	ID       int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	BookID   int64  `gorm:"index;not null;comment:所属教程集 ID" json:"bookId"`
	ParentID int64  `gorm:"index;default:0;comment:父节点 ID，0 表示章" json:"parentId"`
	Title    string `gorm:"size:200;comment:章节标题" json:"title"`
	// ArticleID 非 0 表示正文是站内文章；与 ExternalURL 二选一。
	ArticleID int64 `gorm:"index;comment:正文文章 ID" json:"articleId"`
	// ExternalURL 指向站外文档。与 ArticleID 二选一。
	ExternalURL string `gorm:"size:512;comment:外部链接" json:"externalUrl"`
	// Summary 是目录里的一句话导语，可为空。
	Summary string `gorm:"size:500;comment:章节导语" json:"summary"`
	Sort    int    `gorm:"default:0;comment:同层排序，越小越靠前" json:"sort"`
}

// TableName 固定表名。
func (ToolBookChapter) TableName() string { return "tool_book_chapters" }
