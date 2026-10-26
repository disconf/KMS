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

// QuestionFilter 悬赏问题列表的查询条件。
type QuestionFilter struct {
	UserID   int64
	Status   string
	Statuses []string
	// SkillID 按技术栈标签筛，接单大厅的"我擅长的方向"用。
	SkillID string
	Keyword string
	Sort    string
}

// QuestionRepo 悬赏问题数据访问。
type QuestionRepo struct {
	db *gorm.DB
}

// NewQuestionRepo 创建 QuestionRepo。
func NewQuestionRepo(db *gorm.DB) *QuestionRepo { return &QuestionRepo{db: db} }

// Create 发布一个问题。
func (r *QuestionRepo) Create(ctx context.Context, q *model.Question) error {
	if err := r.db.WithContext(ctx).Create(q).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建问题失败: %w", err))
	}
	return nil
}

// CreateWithOrder 在同一个事务里创建问题与它的订单。
//
// 为什么跨两张表也要放在仓储层：事务需要 *gorm.DB，而 service 层不持有它
// （这是本项目分层的既定边界，事务一律写在 repo 里）。
//
// 为什么必须原子：悬赏问题与它的订单是同一个业务对象的两面——
// 只落问题不落订单，问题会一直停在"待响应"却永远不出现在接单大厅
// （大厅读的是 pending_accept 订单），用户以为发出去了，专家永远看不到；
// 只落订单不落问题，订单详情点进去没有故障描述可读。
// 这两种半成品都不会报错，只能靠事务避免。
func (r *QuestionRepo) CreateWithOrder(ctx context.Context, q *model.Question, o *model.Order) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(q).Error; err != nil {
			return err
		}
		// 订单要回填问题 ID，所以必须在问题落库拿到自增主键之后再建。
		o.QuestionID = q.ID
		return tx.Create(o).Error
	})
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("发布悬赏失败: %w", err))
	}
	return nil
}

// ListByIDs 批量查询问题，避免列表页按订单逐条查问题造成 N+1。
//
// 返回 map 而不是切片：调用方是按订单里的 question_id 去取的，
// 用切片还要再建一次索引，白白多一遍循环。
func (r *QuestionRepo) ListByIDs(ctx context.Context, ids []int64) (map[int64]*model.Question, error) {
	out := make(map[int64]*model.Question, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	items := make([]model.Question, 0, len(ids))
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&items).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("批量查询问题失败: %w", err))
	}
	for i := range items {
		out[items[i].ID] = &items[i]
	}
	return out, nil
}

// GetByID 按主键查询。
func (r *QuestionRepo) GetByID(ctx context.Context, id int64) (*model.Question, error) {
	var q model.Question
	if err := r.db.WithContext(ctx).First(&q, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrQuestionNotFound)
	}
	return &q, nil
}

// UpdateFields 局部更新问题。
func (r *QuestionRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	return updateFieldsByID(ctx, r.db, &model.Question{}, id, fields, "问题")
}

// Transition 原子地把问题从 from 流转到 to。理由与订单一致：并发下只有带条件的
// UPDATE 靠得住，「已被别人接单」这类判断必须由数据库来裁决。
func (r *QuestionRepo) Transition(ctx context.Context, id int64, from, to string, extra map[string]any) error {
	fields := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		if k == "status" {
			continue
		}
		fields[k] = v
	}
	fields["status"] = to

	res := r.db.WithContext(ctx).Model(&model.Question{}).
		Where("id = ? AND status = ?", id, from).
		Updates(fields)
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("流转问题 %d 失败: %w", id, res.Error))
	}
	if res.RowsAffected == 0 {
		if _, err := r.GetByID(ctx, id); err != nil {
			return err
		}
		return errcode.ErrQuestionStateBad
	}
	return nil
}

// List 分页查询。
func (r *QuestionRepo) List(ctx context.Context, f QuestionFilter, offset, limit int) ([]model.Question, int64, error) {
	base := func() *gorm.DB {
		return applyQuestionFilter(r.db.WithContext(ctx).Model(&model.Question{}), f)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计问题数失败: %w", err))
	}
	if total == 0 {
		return []model.Question{}, 0, nil
	}

	items := make([]model.Question, 0, limit)
	err := base().Order(questionListClause(f.Sort)).Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询问题列表失败: %w", err))
	}
	return items, total, nil
}

// CountByUser 统计某用户发布的问题数，按状态分布。
func (r *QuestionRepo) CountByUser(ctx context.Context, userID int64) (map[string]int64, error) {
	var rows []struct {
		Status string
		Total  int64
	}
	err := r.db.WithContext(ctx).Model(&model.Question{}).
		Select("status, COUNT(*) AS total").
		Where("user_id = ?", userID).
		Group("status").Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计用户问题分布失败: %w", err))
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Status] = row.Total
	}
	return counts, nil
}

// ListExpiredOpen 找出悬赏已过期但仍处于待响应状态的问题（定时任务用）。
//
// 只扫 open：已经有人接单（matched）的问题不能因为悬赏期到了就作废，
// 专家已经投入了时间，把它标成过期等于把专家的付出抹掉。
func (r *QuestionRepo) ListExpiredOpen(ctx context.Context, now time.Time, limit int) ([]model.Question, error) {
	items := make([]model.Question, 0, limit)
	err := r.db.WithContext(ctx).
		Where("status = ? AND expired_at < ?", model.QuestionStatusOpen, now).
		Order("expired_at ASC, id ASC").Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("扫描过期悬赏失败: %w", err))
	}
	return items, nil
}

func applyQuestionFilter(q *gorm.DB, f QuestionFilter) *gorm.DB {
	if f.UserID > 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	if skill := strings.TrimSpace(f.SkillID); skill != "" {
		// 标签列是 JSON 数组，比对时带上引号，避免 "10" 命中 "110"。
		q = q.Where("skill_ids LIKE ? ESCAPE '\\'", likePattern(`"`+skill+`"`))
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		q = q.Where("(title LIKE ? ESCAPE '\\' OR description LIKE ? ESCAPE '\\')",
			likePattern(kw), likePattern(kw))
	}
	return q
}

// questionListClause 默认按悬赏金额倒序，其次最新。
//
// 接单大厅里专家先看钱：只按时间排会让 99 元的单把 999 元的单挤到第二页，
// 而专家一天只翻一两页。金额相同时再按时间，保证分页稳定。
func questionListClause(sort string) string {
	switch sort {
	case "latest":
		return "created_at DESC, id DESC"
	case "expiring":
		return "expired_at ASC, id ASC"
	default:
		return "budget_cents DESC, created_at DESC, id DESC"
	}
}
