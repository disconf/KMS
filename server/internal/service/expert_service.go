package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/storage"
	"github.com/opsexpert/platform/internal/repository"
)

const (
	// expertWalletItems 钱包页返回的收入明细 / 提现记录条数。
	expertWalletItems = 20
	// certKeyPrefix 资质证书的存储目录前缀，用于校验提交的 key 确实来自证书上传。
	certKeyPrefix = "expert/cert/"
)

// 长度与金额约束统一从 dto 读取，保证 service 校验与接口文档出自同一份定义，
// 避免将来只在一边改数字导致"文档说 5 个、实际允许 10 个"这类不一致。
var (
	limitsRealNameMin, limitsRealNameMax,
	limitsIntroMin, limitsIntroMax,
	limitsExperienceMax, limitsAvatarMax = dto.ExpertLimits()

	limitsMaxSkills, limitsMaxCerts = dto.ExpertSkillLimits()
	limitsMinPrice, limitsMaxPrice  = dto.PriceLimits()
	limitsIDCardLen                 = dto.IDCardLen()
)

// ExpertService 专家业务逻辑：入驻申请、资质审核、对外展示、服务设置与钱包。
type ExpertService struct {
	experts     *repository.ExpertRepo
	skills      *repository.SkillRepo
	users       *repository.UserRepo
	orders      *repository.OrderRepo
	settlements *repository.SettlementRepo
	withdrawals *repository.WithdrawalRepo
	reviews     *repository.ReviewRepo
	uploads     *UploadService
}

// NewExpertService 创建 ExpertService。
func NewExpertService(
	experts *repository.ExpertRepo,
	skills *repository.SkillRepo,
	users *repository.UserRepo,
	orders *repository.OrderRepo,
	settlements *repository.SettlementRepo,
	withdrawals *repository.WithdrawalRepo,
	reviews *repository.ReviewRepo,
	uploads *UploadService,
) *ExpertService {
	return &ExpertService{
		experts: experts, skills: skills, users: users,
		orders: orders, settlements: settlements, withdrawals: withdrawals,
		reviews: reviews, uploads: uploads,
	}
}

// ============================ 入驻申请 ============================

// Apply 提交专家入驻申请。
//
// 三种情况可以提交：从未申请、上次被驳回、已被停牌后申请复牌。
// 审核中或已通过的重复提交会被拒绝，避免同一份资质被反复刷进审核队列。
func (s *ExpertService) Apply(ctx context.Context, userID int64, req dto.ExpertApplyReq) (*dto.ExpertApplyStatus, error) {
	realName, err := normalizeRealName(req.RealName)
	if err != nil {
		return nil, err
	}
	// 证件号允许留空（重新申请时沿用库里已存的那份），但填了就必须真的合法——
	// 空值不在这里判定，因为"有没有底可沿"取决于库里有没有老记录。
	idCard := strings.TrimSpace(req.IDCardNo)
	if idCard != "" && !validateIDCard(idCard) {
		return nil, errcode.ErrIDCardInvalid
	}
	intro, err := normalizeIntro(req.Intro)
	if err != nil {
		return nil, err
	}
	if err := validatePrice(req.PricePerHourCents); err != nil {
		return nil, err
	}
	if err := validatePrice(req.PricePerTaskCents); err != nil {
		return nil, err
	}
	modes, err := normalizeServiceModes(req.ServiceModes)
	if err != nil {
		return nil, err
	}
	skills, err := s.normalizeSkills(ctx, req.Skills)
	if err != nil {
		return nil, err
	}
	certs, err := normalizeCertKeys(req.CertFiles)
	if err != nil {
		return nil, err
	}

	existing, findErr := s.experts.FindByUserID(ctx, userID)
	if findErr != nil && !errors.Is(findErr, errcode.ErrExpertNotApplied) {
		return nil, findErr
	}
	if findErr != nil {
		existing = nil
	}
	// 证件号留空 = 沿用库里已登记的那份。首次申请没有老记录，必须在这里挡住，
	// 否则会带着空证件号落库，审核时无据可查。
	if idCard == "" {
		if existing == nil || existing.IDCardNo == "" {
			return nil, errcode.ErrIDCardInvalid.WithMsg("请填写身份证号")
		}
		idCard = existing.IDCardNo
	}

	switch {
	case findErr == nil:
		switch existing.Status {
		case model.ExpertStatusPending:
			return nil, errcode.ErrExpertApplyDup.WithMsg("申请正在审核中，请耐心等待")
		case model.ExpertStatusApproved:
			return nil, errcode.ErrExpertApplyDup.WithMsg("你已经是认证专家，无需重复申请")
		}
		// rejected / suspended：允许修改后重新提交，状态回到待审核。
		fields := map[string]any{
			"real_name":            realName,
			"id_card_no":           idCard,
			"cert_files":           model.StringList(certs),
			"intro":                intro,
			"experience":           strings.TrimSpace(req.Experience),
			"service_modes":        model.StringList(modes),
			"price_per_hour_cents": req.PricePerHourCents,
			"price_per_task_cents": req.PricePerTaskCents,
			"status":               model.ExpertStatusPending,
			"audit_remark":         "",
			"applied_at":           time.Now(),
			"audited_at":           nil,
		}
		if avatar := strings.TrimSpace(req.Avatar); avatar != "" {
			fields["avatar"] = avatar
		}
		if err := s.experts.UpdateFields(ctx, existing.ID, fields); err != nil {
			return nil, err
		}
		if err := s.experts.ReplaceSkills(ctx, existing.ID, skills); err != nil {
			return nil, err
		}

	case errors.Is(findErr, errcode.ErrExpertNotApplied):
		e := &model.Expert{
			UserID:            userID,
			RealName:          realName,
			IDCardNo:          idCard,
			CertFiles:         model.StringList(certs),
			Intro:             intro,
			Experience:        strings.TrimSpace(req.Experience),
			Avatar:            strings.TrimSpace(req.Avatar),
			ServiceModes:      model.StringList(modes),
			PricePerHourCents: req.PricePerHourCents,
			PricePerTaskCents: req.PricePerTaskCents,
			// 新提交的申请默认不接单，等审核通过后由本人开启，
			// 避免资质未核实就先出现在接单列表里。
			AcceptingOrder: false,
			Status:         model.ExpertStatusPending,
			AppliedAt:      time.Now(),
		}
		if err := s.experts.Create(ctx, e); err != nil {
			return nil, err
		}
		if err := s.experts.ReplaceSkills(ctx, e.ID, skills); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	// 申请后把角色提升为 expert：这样即便审核未过，中间件也能给出
	// "审核中" 而不是 "请先申请"，前端才能把用户引到正确的页面。
	if err := s.users.UpdateFields(ctx, userID, map[string]any{"role": model.RoleExpert}); err != nil {
		logger.Warn("更新用户角色为专家失败", "user_id", userID, "err", err.Error())
	}

	return s.ApplyStatus(ctx, userID)
}

// ApplyStatus 返回当前用户的入驻申请状态。
func (s *ExpertService) ApplyStatus(ctx context.Context, userID int64) (*dto.ExpertApplyStatus, error) {
	e, err := s.experts.FindByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, errcode.ErrExpertNotApplied) {
			return &dto.ExpertApplyStatus{Status: ""}, nil
		}
		return nil, err
	}

	out := &dto.ExpertApplyStatus{
		Status:       e.Status,
		AuditRemark:  e.AuditRemark,
		RealName:     e.RealName,
		MaskedIDCard: maskIDCard(e.IDCardNo),
		AppliedAt:    &e.AppliedAt,
		AuditedAt:    e.AuditedAt,
	}
	// 只有被驳回或被停牌时才回显草稿：其余状态下用户不需要改，
	// 回显反而会把资质材料多暴露一次。
	if e.Status != model.ExpertStatusRejected && e.Status != model.ExpertStatusSuspended {
		return out, nil
	}

	skills, err := s.skillsOf(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out.Draft = &dto.ExpertApplyDraft{
		RealName:          e.RealName,
		Intro:             e.Intro,
		Experience:        e.Experience,
		Avatar:            e.Avatar,
		Skills:            skills,
		ServiceModes:      []string(e.ServiceModes),
		PricePerHourCents: e.PricePerHourCents,
		PricePerTaskCents: e.PricePerTaskCents,
		CertFiles:         s.toFileRefs([]string(e.CertFiles)),
	}
	return out, nil
}

// ============================ 对外展示 ============================

// ListExperts 查询已通过审核的专家列表。
func (s *ExpertService) ListExperts(ctx context.Context, q dto.ExpertListQuery) ([]dto.ExpertCard, int64, error) {
	rows, total, err := s.experts.List(ctx, repository.ExpertFilter{
		// 前台永远只看到审核通过的专家，这是硬约束，不接受调用方覆盖。
		Status:        model.ExpertStatusApproved,
		SkillID:       q.SkillID,
		Keyword:       q.Keyword,
		MinPriceCents: q.MinPriceCents,
		MaxPriceCents: q.MaxPriceCents,
		OnlineOnly:    q.OnlineOnly,
		MinRating:     q.MinRating,
		Sort:          q.Sort,
		// 被平台封禁的账号不再出现在前台列表里。
		OnlyActiveUser: true,
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	cards, err := s.toCards(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	return cards, total, nil
}

// ExpertDetail 查询专家公开详情。
func (s *ExpertService) ExpertDetail(ctx context.Context, id int64) (*dto.ExpertDetail, error) {
	row, err := s.experts.GetRow(ctx, id)
	if err != nil {
		return nil, err
	}
	// 未通过审核的专家对外不可见：直接按不存在处理，
	// 避免通过 ID 遍历探测出还在审核中的人。
	if row.Status != model.ExpertStatusApproved {
		return nil, errcode.ErrExpertNotFound
	}

	skillRows, err := s.experts.SkillsOf(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	appliedAt := row.AppliedAt
	return &dto.ExpertDetail{
		ExpertCard:     buildExpertCard(row, skillRows),
		Intro:          row.Intro,
		Experience:     row.Experience,
		ResolveRate:    row.ResolveRate,
		AvgResponseSec: row.AvgResponseSec,
		AppliedAt:      &appliedAt,
		JoinedAt:       row.CreatedAt,
	}, nil
}

// ReviewsOfExpert 查询专家主页的公开评价（含汇总与星级分布）。
//
// 与详情接口分开而不是塞进 ExpertDetail：详情是首屏必拿的，
// 评价要分页、要"加载更多"，塞进去会让首屏返回体随评价数增长。
func (s *ExpertService) ReviewsOfExpert(ctx context.Context, expertID int64, q dto.PageQuery) (*dto.ExpertReviewPage, error) {
	row, err := s.experts.GetRow(ctx, expertID)
	if err != nil {
		return nil, err
	}
	// 未通过审核的专家连主页都是 404，评价自然也不该拿到——
	// 否则可以靠评价接口绕过"审核中不可见"。
	if row.Status != model.ExpertStatusApproved {
		return nil, errcode.ErrExpertNotFound
	}

	// reviews.expert_id 存的是被评价专家的**用户 ID**，不是档案 ID，
	// 这里必须换成 row.UserID，直接用路径上的档案 ID 会永远查不到东西。
	stats, err := s.reviews.StatsOfExpert(ctx, row.UserID)
	if err != nil {
		return nil, err
	}
	dist, err := s.reviews.ScoreDistribution(ctx, row.UserID)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.reviews.ListPublicByExpert(ctx, row.UserID, q.Offset(), q.Limit())
	if err != nil {
		return nil, err
	}

	items := make([]dto.ExpertReviewItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, toPublicReview(r))
	}
	// 固定 5..1 五档，缺的补 0：前端分布条要画五根。
	buckets := make([]dto.ScoreBucket, 0, 5)
	for score := 5; score >= 1; score-- {
		buckets = append(buckets, dto.ScoreBucket{Score: score, Count: dist[score]})
	}

	return &dto.ExpertReviewPage{
		Summary: dto.ExpertReviewSummary{
			Total:    stats.Total,
			AvgScore: roundTo1(stats.AvgScore),
			GoodRate: roundTo1(stats.GoodRate),
			// 平均分保留一位小数：4.66666 直接下发，前端 toFixed 与
			// 列表里的星级会对不上（一个显示 4.7 一个显示 4.7 星半）。
			Distribution: buckets,
		},
		List:     items,
		Total:    total,
		Page:     q.Page,
		PageSize: q.PageSize,
	}, nil
}

// anonymousNickname 匿名评价对外展示的名字。
const anonymousNickname = "匿名用户"

// toPublicReview 把仓储行转成公开评价项。
//
// 脱敏只在这一层做：模型带着 userId / orderId，直接下发等于把
// "这条评价对应哪一单、谁下的"交给任何能打开主页的人。
func toPublicReview(r repository.ReviewRow) dto.ExpertReviewItem {
	item := dto.ExpertReviewItem{
		Nickname:           r.Nickname,
		Avatar:             r.Avatar,
		Score:              r.Score,
		ProfessionalScore:  r.ProfessionalScore,
		SpeedScore:         r.SpeedScore,
		CommunicationScore: r.CommunicationScore,
		Content:            r.Content,
		Recommend:          r.Recommend,
		Anonymous:          r.Anonymous,
		Reply:              r.Reply,
		RepliedAt:          r.RepliedAt,
		CreatedAt:          r.CreatedAt,
	}
	if item.Nickname == "" {
		item.Nickname = "运维用户"
	}
	// 匿名连头像一起抹掉：只藏名字、头像还能认出人，等于没匿名。
	if r.Anonymous {
		item.Nickname = anonymousNickname
		item.Avatar = ""
	}
	return item
}

// roundTo1 保留一位小数。
func roundTo1(v float64) float64 {
	return math.Round(v*10) / 10
}

// ============================ 专家端 ============================

// UpdateServices 更新服务设置（技能、定价、服务方式、接单开关）。
func (s *ExpertService) UpdateServices(ctx context.Context, userID int64, req dto.ExpertServiceUpdateReq) error {
	e, err := s.experts.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}

	fields := map[string]any{}
	if req.Skills != nil {
		skills, err := s.normalizeSkills(ctx, *req.Skills)
		if err != nil {
			return err
		}
		if err := s.experts.ReplaceSkills(ctx, e.ID, skills); err != nil {
			return err
		}
	}
	if req.ServiceModes != nil {
		modes, err := normalizeServiceModes(*req.ServiceModes)
		if err != nil {
			return err
		}
		fields["service_modes"] = model.StringList(modes)
	}
	if req.PricePerHourCents != nil {
		if err := validatePrice(*req.PricePerHourCents); err != nil {
			return err
		}
		fields["price_per_hour_cents"] = *req.PricePerHourCents
	}
	if req.PricePerTaskCents != nil {
		if err := validatePrice(*req.PricePerTaskCents); err != nil {
			return err
		}
		fields["price_per_task_cents"] = *req.PricePerTaskCents
	}
	if req.AcceptingOrder != nil {
		fields["accepting_order"] = *req.AcceptingOrder
	}

	return s.experts.UpdateFields(ctx, e.ID, fields)
}

// UpdateProfile 更新对外主页内容。
func (s *ExpertService) UpdateProfile(ctx context.Context, userID int64, req dto.ExpertProfileUpdateReq) error {
	e, err := s.experts.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}

	fields := map[string]any{}
	if req.Intro != nil {
		intro, err := normalizeIntro(*req.Intro)
		if err != nil {
			return err
		}
		fields["intro"] = intro
	}
	if req.Experience != nil {
		exp := strings.TrimSpace(*req.Experience)
		if runeLen(exp) > limitsExperienceMax {
			return errcode.ErrIntroInvalid.WithMsg("工作经历过长")
		}
		fields["experience"] = exp
	}
	if req.Avatar != nil {
		avatar := strings.TrimSpace(*req.Avatar)
		if runeLen(avatar) > limitsAvatarMax {
			return errcode.ErrParamInvalid.WithMsg("头像地址过长")
		}
		fields["avatar"] = avatar
	}
	return s.experts.UpdateFields(ctx, e.ID, fields)
}

// Dashboard 返回专家工作台概览。
func (s *ExpertService) Dashboard(ctx context.Context, userID int64) (*dto.ExpertDashboard, error) {
	e, err := s.experts.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := &dto.ExpertDashboard{
		Rating:           e.Rating,
		GoodRate:         e.GoodRate,
		BalanceCents:     e.BalanceCents,
		FrozenCents:      e.FrozenCents,
		TotalIncomeCents: e.TotalIncomeCents,
		AcceptingOrder:   e.AcceptingOrder,
		Status:           e.Status,

		ExpertID:          e.ID,
		RealName:          e.RealName,
		MaskedIDCard:      maskIDCard(e.IDCardNo),
		Avatar:            e.Avatar,
		Intro:             e.Intro,
		Experience:        e.Experience,
		ServiceModes:      []string(e.ServiceModes),
		PricePerHourCents: e.PricePerHourCents,
		PricePerTaskCents: e.PricePerTaskCents,
	}

	// 技能是编辑器的初始值，取不到就整页报错：给一张空表单让人保存，
	// 等于用一次点击把已有的技能清空。
	skills, err := s.skillsOf(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out.Skills = skills

	// 以下统计失败不影响工作台其余数据的展示，逐项降级。
	if n, err := s.orders.CountByExpert(ctx, userID); err != nil {
		logger.Warn("统计专家订单数失败", "user_id", userID, "err", err.Error())
	} else {
		out.OrderCount = n
	}
	if n, err := s.orders.CountByExpert(ctx, userID, model.OrderStatusPendingAccept); err != nil {
		logger.Warn("统计待接单数失败", "user_id", userID, "err", err.Error())
	} else {
		out.PendingAccept = n
	}
	monthStart := time.Now().Truncate(24*time.Hour).AddDate(0, 0, 1-time.Now().Day())
	if cents, err := s.settlements.SumSettledSince(ctx, userID, monthStart); err != nil {
		logger.Warn("统计本月收入失败", "user_id", userID, "err", err.Error())
	} else {
		out.MonthIncomeCents = cents
	}
	return out, nil
}

// Wallet 返回收入钱包：余额、冻结、累计收入与明细。
func (s *ExpertService) Wallet(ctx context.Context, userID int64) (*dto.ExpertWallet, error) {
	e, err := s.experts.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := &dto.ExpertWallet{
		BalanceCents:     e.BalanceCents,
		FrozenCents:      e.FrozenCents,
		TotalIncomeCents: e.TotalIncomeCents,
	}

	settlements, err := s.settlements.ListByExpert(ctx, userID, expertWalletItems)
	if err != nil {
		return nil, err
	}
	out.Items = make([]dto.SettlementItem, 0, len(settlements))
	for i := range settlements {
		it := settlements[i]
		out.Items = append(out.Items, dto.SettlementItem{
			ID: it.ID, SettlementNo: it.SettlementNo,
			OrderID: it.OrderID, OrderNo: it.OrderNo,
			AmountCents: it.AmountCents, CommissionCents: it.CommissionCents,
			ExpertAmountCents: it.AmountCents - it.CommissionCents,
			Status:            it.Status, SettledAt: it.SettledAt, CreatedAt: it.CreatedAt,
		})
	}

	withdrawals, err := s.withdrawals.ListByExpert(ctx, userID, expertWalletItems)
	if err != nil {
		return nil, err
	}
	out.Withdrawals = toWithdrawalItems(withdrawals)

	if pending, err := s.withdrawals.SumPending(ctx, userID); err != nil {
		logger.Warn("统计处理中提现失败", "user_id", userID, "err", err.Error())
	} else {
		out.PendingWithdrawCents = pending
	}
	return out, nil
}

// CreateWithdrawal 提交提现申请。
func (s *ExpertService) CreateWithdrawal(ctx context.Context, userID int64, req dto.WithdrawalCreateReq) (*dto.WithdrawalItem, error) {
	e, err := s.experts.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if e.Status != model.ExpertStatusApproved {
		return nil, errcode.ErrExpertUnderReview.WithMsg("资质通过审核后才可提现")
	}

	if req.AmountCents < dto.MinWithdrawCents() {
		return nil, errcode.ErrWithdrawAmountInvalid.WithMsg(
			fmt.Sprintf("单次提现不得低于 %d 元", dto.MinWithdrawCents()/100))
	}
	if err := validateWithdrawAccount(req.AccountType, req.AccountName, req.AccountNo); err != nil {
		return nil, err
	}

	// 同一时间只允许一笔处理中的申请：否则同一笔余额可以被重复申请，
	// 后续两笔都通过就会把余额提成负数。
	hasPending, err := s.withdrawals.HasPending(ctx, userID)
	if err != nil {
		return nil, err
	}
	if hasPending {
		return nil, errcode.ErrWithdrawPending
	}

	// 原子扣减：余额不足时 RowsAffected 为 0，避免并发下超额提现。
	ok, err := s.experts.TransferToFrozen(ctx, e.ID, req.AmountCents)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errcode.ErrBalanceInsufficient.WithMsg("可提现余额不足")
	}

	w := &model.Withdrawal{
		WithdrawalNo:    idgen.WithdrawalNo(),
		ExpertID:        userID,
		AmountCents:     req.AmountCents,
		FeeCents:        0,
		ActualCents:     req.AmountCents,
		AccountType:     req.AccountType,
		AccountName:     strings.TrimSpace(req.AccountName),
		AccountNoMasked: maskAccountNo(strings.TrimSpace(req.AccountNo)),
		Status:          model.WithdrawalStatusPending,
	}
	if err := s.withdrawals.Create(ctx, w); err != nil {
		// 落库失败要把刚才冻结的金额退回去，否则这笔钱会悬在冻结里。
		if rbErr := s.experts.RestoreFromFrozen(ctx, e.ID, req.AmountCents); rbErr != nil {
			logger.Error("提现落库失败且冻结额回退失败，需人工核对",
				"user_id", userID, "amount_cents", req.AmountCents, "err", rbErr.Error())
		}
		return nil, err
	}

	items := toWithdrawalItems([]model.Withdrawal{*w})
	return &items[0], nil
}

// ListWithdrawals 返回提现记录。
func (s *ExpertService) ListWithdrawals(ctx context.Context, userID int64) ([]dto.WithdrawalItem, error) {
	rows, err := s.withdrawals.ListByExpert(ctx, userID, expertWalletItems)
	if err != nil {
		return nil, err
	}
	return toWithdrawalItems(rows), nil
}

// ============================ 技能字典 ============================

// ListSkills 返回技能字典。enabledOnly 为 true 时只返回启用项。
func (s *ExpertService) ListSkills(ctx context.Context, enabledOnly bool) ([]dto.SkillItem, error) {
	skills, err := s.skills.List(ctx, enabledOnly)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SkillItem, 0, len(skills))
	for i := range skills {
		out = append(out, dto.SkillItem{
			ID: skills[i].ID, Name: skills[i].Name,
			Category: skills[i].Category, Sort: skills[i].Sort,
		})
	}
	return out, nil
}

// ============================ 后台 ============================

// AdminList 后台专家列表，可按审核状态过滤。
func (s *ExpertService) AdminList(ctx context.Context, q dto.AdminExpertListQuery) ([]dto.AdminExpertItem, int64, error) {
	rows, total, err := s.experts.List(ctx, repository.ExpertFilter{
		Status:  q.Status,
		Keyword: q.Keyword,
		// 后台默认按提交时间倒序，待审核的排最前面才有意义，
		// 这个由排序键 latest 保证。
		Sort: "latest",
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}

	cards, err := s.toCards(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	items := make([]dto.AdminExpertItem, 0, len(cards))
	for i := range rows {
		applied := rows[i].AppliedAt
		items = append(items, dto.AdminExpertItem{
			ExpertCard:   cards[i],
			RealName:     rows[i].RealName,
			MaskedIDCard: maskIDCard(rows[i].IDCardNo),
			Status:       rows[i].Status,
			AuditRemark:  rows[i].AuditRemark,
			AppliedAt:    &applied,
			AuditedAt:    rows[i].AuditedAt,
		})
	}
	return items, total, nil
}

// AdminDetail 后台审核详情，含资质材料（签名地址）。
func (s *ExpertService) AdminDetail(ctx context.Context, id int64) (*dto.AdminExpertDetail, error) {
	row, err := s.experts.GetRow(ctx, id)
	if err != nil {
		return nil, err
	}
	skillRows, err := s.experts.SkillsOf(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	applied := row.AppliedAt
	return &dto.AdminExpertDetail{
		AdminExpertItem: dto.AdminExpertItem{
			ExpertCard:   buildExpertCard(row, skillRows),
			RealName:     row.RealName,
			MaskedIDCard: maskIDCard(row.IDCardNo),
			Status:       row.Status,
			AuditRemark:  row.AuditRemark,
			AppliedAt:    &applied,
			AuditedAt:    row.AuditedAt,
		},
		Intro:      row.Intro,
		Experience: row.Experience,
		CertFiles:  s.toFileRefs([]string(row.CertFiles)),
	}, nil
}

// Audit 审核专家申请。
func (s *ExpertService) Audit(ctx context.Context, adminID, expertID int64, req dto.ExpertAuditReq) error {
	e, err := s.experts.GetByID(ctx, expertID)
	if err != nil {
		return err
	}
	// 只允许审核处于待审核或停牌状态的申请：
	// 已通过的专家要走停牌 / 复牌流程，不能靠重复审核改状态。
	switch e.Status {
	case model.ExpertStatusPending, model.ExpertStatusSuspended:
	default:
		return errcode.ErrExpertAudited
	}

	remark := strings.TrimSpace(req.Remark)
	if !req.Approved && remark == "" {
		// 驳回不写原因，用户只会看到"审核未通过"然后反复重提同样的材料。
		return errcode.ErrAuditRemarkRequired
	}
	if runeLen(remark) > 500 {
		return errcode.ErrParamInvalid.WithMsg("审核备注过长")
	}

	status := model.ExpertStatusRejected
	if req.Approved {
		status = model.ExpertStatusApproved
	}
	now := time.Now()
	fields := map[string]any{
		"status":       status,
		"audit_remark": remark,
		"audited_at":   now,
		"audited_by":   adminID,
	}
	// 审核通过后才允许出现在接单列表；被驳回 / 停牌时强制关闭接单开关，
	// 避免专家在不可服务状态下仍被用户预约到。
	fields["accepting_order"] = req.Approved

	if err := s.experts.UpdateFields(ctx, expertID, fields); err != nil {
		return err
	}
	// 审核结论要同步到用户角色，否则中间件仍按旧角色放行 / 拦截。
	role := model.RoleExpert
	if !req.Approved {
		role = model.RoleUser
	}
	if err := s.users.UpdateFields(ctx, e.UserID, map[string]any{"role": role}); err != nil {
		logger.Warn("同步专家角色失败", "user_id", e.UserID, "err", err.Error())
	}
	return nil
}

// ============================ 后台 · 提现审批 ============================

// AdminListWithdrawals 后台提现列表。
func (s *ExpertService) AdminListWithdrawals(ctx context.Context, q dto.AdminWithdrawalListQuery) ([]dto.AdminWithdrawalItem, int64, error) {
	rows, total, err := s.withdrawals.ListAdmin(ctx, repository.WithdrawalAdminFilter{
		Status:  q.Status,
		Keyword: strings.TrimSpace(q.Keyword),
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.AdminWithdrawalItem, 0, len(rows))
	for i := range rows {
		row := rows[i]
		items = append(items, dto.AdminWithdrawalItem{
			WithdrawalItem: toWithdrawalItem(row.Withdrawal),
			ExpertID:       row.ExpertID,
			ExpertName:     row.ExpertName,
			ExpertNick:     row.ExpertNick,
			ExpertStatus:   row.ExpertStatus,
			ExpertBalance:  row.ExpertBalance,
			ExpertFrozen:   row.ExpertFrozen,
			ReviewedAt:     row.ReviewedAt,
			ReviewedBy:     row.ReviewedBy,
		})
	}
	return items, total, nil
}

// AdminReviewWithdrawal 审核提现：通过（待打款）或驳回（冻结退回余额）。
//
// 驳回会把钱退回专家的可提现余额，所以顺序是「先 CAS 改状态，再动钱」：
// 反过来的话，两个人同时点驳回会退两次；而 CAS 失败的人不会走到动钱那一步。
func (s *ExpertService) AdminReviewWithdrawal(ctx context.Context, adminID, id int64, req dto.WithdrawalReviewReq) error {
	w, err := s.withdrawals.GetByID(ctx, id)
	if err != nil {
		return err
	}

	target := model.WithdrawalStatusRejected
	if req.Action == "approve" {
		target = model.WithdrawalStatusApproved
	}
	if !w.CanTransitionTo(target) {
		return errcode.ErrWithdrawalStateInvalid.WithMsg(withdrawalStateHint(w.Status))
	}

	remark := strings.TrimSpace(req.Remark)
	if target == model.WithdrawalStatusRejected && remark == "" {
		// 不写原因的驳回，专家只会看到「已驳回」然后原样重提。
		return errcode.ErrAuditRemarkRequired
	}
	if runeLen(remark) > 255 {
		return errcode.ErrParamInvalid.WithMsg("审批备注过长")
	}

	now := time.Now()
	ok, err := s.withdrawals.UpdateFieldsIfStatus(ctx, id, w.Status, map[string]any{
		"status":      target,
		"remark":      remark,
		"reviewed_at": now,
		"reviewed_by": adminID,
	})
	if err != nil {
		return err
	}
	if !ok {
		// 走到这里说明状态在"读出来"和"写回去"之间被改过，通常是另一个管理员同时点了。
		return errcode.ErrWithdrawalStateInvalid
	}

	if w.ReleasesFrozen(target) {
		if err := s.releaseFrozen(ctx, w, target); err != nil {
			return err
		}
	}
	return nil
}

// AdminMarkWithdrawalPaid 标记打款完成：冻结金额被扣掉，钱真正离开平台。
func (s *ExpertService) AdminMarkWithdrawalPaid(ctx context.Context, adminID, id int64, req dto.WithdrawalPaidReq) error {
	w, err := s.withdrawals.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// 只有「审核通过、待打款」才能标记已打款。
	// 直接从待审核跳到已打款意味着没人核对过收款人，钱可能打给错的人。
	if !w.CanTransitionTo(model.WithdrawalStatusPaid) {
		return errcode.ErrWithdrawalStateInvalid.WithMsg(withdrawalStateHint(w.Status))
	}

	remark := strings.TrimSpace(req.Remark)
	// 打款流水号是这笔钱的唯一线下凭据：不填就无法对账，
	// 事后只能看到"已打款"三个字，查不出打给了谁、什么时候打的。
	if remark == "" {
		return errcode.ErrParamInvalid.WithMsg("标记已打款需要填写打款流水号或备注")
	}
	if runeLen(remark) > 255 {
		return errcode.ErrParamInvalid.WithMsg("打款备注过长")
	}

	now := time.Now()
	ok, err := s.withdrawals.UpdateFieldsIfStatus(ctx, id, w.Status, map[string]any{
		"status":      model.WithdrawalStatusPaid,
		"remark":      remark,
		"reviewed_by": adminID,
		"paid_at":     now,
	})
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrWithdrawalStateInvalid
	}

	e, err := s.experts.FindByUserID(ctx, w.ExpertID)
	if err != nil {
		// 状态已经改成已打款但档案找不到，属于数据异常：日志留足线索，别把错误吞掉。
		logger.Error("提现已标记打款但专家档案缺失，需人工核对",
			"withdrawal_id", id, "expert_user_id", w.ExpertID)
		return err
	}
	if err := s.experts.ConsumeFrozen(ctx, e.ID, w.AmountCents); err != nil {
		logger.Error("提现已标记打款但冻结金额扣减失败，需人工核对",
			"withdrawal_id", id, "expert_id", e.ID, "amount_cents", w.AmountCents, "err", err.Error())
		return errcode.ErrDatabase.WithMsg("已标记打款，但冻结金额扣减失败，请联系技术核对")
	}
	return nil
}

// AdminFailWithdrawal 打款失败：冻结金额退回可提现余额，专家可以重新申请。
//
// 渠道退回、账号信息填错都走这里。不提供「失败后自动重试」：
// 账号问题不是重试能解决的，重试只会把同一笔钱反复打给同一个错账号。
func (s *ExpertService) AdminFailWithdrawal(ctx context.Context, adminID, id int64, req dto.WithdrawalPaidReq) error {
	w, err := s.withdrawals.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !w.CanTransitionTo(model.WithdrawalStatusFailed) {
		return errcode.ErrWithdrawalStateInvalid.WithMsg(withdrawalStateHint(w.Status))
	}

	remark := strings.TrimSpace(req.Remark)
	if remark == "" {
		return errcode.ErrAuditRemarkRequired.WithMsg("打款失败必须写明原因")
	}
	if runeLen(remark) > 255 {
		return errcode.ErrParamInvalid.WithMsg("打款备注过长")
	}

	ok, err := s.withdrawals.UpdateFieldsIfStatus(ctx, id, w.Status, map[string]any{
		"status":      model.WithdrawalStatusFailed,
		"remark":      remark,
		"reviewed_at": time.Now(),
		"reviewed_by": adminID,
	})
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrWithdrawalStateInvalid
	}
	if err := s.releaseFrozen(ctx, w, model.WithdrawalStatusFailed); err != nil {
		return err
	}
	return nil
}

// releaseFrozen 把冻结金额退回可提现余额。
//
// 走的是「改完状态再退钱」：状态没改成功就绝不退钱，
// 宁可钱暂时留在冻结里等人工核对，也不能出现「状态还是待审核、钱已经退回余额」。
func (s *ExpertService) releaseFrozen(ctx context.Context, w *model.Withdrawal, target string) error {
	e, err := s.experts.FindByUserID(ctx, w.ExpertID)
	if err != nil {
		logger.Error("提现状态已变更但专家档案缺失，冻结金额未退回",
			"withdrawal_id", w.ID, "expert_user_id", w.ExpertID, "target", target)
		return err
	}
	if err := s.experts.RestoreFromFrozen(ctx, e.ID, w.AmountCents); err != nil {
		logger.Error("提现状态已变更但冻结金额退回失败，需人工核对",
			"withdrawal_id", w.ID, "expert_id", e.ID, "amount_cents", w.AmountCents,
			"target", target, "err", err.Error())
		return errcode.ErrDatabase.WithMsg("状态已更新，但冻结金额退回失败，请联系技术核对")
	}
	return nil
}

// withdrawalStateHint 把当前状态翻成一句给人看的话。
// 只说"状态不合法"会让人不知道是重复点了，还是顺序弄反了。
func withdrawalStateHint(status string) string {
	switch status {
	case model.WithdrawalStatusApproved:
		return "这笔提现已通过审核，请直接标记打款"
	case model.WithdrawalStatusPaid:
		return "这笔提现已经打款完成"
	case model.WithdrawalStatusRejected:
		return "这笔提现已驳回"
	case model.WithdrawalStatusFailed:
		return "这笔提现已标记打款失败"
	default:
		return "请刷新后重试"
	}
}

// ============================ 内部辅助 ============================

// skillsOf 查询单个专家的技能展示项。
func (s *ExpertService) skillsOf(ctx context.Context, expertID int64) ([]dto.ExpertSkillItem, error) {
	rows, err := s.experts.SkillsOf(ctx, expertID)
	if err != nil {
		return nil, err
	}
	return toSkillItems(rows), nil
}

// toCards 批量组装专家卡片，技能一次性查出，避免列表页 N+1。
func (s *ExpertService) toCards(ctx context.Context, rows []repository.ExpertRow) ([]dto.ExpertCard, error) {
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	skillMap, err := s.experts.SkillsOfMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ExpertCard, 0, len(rows))
	for i := range rows {
		out = append(out, buildExpertCard(&rows[i], skillMap[rows[i].ID]))
	}
	return out, nil
}

// normalizeSkills 校验并转换提交的技能。
func (s *ExpertService) normalizeSkills(ctx context.Context, reqs []dto.ExpertSkillReq) ([]model.ExpertSkill, error) {
	if len(reqs) == 0 {
		return nil, errcode.ErrSkillInvalid.WithMsg("请至少选择一个技能方向")
	}
	if len(reqs) > limitsMaxSkills {
		return nil, errcode.ErrSkillInvalid.WithMsg(
			fmt.Sprintf("技能最多选择 %d 个", limitsMaxSkills))
	}

	seen := make(map[int64]struct{}, len(reqs))
	ids := make([]int64, 0, len(reqs))
	out := make([]model.ExpertSkill, 0, len(reqs))
	for _, r := range reqs {
		if r.SkillID <= 0 {
			return nil, errcode.ErrSkillInvalid.WithMsg("技能 ID 不合法")
		}
		if _, dup := seen[r.SkillID]; dup {
			// 重复提交同一技能会让 (expert_id, skill_id) 唯一索引直接报错，
			// 这里静默去重比抛错体验更好，用户意图是明确的。
			continue
		}
		seen[r.SkillID] = struct{}{}
		ids = append(ids, r.SkillID)

		level := strings.TrimSpace(r.Level)
		if level == "" {
			level = model.SkillLevelSenior
		}
		switch level {
		case model.SkillLevelJunior, model.SkillLevelSenior, model.SkillLevelExpert:
		default:
			return nil, errcode.ErrSkillInvalid.WithMsg("技能等级不合法")
		}
		out = append(out, model.ExpertSkill{SkillID: r.SkillID, Level: level, Years: r.Years})
	}

	// 校验技能真实存在且未停用，避免前端传随意 ID 造出无效标签。
	count, err := s.skills.CountEnabled(ctx, ids)
	if err != nil {
		return nil, err
	}
	if int(count) != len(ids) {
		return nil, errcode.ErrSkillInvalid.WithMsg("包含不存在或已停用的技能")
	}
	return out, nil
}

// toFileRefs 把存储 key 列表转成带访问地址的引用列表。
func (s *ExpertService) toFileRefs(keys []string) []dto.FileRef {
	out := make([]dto.FileRef, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		url, exp, _ := s.uploads.AssignURL(k)
		out = append(out, dto.FileRef{Key: k, URL: url, ExpireAt: exp})
	}
	return out
}

// buildExpertCard 由专家档案与技能组装卡片。
func buildExpertCard(row *repository.ExpertRow, skills []repository.ExpertSkillRow) dto.ExpertCard {
	avatar := row.Avatar
	if strings.TrimSpace(avatar) == "" {
		avatar = row.UserAvatar
	}
	nickname := strings.TrimSpace(row.Nickname)
	if nickname == "" {
		// 昵称缺失（用户没设）时退化展示，不要给前端一个空标题。
		nickname = "运维专家"
	}
	return dto.ExpertCard{
		ID:                row.ID,
		UserID:            row.UserID,
		Nickname:          nickname,
		Avatar:            avatar,
		RealNameVerified:  row.Status == model.ExpertStatusApproved,
		Skills:            toSkillItems(skills),
		Rating:            row.Rating,
		OrderCount:        row.OrderCount,
		GoodRate:          row.GoodRate,
		PricePerHourCents: row.PricePerHourCents,
		PricePerTaskCents: row.PricePerTaskCents,
		AcceptingOrder:    row.AcceptingOrder,
		ServiceModes:      []string(row.ServiceModes),
	}
}

// toSkillItems 转换技能展示项。
func toSkillItems(rows []repository.ExpertSkillRow) []dto.ExpertSkillItem {
	out := make([]dto.ExpertSkillItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ExpertSkillItem{
			SkillID: r.SkillID, Name: r.Name, Category: r.Category,
			Level: r.Level, Years: r.Years,
		})
	}
	return out
}

// toWithdrawalItems 转换提现记录。
func toWithdrawalItems(rows []model.Withdrawal) []dto.WithdrawalItem {
	out := make([]dto.WithdrawalItem, 0, len(rows))
	for i := range rows {
		out = append(out, toWithdrawalItem(rows[i]))
	}
	return out
}

// toWithdrawalItem 单条转换。后台列表要在此基础上再补专家档案，抽出来复用。
func toWithdrawalItem(w model.Withdrawal) dto.WithdrawalItem {
	return dto.WithdrawalItem{
		ID: w.ID, WithdrawalNo: w.WithdrawalNo,
		AmountCents: w.AmountCents, FeeCents: w.FeeCents, ActualCents: w.ActualCents,
		AccountType: w.AccountType, AccountName: w.AccountName,
		AccountNoMasked: w.AccountNoMasked,
		Status:          w.Status, Remark: w.Remark,
		CreatedAt: w.CreatedAt, PaidAt: w.PaidAt,
	}
}

// ============================ 校验与脱敏 ============================

// normalizeRealName 校验并规范化真实姓名。
func normalizeRealName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	n := runeLen(name)
	if n < limitsRealNameMin || n > limitsRealNameMax {
		return "", errcode.ErrRealNameInvalid.WithMsg("真实姓名长度不合法")
	}
	return name, nil
}

// normalizeIntro 校验并规范化个人简介。
func normalizeIntro(raw string) (string, error) {
	intro := strings.TrimSpace(raw)
	n := runeLen(intro)
	if n < limitsIntroMin || n > limitsIntroMax {
		return "", errcode.ErrIntroInvalid.WithMsg(
			fmt.Sprintf("个人简介需在 %d-%d 字之间", limitsIntroMin, limitsIntroMax))
	}
	return intro, nil
}

// normalizeServiceModes 校验服务方式。
func normalizeServiceModes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, errcode.ErrServiceModeInvalid.WithMsg("请至少选择一种服务方式")
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, m := range raw {
		m = strings.TrimSpace(m)
		switch m {
		case model.ServiceModeIM, model.ServiceModeVoice, model.ServiceModeVideo, model.ServiceModeScreen:
		default:
			return nil, errcode.ErrServiceModeInvalid
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out, nil
}

// normalizeCertKeys 校验资质证书 key。
//
// 必须确认 key 真的落在 expert/cert 目录下：否则调用方可以传任意
// 文件 key（比如别人的头像）来通过"已上传证书"的校验。
func normalizeCertKeys(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, errcode.ErrCertRequired
	}
	if len(raw) > limitsMaxCerts {
		return nil, errcode.ErrCertRequired.WithMsg(
			fmt.Sprintf("资质证书最多上传 %d 份", limitsMaxCerts))
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, k := range raw {
		cleaned, err := storage.CleanKey(k)
		if err != nil {
			return nil, errcode.ErrCertRequired.WithMsg("资质证书文件标识不合法")
		}
		if !strings.HasPrefix(cleaned, certKeyPrefix) {
			return nil, errcode.ErrCertRequired.WithMsg("资质证书必须通过 expert_cert 场景上传")
		}
		if _, dup := seen[cleaned]; dup {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	if len(out) == 0 {
		return nil, errcode.ErrCertRequired
	}
	return out, nil
}

// validatePrice 校验定价（分）。0 表示未定价，是允许的。
func validatePrice(cents int64) error {
	if cents < 0 {
		return errcode.ErrPriceInvalid.WithMsg("定价不能为负数")
	}
	if cents > 0 && cents < limitsMinPrice {
		return errcode.ErrPriceInvalid.WithMsg("定价过低，请重新设置")
	}
	if cents > limitsMaxPrice {
		return errcode.ErrPriceInvalid.WithMsg("定价超出上限")
	}
	return nil
}

// validateWithdrawAccount 校验收款账户。
func validateWithdrawAccount(accountType, name, no string) error {
	name = strings.TrimSpace(name)
	if runeLen(name) < 2 || runeLen(name) > 64 {
		return errcode.ErrWithdrawAccountInvalid.WithMsg("收款人姓名不合法")
	}
	no = strings.TrimSpace(no)
	if len(no) < 6 || len(no) > 64 {
		return errcode.ErrWithdrawAccountInvalid.WithMsg("收款账号长度不合法")
	}
	for _, ch := range no {
		// 收款账号只允许数字与邮箱常见字符（支付宝账号可能是邮箱或手机号）。
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') || ch == '@' || ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return errcode.ErrWithdrawAccountInvalid.WithMsg("收款账号包含非法字符")
	}
	switch accountType {
	case model.AccountTypeBank, model.AccountTypeAlipay, model.AccountTypeWechat:
	default:
		return errcode.ErrWithdrawAccountInvalid.WithMsg("收款账户类型不合法")
	}
	return nil
}

// validateIDCard 校验二代身份证号。
//
// 18 位：前 17 位数字，末位为数字或 X，且通过 ISO 7064:1983 MOD 11-2
// 校验位算法。只校验长度是拦不住随手编造一个号码的。
func validateIDCard(id string) bool {
	if len(id) != limitsIDCardLen {
		return false
	}
	for i := 0; i < 17; i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
	}
	last := id[17]
	if last == 'x' {
		last = 'X'
	}
	if last != 'X' && (last < '0' || last > '9') {
		return false
	}

	weights := [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	const checks = "10X98765432"
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(id[i]-'0') * weights[i]
	}
	return checks[sum%11] == last
}

// maskIDCard 保留身份证前 6 位与后 4 位。
//
// 前 6 位是地区码、后 4 位含性别与校验位，足够让本人确认填的是哪张证件，
// 又不足以还原完整号码。接口出参一律用它，绝不返回明文。
func maskIDCard(id string) string {
	if len(id) < 12 {
		return "***"
	}
	return id[:6] + "********" + id[len(id)-4:]
}

// maskAccountNo 只保留收款账号后 4 位。
func maskAccountNo(no string) string {
	if len(no) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(no)-4) + no[len(no)-4:]
}

// runeLen 按字符数（而非字节数）计算长度。
// 中文姓名与简介按字节算会误判，必须用 rune。
func runeLen(s string) int { return len([]rune(s)) }
