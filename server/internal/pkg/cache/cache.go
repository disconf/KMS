// Package cache 定义平台统一的键值缓存抽象。
//
// 之所以抽象：本地开发不希望被 Redis 卡住，生产又必须用 Redis。
// 通过 Cache 接口，业务代码只依赖抽象，配置里把 redis.addr 留空即自动降级为进程内缓存。
//
// 注意：内存实现仅适用于单机开发/测试，多实例部署必须配置 Redis。
package cache

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNotFound 表示键不存在或已过期。
var ErrNotFound = errors.New("cache: key not found")

// Cache 是缓存统一接口。
type Cache interface {
	// Get 读取键值，不存在时返回 ErrNotFound。
	Get(ctx context.Context, key string) (string, error)
	// Set 写入键值并设置过期时间，ttl <= 0 表示永不过期。
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// Del 删除一个或多个键。
	Del(ctx context.Context, keys ...string) error
	// Incr 原子自增，键不存在时从 1 开始并应用 ttl。
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	// TTL 返回剩余有效期，永不过期返回 0，不存在返回 ErrNotFound。
	TTL(ctx context.Context, key string) (time.Duration, error)
	// Name 返回实现名称，用于启动日志。
	Name() string
	// Close 释放资源。
	Close() error
}

type memItem struct {
	value     string
	expiresAt time.Time // 零值表示永不过期
}

func (i memItem) expired(now time.Time) bool {
	return !i.expiresAt.IsZero() && !now.Before(i.expiresAt)
}

// Memory 是进程内缓存实现，带惰性过期与后台清理协程。
type Memory struct {
	mu     sync.RWMutex
	items  map[string]memItem
	stop   chan struct{}
	closed bool
}

// NewMemory 创建内存缓存并启动清理协程。
func NewMemory() *Memory {
	m := &Memory{
		items: make(map[string]memItem),
		stop:  make(chan struct{}),
	}
	go m.janitor()
	return m
}

const janitorInterval = time.Minute

func (m *Memory) janitor() {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.mu.Lock()
			for k, v := range m.items {
				if v.expired(now) {
					delete(m.items, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

// Get 实现 Cache。
func (m *Memory) Get(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	m.mu.RUnlock()
	if !ok || item.expired(time.Now()) {
		return "", ErrNotFound
	}
	return item.value, nil
}

// Set 实现 Cache。
func (m *Memory) Set(_ context.Context, key, value string, ttl time.Duration) error {
	item := memItem{value: value}
	if ttl > 0 {
		item.expiresAt = time.Now().Add(ttl)
	}
	m.mu.Lock()
	m.items[key] = item
	m.mu.Unlock()
	return nil
}

// Del 实现 Cache。
func (m *Memory) Del(_ context.Context, keys ...string) error {
	m.mu.Lock()
	for _, k := range keys {
		delete(m.items, k)
	}
	m.mu.Unlock()
	return nil
}

// Incr 实现 Cache。
func (m *Memory) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[key]
	if !ok || item.expired(now) {
		item = memItem{value: "1"}
		if ttl > 0 {
			item.expiresAt = now.Add(ttl)
		}
		m.items[key] = item
		return 1, nil
	}

	n := parseCounter(item.value) + 1
	item.value = formatCounter(n)
	m.items[key] = item
	return n, nil
}

// TTL 实现 Cache。
func (m *Memory) TTL(_ context.Context, key string) (time.Duration, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	m.mu.RUnlock()
	if !ok {
		return 0, ErrNotFound
	}
	if item.expiresAt.IsZero() {
		return 0, nil
	}
	remain := time.Until(item.expiresAt)
	if remain <= 0 {
		return 0, ErrNotFound
	}
	return remain, nil
}

// Name 实现 Cache。
func (m *Memory) Name() string { return "memory" }

// Close 实现 Cache，幂等。
func (m *Memory) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()

	close(m.stop)
	return nil
}
