package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/storage"
)

const (
	// fileHeadSize 类型探测读取的字节数。
	// 512 字节足以覆盖现有全部魔数，也够判断内容是否像纯文本。
	fileHeadSize = 512
	// privateURLTTL 私有文件签名的默认有效期。
	// 太短会让用户停在页面上图片就失效，太长则削弱权限回收能力。
	privateURLTTL = 30 * time.Minute
)

// UploadService 处理文件上传：场景校验、真实类型识别、落盘与私有链接签名。
type UploadService struct {
	store  storage.Storage
	secret string
	// globalMaxBytes 全局兜底上限，与场景上限取小值生效。
	globalMaxBytes int64
}

// NewUploadService 创建 UploadService。
func NewUploadService(store storage.Storage, signSecret string, maxSizeMB int) *UploadService {
	if maxSizeMB <= 0 {
		maxSizeMB = 20
	}
	return &UploadService{
		store:          store,
		secret:         signSecret,
		globalMaxBytes: int64(maxSizeMB) << 20,
	}
}

// Driver 返回当前存储驱动名。
func (s *UploadService) Driver() string { return s.store.Driver() }

// Scenes 返回全部上传场景说明。
func (s *UploadService) Scenes() []dto.UploadSceneItem {
	policies := storage.AllPolicies()
	items := make([]dto.UploadSceneItem, 0, len(policies))
	for _, p := range policies {
		mimes := make([]string, 0, len(p.MIMEs))
		for m := range p.MIMEs {
			mimes = append(mimes, m)
		}
		sort.Strings(mimes)
		items = append(items, dto.UploadSceneItem{
			Scene:     string(p.Scene),
			Desc:      p.Desc,
			MaxSizeMB: float64(s.limitOf(p)) / (1 << 20),
			Mimes:     mimes,
			Private:   p.Private,
			Accept:    strings.Join(mimes, ","),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Scene < items[j].Scene })
	return items
}

// Upload 校验并保存文件。
func (s *UploadService) Upload(ctx context.Context, sceneRaw string, file *multipart.FileHeader) (*dto.UploadResult, error) {
	policy, ok := storage.LookupPolicy(storage.Scene(strings.TrimSpace(sceneRaw)))
	if !ok {
		return nil, errcode.ErrSceneInvalid.WithMsg("未知的上传场景，请先调用 /upload/scenes 获取可用场景")
	}
	if file == nil || file.Size <= 0 {
		return nil, errcode.ErrFileMissing
	}
	if limit := s.limitOf(policy); file.Size > limit {
		return nil, errcode.ErrFileTooLarge.WithMsg(fmt.Sprintf("该场景文件上限为 %s", humanSize(limit)))
	}

	src, err := file.Open()
	if err != nil {
		return nil, errcode.ErrUploadFailed.Wrap(fmt.Errorf("读取上传文件失败: %w", err))
	}
	defer func() { _ = src.Close() }()

	// 只读文件头做类型判断，绝不把整个文件读进内存。
	head := make([]byte, fileHeadSize)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, errcode.ErrUploadFailed.Wrap(fmt.Errorf("读取文件头失败: %w", err))
	}
	head = head[:n]

	mime := storage.DetectMIME(head)
	if mime == "" && storage.LooksLikeText(head) {
		// 日志类文件没有魔数，只能靠"不含控制字符"判定为文本。
		mime = "text/plain"
	}
	if mime == "" {
		return nil, errcode.ErrFileTypeNotAllowed.WithMsg("无法识别的文件类型")
	}
	if !policy.Allow(mime) {
		return nil, errcode.ErrFileTypeNotAllowed.WithMsg(fmt.Sprintf("该场景仅支持：%s", policy.Desc))
	}

	key := policy.BuildKey(mime, idgen.NumericCode(16))
	// 已消费的文件头必须拼回去，否则落盘内容会缺前 512 字节。
	reader := io.MultiReader(bytes.NewReader(head), src)
	if err := s.store.Save(ctx, key, reader, file.Size); err != nil {
		return nil, err
	}
	return s.buildResult(policy, key, file.Filename, file.Size, mime), nil
}

// SignedURL 为私有文件生成带过期时间的访问地址。
func (s *UploadService) SignedURL(key string) (string, int64) {
	k, err := storage.CleanKey(key)
	if err != nil {
		return "", 0
	}
	expireAt := time.Now().Add(privateURLTTL).Unix()
	sig := storage.Sign(s.secret, k, expireAt)
	return fmt.Sprintf("/api/v1/files?key=%s&e=%d&s=%s", url.QueryEscape(k), expireAt, sig), expireAt
}

// OpenPrivate 校验签名后返回文件内容与 Content-Type。
func (s *UploadService) OpenPrivate(ctx context.Context, key, expireRaw, sig string) (io.ReadCloser, string, error) {
	k, err := storage.CleanKey(key)
	if err != nil {
		return nil, "", err
	}
	if err := storage.Verify(s.secret, k, expireRaw, sig); err != nil {
		return nil, "", err
	}
	rc, err := s.store.Open(ctx, k)
	if err != nil {
		return nil, "", err
	}
	return rc, contentTypeOf(k), nil
}

// PublicURL 返回公开文件地址。
func (s *UploadService) PublicURL(key string) string { return s.store.PublicURL(key) }

// UploadedFile 是已存储文件的实测元信息。
type UploadedFile struct {
	Key    string
	Size   int64
	SHA256 string
	MIME   string
	Ext    string
}

// Inspect 读取已存储文件的真实元信息：大小、SHA256、类型、扩展名。
//
// 为什么不信任客户端上传时返回的 size：那是 multipart 声明的值，
// 客户端可以随便改。资源下载要展示大小并给出校验和，这两项必须是
// 从落盘内容里实测出来的，否则"校验和"就成了摆设。
//
// 全程流式计算，不把文件读进内存——资源包上限 50MB，
// 并发几个就能把内存打满。
func (s *UploadService) Inspect(ctx context.Context, key string) (*UploadedFile, error) {
	k, err := storage.CleanKey(key)
	if err != nil {
		return nil, err
	}
	rc, err := s.store.Open(ctx, k)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	head := make([]byte, fileHeadSize)
	n, err := io.ReadFull(rc, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, errcode.ErrUploadFailed.Wrap(fmt.Errorf("读取文件头失败: %w", err))
	}
	head = head[:n]

	hasher := sha256.New()
	if _, err := hasher.Write(head); err != nil {
		return nil, errcode.ErrUploadFailed.Wrap(fmt.Errorf("计算文件校验和失败: %w", err))
	}
	total := int64(n)
	rest, err := io.Copy(hasher, rc)
	if err != nil {
		return nil, errcode.ErrUploadFailed.Wrap(fmt.Errorf("计算文件校验和失败: %w", err))
	}
	total += rest

	mime := storage.DetectMIME(head)
	if mime == "" && storage.LooksLikeText(head) {
		mime = "text/plain"
	}
	return &UploadedFile{
		Key:    k,
		Size:   total,
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
		MIME:   mime,
		Ext:    fileExtOf(k),
	}, nil
}

// HasPrefix 判断存储 key 是否落在指定场景目录下。
//
// 用途：资源下载只接受通过资源场景上传的文件。没有这道校验，
// 运营可以把专家资质证书（私有）的 key 挂成公开下载资源，
// 等于用一个表单字段绕过了整个私有文件隔离机制。
func HasPrefix(key, scene string) bool {
	p, ok := storage.LookupPolicy(storage.Scene(scene))
	if !ok {
		return false
	}
	return strings.HasPrefix(key, p.Dir+"/")
}

// Delete 删除文件。
func (s *UploadService) Delete(ctx context.Context, key string) error {
	return s.store.Delete(ctx, key)
}

// AssignURL 为已存储的 key 生成访问地址：
// 公开文件给静态地址，私有文件给带签名的临时地址。
//
// 专家资质、问题附件这类场景统一走它，避免各处自己拼 URL 时
// 把私有文件拼成了公开地址——那是直接的越权读取。
func (s *UploadService) AssignURL(key string) (url string, expireAt int64, private bool) {
	k, err := storage.CleanKey(key)
	if err != nil {
		return "", 0, false
	}
	if IsPrivateKey(k) {
		u, exp := s.SignedURL(k)
		return u, exp, true
	}
	return s.store.PublicURL(k), 0, false
}

// IsPrivateKey 通过目录前缀判断 key 是否属于私有场景。
//
// 按 key 判断而不是让调用方传场景，是因为文件一旦上传完成，
// 后续引用它时（订单附件、专家资质）手上只有 key。
func IsPrivateKey(key string) bool {
	for _, p := range storage.AllPolicies() {
		if p.Private && strings.HasPrefix(key, p.Dir+"/") {
			return true
		}
	}
	return false
}

// buildResult 组装上传返回。
func (s *UploadService) buildResult(policy storage.Policy, key, name string, size int64, mime string) *dto.UploadResult {
	out := &dto.UploadResult{
		Key: key, Name: filepath.Base(name), Size: size, MIME: mime, Private: policy.Private,
	}
	if policy.Private {
		out.URL, out.ExpireAt = s.SignedURL(key)
	} else {
		out.URL = s.store.PublicURL(key)
	}
	return out
}

func (s *UploadService) limitOf(p storage.Policy) int64 {
	if s.globalMaxBytes > 0 && s.globalMaxBytes < p.MaxSize {
		return s.globalMaxBytes
	}
	return p.MaxSize
}

// contentTypeOf 由扩展名推断 Content-Type，用于私有文件下载。
func contentTypeOf(key string) string {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".gz":
		// 资源包多为 .tar.gz，直接给出归档类型让浏览器走下载而非尝试渲染。
		return "application/gzip"
	case ".7z":
		return "application/x-7z-compressed"
	case ".txt", ".sh", ".yaml", ".yml", ".conf", ".sql", ".md", ".json":
		// 这类纯文本资源下载后是要给人看或直接执行的，
		// 明确 charset 才能避免浏览器把中文注释显示成乱码。
		return "text/plain; charset=utf-8"
	default:
		// 未知类型一律按二进制流下载，绝不回落到 text/html 之类的可渲染类型，
		// 否则上传的 SVG / HTML 会变成存储型 XSS 的载荷。
		return "application/octet-stream"
	}
}

// humanSize 把字节数转成易读文案。
func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// fileExtOf 从存储 key 取出扩展名（不含点、小写）。
// 注意这是**服务端规范过的**扩展名，与用户原始文件名的扩展名可能不同。
func fileExtOf(key string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(key)), ".")
}
