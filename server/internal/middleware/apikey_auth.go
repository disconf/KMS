// Package middleware — 开放 API 密钥鉴权与限流。
package middleware

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// ============================================================================
// API Key 限流器。
//
// 密钥是给脚本用的，脚本最大的风险是写成死循环把服务打挂。
// 每键一个滑动窗口计数，进程内存实现：多实例部署时各自为政，
// 限流变成「每实例 N/分钟」——对防滥用来说可接受，不值得为它引入 Redis 依赖。
// ============================================================================

type rateWindow struct {
	count int
	start time.Time
}

// APIKeyLimiter 每键滑动窗口限流。
type APIKeyLimiter struct {
	mu      sync.Mutex
	windows map[int64]*rateWindow
	limit   int
	per     time.Duration
}

// NewAPIKeyLimiter 创建限流器（limit 次 / per 窗口）。
func NewAPIKeyLimiter(limit int, per time.Duration) *APIKeyLimiter {
	return &APIKeyLimiter{
		windows: make(map[int64]*rateWindow),
		limit:   limit,
		per:     per,
	}
}

// Allow 判断某密钥本窗口是否还有配额。
func (l *APIKeyLimiter) Allow(keyID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w, ok := l.windows[keyID]
	if !ok || now.Sub(w.start) >= l.per {
		// 惰性清理：map 里攒的旧窗口在各自过期后被覆盖，
		// 密钥吊销后窗口留在 map 里也无害（几十字节）。
		l.windows[keyID] = &rateWindow{count: 1, start: now}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

// Remaining 返回本窗口剩余配额（响应头用）。
func (l *APIKeyLimiter) Remaining(keyID int64) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.windows[keyID]
	if !ok {
		return l.limit
	}
	if time.Since(w.start) >= l.per {
		return l.limit
	}
	return l.limit - w.count
}

// 限流默认值：60 次/分钟。密钥是给巡检脚本轮询用的，这个量级足够；
// 真有批量导出需求的用户应该走人，而不是把公开接口当爬虫靶子。
const (
	apiKeyRateLimit = 60
	apiKeyRatePer   = time.Minute
)

// APIKeyAuth 开放 API 鉴权中间件。
//
// 取钥顺序：Authorization: Bearer opsk_... → X-Api-Key 头。
// 鉴权失败一律 401/403，不在响应里区分「密钥不存在」与「密钥格式错」——
// 对脚本来说下一步都是去开发者页重新生成。
func APIKeyAuth(svc *service.ApiKeyService) gin.HandlerFunc {
	limiter := NewAPIKeyLimiter(apiKeyRateLimit, apiKeyRatePer)

	return func(c *gin.Context) {
		plain := extractAPIKey(c)
		if plain == "" {
			response.FailCode(c, errcode.ErrAPIKeyInvalid)
			c.Abort()
			return
		}

		key, err := svc.Verify(c.Request.Context(), plain)
		if err != nil {
			response.Fail(c, err)
			c.Abort()
			return
		}

		if !limiter.Allow(key.ID) {
			c.Header("X-RateLimit-Limit", "60")
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "60")
			response.FailCode(c, errcode.ErrTooManyReqs)
			c.Abort()
			return
		}

		c.Header("X-RateLimit-Limit", "60")
		c.Header("X-RateLimit-Remaining", strconv.Itoa(limiter.Remaining(key.ID)))

		// 记账采样：10% 的请求写一次统计，够用来判断"这把钥匙还活着吗"。
		if key.ID%10 == 0 || time.Now().UnixNano()%10 == 0 {
			go svc.Touch(c.Request.Context(), key.ID)
		}

		c.Set("apiKeyID", key.ID)
		c.Set("apiKeyUserID", key.UserID)
		c.Next()
	}
}

// extractAPIKey 从请求头提取明文密钥。
func extractAPIKey(c *gin.Context) string {
	if h := c.GetHeader("X-Api-Key"); h != "" {
		return h
	}
	auth := c.GetHeader("Authorization")
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}
