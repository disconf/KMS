package jwtx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

const testSecret = "unit-test-secret-0123456789abcdefghijklmnop"

func newTestManager(accessTTL time.Duration) *Manager {
	return NewManager(testSecret, "test-issuer", accessTTL, 24*time.Hour)
}

func TestSignAndParseAccess(t *testing.T) {
	m := newTestManager(time.Hour)

	token, err := m.SignAccess(42, "expert", "approved")
	require.NoError(t, err)
	assert.NotEmpty(t, token.Value)
	assert.NotEmpty(t, token.JTI)
	assert.WithinDuration(t, time.Now().Add(time.Hour), token.ExpiresAt, 5*time.Second)

	claims, err := m.Parse(token.Value)
	require.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserID)
	assert.Equal(t, "expert", claims.Role)
	assert.Equal(t, "approved", claims.ExpertStatus)
	assert.Equal(t, TypeAccess, claims.Type)
	assert.Equal(t, token.JTI, claims.ID)
	assert.Equal(t, "test-issuer", claims.Issuer)
}

func TestParseRejectsExpiredToken(t *testing.T) {
	// 负数 TTL 直接签出一个已过期的令牌，用真实时间轴验证过期分支。
	m := newTestManager(-time.Minute)

	token, err := m.SignAccess(1, "user", "")
	require.NoError(t, err)

	_, err = m.Parse(token.Value)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTokenExpired)
}

func TestParseRejectsWrongSecret(t *testing.T) {
	m := newTestManager(time.Hour)
	token, err := m.SignAccess(1, "user", "")
	require.NoError(t, err)

	other := NewManager("another-secret-0123456789abcdefghijklmn", "test-issuer", time.Hour, 24*time.Hour)
	_, err = other.Parse(token.Value)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTokenInvalid)
}

func TestParseRejectsWrongIssuer(t *testing.T) {
	m := newTestManager(time.Hour)
	token, err := m.SignAccess(1, "user", "")
	require.NoError(t, err)

	other := NewManager(testSecret, "other-issuer", time.Hour, 24*time.Hour)
	_, err = other.Parse(token.Value)
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrTokenInvalid)
}

func TestParseRejectsEmptyToken(t *testing.T) {
	m := newTestManager(time.Hour)
	_, err := m.Parse("")
	require.Error(t, err)
	assert.ErrorIs(t, err, errcode.ErrUnauthorized)
}

func TestRefreshTokenTypeDiffers(t *testing.T) {
	m := newTestManager(time.Hour)

	access, err := m.SignAccess(7, "user", "")
	require.NoError(t, err)
	refresh, err := m.SignRefresh(7, "user", "")
	require.NoError(t, err)

	accessClaims, err := m.Parse(access.Value)
	require.NoError(t, err)
	refreshClaims, err := m.Parse(refresh.Value)
	require.NoError(t, err)

	assert.Equal(t, TypeAccess, accessClaims.Type)
	assert.Equal(t, TypeRefresh, refreshClaims.Type)
	// 两种令牌必须可区分，否则刷新令牌就能当访问令牌用。
	assert.NotEqual(t, accessClaims.Type, refreshClaims.Type)
}

func TestTTLAccessors(t *testing.T) {
	m := NewManager(testSecret, "test-issuer", 2*time.Hour, 168*time.Hour)
	assert.Equal(t, 2*time.Hour, m.AccessTTL())
	assert.Equal(t, 168*time.Hour, m.RefreshTTL())
}
