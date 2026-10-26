package router

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/config"
)

// newTestEngine 用一个只填了 Cfg 的 Options 建引擎。
//
// 路由注册只把 handler 方法包成 gin.HandlerFunc，不会解引用任何 service，
// 所以依赖留空是安全的。这样这个测试就能在不连数据库的情况下回答唯一
// 真正需要它回答的问题：路由树能不能建起来、建成了些什么。
func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	// CORS 中间件拿到全空的配置会直接 panic（gin-contrib/cors 认为
	// "没有任何来源放行"是配置错误），所以这里必须给一个来源，
	// 否则测的根本不是路由注册，而是 CORS 的入参校验。
	cfg.CORS.AllowOrigins = []string{"http://localhost:3000"}
	// 打开前端托管，让静态站点那一组路由也真挂上——
	// "/" 与 "/assets/*filepath" 与 API 共处一棵路由树，
	// 正是最需要验证"互不遮挡"的组合。
	cfg.Web.Enabled = true
	return New(Options{Cfg: cfg})
}

// TestRouteTreeBuilds 通过"能建起来"来验证没有路由冲突。
//
// gin 在同一路径位置混用不同名的参数（:id 与 :orderNo）会在注册时 panic，
// 而这个问题只在进程启动时暴露——单测里先撞一次，比等到部署时才发现好。
func TestRouteTreeBuilds(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("路由注册 panic：%v", r)
		}
	}()
	r := newTestEngine(t)
	if r == nil {
		t.Fatal("引擎为空")
	}
}

// TestOrderRoutesRegistered 逐条核对订单域路由。
//
// 表里写的是"期望存在的 METHOD + 完整路径"，与实现里挂载的位置对不上就会失败。
// 之所以值得逐条钉住：订单接口的路径前缀高度重叠（/orders、/orders/:orderNo、
// /orders/:orderNo/review...），少挂一条或挂错一层，症状只是前端 404，
// 排查起来要翻半天。
func TestOrderRoutesRegistered(t *testing.T) {
	r := newTestEngine(t)
	got := make(map[string]bool, 64)
	for _, rt := range r.Routes() {
		got[rt.Method+" "+rt.Path] = true
	}

	want := []string{
		// 悬赏问题
		"POST /api/v1/questions",
		"GET /api/v1/questions",
		"GET /api/v1/questions/:id",
		"POST /api/v1/questions/:id/cancel",

		// 订单
		"POST /api/v1/orders",
		"GET /api/v1/orders",
		"GET /api/v1/orders/counts",
		"GET /api/v1/orders/pay-channels",
		"GET /api/v1/orders/:orderNo",
		"POST /api/v1/orders/:orderNo/pay",
		"POST /api/v1/orders/:orderNo/cancel",
		"POST /api/v1/orders/:orderNo/sessions",
		"POST /api/v1/orders/:orderNo/sessions/end",
		"POST /api/v1/orders/:orderNo/finish",
		"POST /api/v1/orders/:orderNo/confirm",
		"POST /api/v1/orders/:orderNo/review",
		"POST /api/v1/orders/:orderNo/review/reply",
		"POST /api/v1/orders/:orderNo/review/appeal",
		"POST /api/v1/orders/:orderNo/dispute",

		// 接单大厅
		"GET /api/v1/orders/hall",
		"POST /api/v1/orders/:orderNo/grab",

		// 后台
		"GET /api/v1/admin/orders",
		"GET /api/v1/admin/orders/:orderNo",
		"POST /api/v1/admin/orders/:orderNo/mark-paid",
		"POST /api/v1/admin/orders/:orderNo/transition",
		"POST /api/v1/admin/orders/:orderNo/settle",
		"POST /api/v1/admin/disputes/:id/resolve",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("缺少路由 %s", w)
		}
	}
}

// TestNoDuplicateRoutes 确保没有同一 METHOD + 路径被注册两次。
//
// gin 本身对重复注册是静默覆盖？不是——它会 panic。但 panic 的提示里
// 只给冲突的两个 handler 名，没有"谁该删"的信息。这里直接列出重复项。
func TestNoDuplicateRoutes(t *testing.T) {
	r := newTestEngine(t)
	seen := make(map[string]int, 256)
	for _, rt := range r.Routes() {
		seen[rt.Method+" "+rt.Path]++
	}
	for key, n := range seen {
		if n > 1 {
			t.Errorf("路由重复注册 %d 次：%s", n, key)
		}
	}
}

// TestOrderNoShapeIsSingleOwner 钉住"订单号参数名只有一个"这条约束。
//
// 混用 :id 与 :orderNo 会让 gin 在启动时 panic，但更隐蔽的坏味道是
// 同一个资源在不同接口里用不同名字标识。这里把订单域所有含参数的路径
// 抽出来，要求它们一律用 :orderNo（后台争议用 :id，那是争议自己的主键）。
func TestOrderNoShapeIsSingleOwner(t *testing.T) {
	r := newTestEngine(t)
	for _, rt := range r.Routes() {
		if !isOrderPath(rt.Path) {
			continue
		}
		if hasParamNamed(rt.Path, "id") && !hasParamNamed(rt.Path, "orderNo") {
			t.Errorf("订单路由 %s %s 使用了 :id，应统一为 :orderNo", rt.Method, rt.Path)
		}
	}
}

func isOrderPath(p string) bool {
	return hasPrefix(p, "/api/v1/orders") || hasPrefix(p, "/api/v1/admin/orders")
}

func hasPrefix(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }

// hasParamNamed 判断路径里是否有名为 name 的段（形如 :name 或 :name/ 结尾）。
func hasParamNamed(path, name string) bool {
	target := ":" + name
	for i := 0; i+len(target) <= len(path); i++ {
		if path[i:i+len(target)] != target {
			continue
		}
		end := i + len(target)
		if end == len(path) || path[end] == '/' {
			return true
		}
	}
	return false
}

// TestFrontendAndAPIcoexist 确认静态站点路由与 API 路由不互相遮挡。
//
// 前端页面挂在 "/"，API 挂在 "/api/v1"。两侧都必须存在，
// 否则要么打开首页是 JSON，要么所有接口 404。
func TestFrontendAndAPIcoexist(t *testing.T) {
	r := newTestEngine(t)
	var hasRoot, hasAPI bool
	for _, rt := range r.Routes() {
		if rt.Path == "/" || rt.Path == "/assets/*filepath" {
			hasRoot = true
		}
		if rt.Path == "/api/v1" {
			hasAPI = true
		}
	}
	if !hasAPI {
		t.Error("缺少 /api/v1 路由")
	}
	if !hasRoot {
		t.Error("缺少前端站点路由")
	}
}
