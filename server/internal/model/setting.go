package model

import "time"

// SettingKeyCommissionRateBP 平台抽佣基点的配置键。
const SettingKeyCommissionRateBP = "order.commission_rate_bp"

// SystemSetting 后台可调的系统配置，键值存储。
//
// 抽佣比例这类「运营要随时调」的值不适合躺在配置文件里：
// 配置文件改一次要重启服务，重启意味着下单窗口中断，而抽佣恰恰是最常被调的值。
// 值统一存字符串、由读取方负责解析——将来加布尔或枚举项不用改表结构。
//
// 刻意不嵌 Base：配置没有"软删除"的需求，键就是主键，
// 删掉一行等于退回默认值，这个语义由读取方表达（查不到就回落默认），不靠 DeletedAt。
type SystemSetting struct {
	Key       string    `gorm:"primaryKey;size:64;comment:配置键" json:"key"`
	Value     string    `gorm:"size:255;comment:配置值（字符串存储，读取方解析）" json:"value"`
	Remark    string    `gorm:"size:255;comment:说明" json:"remark"`
	UpdatedBy int64     `gorm:"comment:最后修改人用户 ID" json:"updatedBy"`
	UpdatedAt time.Time `gorm:"comment:最后修改时间" json:"updatedAt"`
}

// TableName 固定表名。
func (SystemSetting) TableName() string { return "system_settings" }
