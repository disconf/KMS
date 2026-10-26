package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

// SkillAdminService 技能字典的后台维护。
//
// 技能名此前只能靠种子数据与手工 SQL 增补——入驻页的下拉框里
// 缺一个新方向时，运营只能找开发发版。这个 service 把字典的增改启停收进后台。
type SkillAdminService struct {
	skills *repository.SkillRepo
}

// NewSkillAdminService 创建 SkillAdminService。
func NewSkillAdminService(skills *repository.SkillRepo) *SkillAdminService {
	return &SkillAdminService{skills: skills}
}

// List 全量技能（含停用），附引用数。
func (s *SkillAdminService) List(ctx context.Context) ([]dto.AdminSkillItem, error) {
	rows, err := s.skills.List(ctx, false)
	if err != nil {
		return nil, err
	}
	counts, err := s.skills.CountRefsBySkill(ctx)
	if err != nil {
		// 引用数查不出来不让列表打不开：它只是「能不能删」的参考，
		// 真正的删除校验在 Delete 里再查一次，不依赖这里的数据。
		counts = map[int64]int64{}
	}
	items := make([]dto.AdminSkillItem, 0, len(rows))
	for i := range rows {
		items = append(items, dto.AdminSkillItem{
			ID:          rows[i].ID,
			Name:        rows[i].Name,
			Category:    rows[i].Category,
			Sort:        rows[i].Sort,
			Enabled:     rows[i].Enabled,
			ExpertCount: counts[rows[i].ID],
		})
	}
	return items, nil
}

// Create 新增技能。
func (s *SkillAdminService) Create(ctx context.Context, req dto.AdminSkillCreateReq) (*dto.AdminSkillItem, error) {
	name, err := validateSkillName(req.Name)
	if err != nil {
		return nil, err
	}
	category, err := validateSkillCategory(req.Category)
	if err != nil {
		return nil, err
	}
	if exists, err := s.skills.ExistsName(ctx, name, 0); err != nil {
		return nil, err
	} else if exists {
		return nil, errcode.ErrSkillTaken
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item := model.Skill{Name: name, Category: category, Sort: req.Sort, Enabled: enabled}
	if err := s.skills.Create(ctx, &item); err != nil {
		return nil, err
	}
	return &dto.AdminSkillItem{
		ID: item.ID, Name: item.Name, Category: item.Category,
		Sort: item.Sort, Enabled: item.Enabled,
	}, nil
}

// Update 局部更新技能。
func (s *SkillAdminService) Update(ctx context.Context, id int64, req dto.AdminSkillUpdateReq) (*dto.AdminSkillItem, error) {
	if _, err := s.skills.GetByID(ctx, id); err != nil {
		return nil, err
	}

	fields := make(map[string]any, 4)
	if req.Name != nil {
		name, err := validateSkillName(*req.Name)
		if err != nil {
			return nil, err
		}
		if exists, err := s.skills.ExistsName(ctx, name, id); err != nil {
			return nil, err
		} else if exists {
			return nil, errcode.ErrSkillTaken
		}
		fields["name"] = name
	}
	if req.Category != nil {
		category, err := validateSkillCategory(*req.Category)
		if err != nil {
			return nil, err
		}
		fields["category"] = category
	}
	if req.Sort != nil {
		fields["sort"] = *req.Sort
	}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
	}

	if len(fields) > 0 {
		if err := s.skills.UpdateFields(ctx, id, fields); err != nil {
			return nil, err
		}
	}
	row, err := s.skills.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.AdminSkillItem{
		ID: row.ID, Name: row.Name, Category: row.Category,
		Sort: row.Sort, Enabled: row.Enabled,
	}, nil
}

// Delete 删除技能。
//
// 被专家档案引用时拒绝并提示改用停用：删掉被引用的技能，
// 专家主页和搜索里就会出现悬空 ID。停用是无损的——
// 下拉框里消失，但已有档案上的展示不受影响。
func (s *SkillAdminService) Delete(ctx context.Context, id int64) error {
	if _, err := s.skills.GetByID(ctx, id); err != nil {
		return err
	}
	counts, err := s.skills.CountRefsBySkill(ctx)
	if err != nil {
		return err
	}
	if n := counts[id]; n > 0 {
		return errcode.ErrSkillInUse.WithMsg(
			fmt.Sprintf("该技能已被 %d 位专家引用，不能删除；如需下架请改用停用", n))
	}
	return s.skills.Delete(ctx, id)
}

// validateSkillName 校验技能名称。
func validateSkillName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errcode.ErrParamInvalid.WithMsg("技能名称不能为空")
	}
	if n := runeLen(name); n > 32 {
		return "", errcode.ErrParamInvalid.WithMsg("技能名称不能超过 32 个字符")
	}
	return name, nil
}

// validateSkillCategory 校验技能类目。类目是自由文本而不是外键：
// 字典按「先窄后宽」演进，新方向不要求先建词条再挂技能。
func validateSkillCategory(category string) (string, error) {
	cat := strings.TrimSpace(category)
	if cat == "" {
		return "", errcode.ErrParamInvalid.WithMsg("技能类目不能为空")
	}
	if n := runeLen(cat); n > 32 {
		return "", errcode.ErrParamInvalid.WithMsg("技能类目不能超过 32 个字符")
	}
	return cat, nil
}
