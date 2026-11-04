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

// ApiKeyRepo 开放 API 密钥仓储。
type ApiKeyRepo struct{ db *gorm.DB }

// NewApiKeyRepo 创建仓储。
func NewApiKeyRepo(db *gorm.DB) *ApiKeyRepo { return &ApiKeyRepo{db: db} }

// Create 建密钥。
func (r *ApiKeyRepo) Create(ctx context.Context, key *model.ApiKey) error {
	if err := r.db.WithContext(ctx).Create(key).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建 API 密钥失败: %w", err))
	}
	return nil
}

// CountActiveOf 用户的有效密钥数（上限校验）。
func (r *ApiKeyRepo) CountActiveOf(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ApiKey{}).
		Where("user_id = ? AND status = ?", userID, model.ApiKeyStatusActive).Count(&n).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计密钥失败: %w", err))
	}
	return n, nil
}

// ListOf 用户的密钥列表（不含哈希）。
func (r *ApiKeyRepo) ListOf(ctx context.Context, userID int64) ([]model.ApiKey, error) {
	items := make([]model.ApiKey, 0, 8)
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("id DESC").Find(&items).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询密钥失败: %w", err))
	}
	return items, nil
}

// ByHash 按密钥哈希找密钥（鉴权路径，唯一索引直达）。
func (r *ApiKeyRepo) ByHash(ctx context.Context, hash string) (*model.ApiKey, error) {
	var key model.ApiKey
	err := r.db.WithContext(ctx).First(&key, "key_hash = ?", hash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrAPIKeyInvalid
		}
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询密钥失败: %w", err))
	}
	return &key, nil
}

// Revoke 吊销（CAS：active → revoked）。返回 false 表示已经吊销过。
func (r *ApiKeyRepo) Revoke(ctx context.Context, userID, keyID int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.ApiKey{}).
		Where("id = ? AND user_id = ? AND status = ?", keyID, userID, model.ApiKeyStatusActive).
		Updates(map[string]any{
			"status":     model.ApiKeyStatusRevoked,
			"revoked_at": time.Now(),
		})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("吊销密钥失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// TouchUsage 记录一次调用（节流写入：每次调用都 UPDATE 太奢侈，
// 由中间件按概率采样，统计值不需要精确到个位）。
func (r *ApiKeyRepo) TouchUsage(ctx context.Context, id int64) {
	now := time.Now()
	r.db.WithContext(ctx).Model(&model.ApiKey{}).Where("id = ?", id).
		Updates(map[string]any{
			"request_count": gorm.Expr("request_count + 1"),
			"last_used_at":  now,
		})
}
