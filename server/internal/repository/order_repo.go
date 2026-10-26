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

// OrderFilter 是订单列表的查询条件。
//
// 各维度是"与"的关系，但下面几个身份维度互斥的含义要写清楚：
//   - UserID：我下的单
//   - ExpertID：我接的单
//   - OnlyUnassigned：还没人接的单（接单大厅），与上面两个互斥
type OrderFilter struct {
	UserID   int64
	ExpertID int64
	// OnlyUnassigned 只看专家位仍为空的单，接单大厅用。
	OnlyUnassigned bool
	// OnlyAssigned 只看已分配专家的单（专家侧列表用，避免把没人接的单混进来）。
	OnlyAssigned bool

	Status   string
	Statuses []string
	Type     string
	OrderNo  string
	// Keyword 只匹配订单号与标题，不搜备注——备注里常写联系方式，不该被检索到。
	Keyword string

	CreatedFrom *time.Time
	CreatedTo   *time.Time

	// Sort 取值 latest（默认）/ oldest / amount。
	Sort string
}

// OrderRepo 订单数据访问。
type OrderRepo struct {
	db *gorm.DB
}

// NewOrderRepo 创建 OrderRepo。
func NewOrderRepo(db *gorm.DB) *OrderRepo { return &OrderRepo{db: db} }

/* ------------------------------------------------------------ 读 -- */

// GetByOrderNo 按订单号查询。订单号是对外暴露的标识，路由上只用它。
func (r *OrderRepo) GetByOrderNo(ctx context.Context, orderNo string) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("order_no = ?", strings.TrimSpace(orderNo)).First(&o).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrOrderNotFound)
	}
	return &o, nil
}

// GetByID 按主键查询。
func (r *OrderRepo) GetByID(ctx context.Context, id int64) (*model.Order, error) {
	var o model.Order
	if err := r.db.WithContext(ctx).First(&o, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrOrderNotFound)
	}
	return &o, nil
}

// GetByQuestionID 按悬赏问题查订单。一个问题只对应一笔订单（发布问题时就落了）。
func (r *OrderRepo) GetByQuestionID(ctx context.Context, questionID int64) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("question_id = ?", questionID).
		Order("id DESC").First(&o).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrOrderNotFound)
	}
	return &o, nil
}

// List 按条件分页查询，返回当页数据与总数。
func (r *OrderRepo) List(ctx context.Context, f OrderFilter, offset, limit int) ([]model.Order, int64, error) {
	base := func() *gorm.DB {
		return applyOrderFilter(r.db.WithContext(ctx).Model(&model.Order{}), f)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计订单数失败: %w", err))
	}
	if total == 0 {
		return []model.Order{}, 0, nil
	}

	items := make([]model.Order, 0, limit)
	err := base().Order(orderListClause(f.Sort)).Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询订单列表失败: %w", err))
	}
	return items, total, nil
}

// CountByStatus 统计某组条件下各状态的订单数，订单中心 Tab 角标与后台看板都用它。
func (r *OrderRepo) CountByStatus(ctx context.Context, f OrderFilter) (map[string]int64, error) {
	var rows []struct {
		Status string
		Total  int64
	}
	q := applyOrderFilter(r.db.WithContext(ctx).Model(&model.Order{}), f).
		Select("status, COUNT(*) AS total").
		Group("status")
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计订单状态分布失败: %w", err))
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Status] = row.Total
	}
	return counts, nil
}

// SumPaidAmountSince 统计某时间点之后已支付订单的成交额（分）。
//
// 口径按 paid_at 而非 created_at：看板要的是"这段时间收了多少钱"，
// 下单时间与到账时间可能跨天，按下单时间统计会让日报与流水对不上。
func (r *OrderRepo) SumPaidAmountSince(ctx context.Context, since time.Time) (int64, error) {
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.Order{}).
		Select("SUM(amount_cents)").
		Where("paid_at IS NOT NULL AND paid_at >= ?", since).
		Scan(&total).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计平台成交额失败: %w", err))
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

/* ------------------------------------------------------------ 写 -- */

// Create 落一笔订单。
func (r *OrderRepo) Create(ctx context.Context, o *model.Order) error {
	if err := r.db.WithContext(ctx).Create(o).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建订单失败: %w", err))
	}
	return nil
}

// Transition 原子地把订单从 from 流转到 to，并附带写入 extra 里的字段（如 paid_at）。
//
// 这是订单模块唯一允许改 status 的入口，理由：
//
//	status 的条件写在 WHERE 里，并发下只有一个请求的 RowsAffected 会是 1。
//	如果先查出来判断再写回（读改写），两个并发的「支付」请求都会读到 pending_pay、
//	都判定可以流转，于是同一笔钱被记两次支付、后写的时间线覆盖先写的。
//
// 失败时返回 errcode.ErrOrderStateInvalid（订单不存在则是 ErrOrderNotFound），
// 调用方直接回给用户即可，不需要自己再判一次状态。
//
// extra 里不允许出现 status——本方法会在最后强制覆盖，避免绕过 CAS 语义。
func (r *OrderRepo) Transition(ctx context.Context, orderNo, from, to string, extra map[string]any) error {
	fields := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		if k == "status" {
			continue // 由本方法统一设定，不接受覆盖
		}
		fields[k] = v
	}
	fields["status"] = to

	res := r.db.WithContext(ctx).Model(&model.Order{}).
		Where("order_no = ? AND status = ?", orderNo, from).
		Updates(fields)
	if res.Error != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("流转订单 %s 失败: %w", orderNo, res.Error))
	}
	if res.RowsAffected == 0 {
		// 到这里有两种可能：订单不存在，或者状态已经不是 from 了。
		// 重新查一次区分开来——一律报"状态不对"会让排查的人以为订单在，
		// 实际是单号写错了。
		if _, err := r.GetByOrderNo(ctx, orderNo); err != nil {
			return err
		}
		return errcode.ErrOrderStateInvalid
	}
	return nil
}

// Grab 专家抢单：把还没人接的单原子地分配给该专家，并转到待支付。
//
// 抢单是典型的"两人同时点，只能一个人成功"：
// expert_id = 0 与 status = pending_accept 两个条件一起放进 WHERE，
// 由数据库保证只有一个 UPDATE 命中。先查后写一定会出现两人都抢到——
// 而且两条 UPDATE 都成功，后一条把前一条的专家覆盖掉，两个人各自以为抢到了。
// 抢失败时还要能说清是"被别人接走"还是"状态已变"，所以命中 0 行后回查一次
// 当前值来区分（见下方 RowsAffected == 0 分支）。
//
// payExpireAt 由调用方按配置算好传进来：抢到单的那一刻订单就进入了
// "等付款"状态，支付倒计时必须同时开始。漏了它的话这条订单的
// pay_expire_at 永远是空的，超时关单扫不到它，用户可以被允许无限期挂着不付。
func (r *OrderRepo) Grab(ctx context.Context, orderID, expertUserID int64, now, payExpireAt time.Time) (*model.Order, error) {
	res := r.db.WithContext(ctx).Model(&model.Order{}).
		Where("id = ? AND status = ? AND expert_id = 0", orderID, model.OrderStatusPendingAccept).
		Updates(map[string]any{
			"expert_id":     expertUserID,
			"status":        model.OrderStatusPendingPay,
			"accepted_at":   now,
			"pay_expire_at": payExpireAt,
		})
	if res.Error != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("抢单失败: %w", res.Error))
	}
	if res.RowsAffected == 0 {
		// 抢失败只有三种可能，要说清楚是哪一种，否则前端只能弹一句"失败了"：
		//   ① 单不存在  ② 已经被别人接走  ③ 状态已不是待接单（被取消 / 已过期）
		current, err := r.GetByID(ctx, orderID)
		if err != nil {
			return nil, err
		}
		if current.ExpertID != 0 {
			return nil, errcode.ErrOrderTaken
		}
		return nil, errcode.ErrOrderStateInvalid
	}
	return r.GetByID(ctx, orderID)
}

// UpdateFields 局部更新订单，用于不改 status 的场景（补写取消原因、备注等）。
//
// 需要改 status 时不要用这个，用 Transition——否则绕过了状态机的并发保护。
func (r *OrderRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	if _, ok := fields["status"]; ok {
		return errcode.ErrInternal.Wrap(fmt.Errorf("UpdateFields 不允许改 status，请用 Transition"))
	}
	return updateFieldsByID(ctx, r.db, &model.Order{}, id, fields, "订单")
}

/* -------------------------------------------------- 超时扫描（定时任务用） -- */

// ListExpiredPendingPay 找出支付超时的待支付订单。
//
// pay_expire_at 为空表示这条订单从未进入过支付倒计时，不该被扫到——
// 用 IS NOT NULL 明确排除，否则刚落的单会被当成"早就过期"直接取消。
func (r *OrderRepo) ListExpiredPendingPay(ctx context.Context, now time.Time, limit int) ([]model.Order, error) {
	items := make([]model.Order, 0, limit)
	err := r.db.WithContext(ctx).
		Where("status = ? AND pay_expire_at IS NOT NULL AND pay_expire_at < ?",
			model.OrderStatusPendingPay, now).
		Order("pay_expire_at ASC, id ASC").Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("扫描超时待支付订单失败: %w", err))
	}
	return items, nil
}

// ListExpiredPendingConfirm 找出到了自动确认时间的待确认订单。
func (r *OrderRepo) ListExpiredPendingConfirm(ctx context.Context, now time.Time, limit int) ([]model.Order, error) {
	items := make([]model.Order, 0, limit)
	err := r.db.WithContext(ctx).
		Where("status = ? AND confirm_deadline IS NOT NULL AND confirm_deadline < ?",
			model.OrderStatusPendingConfirm, now).
		Order("confirm_deadline ASC, id ASC").Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("扫描待自动确认订单失败: %w", err))
	}
	return items, nil
}

// ListSettleable 找出已完成但还没结算的订单。
func (r *OrderRepo) ListSettleable(ctx context.Context, limit int) ([]model.Order, error) {
	items := make([]model.Order, 0, limit)
	err := r.db.WithContext(ctx).
		Where("status = ?", model.OrderStatusCompleted).
		Order("confirmed_at ASC, id ASC").Limit(limit).
		Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("扫描待结算订单失败: %w", err))
	}
	return items, nil
}

/* ------------------------------------------------------ 专家侧统计（沿用） -- */

// CountByExpert 统计某专家（按 users.id）的订单数。
// statuses 为空表示不限状态。
func (r *OrderRepo) CountByExpert(ctx context.Context, expertUserID int64, statuses ...string) (int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Order{}).Where("expert_id = ?", expertUserID)
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家订单数失败: %w", err))
	}
	return count, nil
}

// SumSettledAmountSince 统计某时间点之后已完成订单的成交额（分）。
func (r *OrderRepo) SumSettledAmountSince(ctx context.Context, expertUserID int64, since time.Time) (int64, error) {
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.Order{}).
		Select("SUM(amount_cents)").
		Where("expert_id = ? AND created_at >= ?", expertUserID, since).
		Scan(&total).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家成交额失败: %w", err))
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

/* ------------------------------------------------------------ 内部 -- */

func applyOrderFilter(q *gorm.DB, f OrderFilter) *gorm.DB {
	if f.UserID > 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.ExpertID > 0 {
		q = q.Where("expert_id = ?", f.ExpertID)
	}
	if f.OnlyUnassigned {
		q = q.Where("expert_id = 0")
	}
	if f.OnlyAssigned {
		q = q.Where("expert_id > 0")
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.OrderNo != "" {
		q = q.Where("order_no = ?", f.OrderNo)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		// 订单检索刻意不走查询理解层的同义词展开：这里输入的几乎都是
		// 订单号或标题片段，把「运维」展开成一堆近义词只会让精确查找变糊。
		q = q.Where("(order_no LIKE ? ESCAPE '\\' OR title LIKE ? ESCAPE '\\')",
			likePattern(kw), likePattern(kw))
	}
	if f.CreatedFrom != nil {
		q = q.Where("created_at >= ?", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		q = q.Where("created_at < ?", *f.CreatedTo)
	}
	return q
}

// orderListClause 返回订单列表排序语句。
//
// 末级一律带 id：只按 created_at 排时，同一秒创建的多笔订单在分页时顺序不稳定，
// 翻页会出现同一单出现两次而另一单从没出现。
func orderListClause(sort string) string {
	switch sort {
	case "amount":
		return "amount_cents DESC, id DESC"
	case "oldest":
		return "created_at ASC, id ASC"
	default:
		return "created_at DESC, id DESC"
	}
}
