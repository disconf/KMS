package storage

import (
	"path"
	"time"
)

// Scene 是上传场景。不同场景的允许类型、大小上限、是否私有都不同，
// 策略集中在一处维护，避免校验散落到各 handler 后口径逐渐不一致。
type Scene string

const (
	SceneAvatar             Scene = "avatar"              // 用户 / 专家头像
	SceneArticleCover       Scene = "article_cover"       // 文章封面
	SceneArticleImage       Scene = "article_image"       // 文章正文插图
	SceneExpertCert         Scene = "expert_cert"         // 专家资质证书
	SceneQuestionAttachment Scene = "question_attachment" // 悬赏问题附件（日志 / 截图）
	SceneReviewImage        Scene = "review_image"        // 评价图片
	SceneResourceFile       Scene = "resource_file"       // 资源下载栏目的附件
)

// Policy 描述单个场景的上传约束。
type Policy struct {
	Scene Scene
	// Dir 存储子目录，同时决定公开文件的 URL 前缀。
	Dir string
	// MaxSize 单文件大小上限（字节）。
	MaxSize int64
	// MIMEs 允许的真实文件类型（依据文件头识别出的类型）。
	MIMEs map[string]struct{}
	// Private 为 true 时文件不进公开静态目录，只接受签名访问。
	Private bool
	// Desc 用于错误提示与接口文档。
	Desc string
}

var policies = map[Scene]Policy{
	SceneAvatar: {
		Scene: SceneAvatar, Dir: "avatar", MaxSize: 2 << 20,
		MIMEs: mimeSet("image/jpeg", "image/png", "image/webp"),
		Desc:  "头像，≤2MB，JPG / PNG / WebP",
	},
	SceneArticleCover: {
		Scene: SceneArticleCover, Dir: "article/cover", MaxSize: 5 << 20,
		MIMEs: mimeSet("image/jpeg", "image/png", "image/webp"),
		Desc:  "文章封面，≤5MB，JPG / PNG / WebP",
	},
	SceneArticleImage: {
		Scene: SceneArticleImage, Dir: "article/image", MaxSize: 5 << 20,
		MIMEs: mimeSet("image/jpeg", "image/png", "image/webp", "image/gif"),
		Desc:  "正文插图，≤5MB，JPG / PNG / WebP / GIF",
	},
	SceneExpertCert: {
		// 资质证书含身份证件与证书照片，属敏感材料，必须走私有通道。
		Scene: SceneExpertCert, Dir: "expert/cert", MaxSize: 10 << 20,
		MIMEs:   mimeSet("image/jpeg", "image/png", "application/pdf"),
		Private: true,
		Desc:    "资质证书，≤10MB，JPG / PNG / PDF，仅审核方可见",
	},
	SceneQuestionAttachment: {
		// 用户提交的日志只应被接单专家看到，不能公开可访问。
		Scene: SceneQuestionAttachment, Dir: "question", MaxSize: 20 << 20,
		MIMEs:   mimeSet("image/jpeg", "image/png", "image/webp", "application/pdf", "application/zip", "text/plain"),
		Private: true,
		Desc:    "问题附件，≤20MB，图片 / PDF / ZIP / 日志文本",
	},
	SceneReviewImage: {
		Scene: SceneReviewImage, Dir: "review", MaxSize: 5 << 20,
		MIMEs: mimeSet("image/jpeg", "image/png", "image/webp"),
		Desc:  "评价图片，≤5MB，JPG / PNG / WebP",
	},
	SceneResourceFile: {
		// 刻意设为私有：只有走签名地址访问，才能让「需登录下载」这个开关
		// 真正生效，也才能拿到可信的下载计数。若放进公开静态目录，
		// 任何人拿到 URL 就能绕开登录直接下载，且无法统计。
		Scene: SceneResourceFile, Dir: "resource", MaxSize: 50 << 20,
		MIMEs: mimeSet(
			"application/pdf", "application/zip", "application/gzip",
			"application/x-7z-compressed", "text/plain",
			"image/jpeg", "image/png", "image/webp",
		),
		Private: true,
		Desc:    "资源文件，≤50MB，PDF / ZIP / TAR.GZ / 7Z / 脚本 / 配置 / 图片",
	},
}

// LookupPolicy 查询场景策略。
func LookupPolicy(s Scene) (Policy, bool) {
	p, ok := policies[s]
	return p, ok
}

// AllPolicies 返回全部场景策略，供接口文档列举。
func AllPolicies() []Policy {
	out := make([]Policy, 0, len(policies))
	for _, p := range policies {
		out = append(out, p)
	}
	return out
}

// Allow 判断识别出的真实类型是否在该场景白名单内。
func (p Policy) Allow(mime string) bool {
	_, ok := p.MIMEs[mime]
	return ok
}

// PublicDir 返回公开场景的静态目录名；私有场景返回空串。
func (p Policy) PublicDir() string {
	if p.Private {
		return ""
	}
	return p.Dir
}

// BuildKey 生成存储 key。
//
// 文件名完全由服务端决定：目录 + 年月 + 随机串 + 规范扩展名。
// 客户端提供的原始文件名一律不参与拼接——那是最典型的路径穿越入口。
func (p Policy) BuildKey(mime, random string) string {
	return path.Join(p.Dir, time.Now().Format("200601"), random+ExtForMIME(mime))
}

func mimeSet(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[it] = struct{}{}
	}
	return m
}
