// Package tokenstore 负责令牌的服务端状态：刷新令牌白名单与访问令牌吊销名单。
//
// MVP 阶段不自建会话表，直接用缓存承载：
//   - 刷新令牌：login 时写入，logout / 刷新时删除，实现"服务端可控登出"
//   - 访问令牌：logout 时写入吊销标记，TTL 取令牌剩余有效期，到期自动清理
package tokenstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/opsexpert/platform/internal/pkg/cache"
)

const (
	prefixRefresh = "auth:refresh:"
	prefixRevoked = "auth:revoked:"
)

// RefreshKey 返回刷新令牌的缓存键。
func RefreshKey(jti string) string { return prefixRefresh + jti }

// RevokedKey 返回访问令牌吊销标记的缓存键。
func RevokedKey(jti string) string { return prefixRevoked + jti }

// Store 封装令牌状态读写。
type Store struct {
	cache cache.Cache
}

// New 创建 Store。
func New(c cache.Cache) *Store { return &Store{cache: c} }

// SaveRefresh 记录一个有效的刷新令牌；userID 一并存入，便于校验令牌与用户是否匹配。
func (s *Store) SaveRefresh(ctx context.Context, jti string, userID int64, ttl time.Duration) error {
	if jti == "" {
		return errors.New("tokenstore: 刷新令牌 jti 为空")
	}
	return s.cache.Set(ctx, RefreshKey(jti), fmt.Sprintf("%d", userID), ttl)
}

// RefreshOwner 返回刷新令牌对应的用户 ID；不存在或已失效时报错。
func (s *Store) RefreshOwner(ctx context.Context, jti string) (int64, error) {
	v, err := s.cache.Get(ctx, RefreshKey(jti))
	if err != nil {
		return 0, err
	}
	var uid int64
	if _, err := fmt.Sscanf(v, "%d", &uid); err != nil {
		return 0, fmt.Errorf("tokenstore: 刷新令牌记录损坏: %w", err)
	}
	return uid, nil
}

// DeleteRefresh 删除刷新令牌（登出 / 轮换时调用）。
func (s *Store) DeleteRefresh(ctx context.Context, jti string) error {
	return s.cache.Del(ctx, RefreshKey(jti))
}

// Revoke 吊销一个访问令牌，ttl 取该令牌的剩余有效期。
func (s *Store) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" {
		return nil
	}
	if ttl <= 0 {
		return nil // 已过期的令牌无需吊销
	}
	return s.cache.Set(ctx, RevokedKey(jti), "1", ttl)
}

// IsRevoked 判断访问令牌是否已被吊销。缓存故障时按"未吊销"处理，
// 避免因缓存不可用导致全站无法登录（可用性优先于极端场景的严格性）。
func (s *Store) IsRevoked(ctx context.Context, jti string) bool {
	if jti == "" {
		return false
	}
	_, err := s.cache.Get(ctx, RevokedKey(jti))
	switch {
	case err == nil:
		return true
	case errors.Is(err, cache.ErrNotFound):
		return false
	default:
		// 缓存不可用时放行：可用性优先。若业务上更看重严格性，改为 return true 即可。
		return false
	}
}
