package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemorySetGetDel(t *testing.T) {
	c := NewMemory()
	defer func() { require.NoError(t, c.Close()) }()

	ctx := context.Background()

	_, err := c.Get(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, c.Set(ctx, "k", "v", time.Minute))
	got, err := c.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)

	require.NoError(t, c.Del(ctx, "k"))
	_, err = c.Get(ctx, "k")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryExpiry(t *testing.T) {
	c := NewMemory()
	defer func() { require.NoError(t, c.Close()) }()

	ctx := context.Background()

	// TTL 设为极短，验证过期后读不到。
	require.NoError(t, c.Set(ctx, "temp", "v", time.Millisecond))
	time.Sleep(10 * time.Millisecond)

	_, err := c.Get(ctx, "temp")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryIncrStartsAtOneAndKeepsTTL(t *testing.T) {
	c := NewMemory()
	defer func() { require.NoError(t, c.Close()) }()

	ctx := context.Background()

	n, err := c.Incr(ctx, "counter", time.Minute)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	n, err = c.Incr(ctx, "counter", time.Minute)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	n, err = c.Incr(ctx, "counter", time.Minute)
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)

	ttl, err := c.TTL(ctx, "counter")
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, time.Minute)
}

func TestMemoryIncrResetsAfterExpiry(t *testing.T) {
	c := NewMemory()
	defer func() { require.NoError(t, c.Close()) }()

	ctx := context.Background()

	_, err := c.Incr(ctx, "win", time.Millisecond)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)

	// 过期后应从 1 重新计数，这是限流窗口的核心语义。
	n, err := c.Incr(ctx, "win", time.Minute)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}

func TestMemoryTTLNoExpiry(t *testing.T) {
	c := NewMemory()
	defer func() { require.NoError(t, c.Close()) }()

	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "forever", "v", 0))

	ttl, err := c.TTL(ctx, "forever")
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), ttl)

	_, err = c.TTL(ctx, "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemoryCloseIsIdempotent(t *testing.T) {
	c := NewMemory()
	require.NoError(t, c.Close())
	require.NoError(t, c.Close())
	assert.Equal(t, "memory", c.Name())
}
