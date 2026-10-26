// Package repository 封装数据访问，隔离 GORM 细节，只向上暴露领域语义。
package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// UserRepo 用户数据访问。
type UserRepo struct {
	db *gorm.DB
}

// NewUserRepo 创建 UserRepo。
func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// FindByID 按主键查询；不存在时返回 errcode.ErrAccountNotFound。
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrAccountNotFound)
	}
	return &u, nil
}

// FindByPhone 按手机号查询；不存在时返回 errcode.ErrAccountNotFound。
func (r *UserRepo) FindByPhone(ctx context.Context, phone string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&u).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrAccountNotFound)
	}
	return &u, nil
}

// FindByOpenID 按微信 openid 查询；不存在时返回 errcode.ErrAccountNotFound。
func (r *UserRepo) FindByOpenID(ctx context.Context, openID string) (*model.User, error) {
	if openID == "" {
		return nil, errcode.ErrAccountNotFound
	}
	var u model.User
	err := r.db.WithContext(ctx).Where("wx_open_id = ?", openID).First(&u).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrAccountNotFound)
	}
	return &u, nil
}

// accountRow 认证中间件一次查询所需的账号视图。
type accountRow struct {
	model.User

	ExpertStatus string `gorm:"column:expert_status"`
}

// FindAccount 返回账号本体与其专家审核状态，供认证中间件复核身份。
//
// 为什么连专家状态一起查：令牌里的 expertStatus 是签发那一刻的快照。
// 只信它就意味着「审核刚通过的专家必须重新登录才能进工作台」——
// 这是运营每天都会碰到的摩擦（用户点完"通过"，本人刷新页面还是进不去）。
// 一次 LEFT JOIN 换掉这一类问题，比让每个用户去调 refresh 划算得多。
func (r *UserRepo) FindAccount(ctx context.Context, id int64) (*model.User, string, error) {
	var row accountRow
	err := r.db.WithContext(ctx).
		Table("users").
		Select("users.*, COALESCE(experts.status, '') AS expert_status").
		Joins("LEFT JOIN experts ON experts.user_id = users.id AND experts.deleted_at IS NULL").
		Where("users.id = ? AND users.deleted_at IS NULL", id).
		Scan(&row).Error
	if err != nil {
		return nil, "", errcode.ErrDatabase.Wrap(fmt.Errorf("查询账号状态失败: %w", err))
	}
	if row.ID == 0 {
		return nil, "", errcode.ErrAccountNotFound
	}
	return &row.User, row.ExpertStatus, nil
}

// FindByIDs 批量按主键查询，返回 id → 用户 的映射。
// 查不到的 ID 不会出现在结果中（已注销或已删除），调用方需自行兜底。
func (r *UserRepo) FindByIDs(ctx context.Context, ids []int64) (map[int64]*model.User, error) {
	out := make(map[int64]*model.User)
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return out, nil
	}

	var users []model.User
	if err := r.db.WithContext(ctx).Where("id IN ?", unique).Find(&users).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("批量查询用户失败: %w", err))
	}
	for i := range users {
		out[users[i].ID] = &users[i]
	}
	return out, nil
}

// Create 创建用户。
func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建用户失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新，避免并发场景下整行覆盖导致的数据丢失。
func (r *UserRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新用户 %d 失败: %w", id, err))
	}
	return nil
}

// TouchLogin 记录最近登录信息。失败不影响登录主流程，因此只返回错误由调用方决定是否忽略。
func (r *UserRepo) TouchLogin(ctx context.Context, id int64, ip string) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{
		"last_login_at": gorm.Expr(nowExpr(r.db)),
		"last_login_ip": ip,
	}).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新登录时间失败: %w", err))
	}
	return nil
}

// BindWechat 绑定微信身份到已有账号。
func (r *UserRepo) BindWechat(ctx context.Context, id int64, openID, unionID string) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{
		"wx_open_id":  openID,
		"wx_union_id": unionID,
	}).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("绑定微信失败: %w", err))
	}
	return nil
}

// nowExpr 返回当前时间表达式，让登录时间由数据库生成，避免应用服务器时钟漂移。
// 采用参数占位写法可兼容 MySQL 与 SQLite。
func nowExpr(db *gorm.DB) string {
	if db.Dialector.Name() == "mysql" {
		return "NOW()"
	}
	return "datetime('now','localtime')"
}

// normalizeNotFound 把 GORM 的 ErrRecordNotFound 归一为业务错误。
func normalizeNotFound(err error, notFound *errcode.Error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return errcode.ErrDatabase.Wrap(err)
}
