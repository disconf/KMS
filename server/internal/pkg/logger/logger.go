// Package logger 基于标准库 log/slog 提供全局结构化日志，避免引入额外日志依赖。
package logger

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	mu     sync.RWMutex
	global *slog.Logger
)

// Init 初始化全局 logger。level: debug|info|warn|error；format: text|json。
func Init(level, format string, addSource bool) {
	opts := &slog.HandlerOptions{
		Level:     parseLevel(level),
		AddSource: addSource,
	}

	var h slog.Handler
	if strings.EqualFold(format, "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}

	l := slog.New(h)
	mu.Lock()
	global = l
	mu.Unlock()
	slog.SetDefault(l)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// L 返回全局 logger，未初始化时按 info/text 兜底。
func L() *slog.Logger {
	mu.RLock()
	l := global
	mu.RUnlock()
	if l != nil {
		return l
	}
	Init("info", "text", false)
	mu.RLock()
	defer mu.RUnlock()
	return global
}

// With 返回带固定字段的派生 logger。
func With(args ...any) *slog.Logger { return L().With(args...) }

// Debug 快捷方法。
func Debug(msg string, args ...any) { L().Debug(msg, args...) }

// Info 快捷方法。
func Info(msg string, args ...any) { L().Info(msg, args...) }

// Warn 快捷方法。
func Warn(msg string, args ...any) { L().Warn(msg, args...) }

// Error 快捷方法。
func Error(msg string, args ...any) { L().Error(msg, args...) }
