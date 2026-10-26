package storage

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// Storage 是文件存储的统一接口。
type Storage interface {
	// Save 保存文件。key 由服务端生成，实现方仍需再次校验其不含越权路径。
	Save(ctx context.Context, key string, r io.Reader, size int64) error
	// Open 读取文件，私有文件下载时使用。
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete 删除文件。文件不存在不算失败（幂等）。
	Delete(ctx context.Context, key string) error
	// PublicURL 返回公开文件的访问地址；私有文件不应使用它。
	PublicURL(key string) string
	// Driver 返回驱动名，便于日志排查与接口回显。
	Driver() string
}

// Options 是构造 Storage 所需的参数。
type Options struct {
	Driver string
	// LocalDir 本地驱动的落盘根目录。
	LocalDir string
	// BaseURL 公开文件的访问前缀，末尾不带斜杠。
	BaseURL string
}

// New 按配置构造 Storage。
func New(opt Options) (Storage, error) {
	switch opt.Driver {
	case "local", "":
		return NewLocal(opt.LocalDir, opt.BaseURL)
	case "oss", "cos":
		// 对象存储需要引入厂商 SDK 并配置 AccessKey。这里返回明确错误而不是
		// 静默降级到本地盘——静默降级会让生产环境把用户文件写进容器本地磁盘，
		// 一次重启或扩容就全丢，而且不会有任何报错。
		return nil, fmt.Errorf("storage.driver=%s 尚未接入，请实现对应驱动或改用 local", opt.Driver)
	default:
		return nil, fmt.Errorf("不支持的存储驱动: %s", opt.Driver)
	}
}

// CleanKey 校验并规范化存储 key，拒绝一切越权路径。
//
// 这是防目录穿越的唯一入口，所有落盘 / 读取 / 删除都必须先过它。
func CleanKey(key string) (string, error) {
	k := strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	if k == "" {
		return "", errcode.ErrFileMissing.WithMsg("文件标识不能为空")
	}
	if strings.HasPrefix(k, "/") || strings.Contains(k, "..") {
		return "", errcode.ErrFileSignInvalid.WithMsg("文件标识不合法")
	}
	cleaned := path.Clean(k)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", errcode.ErrFileSignInvalid.WithMsg("文件标识不合法")
	}
	return cleaned, nil
}
