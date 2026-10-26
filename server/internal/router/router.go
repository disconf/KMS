// Package router 负责路由注册与全局中间件装配。
package router

import (
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/docs"
	"github.com/opsexpert/platform/internal/handler"
	"github.com/opsexpert/platform/internal/middleware"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/jwtx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/pkg/tokenstore"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/service"
	"github.com/opsexpert/platform/web"
)

// Options 是构建路由所需的依赖。
type Options struct {
	Cfg       *config.Config
	DB        *gorm.DB
	Auth      *service.AuthService
	Articles  *service.ArticleService
	Uploads   *service.UploadService
	Experts   *service.ExpertService
	Qa        *service.QaService
	Channels  *service.ChannelService
	Tools     *service.ToolService
	ToolBooks *service.ToolBookService
	Resources *service.ResourceService
	Quizzes   *service.QuizService
	Orders    *service.OrderService
	News      *service.NewsService
	UserAdmin *service.UserAdminService
	Finance   *service.FinanceService
	Settings  *service.SettingService
	Skills    *service.SkillAdminService
	JWT       *jwtx.Manager
	Tokens    *tokenstore.Store

	// Users 供认证中间件复核账号状态（封禁即时生效）。
	// 它是必填项：漏传会让中间件无法判断账号是否可用，等于封禁不生效。
	Users *repository.UserRepo
}

// authMW 返回强制登录中间件。
//
// 全站只此一处构造它，就是为了「账号状态校验」这件事不可能被某条路由漏掉。
// 各分组自己拼 middleware.Auth(...) 时，新加一条路由很容易复制到一份忘了传
// accounts 的旧写法——那种错误编译期发现不了，上线后表现为"某几个接口封禁无效"。
func (o Options) authMW() gin.HandlerFunc {
	return middleware.Auth(o.JWT, o.Tokens, o.Users)
}

// New 装配 gin 引擎。
func New(opt Options) *gin.Engine {
	cfg := opt.Cfg

	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.RedirectTrailingSlash = false

	r.Use(
		middleware.RequestID(),
		middleware.AccessLog(),
		middleware.Recovery(),
		middleware.CORS(cfg.CORS),
	)
	r.NoRoute(middleware.NoRoute())

	registerHealthRoutes(r, opt)
	if cfg.ExposeAPIDocs() {
		registerDocsRoutes(r)
	}
	registerFrontendRoutes(r, cfg)

	v1 := r.Group("/api/v1")
	v1.GET("", serviceInfo(cfg))
	registerAuthRoutes(v1, opt)
	registerUserRoutes(v1, opt)
	registerKnowledgeRoutes(v1, opt)
	registerContentRoutes(v1, opt)
	registerQuizRoutes(v1, opt)
	registerUploadRoutes(v1, opt)
	registerExpertRoutes(v1, opt)
	registerQaRoutes(v1, opt)
	registerOrderRoutes(v1, opt)
	registerAdminRoutes(v1, opt)

	return r
}

// registerFrontendRoutes 挂载知识库前端页面。
//
// 根路径交给页面而不是回一段 JSON：这个地址是给人看的，
// 要看服务信息走 /api/v1。关闭 web.enabled 时才退回原来的 JSON 响应，
// 方便前后端分离部署时仍有一个可探活的根路径。
func registerFrontendRoutes(r *gin.Engine, cfg *config.Config) {
	if !cfg.Web.Enabled {
		r.GET("/", serviceInfo(cfg))
		return
	}
	if dir := strings.TrimSpace(cfg.Web.Dir); dir != "" {
		web.SetDiskDir(dir)
	}
	web.Register(r)
}

func serviceInfo(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		info := gin.H{
			"service": cfg.App.Name,
			"env":     cfg.App.Env,
		}
		// 文档关了就别在探活响应里留两个 404 的地址，
		// 免得对接方照着响应里的链接去试、还以为是挂了。
		if cfg.ExposeAPIDocs() {
			info["docs"] = "/docs"
			info["openapi"] = "/openapi.yaml"
		}
		response.OK(c, info)
	}
}

func registerHealthRoutes(r *gin.Engine, opt Options) {
	h := handler.NewHealthHandler(opt.DB, opt.Cfg)
	r.GET("/healthz", h.Live)
	r.GET("/readyz", h.Ready)
}

func registerDocsRoutes(r *gin.Engine) {
	docs.Register(r)
}

func registerAuthRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewAuthHandler(opt.Auth)
	auth := g.Group("/auth")

	// 公开接口：登录链路本身就是用来换取身份的，不能要求已登录。
	auth.POST("/sms/send", h.SendCode)
	auth.POST("/login/sms", h.LoginBySMS)
	// 密码登录：线上短信通道未接通时的后台入口。只对已设密码的账号生效，
	// 失败限流在 service 层（按手机号 15 分钟 5 次）。
	auth.POST("/login/password", h.LoginByPassword)
	auth.GET("/wechat/url", h.WechatAuthURL)
	auth.POST("/login/wechat", h.LoginByWechat)
	auth.POST("/refresh", h.Refresh)

	// 需要登录：登出要能定位到当前令牌才能吊销。
	auth.POST("/logout", opt.authMW(), h.Logout)
}

func registerUserRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewUserHandler(opt.Auth)
	authed := g.Group("", opt.authMW())
	authed.GET("/me", h.Me)
	authed.PUT("/me", h.UpdateMe)
	authed.POST("/me/password", h.ChangePassword)
}

// registerKnowledgeRoutes 知识库前台路由。
//
// 用 OptionalAuth 而非 Auth：游客可读是 SEO 获客的前提，
// 但带上令牌时又要能识别出"这篇我点过赞"，所以要有条件地解析身份。
func registerKnowledgeRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewArticleHandler(opt.Articles)

	// 注意：详情与点赞共用同一个参数名 :key。
	// gin 的路由树不允许同一层级出现两个不同的通配符名，写岔会直接 panic。
	guest := g.Group("/kb", middleware.OptionalAuth(opt.JWT, opt.Tokens))
	guest.GET("/categories", h.ListCategories)
	guest.GET("/tags", h.ListTags)
	guest.GET("/articles", h.ListArticles)
	guest.GET("/search/suggest", h.SearchSuggest)
	guest.GET("/articles/:key", h.ArticleDetail)

	authed := g.Group("/kb", opt.authMW())
	authed.POST("/articles/:key/like", h.ToggleLike)

	// 投稿。路径写在 :key 之前只是阅读顺序，gin 的静态节点优先于通配节点，
	// /kb/articles/contribute 不会被当成 slug 为 "contribute" 的文章。
	// 真正需要防的是「有人投稿时把 slug 指定成 contribute」——但不开放自定义 slug，
	// 且 slug 由标题推导（中文标题必带汉字，纯 ASCII "contribute" 只可能撞库），故无冲突。
	authed.POST("/articles/contribute", h.Contribute)

	// 我的投稿放在 /me 下而不是 /kb 下：它是个人的东西，不是知识库的公共内容。
	// 注意这里必须**另起一个不挂 /kb 前缀的组**——写成 authed.GET("/me/contributions")
	// 会挂到 /kb/me/contributions 上，与前端约定和 openapi 全对不上。
	me := g.Group("", opt.authMW())
	me.GET("/me/contributions", h.MyContributions)
}

// registerContentRoutes 知识库其余栏目的前台路由：栏目导航、开源工具、资源下载。
//
// 全部挂在 OptionalAuth 上。知识库是 SEO 与获客的主入口，游客必须能读；
// 需要登录才能取下载地址的资源，由 service 依据 needLogin 单独拦截——
// 用整组 Auth 守会把不需要登录的绝大多数资源也一起挡住。
func registerContentRoutes(g *gin.RouterGroup, opt Options) {
	ch := handler.NewChannelHandler(opt.Channels)
	th := handler.NewToolHandler(opt.Tools, opt.ToolBooks)
	rh := handler.NewResourceHandler(opt.Resources)

	guest := g.Group("", middleware.OptionalAuth(opt.JWT, opt.Tokens))
	guest.GET("/channels", ch.List)

	guest.GET("/tools", th.List)
	// filters 必须注册在 :key 之前？不必——gin 会用静态节点优先匹配，
	// 实测 /tools/filters 命中静态处理函数，/tools/kubectl 落到 :key。
	// 两个都存在时不会有歧义，这里保持业务可读的顺序即可。
	guest.GET("/tools/filters", th.Filters)
	guest.GET("/tools/:key", th.Detail)

	// 教程集的独立详情页。工具详情已经带回整棵目录树，
	// 这个端点服务的是「从分享链接/收藏直接打开一本书」的场景。
	bh := handler.NewToolBookHandler(opt.ToolBooks)
	guest.GET("/tool-books/:id", bh.Detail)

	guest.GET("/resources", rh.List)
	guest.GET("/resources/file-exts", rh.FileExts)
	guest.GET("/resources/:key", rh.Detail)
	guest.GET("/resources/:key/download", rh.Download)
}

// registerQuizRoutes 运维题库路由。
//
// 浏览与抽题允许游客（引流），作答相关必须登录——答题记录、错题本
// 都挂在账号上，没有身份就无处可存。
func registerQuizRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewQuizHandler(opt.Quizzes)

	guest := g.Group("/quiz", middleware.OptionalAuth(opt.JWT, opt.Tokens))
	guest.GET("/categories", h.Categories)
	guest.GET("/questions", h.List)
	guest.GET("/practice", h.Practice)

	authed := g.Group("/quiz", opt.authMW())
	authed.POST("/questions/:id/answer", h.Answer)
	authed.POST("/attempts", h.Submit)
	authed.GET("/attempts", h.Attempts)
	authed.GET("/stats", h.Stats)
	authed.GET("/wrong-book", h.WrongBook)
	// 题目投稿。「我的投稿」放在 /quiz 下而不是 /me 下，
	// 与文章那条 /me/contributions 并存：两者是不同表，翻页各算各的，
	// 合并成一条跨表游标列表的复杂度远大于收益，前端在同一页用 tab 展示。
	authed.POST("/contribute", h.Contribute)
	authed.GET("/my/contributions", h.MyContributions)
}

// registerUploadRoutes 文件上传路由。
//
// 三个接口的鉴权强度刻意不同：
//   - /upload/scenes 只是静态的能力清单（允许的格式与大小），不含任何用户数据，公开即可；
//   - /upload 必须登录，否则任何人都能往存储里灌垃圾；
//   - /files 不能要求登录——私有文件的 URL 要能直接塞进 <img src>，
//     浏览器的图片请求带不上 Authorization 头，这里的凭证就是 URL 上的签名。
func registerUploadRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewUploadHandler(opt.Uploads)

	g.GET("/upload/scenes", h.Scenes)
	g.GET("/files", h.Download)

	authed := g.Group("", opt.authMW())
	authed.POST("/upload", h.Upload)
}

// registerExpertRoutes 专家域路由。
//
// 分三层挂载：游客可读的展示层、登录即可用的申请层、以及要求审核通过的自助层。
// 三层混在一起挂会让"申请"这类接口被专家守卫误伤——申请人本来就还不是专家。
func registerExpertRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewExpertHandler(opt.Experts)

	guest := g.Group("", middleware.OptionalAuth(opt.JWT, opt.Tokens))
	guest.GET("/experts", h.List)
	guest.GET("/experts/:id", h.Detail)
	guest.GET("/experts/:id/reviews", h.Reviews)
	guest.GET("/skills", h.Skills)

	// 申请层：只要求登录。未通过审核的用户正是这里的主要使用者，
	// 所以不能再叠加角色或审核守卫——那会把目标用户挡在门外。
	applying := g.Group("/expert", opt.authMW())
	applying.POST("/apply", h.Apply)
	applying.GET("/apply", h.ApplyStatus)
	// 工作台只要求角色，不要求审核通过，让"审核中 / 被驳回"的人也能看到状态横幅。
	applying.GET("/dashboard", h.Dashboard)

	// 自助层：必须是已通过审核的专家。
	approved := g.Group("/expert",
		opt.authMW(),
		middleware.RequireApprovedExpert(),
	)
	approved.PUT("/services", h.UpdateServices)
	approved.PUT("/profile", h.UpdateProfile)
	approved.GET("/wallet", h.Wallet)
	approved.POST("/withdrawals", h.CreateWithdrawal)
	approved.GET("/withdrawals", h.ListWithdrawals)
}

// registerQaRoutes 问答广场。
//
// 与悬赏问题（/questions）分开挂：那是订单链路的入口，这是社区内容，
// 两者的鉴权、限流、审核策略都不同，混在一组迟早会有人把其中一条挂错守卫。
func registerQaRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewQaHandler(opt.Qa)

	guest := g.Group("/qa", middleware.OptionalAuth(opt.JWT, opt.Tokens))
	guest.GET("/questions", h.List)
	guest.GET("/questions/:id", h.Detail)

	authed := g.Group("/qa", opt.authMW())
	authed.POST("/questions", h.Ask)
	authed.DELETE("/questions/:id", h.Delete)
	authed.POST("/questions/:id/answers", h.Answer)
	authed.POST("/questions/:id/accept/:answerId", h.Accept)
	authed.POST("/questions/:id/close", h.Close)
	authed.PUT("/answers/:id", h.UpdateAnswer)
	authed.DELETE("/answers/:id", h.DeleteAnswer)
	authed.POST("/answers/:id/like", h.Like)
	authed.DELETE("/answers/:id/like", h.Unlike)
	authed.GET("/my/points", h.MyPoints)
}

// registerOrderRoutes 悬赏、订单与接单大厅。
//
// 分两层：悬赏与订单是登录即可（下单人 / 服务专家本就是普通用户），
// 接单大厅与抢单额外要求审核通过的专家——大厅里全是别人的悬赏，
// 让普通用户看到既没有可执行的动作，也会把"待接单问题池"暴露给无关的人。
//
// 订单号是路由里的唯一主键，全程用 :orderNo 同一个参数名：
// gin 在同一路径位置混用不同名的参数（:id / :orderNo）会在启动时 panic。
func registerOrderRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewOrderHandler(opt.Orders)

	authed := g.Group("", opt.authMW())

	authed.POST("/questions", h.CreateQuestion)
	authed.GET("/questions", h.ListMyQuestions)
	authed.GET("/questions/:id", h.QuestionDetail)
	authed.POST("/questions/:id/cancel", h.CancelQuestion)

	authed.POST("/orders", h.CreateBooking)
	authed.GET("/orders", h.List)
	authed.GET("/orders/counts", h.Counts)
	authed.GET("/orders/pay-channels", h.PayChannels)
	authed.GET("/orders/:orderNo", h.Detail)
	authed.POST("/orders/:orderNo/pay", h.Pay)
	authed.POST("/orders/:orderNo/cancel", h.Cancel)
	authed.POST("/orders/:orderNo/sessions", h.StartSession)
	authed.POST("/orders/:orderNo/sessions/end", h.EndSession)
	authed.POST("/orders/:orderNo/finish", h.Finish)
	authed.POST("/orders/:orderNo/confirm", h.Confirm)
	authed.POST("/orders/:orderNo/review", h.Review)
	authed.POST("/orders/:orderNo/review/reply", h.ReplyReview)
	authed.POST("/orders/:orderNo/review/appeal", h.AppealReview)
	authed.POST("/orders/:orderNo/dispute", h.RaiseDispute)

	expert := g.Group("",
		opt.authMW(),
		middleware.RequireApprovedExpert(),
	)
	expert.GET("/orders/hall", h.Hall)
	expert.POST("/orders/:orderNo/grab", h.Grab)
}

// registerAdminRoutes 管理后台路由。整组要求 admin 角色。
func registerAdminRoutes(g *gin.RouterGroup, opt Options) {
	h := handler.NewAdminArticleHandler(opt.Articles)
	eh := handler.NewAdminExpertHandler(opt.Experts)
	wh := handler.NewAdminWithdrawalHandler(opt.Experts)
	th := handler.NewAdminToolHandler(opt.Tools)
	tbh := handler.NewAdminToolBookHandler(opt.ToolBooks)
	rh := handler.NewAdminResourceHandler(opt.Resources)
	qh := handler.NewAdminQuizHandler(opt.Quizzes)
	nh := handler.NewAdminNewsHandler(opt.News)
	uh := handler.NewAdminUserHandler(opt.UserAdmin)

	admin := g.Group("/admin",
		opt.authMW(),
		middleware.RequireRole(model.RoleAdmin),
	)

	categories := admin.Group("/kb/categories")
	categories.GET("", h.ListCategories)
	categories.POST("", h.CreateCategory)
	categories.PUT("/:id", h.UpdateCategory)
	categories.DELETE("/:id", h.DeleteCategory)

	articles := admin.Group("/kb/articles")
	articles.GET("", h.List)
	articles.POST("", h.Create)
	articles.GET("/:id", h.Get)
	articles.PUT("/:id", h.Update)
	articles.DELETE("/:id", h.Delete)
	articles.POST("/:id/publish", h.Publish)
	articles.POST("/:id/offline", h.Offline)
	// 审核是发布流程的前置动作，不是另一种发布：走独立接口才能强制带审核意见与审核人。
	articles.POST("/:id/review", h.Review)

	// 与文章同属知识库，统一挂在 /admin/kb 下，
	// 后台导航的"知识库"一级菜单下直接就是各类内容的子菜单。
	tools := admin.Group("/kb/tools")
	tools.GET("", th.List)
	tools.POST("", th.Create)
	tools.GET("/:id", th.Get)
	tools.PUT("/:id", th.Update)
	tools.DELETE("/:id", th.Delete)
	tools.POST("/:id/publish", th.Publish)
	tools.POST("/:id/offline", th.Offline)
	// 四类实战教程是工具的「配置」，整体读写，不做逐条增删接口。
	tools.GET("/:id/guides", th.ListGuides)

	// 教程集挂在工具下：先用工具 ID 建书，之后按书 ID 维护。
	// 章节走大纲文本整体替换（见 SetOutline 的格式说明），不做逐条增删。
	tools.GET("/:id/books", tbh.List)
	tools.POST("/:id/books", tbh.Create)
	books := admin.Group("/kb/tool-books")
	books.GET("/:id", tbh.Get)
	books.PUT("/:id", tbh.Update)
	books.DELETE("/:id", tbh.Delete)
	books.POST("/:id/publish", tbh.Publish)
	books.POST("/:id/offline", tbh.Offline)
	books.GET("/:id/outline", tbh.Outline)
	books.PUT("/:id/outline", tbh.SetOutline)
	tools.PUT("/:id/guides", th.SetGuides)

	resources := admin.Group("/kb/resources")
	resources.GET("", rh.List)
	resources.POST("", rh.Create)
	resources.GET("/:id", rh.Get)
	resources.PUT("/:id", rh.Update)
	resources.DELETE("/:id", rh.Delete)
	resources.POST("/:id/publish", rh.Publish)
	resources.POST("/:id/offline", rh.Offline)
	// 外链下载地址与托管文件并存：有的组件只给官网/镜像地址，不自己存包。
	resources.GET("/:id/links", rh.ListLinks)
	resources.PUT("/:id/links", rh.SetLinks)

	quizzes := admin.Group("/kb/quiz/questions")
	quizzes.GET("", qh.List)
	quizzes.POST("", qh.Create)
	// /import 必须放在 /:id 之前注册：gin 的路由树对同段静态与通配混用时，
	// 先注册的优先匹配，写反了 /import 会被当成 id=import 而 400。
	quizzes.POST("/import", qh.Import)
	quizzes.GET("/:id", qh.Get)
	quizzes.PUT("/:id", qh.Update)
	quizzes.DELETE("/:id", qh.Delete)
	quizzes.POST("/:id/publish", qh.Publish)
	quizzes.POST("/:id/offline", qh.Offline)
	// 审核与发布分开：Publish 是运营发布自己的草稿，Review 是处理用户投稿，
	// 后者会写审核痕迹并给作者记分，语义不能混。
	quizzes.POST("/:id/review", qh.Review)

	// 资讯订阅源。它不是内容本身，而是「内容从哪来」的配置，
	// 因此单独一个前缀，不与 /kb/articles 混在一起。
	news := admin.Group("/news")
	news.GET("/sources", nh.ListSources)
	news.POST("/sources", nh.CreateSource)
	news.PUT("/sources/:id", nh.UpdateSource)
	news.DELETE("/sources/:id", nh.DeleteSource)
	news.POST("/sources/:id/fetch", nh.FetchNow)

	experts := admin.Group("/experts")
	experts.GET("", eh.List)
	experts.GET("/:id", eh.Detail)
	experts.POST("/:id/audit", eh.Audit)

	// 提现审批单独一组，不挂在 /admin/experts 下：
	// 它是钱的操作，以后要加财务角色或双人复核，边界必须一开始就划清。
	withdrawals := admin.Group("/withdrawals")
	withdrawals.GET("", wh.List)
	withdrawals.POST("/:id/review", wh.Review)
	withdrawals.POST("/:id/mark-paid", wh.MarkPaid)
	withdrawals.POST("/:id/fail", wh.Fail)

	// 用户治理。与 /admin/experts 分开：专家是「谁可以接单」，用户是「谁可以用平台」，
	// 封禁一个专家不需要动他的专家档案（解封即恢复），两条链路各自独立。
	users := admin.Group("/users")
	users.GET("", uh.List)
	users.GET("/:id", uh.Detail)
	users.POST("/:id/status", uh.UpdateStatus)
	users.POST("/:id/role", uh.UpdateRole)

	// 问答运营：待审回答、删帖、把优质问答转成教程草稿。
	// 单独一组而不是并进 /admin/kb：问答不是知识库栏目，它是社区内容，
	// 审核节奏与责任人都不同（前者按文章排期，后者要当天处理）。
	qah := handler.NewQaHandler(opt.Qa)
	qaAdmin := admin.Group("/qa")
	qaAdmin.GET("/answers/pending", qah.AdminPending)
	qaAdmin.POST("/answers/:id/review", qah.AdminReview)
	qaAdmin.DELETE("/questions/:id", qah.AdminDeleteQuestion)
	qaAdmin.POST("/questions/:id/to-article", qah.AdminToArticle)

	// 订单与争议同属"交易"这一块，统一挂在 /admin/orders 与 /admin/disputes 下。
	// 争议单独一个前缀而不是 /admin/orders/{orderNo}/dispute：后台是从
	// "待处理争议"列表点进来的，手里拿的是争议 ID，不是订单号。
	oh := handler.NewAdminOrderHandler(opt.Orders)
	orders := admin.Group("/orders")
	orders.GET("", oh.List)
	orders.GET("/:orderNo", oh.Detail)
	orders.POST("/:orderNo/mark-paid", oh.MarkPaid)
	orders.POST("/:orderNo/transition", oh.Transition)
	orders.POST("/:orderNo/settle", oh.Settle)

	disputes := admin.Group("/disputes")
	disputes.POST("/:id/resolve", oh.ResolveDispute)

	// 财务对账。只读，与提现审批分成两件事：
	// 审批是「要不要付这一笔」，对账是「已经发生的事对不对得上」。
	// 合成一组会让以后给财务单独开只读账号时无从下手。
	fh := handler.NewAdminFinanceHandler(opt.Finance)
	recon := admin.Group("/finance/reconciliation")
	recon.GET("", fh.Report)
	// 明细用 /day/{date} 而不是 /{date}：根下的静态段 export 与参数段同层会撞路由。
	recon.GET("/day/:date", fh.Day)
	recon.GET("/export", fh.Export)

	// 系统配置。抽佣比例会参与下单金额计算、技能字典决定入驻时的可选方向，
	// 两者都是「改错了会影响交易或准入」的值，因此只给 admin。
	sh := handler.NewAdminSettingHandler(opt.Settings, opt.Skills)
	settings := admin.Group("/settings")
	settings.GET("", sh.Overview)
	settings.PUT("/commission", sh.UpdateCommission)

	skills := admin.Group("/skills")
	skills.GET("", sh.ListSkills)
	skills.POST("", sh.CreateSkill)
	skills.PUT("/:id", sh.UpdateSkill)
	skills.DELETE("/:id", sh.DeleteSkill)
}
