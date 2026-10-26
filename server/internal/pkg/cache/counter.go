package cache

import "strconv"

// parseCounter 解析计数器值，非法内容按 0 处理（不 panic）。
func parseCounter(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// formatCounter 序列化计数器值。
func formatCounter(n int64) string { return strconv.FormatInt(n, 10) }
