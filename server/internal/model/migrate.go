package model

import (
	"fmt"

	"gorm.io/gorm"
)

// AllModels 返回需要自动迁移的全部模型。
// 顺序即建表顺序：先无外键依赖的字典表，再业务表。
func AllModels() []any {
	return []any{
		// 字典与账号
		&User{},
		&Skill{},
		&Expert{},
		&ExpertSkill{},
		&ArticleCategory{},
		&Article{},
		&ArticleLike{},

		// 知识库其余栏目。分类表在最前面已建好，这几张都引用它的主键。
		&Tool{},
		// 工具的四类实战教程关联，必须排在 Tool 与 Article 之后。
		&ToolGuide{},
		// 系统教程集（目录组织）。章节表引用工具集主键，排在 ToolBook 之后。
		// 章节的 ArticleID 不做外键约束：文章可以被删除，而目录只是引用，
		// 由 service 在读取时跳过失效条目（与 ToolGuide 一致）。
		&ToolBook{},
		&ToolBookChapter{},
		&QuizQuestion{},
		&QuizAttempt{},
		&QuizRecord{},
		&Resource{},
		// 资源的外链下载地址，排在 Resource 之后。
		&ResourceLink{},

		// 运维资讯的订阅源。抓取结果直接落 articles，不需要额外的条目表。
		&NewsSource{},

		// 交易
		&Question{},
		&Order{},
		&ServiceSession{},
		&Review{},

		// 财务
		&Settlement{},
		&Withdrawal{},
		&Dispute{},

		// 企业（V1.5 启用，MVP 先建表避免后续迁移）
		&Enterprise{},
		&Ticket{},
		&Subscription{},

		// 问答广场。问题 → 回答 → 点赞，按依赖顺序建。
		&QaQuestion{},
		&QaAnswer{},
		&QaAnswerLike{},

		// 审计、系统配置与积分流水
		&AuditLog{},
		&SystemSetting{},
		&PointLog{},
	}
}

// AutoMigrate 执行结构同步。
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}
