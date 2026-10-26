package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

// commissionRateMaxBP 抽佣基点上限，与 config.Validate 的口径一致：
// 超过 50% 基本可以断定是把基点当百分比填了（1500 想表达 15% 却写成了 1500%）。
const commissionRateMaxBP = 5000

// SettingService 后台系统配置。
//
// 抽佣比例从配置文件搬到数据库，是为了让运营改比例不再依赖一次重启。
// 关键取舍：**下单时实时查库、不加缓存**。订单创建是低频操作，
// 一次主键查询的成本可以忽略；而缓存（哪怕 60 秒）意味着"改完比例，
// 60 秒窗口内下的单还在按旧比例算钱"，运营改完立刻想验证时对不上数，
// 多副本部署时各节点缓存还会不一致——为一个查询缓存，买来的全是坑。
type SettingService struct {
	settings *repository.SettingRepo
	audits   *repository.AuditRepo
	// defaultBP 配置文件里的默认值：库里没设过时生效，也是页面上的「出厂值」。
	defaultBP int
}

// NewSettingService 创建 SettingService。defaultBP 取自配置文件的抽佣基点。
func NewSettingService(settings *repository.SettingRepo, audits *repository.AuditRepo, defaultBP int) *SettingService {
	return &SettingService{settings: settings, audits: audits, defaultBP: defaultBP}
}

// GetCommissionRateBP 返回当前生效的抽佣基点。
//
// 两个失败场景分开对待：
//   - 查不到 → 回落配置文件默认值（正常：还没人在后台改过）；
//   - 查库失败 → 直接返回错误。宁可让这一单创建失败，也不能按一个
//     "我们不知道对不对"的比例把钱算出来——错一分钱对账时都要人工核。
//
// 库里值本身坏了（手工改库改出非数字）记 error 后回落默认值：
// 库是好的、只是这一行脏了，让全站下单都失败来惩罚一个配置项不划算。
func (s *SettingService) GetCommissionRateBP(ctx context.Context) (int, error) {
	row, err := s.settings.Get(ctx, model.SettingKeyCommissionRateBP)
	if err != nil {
		if errors.Is(err, errcode.ErrNotFound) {
			return s.defaultBP, nil
		}
		return 0, err
	}
	bp, convErr := strconv.Atoi(strings.TrimSpace(row.Value))
	if convErr != nil || bp < 0 || bp > commissionRateMaxBP {
		logger.Error("系统配置里的抽佣基点非法，已回落默认值",
			"key", model.SettingKeyCommissionRateBP,
			"value", row.Value, "default", s.defaultBP)
		return s.defaultBP, nil
	}
	return bp, nil
}

// Overview 后台配置总览。
func (s *SettingService) Overview(ctx context.Context) (*dto.AdminSettingOverview, error) {
	bp, err := s.GetCommissionRateBP(ctx)
	if err != nil {
		return nil, err
	}
	view := dto.CommissionSettingView{RateBP: bp, DefaultRateBP: s.defaultBP}

	row, err := s.settings.Get(ctx, model.SettingKeyCommissionRateBP)
	switch {
	case err == nil:
		view.IsOverride = true
		view.UpdatedAt = &row.UpdatedAt
		view.UpdatedBy = row.UpdatedBy
	case errors.Is(err, errcode.ErrNotFound):
		// 还没被后台改过，IsOverride 维持 false。
	default:
		return nil, err
	}
	return &dto.AdminSettingOverview{Commission: view}, nil
}

// SetCommissionRateBP 修改抽佣比例。
//
// 修改即审计：from/to 都要留——审计日志回答的不只是"改成了多少"，
// 更重要的是"改之前是多少"，否则出问题回看时只有终值没有起点。
func (s *SettingService) SetCommissionRateBP(
	ctx context.Context, operatorID int64, operatorRole string,
	bp int, meta AuditMeta,
) (*dto.CommissionSettingView, error) {
	if bp < 0 || bp > commissionRateMaxBP {
		return nil, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("抽佣比例应在 0 到 50%% 之间（当前 %d 基点，1500 = 15%%）", bp))
	}

	before, err := s.GetCommissionRateBP(ctx)
	if err != nil {
		return nil, err
	}
	if before == bp {
		// 幂等：值没变就当没改，不写库也不留审计——
		// 否则审计里很快会堆满"点了但没变"的噪声，真正的变更被淹没。
		return s.commissionView(ctx, bp, false, nil, 0)
	}

	if err := s.settings.Upsert(ctx, &model.SystemSetting{
		Key:       model.SettingKeyCommissionRateBP,
		Value:     strconv.Itoa(bp),
		Remark:    "平台抽佣基点，1500 = 15%",
		UpdatedBy: operatorID,
		UpdatedAt: time.Now(),
	}); err != nil {
		return nil, err
	}

	appendAudit(ctx, s.audits, operatorID, operatorRole,
		AuditActionSettingCommission, AuditTargetSetting, 0,
		map[string]any{
			"key":   model.SettingKeyCommissionRateBP,
			"from":  before,
			"to":    bp,
			"unit":  "bp",
			"today": "只影响之后创建的订单，已下单仍按各自快照结算",
		}, meta)

	return s.commissionView(ctx, bp, true, nil, operatorID)
}

// commissionView 组装抽佣配置视图；带出最后修改时间供页面展示。
func (s *SettingService) commissionView(ctx context.Context, bp int, isOverride bool, at *time.Time, by int64) (*dto.CommissionSettingView, error) {
	view := &dto.CommissionSettingView{
		RateBP: bp, DefaultRateBP: s.defaultBP, IsOverride: isOverride,
		UpdatedAt: at, UpdatedBy: by,
	}
	if !isOverride {
		return view, nil
	}
	if row, err := s.settings.Get(ctx, model.SettingKeyCommissionRateBP); err == nil {
		view.UpdatedAt = &row.UpdatedAt
		view.UpdatedBy = row.UpdatedBy
	}
	return view, nil
}
