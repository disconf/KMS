// Package dto 定义对外的请求与响应结构。
//
// 约定：模型层字段不外泄，所有出入参必须经 DTO 转换，
// 尤其是手机号、身份证号、收款账号等敏感字段必须在 DTO 层完成脱敏。
package dto

// PageQuery 是通用分页入参。
type PageQuery struct {
	Page     int `form:"page" json:"page"`
	PageSize int `form:"pageSize" json:"pageSize"`
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// Normalize 修正非法分页参数，保证 SQL 侧不会出现负数 offset 或超大 limit。
func (p *PageQuery) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = defaultPageSize
	}
	if p.PageSize > maxPageSize {
		p.PageSize = maxPageSize
	}
}

// Offset 返回 SQL OFFSET。
func (p PageQuery) Offset() int { return (p.Page - 1) * p.PageSize }

// Limit 返回 SQL LIMIT。
func (p PageQuery) Limit() int {
	if p.PageSize < 1 {
		return defaultPageSize
	}
	if p.PageSize > maxPageSize {
		return maxPageSize
	}
	return p.PageSize
}
