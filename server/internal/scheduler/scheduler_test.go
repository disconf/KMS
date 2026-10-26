package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opsexpert/platform/internal/config"
)

// discardLogger 让测试输出保持干净：这里断言的是行为，不是日志格式。
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig(enabled bool) config.SchedulerConfig {
	return config.SchedulerConfig{
		Enabled:    enabled,
		Interval:   config.Duration(50 * time.Millisecond),
		BatchSize:  10,
		JobTimeout: config.Duration(500 * time.Millisecond),
		RunOnStart: true,
	}
}

func TestNewRejectsInvalidJobs(t *testing.T) {
	cases := []struct {
		name string
		job  Job
	}{
		{"缺少 Name", Job{Run: func(context.Context) (int, error) { return 0, nil }}},
		{"缺少 Run", Job{Name: "no-run"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := New(testConfig(true), discardLogger(), c.job); err == nil {
				t.Fatal("应当拒绝不合法的任务定义")
			}
		})
	}
}

func TestNewRejectsNonPositiveInterval(t *testing.T) {
	if _, err := New(config.SchedulerConfig{Interval: 0, JobTimeout: config.Duration(time.Second)},
		discardLogger()); err == nil {
		t.Fatal("interval 为 0 时应当报错，而不是退化成一个空转的循环")
	}
}

// RunOnce 是测试与手工补跑的入口，注册顺序必须就是执行顺序。
func TestRunOnceRunsAllJobsInOrder(t *testing.T) {
	var order []string

	s, err := New(testConfig(false), discardLogger(),
		Job{Name: "first", Run: func(context.Context) (int, error) {
			order = append(order, "first")
			// 睡一下再返回：Windows 上 time.Now 的粒度足以把"空函数"量成 0，
			// 拿空函数去断言"耗时为正值"会得到一个与实现无关的失败。
			time.Sleep(5 * time.Millisecond)
			return 3, nil
		}},
		Job{Name: "second", Run: func(context.Context) (int, error) {
			order = append(order, "second")
			return 0, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	results := s.RunOnce(context.Background())
	if len(results) != 2 {
		t.Fatalf("应当返回 2 条结果，实际 %d", len(results))
	}
	if results[0].Job != "first" || results[1].Job != "second" {
		t.Fatalf("执行顺序应与注册顺序一致，实际 %v", order)
	}
	if results[0].Processed != 3 || results[1].Processed != 0 {
		t.Fatalf("处理条数不应被改写，实际 %d / %d", results[0].Processed, results[1].Processed)
	}
	if results[0].Duration < time.Millisecond {
		t.Fatalf("耗时应当覆盖执行体本身，实际 %s", results[0].Duration)
	}
}

// 后台任务里的 panic 会终结整个进程，所以必须就地隔离：
// 后面的任务照常执行，这一条只留下错误。
func TestRunOnceIsolatesPanic(t *testing.T) {
	var secondRan atomic.Bool

	s, err := New(testConfig(false), discardLogger(),
		Job{Name: "boom", Run: func(context.Context) (int, error) {
			panic("模拟的数据异常")
		}},
		Job{Name: "next", Run: func(context.Context) (int, error) {
			secondRan.Store(true)
			return 1, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	results := s.RunOnce(context.Background())

	if results[0].Err == nil {
		t.Fatal("panic 必须被转成错误返回")
	}
	if !secondRan.Load() {
		t.Fatal("前一个任务 panic 后，后面的任务仍应执行")
	}
	if results[1].Err != nil || results[1].Processed != 1 {
		t.Fatalf("后续任务不应受影响，实际 err=%v processed=%d", results[1].Err, results[1].Processed)
	}
}

// 单个任务的失败不能影响其他任务：这正是分成多个 goroutine 的理由。
func TestRunOnceIsolatesError(t *testing.T) {
	sentinel := errors.New("数据库连接断了")

	s, err := New(testConfig(false), discardLogger(),
		Job{Name: "fails", Run: func(context.Context) (int, error) { return 0, sentinel }},
		Job{Name: "ok", Run: func(context.Context) (int, error) { return 2, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}

	results := s.RunOnce(context.Background())
	if !errors.Is(results[0].Err, sentinel) {
		t.Fatalf("错误应原样带出来（便于上层 errors.Is 判断），实际 %v", results[0].Err)
	}
	if results[1].Err != nil || results[1].Processed != 2 {
		t.Fatalf("后面那个任务不该被牵连，实际 err=%v processed=%d", results[1].Err, results[1].Processed)
	}
}

// 一个卡死的任务必须在超时后被放弃，否则它会永久占住自己那个 goroutine。
func TestJobTimeoutAbandonsStuckJob(t *testing.T) {
	var ctxErr error

	s, err := New(testConfig(false), discardLogger(),
		Job{
			Name:    "stuck",
			Timeout: 60 * time.Millisecond,
			Run: func(ctx context.Context) (int, error) {
				<-ctx.Done()
				ctxErr = ctx.Err()
				return 0, ctx.Err()
			},
		},
		Job{Name: "fine", Run: func(context.Context) (int, error) { return 1, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	results := s.RunOnce(context.Background())
	elapsed := time.Since(started)

	if !errors.Is(ctxErr, context.DeadlineExceeded) {
		t.Fatalf("任务内应看到 ctx 超时，实际 %v", ctxErr)
	}
	if !errors.Is(results[0].Err, context.DeadlineExceeded) {
		t.Fatalf("超时应作为错误返回，实际 %v", results[0].Err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("超时后应立即返回，实际耗时 %s", elapsed)
	}
	if results[1].Processed != 1 {
		t.Fatal("卡死的任务不应拖住后面的任务")
	}
}

// 未指定 Interval 的任务跟随全局周期；指定了则用自己的。
func TestPerJobIntervalFallsBackToGlobal(t *testing.T) {
	global := 50 * time.Millisecond
	if got := (Job{Name: "a"}).interval(global); got != global {
		t.Fatalf("未指定周期时应回落到全局周期，实际 %s", got)
	}
	if got := (Job{Name: "a", Interval: time.Hour}).interval(global); got != time.Hour {
		t.Fatalf("指定了周期就应当用自己的，实际 %s", got)
	}
}

// Start 之后既要跑启动那一轮，也要按周期继续跑。
func TestStartRunsOnStartAndKeepsTicking(t *testing.T) {
	var count atomic.Int32

	s, err := New(testConfig(true), discardLogger(),
		Job{Name: "tick", Run: func(context.Context) (int, error) {
			count.Add(1)
			return 0, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// 周期 50ms + 抖动最多 50ms，250ms 内至少应有启动 1 次 + 周期 2 次。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && count.Load() < 3 {
		time.Sleep(10 * time.Millisecond)
	}

	if got := count.Load(); got < 3 {
		t.Fatalf("启动后应持续执行，实际只跑了 %d 次", got)
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("停止失败: %v", err)
	}

	// 停完之后不应再有新的执行，否则说明 goroutine 没退干净。
	after := count.Load()
	time.Sleep(200 * time.Millisecond)
	if got := count.Load(); got != after {
		t.Fatalf("Stop 之后仍在执行：%d -> %d", after, got)
	}
}

func TestStartRejectsSecondCall(t *testing.T) {
	s, err := New(testConfig(true), discardLogger(),
		Job{Name: "noop", Run: func(context.Context) (int, error) { return 0, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx); err == nil {
		t.Fatal("重复启动应当报错：启动两份会让日志里的处理条数凭空翻倍")
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	_ = s.Stop(stopCtx)
}

// Stop 要等在跑的那一轮结束，而不是把它从中打断。
func TestStopWaitsForRunningJob(t *testing.T) {
	started := make(chan struct{})
	var finished atomic.Bool

	s, err := New(testConfig(true), discardLogger(),
		Job{Name: "slow", Run: func(context.Context) (int, error) {
			close(started)
			time.Sleep(120 * time.Millisecond)
			finished.Store(true)
			return 1, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("任务没有被启动")
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelStop()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if !finished.Load() {
		t.Fatal("Stop 应当等这一轮跑完，而不是中途打断（打断结算比跑完更危险）")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	s, err := New(testConfig(true), discardLogger(),
		Job{Name: "noop", Run: func(context.Context) (int, error) { return 0, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), time.Second)
		err := s.Stop(stopCtx)
		cancelStop()
		if err != nil {
			t.Fatalf("第 %d 次 Stop 失败: %v", i+1, err)
		}
	}
}

// 关掉调度器时 Start 不应执行任何任务，但仍允许手动 RunOnce（事故后补跑）。
func TestDisabledSchedulerDoesNotRun(t *testing.T) {
	var count atomic.Int32

	s, err := New(testConfig(false), discardLogger(),
		Job{Name: "tick", Run: func(context.Context) (int, error) {
			count.Add(1)
			return 0, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)
	if got := count.Load(); got != 0 {
		t.Fatalf("enabled=false 时不应执行任务，实际跑了 %d 次", got)
	}

	if res := s.RunOnce(context.Background()); len(res) != 1 || count.Load() != 1 {
		t.Fatal("手动触发不受 enabled 影响（这是运维补跑的唯一手段）")
	}
}

// parent ctx 取消等同于停机：main 直接把信号 ctx 传进来。
func TestCancelParentStopsLoops(t *testing.T) {
	var count atomic.Int32

	s, err := New(testConfig(true), discardLogger(),
		Job{Name: "tick", Run: func(context.Context) (int, error) {
			count.Add(1)
			return 0, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	time.Sleep(120 * time.Millisecond)
	cancel()

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatalf("父 ctx 取消后 Stop 应立即返回，实际 %v", err)
	}

	after := count.Load()
	time.Sleep(150 * time.Millisecond)
	if got := count.Load(); got != after {
		t.Fatalf("父 ctx 取消后仍在执行：%d -> %d", after, got)
	}
}
