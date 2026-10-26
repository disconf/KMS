// Package bootstrap 负责把配置变成可用的运行时依赖（数据库、缓存、种子数据）。
package bootstrap

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/model"
)

// OpenDB 按配置打开数据库连接并完成迁移。
func OpenDB(cfg *config.Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger:                                   newGormLogger(cfg.Database.LogLevel),
		NowFunc:                                  func() time.Time { return time.Now().Local() },
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	var (
		db  *gorm.DB
		err error
	)

	switch cfg.Database.Driver {
	case "mysql":
		db, err = gorm.Open(mysql.Open(cfg.Database.DSN), gormCfg)
		if err != nil {
			return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
		}
	case "sqlite":
		if dir := filepath.Dir(cfg.Database.SQLitePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("创建 SQLite 数据目录失败: %w", err)
			}
		}
		dsn := buildSQLiteDSN(cfg.Database.SQLitePath)
		db, err = gorm.Open(sqlite.Open(dsn), gormCfg)
		if err != nil {
			return nil, fmt.Errorf("连接 SQLite 失败: %w", err)
		}
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Database.Driver)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接池失败: %w", err)
	}

	// SQLite 写锁是库级锁，连接数必须收敛为 1，否则并发写会报 database is locked。
	if cfg.Database.Driver == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
		sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	}
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime.Std())

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库探活失败: %w", err)
	}

	if cfg.Database.AutoMigrate {
		if err := model.AutoMigrate(db); err != nil {
			return nil, err
		}
	}
	return db, nil
}

// buildSQLiteDSN 追加 WAL 与忙碌超时，降低本地并发读写失败概率。
func buildSQLiteDSN(path string) string {
	if strings.Contains(path, "?") {
		return path
	}
	return path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
}

// gormLogWriter 把 GORM 日志转接到 slog，统一输出格式。
type gormLogWriter struct{}

func (gormLogWriter) Printf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(strings.TrimSuffix(format, "\n"), args...), slog.String("component", "gorm"))
}

func newGormLogger(level string) gormlogger.Interface {
	var lv gormlogger.LogLevel
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "silent":
		lv = gormlogger.Silent
	case "error":
		lv = gormlogger.Error
	case "info":
		lv = gormlogger.Info
	default:
		lv = gormlogger.Warn
	}

	return gormlogger.New(gormLogWriter{}, gormlogger.Config{
		SlowThreshold:             300 * time.Millisecond,
		LogLevel:                  lv,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
}
