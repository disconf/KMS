package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// QuizFilter 是题目列表的查询条件。
type QuizFilter struct {
	Status     string
	CategoryID int64
	Difficulty string
	Type       string
	Keyword    string
	// KeywordGroups 是展开后的检索词组，由 service 的查询理解层产出。
	KeywordGroups KeywordGroups
	Tag           string
	// AuthorID 非 0 时只看这个人录/投的题（「我的投稿」用）。
	AuthorID int64
	// Sort 取值 latest（默认）/ hot（按作答数）/ hard（正确率升序）。
	Sort string
}

// QuizRepo 题库数据访问：题目、作答流水、练习成绩。
type QuizRepo struct {
	db *gorm.DB
}

// NewQuizRepo 创建 QuizRepo。
func NewQuizRepo(db *gorm.DB) *QuizRepo { return &QuizRepo{db: db} }

// ============================ 题目 ============================

// GetByID 按主键查询；不存在时返回 errcode.ErrQuizNotFound。
func (r *QuizRepo) GetByID(ctx context.Context, id int64) (*model.QuizQuestion, error) {
	var q model.QuizQuestion
	if err := r.db.WithContext(ctx).First(&q, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrQuizNotFound)
	}
	return &q, nil
}

// GetMany 批量按主键查询，保持传入顺序由调用方自行处理。
func (r *QuizRepo) GetMany(ctx context.Context, ids []int64) ([]model.QuizQuestion, error) {
	ids = dedupeIDs(ids)
	if len(ids) == 0 {
		return []model.QuizQuestion{}, nil
	}
	items := make([]model.QuizQuestion, 0, len(ids))
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("批量查询题目失败: %w", err))
	}
	return items, nil
}

// List 按条件分页查询。
func (r *QuizRepo) List(ctx context.Context, f QuizFilter, offset, limit int) ([]model.QuizQuestion, int64, error) {
	groups := f.KeywordGroups.Resolve(f.Keyword)

	totalQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.QuizQuestion{}), f),
		quizSearchColumns, groups, "题目")
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := totalQ.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计题目数量失败: %w", err))
	}
	if total == 0 {
		return []model.QuizQuestion{}, 0, nil
	}

	findQ, err := filterByKeyword(ctx,
		r.applyFilter(r.db.WithContext(ctx).Model(&model.QuizQuestion{}), f),
		quizSearchColumns, groups, "题目")
	if err != nil {
		return nil, 0, err
	}

	items := make([]model.QuizQuestion, 0, limit)
	err = applyKeywordOrder(findQ, quizSearchColumns, groups, quizOrderClause(f.Sort)).
		Offset(offset).Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询题目列表失败: %w", err))
	}
	return items, total, nil
}

// SampleRandom 随机抽题，用于"来一组练习"。
//
// 抽题必须在数据库侧完成：把整库题目取回内存再随机，
// 题库上千条时每个用户每次练习都要全表传输，成本完全没必要。
func (r *QuizRepo) SampleRandom(ctx context.Context, f QuizFilter, count int) ([]model.QuizQuestion, error) {
	if count <= 0 {
		return []model.QuizQuestion{}, nil
	}
	// 前台抽题永远只抽已发布的，防止把草稿题目泄给用户。
	if f.Status == "" {
		f.Status = model.QuizStatusPublished
	}
	items := make([]model.QuizQuestion, 0, count)
	err := r.applyFilter(r.db.WithContext(ctx).Model(&model.QuizQuestion{}), f).
		Order(randomOrderClause(r.db)).
		Limit(count).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("随机抽题失败: %w", err))
	}
	return items, nil
}

// CountPublished 统计已发布题目数。
func (r *QuizRepo) CountPublished(ctx context.Context) (int64, error) {
	return countPublished(ctx, r.db, &model.QuizQuestion{}, model.QuizStatusPublished, "题目")
}

// CategoryCount 是某个分类下的题目数。
type CategoryCount struct {
	CategoryID int64
	Count      int64
}

// CountPublishedByCategory 按分类统计已发布题目数。
//
// 用一次 GROUP BY 取全部，而不是每个分类查一次：分类页要渲染十来个卡片，
// 逐个查会变成十几次往返，而这个查询本身走 category_id 索引，一次就够。
func (r *QuizRepo) CountPublishedByCategory(ctx context.Context) ([]CategoryCount, error) {
	out := make([]CategoryCount, 0, 12)
	err := r.db.WithContext(ctx).Model(&model.QuizQuestion{}).
		Select("category_id AS category_id, COUNT(*) AS count").
		Where("status = ? AND category_id > 0", model.QuizStatusPublished).
		Group("category_id").
		Scan(&out).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计题目分类分布失败: %w", err))
	}
	return out, nil
}

// CategoryProgress 是用户在某个分类下的刷题进度。
//
// Answered / Correct 都按**题目去重**：同一道题刷三遍，进度不该变成 3。
// 这也是错题本的口径 —— 它回答的是"哪些题还没掌握"，不是"刷了多少次"。
type CategoryProgress struct {
	CategoryID int64
	Answered   int64
	Correct    int64
}

// CategoryProgressByUser 统计用户在各类目下的作答进度。
func (r *QuizRepo) CategoryProgressByUser(ctx context.Context, userID int64) ([]CategoryProgress, error) {
	out := make([]CategoryProgress, 0, 12)
	if userID <= 0 {
		return out, nil
	}
	// 显式 join 题目表是为了排除已软删除的题：题目被删了，
	// 历史作答记录仍在 quiz_records 里，若不过滤会算进分母显示"做了 10 题但只有 6 题"。
	err := r.db.WithContext(ctx).
		Table("quiz_records AS r").
		Select("q.category_id AS category_id, COUNT(DISTINCT r.question_id) AS answered, "+
			"COUNT(DISTINCT CASE WHEN r.correct THEN r.question_id END) AS correct").
		Joins("JOIN quiz_questions AS q ON q.id = r.question_id AND q.deleted_at IS NULL").
		Where("r.user_id = ? AND q.category_id > 0", userID).
		Group("q.category_id").
		Scan(&out).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计分类刷题进度失败: %w", err))
	}
	return out, nil
}

// applyFilter 把过滤条件作用到查询上。
//
// 关键词不在这里处理：List 需要先探一次严格模式的总量再决定是否放宽，
// 所以那部分（含本函数的调用）由 filterByKeyword 接管。
// SampleRandom 走得是同一套过滤，它不带关键词。
func (r *QuizRepo) applyFilter(q *gorm.DB, f QuizFilter) *gorm.DB {
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.CategoryID > 0 {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.Difficulty != "" {
		q = q.Where("difficulty = ?", f.Difficulty)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.AuthorID > 0 {
		q = q.Where("author_id = ?", f.AuthorID)
	}
	if tag := strings.TrimSpace(f.Tag); tag != "" {
		q = q.Where("tags LIKE ? ESCAPE '\\'", likePattern(`"`+tag+`"`))
	}
	return q
}

// quizOrderClause 返回排序语句。
//
// hard 排序用「无人作答的排最后」而不是当成 0% 正确率：
// 新题一道都没人做过，把它排在最难的位置没有意义。
func quizOrderClause(sort string) string {
	switch sort {
	case "hot":
		return "answer_count DESC, id DESC"
	case "hard":
		return "(CASE WHEN answer_count = 0 THEN 1 ELSE correct_count * 1.0 / answer_count END) ASC, answer_count DESC, id DESC"
	default:
		return "published_at DESC, created_at DESC, id DESC"
	}
}

// randomOrderClause 按方言给出随机排序函数。
// MySQL 用 RAND()，SQLite 用 RANDOM()，两者互不兼容，必须显式区分。
func randomOrderClause(db *gorm.DB) string {
	if db.Dialector.Name() == "mysql" {
		return "RAND()"
	}
	return "RANDOM()"
}

// Create 创建题目。
func (r *QuizRepo) Create(ctx context.Context, q *model.QuizQuestion) error {
	if err := r.db.WithContext(ctx).Create(q).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建题目失败: %w", err))
	}
	return nil
}

// CreateMany 批量创建题目。
//
// 一次 Create 会拼成单条 INSERT，天然原子：要么全部写进去，要么一条都没有。
// 这正是导入要的语义——校验在 service 里对内存数组做，通过后一次落库，
// 不必手写事务；反倒"逐条 create 包在事务里"会多几十次往返。
func (r *QuizRepo) CreateMany(ctx context.Context, items []*model.QuizQuestion) error {
	if len(items) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&items).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("批量创建题目失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新题目。
func (r *QuizRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	return updateFieldsByID(ctx, r.db, &model.QuizQuestion{}, id, fields, "题目")
}

// UpdateFieldsIfStatus 按「流转前状态」条件更新，返回是否命中。
//
// 审核用 CAS 而不是先查再改：两个管理员同时点「通过」，
// 第二次的 WHERE 会命中 0 行，由调用方给出"已被其他人处理"，
// 而不是把审核人和时间再覆盖一遍（与文章审核同一个修法）。
func (r *QuizRepo) UpdateFieldsIfStatus(ctx context.Context, id int64, fromStatus string, fields map[string]any) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.QuizQuestion{}).
		Where("id = ? AND status = ?", id, fromStatus).
		Updates(fields)
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("审核题目 %d 失败: %w", id, res.Error))
	}
	return res.RowsAffected > 0, nil
}

// Delete 软删除题目。
func (r *QuizRepo) Delete(ctx context.Context, id int64) error {
	return softDeleteByID(ctx, r.db, &model.QuizQuestion{}, id, "题目")
}

// AddAnswerStat 累加作答统计。
//
// correct 为 true 时两个计数一起加，为 false 时只加作答数。
// 用 SQL 表达式而不是"先读再写"，避免并发答题时统计错乱；
// 用 UpdateColumns 而非 Updates，跳过 updated_at 的连带写入——
// 每次答题都改整行会让这道题的写放大远超实际需要。
func (r *QuizRepo) AddAnswerStat(ctx context.Context, questionID int64, correct bool) error {
	fields := map[string]any{"answer_count": gorm.Expr("answer_count + 1")}
	if correct {
		fields["correct_count"] = gorm.Expr("correct_count + 1")
	}
	err := r.db.WithContext(ctx).Model(&model.QuizQuestion{}).Where("id = ?", questionID).
		UpdateColumns(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新题目作答统计失败: %w", err))
	}
	return nil
}

// ============================ 作答流水 ============================

// CreateRecord 写入一条作答流水。
func (r *QuizRepo) CreateRecord(ctx context.Context, rec *model.QuizRecord) error {
	if err := r.db.WithContext(ctx).Create(rec).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("写入作答记录失败: %w", err))
	}
	return nil
}

// CreateRecords 批量写入作答流水。
func (r *QuizRepo) CreateRecords(ctx context.Context, recs []model.QuizRecord) error {
	if len(recs) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&recs).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("批量写入作答记录失败: %w", err))
	}
	return nil
}

// HasAnswered 判断用户是否作答过该题。
// 用于决定要不要累加题目的正确率统计——重复刷同一题会把正确率刷得没意义。
func (r *QuizRepo) HasAnswered(ctx context.Context, userID, questionID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.QuizRecord{}).
		Where("user_id = ? AND question_id = ?", userID, questionID).
		Count(&count).Error
	if err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("查询作答记录失败: %w", err))
	}
	return count > 0, nil
}

// AnsweredQuestionIDs 返回给定题目中用户已作答过的那些。
func (r *QuizRepo) AnsweredQuestionIDs(ctx context.Context, userID int64, questionIDs []int64) (map[int64]struct{}, error) {
	questionIDs = dedupeIDs(questionIDs)
	if userID <= 0 || len(questionIDs) == 0 {
		return map[int64]struct{}{}, nil
	}
	var ids []int64
	err := r.db.WithContext(ctx).Model(&model.QuizRecord{}).
		Distinct().Where("user_id = ? AND question_id IN ?", userID, questionIDs).
		Pluck("question_id", &ids).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询已作答题号失败: %w", err))
	}
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// WrongRow 是错题本一行：题目 ID、最后一次答错的时间，以及当时的作答。
type WrongRow struct {
	QuestionID  int64
	LastWrongAt time.Time
	Chosen      model.StringList
}

// ListWrong 分页返回错题。
//
// 口径是「最近一次作答仍然答错」而不是「曾经答错过」：
// 后者会让用户答对之后错题本里还挂着这道题，复习时反复看到已经掌握的题，
// 错题本很快就没人用了。
//
// 步骤：先按题取最大记录 ID（即最后一次作答），再筛出其中答错的。
func (r *QuizRepo) ListWrong(ctx context.Context, userID int64, categoryID int64, offset, limit int) ([]WrongRow, int64, error) {
	if userID <= 0 {
		return []WrongRow{}, 0, nil
	}

	latest := r.db.Model(&model.QuizRecord{}).
		Select("user_id, question_id, MAX(id) AS last_id").
		Where("user_id = ?", userID).
		Group("user_id, question_id")

	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("quiz_records AS rec").
			Joins("JOIN (?) AS lt ON lt.last_id = rec.id", latest).
			Where("rec.correct = ?", false)
		if categoryID > 0 {
			// 分类条件落在题目表上，错题本要能按方向过滤。
			q = q.Joins("JOIN quiz_questions AS qn ON qn.id = rec.question_id").
				Where("qn.category_id = ? AND qn.deleted_at IS NULL", categoryID)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计错题数失败: %w", err))
	}
	if total == 0 {
		return []WrongRow{}, 0, nil
	}

	rows := make([]WrongRow, 0, limit)
	err := base().
		Select("rec.question_id AS question_id, rec.created_at AS last_wrong_at, rec.chosen AS chosen").
		Order("rec.id DESC").
		Offset(offset).Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询错题列表失败: %w", err))
	}
	return rows, total, nil
}

// QuizStatsRow 是个人答题总览的聚合结果。
type QuizStatsRow struct {
	TotalAnswered int64
	TotalCorrect  int64
}

// Stats 统计个人答题情况。
//
// TotalAnswered 按去重题数计，不是作答次数——同一道题刷十遍不该让
// "累计作答"变成十位数，那反映的是手速不是掌握度。
func (r *QuizRepo) Stats(ctx context.Context, userID int64) (QuizStatsRow, error) {
	if userID <= 0 {
		return QuizStatsRow{}, nil
	}
	// 先取每题的最后一次作答，再统计，保证同一道题只算一次。
	latest := r.db.Model(&model.QuizRecord{}).
		Select("user_id, question_id, MAX(id) AS last_id").
		Where("user_id = ?", userID).
		Group("user_id, question_id")

	var row struct {
		Total   int64
		Correct int64
	}
	err := r.db.WithContext(ctx).Table("quiz_records AS rec").
		Joins("JOIN (?) AS lt ON lt.last_id = rec.id", latest).
		Select("COUNT(*) AS total, SUM(CASE WHEN rec.correct = 1 THEN 1 ELSE 0 END) AS correct").
		Scan(&row).Error
	if err != nil {
		return QuizStatsRow{}, errcode.ErrDatabase.Wrap(fmt.Errorf("统计答题情况失败: %w", err))
	}
	return QuizStatsRow{TotalAnswered: row.Total, TotalCorrect: row.Correct}, nil
}

// ============================ 练习成绩 ============================

// CreateAttempt 创建一轮练习成绩。
func (r *QuizRepo) CreateAttempt(ctx context.Context, a *model.QuizAttempt) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("写入练习成绩失败: %w", err))
	}
	return nil
}

// CreateAttemptWithRecords 在一个事务里写入成绩单与逐题流水。
//
// 必须同事务：只有成绩没有流水，错题本就少了这一轮的记录；
// 只有流水没有成绩，用户的练习历史里会凭空少一次。
// 这类不一致排查起来极费劲，不值得为省一个事务去冒这个风险。
//
// 传入的 a 会在事务内被写回自增 ID，调用方可直接使用。
func (r *QuizRepo) CreateAttemptWithRecords(ctx context.Context, a *model.QuizAttempt, recs []model.QuizRecord) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(a).Error; err != nil {
			return fmt.Errorf("写入练习成绩失败: %w", err)
		}
		for i := range recs {
			recs[i].AttemptID = a.ID
		}
		if len(recs) > 0 {
			if err := tx.Create(&recs).Error; err != nil {
				return fmt.Errorf("写入作答记录失败: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return errcode.ErrDatabase.Wrap(err)
	}
	return nil
}

// ListAttempts 返回用户的答题历史，按时间倒序。
func (r *QuizRepo) ListAttempts(ctx context.Context, userID int64, limit int) ([]model.QuizAttempt, error) {
	if limit <= 0 {
		limit = 20
	}
	items := make([]model.QuizAttempt, 0, limit)
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询答题历史失败: %w", err))
	}
	return items, nil
}

// CountAttempts 统计练习轮次。
func (r *QuizRepo) CountAttempts(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.QuizAttempt{}).
		Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计练习轮次失败: %w", err))
	}
	return count, nil
}
