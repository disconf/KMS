package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// SettlementRepo 分账记录数据访问。
type SettlementRepo struct {
	db *gorm.DB
}

// NewSettlementRepo 创建 SettlementRepo。
func NewSettlementRepo(db *gorm.DB) *SettlementRepo { return &SettlementRepo{db: db} }

// Create 写入一条分账记录。
//
// order_id 上有唯一索引，"一单只结算一次"交给索引守：
// 定时任务重跑、或者两个人同时点了结算，也只有一条能落进去。
// 靠"先查有没有结算过再插入"是挡不住的，两次检查会同时看到"没有"。
func (r *SettlementRepo) Create(ctx context.Context, s *model.Settlement) error {
	err := r.db.WithContext(ctx).Create(s).Error
	if err != nil {
		if isDuplicateKeyError(err) {
			return errcode.ErrSettleDuplicated
		}
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建分账记录失败: %w", err))
	}
	return nil
}

// GetByOrderID 按订单查分账记录。
func (r *SettlementRepo) GetByOrderID(ctx context.Context, orderID int64) (*model.Settlement, error) {
	var s model.Settlement
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&s).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &s, nil
}

// MarkSettled 把分账记录标记为已结算。
func (r *SettlementRepo) MarkSettled(ctx context.Context, id int64, now time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.Settlement{}).
		Where("id = ? AND status = ?", id, model.SettlementStatusPending).
		Updates(map[string]any{"status": model.SettlementStatusSettled, "settled_at": now})
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("标记分账完成失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return errcode.ErrSettleDuplicated
	}
	return nil
}

// SettleOrder 把一笔已完成订单结算掉：占订单、写分账记录、给专家加余额，三件事同一事务。
//
// 顺序是刻意的——**先用带条件的 UPDATE 抢占订单，抢到才继续**：
//
//	① UPDATE orders SET status='settled' WHERE status='completed'
//	   定时任务重跑、或两个人同时点结算时，只有一个请求能把 completed 改走，
//	   另一个 RowsAffected=0 直接退出。这就是"不会重复给专家打钱"的根据。
//	② 写 settlements（order_id 唯一索引是第二道防线）
//	③ experts.balance_cents += 专家实收
//
// 如果先写分账记录再占订单，两个并发请求会各写一条记录，然后一条被卡住，
// 结果是专家收到两份钱——分账记录还都是"成功"的，无从对账。
func (r *SettlementRepo) SettleOrder(ctx context.Context, s *model.Settlement, now time.Time) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// ① 抢占订单
		res := tx.Model(&model.Order{}).
			Where("order_no = ? AND status = ?", s.OrderNo, model.OrderStatusCompleted).
			Updates(map[string]any{
				"status":     model.OrderStatusSettled,
				"settled_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.ErrSettleNotAllowed
		}

		// ② 分账记录
		if err := tx.Create(s).Error; err != nil {
			return err
		}

		// ③ 到账。金额为 0（比如全额退款后仍需结算收尾）时跳过，避免写一个空流水。
		if s.ExpertAmountCents > 0 {
			if err := tx.Model(&model.Expert{}).Where("user_id = ?", s.ExpertID).
				Updates(map[string]any{
					"balance_cents":      gorm.Expr("balance_cents + ?", s.ExpertAmountCents),
					"total_income_cents": gorm.Expr("total_income_cents + ?", s.ExpertAmountCents),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})

	switch {
	case err == nil:
		return nil
	case errors.Is(err, errcode.ErrSettleNotAllowed):
		// 订单不是 completed，说明已经被结算过或还没到结算条件。
		return errcode.ErrSettleNotAllowed
	case isDuplicateKeyError(err):
		return errcode.ErrSettleDuplicated
	default:
		return errcode.ErrDatabase.Wrap(fmt.Errorf("结算订单 %s 失败: %w", s.OrderNo, err))
	}
}

// ListByExpert 返回专家的收入明细，按时间倒序。
func (r *SettlementRepo) ListByExpert(ctx context.Context, expertUserID int64, limit int) ([]model.Settlement, error) {
	if limit <= 0 {
		limit = 20
	}
	items := make([]model.Settlement, 0, limit)
	err := r.db.WithContext(ctx).
		Where("expert_id = ?", expertUserID).
		Order("id DESC").Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询收入明细失败: %w", err))
	}
	return items, nil
}

// SumSettledSince 统计某时间点之后已结算的专家实收金额（分）。
func (r *SettlementRepo) SumSettledSince(ctx context.Context, expertUserID int64, since time.Time) (int64, error) {
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.Settlement{}).
		Select("SUM(expert_amount_cents)").
		Where("expert_id = ? AND status = ? AND created_at >= ?",
			expertUserID, model.SettlementStatusSettled, since).
		Scan(&total).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计已结算收入失败: %w", err))
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// WithdrawalRepo 提现记录数据访问。
type WithdrawalRepo struct {
	db *gorm.DB
}

// NewWithdrawalRepo 创建 WithdrawalRepo。
func NewWithdrawalRepo(db *gorm.DB) *WithdrawalRepo { return &WithdrawalRepo{db: db} }

// Create 创建提现申请。
func (r *WithdrawalRepo) Create(ctx context.Context, w *model.Withdrawal) error {
	if err := r.db.WithContext(ctx).Create(w).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建提现申请失败: %w", err))
	}
	return nil
}

// ListByExpert 返回专家的提现记录。
func (r *WithdrawalRepo) ListByExpert(ctx context.Context, expertUserID int64, limit int) ([]model.Withdrawal, error) {
	if limit <= 0 {
		limit = 20
	}
	items := make([]model.Withdrawal, 0, limit)
	err := r.db.WithContext(ctx).
		Where("expert_id = ?", expertUserID).
		Order("id DESC").Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询提现记录失败: %w", err))
	}
	return items, nil
}

// SumPending 统计处理中的提现金额（分）。
// 待审核与待打款都属于"处理中"，前端要能看到这部分钱去哪了。
func (r *WithdrawalRepo) SumPending(ctx context.Context, expertUserID int64) (int64, error) {
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.Withdrawal{}).
		Select("SUM(amount_cents)").
		Where("expert_id = ? AND status IN ?", expertUserID,
			[]string{model.WithdrawalStatusPending, model.WithdrawalStatusApproved}).
		Scan(&total).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计处理中提现失败: %w", err))
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// HasPending 判断是否存在处理中的提现申请。
// 有未完成的申请时不允许再次提交，避免同一笔余额被重复申请。
func (r *WithdrawalRepo) HasPending(ctx context.Context, expertUserID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Withdrawal{}).
		Where("expert_id = ? AND status IN ?", expertUserID,
			[]string{model.WithdrawalStatusPending, model.WithdrawalStatusApproved}).
		Count(&count).Error
	if err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("查询提现状态失败: %w", err))
	}
	return count > 0, nil
}

// AdminWithdrawalRow 后台提现列表行：提现单 + 专家档案。
//
// 专家姓名与当前余额要一起带出来，因为审核时真正要核对的是
// 「收款人姓名是否与实名一致」「这笔钱占他余额多少」。
// 只给一个 expert_id，处理一笔要多点三次接口。
type AdminWithdrawalRow struct {
	model.Withdrawal

	ExpertName    string
	ExpertNick    string
	ExpertStatus  string
	ExpertBalance int64
	ExpertFrozen  int64
}

// WithdrawalAdminFilter 后台提现筛选条件。
type WithdrawalAdminFilter struct {
	Status  string
	Keyword string
}

// adminQuery 后台列表与计数的共用条件。
//
// 手写 JOIN 而不是用 Preload：提现的 expert_id 存的是**用户 ID**，
// 与 experts.user_id 对应，不是 experts.id。写成 Preload 会按主键关联，静默关联到别人。
func (r *WithdrawalRepo) adminQuery(ctx context.Context, f WithdrawalAdminFilter) *gorm.DB {
	q := r.db.WithContext(ctx).
		Table("withdrawals AS w").
		Joins("LEFT JOIN experts AS e ON e.user_id = w.expert_id AND e.deleted_at IS NULL").
		Where("w.deleted_at IS NULL")
	if f.Status != "" {
		q = q.Where("w.status = ?", f.Status)
	}
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		q = q.Where("w.withdrawal_no LIKE ? OR e.real_name LIKE ?", like, like)
	}
	return q
}

// ListAdmin 后台提现列表。默认把待处理的排在前面，其次按申请时间倒序。
func (r *WithdrawalRepo) ListAdmin(ctx context.Context, f WithdrawalAdminFilter, offset, limit int) ([]AdminWithdrawalRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	base := r.adminQuery(ctx, f)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计提现记录失败: %w", err))
	}

	rows := make([]AdminWithdrawalRow, 0, limit)
	err := r.adminQuery(ctx, f).
		Select(`w.*, e.real_name AS expert_name, u.nickname AS expert_nick,
			e.status AS expert_status, e.balance_cents AS expert_balance,
			e.frozen_cents AS expert_frozen`).
		Joins("LEFT JOIN users AS u ON u.id = w.expert_id AND u.deleted_at IS NULL").
		Order("CASE w.status WHEN 'pending' THEN 0 WHEN 'approved' THEN 1 ELSE 2 END, w.id DESC").
		Offset(offset).Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询提现记录失败: %w", err))
	}
	return rows, total, nil
}

// GetByID 按 ID 取提现单。
func (r *WithdrawalRepo) GetByID(ctx context.Context, id int64) (*model.Withdrawal, error) {
	var w model.Withdrawal
	err := r.db.WithContext(ctx).First(&w, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.ErrWithdrawalNotFound
	}
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询提现记录失败: %w", err))
	}
	return &w, nil
}

// UpdateFieldsIfStatus 条件更新：只有当前状态仍是 from 时才写入。
//
// 返回值 false 表示状态已经被别人改过（两个人同时点通过、或者专家侧刚好撤单），
// 调用方必须据此拒绝后续的钱款操作——这正是「同一笔钱只动一次」的仲裁点。
func (r *WithdrawalRepo) UpdateFieldsIfStatus(ctx context.Context, id int64, from string, fields map[string]any) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Withdrawal{}).
		Where("id = ? AND status = ?", id, from).
		Updates(fields)
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("更新提现状态失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}
