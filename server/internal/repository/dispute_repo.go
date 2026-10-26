package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// DisputeFilter 争议列表的查询条件。
type DisputeFilter struct {
	OrderID  int64
	UserID   int64
	ExpertID int64
	Status   string
	Statuses []string
}

// DisputeRepo 订单争议数据访问。
type DisputeRepo struct {
	db *gorm.DB
}

// NewDisputeRepo 创建 DisputeRepo。
func NewDisputeRepo(db *gorm.DB) *DisputeRepo { return &DisputeRepo{db: db} }

// Create 发起争议。
func (r *DisputeRepo) Create(ctx context.Context, d *model.Dispute) error {
	if err := r.db.WithContext(ctx).Create(d).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("发起争议失败: %w", err))
	}
	return nil
}

// GetByID 按主键查询。
func (r *DisputeRepo) GetByID(ctx context.Context, id int64) (*model.Dispute, error) {
	var d model.Dispute
	if err := r.db.WithContext(ctx).First(&d, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &d, nil
}

// OpenOfOrder 查订单当前未结案的争议。
//
// 只看 open / processing：已解决的争议留在表里作为历史，
// 不能因为"这单以前有过争议"就挡住用户再次发起。
func (r *DisputeRepo) OpenOfOrder(ctx context.Context, orderID int64) (*model.Dispute, error) {
	var d model.Dispute
	err := r.db.WithContext(ctx).
		Where("order_id = ? AND status IN ?", orderID,
			[]string{model.DisputeStatusOpen, model.DisputeStatusProcessing}).
		Order("id DESC").First(&d).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &d, nil
}

// LatestOfOrder 查订单最近的一条争议，不论是否已结案。
//
// 与 OpenOfOrder 的分工：
//   - OpenOfOrder 回答"还能不能再发起"（只看未结案，结案的不该挡住新争议）；
//   - LatestOfOrder 回答"这单最后是怎么收场的"（要包含已结案的结论）。
//
// 订单详情用后者。只用前者的话，争议一旦处理完就从详情里消失了，
// 用户看到订单变成"已退款"却找不到退款理由与处理结论。
func (r *DisputeRepo) LatestOfOrder(ctx context.Context, orderID int64) (*model.Dispute, error) {
	var d model.Dispute
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("id DESC").First(&d).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &d, nil
}

// UpdateFields 局部更新争议。
func (r *DisputeRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	return updateFieldsByID(ctx, r.db, &model.Dispute{}, id, fields, "争议")
}

// Resolve 结案。带 from 条件，避免两个人同时点"处理"把结果互相覆盖。
func (r *DisputeRepo) Resolve(ctx context.Context, id int64, from []string, to, result string, handlerID int64, now time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.Dispute{}).
		Where("id = ? AND status IN ?", id, from).
		Updates(map[string]any{
			"status":     to,
			"result":     result,
			"handler_id": handlerID,
			"handled_at": now,
		})
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("处理争议失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return errcode.ErrDisputeClosed
	}
	return nil
}

// List 分页查询。
func (r *DisputeRepo) List(ctx context.Context, f DisputeFilter, offset, limit int) ([]model.Dispute, int64, error) {
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.Dispute{})
		if f.OrderID > 0 {
			q = q.Where("order_id = ?", f.OrderID)
		}
		if f.UserID > 0 {
			q = q.Where("user_id = ?", f.UserID)
		}
		if f.ExpertID > 0 {
			q = q.Where("expert_id = ?", f.ExpertID)
		}
		if f.Status != "" {
			q = q.Where("status = ?", f.Status)
		}
		if len(f.Statuses) > 0 {
			q = q.Where("status IN ?", f.Statuses)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计争议数失败: %w", err))
	}
	if total == 0 {
		return []model.Dispute{}, 0, nil
	}

	items := make([]model.Dispute, 0, limit)
	err := base().Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询争议列表失败: %w", err))
	}
	return items, total, nil
}

// CountOpen 统计未结案的争议数，后台待办角标用。
func (r *DisputeRepo) CountOpen(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Dispute{}).
		Where("status IN ?", []string{model.DisputeStatusOpen, model.DisputeStatusProcessing}).
		Count(&count).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计未结案争议失败: %w", err))
	}
	return count, nil
}
