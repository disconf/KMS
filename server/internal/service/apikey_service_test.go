package service

// 开放 API 密钥的口径测试：生成-鉴权-吊销-封禁联动。

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

func newApiKeyTestEnv(t *testing.T) (*ApiKeyService, *repository.UserRepo, context.Context) {
	t.Helper()
	cfg, err := config.Load("")
	require.NoError(t, err)
	cfg.Database.Driver = "sqlite"
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "apikey.db")
	cfg.Database.AutoMigrate = true
	cfg.Redis.Addr = ""

	db, err := bootstrap.OpenDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	users := repository.NewUserRepo(db)
	return NewApiKeyService(repository.NewApiKeyRepo(db), users), users, context.Background()
}

func TestApiKeyLifecycle(t *testing.T) {
	svc, users, ctx := newApiKeyTestEnv(t)

	u := &model.User{Phone: model.StrPtr("13900000201"), Nickname: "脚本用户", Role: model.RoleUser, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, u))

	// 创建：明文只出现一次，库里只存哈希。
	created, err := svc.Create(ctx, u.ID, "公司巡检脚本")
	require.NoError(t, err)
	assert.True(t, len(created.Key) >= 32, "明文密钥应有足够长度")
	assert.NotContains(t, created.Prefix, created.Key[len(created.Key)-8:], "前缀不能泄露密钥主体")

	// 鉴权成功。
	key, err := svc.Verify(ctx, created.Key)
	require.NoError(t, err)
	assert.Equal(t, created.ID, key.ID)

	// 篡改一个字符 → 无效。
	plain := []byte(created.Key)
	plain[len(plain)-1] ^= 0x01
	_, err = svc.Verify(ctx, string(plain))
	assert.ErrorIs(t, err, errcode.ErrAPIKeyInvalid)

	// 非法格式直接拒绝（无前缀 / 过短）。
	_, err = svc.Verify(ctx, "not-a-key")
	assert.ErrorIs(t, err, errcode.ErrAPIKeyInvalid)
	_, err = svc.Verify(ctx, "opsk_short")
	assert.ErrorIs(t, err, errcode.ErrAPIKeyInvalid)

	// 列表不含明文。
	keys, err := svc.List(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1)

	// 吊销后鉴权失败，且不能吊销别人的/不存在的密钥。
	require.NoError(t, svc.Revoke(ctx, u.ID, created.ID))
	_, err = svc.Verify(ctx, created.Key)
	assert.ErrorIs(t, err, errcode.ErrAPIKeyRevoked)
	err = svc.Revoke(ctx, u.ID, created.ID)
	assert.ErrorIs(t, err, errcode.ErrAPIKeyAbsent)

	// 账号封禁 → 密钥同步失效。
	created2, err := svc.Create(ctx, u.ID, "第二把")
	require.NoError(t, err)
	require.NoError(t, users.UpdateFields(ctx, u.ID, map[string]any{"status": model.UserStatusDisabled}))
	_, err = svc.Verify(ctx, created2.Key)
	assert.ErrorIs(t, err, errcode.ErrAPIKeyBanned)
}

func TestApiKeyLimit(t *testing.T) {
	svc, users, ctx := newApiKeyTestEnv(t)
	u := &model.User{Phone: model.StrPtr("13900000202"), Nickname: "囤钥户", Role: model.RoleUser, Status: model.UserStatusActive}
	require.NoError(t, users.Create(ctx, u))

	for i := 0; i < 10; i++ {
		_, err := svc.Create(ctx, u.ID, "密钥")
		require.NoError(t, err)
	}
	_, err := svc.Create(ctx, u.ID, "第十一把")
	assert.ErrorIs(t, err, errcode.ErrAPIKeyLimit)
}
