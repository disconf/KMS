// Package config 负责加载与校验应用配置。
//
// 加载优先级：内置默认值 < configs/config.yaml < 环境变量(.env 或系统环境)。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Duration 让 YAML 支持 "2h" / "60s" 这类人类可读写法。
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration 需为字符串（如 2h / 60s）: %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("非法 duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// Std 返回标准库 time.Duration。
func (d Duration) Std() time.Duration { return time.Duration(d) }

type AppConfig struct {
	Name            string   `yaml:"name"`
	Env             string   `yaml:"env"`
	Addr            string   `yaml:"addr"`
	BaseURL         string   `yaml:"base_url"`
	FrontendURL     string   `yaml:"frontend_url"`
	ReadTimeout     Duration `yaml:"read_timeout"`
	WriteTimeout    Duration `yaml:"write_timeout"`
	ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	// ExposeDocs 决定是否对外暴露 /docs 与 /openapi.yaml。
	// 用指针是为了区分「没配置」和「显式配成 false」，默认值见 ExposeAPIDocs。
	ExposeDocs *bool `yaml:"expose_docs"`
}

type LogConfig struct {
	Level     string `yaml:"level"`
	Format    string `yaml:"format"`
	AddSource bool   `yaml:"add_source"`
}

type DatabaseConfig struct {
	Driver          string   `yaml:"driver"`
	SQLitePath      string   `yaml:"sqlite_path"`
	DSN             string   `yaml:"dsn"`
	MaxOpenConns    int      `yaml:"max_open_conns"`
	MaxIdleConns    int      `yaml:"max_idle_conns"`
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime"`
	AutoMigrate     bool     `yaml:"auto_migrate"`
	LogLevel        string   `yaml:"log_level"`
}

type RedisConfig struct {
	Addr        string   `yaml:"addr"`
	Password    string   `yaml:"password"`
	DB          int      `yaml:"db"`
	PoolSize    int      `yaml:"pool_size"`
	DialTimeout Duration `yaml:"dial_timeout"`
}

type JWTConfig struct {
	Secret     string   `yaml:"secret"`
	Issuer     string   `yaml:"issuer"`
	AccessTTL  Duration `yaml:"access_ttl"`
	RefreshTTL Duration `yaml:"refresh_ttl"`
}

type SMSAliyunConfig struct {
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	SignName        string `yaml:"sign_name"`
	TemplateCode    string `yaml:"template_code"`
}

type SMSTencentConfig struct {
	SecretID   string `yaml:"secret_id"`
	SecretKey  string `yaml:"secret_key"`
	SdkAppID   string `yaml:"sdk_app_id"`
	SignName   string `yaml:"sign_name"`
	TemplateID string `yaml:"template_id"`
}

// SMSWebhookConfig 通过 HTTP 网关转发短信。
// 适用于已有内部短信网关，或希望用一个 provider 桥接任意厂商的场景。
type SMSWebhookConfig struct {
	URL     string   `yaml:"url"`
	Token   string   `yaml:"token"`
	Timeout Duration `yaml:"timeout"`
}

type SMSConfig struct {
	Provider     string           `yaml:"provider"`
	CodeTTL      Duration         `yaml:"code_ttl"`
	SendInterval Duration         `yaml:"send_interval"`
	HourlyLimit  int              `yaml:"hourly_limit"`
	DailyLimit   int              `yaml:"daily_limit"`
	ExposeCode   bool             `yaml:"expose_code"`
	Aliyun       SMSAliyunConfig  `yaml:"aliyun"`
	Tencent      SMSTencentConfig `yaml:"tencent"`
	Webhook      SMSWebhookConfig `yaml:"webhook"`
}

type WechatConfig struct {
	AppID       string `yaml:"app_id"`
	AppSecret   string `yaml:"app_secret"`
	RedirectURI string `yaml:"redirect_uri"`
	Mock        bool   `yaml:"mock"`
}

// StorageLocalConfig 本地磁盘存储配置，开发环境默认使用。
type StorageLocalConfig struct {
	// Dir 上传根目录。相对路径以进程工作目录为基准。
	Dir string `yaml:"dir"`
}

// StorageConfig 对象存储配置。
//
// 开发默认走 local（落盘到 ./data/uploads），生产切 oss/cos 时
// 只要换 driver 并补上对应厂商凭据，业务代码不需要改动。
type StorageConfig struct {
	// Driver 取值 local / oss / cos。
	Driver string `yaml:"driver"`
	// PublicBaseURL 公开文件的访问前缀。
	// 留空时回落到 app.base_url，便于本地开发直接拿到可点的链接。
	PublicBaseURL string `yaml:"public_base_url"`
	// MaxSizeMB 单文件大小上限（MB）。这是全局兜底值，
	// 具体场景的限制在 storage.Policy 里，两者取小。
	MaxSizeMB int `yaml:"max_size_mb"`
	// SignSecret 私有文件访问签名密钥。
	// 留空时回落到 jwt.secret，让开发期少配一个变量；
	// 但生产环境会要求显式配置（见 Validate），避免密钥复用。
	SignSecret string             `yaml:"sign_secret"`
	Local      StorageLocalConfig `yaml:"local"`
}

type CORSConfig struct {
	AllowOrigins     []string `yaml:"allow_origins"`
	AllowMethods     []string `yaml:"allow_methods"`
	AllowHeaders     []string `yaml:"allow_headers"`
	ExposeHeaders    []string `yaml:"expose_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           Duration `yaml:"max_age"`
}

// WebConfig 控制前端页面的托管方式。
type WebConfig struct {
	// Enabled 为 false 时本服务只提供 API，根路径不再返回页面。
	// 前后端分离部署（前端走 CDN 或独立 Nginx）时需要关掉，
	// 否则根路径会被本服务占住。
	Enabled bool `yaml:"enabled"`
	// Dir 前端资源的磁盘目录。留空时使用编译期内嵌的副本——
	// 这是生产默认值，因为二进制自包含才不会出现"页面没跟着一起发"。
	// 填了则优先读磁盘，便于改完页面不重新编译就能看到效果。
	Dir string `yaml:"dir"`
}

// PaymentConfig 支付与结算配置。
type PaymentConfig struct {
	// Provider 取值 mock / offline / wechat / alipay。
	//   - mock    ：直接置为支付成功，仅供本地联调（生产禁用）
	//   - offline ：线下转账，后台人工标记已支付，MVP 阶段可先这样跑
	//   - wechat / alipay：保留骨架，未接通前会返回"暂未开通"而不是假装成功
	Provider string `yaml:"provider"`

	// PayTimeout 待支付订单的有效期。超时后由定时任务自动取消，释放专家的接单名额。
	PayTimeout Duration `yaml:"pay_timeout"`

	// AutoConfirmAfter 服务完成后多久自动确认。
	// 没有这个兜底，用户忘了点确认的订单会永远卡在"待确认"，
	// 专家干了活却拿不到钱——这是比"误判完成"更严重的故障。
	AutoConfirmAfter Duration `yaml:"auto_confirm_after"`

	// CommissionRateBP 平台抽佣基点，1500 = 15%。
	// 用基点而不是百分比浮点：1.5% 这类比例写成 float 会有精度问题，
	// 而抽佣要参与金额计算，一分钱的误差在对账时都要人工核。
	CommissionRateBP int `yaml:"commission_rate_bp"`

	// NotifyURL 支付结果回调地址，真实通道需要。
	NotifyURL string `yaml:"notify_url"`
}

// SchedulerConfig 后台定时任务配置。
//
// 这些任务没有一个是"锦上添花"的：
//   - 支付超时不关单 → 专家的接单名额被占着，别人看不到这条悬赏；
//   - 到期不自动确认 → 用户忘了点确认，专家干了活永远拿不到钱；
//   - 结算不兜底     → 确认成功但结算那步失败的单子永远停在已完成。
//
// 所以默认开启。关掉只应发生在"任务改由外部调度器触发"的场景
// （多副本部署时也可以只让一个副本跑，虽然并发跑本身是安全的，见 scheduler 包注释）。
type SchedulerConfig struct {
	// Enabled 是否在本进程内运行定时任务。
	Enabled bool `yaml:"enabled"`

	// Interval 扫描周期。配得太小等于拿数据库当轮询靶子，
	// 配得太大则超时单要在库里多躺一会儿，一分钟是这几件事的合理粒度。
	Interval Duration `yaml:"interval"`

	// BatchSize 单轮每个任务最多处理多少条。
	// 有上限才能保证一轮的耗时可控——没有上限时，积压十万单的第一轮
	// 会把进程钉死在数据库上，HTTP 请求也跟着一起变慢。
	BatchSize int `yaml:"batch_size"`

	// JobTimeout 单个任务的执行超时。
	// 它防的是"某条 SQL 卡住"，而不是业务慢：超时后这一轮直接放弃剩下的，
	// 下一轮再来，因为三个任务的每个动作都是幂等的。
	JobTimeout Duration `yaml:"job_timeout"`

	// RunOnStart 启动时是否立刻跑一轮。
	// 默认开：进程重启后积压的单子不必等第一个周期才被处理；
	// 也让运维在启动日志里立刻看到"任务确实跑起来了"。
	RunOnStart bool `yaml:"run_on_start"`
}

type Config struct {
	App       AppConfig       `yaml:"app"`
	Log       LogConfig       `yaml:"log"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	JWT       JWTConfig       `yaml:"jwt"`
	SMS       SMSConfig       `yaml:"sms"`
	Wechat    WechatConfig    `yaml:"wechat"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Storage   StorageConfig   `yaml:"storage"`
	Payment   PaymentConfig   `yaml:"payment"`
	CORS      CORSConfig      `yaml:"cors"`
	Web       WebConfig       `yaml:"web"`

	path string
}

// FileSignSecret 返回私有文件签名密钥。
// 未显式配置时回落到 JWT 密钥，保证开发期零配置可用。
func (c *Config) FileSignSecret() string {
	if s := strings.TrimSpace(c.Storage.SignSecret); s != "" {
		return s
	}
	return c.JWT.Secret
}

// PublicFileBaseURL 返回公开文件的可访问前缀，末尾不带斜杠。
func (c *Config) PublicFileBaseURL() string {
	base := strings.TrimSpace(c.Storage.PublicBaseURL)
	if base == "" {
		base = strings.TrimSpace(c.App.BaseURL)
	}
	return strings.TrimRight(base, "/")
}

// IsProd 判断是否生产环境。
func (c *Config) IsProd() bool { return c.App.Env == "prod" }

// ExposeAPIDocs 决定是否对外提供 API 文档。
//
// 默认只在本机与测试环境开放：/openapi.yaml 写的是全部接口，含后台管理接口的
// 路径、字段与错误码，公开挂出去等于替扫描器省掉逆向这一步。线上站点收手机号、
// 走支付，没有对外提供接口文档的理由。需要给合作方看时在配置里写
// `app.expose_docs: true` 显式打开，而不是靠改代码重新发版。
func (c *Config) ExposeAPIDocs() bool {
	if c.App.ExposeDocs != nil {
		return *c.App.ExposeDocs
	}
	return !c.IsProd()
}

// IsDev 判断是否开发环境。
func (c *Config) IsDev() bool { return c.App.Env == "dev" }

// Source 返回实际加载的配置文件路径，未加载时为空。
func (c *Config) Source() string { return c.path }

// MemCache 表示是否降级为进程内缓存。
func (c *Config) MemCache() bool { return strings.TrimSpace(c.Redis.Addr) == "" }

func defaults() *Config {
	return &Config{
		App: AppConfig{
			Name:            "ops-expert-platform",
			Env:             "dev",
			Addr:            ":8080",
			BaseURL:         "http://localhost:8080",
			FrontendURL:     "http://localhost:3000",
			ReadTimeout:     Duration(15 * time.Second),
			WriteTimeout:    Duration(30 * time.Second),
			ShutdownTimeout: Duration(10 * time.Second),
		},
		Log: LogConfig{Level: "info", Format: "text"},
		Database: DatabaseConfig{
			Driver:          "sqlite",
			SQLitePath:      "./data/ops_expert.db",
			MaxOpenConns:    50,
			MaxIdleConns:    10,
			ConnMaxLifetime: Duration(time.Hour),
			AutoMigrate:     true,
			LogLevel:        "warn",
		},
		Redis: RedisConfig{PoolSize: 20, DialTimeout: Duration(3 * time.Second)},
		JWT: JWTConfig{
			// 这是开发默认值，Validate 会在生产环境直接拒绝它，
			// 目的是让 `go run ./cmd/api` 零配置就能起来，同时不给生产留后门。
			Secret:     "dev-only-secret-change-me-0123456789abcdef",
			Issuer:     "ops-expert-platform",
			AccessTTL:  Duration(2 * time.Hour),
			RefreshTTL: Duration(168 * time.Hour),
		},
		SMS: SMSConfig{
			Provider:     "mock",
			CodeTTL:      Duration(5 * time.Minute),
			SendInterval: Duration(60 * time.Second),
			HourlyLimit:  5,
			DailyLimit:   20,
			Webhook:      SMSWebhookConfig{Timeout: Duration(5 * time.Second)},
		},
		Wechat: WechatConfig{Mock: true},
		Payment: PaymentConfig{
			Provider:         "mock",
			PayTimeout:       Duration(15 * time.Minute),
			AutoConfirmAfter: Duration(48 * time.Hour),
			CommissionRateBP: 1500,
		},
		Storage: StorageConfig{
			Driver:    "local",
			MaxSizeMB: 50,
			Local:     StorageLocalConfig{Dir: "./data/uploads"},
		},
		Scheduler: SchedulerConfig{
			Enabled:    true,
			Interval:   Duration(time.Minute),
			BatchSize:  100,
			JobTimeout: Duration(30 * time.Second),
			RunOnStart: true,
		},
		CORS: CORSConfig{
			AllowOrigins:     []string{"http://localhost:3000", "http://localhost:5173"},
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-Id"},
			ExposeHeaders:    []string{"X-Request-Id"},
			AllowCredentials: true,
			MaxAge:           Duration(12 * time.Hour),
		},
		// 默认开启页面托管且不指定磁盘目录，即"用编译期内嵌的那份"。
		// 本地开发想改页面立刻见效时，把 Web.Dir 指向 server/web/static 即可。
		Web: WebConfig{Enabled: true},
	}
}

// Load 按给定路径加载配置；path 为空时只使用默认值 + 环境变量。
func Load(path string) (*Config, error) {
	loadDotEnv()

	cfg := defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, cfg); err != nil {
				return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
			}
			cfg.path = path
		case os.IsNotExist(err):
			// 配置文件缺失是允许的（纯环境变量模式）
		default:
			return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
		}
	}

	applyEnv(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func loadDotEnv() {
	for _, p := range []string{".env", "configs/.env", "../.env"} {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			return
		}
	}
}

func applyEnv(c *Config) {
	setStr(&c.App.Env, "APP_ENV")
	setStr(&c.App.Addr, "APP_ADDR")
	setStr(&c.App.BaseURL, "APP_BASE_URL")
	setStr(&c.App.FrontendURL, "APP_FRONTEND_URL")

	setStr(&c.Log.Level, "LOG_LEVEL")
	setStr(&c.Log.Format, "LOG_FORMAT")
	setBool(&c.Log.AddSource, "LOG_ADD_SOURCE")

	setStr(&c.Database.Driver, "DB_DRIVER")
	setStr(&c.Database.SQLitePath, "DB_SQLITE_PATH")
	setStr(&c.Database.DSN, "DB_DSN")
	setInt(&c.Database.MaxOpenConns, "DB_MAX_OPEN_CONNS")
	setInt(&c.Database.MaxIdleConns, "DB_MAX_IDLE_CONNS")
	setDur(&c.Database.ConnMaxLifetime, "DB_CONN_MAX_LIFETIME")
	setBool(&c.Database.AutoMigrate, "DB_AUTO_MIGRATE")
	setStr(&c.Database.LogLevel, "DB_LOG_LEVEL")

	setStr(&c.Redis.Addr, "REDIS_ADDR")
	setStr(&c.Redis.Password, "REDIS_PASSWORD")
	setInt(&c.Redis.DB, "REDIS_DB")
	setInt(&c.Redis.PoolSize, "REDIS_POOL_SIZE")

	setStr(&c.JWT.Secret, "JWT_SECRET")
	setStr(&c.JWT.Issuer, "JWT_ISSUER")
	setDur(&c.JWT.AccessTTL, "JWT_ACCESS_TTL")
	setDur(&c.JWT.RefreshTTL, "JWT_REFRESH_TTL")

	setStr(&c.SMS.Provider, "SMS_PROVIDER")
	setDur(&c.SMS.CodeTTL, "SMS_CODE_TTL")
	setDur(&c.SMS.SendInterval, "SMS_SEND_INTERVAL")
	setInt(&c.SMS.HourlyLimit, "SMS_HOURLY_LIMIT")
	setInt(&c.SMS.DailyLimit, "SMS_DAILY_LIMIT")
	setBool(&c.SMS.ExposeCode, "SMS_EXPOSE_CODE")
	setStr(&c.SMS.Webhook.URL, "SMS_WEBHOOK_URL")
	setStr(&c.SMS.Webhook.Token, "SMS_WEBHOOK_TOKEN")

	setStr(&c.Wechat.AppID, "WECHAT_APP_ID")
	setStr(&c.Wechat.AppSecret, "WECHAT_APP_SECRET")
	setStr(&c.Wechat.RedirectURI, "WECHAT_REDIRECT_URI")
	setBool(&c.Wechat.Mock, "WECHAT_MOCK")

	setStr(&c.Storage.Driver, "STORAGE_DRIVER")
	setStr(&c.Storage.PublicBaseURL, "STORAGE_PUBLIC_BASE_URL")
	setInt(&c.Storage.MaxSizeMB, "STORAGE_MAX_SIZE_MB")
	setStr(&c.Storage.SignSecret, "STORAGE_SIGN_SECRET")
	setStr(&c.Storage.Local.Dir, "STORAGE_LOCAL_DIR")

	setStr(&c.Payment.Provider, "PAYMENT_PROVIDER")
	setDur(&c.Payment.PayTimeout, "PAYMENT_PAY_TIMEOUT")
	setDur(&c.Payment.AutoConfirmAfter, "PAYMENT_AUTO_CONFIRM_AFTER")
	setInt(&c.Payment.CommissionRateBP, "PAYMENT_COMMISSION_RATE_BP")
	setStr(&c.Payment.NotifyURL, "PAYMENT_NOTIFY_URL")

	setBool(&c.Web.Enabled, "WEB_ENABLED")
	setStr(&c.Web.Dir, "WEB_DIR")

	setBool(&c.Scheduler.Enabled, "SCHEDULER_ENABLED")
	setDur(&c.Scheduler.Interval, "SCHEDULER_INTERVAL")
	setInt(&c.Scheduler.BatchSize, "SCHEDULER_BATCH_SIZE")
	setDur(&c.Scheduler.JobTimeout, "SCHEDULER_JOB_TIMEOUT")
	setBool(&c.Scheduler.RunOnStart, "SCHEDULER_RUN_ON_START")
}

func setStr(dst *string, key string) {
	if v, ok := os.LookupEnv(key); ok {
		*dst = v
	}
}

func setBool(dst *bool, key string) {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			*dst = b
		}
	}
}

func setInt(dst *int, key string) {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			*dst = n
		}
	}
}

func setDur(dst *Duration, key string) {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
			*dst = Duration(d)
		}
	}
}

// Validate 校验必填项与数值边界。
func (c *Config) Validate() error {
	switch c.App.Env {
	case "dev", "test", "prod":
	default:
		return fmt.Errorf("app.env 取值非法: %q（应为 dev/test/prod）", c.App.Env)
	}
	if c.App.Addr == "" {
		return fmt.Errorf("app.addr 不能为空")
	}

	switch c.Database.Driver {
	case "mysql":
		if strings.TrimSpace(c.Database.DSN) == "" {
			return fmt.Errorf("database.driver=mysql 时 database.dsn 不能为空")
		}
	case "sqlite":
		if strings.TrimSpace(c.Database.SQLitePath) == "" {
			return fmt.Errorf("database.driver=sqlite 时 database.sqlite_path 不能为空")
		}
	default:
		return fmt.Errorf("database.driver 取值非法: %q（应为 mysql/sqlite）", c.Database.Driver)
	}

	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("jwt.secret 长度不足 32 字节，存在被爆破风险")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("jwt 过期时间必须为正数")
	}
	if c.JWT.RefreshTTL <= c.JWT.AccessTTL {
		return fmt.Errorf("jwt.refresh_ttl 必须大于 jwt.access_ttl")
	}

	switch c.SMS.Provider {
	case "mock", "aliyun", "tencent", "webhook":
	default:
		return fmt.Errorf("sms.provider 取值非法: %q（应为 mock/aliyun/tencent/webhook）", c.SMS.Provider)
	}
	if c.SMS.Provider == "webhook" && strings.TrimSpace(c.SMS.Webhook.URL) == "" {
		return fmt.Errorf("sms.provider=webhook 时 sms.webhook.url 不能为空")
	}

	switch c.Storage.Driver {
	case "local", "oss", "cos":
	default:
		return fmt.Errorf("storage.driver 取值非法: %q（应为 local/oss/cos）", c.Storage.Driver)
	}
	if c.Storage.MaxSizeMB <= 0 {
		return fmt.Errorf("storage.max_size_mb 必须为正数")
	}
	if c.Storage.Driver == "local" && strings.TrimSpace(c.Storage.Local.Dir) == "" {
		return fmt.Errorf("storage.driver=local 时 storage.local.dir 不能为空")
	}
	if c.Storage.Driver != "local" && strings.TrimSpace(c.Storage.PublicBaseURL) == "" {
		return fmt.Errorf("storage.driver=%s 时 storage.public_base_url 不能为空", c.Storage.Driver)
	}

	switch c.Payment.Provider {
	case "mock", "offline", "wechat", "alipay":
	default:
		return fmt.Errorf("payment.provider 取值非法: %q（应为 mock/offline/wechat/alipay）", c.Payment.Provider)
	}
	if c.Payment.PayTimeout <= 0 {
		return fmt.Errorf("payment.pay_timeout 必须为正数")
	}
	if c.Payment.AutoConfirmAfter <= 0 {
		return fmt.Errorf("payment.auto_confirm_after 必须为正数")
	}
	// 抽佣超过 50% 基本可以断定是把基点当成了百分比填（1500 想表达 15 却填了 1500%）。
	// 这种错不会报错，只会在结算时把专家的收入算成负的，所以在这里拦掉。
	if c.Payment.CommissionRateBP < 0 || c.Payment.CommissionRateBP > 5000 {
		return fmt.Errorf("payment.commission_rate_bp 应在 0-5000 之间（基点，1500 = 15%%），当前 %d",
			c.Payment.CommissionRateBP)
	}

	// 定时任务的边界一律显式校验，不做"超范围就退回默认值"的宽容处理：
	// interval 写成 30h（本意 30s）时，如果静默改回 1m，问题是解决了；
	// 但写成 30m（本意 30s）时不会报错也不违反下限，超时单就会在库里多躺半小时，
	// 而这种偏差从外部完全看不出来。宁可启动失败，也不要跑一个节奏不对的任务。
	if c.Scheduler.Interval.Std() < time.Second || c.Scheduler.Interval.Std() > 24*time.Hour {
		return fmt.Errorf("scheduler.interval 应在 1s-24h 之间（如 30s / 1m），当前 %s",
			c.Scheduler.Interval.Std())
	}
	if c.Scheduler.BatchSize <= 0 || c.Scheduler.BatchSize > 1000 {
		return fmt.Errorf("scheduler.batch_size 应在 1-1000 之间，当前 %d", c.Scheduler.BatchSize)
	}
	if c.Scheduler.JobTimeout.Std() <= 0 || c.Scheduler.JobTimeout.Std() > 10*time.Minute {
		return fmt.Errorf("scheduler.job_timeout 应在 1s-10m 之间，当前 %s", c.Scheduler.JobTimeout.Std())
	}

	// 生产环境的红线：不允许把验证码回传给前端，不允许使用 mock 通道。
	if c.IsProd() {
		if c.SMS.ExposeCode {
			return fmt.Errorf("生产环境禁止开启 sms.expose_code")
		}
		if c.SMS.Provider == "mock" {
			return fmt.Errorf("生产环境禁止使用 sms.provider=mock")
		}
		if c.Wechat.Mock {
			return fmt.Errorf("生产环境禁止开启 wechat.mock")
		}
		if c.Payment.Provider == "mock" {
			return fmt.Errorf("生产环境禁止使用 payment.provider=mock（它会把订单直接置为已支付）")
		}
		if strings.Contains(c.JWT.Secret, "change-me") || strings.Contains(c.JWT.Secret, "dev-only") {
			return fmt.Errorf("生产环境必须替换 jwt.secret 默认值")
		}
	}
	return nil
}
