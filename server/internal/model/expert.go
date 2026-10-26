package model

import "time"

// 专家审核状态。
const (
	ExpertStatusPending   = "pending"   // 待审核
	ExpertStatusApproved  = "approved"  // 审核通过，可接单
	ExpertStatusRejected  = "rejected"  // 审核驳回
	ExpertStatusSuspended = "suspended" // 已停牌
)

// 专家服务方式。
const (
	ServiceModeIM     = "im"     // 图文
	ServiceModeVoice  = "voice"  // 语音
	ServiceModeVideo  = "video"  // 视频
	ServiceModeScreen = "screen" // 视频 + 屏幕共享
)

// Expert 专家档案，与 users 一对一。
type Expert struct {
	Base

	UserID int64 `gorm:"uniqueIndex;comment:关联用户 ID" json:"userId"`

	// 实名信息。方案第九节要求敏感信息脱敏：接口层不返回身份证号原文。
	RealName  string     `gorm:"size:64;comment:真实姓名" json:"realName"`
	IDCardNo  string     `gorm:"size:64;comment:身份证号（接口层脱敏输出）" json:"-"`
	CertFiles StringList `gorm:"comment:资质证书文件列表" json:"certFiles"`

	// 对外展示
	Intro      string `gorm:"size:2000;comment:个人简介" json:"intro"`
	Experience string `gorm:"size:2000;comment:工作经历" json:"experience"`
	Avatar     string `gorm:"size:512;comment:主页头像（可与用户头像不同）" json:"avatar"`

	// 定价（单位：分）
	PricePerHourCents int64 `gorm:"default:0;comment:按小时定价（分）" json:"pricePerHourCents"`
	PricePerTaskCents int64 `gorm:"default:0;comment:按次定价（分）" json:"pricePerTaskCents"`

	// 服务能力
	ServiceModes   StringList `gorm:"comment:支持的服务方式" json:"serviceModes"`
	AcceptingOrder bool       `gorm:"default:false;comment:是否开放接单" json:"acceptingOrder"`

	// 信誉数据（由订单/评价流水汇总，不手工维护）
	Rating         float64 `gorm:"type:decimal(3,2);default:5;comment:综合评分 0-5" json:"rating"`
	OrderCount     int64   `gorm:"default:0;comment:累计完成订单数" json:"orderCount"`
	GoodRate       float64 `gorm:"type:decimal(5,2);default:100;comment:好评率（%）" json:"goodRate"`
	ResolveRate    float64 `gorm:"type:decimal(5,2);default:0;comment:解决率（%）" json:"resolveRate"`
	AvgResponseSec int64   `gorm:"default:0;comment:平均响应秒数" json:"avgResponseSec"`

	// 财务
	BalanceCents     int64 `gorm:"default:0;comment:可提现余额（分）" json:"balanceCents"`
	FrozenCents      int64 `gorm:"default:0;comment:冻结中金额（分）" json:"frozenCents"`
	TotalIncomeCents int64 `gorm:"default:0;comment:累计收入（分）" json:"totalIncomeCents"`

	// 审核
	Status      string     `gorm:"size:16;index;default:pending;comment:审核状态" json:"status"`
	AuditRemark string     `gorm:"size:500;comment:审核备注/驳回原因" json:"auditRemark"`
	AuditedAt   *time.Time `gorm:"comment:审核时间" json:"auditedAt"`
	AuditedBy   int64      `gorm:"comment:审核人用户 ID" json:"-"`
	AppliedAt   time.Time  `gorm:"comment:提交申请时间" json:"appliedAt"`
}

// TableName 固定表名。
func (Expert) TableName() string { return "experts" }

// IsApproved 判断专家是否已通过审核。
func (e *Expert) IsApproved() bool { return e.Status == ExpertStatusApproved }

// Skill 技能标签字典，由平台运营维护。
type Skill struct {
	Base

	Name     string `gorm:"size:64;uniqueIndex;comment:技能名称" json:"name"`
	Category string `gorm:"size:64;index;comment:所属类目，如 K8s/数据库/网络" json:"category"`
	Sort     int    `gorm:"default:0;comment:排序权重" json:"sort"`
	Enabled  bool   `gorm:"default:true;comment:是否启用" json:"enabled"`
}

// TableName 固定表名。
func (Skill) TableName() string { return "skills" }

// 专家技能等级。
const (
	SkillLevelJunior = "junior" // 熟练
	SkillLevelSenior = "senior" // 精通
	SkillLevelExpert = "expert" // 专家
)

// ExpertSkill 专家与技能的关联，附带年限与等级，直接影响搜索权重。
type ExpertSkill struct {
	Base

	ExpertID int64  `gorm:"uniqueIndex:uk_expert_skill;index;comment:专家 ID" json:"expertId"`
	SkillID  int64  `gorm:"uniqueIndex:uk_expert_skill;index;comment:技能 ID" json:"skillId"`
	Level    string `gorm:"size:16;default:senior;comment:等级" json:"level"`
	Years    int    `gorm:"default:0;comment:经验年限" json:"years"`
}

// TableName 固定表名。
func (ExpertSkill) TableName() string { return "expert_skills" }
