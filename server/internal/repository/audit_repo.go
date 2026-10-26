package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// AuditRepo 操作审计日志。
//
// 这张表从第一版就建好了，但一直没人写——等于「我们打算留痕」只是句口号。
// 后台封禁账号是第一个接入的动作：封禁必须能回答「谁在什么时候、以什么理由封的」，
// 否则被误封的人申诉时，客服只能看到 status=disabled 三个字。
type AuditRepo struct {
	db *gorm.DB
}

// NewAuditRepo 创建 AuditRepo。
func NewAuditRepo(db *gorm.DB) *AuditRepo { return &AuditRepo{db: db} }

// Create 写入一条审计记录。
//
// 失败只返回错误、由调用方决定是否中断：审计是"事后追责"的补充，
// 而封禁本身是"立刻止血"。为了让审计写成功而回滚封禁，等于门锁不上还怪本子没墨。
// 所以用户侧的动作一律先落库成功，审计失败只记日志。
func (r *AuditRepo) Create(ctx context.Context, log *model.AuditLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("写入审计日志失败: %w", err))
	}
	return nil
}

// ListByTarget 取某个对象上的审计记录，新的在前。
func (r *AuditRepo) ListByTarget(ctx context.Context, targetType string, targetID int64, limit int) ([]model.AuditLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows := make([]model.AuditLog, 0, limit)
	err := r.db.WithContext(ctx).
		Where("target_type = ? AND target_id = ?", targetType, targetID).
		Order("id DESC").Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询审计日志失败: %w", err))
	}
	return rows, nil
}
