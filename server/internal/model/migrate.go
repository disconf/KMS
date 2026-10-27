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
	// 先做数据层面的兼容清理，再改结构。
	//
	// 原因：users.email 从"非空默认空串"改成"可空唯一索引"。
	// 库里已有的账号 email 全是 ''，直接加唯一索引会撞上
	// "多个空串重复"——MySQL 报 1062，SQLite 报 UNIQUE constraint failed，
	// 两种都是启动即失败。先把空串转成 NULL（唯一索引对多个 NULL 是放行的）。
	if err := normalizeLegacyData(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}

// normalizeLegacyData 收敛历史数据里会挡住迁移的脏值。
//
// 每个动作都必须幂等：这段代码在每次启动时都会跑。
func normalizeLegacyData(db *gorm.DB) error {
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}
	// 列可能还不存在（首次升级前），HasColumn 保护一下。
	if !db.Migrator().HasColumn(&User{}, "email") {
		return nil
	}

	// 第一步只能是"放宽列约束"，不能是 UPDATE。
	// email 现在是 NOT NULL DEFAULT ''，直接写 NULL 会被数据库拒绝，
	// 而 AutoMigrate 建唯一索引时又会因为一堆 '' 触发重复键。
	// 所以先单独把列改成可空，再清数据，最后交给 AutoMigrate 建索引。
	if nullable, known := emailColumnNullable(db); known && !nullable {
		if err := db.Migrator().AlterColumn(&User{}, "Email"); err != nil {
			// SQLite 的 AlterColumn 是整表重建，某些版本会拒绝；失败不阻断，
			// 后续 AUTO 迁移仍会尝试一次，最坏情况只是唯一索引建不出来。
			return nil
		}
	}

	db.Exec("UPDATE users SET email = NULL WHERE email = ''")
	// 历史账号补齐 speak_status：老数据这一列可能被加成空串。
	db.Exec("UPDATE users SET speak_status = ? WHERE speak_status IS NULL OR speak_status = ''", SpeakNone)
	return nil
}

// emailColumnNullable 判断 email 列当前是否可空。
// known=false 表示查不到列信息（方言不支持），调用方据此跳过 ALTER。
func emailColumnNullable(db *gorm.DB) (nullable, known bool) {
	cols, err := db.Migrator().ColumnTypes(&User{})
	if err != nil {
		return false, false
	}
	for _, c := range cols {
		if c.Name() != "email" {
			continue
		}
		n, ok := c.Nullable()
		if !ok {
			return false, false
		}
		return n, true
	}
	return false, false
}
