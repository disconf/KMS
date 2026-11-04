package service

import (
	"context"
	"strings"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

// MallService 积分商城。
//
// 积分以前是「只进不出」的展示数据，商城是它的第一个消费出口。
// 服务层职责很薄：校验 + 把仓储的 CAS 事务结果翻译成用户能懂的提示。
type MallService struct {
	repo   *repository.MallRepo
	points *repository.PointRepo
}

// NewMallService 创建服务。

func NewMallService(repo *repository.MallRepo, points *repository.PointRepo) *MallService {
	return &MallService{repo: repo, points: points}
}

// mallKindLabel 商城类型的展示名。
func mallKindLabel(kind string) string {
	switch kind {
	case model.MallKindDownload:
		return "下载解锁"
	case model.MallKindTemplate:
		return "模板文档"
	case model.MallKindRedeemCode:
		return "兑换码"
	case model.MallKindBenefit:
		return "站内权益"
	default:
		return "权益"
	}
}

// mallStatusLabel 兑换单状态展示名。
func mallStatusLabel(status string) string {
	switch status {
	case model.MallRedemptionPending:
		return "待发放"
	case model.MallRedemptionFulfilled:
		return "已发放"
	case model.MallRedemptionCancelled:
		return "已取消"
	default:
		return status
	}
}

// mallItemDTO 商品 → DTO。
func mallItemDTO(item model.MallItem) dto.MallItemItem {
	soldOut := !item.Unlimited && item.Stock <= 0
	return dto.MallItemItem{
		ID:          item.ID,
		Title:       item.Title,
		Description: item.Description,
		Kind:        item.Kind,
		KindLabel:   mallKindLabel(item.Kind),
		PointsCost:  item.PointsCost,
		Stock:       item.Stock,
		Unlimited:   item.Unlimited,
		Active:      item.Active,
		SoldOut:     soldOut,
		RedeemNote:  item.RedeemNote,
	}
}

// mallRedemptionDTO 兑换单 → DTO。
func mallRedemptionDTO(rec model.MallRedemption) dto.MallRedemptionItem {
	return dto.MallRedemptionItem{
		ID:          rec.ID,
		ItemTitle:   rec.ItemTitle,
		PointsCost:  rec.PointsCost,
		Status:      rec.Status,
		StatusLabel: mallStatusLabel(rec.Status),
		Remark:      rec.Remark,
		CreatedAt:   rec.CreatedAt,
		FulfilledAt: rec.FulfilledAt,
	}
}

// ListItems 商城首页数据：上架商品 + 我的余额。
//
// 游客可浏览（商城本身就是攒分的动机展示），余额用 -1 表示未登录。
func (s *MallService) ListItems(ctx context.Context, userID int64) (*dto.MallListResp, error) {
	items, err := s.repo.ListItems(ctx, repository.MallItemFilter{ActiveOnly: true})
	if err != nil {
		return nil, err
	}
	resp := &dto.MallListResp{MyPoints: -1, Items: make([]dto.MallItemItem, 0, len(items))}
	for _, it := range items {
		resp.Items = append(resp.Items, mallItemDTO(it))
	}
	if userID > 0 {
		if pts, err := s.points.PointsOf(ctx, userID); err == nil {
			resp.MyPoints = pts
		}
	}
	return resp, nil
}

// Redeem 兑换。
//
// 校验（上架、未罄、余额够）都在事务的 CAS 里最后裁决——这里只做
// 提前挡一道的好心检查，真正的并发安全靠仓储层的条件更新。
func (s *MallService) Redeem(ctx context.Context, userID, itemID int64) (*dto.MallRedeemResp, error) {
	item, err := s.repo.ItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if !item.Active {
		return nil, errcode.ErrMallItemNotFound
	}
	if !item.Unlimited && item.Stock <= 0 {
		return nil, errcode.ErrMallStockEmpty
	}

	redemption, err := s.repo.RedeemOne(ctx, userID, item)
	if err != nil {
		return nil, err
	}

	pointsAfter, err := s.points.PointsOf(ctx, userID)
	if err != nil {
		// 兑换已成功，余额查询失败不该让整个请求报错；前端下次进页会刷新。
		pointsAfter = 0
	}
	return &dto.MallRedeemResp{
		Redemption:  mallRedemptionDTO(*redemption),
		PointsAfter: pointsAfter,
	}, nil
}

// MyRedemptions 我的兑换记录。
func (s *MallService) MyRedemptions(ctx context.Context, userID int64, limit int) ([]dto.MallRedemptionItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	recs, err := s.repo.ListRedemptionsOf(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MallRedemptionItem, 0, len(recs))
	for _, rec := range recs {
		out = append(out, mallRedemptionDTO(rec))
	}
	return out, nil
}

// ============================================================================
// 后台管理。
// ============================================================================

var mallKinds = map[string]bool{
	model.MallKindDownload:   true,
	model.MallKindTemplate:   true,
	model.MallKindRedeemCode: true,
	model.MallKindBenefit:    true,
}

// AdminCreateItem 新建商品。
func (s *MallService) AdminCreateItem(ctx context.Context, in dto.MallAdminItemUpsert) (*dto.MallItemItem, error) {
	if err := validateMallUpsert(in); err != nil {
		return nil, err
	}
	item := model.MallItem{
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		Kind:        in.Kind,
		PointsCost:  in.PointsCost,
		Stock:       in.Stock,
		Unlimited:   in.Unlimited,
		Active:      in.Active,
		Sort:        in.Sort,
		RedeemNote:  strings.TrimSpace(in.RedeemNote),
	}
	if err := s.repo.CreateItem(ctx, &item); err != nil {
		return nil, err
	}
	out := mallItemDTO(item)
	return &out, nil
}

// AdminUpdateItem 编辑商品。指针字段区分「没传」与「传了零值」。
func (s *MallService) AdminUpdateItem(ctx context.Context, id int64, in dto.MallAdminItemUpsert) error {
	if _, err := s.repo.ItemByID(ctx, id); err != nil {
		return err
	}
	if err := validateMallUpsert(in); err != nil {
		return err
	}
	fields := map[string]any{
		"title":       strings.TrimSpace(in.Title),
		"description": strings.TrimSpace(in.Description),
		"kind":        in.Kind,
		"points_cost": in.PointsCost,
		"stock":       in.Stock,
		"unlimited":   in.Unlimited,
		"active":      in.Active,
		"sort":        in.Sort,
		"redeem_note": strings.TrimSpace(in.RedeemNote),
	}
	return s.repo.UpdateItemFields(ctx, id, fields)
}

// AdminDeleteItem 删除商品（软删）。
func (s *MallService) AdminDeleteItem(ctx context.Context, id int64) error {
	return s.repo.DeleteItem(ctx, id)
}

// AdminListItems 后台商品列表（含下架）。
func (s *MallService) AdminListItems(ctx context.Context) ([]dto.MallItemItem, error) {
	items, err := s.repo.ListItems(ctx, repository.MallItemFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]dto.MallItemItem, 0, len(items))
	for _, it := range items {
		out = append(out, mallItemDTO(it))
	}
	return out, nil
}

// AdminListRedemptions 后台兑换单队列。
func (s *MallService) AdminListRedemptions(ctx context.Context, status string, page, pageSize int) ([]dto.MallRedemptionItem, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	recs, total, err := s.repo.ListRedemptions(ctx, status, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.MallRedemptionItem, 0, len(recs))
	for _, rec := range recs {
		out = append(out, mallRedemptionDTO(rec))
	}
	return out, total, nil
}

// AdminFulfill 发放。凭证必填：发放出去的东西必须可追溯。
func (s *MallService) AdminFulfill(ctx context.Context, id int64, remark string) error {
	remark = strings.TrimSpace(remark)
	if remark == "" {
		return errcode.ErrParamMissing
	}
	ok, err := s.repo.FulfillRedemption(ctx, id, remark)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrMallRedemptionBad
	}
	return nil
}

// AdminCancel 取消并退分。理由必填，进审计视角。
func (s *MallService) AdminCancel(ctx context.Context, id int64, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errcode.ErrParamMissing
	}
	rec, err := s.repo.RedemptionByID(ctx, id)
	if err != nil {
		return err
	}
	ok, err := s.repo.CancelRedemption(ctx, rec)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrMallRedemptionBad
	}
	return nil
}

// validateMallUpsert 商品入参校验。
func validateMallUpsert(in dto.MallAdminItemUpsert) error {
	if strings.TrimSpace(in.Title) == "" {
		return errcode.ErrMallTitleRequired
	}
	if in.PointsCost <= 0 {
		return errcode.ErrMallCostInvalid
	}
	if !mallKinds[in.Kind] {
		return errcode.ErrMallKindInvalid
	}
	return nil
}
