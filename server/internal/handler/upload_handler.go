package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/pkg/storage"
	"github.com/opsexpert/platform/internal/service"
)

// uploadBodyLimit 上传请求体的硬上限（字节）。
//
// 取略大于最大场景上限（资源文件 50MB）：超出的请求在进入 multipart 解析前
// 就被 MaxBytesReader 截断，不会先被读进内存再被拒绝。
const uploadBodyLimit = 60 << 20

// UploadHandler 文件上传与私有文件访问。
type UploadHandler struct {
	uploads *service.UploadService
}

// NewUploadHandler 创建 UploadHandler。
func NewUploadHandler(uploads *service.UploadService) *UploadHandler {
	return &UploadHandler{uploads: uploads}
}

// Scenes 上传场景列表，前端据此做前置校验与提示。
//
//	@Summary  上传场景与限制
//	@Tags     upload
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /upload/scenes [get]
func (h *UploadHandler) Scenes(c *gin.Context) {
	response.OK(c, gin.H{
		"list":   h.uploads.Scenes(),
		"driver": h.uploads.Driver(),
	})
}

// Upload 上传单个文件。表单字段固定为 file，场景通过 scene 字段传入。
//
//	@Summary  上传文件
//	@Tags     upload
//	@Security BearerAuth
//	@Accept   multipart/form-data
//	@Produce  json
//	@Param    scene formData string true  "上传场景，见 /upload/scenes"
//	@Param    file  formData file   true  "文件内容"
//	@Success  201 {object} response.Body
//	@Router   /upload [post]
func (h *UploadHandler) Upload(c *gin.Context) {
	// 在任何解析动作之前收紧请求体，避免超大请求被完整读入内存。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadBodyLimit)

	// 只读账号只放行头像：换头像不产生公共内容，
	// 而其他场景（资源附件、证书、文章配图）本质都是"发布"，
	// 对只读账号一律挡住，否则存储会先被灌满。
	scene := strings.TrimSpace(c.PostForm("scene"))
	if scene != string(storage.SceneAvatar) && !RequireSpeak(c) {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, errcode.ErrFileMissing.WithMsg("请选择要上传的文件（表单字段名应为 file）"))
		return
	}

	result, err := h.uploads.Upload(c.Request.Context(), scene, file)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, result)
}

// Public 公开文件的静态访问入口（头像、封面、正文插图）。
//
// 这些地址会直接出现在 <img src> 与 Markdown 正文里，不能要求登录；
// key 里带随机段且写入后不再覆盖，按 immutable 长缓存处理。
//
//	@Summary  公开文件访问
//	@Tags     upload
//	@Param    key path string true "存储 key"
//	@Success  200 {file} binary
//	@Router   /uploads/{key}
func (h *UploadHandler) Public(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("key"), "/")
	rc, ctype, err := h.uploads.OpenPublic(c.Request.Context(), key)
	if err != nil {
		response.Fail(c, err)
		return
	}
	defer func() { _ = rc.Close() }()

	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.DataFromReader(http.StatusOK, -1, ctype, rc, nil)
}

// Download 私有文件下载，URL 上必须带有效签名。
//
//	@Summary  下载私有文件
//	@Tags     upload
//	@Produce  octet-stream
//	@Param    key query string true "文件标识"
//	@Param    e   query int    true "签名到期时间（Unix 秒）"
//	@Param    s   query string true "签名"
//	@Success  200 {file} binary
//	@Router   /files [get]
func (h *UploadHandler) Download(c *gin.Context) {
	rc, ctype, err := h.uploads.OpenPrivate(
		c.Request.Context(),
		c.Query("key"), c.Query("e"), c.Query("s"),
	)
	if err != nil {
		response.Fail(c, err)
		return
	}
	defer func() { _ = rc.Close() }()

	// 私有文件禁止任何缓存：签名一旦过期就必须立刻无效，
	// 否则浏览器缓存会让已撤销的访问继续可用。
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Type", ctype)
	// 图片需要内联才能直接展示；其余类型强制下载，
	// 避免 PDF / SVG 之类在浏览器里被当作可执行内容渲染。
	if !strings.HasPrefix(ctype, "image/") {
		c.Header("Content-Disposition", "attachment")
	}

	if _, err := io.Copy(c.Writer, rc); err != nil {
		// 响应头已写出，此时无法再改状态码，只能留日志。
		logger.Warn("私有文件写出中断", "key", c.Query("key"), "err", err.Error())
	}
}
