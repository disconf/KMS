package model

import "time"

// 问答状态。
//
// 只有三态且基本单向：open → resolved（提问人采纳）/ open → closed（提问人放弃）。
// 刻意不做「回答中 / 待确认」这类中间态：社区问答的价值在于沉淀，
// 状态机越长，帖子越容易卡在没人处理的一格里。
const (
	QaStatusOpen     = "open"     // 待解答
	QaStatusResolved = "resolved" // 已采纳
	QaStatusClosed   = "closed"   // 提问人关闭（未解决也不再接收回答）
)

// 回答状态。
//
// 回答也要审核，但不是全部：新账号的前几条先过人工，专家与老用户直发。
// 一律先审会把「有人愿意答」这个最稀缺的动作拖成隔天见，问答区当场就死了。
const (
	QaAnswerPending   = "pending"   // 待审核（仅作者与管理员可见）
	QaAnswerPublished = "published" // 已发布
	QaAnswerRejected  = "rejected"  // 驳回
)

// QaQuestion 问答广场的问题。
//
// 与 Question（悬赏提问）是两套东西，别合并：
// Question 是付费订单的前置单据，有钱流、有接单、有状态机；
// QaQuestion 是免费社区帖，没有金额字段也不该有——一旦可悬赏，
// 就等于在订单链路之外又长出一条要结算、要退款、要处理纠纷的支线。
type QaQuestion struct {
	Base

	UserID      int64      `gorm:"index;comment:提问人用户 ID" json:"userId"`
	Title       string     `gorm:"size:200;index;comment:标题（故障现象式）" json:"title"`
	Description string     `gorm:"type:text;comment:正文，Markdown" json:"description"`
	Attachments StringList `gorm:"comment:日志/截图附件" json:"attachments"`
	SkillIDs    StringList `gorm:"comment:技术标签 ID 列表" json:"skillIds"`

	ViewCount   int64 `gorm:"default:0;comment:浏览数" json:"viewCount"`
	AnswerCount int64 `gorm:"default:0;comment:已发布回答数（冗余，用于列表展示与排序）" json:"answerCount"`

	Status string `gorm:"size:16;index;default:open;comment:状态 open|resolved|closed" json:"status"`

	// AcceptedAnswerID 指向被采纳的回答（0 = 未采纳）。
	//
	// 用字段而不是「回答表里的 is_accepted」单点真相：采纳是问题侧的决定，
	// 放问题表才能用一条 CAS 把它和状态流转一起原子完成。
	// 回答表的 IsAccepted 是给「按回答查」用的冗余。
	AcceptedAnswerID int64      `gorm:"default:0;comment:被采纳的回答 ID" json:"acceptedAnswerId"`
	AcceptedAt       *time.Time `gorm:"comment:采纳时间" json:"acceptedAt"`

	// LastAnswerAt 是列表排序键：有新回答的帖子顶上去，而不是按发布时间。
	// 没有它，新帖子发出来五分钟就沉底，提问人拿不到回答就再也不来了。
	LastAnswerAt *time.Time `gorm:"index;comment:最近一条回答时间" json:"lastAnswerAt"`
}

// TableName 固定表名。
func (QaQuestion) TableName() string { return "qa_questions" }

// IsOpen 判断是否还能回答。
func (q *QaQuestion) IsOpen() bool { return q.Status == QaStatusOpen }

// QaAnswer 一条回答。
//
// (QuestionID, UserID) 联合唯一：一人一答。不这么做，一个帖子会被同一个人用
// 「补充一下 / 再补充」刷成几十层，真正有用的答案被顶到看不见的地方。
// 想补充就编辑自己那条。
type QaAnswer struct {
	Base

	QuestionID int64 `gorm:"uniqueIndex:uk_qa_answer,priority:1;index;comment:问题 ID" json:"questionId"`
	UserID     int64 `gorm:"uniqueIndex:uk_qa_answer,priority:2;index;comment:回答人用户 ID" json:"userId"`

	Content string `gorm:"type:text;comment:回答正文，Markdown" json:"content"`
	// Score 是点赞数。存冗余计数而不是每次 COUNT：
	// 列表页要按赞数排序，逐条 COUNT 会让一页 20 条变成 20 次子查询。
	Score int64 `gorm:"default:0;comment:点赞数" json:"score"`

	IsAccepted bool   `gorm:"default:false;index;comment:是否被采纳" json:"isAccepted"`
	Status     string `gorm:"size:16;index;default:published;comment:状态 pending|published|rejected" json:"status"`

	// 审核痕迹，复用内容审核那套匿名嵌入（驳回要能看到原因）。
	ReviewInfo
}

// TableName 固定表名。
func (QaAnswer) TableName() string { return "qa_answers" }

// QaAnswerLike 回答点赞关系。
//
// 纯关系表：不嵌 Base、不做软删除。踩过的坑是 ArticleLike ——
// 软删除的行仍然占着唯一索引，upsert 静默失败，接口说"已点赞"而库里没有记录。
// 取消点赞一律物理删除。
type QaAnswerLike struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`

	AnswerID int64 `gorm:"uniqueIndex:uk_qa_answer_like;index;comment:回答 ID" json:"answerId"`
	UserID   int64 `gorm:"uniqueIndex:uk_qa_answer_like;index;comment:点赞用户 ID" json:"userId"`
}

// TableName 固定表名。
func (QaAnswerLike) TableName() string { return "qa_answer_likes" }

// 贡献积分动作。
//
// 只做展示（徽章、贡献等级），**不兑换任何东西**。一旦能换钱或换权益，
// 就得防刷、要对账、要能追回，等于又开一条准资金链路。
const (
	PointActionAcceptAnswer = "qa.answer_accepted" // 回答被采纳
	PointActionAnswerLiked  = "qa.answer_liked"    // 回答被点赞
	PointActionArticlePass  = "article.published"  // 教程过审发布
	PointActionQuizPass     = "quiz.published"     // 题目过审入库
)

// PointLog 积分流水。
//
// UniqKey 是幂等键，承担「同一件事只记一次」的硬约束：
// 采纳重复提交、同一个人反复点赞、审核回调重放，都靠它挡住。
// 应用层先查再写，在并发下不可靠——真正兜底的是唯一索引。
type PointLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`

	UserID int64  `gorm:"index;comment:获得积分的用户" json:"userId"`
	Action string `gorm:"size:32;index;comment:动作" json:"action"`
	Points int64  `gorm:"comment:积分（正数累加）" json:"points"`

	TargetType string `gorm:"size:16;comment:对象类型 answer|article|quiz" json:"targetType"`
	TargetID   int64  `gorm:"comment:对象 ID" json:"targetId"`

	UniqKey string `gorm:"size:160;uniqueIndex;comment:幂等键" json:"-"`
}

// TableName 固定表名。
func (PointLog) TableName() string { return "point_logs" }
