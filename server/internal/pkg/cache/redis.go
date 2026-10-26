package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis 是基于 go-redis 的分布式缓存实现。
type Redis struct {
	client *redis.Client
}

// RedisOptions 是 Redis 连接参数。
type RedisOptions struct {
	Addr        string
	Password    string
	DB          int
	PoolSize    int
	DialTimeout time.Duration
}

// NewRedis 建立连接并做一次 Ping 探活。
func NewRedis(ctx context.Context, opt RedisOptions) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        opt.Addr,
		Password:    opt.Password,
		DB:          opt.DB,
		PoolSize:    opt.PoolSize,
		DialTimeout: opt.DialTimeout,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("连接 Redis(%s) 失败: %w", opt.Addr, err)
	}
	return &Redis{client: client}, nil
}

// Client 暴露底层客户端，便于后续使用 Set/Hash 等高级结构。
func (r *Redis) Client() *redis.Client { return r.client }

// Get 实现 Cache。
func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	v, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// Set 实现 Cache。
func (r *Redis) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Del 实现 Cache。
func (r *Redis) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

// Incr 实现 Cache。
func (r *Redis) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	if ttl > 0 {
		// 仅在首次创建时设置过期，避免每次自增都刷新窗口。
		pipe.ExpireNX(ctx, key, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// TTL 实现 Cache。
func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	switch d {
	case -2:
		return 0, ErrNotFound
	case -1:
		return 0, nil
	default:
		return d, nil
	}
}

// Name 实现 Cache。
func (r *Redis) Name() string { return "redis" }

// Close 实现 Cache。
func (r *Redis) Close() error { return r.client.Close() }
