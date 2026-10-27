package service

import (
	"context"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

// UserAdminService 后台用户治理。
//
// 它只做"人能点的动作"，不碰认证：账号状态的最终效果由认证中间件按库判断，
// 所以这里改完库，对方下一次请求就会被拦——不需要吊销令牌、不需要他重新登录。
type UserAdminService struct {
	users  *repository.UserAdminRepo
	audits *repository.AuditRepo
}

// NewUserAdminService 创建 UserAdminService。
func NewUserAdminService(users *repository.UserAdminRepo, audits *repository.AuditRepo) *UserAdminService {
	return &UserAdminService{users: users, audits: audits}
}

// List 后台用户列表。
func (s *UserAdminService) List(ctx context.Context, q dto.AdminUserListQuery) ([]dto.AdminUserItem, int64, error) {
	rows, total, err := s.users.List(ctx, repository.UserAdminFilter{
		Keyword: strings.TrimSpace(q.Keyword),
		Role:    q.Role,
		Status:  q.Status,
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.AdminUserItem, 0, len(rows))
	for i := range rows {
		items = append(items, toAdminUserItem(&rows[i]))
	}
	return items, total, nil
}

// Detail 后台用户详情：账号、专家档案、订单聚合、以及这个人的操作留痕。
func (s *UserAdminService) Detail(ctx context.Context, id int64) (*dto.AdminUserDetail, error) {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	item := toAdminUserItem(&repository.UserAdminRow{User: *user})
	detail := &dto.AdminUserDetail{AdminUserItem: item}

	stats, err := s.users.OrderStats(ctx, id)
	if err != nil {
		// 订单统计查不出来不该让整个详情页打不开：账号本身的信息才是主内容，
		// 少两块数字比看不到这个人更好。记日志，前端会看到 0。
		logger.Error("查询用户订单统计失败", "user_id", id, "err", err.Error())
	} else {
		detail.OrderStats = dto.UserOrderStats{
			AsUserTotal:         stats.AsUserTotal,
			AsUserPaidCents:     stats.AsUserPaidCents,
			AsExpertTotal:       stats.AsExpertTotal,
			AsExpertIncomeCents: stats.AsExpertIncomeCents,
			ActiveCount:         stats.ActiveCount,
		}
	}

	expert, err := s.users.FindExpert(ctx, id)
	if err != nil {
		logger.Error("查询用户专家档案失败", "user_id", id, "err", err.Error())
	} else if expert != nil {
		detail.Expert = &dto.AdminUserExpert{
			ID:                expert.ID,
			RealName:          expert.RealName,
			Status:            expert.Status,
			Rating:            expert.Rating,
			OrderCount:        expert.OrderCount,
			BalanceCents:      expert.BalanceCents,
			FrozenCents:       expert.FrozenCents,
			TotalIncomeCents:  expert.TotalIncomeCents,
			AcceptingOrder:    expert.AcceptingOrder,
			PricePerHourCents: expert.PricePerHourCents,
			PricePerTaskCents: expert.PricePerTaskCents,
			AuditRemark:       expert.AuditRemark,
			AppliedAt:         expert.AppliedAt,
			AuditedAt:         expert.AuditedAt,
		}
	}

	logs, err := s.audits.ListByTarget(ctx, AuditTargetUser, id, 20)
	if err != nil {
		logger.Error("查询用户审计日志失败", "user_id", id, "err", err.Error())
	} else {
		detail.AuditLogs = make([]dto.AdminAuditItem, 0, len(logs))
		for i := range logs {
			row := logs[i]
			detail.AuditLogs = append(detail.AuditLogs, dto.AdminAuditItem{
				ID:        row.ID,
				Action:    row.Action,
				UserRole:  row.UserRole,
				Detail:    map[string]any(row.Detail),
				IP:        row.IP,
				CreatedAt: row.CreatedAt,
			})
		}
	}

	return detail, nil
}

// UpdateStatus 封禁 / 解封账号。
//
// 三道闸按顺序：
//  1. 不能操作自己——管理员把自己封了，只能改数据库救回来；
//  2. 不能封掉最后一个可用管理员——否则后台再无入口；
//  3. 封禁必须写原因——半年后有人申诉，光看到 disabled 说不出所以然。
func (s *UserAdminService) UpdateStatus(
	ctx context.Context, operatorID int64, operatorRole string,
	targetID int64, req dto.AdminUpdateStatusReq, meta AuditMeta,
) error {
	if targetID == operatorID {
		return errcode.ErrUserSelfOperate.WithMsg("不能封禁或解封自己的账号")
	}

	target, err := s.users.FindByID(ctx, targetID)
	if err != nil {
		return err
	}

	// 已经是目标状态就直接成功：运营连点两下封禁、或者两个人先后点，
	// 报一个"状态已变化"只会让人以为没生效，然后再点一次。
	if target.Status == req.Status {
		return nil
	}

	reason := strings.TrimSpace(req.Reason)
	if req.Status == model.UserStatusDisabled {
		if reason == "" {
			return errcode.ErrStatusReasonReq
		}
		if runeLen(reason) > 255 {
			return errcode.ErrParamInvalid.WithMsg("封禁原因过长")
		}
		if err := s.guardLastAdmin(ctx, target, "封禁"); err != nil {
			return err
		}
	} else {
		// 解封时把原因清掉：它是「当前状态的原因」，不是历史档案，
		// 历史在审计日志里，一直挂在列表上会让人误以为还在封着。
		reason = ""
	}

	now := time.Now()
	ok, err := s.users.UpdateStatusIf(ctx, targetID, target.Status, map[string]any{
		"status":            req.Status,
		"status_reason":     reason,
		"status_changed_at": now,
	})
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrUserStateChanged
	}

	action := AuditActionUserEnable
	if req.Status == model.UserStatusDisabled {
		action = AuditActionUserDisable
	}
	s.appendAudit(ctx, operatorID, operatorRole, action, targetID, map[string]any{
		"from":   target.Status,
		"to":     req.Status,
		"reason": reason,
	}, meta)
	return nil
}

// UpdateRole 修改角色。
//
// 改完立即生效（认证中间件以库里的 role 为准），所以不需要让用户重新登录。
func (s *UserAdminService) UpdateRole(
	ctx context.Context, operatorID int64, operatorRole string,
	targetID int64, req dto.AdminUpdateRoleReq, meta AuditMeta,
) error {
	if targetID == operatorID {
		return errcode.ErrUserSelfOperate.WithMsg("不能修改自己的角色")
	}
	if !model.ValidUserRole(req.Role) {
		return errcode.ErrParamInvalid.WithMsg("角色不合法")
	}

	target, err := s.users.FindByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.Role == req.Role {
		return nil
	}

	// 升成专家必须已有审核通过的档案，否则等于手工绕过入驻审核。
	if req.Role == model.RoleExpert {
		expert, err := s.users.FindExpert(ctx, targetID)
		if err != nil {
			return err
		}
		if expert == nil || !expert.IsApproved() {
			return errcode.ErrRoleNeedsExpert
		}
	}

	// 从管理员降级时同样要留住最后一个可用管理员。
	if target.Role == model.RoleAdmin {
		if err := s.guardLastAdmin(ctx, target, "降级"); err != nil {
			return err
		}
	}

	ok, err := s.users.UpdateRoleIf(ctx, targetID, target.Role, req.Role)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrUserStateChanged
	}

	s.appendAudit(ctx, operatorID, operatorRole, AuditActionUserRole, targetID, map[string]any{
		"from": target.Role,
		"to":   req.Role,
	}, meta)
	return nil
}

// guardLastAdmin 阻止把最后一个可用管理员弄没。
//
// 只有目标是「当前可用管理员」时才需要数一遍：一个已经被封的、或者本来就是普通用户的，
// 再怎么改都不会影响后台入口。
//
// 并发的窄窗口（两个管理员同时把对方封掉）这里挡不住——真要做到，得在一个事务里
// 锁 users 表。对内部后台来说，加这一层判断已经能挡住 99% 的误操作，
// 剩下的交给「另一个管理员用命令行改回来」这条运维预案。
func (s *UserAdminService) guardLastAdmin(ctx context.Context, target *model.User, verb string) error {
	if target.Role != model.RoleAdmin || !target.IsActive() {
		return nil
	}
	n, err := s.users.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errcode.ErrLastAdmin.WithMsg("这是最后一个可用管理员，不能" + verb)
	}
	return nil
}

// appendAudit 调用共享审计函数（audit.go）。
func (s *UserAdminService) appendAudit(
	ctx context.Context, operatorID int64, operatorRole, action string,
	targetID int64, detail map[string]any, meta AuditMeta,
) {
	appendAudit(ctx, s.audits, operatorID, operatorRole, action,
		AuditTargetUser, targetID, detail, meta)
}

// toAdminUserItem 把仓储行转成列表项。
func toAdminUserItem(row *repository.UserAdminRow) dto.AdminUserItem {
	return dto.AdminUserItem{
		ID:              row.ID,
		Nickname:        row.Nickname,
		Avatar:          row.Avatar,
		MaskedPhone:     row.MaskedPhone(),
		Email:           row.EmailValue(),
		SpeakStatus:     speakStatusOrDefault(row.SpeakStatus),
		Role:            row.Role,
		Status:          row.Status,
		StatusReason:    row.StatusReason,
		StatusChangedAt: row.StatusChangedAt,
		ExpertID:        row.ExpertID,
		ExpertStatus:    row.ExpertStatus,
		CreatedAt:       row.CreatedAt,
		LastLoginAt:     row.LastLoginAt,
	}
}

// truncateRunes 见 order_service.go：按字符截断，避免把多字节字符切成乱码。
