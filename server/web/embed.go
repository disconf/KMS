// Package web 内嵌知识库前端的静态资源，并把它挂到服务根路径上。
//
// 前端刻意做成零构建的静态页面、由同一个服务托管：前后端同源就不需要
// 任何跨域配置，交付物也只有一个可执行文件，不会出现"接口起来了但页面忘了带上"
// 这种部署事故。开发时可以用 SetDiskDir 切到磁盘目录，改页面不必重新编译。
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:static
var embedded embed.FS

const (
	// staticRoot 是前端资源在嵌入文件系统里的根目录。
	staticRoot = "static"
	// assetPrefix 是静态资源对外暴露的 URL 前缀。
	//
	// 用 /assets 而不是 /static，是为了和嵌入目录名解耦：
	// 将来若把前端换成构建产物（例如 Vite 的 dist），对外地址不用跟着改。
	assetPrefix = "/assets"
)

// diskDir 非空时优先从磁盘读取前端资源，用于开发期热加载。
var diskDir string

// SetDiskDir 指定优先读取的磁盘目录。传空串即回退到内嵌副本。
// 目录里没有 index.html 时同样回退——配置写错不该让整个前端 404。
func SetDiskDir(dir string) {
	diskDir = strings.TrimSpace(dir)
}

// FS 返回前端资源的根文件系统，根目录下即为 index.html 与 assets/。
func FS() fs.FS {
	if diskDir != "" {
		if _, err := os.Stat(filepath.Join(diskDir, "index.html")); err == nil {
			return os.DirFS(diskDir)
		}
	}
	sub, err := fs.Sub(embedded, staticRoot)
	if err != nil {
		// static 目录在编译期已被 embed 校验存在，正常走不到这里。
		panic("web: 内嵌静态资源缺失: " + err.Error())
	}
	return sub
}

// Register 把前端挂在根路径上。
//
// 只接管 "/" 与 "/assets/*"，其余一概不碰：/healthz、/readyz、/docs、
// /openapi.yaml 以及 /api/v1/** 都是服务自己的地址，用一条通配路由兜底
// 会把它们连同 NoRoute 的错误响应一起吞掉。
func Register(r gin.IRouter) {
	fileServer := http.FileServer(http.FS(FS()))

	r.GET(assetPrefix+"/*filepath", serveAsset(fileServer))
	r.HEAD(assetPrefix+"/*filepath", serveAsset(fileServer))
	r.GET("/", serveIndex)
	r.HEAD("/", serveIndex)
}

func serveAsset(fileServer http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 目录请求直接拒掉：这里没有任何需要列举的目录，
		// 放行只会把工程结构白送出去。
		if p := c.Param("filepath"); p == "" || strings.HasSuffix(p, "/") {
			c.Status(http.StatusNotFound)
			return
		}
		// 前端资源没有内容哈希，不做长缓存。
		// 本地部署下这点重复传输无关紧要，换来的是"改完刷新就生效"。
		c.Header("Cache-Control", "no-cache")
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func serveIndex(c *gin.Context) {
	page, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "前端入口文件缺失，部署包可能不完整")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", page)
}
