// Package model 定义平台的 GORM 数据模型。
//
// 两条硬约定：
//  1. 所有金额字段一律以「分」为单位的 int64 存储（字段名带 Cents 后缀），
//     杜绝浮点累加误差；对外输出时由 DTO 层转换为元。
//  2. JSON 类字段统一使用 StringList / JSONMap 类型，MySQL 落 JSON 列，
//     SQLite 落同名列（SQLite 动态类型，可正常工作）。
package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Base 是所有业务表的公共字段。
type Base struct {
	ID        int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// StringList 是字符串数组的 JSON 存储类型。
type StringList []string

// Value 实现 driver.Valuer。
func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, fmt.Errorf("序列化 StringList 失败: %w", err)
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (s *StringList) Scan(src any) error {
	if src == nil {
		*s = StringList{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("StringList 无法解析的类型: %T", src)
	}
	if len(raw) == 0 {
		*s = StringList{}
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("解析 StringList 失败: %w", err)
	}
	*s = out
	return nil
}

// GormDataType 声明列类型为 json。
func (StringList) GormDataType() string { return "json" }

// GormDBDataType 按方言给出精确列类型。
func (StringList) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	return jsonColumnType(db)
}

// JSONMap 是键值对象的 JSON 存储类型，用于审计详情等结构不固定的数据。
type JSONMap map[string]any

// Value 实现 driver.Valuer。
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	b, err := json.Marshal(map[string]any(m))
	if err != nil {
		return nil, fmt.Errorf("序列化 JSONMap 失败: %w", err)
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (m *JSONMap) Scan(src any) error {
	if src == nil {
		*m = JSONMap{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("JSONMap 无法解析的类型: %T", src)
	}
	if len(raw) == 0 {
		*m = JSONMap{}
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("解析 JSONMap 失败: %w", err)
	}
	*m = out
	return nil
}

// GormDataType 声明列类型为 json。
func (JSONMap) GormDataType() string { return "json" }

// GormDBDataType 按方言给出精确列类型。
func (JSONMap) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	return jsonColumnType(db)
}

// jsonColumnType 在 MySQL 下使用原生 JSON 类型，其余方言退化为 text。
func jsonColumnType(db *gorm.DB) string {
	switch db.Dialector.Name() {
	case "mysql":
		return "json"
	default:
		// SQLite 没有真正的 JSON 类型，text 足够且可读性更好。
		return "text"
	}
}
