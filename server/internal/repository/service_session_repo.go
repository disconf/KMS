package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// ServiceSessionRepo 服务会话数据访问。
type ServiceSessionRepo struct {
	db *gorm.DB
}

// NewServiceSessionRepo 创建 ServiceSessionRepo。
func NewServiceSessionRepo(db *gorm.DB) *ServiceSessionRepo {
	return &ServiceSessionRepo{db: db}
}

// Create 创建会话。
//
// order_id 上没有唯一索引：一次订单可以有多次连线（首次沟通不顺利、
// 换个时间再连一次都很正常），所以这里不做"一单一会话"的约束。
func (r *ServiceSessionRepo) Create(ctx context.Context, s *model.ServiceSession) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建服务会话失败: %w", err))
	}
	return nil
}

// GetByID 按主键查询。
func (r *ServiceSessionRepo) GetByID(ctx context.Context, id int64) (*model.ServiceSession, error) {
	var s model.ServiceSession
	if err := r.db.WithContext(ctx).First(&s, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrSessionNotFound)
	}
	return &s, nil
}

// ActiveOfOrder 查订单当前进行中的会话（created / ongoing）。
//
// 用于"重复点开始服务不要再建一个会话"：一个订单同一时刻只该有一个活着的会话，
// 否则计时会分散在两个会话上，订单时长统计不出来。
func (r *ServiceSessionRepo) ActiveOfOrder(ctx context.Context, orderID int64) (*model.ServiceSession, error) {
	var s model.ServiceSession
	err := r.db.WithContext(ctx).
		Where("order_id = ? AND status IN ?", orderID,
			[]string{model.SessionStatusCreated, model.SessionStatusOngoing}).
		Order("id DESC").First(&s).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrSessionNotFound)
	}
	return &s, nil
}

// ListByOrder 列出订单的全部会话，最新在前。
func (r *ServiceSessionRepo) ListByOrder(ctx context.Context, orderID int64) ([]model.ServiceSession, error) {
	items := make([]model.ServiceSession, 0, 4)
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).
		Order("id DESC").Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询订单会话失败: %w", err))
	}
	return items, nil
}

// Start 开始计时。
//
// 计时基准放在服务端：客户端上报的时长不可信（网络延迟、时钟不准、
// 甚至关掉页面就不上报了），所以只记服务端时间，时长由两端时间相减得出。
func (r *ServiceSessionRepo) Start(ctx context.Context, id int64, now time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.ServiceSession{}).
		Where("id = ? AND status = ?", id, model.SessionStatusCreated).
		Updates(map[string]any{
			"status":     model.SessionStatusOngoing,
			"start_time": now,
		})
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("开始会话失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return errcode.ErrSessionStateBad
	}
	return nil
}

// End 结束会话并落到服务端算出的时长。
func (r *ServiceSessionRepo) End(ctx context.Context, id int64, now time.Time, durationSec int64) error {
	res := r.db.WithContext(ctx).Model(&model.ServiceSession{}).
		Where("id = ? AND status = ?", id, model.SessionStatusOngoing).
		Updates(map[string]any{
			"status":       model.SessionStatusEnded,
			"end_time":     now,
			"duration_sec": durationSec,
		})
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("结束会话失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return errcode.ErrSessionStateBad
	}
	return nil
}

// Cancel 取消会话。取消不改时长，因为这次服务没真正发生。
func (r *ServiceSessionRepo) Cancel(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Model(&model.ServiceSession{}).
		Where("id = ? AND status IN ?", id,
			[]string{model.SessionStatusCreated, model.SessionStatusOngoing}).
		Update("status", model.SessionStatusCancelled)
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("取消会话失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		return errcode.ErrSessionStateBad
	}
	return nil
}

// SumDurationOfOrder 汇总订单已发生的服务时长（秒），不含已取消的会话。
func (r *ServiceSessionRepo) SumDurationOfOrder(ctx context.Context, orderID int64) (int64, error) {
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.ServiceSession{}).
		Select("SUM(duration_sec)").
		Where("order_id = ? AND status = ?", orderID, model.SessionStatusEnded).
		Scan(&total).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("汇总订单服务时长失败: %w", err))
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}
