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

// QaRepo 问答广场的数据访问。
type QaRepo struct{ db *gorm.DB }

// NewQaRepo 创建仓储。
func NewQaRepo(db *gorm.DB) *QaRepo { return &QaRepo{db: db} }

// QaFilter 列表筛选条件。
type QaFilter struct {
	// Status 为空表示不限；resolved / open / closed 精确匹配。
	Status string
	// SkillID 按技术标签过滤。skillIds 是 JSON 列，用 LIKE '"12"' 匹配，
	// 加引号是必须的——否则 1 会命中 11、112。
	SkillID int64
	// Unanswered 只看零回答的帖子（「我来答」入口）。
	Unanswered    bool
	Keyword       string
	KeywordGroups KeywordGroups
}

// qaSearchColumns 参与检索的列。标题权重高于正文。
//
// 列名必须带表前缀：列表查询会 LEFT JOIN users 取昵称，
// 而 users 也有 status 之类同名列——裸写会被 SQLite/MySQL 判为
// ambiguous column，表现为"列表一有数据就 500"（空列表时不会触发）。
var qaSearchColumns = []searchColumn{
	{Name: "qa_questions.title", Weight: 6},
	{Name: "qa_questions.description", Weight: 2},
}

// applyFilter 拼筛选条件。列名一律带 qa_questions 前缀，理由同 qaSearchColumns。
func (r *QaRepo) applyFilter(q *gorm.DB, f QaFilter) *gorm.DB {
	if f.Status != "" {
		q = q.Where("qa_questions.status = ?", f.Status)
	}
	if f.SkillID > 0 {
		// skill_ids 是 JSON 数组文本，匹配 "12" 而不是 12：
		// 不加引号时 1 会命中 11、112 与 21。
		q = q.Where("qa_questions.skill_ids LIKE ?", fmt.Sprintf("%%\"%d\"%%", f.SkillID))
	}
	if f.Unanswered {
		q = q.Where("qa_questions.answer_count = 0")
	}
	return q
}

// QaQuestionRow 问题列表行：问题本体 + 提问人展示信息。
type QaQuestionRow struct {
	model.QaQuestion
	Nickname   string `json:"nickname"`
	Avatar     string `json:"avatar"`
	AuthorRole string `json:"authorRole"`
}

// qaSelect 是问题行的公共 SELECT：JOIN users 取昵称头像与角色，
// 一次查询带出列表/详情要展示的东西，别让前端为每张卡片再发一次请求。
const qaSelect = "qa_questions.*, COALESCE(u.nickname,'') AS nickname, COALESCE(u.avatar,'') AS avatar, " +
	"COALESCE(u.role,'user') AS author_role"

// List 分页查询问题列表。
//
// 排序固定为「有回答的按最近回答时间，无回答的按发布时间」：
// 待答区要让新问题浮上来，已答区要让刚有人回答的浮上来。
// 用 COALESCE(last_answer_at, created_at) 一条表达式同时覆盖两种情况，
// 比按 tab 分两套 SQL 更容易保持一致。
func (r *QaRepo) List(ctx context.Context, f QaFilter, offset, limit int) ([]QaQuestionRow, int64, error) {
	groups := f.KeywordGroups.Resolve(f.Keyword)

	countQ, err := filterByKeyword(ctx, r.applyFilter(r.db.WithContext(ctx).Model(&model.QaQuestion{}), f),
		qaSearchColumns, groups, "问答")
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计问答数量失败: %w", err))
	}
	if total == 0 {
		return []QaQuestionRow{}, 0, nil
	}

	findQ, err := filterByKeyword(ctx, r.applyFilter(r.db.WithContext(ctx).Model(&model.QaQuestion{}), f),
		qaSearchColumns, groups, "问答")
	if err != nil {
		return nil, 0, err
	}
	items := make([]QaQuestionRow, 0, limit)
	err = findQ.Select(qaSelect).
		Joins("LEFT JOIN users u ON u.id = qa_questions.user_id AND u.deleted_at IS NULL").
		Order("COALESCE(qa_questions.last_answer_at, qa_questions.created_at) DESC, qa_questions.id DESC").
		Offset(offset).Limit(limit).Scan(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询问答列表失败: %w", err))
	}
	return items, total, nil
}

// Get 按 ID 取问题。未软删即可见——问答不做「草稿」，提问即公开。
func (r *QaRepo) Get(ctx context.Context, id int64) (*QaQuestionRow, error) {
	var q QaQuestionRow
	err := r.db.WithContext(ctx).Model(&model.QaQuestion{}).Select(qaSelect).
		Joins("LEFT JOIN users u ON u.id = qa_questions.user_id AND u.deleted_at IS NULL").
		Where("qa_questions.id = ?", id).Limit(1).Scan(&q).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询问题失败: %w", err))
	}
	if q.ID == 0 {
		return nil, errcode.ErrQaNotFound
	}
	return &q, nil
}

// LastAskedAt 返回用户最近一次提问时间，用于发布限流。
func (r *QaRepo) LastAskedAt(ctx context.Context, userID int64) (*time.Time, error) {
	var t *time.Time
	err := r.db.WithContext(ctx).Model(&model.QaQuestion{}).
		Where("user_id = ?", userID).Order("id DESC").Limit(1).
		Pluck("created_at", &t).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询最近提问时间失败: %w", err))
	}
	return t, nil
}

// Create 发布问题。
func (r *QaRepo) Create(ctx context.Context, q *model.QaQuestion) error {
	if err := r.db.WithContext(ctx).Create(q).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("发布问题失败: %w", err))
	}
	return nil
}

// IncrView 浏览数自增。
//
// 失败只由调用方记日志，不影响详情返回：浏览数统计出错不该让用户看不到内容。
func (r *QaRepo) IncrView(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&model.QaQuestion{}).
		Where("id = ?", id).UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

// IncrAnswerCount 调整回答计数。delta 可为负（回答被删/被驳回）。
//
// 用表达式而不是「先读后写」：并发下读改写会丢计数。
// 下限在 SQL 里夹住，防止历史脏数据把计数减成负数后在页面上显示 -1。
func (r *QaRepo) IncrAnswerCount(ctx context.Context, id, delta int64) error {
	return r.db.WithContext(ctx).Model(&model.QaQuestion{}).
		Where("id = ?", id).
		UpdateColumn("answer_count", gorm.Expr("CASE WHEN answer_count + ? < 0 THEN 0 ELSE answer_count + ? END", delta, delta)).Error
}

// TouchLastAnswer 更新最近回答时间。
func (r *QaRepo) TouchLastAnswer(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&model.QaQuestion{}).
		Where("id = ?", id).
		UpdateColumn("last_answer_at", time.Now()).Error
}

// AcceptAnswer 采纳回答。
//
// 一条 CAS 同时完成三件事：校验提问人身份、校验问题仍处于 open、
// 写入采纳结果与状态。拆成「先查再改」会在并发下出现两次采纳都成功的可能
// （第二个回答覆盖第一个），从而给两个人都记了积分。
func (r *QaRepo) AcceptAnswer(ctx context.Context, questionID, answerID, userID int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.QaQuestion{}).
		Where("id = ? AND user_id = ? AND status = ?", questionID, userID, model.QaStatusOpen).
		Updates(map[string]any{
			"status":             model.QaStatusResolved,
			"accepted_answer_id": answerID,
			// 时间由 Go 侧给出而不是 CURRENT_TIMESTAMP：SQLite 的
			// CURRENT_TIMESTAMP 是 UTC，会写进一个比本地早 8 小时的时间
			// （十四·对账已经踩过一次同样的坑）。
			"accepted_at": time.Now(),
		})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("采纳回答失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// CloseQuestion 关闭问题（未解决也不再接收回答）。
func (r *QaRepo) CloseQuestion(ctx context.Context, questionID, userID int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.QaQuestion{}).
		Where("id = ? AND user_id = ? AND status = ?", questionID, userID, model.QaStatusOpen).
		Update("status", model.QaStatusClosed)
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("关闭问题失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// DeleteQuestion 软删问题（回答保留，见 service 层说明）。
func (r *QaRepo) DeleteQuestion(ctx context.Context, id int64) error {
	// 必须连回答一起软删：只删问题会留下一批孤儿回答——它们在任何一个页面里都
	// 看不见（详情 404、列表不可达），却仍然计入全局「已发布回答」数，
	// 后台待审队列里也会冒出标题为空、点进去 404 的行。
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 点赞行是纯关系表（物理删除、无 Base），不跟着软删就会一直留在表里占着 answer_id。
		answerIDs := tx.Model(&model.QaAnswer{}).Select("id").Where("question_id = ?", id)
		if err := tx.Where("answer_id IN (?)", answerIDs).Delete(&model.QaAnswerLike{}).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("删除问题下回答的点赞失败: %w", err))
		}
		if err := tx.Where("question_id = ?", id).Delete(&model.QaAnswer{}).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("删除问题下的回答失败: %w", err))
		}
		if err := tx.Delete(&model.QaQuestion{}, id).Error; err != nil {
			return errcode.ErrDatabase.Wrap(fmt.Errorf("删除问题失败: %w", err))
		}
		return nil
	})
}

// QaAnswerRow 回答列表行：回答本体 + 作者展示信息。
//
// JOIN users 一次取回昵称头像与积分，避免前端逐条再查。
type QaAnswerRow struct {
	model.QaAnswer
	Nickname           string `json:"nickname"`
	Avatar             string `json:"avatar"`
	ContributionPoints int64  `json:"contributionPoints"`
	// AuthorRole 用于展示「专家」标记：专家回答的可信度天然更高，值得一个角标。
	AuthorRole string `json:"authorRole"`
}

// Answers 取某问题下的回答。
//
// includePending 只有作者本人与管理员该传 true；公开访问一律 false。
// 排序：被采纳的置顶，其余按赞数、再按时间——赞数相同的新回答排前面更公平。
func (r *QaRepo) Answers(ctx context.Context, questionID int64, includePending bool, viewerID int64) ([]QaAnswerRow, error) {
	q := r.db.WithContext(ctx).Model(&model.QaAnswer{}).
		Select("qa_answers.*, COALESCE(u.nickname,'') AS nickname, COALESCE(u.avatar,'') AS avatar, "+
			"COALESCE(u.contribution_points,0) AS contribution_points, COALESCE(u.role,'user') AS author_role").
		Joins("LEFT JOIN users u ON u.id = qa_answers.user_id AND u.deleted_at IS NULL").
		Where("qa_answers.question_id = ?", questionID)

	if includePending {
		// 管理员：全部状态都要看得到（含已驳回，便于回溯处理记录）。
	} else if viewerID > 0 {
		// 访客带登录态：已发布的 + 自己那条待审/被驳回的。
		// 少了「自己那条」，作者投完回答会发现内容凭空消失，只会重发一遍。
		q = q.Where("qa_answers.status = ? OR qa_answers.user_id = ?", model.QaAnswerPublished, viewerID)
	} else {
		q = q.Where("qa_answers.status = ?", model.QaAnswerPublished)
	}

	rows := make([]QaAnswerRow, 0, 8)
	err := q.Order("qa_answers.is_accepted DESC, qa_answers.score DESC, qa_answers.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询回答失败: %w", err))
	}
	return rows, nil
}

// GetAnswer 按 ID 取回答。
func (r *QaRepo) GetAnswer(ctx context.Context, id int64) (*model.QaAnswer, error) {
	var a model.QaAnswer
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.ErrNotFound.WithMsg("回答不存在")
		}
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询回答失败: %w", err))
	}
	return &a, nil
}

// CreateAnswer 发布回答。
func (r *QaRepo) CreateAnswer(ctx context.Context, a *model.QaAnswer) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		if isDuplicateKeyError(err) {
			// 命中 (question_id, user_id) 唯一索引：一人一答。
			return errcode.ErrQaAlreadyAnswered
		}
		return errcode.ErrDatabase.Wrap(fmt.Errorf("发布回答失败: %w", err))
	}
	return nil
}

// UpdateAnswerContent 编辑自己的回答。
func (r *QaRepo) UpdateAnswerContent(ctx context.Context, id, userID int64, content string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.QaAnswer{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("content", content)
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("编辑回答失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// SetAnswerAccepted 标记/取消采纳标记（冗余字段，供「按回答查」使用）。
func (r *QaRepo) SetAnswerAccepted(ctx context.Context, answerID int64, accepted bool) error {
	if err := r.db.WithContext(ctx).Model(&model.QaAnswer{}).
		Where("id = ?", answerID).Update("is_accepted", accepted).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新采纳标记失败: %w", err))
	}
	return nil
}

// DeleteAnswer 软删回答。
func (r *QaRepo) DeleteAnswer(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&model.QaAnswer{}, id).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("删除回答失败: %w", err))
	}
	return nil
}

// ReviewAnswer 审核回答（CAS：只有仍处于 pending 的才能被处理）。
func (r *QaRepo) ReviewAnswer(ctx context.Context, id int64, status, note string, reviewerID int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.QaAnswer{}).
		Where("id = ? AND status = ?", id, model.QaAnswerPending).
		Updates(map[string]any{
			"status":      status,
			"review_note": note,
			"reviewer_id": reviewerID,
			"reviewed_at": time.Now(),
		})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("审核回答失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// CountPublishedByUser 统计某用户已发布的回答数。
//
// 用于「新账号前几条先审」：口径必须是已发布（含被驳回的不算），
// 否则一个反复发垃圾内容被驳回的人会永远停在「新人免审」之外。
func (r *QaRepo) CountPublishedByUser(ctx context.Context, userID int64) (int64, error) {
	var n int64
	// 问题已删的回答不该计入「前 3 条进待审」的门槛，否则用户被删帖删多了，
	// 之后本该进审的回答会直接免审发布。
	err := r.db.WithContext(ctx).Model(&model.QaAnswer{}).Joins(liveAnswerJoin).
		Where("qa_answers.user_id = ? AND qa_answers.status = ?", userID, model.QaAnswerPublished).Count(&n).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计用户回答数失败: %w", err))
	}
	return n, nil
}

// LikeAnswer 点赞。
//
// 返回 created=false 表示早就点过（唯一索引挡住），调用方按成功处理但不加积分——
// 幂等比报错友好：用户连点两下不该看到红字。
func (r *QaRepo) LikeAnswer(ctx context.Context, answerID, userID int64) (bool, error) {
	err := r.db.WithContext(ctx).Create(&model.QaAnswerLike{AnswerID: answerID, UserID: userID}).Error
	if err != nil {
		if isDuplicateKeyError(err) {
			return false, nil
		}
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("点赞失败: %w", err))
	}
	if err := r.db.WithContext(ctx).Model(&model.QaAnswer{}).Where("id = ?", answerID).
		UpdateColumn("score", gorm.Expr("score + 1")).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("更新点赞数失败: %w", err))
	}
	return true, nil
}

// UnlikeAnswer 取消点赞（物理删除）。
func (r *QaRepo) UnlikeAnswer(ctx context.Context, answerID, userID int64) (bool, error) {
	res := r.db.WithContext(ctx).
		Where("answer_id = ? AND user_id = ?", answerID, userID).
		Delete(&model.QaAnswerLike{})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("取消点赞失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	if err := r.db.WithContext(ctx).Model(&model.QaAnswer{}).Where("id = ?", answerID).
		UpdateColumn("score", gorm.Expr("CASE WHEN score > 0 THEN score - 1 ELSE 0 END")).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("更新点赞数失败: %w", err))
	}
	return true, nil
}

// LikedSet 返回 viewer 在给定回答里点过赞的集合。
//
// 一次 IN 查询而不是逐条判断：页面上十来个回答，逐条问就是十几次往返。
func (r *QaRepo) LikedSet(ctx context.Context, viewerID int64, answerIDs []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if viewerID <= 0 || len(answerIDs) == 0 {
		return out, nil
	}
	var liked []int64
	err := r.db.WithContext(ctx).Model(&model.QaAnswerLike{}).
		Where("user_id = ? AND answer_id IN ?", viewerID, answerIDs).
		Pluck("answer_id", &liked).Error
	if err != nil {
		return out, errcode.ErrDatabase.Wrap(fmt.Errorf("查询点赞状态失败: %w", err))
	}
	for _, id := range liked {
		out[id] = true
	}
	return out, nil
}

// PendingAnswers 后台待审回答列表。
// liveAnswerJoin 限定"问题还在"的回答。
//
// 两个用途：一是历史数据兜底（早期删问题只软删问题本身，答案留下成了孤儿），
// 二是防止后台待审队列出现标题为空、点进去 404 的行。
// 现在删问题已改为级联软删，新数据不会再产生孤儿，这条 JOIN 只作为约束保留。
const liveAnswerJoin = "JOIN qa_questions q ON q.id = qa_answers.question_id AND q.deleted_at IS NULL"

func (r *QaRepo) PendingAnswers(ctx context.Context, offset, limit int) ([]QaAnswerRow, int64, error) {
	base := r.db.WithContext(ctx).Model(&model.QaAnswer{}).Joins(liveAnswerJoin).
		Where("qa_answers.status = ?", model.QaAnswerPending)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计待审回答失败: %w", err))
	}
	if total == 0 {
		return []QaAnswerRow{}, 0, nil
	}

	rows := make([]QaAnswerRow, 0, limit)
	err := r.db.WithContext(ctx).Model(&model.QaAnswer{}).
		Select("qa_answers.*, COALESCE(u.nickname,'') AS nickname, COALESCE(u.avatar,'') AS avatar, "+
			"COALESCE(u.contribution_points,0) AS contribution_points, COALESCE(u.role,'user') AS author_role").
		Joins(liveAnswerJoin).
		Joins("LEFT JOIN users u ON u.id = qa_answers.user_id AND u.deleted_at IS NULL").
		Where("qa_answers.status = ?", model.QaAnswerPending).
		Order("qa_answers.id ASC").
		Offset(offset).Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询待审回答失败: %w", err))
	}
	return rows, total, nil
}

// QaStats 概览数字，给后台首页与问答列表顶部用。
type QaStats struct {
	Open     int64 `json:"open"`
	Resolved int64 `json:"resolved"`
	Pending  int64 `json:"pending"`
	Answers  int64 `json:"answers"`
}

// Stats 统计各状态数量。
func (r *QaRepo) Stats(ctx context.Context) (QaStats, error) {
	var s QaStats
	db := r.db.WithContext(ctx)
	if err := db.Model(&model.QaQuestion{}).Where("status = ?", model.QaStatusOpen).Count(&s.Open).Error; err != nil {
		return s, errcode.ErrDatabase.Wrap(fmt.Errorf("统计未解决问答失败: %w", err))
	}
	if err := db.Model(&model.QaQuestion{}).Where("status = ?", model.QaStatusResolved).Count(&s.Resolved).Error; err != nil {
		return s, errcode.ErrDatabase.Wrap(fmt.Errorf("统计已解决问答失败: %w", err))
	}
	if err := db.Model(&model.QaAnswer{}).Joins(liveAnswerJoin).Where("qa_answers.status = ?", model.QaAnswerPending).Count(&s.Pending).Error; err != nil {
		return s, errcode.ErrDatabase.Wrap(fmt.Errorf("统计待审回答失败: %w", err))
	}
	if err := db.Model(&model.QaAnswer{}).Joins(liveAnswerJoin).Where("qa_answers.status = ?", model.QaAnswerPublished).Count(&s.Answers).Error; err != nil {
		return s, errcode.ErrDatabase.Wrap(fmt.Errorf("统计回答数失败: %w", err))
	}
	return s, nil
}

// UpdateQuestionFields 供「精选转教程」后回写标记用（当前仅记录来源，不改状态）。
func (r *QaRepo) UpdateQuestionFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&model.QaQuestion{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新问题失败: %w", err))
	}
	return nil
}

// TitleOf 取问题标题（写审计/积分目标展示用，避免整行加载）。
func (r *QaRepo) TitleOf(ctx context.Context, id int64) string {
	var q struct{ Title string }
	_ = r.db.WithContext(ctx).Model(&model.QaQuestion{}).Select("title").Where("id = ?", id).Scan(&q).Error
	return strings.TrimSpace(q.Title)
}

// PointRepo 积分流水。
type PointRepo struct{ db *gorm.DB }

// NewPointRepo 创建仓储。
func NewPointRepo(db *gorm.DB) *PointRepo { return &PointRepo{db: db} }

// Award 记一笔积分（幂等）。
//
// 返回 awarded=false 表示这条积分早就记过（UniqKey 撞唯一索引），
// 此时**不再累加用户总分**——这正是幂等的意义：重放不会把人刷成高等级。
func (r *PointRepo) Award(ctx context.Context, log *model.PointLog) (bool, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(log).Error; err != nil {
			return err
		}
		// 总分与流水必须在同一事务：只写流水不累加，徽章永远不涨；
		// 只累加不写流水，出问题查不出这分是哪来的。
		return tx.Model(&model.User{}).Where("id = ?", log.UserID).
			UpdateColumn("contribution_points", gorm.Expr("contribution_points + ?", log.Points)).Error
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			return false, nil
		}
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("记录积分失败: %w", err))
	}
	return true, nil
}

// PointsOf 取用户当前积分。
func (r *PointRepo) PointsOf(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", userID).Pluck("contribution_points", &n).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询积分失败: %w", err))
	}
	return n, nil
}

// RecentOf 取用户最近的积分流水（个人中心展示「分从哪来」）。
func (r *PointRepo) RecentOf(ctx context.Context, userID int64, limit int) ([]model.PointLog, error) {
	items := make([]model.PointLog, 0, limit)
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("id DESC").Limit(limit).Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询积分流水失败: %w", err))
	}
	return items, nil
}
