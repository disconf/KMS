package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// ExpertRepo 专家档案数据访问。
type ExpertRepo struct {
	db *gorm.DB
}

// NewExpertRepo 创建 ExpertRepo。
func NewExpertRepo(db *gorm.DB) *ExpertRepo { return &ExpertRepo{db: db} }

// FindByUserID 按用户 ID 查询专家档案；不存在时返回 errcode.ErrExpertNotApplied。
func (r *ExpertRepo) FindByUserID(ctx context.Context, userID int64) (*model.Expert, error) {
	var e model.Expert
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&e).Error
	if err != nil {
		return nil, normalizeNotFound(err, errcode.ErrExpertNotApplied)
	}
	return &e, nil
}

// StatusOf 返回专家审核状态；未申请时返回空串，不报错。
// 登录签发令牌时会调用它，因此这里刻意对"未申请"保持静默。
func (r *ExpertRepo) StatusOf(ctx context.Context, userID int64) string {
	var e model.Expert
	err := r.db.WithContext(ctx).Select("status").Where("user_id = ?", userID).First(&e).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// 只读降级：查不到状态不影响登录，仅为令牌少一个可选声明。
			return ""
		}
		return ""
	}
	return e.Status
}

// ExistsByUserID 判断是否已提交过入驻申请。
func (r *ExpertRepo) ExistsByUserID(ctx context.Context, userID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Expert{}).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家档案失败: %w", err))
	}
	return count > 0, nil
}

// ============================ 写入 ============================

// Create 创建专家档案。
func (r *ExpertRepo) Create(ctx context.Context, e *model.Expert) error {
	if err := r.db.WithContext(ctx).Create(e).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("创建专家档案失败: %w", err))
	}
	return nil
}

// UpdateFields 按字段名局部更新专家档案，避免并发下整行覆盖。
func (r *ExpertRepo) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Expert{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新专家档案 %d 失败: %w", id, err))
	}
	return nil
}

// GetByID 按主键查询专家；不存在时返回 errcode.ErrExpertNotFound。
func (r *ExpertRepo) GetByID(ctx context.Context, id int64) (*model.Expert, error) {
	var e model.Expert
	if err := r.db.WithContext(ctx).First(&e, id).Error; err != nil {
		return nil, normalizeNotFound(err, errcode.ErrExpertNotFound)
	}
	return &e, nil
}

// ExpertRow 是专家档案联表用户后的展示数据。
type ExpertRow struct {
	model.Expert
	Nickname   string
	UserAvatar string
}

// GetRow 查询单个专家及其账号展示信息。
func (r *ExpertRepo) GetRow(ctx context.Context, id int64) (*ExpertRow, error) {
	var row ExpertRow
	err := r.expertQuery(ctx, ExpertFilter{}).
		Select("experts.*, users.nickname AS nickname, users.avatar AS user_avatar").
		Where("experts.id = ?", id).
		Scan(&row).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询专家失败: %w", err))
	}
	if row.ID == 0 {
		return nil, errcode.ErrExpertNotFound
	}
	return &row, nil
}

// ============================ 列表 ============================

// ExpertFilter 专家列表查询条件，零值表示该维度不过滤。
type ExpertFilter struct {
	// Status 精确匹配审核状态；前台固定传 approved。
	Status        string
	SkillID       int64
	Keyword       string
	MinPriceCents int64
	MaxPriceCents int64
	OnlineOnly    bool
	MinRating     float64
	Sort          string

	// OnlyActiveUser 只返回账号未被封禁的专家。
	//
	// 前台必须带上：封禁只改 users.status，不动专家档案（解封即恢复接单能力），
	// 不在这里过滤的话，被封的人还挂在专家列表里等着被下单。
	// 后台审核队列不能带：否则被封禁者那份仍待审核的申请会凭空消失。
	OnlyActiveUser bool
}

// List 分页查询专家。
func (r *ExpertRepo) List(ctx context.Context, f ExpertFilter, offset, limit int) ([]ExpertRow, int64, error) {
	var total int64
	if err := r.expertQuery(ctx, f).Count(&total).Error; err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家数量失败: %w", err))
	}
	if total == 0 {
		return []ExpertRow{}, 0, nil
	}

	rows := make([]ExpertRow, 0, limit)
	err := r.expertQuery(ctx, f).
		Select("experts.*, users.nickname AS nickname, users.avatar AS user_avatar").
		Order(expertOrderClause(f.Sort)).
		Offset(offset).Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, errcode.ErrDatabase.Wrap(fmt.Errorf("查询专家列表失败: %w", err))
	}
	return rows, total, nil
}

// CountByStatus 统计各审核状态的专家数，后台看板与待办入口用。
func (r *ExpertRepo) CountByStatus(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		Status string
		Total  int64
	}
	err := r.db.WithContext(ctx).Model(&model.Expert{}).
		Select("status, COUNT(*) AS total").Group("status").Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家状态失败: %w", err))
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.Total
	}
	return out, nil
}

// expertQuery 构造带过滤条件的查询。
//
// 始终联表 users：关键词需要能匹配昵称，而 Count 与 Find 必须共用同一份
// where 条件，否则会出现"总数说有 5 个、实际只返回 2 个"的翻页错乱。
func (r *ExpertRepo) expertQuery(ctx context.Context, f ExpertFilter) *gorm.DB {
	q := r.db.WithContext(ctx).
		Model(&model.Expert{}).
		Joins("LEFT JOIN users ON users.id = experts.user_id AND users.deleted_at IS NULL")

	if f.Status != "" {
		q = q.Where("experts.status = ?", f.Status)
	}
	if f.OnlyActiveUser {
		q = q.Where("users.status = ?", model.UserStatusActive)
	}
	if f.SkillID > 0 {
		// 用子查询而不是 JOIN：JOIN 会让一个专家按技能数量产生多行，
		// Count 与 Find 的口径都会跑偏。
		sub := r.db.Model(&model.ExpertSkill{}).
			Select("expert_id").
			Where("skill_id = ? AND deleted_at IS NULL", f.SkillID)
		q = q.Where("experts.id IN (?)", sub)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		p := likePattern(kw)
		q = q.Where("(users.nickname LIKE ? ESCAPE '\\' OR experts.intro LIKE ? ESCAPE '\\')", p, p)
	}
	if f.MinPriceCents > 0 {
		q = q.Where("experts.price_per_hour_cents >= ?", f.MinPriceCents)
	}
	if f.MaxPriceCents > 0 {
		q = q.Where("experts.price_per_hour_cents <= ?", f.MaxPriceCents)
	}
	if f.OnlineOnly {
		q = q.Where("experts.accepting_order = ?", true)
	}
	if f.MinRating > 0 {
		q = q.Where("experts.rating >= ?", f.MinRating)
	}
	return q
}

// expertOrderClause 返回排序语句。
//
// 所有排序都追加 id 作为末级键：口碑分相同的专家在翻页时顺序不稳定，
// 会导致同一个人重复出现或漏掉。
func expertOrderClause(sort string) string {
	switch sort {
	case "orders":
		return "experts.order_count DESC, experts.id DESC"
	case "price_asc":
		// 未定价（0）的排最后，否则"价格从低到高"会把没定价的全顶到最前。
		return "CASE WHEN experts.price_per_hour_cents = 0 THEN 1 ELSE 0 END ASC, experts.price_per_hour_cents ASC, experts.id DESC"
	case "price_desc":
		return "experts.price_per_hour_cents DESC, experts.id DESC"
	case "latest":
		return "experts.applied_at DESC, experts.id DESC"
	default: // rating
		return "experts.rating DESC, experts.order_count DESC, experts.id DESC"
	}
}

// ============================ 技能关联 ============================

// ExpertSkillRow 是专家技能的联表结果。
type ExpertSkillRow struct {
	ExpertID int64
	SkillID  int64
	Name     string
	Category string
	Level    string
	Years    int
}

// SkillsOfMany 批量查询多个专家的技能，避免列表页 N+1 查询。
func (r *ExpertRepo) SkillsOfMany(ctx context.Context, expertIDs []int64) (map[int64][]ExpertSkillRow, error) {
	out := make(map[int64][]ExpertSkillRow)
	ids := dedupeIDs(expertIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []ExpertSkillRow
	err := r.db.WithContext(ctx).
		Table("expert_skills").
		Select("expert_skills.expert_id AS expert_id, skills.id AS skill_id, "+
			"skills.name AS name, skills.category AS category, "+
			"expert_skills.level AS level, expert_skills.years AS years").
		Joins("JOIN skills ON skills.id = expert_skills.skill_id AND skills.deleted_at IS NULL").
		Where("expert_skills.expert_id IN ? AND expert_skills.deleted_at IS NULL", ids).
		Order("expert_skills.years DESC, expert_skills.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("批量查询专家技能失败: %w", err))
	}
	for _, row := range rows {
		out[row.ExpertID] = append(out[row.ExpertID], row)
	}
	return out, nil
}

// SkillsOf 查询单个专家的技能。
func (r *ExpertRepo) SkillsOf(ctx context.Context, expertID int64) ([]ExpertSkillRow, error) {
	m, err := r.SkillsOfMany(ctx, []int64{expertID})
	if err != nil {
		return nil, err
	}
	return m[expertID], nil
}

// ReplaceSkills 在一个事务内整体替换专家的技能关联。
//
// 必须用物理删除（Unscoped）：expert_skills 上 (expert_id, skill_id) 是唯一索引，
// 软删除的旧行仍然占着这个索引位，重新添加同一技能时 INSERT 会直接报重复键，
// 而调用方看起来只是"改技能失败"。知识库点赞表踩过完全相同的坑。
func (r *ExpertRepo) ReplaceSkills(ctx context.Context, expertID int64, skills []model.ExpertSkill) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("expert_id = ?", expertID).Delete(&model.ExpertSkill{}).Error; err != nil {
			return err
		}
		if len(skills) == 0 {
			return nil
		}
		for i := range skills {
			skills[i].ExpertID = expertID
		}
		return tx.Create(&skills).Error
	})
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新专家技能失败: %w", err))
	}
	return nil
}

// ============================ 财务 ============================

// TransferToFrozen 把可提现余额转入冻结（提交提现申请时）。
//
// 用带条件的原子更新：只有余额足够时 RowsAffected 才为 1，
// 从而避免"先查余额再扣减"在并发下扣成负数。
func (r *ExpertRepo) TransferToFrozen(ctx context.Context, expertID, amount int64) (bool, error) {
	if amount <= 0 {
		return false, errcode.ErrWithdrawAmountInvalid
	}
	res := r.db.WithContext(ctx).Model(&model.Expert{}).
		Where("id = ? AND balance_cents >= ?", expertID, amount).
		Updates(map[string]any{
			"balance_cents": gorm.Expr("balance_cents - ?", amount),
			"frozen_cents":  gorm.Expr("frozen_cents + ?", amount),
		})
	if res.Error != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("冻结余额失败: %w", res.Error))
	}
	return res.RowsAffected > 0, nil
}

// RestoreFromFrozen 提现被驳回时把冻结金额退回可提现余额。
func (r *ExpertRepo) RestoreFromFrozen(ctx context.Context, expertID, amount int64) error {
	if amount <= 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Expert{}).Where("id = ?", expertID).
		Updates(map[string]any{
			"balance_cents": gorm.Expr("balance_cents + ?", amount),
			// 冻结额减去金额，并用 CASE 钳到 0，防止并发下出现负数。
			"frozen_cents": gorm.Expr(
				"CASE WHEN frozen_cents < ? THEN 0 ELSE frozen_cents - ? END", amount, amount),
		}).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("退回冻结余额失败: %w", err))
	}
	return nil
}

// ConsumeFrozen 提现打款完成后扣除冻结金额。
func (r *ExpertRepo) ConsumeFrozen(ctx context.Context, expertID, amount int64) error {
	if amount <= 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Expert{}).Where("id = ?", expertID).
		UpdateColumn("frozen_cents", gorm.Expr(
			"CASE WHEN frozen_cents < ? THEN 0 ELSE frozen_cents - ? END", amount, amount)).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("扣除冻结余额失败: %w", err))
	}
	return nil
}

// AddIncome 结算到账：增加可提现余额与累计收入。
func (r *ExpertRepo) AddIncome(ctx context.Context, expertID, amount int64) error {
	if amount <= 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Expert{}).Where("id = ?", expertID).
		Updates(map[string]any{
			"balance_cents":      gorm.Expr("balance_cents + ?", amount),
			"total_income_cents": gorm.Expr("total_income_cents + ?", amount),
		}).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("增加专家收入失败: %w", err))
	}
	return nil
}
