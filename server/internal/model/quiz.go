package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// 题目状态。
const (
	QuizStatusDraft     = "draft"
	QuizStatusPending   = "pending"
	QuizStatusPublished = "published"
	QuizStatusRejected  = "rejected"
	QuizStatusOffline   = "offline"
)

// 题型。
const (
	QuizTypeSingle   = "single"   // 单选
	QuizTypeMultiple = "multiple" // 多选
	QuizTypeJudge    = "judge"    // 判断
)

// 难度。
const (
	QuizDifficultyEasy   = "easy"
	QuizDifficultyMedium = "medium"
	QuizDifficultyHard   = "hard"
)

// QuizOption 是题目选项。
//
// Key 用 A/B/C/D 而不是下标：答案、用户作答、错题本三处都要引用选项，
// 用字母做键既稳定又便于前端直接渲染，不会因为选项重排而串位。
type QuizOption struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// QuizOptionList 是选项数组的 JSON 存储类型。
type QuizOptionList []QuizOption

// Value 实现 driver.Valuer。
func (o QuizOptionList) Value() (driver.Value, error) {
	if o == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]QuizOption(o))
	if err != nil {
		return nil, fmt.Errorf("序列化 QuizOptionList 失败: %w", err)
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (o *QuizOptionList) Scan(src any) error {
	if src == nil {
		*o = QuizOptionList{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("QuizOptionList 无法解析的类型: %T", src)
	}
	if len(raw) == 0 {
		*o = QuizOptionList{}
		return nil
	}
	var out []QuizOption
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("解析 QuizOptionList 失败: %w", err)
	}
	*o = out
	return nil
}

// GormDataType 声明列类型为 json。
func (QuizOptionList) GormDataType() string { return "json" }

// GormDBDataType 按方言给出精确列类型。
func (QuizOptionList) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	return jsonColumnType(db)
}

// QuizQuestion 题库题目。
type QuizQuestion struct {
	Base

	// 题干是 TEXT，索引必须带前缀长度：MySQL 不允许对 TEXT 建无长度索引
	// （Error 1170），SQLite 则忽略 length，两边都能过。
	Stem string `gorm:"type:text;index:idx_quiz_stem,length:191;comment:题干" json:"stem"`
	// Type 决定判分规则：单选/判断必须完全等于答案，多选要求集合相等。
	Type    string         `gorm:"size:16;index;default:single;comment:题型" json:"type"`
	Options QuizOptionList `gorm:"comment:选项列表" json:"options"`
	// Correct 是正确选项的 key 集合。多选题会有多个，所以是数组而非单值。
	Correct  StringList `gorm:"comment:正确选项 key" json:"-"`
	Analysis string     `gorm:"type:text;comment:答案解析" json:"analysis,omitempty"`

	Difficulty string `gorm:"size:16;index;default:medium;comment:难度" json:"difficulty"`

	CategoryID int64      `gorm:"index;comment:所属分类 ID" json:"categoryId"`
	Tags       StringList `gorm:"comment:标签" json:"tags"`

	// 审核痕迹。题库暂不开放投稿，但保留字段，
	// 让后台的「待审内容」能用同一条查询覆盖四类内容。
	ReviewInfo

	Status      string     `gorm:"size:16;index;default:draft;comment:状态" json:"status"`
	AuthorID    int64      `gorm:"index;comment:录入人用户 ID" json:"authorId"`
	PublishedAt *time.Time `gorm:"comment:发布时间" json:"publishedAt"`

	// AnswerCount / CorrectCount 由答题流水累加，用于展示"本题 120 人作答、正确率 68%"。
	AnswerCount  int64 `gorm:"default:0;comment:累计作答次数" json:"answerCount"`
	CorrectCount int64 `gorm:"default:0;comment:累计答对次数" json:"correctCount"`
}

// TableName 固定表名。
func (QuizQuestion) TableName() string { return "quiz_questions" }

// CorrectRate 返回正确率（%）。无人作答时返回 0 而不是 100，
// 否则新题会被误判成"人人都对"。
func (q *QuizQuestion) CorrectRate() float64 {
	if q.AnswerCount <= 0 {
		return 0
	}
	return float64(q.CorrectCount) / float64(q.AnswerCount) * 100
}

// QuizRecord 单题作答流水，错题本的数据来源。
//
// 与 QuizAttempt 是一对多：Attempt 记一轮练习的整体成绩，
// Record 记这一轮里每道题答了什么。错题本必须精确到题，
// 只留整卷成绩无法回答"我到底错在哪几题"。
//
// 刻意不嵌入 Base：这是纯流水表，不做软删除，
// 否则重复作答会在错题本里堆出一串带 DeletedAt 的幽灵记录。
type QuizRecord struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`

	UserID     int64 `gorm:"index;comment:作答用户 ID" json:"userId"`
	QuestionID int64 `gorm:"index;comment:题目 ID" json:"questionId"`
	// AttemptID 为 0 表示单题练习（没走整卷）。
	AttemptID int64 `gorm:"index;comment:所属练习轮次" json:"attemptId"`

	// Chosen 是用户选的 key 集合，保留原始选择不做归一化，
	// 错题本要能回放"当时到底选了什么"。
	Chosen  StringList `gorm:"comment:用户选择的选项 key" json:"chosen"`
	Correct bool       `gorm:"index;comment:是否答对" json:"correct"`
}

// TableName 固定表名。
func (QuizRecord) TableName() string { return "quiz_records" }

// QuizAttempt 一轮答题的成绩单。
type QuizAttempt struct {
	Base

	UserID        int64 `gorm:"index;comment:作答用户 ID" json:"userId"`
	QuestionCount int64 `gorm:"default:0;comment:题目数" json:"questionCount"`
	CorrectCount  int64 `gorm:"default:0;comment:答对数" json:"correctCount"`
	// Score 百分制得分，保留整数避免浮点比较带来的展示歧义。
	Score       int   `gorm:"default:0;comment:百分制得分" json:"score"`
	DurationSec int64 `gorm:"default:0;comment:耗时秒数" json:"durationSec"`
	// CategoryID 非 0 表示这轮是按分类刷题，用于"分项正确率"。
	CategoryID int64 `gorm:"index;comment:练习分类 ID" json:"categoryId"`
}

// TableName 固定表名。
func (QuizAttempt) TableName() string { return "quiz_attempts" }
