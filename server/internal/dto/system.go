package dto

import "time"

/* ============================ 系统配置 ============================ */

// CommissionSettingView 抽佣比例配置视图。
//
// RateBP 是当前生效值（库里没设时等于 DefaultRateBP）；
// IsOverride 标记「后台改过、不再是配置文件的默认值」，
// 运营需要知道当前比例是"写死在发布物里的"还是"有人在后台调过"。
type CommissionSettingView struct {
	RateBP        int        `json:"rateBp"`
	DefaultRateBP int        `json:"defaultRateBp"`
	IsOverride    bool       `json:"isOverride"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty"`
	UpdatedBy     int64      `json:"updatedBy,omitempty"`
}

// AdminSettingOverview 后台系统配置总览。
// 目前只有一个配置项；按对象嵌套而不是拍平成若干字段，
// 以后加新配置（如提现最低额度）直接往里加成员，前端按块渲染。
type AdminSettingOverview struct {
	Commission CommissionSettingView `json:"commission"`
}

// AdminCommissionUpdateReq 修改抽佣比例。基点单位，1500 = 15%。
type AdminCommissionUpdateReq struct {
	RateBP int `json:"rateBp"`
}

/* ============================ 技能字典管理 ============================ */

// AdminSkillItem 后台技能项。ExpertCount 展示「被多少位专家使用」，
// 是运营决定能不能删它的依据——有引用的技能只能停用。
type AdminSkillItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Sort        int    `json:"sort"`
	Enabled     bool   `json:"enabled"`
	ExpertCount int64  `json:"expertCount"`
}

// AdminSkillCreateReq 新增技能。
type AdminSkillCreateReq struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Sort     int    `json:"sort"`
	Enabled  *bool  `json:"enabled"`
}

// AdminSkillUpdateReq 更新技能，全字段可选（nil 表示不改）。
type AdminSkillUpdateReq struct {
	Name     *string `json:"name"`
	Category *string `json:"category"`
	Sort     *int    `json:"sort"`
	Enabled  *bool   `json:"enabled"`
}
