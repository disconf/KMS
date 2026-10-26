package dto

import "time"

// ============================ 通用 ============================

// FileRef 是文件引用：key 是稳定标识，url 是可直接访问的地址。
// 私有文件（资质证书、问题附件）的 url 带签名与有效期。
type FileRef struct {
	Key      string `json:"key"`
	URL      string `json:"url"`
	ExpireAt int64  `json:"expireAt,omitempty"`
}

// ============================ 入驻申请 ============================

// ExpertSkillReq 申请时提交的单个技能。
type ExpertSkillReq struct {
	SkillID int64  `json:"skillId" binding:"required"`
	Level   string `json:"level" binding:"omitempty,oneof=junior senior expert"`
	Years   int    `json:"years" binding:"min=0,max=60"`
}

// ExpertApplyReq 专家入驻申请。
type ExpertApplyReq struct {
	RealName string `json:"realName" binding:"required"`
	// IDCardNo 首次申请必填；被驳回或停牌后重新提交可以留空，沿用已登记的号码。
	//
	// 接口从来不回显完整证件号（出参只有 maskedIdCard），所以强制必填等于
	// 让同一个人把 18 位号码重抄一遍——抄错概率比"少一次校验"更大。
	// 是否真的有底可沿，由 service 层按库里是否有记录判定，不在这里放行。
	IDCardNo string `json:"idCardNo"`
	// CertFiles 是上传接口返回的 key 列表，不是 URL。
	CertFiles  []string `json:"certFiles" binding:"required,min=1,max=10"`
	Intro      string   `json:"intro" binding:"required"`
	Experience string   `json:"experience"`
	Avatar     string   `json:"avatar"`

	Skills       []ExpertSkillReq `json:"skills" binding:"required,min=1,max=5"`
	ServiceModes []string         `json:"serviceModes" binding:"required,min=1"`

	PricePerHourCents int64 `json:"pricePerHourCents"`
	PricePerTaskCents int64 `json:"pricePerTaskCents"`
}

// ExpertSkillItem 技能展示项。
type ExpertSkillItem struct {
	SkillID  int64  `json:"skillId"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Level    string `json:"level"`
	Years    int    `json:"years"`
}

// ExpertApplyStatus 入驻申请状态。
//
// 被驳回时回显已填内容，让用户改一改重新提交，而不是从零重填一遍。
type ExpertApplyStatus struct {
	Status       string     `json:"status"`
	AuditRemark  string     `json:"auditRemark"`
	RealName     string     `json:"realName"`
	MaskedIDCard string     `json:"maskedIdCard"`
	AppliedAt    *time.Time `json:"appliedAt"`
	AuditedAt    *time.Time `json:"auditedAt"`
	// Draft 仅在被驳回（rejected）时返回。
	Draft *ExpertApplyDraft `json:"draft,omitempty"`
}

// ExpertApplyDraft 驳回后可复用的申请内容。
// 故意不含身份证号明文：接口出参一律不返回完整证件号。
type ExpertApplyDraft struct {
	RealName          string            `json:"realName"`
	Intro             string            `json:"intro"`
	Experience        string            `json:"experience"`
	Avatar            string            `json:"avatar"`
	Skills            []ExpertSkillItem `json:"skills"`
	ServiceModes      []string          `json:"serviceModes"`
	PricePerHourCents int64             `json:"pricePerHourCents"`
	PricePerTaskCents int64             `json:"pricePerTaskCents"`
	CertFiles         []FileRef         `json:"certFiles"`
}

// ============================ 公开列表与详情 ============================

// ExpertListQuery 专家列表查询条件。
type ExpertListQuery struct {
	PageQuery

	SkillID       int64   `form:"skillId"`
	Keyword       string  `form:"keyword"`
	MinPriceCents int64   `form:"minPriceCents"`
	MaxPriceCents int64   `form:"maxPriceCents"`
	OnlineOnly    bool    `form:"onlineOnly"`
	MinRating     float64 `form:"minRating"`
	// Sort 取值 rating（默认）/ orders / price_asc / price_desc / latest。
	Sort string `form:"sort" binding:"omitempty,oneof=rating orders price_asc price_desc latest"`
}

// ExpertCard 专家卡片，列表与横滑位共用。
//
// 字段顺序即前端展示优先级，与前端设计文档 3.4 节一致。
type ExpertCard struct {
	ID                int64             `json:"id"`
	UserID            int64             `json:"userId"`
	Nickname          string            `json:"nickname"`
	Avatar            string            `json:"avatar"`
	RealNameVerified  bool              `json:"realNameVerified"`
	Skills            []ExpertSkillItem `json:"skills"`
	Rating            float64           `json:"rating"`
	OrderCount        int64             `json:"orderCount"`
	GoodRate          float64           `json:"goodRate"`
	PricePerHourCents int64             `json:"pricePerHourCents"`
	PricePerTaskCents int64             `json:"pricePerTaskCents"`
	AcceptingOrder    bool              `json:"acceptingOrder"`
	ServiceModes      []string          `json:"serviceModes"`
}

// ExpertDetail 专家详情。
// 刻意不含余额、累计收入等财务字段——那是只有本人能看的数据。
type ExpertDetail struct {
	ExpertCard

	Intro          string     `json:"intro"`
	Experience     string     `json:"experience"`
	ResolveRate    float64    `json:"resolveRate"`
	AvgResponseSec int64      `json:"avgResponseSec"`
	AppliedAt      *time.Time `json:"appliedAt"`
	JoinedAt       time.Time  `json:"joinedAt"`
}

// ============================ 公开评价 ============================

// ExpertReviewItem 专家主页展示的单条评价。
//
// 刻意不含 orderId / userId：评价区是公开页面，订单号能反查下单人，
// userId 能点进个人主页——这两样都不该出现在这里。匿名评价连昵称头像一起抹掉，
// 只留「匿名用户」这个身份，否则"匿名"等于只藏了个名字、头像照样认得出人。
type ExpertReviewItem struct {
	Nickname           string     `json:"nickname"`
	Avatar             string     `json:"avatar"`
	Score              int        `json:"score"`
	ProfessionalScore  int        `json:"professionalScore"`
	SpeedScore         int        `json:"speedScore"`
	CommunicationScore int        `json:"communicationScore"`
	Content            string     `json:"content"`
	Recommend          bool       `json:"recommend"`
	Anonymous          bool       `json:"anonymous"`
	Reply              string     `json:"reply,omitempty"`
	RepliedAt          *time.Time `json:"repliedAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

// ScoreBucket 星级分布的一档。固定下发 5..1 五档，没有数据的那档 count 为 0。
type ScoreBucket struct {
	Score int   `json:"score"`
	Count int64 `json:"count"`
}

// ExpertReviewSummary 评价汇总，详情页顶部与分布条共用。
//
// AvgScore 在没有任何评价时为 0；前端据此区分"新专家"与"评分很差"——
// 0 分和"还没人评"在展示上是两件事，不能让新入驻的专家看起来像被差评了。
type ExpertReviewSummary struct {
	Total        int64         `json:"total"`
	AvgScore     float64       `json:"avgScore"`
	GoodRate     float64       `json:"goodRate"`
	Distribution []ScoreBucket `json:"distribution"`
}

// ExpertReviewPage 公开评价分页。
type ExpertReviewPage struct {
	Summary  ExpertReviewSummary `json:"summary"`
	List     []ExpertReviewItem  `json:"list"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

// ============================ 专家端（自身） ============================

// ExpertServiceUpdateReq 服务设置更新，指针字段实现"未传即不改"。
type ExpertServiceUpdateReq struct {
	Skills            *[]ExpertSkillReq `json:"skills"`
	ServiceModes      *[]string         `json:"serviceModes"`
	PricePerHourCents *int64            `json:"pricePerHourCents"`
	PricePerTaskCents *int64            `json:"pricePerTaskCents"`
	AcceptingOrder    *bool             `json:"acceptingOrder"`
}

// ExpertProfileUpdateReq 对外主页内容更新。
type ExpertProfileUpdateReq struct {
	Intro      *string `json:"intro"`
	Experience *string `json:"experience"`
	Avatar     *string `json:"avatar"`
}

// ExpertDashboard 工作台概览。
//
// 除统计数字外，一并下发「本人主页与报价」的当前值：前端的编辑器需要
// 初始数据，而公开详情接口是按专家 ID 取的，本人手里只有 userID——
// 让前端先去列表里搜自己再取详情，是最容易被分页与筛选坑掉的做法。
type ExpertDashboard struct {
	OrderCount       int64   `json:"orderCount"`
	PendingAccept    int64   `json:"pendingAccept"`
	MonthIncomeCents int64   `json:"monthIncomeCents"`
	Rating           float64 `json:"rating"`
	GoodRate         float64 `json:"goodRate"`
	BalanceCents     int64   `json:"balanceCents"`
	FrozenCents      int64   `json:"frozenCents"`
	TotalIncomeCents int64   `json:"totalIncomeCents"`
	AcceptingOrder   bool    `json:"acceptingOrder"`
	Status           string  `json:"status"`

	// 以下为本人资料与报价，供工作台表单回填。
	ExpertID          int64             `json:"expertId"`
	RealName          string            `json:"realName"`
	MaskedIDCard      string            `json:"maskedIdCard"`
	Avatar            string            `json:"avatar"`
	Intro             string            `json:"intro"`
	Experience        string            `json:"experience"`
	Skills            []ExpertSkillItem `json:"skills"`
	ServiceModes      []string          `json:"serviceModes"`
	PricePerHourCents int64             `json:"pricePerHourCents"`
	PricePerTaskCents int64             `json:"pricePerTaskCents"`
}

// ExpertWallet 收入钱包。
type ExpertWallet struct {
	BalanceCents         int64            `json:"balanceCents"`
	FrozenCents          int64            `json:"frozenCents"`
	TotalIncomeCents     int64            `json:"totalIncomeCents"`
	PendingWithdrawCents int64            `json:"pendingWithdrawCents"`
	Items                []SettlementItem `json:"items"`
	Withdrawals          []WithdrawalItem `json:"withdrawals"`
}

// SettlementItem 收入明细项。
type SettlementItem struct {
	ID                int64      `json:"id"`
	SettlementNo      string     `json:"settlementNo"`
	OrderID           int64      `json:"orderId"`
	OrderNo           string     `json:"orderNo"`
	AmountCents       int64      `json:"amountCents"`
	CommissionCents   int64      `json:"commissionCents"`
	ExpertAmountCents int64      `json:"expertAmountCents"`
	Status            string     `json:"status"`
	SettledAt         *time.Time `json:"settledAt"`
	CreatedAt         time.Time  `json:"createdAt"`
}

// WithdrawalItem 提现记录项。
type WithdrawalItem struct {
	ID              int64      `json:"id"`
	WithdrawalNo    string     `json:"withdrawalNo"`
	AmountCents     int64      `json:"amountCents"`
	FeeCents        int64      `json:"feeCents"`
	ActualCents     int64      `json:"actualCents"`
	AccountType     string     `json:"accountType"`
	AccountName     string     `json:"accountName"`
	AccountNoMasked string     `json:"accountNoMasked"`
	Status          string     `json:"status"`
	Remark          string     `json:"remark"`
	CreatedAt       time.Time  `json:"createdAt"`
	PaidAt          *time.Time `json:"paidAt"`
}

// WithdrawalCreateReq 提现申请。
type WithdrawalCreateReq struct {
	AmountCents int64  `json:"amountCents" binding:"required"`
	AccountType string `json:"accountType" binding:"required,oneof=bank alipay wechat"`
	AccountName string `json:"accountName" binding:"required"`
	AccountNo   string `json:"accountNo" binding:"required"`
}

// AdminWithdrawalListQuery 后台提现列表查询。
type AdminWithdrawalListQuery struct {
	PageQuery

	Status  string `form:"status" binding:"omitempty,oneof=pending approved paid rejected failed"`
	Keyword string `form:"keyword"`
}

// AdminWithdrawalItem 后台提现列表项。
//
// 比专家自己看到的多出三样：专家的实名与账号状态、他当前的余额/冻结额、审核痕迹。
// 审核时要判的是「收款人姓名对不对得上实名」「这笔钱是不是他全部余额」，
// 光有一张提现单是判不了的。
type AdminWithdrawalItem struct {
	WithdrawalItem

	ExpertID      int64      `json:"expertId"`
	ExpertName    string     `json:"expertName"`
	ExpertNick    string     `json:"expertNick"`
	ExpertStatus  string     `json:"expertStatus"`
	ExpertBalance int64      `json:"expertBalanceCents"`
	ExpertFrozen  int64      `json:"expertFrozenCents"`
	ReviewedAt    *time.Time `json:"reviewedAt"`
	ReviewedBy    int64      `json:"reviewedBy"`
}

// WithdrawalReviewReq 提现审核。
//
// 刻意用 action 而不是 approved:false/true：布尔字段缺省就是 false，
// 请求体少一个字段就会把「想通过」变成「驳回」，这种默认值事故在钱上不能发生。
type WithdrawalReviewReq struct {
	Action string `json:"action" binding:"required,oneof=approve reject"`
	Remark string `json:"remark"`
}

// WithdrawalPaidReq 标记打款完成。流水号写进备注便于对账。
type WithdrawalPaidReq struct {
	Remark string `json:"remark"`
}

// ============================ 后台 ============================

// AdminExpertListQuery 后台专家审核列表查询。
type AdminExpertListQuery struct {
	PageQuery

	Status  string `form:"status" binding:"omitempty,oneof=pending approved rejected suspended"`
	Keyword string `form:"keyword"`
}

// AdminExpertItem 后台列表项，比前台多出审核态与实名信息。
type AdminExpertItem struct {
	ExpertCard

	RealName     string     `json:"realName"`
	MaskedIDCard string     `json:"maskedIdCard"`
	Status       string     `json:"status"`
	AuditRemark  string     `json:"auditRemark"`
	AppliedAt    *time.Time `json:"appliedAt"`
	AuditedAt    *time.Time `json:"auditedAt"`
}

// AdminExpertDetail 后台审核详情，含资质材料（签名地址）。
type AdminExpertDetail struct {
	AdminExpertItem

	Intro      string    `json:"intro"`
	Experience string    `json:"experience"`
	CertFiles  []FileRef `json:"certFiles"`
}

// ExpertAuditReq 审核操作。
type ExpertAuditReq struct {
	Approved bool   `json:"approved"`
	Remark   string `json:"remark"`
}

// SkillItem 技能字典项。
type SkillItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Sort        int    `json:"sort"`
	ExpertCount int64  `json:"expertCount"`
}

// ============================ 长度与金额约束 ============================
//
// 集中在此处，service 层校验与接口文档共用一套数字，避免两边写岔。

const (
	realNameMinLen = 2
	realNameMaxLen = 32
	// idCardLen 二代身份证固定 18 位。
	idCardLen = 18

	introMinLen      = 20
	introMaxLen      = 2000
	experienceMaxLen = 2000
	// 头像长度上限复用 user.go 的 avatarMaxLen，两处都指向同一个上传场景，
	// 各写一份迟早会跑偏，这里刻意不再声明。

	maxSkillsPerExpert = 5
	maxCertFiles       = 10

	// 定价上下限（分）。0 表示未定价，是允许的（可后补）。
	minPriceCents = 100      // 1 元
	maxPriceCents = 10000000 // 10 万元

	// 提现下限（分），低于 10 元不允许提现，否则手续费都不够。
	minWithdrawCents = 1000
)

// ExpertLimits 返回专家资料各字段长度上限。
func ExpertLimits() (realNameMin, realNameMax, introMin, introMax, experienceMax, avatarMax int) {
	return realNameMinLen, realNameMaxLen, introMinLen, introMaxLen, experienceMaxLen, avatarMaxLen
}

// ExpertSkillLimits 返回技能个数与证书个数上限。
func ExpertSkillLimits() (skills, certs int) {
	return maxSkillsPerExpert, maxCertFiles
}

// PriceLimits 返回定价上下限（分）。
func PriceLimits() (min, max int64) { return minPriceCents, maxPriceCents }

// MinWithdrawCents 返回提现下限（分）。
func MinWithdrawCents() int64 { return minWithdrawCents }

// IDCardLen 返回身份证号长度。
func IDCardLen() int { return idCardLen }
