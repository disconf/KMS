package dto

import "time"

// ============================ 出参 ============================

// ToolChapterItem 教程集目录里的一个节点。
//
// 两级封顶：顶层是「章」，章下是「节」。前端渲染成可折叠的目录树，
// 因此用 Children 嵌套而不是给每条一个 parentId 让前端自己拼——
// 拼树是展示逻辑，不该让每个调用方各写一遍。
type ToolChapterItem struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	// ArticleID/ArticleSlug 指向站内正文文章；URL 指向站外文档。二者最多一个非空。
	ArticleID   int64  `json:"articleId,omitempty"`
	ArticleSlug string `json:"articleSlug,omitempty"`
	URL         string `json:"url,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Sort        int    `json:"sort"`
	// Broken 表示这条的正文已失效（文章被删或已下线）。
	// 前台直接跳过，后台标红——运营得知道哪一条配置挂了。
	Broken   bool              `json:"broken,omitempty"`
	Children []ToolChapterItem `json:"children,omitempty"`
}

// ToolBookItem 教程集列表项。
type ToolBookItem struct {
	ID      int64  `json:"id"`
	ToolID  int64  `json:"toolId"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Cover   string `json:"cover"`
	// Level 取值 beginner / advanced / expert。
	Level  string `json:"level"`
	Status string `json:"status"`
	Sort   int    `json:"sort"`
	// ChapterCount 只算**可见**节点（失效文章不计），用于「共 12 节」这类文案。
	ChapterCount int               `json:"chapterCount"`
	PublishedAt  *time.Time        `json:"publishedAt,omitempty"`
	Chapters     []ToolChapterItem `json:"chapters,omitempty"`
}

// ToolBookDetail 教程集详情页数据：书 + 所属工具的跳转信息。
type ToolBookDetail struct {
	ToolBookItem

	ToolName string `json:"toolName"`
	ToolSlug string `json:"toolSlug"`
}

// ============================ 后台入参 ============================

// ToolBookCreateReq 新建教程集。
//
// 没有 Status 字段：新建一律是草稿，上线走发布接口。
// 让创建接口带状态，等于允许"一次请求直接把未检查的目录推给用户"。
type ToolBookCreateReq struct {
	Title   string `json:"title" binding:"required"`
	Summary string `json:"summary"`
	Cover   string `json:"cover"`
	Level   string `json:"level" binding:"omitempty,oneof=beginner advanced expert"`
	Sort    int    `json:"sort"`
}

// ToolBookUpdateReq 更新教程集，指针字段实现「未传即不改」。
type ToolBookUpdateReq struct {
	Title   *string `json:"title"`
	Summary *string `json:"summary"`
	Cover   *string `json:"cover"`
	Level   *string `json:"level" binding:"omitempty,oneof=beginner advanced expert"`
	Sort    *int    `json:"sort"`
}

// ToolChapterOutlineReq 用大纲文本整体替换目录。
//
// 为什么是文本框而不是拖拽编辑器：
//
//	运维教程的大纲本来就是在文档里写好的（「第一章 / 1.1 / 1.2」），
//	拖拽器要先一条条点出来再拖顺序；文本框可以直接整段粘进来。
//	而目录是一个整体结构，整体替换天然避免了「删了章、它的节还挂着
//	指向空气的 parent_id」这种中间态。后台的维护频率是每周几次，
//	不是每小时——为它写一套拖拽组件不划算。
//
// 格式（见 FormatToolChapterOutline 的说明）：
//
//	# 第一章 安装与配置
//	## 1.1 编译安装 | nginx-compile | 一句话导语
//	## 1.2 目录结构 | nginx-dirs
//	# 第二章 反向代理
//	## 2.1 upstream | https://nginx.org/en/docs/http/ngx_http_upstream_module.html
//
// 分隔符是竖线（全角也接受）。第二节里第二列可以是站内文章 slug，
// 也可以是 http(s) 外链。第三列导语可省略。
type ToolChapterOutlineReq struct {
	Outline string `json:"outline"`
}

// ToolChapterOutlineResult 保存大纲后的结果。
type ToolChapterOutlineResult struct {
	// Chapters 是保存后回读的树，前端直接用它渲染，不必再请求一次。
	Chapters []ToolChapterItem `json:"chapters"`
	// Count 是节点总数（章 + 节）。
	Count int `json:"count"`
	// Warnings 是不会导致保存失败、但运营需要知道的问题：
	// 某行格式不合法被忽略、某个 slug 找不到文章（按外链处理或丢弃）。
	Warnings []string `json:"warnings"`
}

// ToolChapterOutlineDoc 后台编辑页回填用的大纲文本。
type ToolChapterOutlineDoc struct {
	Outline string `json:"outline"`
	// Count 便于前端在空目录时给引导文案。
	Count int `json:"count"`
}
