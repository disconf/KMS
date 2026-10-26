package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// SkillRepo 技能字典数据访问。
type SkillRepo struct {
	db *gorm.DB
}

// NewSkillRepo 创建 SkillRepo。
func NewSkillRepo(db *gorm.DB) *SkillRepo { return &SkillRepo{db: db} }

// List 返回技能字典。enabledOnly 为 true 时只返回启用项（前台口径）。
func (r *SkillRepo) List(ctx context.Context, enabledOnly bool) ([]model.Skill, error) {
	var items []model.Skill
	q := r.db.WithContext(ctx).Model(&model.Skill{})
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Order("sort ASC, id ASC").Find(&items).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询技能字典失败: %w", err))
	}
	return items, nil
}

// FindByIDs 批量查询，返回 id → 技能 的映射。
func (r *SkillRepo) FindByIDs(ctx context.Context, ids []int64) (map[int64]*model.Skill, error) {
	out := make(map[int64]*model.Skill)
	unique := dedupeIDs(ids)
	if len(unique) == 0 {
		return out, nil
	}
	var items []model.Skill
	if err := r.db.WithContext(ctx).Where("id IN ?", unique).Find(&items).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("批量查询技能失败: %w", err))
	}
	for i := range items {
		out[items[i].ID] = &items[i]
	}
	return out, nil
}

// CountEnabled 返回启用中的技能总数，用于校验提交的技能 ID 是否合法。
func (r *SkillRepo) CountEnabled(ctx context.Context, ids []int64) (int64, error) {
	unique := dedupeIDs(ids)
	if len(unique) == 0 {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Skill{}).
		Where("id IN ? AND enabled = ?", unique, true).Count(&count).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("校验技能可用性失败: %w", err))
	}
	return count, nil
}

// dedupeIDs 去重并剔除非法 ID，避免 IN 查询里混入 0 与重复值。
func dedupeIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

/* ============================ 后台管理 ============================ */

// GetByID 按主键查询；不存在时返回 errcode.ErrSkillNotFound。
func (r *SkillRepo) GetByID(ctx context.Context, id int64) (*model.Skill, error) {
	var s model.Skill
	if err := r.db.WithContext(ctx).First(&s, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrSkillNotFound)
	}
	return &s, nil
}

// ExistsName 判断技能名是否已存在（excludeID 用于更新时排除自身）。
func (r *SkillRepo) ExistsName(ctx context.Context, name string, excludeID int64) (bool, error) {
	q := r.db.WithContext(ctx).Model(&model.Skill{}).Where("name = ?", name)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("校验技能名唯一性失败: %w", err))
	}
	return count > 0, nil
}

// Create 新增技能。
func (r *SkillRepo) Create(ctx context.Context, s *model.Skill) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建技能失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新技能。
func (r *SkillRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Skill{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新技能 %d 失败: %w", id, err))
	}
	return nil
}

// CountRefsBySkill 一次统计所有技能被专家档案引用的数量，返回 skill_id → 引用数。
//
// 后台列表要给每个技能展示「被多少位专家使用」，逐条查会发 N 次查询；
// 一句 GROUP BY 就够。删除校验也复用这张表，不在 Delete 里单独再查。
func (r *SkillRepo) CountRefsBySkill(ctx context.Context) (map[int64]int64, error) {
	var rows []struct {
		SkillID int64
		Count   int64
	}
	err := r.db.WithContext(ctx).Model(&model.ExpertSkill{}).
		Select("skill_id, COUNT(*) AS count").
		Group("skill_id").
		Find(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计技能引用数失败: %w", err))
	}
	out := make(map[int64]int64, len(rows))
	for _, row := range rows {
		out[row.SkillID] = row.Count
	}
	return out, nil
}

// Delete 删除技能。必须物理删除（Unscoped）：
// skills.name 上有唯一索引，软删的行会一直占着名字，改名重建同名词时
// 会静默撞索引——和 ArticleLike 踩过的坑同源。调用方先过 CountExpertRefs。
func (r *SkillRepo) Delete(ctx context.Context, id int64) error {
	err := r.db.WithContext(ctx).Unscoped().Delete(&model.Skill{}, id).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("删除技能 %d 失败: %w", id, err))
	}
	return nil
}
