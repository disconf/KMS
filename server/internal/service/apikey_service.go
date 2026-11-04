// Package service — 开放 API 密钥。
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

// apiKeysPerUser 每人有效密钥上限。
const apiKeysPerUser = 10

// apiKeyPrefix 密钥明文前缀。带可识别前缀有两个好处：
// 泄露时能在日志/代码里被 grep 出来；扫描器拿到一串 opsk_ 开头的串也知道这是什么。
const apiKeyPrefix = "opsk_"

// ApiKeyService 开放 API 密钥管理。
type ApiKeyService struct {
	repo  *repository.ApiKeyRepo
	users *repository.UserRepo
}

// NewApiKeyService 创建服务。
func NewApiKeyService(repo *repository.ApiKeyRepo, users *repository.UserRepo) *ApiKeyService {
	return &ApiKeyService{repo: repo, users: users}
}

// generateAPIKey 生成明文密钥：opsk_ + 32 位 hex（128 位熵）。
func generateAPIKey() (plain, prefix string, hash string, err error) {
	buf := make([]byte, 16)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("生成随机密钥失败: %w", err)
	}
	body := hex.EncodeToString(buf)
	plain = apiKeyPrefix + body
	prefix = apiKeyPrefix + body[:8]
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return plain, prefix, hash, nil
}

// HashAPIKey 对外暴露的明文 → 哈希（中间件鉴权用）。
func HashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// Create 新建密钥，明文只在这一次响应里返回。
func (s *ApiKeyService) Create(ctx context.Context, userID int64, name string) (*dto.APIKeyCreated, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "未命名密钥"
	}
	if len(name) > 60 {
		name = name[:60]
	}
	n, err := s.repo.CountActiveOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n >= apiKeysPerUser {
		return nil, errcode.ErrAPIKeyLimit
	}

	plain, prefix, hash, err := generateAPIKey()
	if err != nil {
		return nil, errcode.ErrInternal.Wrap(err)
	}
	key := model.ApiKey{
		UserID:  userID,
		Name:    name,
		Prefix:  prefix,
		KeyHash: hash,
		Status:  model.ApiKeyStatusActive,
	}
	if err := s.repo.Create(ctx, &key); err != nil {
		return nil, err
	}
	return &dto.APIKeyCreated{
		ID:     key.ID,
		Name:   key.Name,
		Prefix: key.Prefix,
		// 明文只此一次：数据库只存哈希，丢了就只能重新生成。
		Key:       plain,
		CreatedAt: key.CreatedAt,
	}, nil
}

// List 我的密钥列表。
func (s *ApiKeyService) List(ctx context.Context, userID int64) ([]dto.APIKeyItem, error) {
	keys, err := s.repo.ListOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.APIKeyItem, 0, len(keys))
	for _, k := range keys {
		out = append(out, dto.APIKeyItem{
			ID:           k.ID,
			Name:         k.Name,
			Prefix:       k.Prefix,
			Status:       k.Status,
			StatusLabel:  apiKeyStatusLabel(k.Status),
			LastUsedAt:   k.LastUsedAt,
			RequestCount: k.RequestCount,
			CreatedAt:    k.CreatedAt,
			RevokedAt:    k.RevokedAt,
		})
	}
	return out, nil
}

// Revoke 吊销自己的密钥。
func (s *ApiKeyService) Revoke(ctx context.Context, userID, keyID int64) error {
	ok, err := s.repo.Revoke(ctx, userID, keyID)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrAPIKeyAbsent
	}
	return nil
}

// Verify 鉴权：明文 → 密钥 → 校验状态与账号状态。
//
// 返回密钥供中间件限流与记账。账号被封禁时密钥同步失效——
// 封禁是即时的，不能让人被封了还能带着密钥继续读站内内容。
func (s *ApiKeyService) Verify(ctx context.Context, plainKey string) (*model.ApiKey, error) {
	plainKey = strings.TrimSpace(plainKey)
	if !strings.HasPrefix(plainKey, apiKeyPrefix) || len(plainKey) < len(apiKeyPrefix)+16 {
		return nil, errcode.ErrAPIKeyInvalid
	}
	// 常量时间比较防时序侧信道（哈希比对理论上可撞时序，代价极低但习惯要养成）。
	hash := HashAPIKey(plainKey)
	key, err := s.repo.ByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(key.KeyHash), []byte(hash)) != 1 {
		return nil, errcode.ErrAPIKeyInvalid
	}
	if key.Status != model.ApiKeyStatusActive {
		return nil, errcode.ErrAPIKeyRevoked
	}
	user, err := s.users.FindByID(ctx, key.UserID)
	if err != nil || user == nil || user.Status != model.UserStatusActive {
		return nil, errcode.ErrAPIKeyBanned
	}
	return key, nil
}

// Touch 记一次使用（采样调用，见仓储注释）。
func (s *ApiKeyService) Touch(ctx context.Context, keyID int64) {
	s.repo.TouchUsage(ctx, keyID)
}

func apiKeyStatusLabel(status string) string {
	if status == model.ApiKeyStatusRevoked {
		return "已吊销"
	}
	return "有效"
}
