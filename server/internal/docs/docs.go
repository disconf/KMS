// Package docs 内嵌 OpenAPI 规范并暴露 Swagger UI。
//
// 这里刻意不用 swag 代码生成：那需要额外安装 swag CLI 并维护 docs 包，
// 而手写一份 OpenAPI 3.0 文件既没有构建依赖，也方便直接导入 Postman / Apifox。
package docs

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.yaml
var spec []byte

const uiHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>个人工作经验分享 · API 文档</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/openapi.yaml',
      dom_id: '#swagger-ui',
      docExpansion: 'list',
      defaultModelsExpandDepth: 1
    });
  </script>
</body>
</html>`

// Register 注册文档路由：/openapi.yaml 提供规范，/docs 提供可视化界面。
func Register(r gin.IRouter) {
	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", spec)
	})
	r.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(uiHTML))
	})
}
