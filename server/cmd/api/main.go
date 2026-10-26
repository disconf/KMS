// Command api 是运维专家服务平台的后端服务入口。
//
// 启动流程：加载配置 → 初始化日志 → 连接数据库与缓存 → 装配依赖 → 启动 HTTP 服务 → 优雅退出。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/bootstrap"
	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/jwtx"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/storage"
	"github.com/opsexpert/platform/internal/pkg/tokenstore"
	"github.com/opsexpert/platform/internal/repository"
	"github.com/opsexpert/platform/internal/router"
	"github.com/opsexpert/platform/internal/scheduler"
	"github.com/opsexpert/platform/internal/service"
	"github.com/opsexpert/platform/internal/service/payment"
	"github.com/opsexpert/platform/internal/service/sms"
	"github.com/opsexpert/platform/internal/service/wechat"
)

// version 由构建时通过 -ldflags 注入。
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", defaultConfigPath(), "配置文件路径")
	showVersion := flag.Bool("version", false, "打印版本号后退出")
	// init-db：建表 + 写种子后退出。生产库的内容初始化借它完成——
	// prod 环境会跳过全部种子，所以初始化那一趟要用 APP_ENV=test 覆盖
	// （compose 的 init 服务负责），种子幂等，跑多少次结果一致。
	initDB := flag.Bool("init-db", false, "初始化数据库结构并写入种子数据后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.AddSource)

	// SIGINT/SIGTERM 触发优雅退出；根 ctx 用作所有启动期操作的父上下文。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		return err
	}
	defer closeDB(db)

	if *initDB {
		// 种子里的示例资源要落真实文件，store 必须在 Seed 之前建好。
		store, err := storage.New(storage.Options{
			Driver:   cfg.Storage.Driver,
			LocalDir: cfg.Storage.Local.Dir,
			BaseURL:  cfg.PublicFileBaseURL(),
		})
		if err != nil {
			return err
		}
		if err := bootstrap.Seed(ctx, db, cfg, store); err != nil {
			return err
		}
		logger.Info("数据库初始化完成", "env", cfg.App.Env)
		return nil
	}

	c, err := bootstrap.OpenCache(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	// 存储驱动在启动时就确定：本地盘还是对象存储必须一次说清，
	// 不能等到用户上传第一个文件才发现配置错了。
	// 种子数据里的示例资源要落真实文件，所以 store 必须在 Seed 之前建好。
	store, err := storage.New(storage.Options{
		Driver:   cfg.Storage.Driver,
		LocalDir: cfg.Storage.Local.Dir,
		BaseURL:  cfg.PublicFileBaseURL(),
	})
	if err != nil {
		return err
	}

	if err := bootstrap.Seed(ctx, db, cfg, store); err != nil {
		return err
	}

	smsProvider, err := sms.New(cfg.SMS)
	if err != nil {
		return err
	}
	wechatProvider := wechat.New(cfg.Wechat)

	// 装配依赖链：repository → service → handler（由 router 内部完成）。
	userRepo := repository.NewUserRepo(db)
	expertRepo := repository.NewExpertRepo(db)
	skillRepo := repository.NewSkillRepo(db)
	articleRepo := repository.NewArticleRepo(db)
	categoryRepo := repository.NewCategoryRepo(db)
	orderRepo := repository.NewOrderRepo(db)
	settlementRepo := repository.NewSettlementRepo(db)
	withdrawalRepo := repository.NewWithdrawalRepo(db)
	// reviewRepo 提前到这里：专家服务的公开评价摘要要用它，
	// 而在下面订单服务里也会传同一个实例（仓储无状态，共用一份即可）。
	reviewRepo := repository.NewReviewRepo(db)
	jwtMgr := jwtx.NewManager(
		cfg.JWT.Secret, cfg.JWT.Issuer,
		cfg.JWT.AccessTTL.Std(), cfg.JWT.RefreshTTL.Std(),
	)
	tokenStore := tokenstore.New(c)

	authSvc := service.NewAuthService(
		userRepo, expertRepo, c, tokenStore, jwtMgr, smsProvider, wechatProvider, cfg,
	)
	// 积分仓储同时被文章（教程过审）与问答（采纳/点赞）使用，
	// 所以先建一份，两边共用。
	pointRepo := repository.NewPointRepo(db)
	articleSvc := service.NewArticleService(articleRepo, categoryRepo, userRepo, pointRepo)

	uploadSvc := service.NewUploadService(store, cfg.FileSignSecret(), cfg.Storage.MaxSizeMB)
	expertSvc := service.NewExpertService(
		expertRepo, skillRepo, userRepo, orderRepo,
		settlementRepo, withdrawalRepo, reviewRepo, uploadSvc,
	)

	// 知识库各栏目。分类表被文章 / 工具 / 题库 / 资源四方共用，
	// 所以 categoryRepo 会被传给多个 service。
	toolRepo := repository.NewToolRepo(db)
	quizRepo := repository.NewQuizRepo(db)
	resourceRepo := repository.NewResourceRepo(db)

	channelSvc := service.NewChannelService(articleRepo, toolRepo, quizRepo, resourceRepo)
	toolSvc := service.NewToolService(toolRepo, categoryRepo)
	resourceSvc := service.NewResourceService(resourceRepo, categoryRepo, uploadSvc)
	quizSvc := service.NewQuizService(quizRepo, categoryRepo, pointRepo)
	// 教程集只做目录组织，正文仍是 articles 里的文章（见 model.ToolBook 的说明）。
	toolBookSvc := service.NewToolBookService(repository.NewToolBookRepo(db), toolRepo)

	// 运维资讯的订阅源。抓取任务按分钟级唤醒但内部按 20 分钟节流，
	// 真实频率写在 NewsService 里，避免"配了 1m 就每分钟去拉别人的站"。
	newsRepo := repository.NewNewsRepo(db)
	newsSvc := service.NewNewsService(newsRepo, categoryRepo)

	// 支付通道在启动时定下来：装配哪个 provider 决定"下单后能不能真的收到钱"，
	// 不能在用户点支付的时候才发现配置的通道还没接通。
	payProvider, err := payment.New(cfg.Payment)
	if err != nil {
		return err
	}

	questionRepo := repository.NewQuestionRepo(db)
	sessionRepo := repository.NewServiceSessionRepo(db)
	disputeRepo := repository.NewDisputeRepo(db)

	// 后台用户治理。审计仓储单独一份：audit_logs 是全站共用的，
	// 以后订单强制流转、内容审核也会写它，不能绑在某一个业务 service 上。
	userAdminRepo := repository.NewUserAdminRepo(db)
	auditRepo := repository.NewAuditRepo(db)
	userAdminSvc := service.NewUserAdminService(userAdminRepo, auditRepo)

	// 财务对账独立一份仓储：它只读，且要跨 orders / settlements / withdrawals
	// 三张表做聚合，挂到任何一个业务仓储上都会让那个仓储的职责变模糊。
	financeSvc := service.NewFinanceService(repository.NewFinanceReportRepo(db))

	// 后台系统配置：抽佣比例等运营常调项。抽佣同时被订单服务读取，
	// 所以它必须建在 orderSvc 之前——顺序反了只能传 nil，比例就永远读配置文件。
	settingSvc := service.NewSettingService(
		repository.NewSettingRepo(db), auditRepo, cfg.Payment.CommissionRateBP,
	)
	skillAdminSvc := service.NewSkillAdminService(skillRepo)

	// 问答广场。积分仓储与问答分开建：积分是跨内容的（教程过审、题目过审也记分），
	// 绑在 QaService 里以后别的模块要记分就得绕过来借。
	qaSvc := service.NewQaService(
		repository.NewQaRepo(db), pointRepo,
		userRepo, auditRepo, articleSvc,
	)

	orderSvc := service.NewOrderService(
		orderRepo, questionRepo, reviewRepo, sessionRepo, disputeRepo,
		settlementRepo, expertRepo, userRepo, uploadSvc,
		payProvider, cfg.Payment, settingSvc,
	)

	engine := router.New(router.Options{
		Cfg:       cfg,
		DB:        db,
		Auth:      authSvc,
		Articles:  articleSvc,
		Uploads:   uploadSvc,
		Experts:   expertSvc,
		Qa:        qaSvc,
		Channels:  channelSvc,
		Tools:     toolSvc,
		ToolBooks: toolBookSvc,
		Resources: resourceSvc,
		Quizzes:   quizSvc,
		Orders:    orderSvc,
		News:      newsSvc,
		JWT:       jwtMgr,
		Tokens:    tokenStore,
		// 认证中间件靠它复核账号状态与角色，封禁与改角色因此即时生效。
		Users:     userRepo,
		UserAdmin: userAdminSvc,
		Finance:   financeSvc,
		Settings:  settingSvc,
		Skills:    skillAdminSvc,
	})

	// 三个周期任务都关乎"钱"或"名额"，缺一个就会有单子烂在库里：
	//   - 支付超时不关单   → 专家的接单名额被占着，这条悬赏再没人看得见；
	//   - 到期不自动确认   → 用户忘了点确认，专家干了活永远拿不到钱；
	//   - 结算不兜底       → 确认成功但结算那步失败的单子永远停在已完成。
	//
	// 三者都用全局周期，不做差异化：兜底任务空跑时只是一次带条件的索引查询，
	// 便宜到不值得为它单独引入一个配置项；而"我配了 1m，所以三件事都是 1m 一次"
	// 比"其中一个其实是 5m"更容易解释，也更容易在出问题时核对。
	batch := cfg.Scheduler.BatchSize
	sched, err := scheduler.New(cfg.Scheduler, logger.L(),
		scheduler.Job{
			Name: "cancel-expired-payments",
			Run: func(ctx context.Context) (int, error) {
				return orderSvc.AutoCancelExpiredPayments(ctx, batch)
			},
		},
		scheduler.Job{
			Name: "auto-confirm-expired",
			Run: func(ctx context.Context) (int, error) {
				return orderSvc.AutoConfirmExpired(ctx, batch)
			},
		},
		scheduler.Job{
			Name: "settle-completed",
			Run: func(ctx context.Context) (int, error) {
				return orderSvc.SettleCompleted(ctx, batch)
			},
		},
		// 资讯抓取与上面三个任务不同：它不碰钱，失败也不影响交易，
		// 所以单源失败只记 warn，不让整轮任务报错。
		scheduler.Job{
			Name: "fetch-ops-news",
			Run: func(ctx context.Context) (int, error) {
				return newsSvc.FetchAll(ctx)
			},
		},
	)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:         cfg.App.Addr,
		Handler:      engine,
		ReadTimeout:  cfg.App.ReadTimeout.Std(),
		WriteTimeout: cfg.App.WriteTimeout.Std(),
	}

	logStartupBanner(cfg, smsProvider, wechatProvider)

	if err := sched.Start(ctx); err != nil {
		return err
	}
	if sched.Enabled() {
		logger.L().Info("定时任务已启动",
			"jobs", jobNames(sched.Jobs()),
			"interval", cfg.Scheduler.Interval.Std().String(),
			"batch_size", cfg.Scheduler.BatchSize,
			"job_timeout", cfg.Scheduler.JobTimeout.Std().String(),
			"run_on_start", cfg.Scheduler.RunOnStart)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("收到退出信号，开始优雅关闭")
	}

	shutdownTimeout := cfg.App.ShutdownTimeout.Std()
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10 * time.Second
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// 顺序：先停 HTTP，再停定时任务。
	// 反过来的话，调度器等当前轮跑完的这几秒里还在接新请求，
	// 用户可能刚好看到"服务还在，但下单后状态不推进"的窗口。
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅关闭超时: %w", err)
	}

	// 定时任务用独立的短超时：它只需要把当前这一轮跑完。
	// 等不到就打 warn 后退出——进程总归要停，卡在这里比丢一轮任务更糟，
	// 缺的那一轮由重启后的新进程补上（三个任务都是幂等的）。
	schedCtx, cancelSched := context.WithTimeout(context.Background(), 5*time.Second)
	if err := sched.Stop(schedCtx); err != nil {
		logger.Warn("定时任务未在期限内退出", "err", err.Error())
	}
	cancelSched()

	logger.Info("服务已停止")
	return nil
}

// jobNames 把任务清单拼成一行，供启动日志展示。
// 直接打结构体会带出闭包指针，运维要的是"启了哪几个任务"。
func jobNames(jobs []scheduler.Job) string {
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		names = append(names, j.Name)
	}
	return strings.Join(names, ",")
}

func logStartupBanner(cfg *config.Config, smsProvider sms.Provider, wechatProvider wechat.Provider) {
	cacheMode := "内存（进程内）"
	if !cfg.MemCache() {
		cacheMode = "Redis " + cfg.Redis.Addr
	}

	attrs := []any{
		slog.String("version", version),
		slog.String("env", cfg.App.Env),
		slog.String("addr", cfg.App.Addr),
		slog.String("database", cfg.Database.Driver),
		slog.String("cache", cacheMode),
		slog.String("sms_provider", sms.Describe(smsProvider)),
		slog.String("wechat_provider", wechatProvider.Name()),
		slog.Bool("auto_migrate", cfg.Database.AutoMigrate),
	}
	if cfg.Source() != "" {
		attrs = append(attrs, slog.String("config_file", cfg.Source()))
	}
	logger.L().Info("服务启动", attrs...)

	if cfg.Database.Driver == "sqlite" {
		logger.Warn("当前使用 SQLite，仅适合本地开发；生产环境请切换 database.driver=mysql")
	}
	if cfg.MemCache() {
		logger.Warn("未配置 Redis，缓存已降级为进程内实现；多实例部署前必须配置 redis.addr")
	}
}

// closeDB 关闭底层连接池。
func closeDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	if err := sqlDB.Close(); err != nil {
		logger.Warn("关闭数据库连接失败", "err", err.Error())
	}
}

// defaultConfigPath 依次探测常见配置位置，找不到时返回空串（纯环境变量模式）。
func defaultConfigPath() string {
	candidates := []string{
		"configs/config.yaml",
		"config.yaml",
		"../configs/config.yaml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
