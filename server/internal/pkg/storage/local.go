package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// Local 是本地磁盘存储，开发环境默认使用。
//
// 生产环境务必换成对象存储：容器本地磁盘没有持久性，扩缩容或重建即丢文件。
type Local struct {
	root    string
	baseURL string
}

// NewLocal 创建本地磁盘存储并确保根目录存在。
func NewLocal(root, baseURL string) (*Local, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("本地存储目录不能为空")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析存储目录失败: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}
	return &Local{root: abs, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

// Driver 实现 Storage。
func (l *Local) Driver() string { return "local" }

// Root 返回存储根目录，路由层挂载静态服务时使用。
func (l *Local) Root() string { return l.root }

// Save 落盘。
//
// 先写同目录下的临时文件再 rename：这样即使写到一半失败，
// 也不会有一个"看起来完整"的半截文件被后续请求读到。
func (l *Local) Save(_ context.Context, key string, r io.Reader, _ int64) error {
	full, err := l.fullPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return errcode.ErrUploadFailed.Wrap(fmt.Errorf("创建目录失败: %w", err))
	}

	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return errcode.ErrUploadFailed.Wrap(fmt.Errorf("创建临时文件失败: %w", err))
	}
	tmpName := tmp.Name()
	// rename 成功后临时文件已不存在，此处的删除对成功路径是空操作。
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return errcode.ErrUploadFailed.Wrap(fmt.Errorf("写入文件失败: %w", err))
	}
	if err := tmp.Close(); err != nil {
		return errcode.ErrUploadFailed.Wrap(fmt.Errorf("关闭文件失败: %w", err))
	}
	// 上传的文件不应带可执行位，显式收紧权限。
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return errcode.ErrUploadFailed.Wrap(fmt.Errorf("设置文件权限失败: %w", err))
	}
	if err := os.Rename(tmpName, full); err != nil {
		return errcode.ErrUploadFailed.Wrap(fmt.Errorf("保存文件失败: %w", err))
	}
	return nil
}

// Open 读取文件。
func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := l.fullPath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errcode.ErrFileMissing.WithMsg("文件不存在或已被删除")
		}
		return nil, errcode.ErrUploadFailed.Wrap(err)
	}
	return f, nil
}

// Delete 删除文件，文件不存在视为成功。
func (l *Local) Delete(_ context.Context, key string) error {
	full, err := l.fullPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return errcode.ErrUploadFailed.Wrap(fmt.Errorf("删除文件失败: %w", err))
	}
	return nil
}

// PublicURL 返回公开访问地址。
func (l *Local) PublicURL(key string) string {
	k, err := CleanKey(key)
	if err != nil {
		return ""
	}
	return l.baseURL + "/uploads/" + k
}

// fullPath 把 key 解析为绝对路径，并确保结果始终落在存储根目录内。
func (l *Local) fullPath(key string) (string, error) {
	k, err := CleanKey(key)
	if err != nil {
		return "", err
	}
	full := filepath.Join(l.root, filepath.FromSlash(k))
	// 双保险：CleanKey 已拒绝 ..，这里再确认一次前缀。
	// 将来若有人改动 CleanKey 引入漏洞，这一层还能兜住越权读写。
	if !strings.HasPrefix(full, l.root+string(os.PathSeparator)) {
		return "", errcode.ErrFileSignInvalid.WithMsg("文件路径越权")
	}
	return full, nil
}
