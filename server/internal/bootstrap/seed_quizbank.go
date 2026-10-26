package bootstrap

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/logger"
)

// 系统题库：由 scripts/quizgen 从「事实矩阵 + 模板」批量生成，
// 覆盖 10 个运维方向、每个方向不少于 1000 道。
//
// 为什么要走 embed 而不是在线接口：题库是静态语料，跟随二进制发布才能保证
// 「同一个版本跑出的题完全一致」，也免掉部署还要额外传一堆文件。
//
//go:embed data/quiz_bank.json
var quizBankFS embed.FS

// quizBankMarker 是系统题目的标记 tag，同时充当幂等键：
// 只有带这个 tag 的题目才由本文件管理，运营手工录入/用户投稿的题目不受影响。
const quizBankMarker = "系统题库"

// quizBankFile 对应 generate.py 的输出结构。
type quizBankFile struct {
	Version    int                           `json:"version"`
	Generated  string                        `json:"generated"`
	Categories map[string][]quizBankQuestion `json:"categories"`
}

type quizBankQuestion struct {
	Stem       string             `json:"stem"`
	Type       string             `json:"type"`
	Options    []model.QuizOption `json:"options"`
	Correct    []string           `json:"correct"`
	Analysis   string             `json:"analysis"`
	Difficulty string             `json:"difficulty"`
	Tags       []string           `json:"tags"`
}

// seedQuizBank 把内嵌的系统题库灌进 quiz_questions，幂等。
//
// 幂等键是「题干 + 该分类下已存在的系统题目集合」：
//   - 先按分类一次性取出所有带 marker 的题干（最多几千行，一次查询）；
//   - 只插入库里没有的题干。
//
// 不用「库里已有系统题就整目录跳过」：那样每次补充题库都得手工清库，
// 而题库是会长的东西（本轮就从几百道涨到一万七千道）。
func seedQuizBank(ctx context.Context, db *gorm.DB) error {
	raw, err := quizBankFS.ReadFile("data/quiz_bank.json")
	if err != nil {
		return fmt.Errorf("读取内嵌题库失败: %w", err)
	}
	var bank quizBankFile
	if err := json.Unmarshal(raw, &bank); err != nil {
		return fmt.Errorf("解析内嵌题库失败: %w", err)
	}
	if len(bank.Categories) == 0 {
		logger.Warn("内嵌题库为空，跳过系统题目写入")
		return nil
	}

	adminID := findAdminID(ctx, db)
	// 分类顺序固定，避免 map 遍历导致每台机器的插入顺序不同（排查时对不上号）。
	slugs := make([]string, 0, len(bank.Categories))
	for slug := range bank.Categories {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	now := time.Now()
	totalInserted := 0
	for _, slug := range slugs {
		categoryID := categoryIDBySlug(ctx, db, slug)
		if categoryID == 0 {
			// 分类字典里没有这个方向：跳过而不是报错。
			// 题库方向是内容规划，分类字典是运营可调的字典，
			// 两者短暂不一致时不该让服务起不来。
			logger.Warn("系统题库的分类不存在，已跳过该方向", "category", slug)
			continue
		}

		existing, err := existingBankStems(ctx, db, categoryID)
		if err != nil {
			return err
		}

		batch := make([]model.QuizQuestion, 0, 200)
		inserted := 0
		for _, q := range bank.Categories[slug] {
			if q.Stem == "" || len(q.Options) == 0 || len(q.Correct) == 0 {
				// 生成的题目理论上不会缺字段，缺了说明 json 被手改坏了，
				// 跳过单题继续跑，别让一行脏数据废掉整个题库。
				logger.Warn("系统题库存在不完整题目，已跳过", "category", slug, "stem", q.Stem)
				continue
			}
			if existing[q.Stem] {
				continue
			}
			existing[q.Stem] = true // 同一批次内也去重，防生成器产出重复题干
			batch = append(batch, model.QuizQuestion{
				Stem:         q.Stem,
				Type:         q.Type,
				Options:      model.QuizOptionList(q.Options),
				Correct:      model.StringList(q.Correct),
				Analysis:     q.Analysis,
				Difficulty:   q.Difficulty,
				CategoryID:   categoryID,
				Tags:         model.StringList(q.Tags),
				Status:       model.QuizStatusPublished,
				AuthorID:     adminID,
				PublishedAt:  &now,
				AnswerCount:  0,
				CorrectCount: 0,
			})
			inserted++
		}

		if len(batch) == 0 {
			continue
		}
		// 分批写入：一万多道题一次性拼 SQL 会超出参数上限（SQLite 的
		// SQLITE_MAX_VARIABLE_NUMBER、MySQL 的 max_allowed_packet 都会踩到）。
		if err := db.WithContext(ctx).CreateInBatches(&batch, quizBankBatchSize(db)).Error; err != nil {
			return fmt.Errorf("写入系统题库（%s，%d 题）失败: %w", slug, inserted, err)
		}
		totalInserted += inserted
	}

	if totalInserted > 0 {
		logger.Info("系统题库已写入", "inserted", totalInserted)
	} else {
		logger.Info("系统题库已是最新，无需写入")
	}
	return nil
}

// existingBankStems 取出某分类下所有系统题目的题干。
//
// tags 是 JSON 数组列，跨 MySQL/SQLite 都用 LIKE 匹配足够：
// 一万多行的量级下没必要为它建额外索引，而这种匹配只发生在启动时。
func existingBankStems(ctx context.Context, db *gorm.DB, categoryID int64) (map[string]bool, error) {
	var stems []string
	err := db.WithContext(ctx).Model(&model.QuizQuestion{}).
		Where("category_id = ? AND tags LIKE ?", categoryID, "%"+quizBankMarker+"%").
		Order("id ASC").
		Pluck("stem", &stems).Error
	if err != nil {
		return nil, fmt.Errorf("查询已有系统题目失败: %w", err)
	}
	set := make(map[string]bool, len(stems))
	for _, s := range stems {
		set[s] = true
	}
	return set, nil
}

// quizBankBatchSize 按方言给批量大小：SQLite 单条语句的参数上限低得多，
// 且写入是逐条的，批小一点反而更稳；MySQL 可以一次写更多。
func quizBankBatchSize(db *gorm.DB) int {
	if db.Dialector.Name() == "sqlite" {
		return 100
	}
	return 200
}
