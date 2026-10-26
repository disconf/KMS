// Package jwtx 封装访问令牌与刷新令牌的签发、校验与解析。
package jwtx

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// TokenType 区分访问令牌与刷新令牌，避免刷新令牌被当作访问令牌使用。
type TokenType string

const (
	TypeAccess  TokenType = "access"
	TypeRefresh TokenType = "refresh"
)

// Claims 是平台自定义的 JWT 载荷。
type Claims struct {
	UserID       int64     `json:"uid"`
	Role         string    `json:"role"`
	ExpertStatus string    `json:"es,omitempty"`
	Type         TokenType `json:"typ"`
	jwt.RegisteredClaims
}

// Token 是签发结果。
type Token struct {
	Value     string    `json:"value"`
	JTI       string    `json:"jti"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Manager 负责令牌生命周期。
type Manager struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewManager 创建令牌管理器。
func NewManager(secret, issuer string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		secret:     []byte(secret),
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// AccessTTL 返回访问令牌有效期。
func (m *Manager) AccessTTL() time.Duration { return m.accessTTL }

// RefreshTTL 返回刷新令牌有效期。
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

// SignAccess 签发访问令牌。
func (m *Manager) SignAccess(userID int64, role, expertStatus string) (*Token, error) {
	return m.sign(userID, role, expertStatus, TypeAccess, m.accessTTL)
}

// SignRefresh 签发刷新令牌。
func (m *Manager) SignRefresh(userID int64, role, expertStatus string) (*Token, error) {
	return m.sign(userID, role, expertStatus, TypeRefresh, m.refreshTTL)
}

func (m *Manager) sign(userID int64, role, expertStatus string, typ TokenType, ttl time.Duration) (*Token, error) {
	now := time.Now()
	exp := now.Add(ttl)
	jti := uuid.NewString()

	claims := &Claims{
		UserID:       userID,
		Role:         role,
		ExpertStatus: expertStatus,
		Type:         typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   fmt.Sprintf("%d", userID),
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return nil, errcode.ErrInternal.Wrap(fmt.Errorf("签发令牌失败: %w", err))
	}
	return &Token{Value: signed, JTI: jti, ExpiresAt: exp}, nil
}

// Parse 校验并解析令牌，返回归一化后的业务错误。
func (m *Manager) Parse(raw string) (*Claims, error) {
	if raw == "" {
		return nil, errcode.ErrUnauthorized
	}

	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名算法: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errcode.ErrTokenExpired
		}
		return nil, errcode.ErrTokenInvalid.Wrap(err)
	}
	if claims.UserID <= 0 {
		return nil, errcode.ErrTokenInvalid.WithMsg("凭证缺少用户标识")
	}
	return claims, nil
}
