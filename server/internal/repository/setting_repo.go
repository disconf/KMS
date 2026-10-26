package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// SettingRepo 系统配置键值存储。
//
// 一行一个配置项，key 是主键。写入用 Upsert：存在则更新、不存在则插入，
// 两个方言（MySQL 的 ON DUPLICATE KEY / SQLite 的 ON CONFLICT DO UPDATE）
// 都由 GORM 的 clause.OnConflict 翻译，不需要在业务层写 if driver == mysql。
type SettingRepo struct {
	db *gorm.DB
}

// NewSettingRepo 创建 SettingRepo。
func NewSettingRepo(db *gorm.DB) *SettingRepo { return &SettingRepo{db: db} }

// Get 按 key 查询；不存在时返回 errcode.ErrNotFound（调用方通常用它触发回落默认值）。
func (r *SettingRepo) Get(ctx context.Context, key string) (*model.SystemSetting, error) {
	var s model.SystemSetting
	if err := r.db.WithContext(ctx).Where("`key` = ?", key).First(&s).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrNotFound)
	}
	return &s, nil
}

// Upsert 插入或更新一条配置。
func (r *SettingRepo) Upsert(ctx context.Context, s *model.SystemSetting) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "remark", "updated_by", "updated_at"}),
		}).
		Create(s).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("保存系统配置 %s 失败: %w", s.Key, err))
	}
	return nil
}
