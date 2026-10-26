package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/config"
)

// HealthHandler 提供存活与就绪探针。
// 两者必须分开：存活探针失败会导致容器被重启，绝不能让数据库抖动触发重启风暴。
type HealthHandler struct {
	db  *gorm.DB
	cfg *config.Config
}

// NewHealthHandler 创建 HealthHandler。
func NewHealthHandler(db *gorm.DB, cfg *config.Config) *HealthHandler {
	return &HealthHandler{db: db, cfg: cfg}
}

// Live 存活探针：只要进程能响应就算健康，不检查依赖。
//
//	@Summary  存活探针
//	@Tags     health
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /healthz [get]
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"env":    h.cfg.App.Env,
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Ready 就绪探针：检查数据库连通性，失败返回 503 让流量不进来。
//
//	@Summary  就绪探针
//	@Tags     health
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Failure  503 {object} response.Body
//	@Router   /readyz [get]
func (h *HealthHandler) Ready(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "reason": "获取连接池失败"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		slog.Warn("就绪探针失败，数据库不可达", slog.String("err", err.Error()))
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "reason": "数据库不可达"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"database": "up",
		"time":     time.Now().Format(time.RFC3339),
	})
}
