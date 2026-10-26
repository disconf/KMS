package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// 本文件收敛知识库各栏目 repo 里高度重复的写法。
//
// 工具 / 题库 / 资源三张表的 CRUD 与计数逻辑几乎一样，
// 各写一份的后果不是代码长，而是改一处漏两处——比如哪天要给所有
// 栏目加"逻辑删除审计"，漏掉的那张表就会静默不生效。

// existsBy 判断某列取值是否已存在（excludeID 用于更新时排除自身）。
// label 只用于错误信息，便于定位是哪张表的唯一性校验失败。
func existsBy(ctx context.Context, db *gorm.DB, model any, column, value string, excludeID int64, label string) (bool, error) {
	q := db.WithContext(ctx).Model(model).Where(column+" = ?", value)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, errcode.ErrDatabase.Wrap(fmt.Errorf("校验%s %s 唯一性失败: %w", label, column, err))
	}
	return count > 0, nil
}

// countPublished 统计指定状态下的行数，栏目角标用。
func countPublished(ctx context.Context, db *gorm.DB, model any, status, label string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(model).Where("status = ?", status).Count(&count).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计已发布%s数失败: %w", label, err))
	}
	return count, nil
}

// distinctValues 返回某列的去重非空取值。
//
// column 由调用方以字面量传入，绝不接受外部输入：
// 这里直接拼进 SQL，一旦被请求参数污染就是注入漏洞。
func distinctValues(ctx context.Context, db *gorm.DB, model any, column, status, label string) ([]string, error) {
	q := db.WithContext(ctx).Model(model).
		Distinct().Where(column + " <> ''")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []string
	if err := q.Pluck(column, &out).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询%s %s 取值失败: %w", label, column, err))
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// updateFieldsByID 按字段名局部更新，len(fields) 为 0 时直接返回。
func updateFieldsByID(ctx context.Context, db *gorm.DB, model any, id int64, fields map[string]any, label string) error {
	if len(fields) == 0 {
		return nil
	}
	err := db.WithContext(ctx).Model(model).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新%s %d 失败: %w", label, id, err))
	}
	return nil
}

// softDeleteByID 软删除一行。
func softDeleteByID(ctx context.Context, db *gorm.DB, model any, id int64, label string) error {
	if err := db.WithContext(ctx).Delete(model, id).Error; err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("删除%s %d 失败: %w", label, id, err))
	}
	return nil
}

// countByCategoryID 统计指定分类下的行数，删除分类前的前置校验用。
func countByCategoryID(ctx context.Context, db *gorm.DB, model any, categoryID int64, label string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(model).
		Where("category_id = ?", categoryID).Count(&count).Error
	if err != nil {
		return 0, errcode.ErrDatabase.Wrap(fmt.Errorf("统计分类 %d 的%s数失败: %w", categoryID, label, err))
	}
	return count, nil
}

// countGroupByCategory 统计各分类下的行数，status 为空表示不过滤状态。
func countGroupByCategory(ctx context.Context, db *gorm.DB, model any, status, label string) (map[int64]int64, error) {
	q := db.WithContext(ctx).Model(model).
		Select("category_id, COUNT(*) AS total")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []struct {
		CategoryID int64
		Total      int64
	}
	if err := q.Group("category_id").Scan(&rows).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计分类%s数失败: %w", label, err))
	}
	counts := make(map[int64]int64, len(rows))
	for _, row := range rows {
		counts[row.CategoryID] = row.Total
	}
	return counts, nil
}

// incrementColumn 让某计数列按 delta 自增。
//
// 用 SQL 表达式而非"读出再写回"：读改写在高并发下会丢计数，
// 下载量、浏览量这类被频繁更新的列尤其明显。
func incrementColumn(ctx context.Context, db *gorm.DB, model any, id int64, column string, delta int64, label string) error {
	err := db.WithContext(ctx).Model(model).Where("id = ?", id).
		UpdateColumn(column, gorm.Expr(column+" + ?", delta)).Error
	if err != nil {
		return errcode.ErrDatabase.Wrap(fmt.Errorf("更新%s失败: %w", label, err))
	}
	return nil
}

// isDuplicateKeyError 判断错误是否来自唯一索引冲突。
//
// 为什么靠字符串匹配而不是错误码：GORM 不把驱动错误归一化，
// MySQL 给的是 Error 1062，SQLite 给的是 "UNIQUE constraint failed"，
// 两边没有共同的结构化标识。所以这里把两边的特征串都认一遍。
//
// 有了它，「一单一评」这类规则就能交给唯一索引来守：并发下两次提交
// 也只有一次能插入成功，另一次拿到一个明确的业务错误，而不是两条评价都写进去。
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"duplicate entry",              // MySQL
		"unique constraint failed",     // SQLite
		"error 1062",                   // MySQL 驱动原始码
		"duplicate key value violates", // PostgreSQL（预留，便于将来换库）
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
