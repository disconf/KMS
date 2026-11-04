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

// MallRepo 积分商城仓储。
//
// 兑换是全站唯一「扣积分」的地方，全部走一个事务里的三步 CAS：
// 扣用户积分（防负）→ 扣库存（防超卖）→ 写负分流水（对账依据）。
// 任何一步 RowsAffected 为 0 都说明条件不满足，回滚并返回对应错误。
type MallRepo struct{ db *gorm.DB }

// NewMallRepo 创建仓储。
func NewMallRepo(db *gorm.DB) *MallRepo { return &MallRepo{db: db} }

// MallItemFilter 商品列表过滤。
type MallItemFilter struct {
	// ActiveOnly 只看上架商品（前台）。后台管理不过滤。
	ActiveOnly bool
}

// ListItems 商品列表。
func (r *MallRepo) ListItems(ctx context.Context, f MallItemFilter) ([]model.MallItem, error) {
	items := make([]model.MallItem, 0, 16)
	q := r.db.WithContext(ctx).Model(&model.MallItem{})
	if f.ActiveOnly {
		q = q.Where("active = ?", true)
	}
	err := q.Order("sort ASC, id ASC").Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询商城商品失败: %w", err))
	}
	return items, nil
}

// ItemByID 取单个商品（不过滤上架状态，上下架判断在调用方）。
func (r *MallRepo) ItemByID(ctx context.Context, id int64) (*model.MallItem, error) {
	var item model.MallItem
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrMallItemNotFound
		}
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询商品失败: %w", err))
	}
	return &item, nil
}

// CreateItem / UpdateItemFields 商品管理。
func (r *MallRepo) CreateItem(ctx context.Context, item *model.MallItem) error {
	if err := r.db.WithContext(ctx).Create(item).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建商品失败: %w", err))
	}
	return nil
}

func (r *MallRepo) UpdateItemFields(ctx context.Context, id int64, fields map[string]any) error {
	err := r.db.WithContext(ctx).Model(&model.MallItem{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新商品失败: %w", err))
	}
	return nil
}

// DeleteItem 软删商品（仅后台清理用错数据时）。
func (r *MallRepo) DeleteItem(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.MallItem{}, "id = ?", id).Error
}

// RedeemOne 执行一次兑换事务，返回兑换单。
//
// 顺序有讲究：
//  1. 先建兑换单拿到 ID（负分流水的幂等键要引用它）；
//  2. CAS 扣积分，RowsAffected=0 即余额不足——这是最常见的失败，提示要友好；
//  3. CAS 扣库存（不限量商品跳过），RowsAffected=0 即超卖临界；
//  4. 写负分流水，与 Award 相同的事务纪律：流水与余额同生共死。
//
// 整个事务在 SQLite/MySQL 下都成立：CAS 语句自身带条件，不依赖 SELECT 快照。
func (r *MallRepo) RedeemOne(ctx context.Context, userID int64, item *model.MallItem) (*model.MallRedemption, error) {
	var redemption model.MallRedemption
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		redemption = model.MallRedemption{
			UserID:     userID,
			ItemID:     item.ID,
			ItemTitle:  item.Title,
			PointsCost: item.PointsCost,
			Status:     model.MallRedemptionPending,
		}
		if err := tx.Create(&redemption).Error; err != nil {
			return err
		}

		res := tx.Model(&model.User{}).
			Where("id = ? AND contribution_points >= ?", userID, item.PointsCost).
			UpdateColumn("contribution_points", gorm.Expr("contribution_points - ?", item.PointsCost))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.ErrMallPointsShort
		}

		if !item.Unlimited {
			res := tx.Model(&model.MallItem{}).
				Where("id = ? AND stock > 0", item.ID).
				UpdateColumn("stock", gorm.Expr("stock - 1"))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errcode.ErrMallStockEmpty
			}
		}

		log := model.PointLog{
			UserID:     userID,
			Action:     "mall.redeem",
			Points:     -item.PointsCost,
			TargetType: "redemption",
			TargetID:   redemption.ID,
			UniqKey:    fmt.Sprintf("mall.redeem:%d", redemption.ID),
		}
		return tx.Create(&log).Error
	})
	if err != nil {
		return nil, err
	}
	return &redemption, nil
}

// RedemptionByID 取兑换单（本人或管理员校验在 service 层）。
func (r *MallRepo) RedemptionByID(ctx context.Context, id int64) (*model.MallRedemption, error) {
	var rec model.MallRedemption
	err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrMallRedemption404
		}
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询兑换单失败: %w", err))
	}
	return &rec, nil
}

// ListRedemptionsOf 用户的兑换记录。
func (r *MallRepo) ListRedemptionsOf(ctx context.Context, userID int64, limit int) ([]model.MallRedemption, error) {
	items := make([]model.MallRedemption, 0, limit)
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("id DESC").Limit(limit).Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询兑换记录失败: %w", err))
	}
	return items, nil
}

// ListRedemptions 后台的兑换单队列。
func (r *MallRepo) ListRedemptions(ctx context.Context, status string, offset, limit int) ([]model.MallRedemption, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.MallRedemption{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计兑换单失败: %w", err))
	}
	items := make([]model.MallRedemption, 0, limit)
	if err := q.Order("id ASC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询兑换单失败: %w", err))
	}
	return items, total, nil
}

// FulfillRedemption 发放（CAS：pending → fulfilled）。
func (r *MallRepo) FulfillRedemption(ctx context.Context, id int64, remark string) (bool, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.MallRedemption{}).
		Where("id = ? AND status = ?", id, model.MallRedemptionPending).
		Updates(map[string]any{
			"status":       model.MallRedemptionFulfilled,
			"remark":       remark,
			"fulfilled_at": now,
		})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("发放兑换单失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// CancelRedemption 取消并退分（CAS：pending → cancelled，退分在同一事务）。
//
// 退分同样是「流水 + 余额」成对写：正分流水幂等键引用兑换单，
// 重复调用撞唯一索引，不会把人刷富。
func (r *MallRepo) CancelRedemption(ctx context.Context, rec *model.MallRedemption) (bool, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		res := tx.Model(&model.MallRedemption{}).
			Where("id = ? AND status = ?", rec.ID, model.MallRedemptionPending).
			Updates(map[string]any{
				"status":       model.MallRedemptionCancelled,
				"cancelled_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.ErrMallRedemptionBad
		}

		log := model.PointLog{
			UserID:     rec.UserID,
			Action:     "mall.refund",
			Points:     rec.PointsCost,
			TargetType: "redemption",
			TargetID:   rec.ID,
			UniqKey:    fmt.Sprintf("mall.refund:%d", rec.ID),
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", rec.UserID).
			UpdateColumn("contribution_points", gorm.Expr("contribution_points + ?", rec.PointsCost)).Error
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// CountPendingRedemptions 后台概览角标。
func (r *MallRepo) CountPendingRedemptions(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.MallRedemption{}).
		Where("status = ?", model.MallRedemptionPending).Count(&n).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计待发放兑换单失败: %w", err))
	}
	return n, nil
}
