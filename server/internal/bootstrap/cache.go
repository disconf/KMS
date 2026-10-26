package bootstrap

import (
	"context"
	"fmt"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/cache"
)

// OpenCache 按配置创建缓存：配置了 redis.addr 用 Redis，否则降级为进程内缓存。
func OpenCache(ctx context.Context, cfg *config.Config) (cache.Cache, error) {
	if cfg.MemCache() {
		return cache.NewMemory(), nil
	}
	c, err := cache.NewRedis(ctx, cache.RedisOptions{
		Addr:        cfg.Redis.Addr,
		Password:    cfg.Redis.Password,
		DB:          cfg.Redis.DB,
		PoolSize:    cfg.Redis.PoolSize,
		DialTimeout: cfg.Redis.DialTimeout.Std(),
	})
	if err != nil {
		return nil, fmt.Errorf("初始化缓存失败: %w", err)
	}
	return c, nil
}
