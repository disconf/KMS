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

// SpeakService 处理"只读账号 → 可发言"的实名申请与审核。
//
// 为什么单开一个 service 而不是塞进 AuthService：
// AuthService 管的是"你是谁"（认证），这里管的是"你能做什么"（授权变更），
// 两者的调用者、失败后果都不同。混在一起以后加别的权限维度会更乱。
type SpeakService struct {
	users  *repository.UserRepo
	audits *repository.AuditRepo
}

// NewSpeakService 创建 SpeakService。
func NewSpeakService(users *repository.UserRepo, audits *repository.AuditRepo) *SpeakService {
	return &SpeakService{users: users, audits: audits}
}

// Apply 提交实名发言申请。
//
// 三条前置：已经能发言的不用申请；待审的不能重复提交（否则后台队列会被刷）；
// 被驳回的可以重新提交——那是产品必须给的路，驳回理由可能是"联系方式填错了"。
func (s *SpeakService) Apply(ctx context.Context, userID int64, req dto.SpeakApplyReq) (*dto.SpeakApplyState, error) {
	realName := strings.TrimSpace(req.RealName)
	if n := len([]rune(realName)); n < 2 || n > 32 {
		return nil, errcode.ErrRealNameInvalid.WithMsg("请填写 2-32 个字符的真实姓名")
	}
	contact := strings.TrimSpace(req.Contact)
	if len(contact) < 5 || len(contact) > 64 {
		return nil, errcode.ErrParamInvalid.WithMsg("请填写有效的联系方式")
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > 255 {
		return nil, errcode.ErrSpeakApplyNote.WithMsg("申请理由不能超过 255 个字")
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.CanSpeak() {
		return nil, errcode.ErrSpeakApplyDup.WithMsg("当前账号已有发言权限")
	}
	if user.SpeakStatus == model.SpeakPending {
		return nil, errcode.ErrSpeakApplyDup
	}

	now := time.Now()
	affected, err := s.users.UpdateSpeakApply(ctx, userID, user.SpeakStatus, map[string]any{
		"speak_status":      model.SpeakPending,
		"speak_real_name":   realName,
		"speak_contact":     contact,
		"speak_apply_note":  note,
		"speak_applied_at":  now,
		"speak_reviewed_at": nil,
		"speak_review_note": "",
	})
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		// 读到提交之间状态被改了（比如同时点了两次提交）。
		return nil, errcode.ErrSpeakApplyDup.WithMsg("申请状态已变化，请刷新后重试")
	}

	logger.Info("收到发言申请", "user_id", userID)
	return s.State(ctx, userID)
}

// State 查询当前账号的发言申请状态。
func (s *SpeakService) State(ctx context.Context, userID int64) (*dto.SpeakApplyState, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &dto.SpeakApplyState{
		Status:     speakStatusOrDefault(user.SpeakStatus),
		CanSpeak:   user.CanSpeak(),
		AppliedAt:  user.SpeakAppliedAt,
		ReviewedAt: user.SpeakReviewedAt,
		ReviewNote: user.SpeakReviewNote,
	}, nil
}

// ListApplications 后台分页查询申请。
func (s *SpeakService) ListApplications(ctx context.Context, status string, page, pageSize int) ([]dto.AdminSpeakApplyItem, int64, error) {
	if status != "" && status != "all" && !model.ValidSpeakStatus(status) {
		return nil, 0, errcode.ErrParamInvalid.WithMsg("状态取值非法")
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	rows, total, err := s.users.ListSpeakApplications(ctx, status, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.AdminSpeakApplyItem, 0, len(rows))
	for i := range rows {
		u := &rows[i]
		items = append(items, dto.AdminSpeakApplyItem{
			UserID:    u.ID,
			Nickname:  u.Nickname,
			Email:     u.EmailValue(),
			Phone:     u.PhoneValue(),
			Role:      u.Role,
			RealName:  u.SpeakRealName,
			Contact:   u.SpeakContact,
			Note:      u.SpeakApplyNote,
			Status:    speakStatusOrDefault(u.SpeakStatus),
			AppliedAt: u.SpeakAppliedAt,
			CreatedAt: u.CreatedAt,
		})
	}
	return items, total, nil
}

// Review 审核发言申请：通过则放开发言，驳回则记录原因。
//
// 用 CAS 保证"一条申请只被处理一次"：两个管理员同时操作时，
// 后到的那个收到明确的状态冲突，而不是静默覆盖前一个人的结论。
func (s *SpeakService) Review(ctx context.Context, adminID int64, role string, userID int64, req dto.AdminSpeakReviewReq, meta AuditMeta) error {
	note := strings.TrimSpace(req.Note)
	if !req.Approve && note == "" {
		// 驳回必须给理由：用户看不到原因就只能反复提交，
		// 后台队列会变成同一个人的刷屏。
		return errcode.ErrAuditRemarkRequired
	}
	if len([]rune(note)) > 255 {
		return errcode.ErrParamInvalid.WithMsg("审核备注不能超过 255 个字")
	}

	now := time.Now()
	status := model.SpeakRejected
	if req.Approve {
		status = model.SpeakApproved
	}

	affected, err := s.users.UpdateSpeakApply(ctx, userID, model.SpeakPending, map[string]any{
		"speak_status":      status,
		"speak_reviewed_at": now,
		"speak_review_note": note,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return errcode.ErrSpeakApplyState
	}

	action := "speak_apply.reject"
	if req.Approve {
		action = "speak_apply.approve"
	}
	appendAudit(ctx, s.audits, adminID, role, action, "user", userID,
		map[string]any{"approve": req.Approve, "note": note}, meta)

	logger.Info("发言申请已审核", "user_id", userID, "approve", req.Approve)
	return nil
}
