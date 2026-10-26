package service

import (
	"context"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

// 审计动作标识。加新动作时沿用 <对象>.<动词> 的形状，
// 后台按前缀筛选就不必维护一张枚举表。
const (
	AuditActionUserDisable = "user.disable"
	AuditActionUserEnable  = "user.enable"
	AuditActionUserRole    = "user.role"

	// 改动抽佣比例会直接影响每一笔新订单里专家拿多少钱，
	// 必须能回答「谁在什么时候把 15% 改成了 20%」。
	AuditActionSettingCommission = "setting.commission"
)

// 审计对象类型。
const (
	AuditTargetUser    = "user"
	AuditTargetSetting = "setting"
)

// AuditMeta 审计记录里的请求上下文。
//
// 由 handler 从 gin.Context 取出来，service 层不碰 HTTP 细节。
type AuditMeta struct {
	IP        string
	UserAgent string
	RequestID string
}

// appendAudit 写一条审计日志。
//
// 失败只记 warn、不返回错误：审计是事后追责的补充，业务动作本身才是止血。
// 为了让"记一笔"成功而回滚封禁或改配置，等于门没锁上还怪本子没墨。
//
// 提成独立函数而不是各 service 各写一遍：UserAgent 截断、空值处理这些细节
// 一旦分叉，排查时会发现同一个字段在两张表里长度不同。
func appendAudit(
	ctx context.Context, audits *repository.AuditRepo,
	operatorID int64, operatorRole, action, targetType string,
	targetID int64, detail map[string]any, meta AuditMeta,
) {
	entry := &model.AuditLog{
		UserID:     operatorID,
		UserRole:   operatorRole,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     model.JSONMap(detail),
		IP:         meta.IP,
		UserAgent:  truncateRunes(meta.UserAgent, 255),
		RequestID:  meta.RequestID,
	}
	if err := audits.Create(ctx, entry); err != nil {
		logger.Warn("写审计日志失败", "action", action, "target_type", targetType,
			"target_id", targetID, "err", err.Error())
	}
}
