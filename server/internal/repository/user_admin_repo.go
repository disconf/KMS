package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// UserAdminRepo 后台用户治理的读写。
//
// 为什么不塞进 UserRepo：UserRepo 服务的是账号本体（登录、绑定、改资料），
// 它不该知道"订单统计""专家档案"这些东西。后台列表要一眼看出
// 「这个人有没有在途订单、是不是专家、账上还有多少钱」，
// 这些是跨三张表的治理视图，单独放一层，改动不会波及登录链路。
type UserAdminRepo struct {
	db *gorm.DB
}

// NewUserAdminRepo 创建 UserAdminRepo。
func NewUserAdminRepo(db *gorm.DB) *UserAdminRepo { return &UserAdminRepo{db: db} }

// UserAdminFilter 后台用户列表筛选条件。
type UserAdminFilter struct {
	// Keyword 支持两种输入：完整手机号（11 位数字，走全等）与昵称/邮箱（走模糊）。
	// 手机号不做模糊匹配——后台列表里手机号是脱敏的，能模糊搜就等于
	// 把「猜一个号段里有哪些人」的能力开放出去了。
	Keyword string
	Role    string
	Status  string
}

// UserAdminRow 后台用户列表行。
type UserAdminRow struct {
	model.User

	ExpertID     int64  `gorm:"column:expert_id"`
	ExpertStatus string `gorm:"column:expert_status"`
}

// orderStatsRow 订单聚合的扫描目标。
type orderStatsRow struct {
	Total       int64 `gorm:"column:total"`
	Active      int64 `gorm:"column:active"`
	PaidCents   int64 `gorm:"column:paid_cents"`
	IncomeCents int64 `gorm:"column:income_cents"`
}

// List 后台用户列表。total 与 rows 必须共用同一份 where，否则翻页会出现
// 「总数说有 30 个、翻到第二页却空了」。
func (r *UserAdminRepo) List(ctx context.Context, f UserAdminFilter, offset, limit int) ([]UserAdminRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	var total int64
	if err := r.adminQuery(ctx, f).Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计用户数量失败: %w", err))
	}
	if total == 0 {
		return []UserAdminRow{}, 0, nil
	}

	rows := make([]UserAdminRow, 0, limit)
	err := r.adminQuery(ctx, f).
		Select("u.*, e.id AS expert_id, e.status AS expert_status").
		Joins("LEFT JOIN experts AS e ON e.user_id = u.id AND e.deleted_at IS NULL").
		// 管理员排前面：后台进来第一眼通常是找自己或找同事，不是找新注册用户。
		// 同角色内按注册时间倒序，最近注册的更要盯。
		Order("CASE u.role WHEN 'admin' THEN 0 WHEN 'expert' THEN 1 ELSE 2 END, u.id DESC").
		Offset(offset).Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询用户列表失败: %w", err))
	}
	return rows, total, nil
}

// adminQuery 构造后台用户查询条件。
//
// 用 Table("users AS u") 而不是 Model(&model.User{})：后面要 JOIN experts，
// 两张表都有 id / status / created_at，不给别名就只能写含糊的裸列名。
// 代价是 GORM 的软删除自动条件失效，所以 deleted_at 必须手写——已经写在下面。
func (r *UserAdminRepo) adminQuery(ctx context.Context, f UserAdminFilter) *gorm.DB {
	q := r.db.WithContext(ctx).Table("users AS u").Where("u.deleted_at IS NULL")

	if f.Role != "" {
		q = q.Where("u.role = ?", f.Role)
	}
	if f.Status != "" {
		q = q.Where("u.status = ?", f.Status)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		if isPhoneLike(kw) {
			// 手机号走全等：脱敏展示 + 精确搜索，运营拿到工单上的完整号码能找人，
			// 但没法用一个片段把同号段的人全捞出来。
			q = q.Where("u.phone = ?", kw)
		} else {
			p := likePattern(kw)
			q = q.Where("(u.nickname LIKE ? ESCAPE '\\' OR u.email LIKE ? ESCAPE '\\')", p, p)
		}
	}
	return q
}

// isPhoneLike 判断关键词是否像一个完整手机号。
//
// 只认 11 位纯数字：太短的数字（如 "138"）在昵称里也常见，
// 按手机号全等去查会得到一个空列表，用户会以为人不存在。
func isPhoneLike(kw string) bool {
	if len(kw) != 11 {
		return false
	}
	for _, c := range kw {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// FindByID 按 ID 取用户（不过滤状态，后台就是要看到被封禁的人）。
func (r *UserAdminRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrAccountNotFound)
	}
	return &u, nil
}

// FindExpert 取该用户的专家档案；不是专家时返回 nil, nil。
func (r *UserAdminRepo) FindExpert(ctx context.Context, userID int64) (*model.Expert, error) {
	var e model.Expert
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询专家档案失败: %w", err))
	}
	return &e, nil
}

// UserOrderStats 用户相关的订单聚合：作为下单人花了多少、作为专家挣了多少、
// 手上还有几笔在途。
//
// 封禁前必须看这两组数：一个还有三笔在途订单的用户被直接封掉，
// 那三笔单子会卡在「服务中」无人能推进，用户和专家都来投诉。
type UserOrderStats struct {
	AsUserTotal         int64 `json:"asUserTotal"`
	AsUserPaidCents     int64 `json:"asUserPaidCents"`
	AsExpertTotal       int64 `json:"asExpertTotal"`
	AsExpertIncomeCents int64 `json:"asExpertIncomeCents"`
	ActiveCount         int64 `json:"activeCount"`
}

// OrderStats 汇总该用户的订单数据。两次聚合查询，都是走索引的计数。
func (r *UserAdminRepo) OrderStats(ctx context.Context, userID int64) (*UserOrderStats, error) {
	out := &UserOrderStats{}

	var asUser orderStatsRow
	err := r.db.WithContext(ctx).Model(&model.Order{}).
		Select(`COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status IN ('pending_pay','in_service','pending_confirm','disputed') THEN 1 ELSE 0 END), 0) AS active,
			COALESCE(SUM(CASE WHEN paid_at IS NOT NULL THEN amount_cents ELSE 0 END), 0) AS paid_cents,
			0 AS income_cents`).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Scan(&asUser).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计用户订单失败: %w", err))
	}
	out.AsUserTotal = asUser.Total
	out.AsUserPaidCents = asUser.PaidCents
	out.ActiveCount = asUser.Active

	var asExpert orderStatsRow
	err = r.db.WithContext(ctx).Model(&model.Order{}).
		Select(`COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status IN ('pending_accept','pending_pay','in_service','pending_confirm','disputed') THEN 1 ELSE 0 END), 0) AS active,
			0 AS paid_cents,
			COALESCE(SUM(CASE WHEN status IN ('completed','settled') THEN expert_amount_cents ELSE 0 END), 0) AS income_cents`).
		Where("expert_id = ? AND deleted_at IS NULL", userID).
		Scan(&asExpert).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家订单失败: %w", err))
	}
	out.AsExpertTotal = asExpert.Total
	out.AsExpertIncomeCents = asExpert.IncomeCents

	// 在途订单以两种身份各算一次：同一笔单子他可能既要以用户身份确认、
	// 又要以专家身份交付，两边的待办都不该因为对方存在而消失。
	out.ActiveCount += asExpert.Active

	return out, nil
}

// CountActiveAdmins 统计仍可用（未删除且未封禁）的管理员数量。
//
// 后台改角色 / 封禁前的最后一道闸：把最后一个管理员降成普通用户，
// 从此没人能进后台，只能改数据库救回来。
func (r *UserAdminRepo) CountActiveAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("role = ? AND status = ?", model.RoleAdmin, model.UserStatusActive).
		Count(&n).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计管理员数量失败: %w", err))
	}
	return n, nil
}

// UpdateStatusIf 条件更新状态：只有当前状态仍是 from 时才写入。
//
// 用 CAS 而不是「查到就改」：两个管理员同时点封禁和解封，后写的会覆盖先写的，
// 最终状态取决于谁的手快，事后从审计日志里都看不出谁赢了。
// 拿不到行就让调用方报「状态已变化」，让人刷新后重看。
func (r *UserAdminRepo) UpdateStatusIf(ctx context.Context, id int64, from string, fields map[string]any) (bool, error) {
	if len(fields) == 0 {
		return false, nil
	}
	res := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND status = ? AND deleted_at IS NULL", id, from).
		Updates(fields)
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("更新用户状态失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// UpdateRoleIf 条件更新角色，语义同 UpdateStatusIf。
func (r *UserAdminRepo) UpdateRoleIf(ctx context.Context, id int64, from, to string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND role = ? AND deleted_at IS NULL", id, from).
		Update("role", to)
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("更新用户角色失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}
