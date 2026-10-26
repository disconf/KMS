// Package scheduler 提供进程内的周期任务调度。
//
// 只做一件事：按周期调用注册进来的 Job，并把每次执行的结果记进日志。
// 不引入 cron 库有两个原因——这个项目只有三个固定任务，不需要表达式解析；
// 而第三方 cron 多半自带"错过就补跑"的语义，对这里的任务反而是负担
// （见下面关于不排队积压的说明）。
//
// # 为什么每个 Job 一个 goroutine
//
// 三个任务共享一个循环的话，任何一次慢查询都会让另外两个跟着延后。
// 分开之后，超时关单被大批积压拖慢时，自动确认与结算兜底照常按自己的节奏跑。
//
// # 为什么 panic 必须就地 recover
//
// 定时任务是后台跑的：goroutine 里逃出去的 panic 会直接终结整个进程，
// 而它发生时没有请求上下文、没有任何人盯着屏幕。一次数据异常就能把线上服务打挂，
// 所以每个 Job 的执行都包在 recover 里，打完整堆栈，然后等下一轮。
//
// # 为什么不给积压的 tick 排队
//
// time.Ticker 在消费者跟不上时只保留最近一个 tick，其余丢弃。这正是需要的语义：
// 如果一轮 30 秒扫描花了 5 分钟，排队补跑 10 次没有意义——积压已经被这一轮处理了，
// 下一次跑最新状态就行。反过来若真的排队，进程只会越跑越滞后。
//
// # 多个副本同时跑是安全的
//
// 调度器本身不做分布式互斥（不抢锁、不选主）。这是刻意的：
// 三个任务在订单侧都是幂等的——状态流转是 CAS（UPDATE ... WHERE status = from），
// 结算与评价分别有 settlements.order_id / reviews.order_id 唯一索引兜底。
// 所以两个副本同时扫到同一笔超时单，只会有一个关单成功，另一个拿到状态冲突并跳过。
// 如果将来加了不幂等的任务，必须先补分布式锁再注册进来，不能靠"应该只有一个副本"来保证。
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"time"

	"github.com/opsexpert/platform/internal/config"
)

// Job 是一个可被周期执行的任务。
type Job struct {
	// Name 任务名，只用于日志与启动清单。取"动词+对象"的形式，例如 cancel-expired-payments。
	Name string

	// Interval 该任务的执行周期。为 0 时使用调度器的全局周期。
	// 允许每个任务有各自的节奏：关单要快（用户在等着订单状态），
	// 结算兜底慢一点没关系（它只在异常路径上才有活干）。
	Interval time.Duration

	// Timeout 单次执行的超时。为 0 时使用调度器的全局超时。
	Timeout time.Duration

	// Run 执行体，返回本次处理了多少条。
	//
	// 必须幂等：超时、panic、多副本并发都会导致同一批数据被处理第二次。
	Run func(ctx context.Context) (int, error)
}

// interval 返回该任务实际使用的周期。
func (j Job) interval(fallback time.Duration) time.Duration {
	if j.Interval > 0 {
		return j.Interval
	}
	return fallback
}

// timeout 返回该任务实际使用的超时。
func (j Job) timeout(fallback time.Duration) time.Duration {
	if j.Timeout > 0 {
		return j.Timeout
	}
	return fallback
}

// Result 是一次执行的观测结果。RunOnce 返回它，测试与手动触发都靠这个判断。
type Result struct {
	Job       string
	Processed int
	Duration  time.Duration
	Err       error
}

// Scheduler 周期执行注册的 Job。
//
// 零值不可用，必须用 New 构造。
type Scheduler struct {
	jobs       []Job
	interval   time.Duration
	jobTimeout time.Duration
	runOnStart bool
	enabled    bool
	log        *slog.Logger
	// jitter 在启动时把多副本的相位错开，为 false 时（测试）不等待。
	jitter bool

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool

	// once 保证 Stop 里的 cancel 只做一次，重复 Stop 不 panic。
	stopOnce sync.Once
}

// New 构造调度器。cfg 的边界值已在 config.Validate 校验过，
// 这里再兜一层是为了防止绕过配置直接调用（测试与手动触发会这么做）。
func New(cfg config.SchedulerConfig, log *slog.Logger, jobs ...Job) (*Scheduler, error) {
	if cfg.Interval.Std() <= 0 {
		return nil, fmt.Errorf("scheduler: interval 必须为正数，当前 %s", cfg.Interval.Std())
	}
	if cfg.JobTimeout.Std() <= 0 {
		return nil, fmt.Errorf("scheduler: job_timeout 必须为正数，当前 %s", cfg.JobTimeout.Std())
	}
	if log == nil {
		log = slog.Default()
	}
	for i, j := range jobs {
		if j.Name == "" {
			return nil, fmt.Errorf("scheduler: 第 %d 个任务缺少 Name（日志里分不清是谁在报错）", i+1)
		}
		if j.Run == nil {
			return nil, fmt.Errorf("scheduler: 任务 %s 的 Run 为空", j.Name)
		}
	}
	return &Scheduler{
		jobs:       jobs,
		interval:   cfg.Interval.Std(),
		jobTimeout: cfg.JobTimeout.Std(),
		runOnStart: cfg.RunOnStart,
		enabled:    cfg.Enabled,
		log:        log.With("component", "scheduler"),
		jitter:     true,
	}, nil
}

// Jobs 返回注册的任务清单，供启动日志打印"到底启了哪些任务"。
// 关掉调度器时这一点尤其重要：没有这句日志，没人会注意到超时单不再被自动关闭。
func (s *Scheduler) Jobs() []Job {
	out := make([]Job, len(s.jobs))
	copy(out, s.jobs)
	return out
}

// Enabled 报告调度器是否真的会执行任务。
func (s *Scheduler) Enabled() bool { return s.enabled }

// Start 启动所有任务，立即返回。
//
// parent 被取消时所有任务随之停止（与 Stop 等价），因此把 main 的信号 ctx 传进来即可。
// 重复调用返回错误而不是静默忽略：重复启动通常意味着装配代码写错了，
// 而"启动了两份"会让日志里的处理条数翻倍，排查时非常误导。
func (s *Scheduler) Start(parent context.Context) error {
	if !s.enabled {
		// 关掉的时候必须留下痕迹。否则"定时任务没生效"会以
		// "订单状态怎么不动了"的形式在几天后才被发现。
		s.log.Warn("定时任务未启用，支付超时不会被自动关单、到期订单不会自动确认",
			"reason", "scheduler.enabled=false")
		return nil
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("scheduler: 已经启动过，不能重复启动")
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started = true
	s.mu.Unlock()

	for _, j := range s.jobs {
		s.wg.Add(1)
		go s.loop(ctx, j)
	}
	return nil
}

// Stop 停止调度并等待正在执行的那一轮结束。
//
// 传进来的 ctx 只用来限制等待时长，不会打断任务本身——打断一个跑到一半的结算
// 比让它跑完更危险。等到截止时间还没结束就返回 ctx 的错误，由调用方决定是否强退。
// 可重复调用。
func (s *Scheduler) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		cancel := s.cancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("等待定时任务退出超时: %w", ctx.Err())
	}
}

// RunOnce 立刻把所有任务各执行一次，返回每次的结果。
//
// 不做并发限制之外的任何编排：注册顺序即执行顺序，一个任务失败不影响后面的。
// 测试、手工触发（例如事故后补跑）都走这里。
func (s *Scheduler) RunOnce(ctx context.Context) []Result {
	out := make([]Result, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, s.runOne(ctx, j))
	}
	return out
}

// loop 是单个任务的执行循环。
func (s *Scheduler) loop(ctx context.Context, j Job) {
	defer s.wg.Done()

	iv := j.interval(s.interval)

	if s.runOnStart {
		s.runOne(ctx, j)
	}

	// 启动那一轮是确定性的（便于运维确认"任务真的跑起来了"），
	// 之后的相位加一点抖动：多个副本同时部署时，不错开就会在同一秒
	// 一起去扫同一批数据，白白放大瞬时压力。
	if s.jitter && iv > time.Millisecond {
		wait := time.Duration(rand.Int64N(int64(iv)))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}

	ticker := time.NewTicker(iv)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOne(ctx, j)
		}
	}
}

// runOne 执行一次任务，负责超时、panic 隔离与日志。
func (s *Scheduler) runOne(parent context.Context, j Job) (res Result) {
	res.Job = j.Name
	started := time.Now()

	ctx, cancel := context.WithTimeout(parent, j.timeout(s.jobTimeout))
	defer cancel()

	defer func() {
		res.Duration = time.Since(started)

		if r := recover(); r != nil {
			// 打完整堆栈。后台 panic 没人看得见"崩溃现场"，
			// 只留一行 "任务 panic" 等于什么线索都没给。
			res.Err = fmt.Errorf("panic: %v", r)
			s.log.Error("定时任务 panic，已隔离，等待下一轮",
				"job", j.Name, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			return
		}

		if res.Err != nil {
			// 超时和业务失败要分开记：前者说明这批数据量大或库慢，
			// 后者说明数据本身有问题，处理方式完全不同。
			attrs := []any{"job", j.Name, "err", res.Err.Error(),
				"processed", res.Processed, "duration", res.Duration.String()}
			if errors.Is(res.Err, context.DeadlineExceeded) && parent.Err() == nil {
				attrs = append(attrs, "timeout", j.timeout(s.jobTimeout).String())
			}
			s.log.Error("定时任务执行失败", attrs...)
			return
		}

		if res.Processed > 0 {
			s.log.Info("定时任务处理完成",
				"job", j.Name, "processed", res.Processed, "duration", res.Duration.String())
			return
		}
		// 空轮占绝大多数，用 debug 免得每分钟三条噪音把有用的日志淹掉。
		s.log.Debug("定时任务空轮", "job", j.Name, "duration", res.Duration.String())
	}()

	res.Processed, res.Err = j.Run(ctx)
	return res
}
