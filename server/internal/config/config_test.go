package config

import (
	"strings"
	"testing"
	"time"
)

// 默认配置必须能直接启动：这是"go run ./cmd/api 零配置就能跑"的前提。
func TestDefaultsAreValid(t *testing.T) {
	if err := defaults().Validate(); err != nil {
		t.Fatalf("默认配置不应校验失败: %v", err)
	}
}

// 定时任务的边界一律显式报错，不做"超范围就退回默认值"的宽容处理。
// 静默纠正会让一个配错的节奏以"订单状态怎么不动了"的形式在几天后暴露。
func TestSchedulerValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:   "正常配置",
			mutate: func(c *Config) { c.Scheduler.Interval = Duration(time.Minute) },
		},
		{
			name:    "周期为零",
			mutate:  func(c *Config) { c.Scheduler.Interval = 0 },
			wantErr: "scheduler.interval",
		},
		{
			name:    "周期小于 1s",
			mutate:  func(c *Config) { c.Scheduler.Interval = Duration(500 * time.Millisecond) },
			wantErr: "scheduler.interval",
		},
		{
			name:    "周期超过 24h（多半是单位写错）",
			mutate:  func(c *Config) { c.Scheduler.Interval = Duration(30 * time.Hour) },
			wantErr: "scheduler.interval",
		},
		{
			name:    "批量为零",
			mutate:  func(c *Config) { c.Scheduler.BatchSize = 0 },
			wantErr: "scheduler.batch_size",
		},
		{
			name:    "批量过大",
			mutate:  func(c *Config) { c.Scheduler.BatchSize = 1001 },
			wantErr: "scheduler.batch_size",
		},
		{
			name:    "任务超时为零",
			mutate:  func(c *Config) { c.Scheduler.JobTimeout = 0 },
			wantErr: "scheduler.job_timeout",
		},
		{
			name:    "任务超时过长",
			mutate:  func(c *Config) { c.Scheduler.JobTimeout = Duration(11 * time.Minute) },
			wantErr: "scheduler.job_timeout",
		},
		{
			// 关掉调度器不等于字段可以随便填：这些值在下次打开时就会生效，
			// 到时候没人记得当初填的是什么。
			name: "关掉调度器时字段仍要合法",
			mutate: func(c *Config) {
				c.Scheduler.Enabled = false
				c.Scheduler.Interval = 0
			},
			wantErr: "scheduler.interval",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := defaults()
			c.mutate(cfg)
			err := cfg.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("不应报错，实际: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("应当报错并提到 %s", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("错误信息应提到 %s，实际: %v", c.wantErr, err)
			}
		})
	}
}

// API 文档的默认策略：线上不暴露，其它环境暴露。
// 这条默认值是安全边界，不能靠"部署时记得改配置"来保证。
func TestExposeAPIDocsDefault(t *testing.T) {
	tru := true
	fals := false

	prod := &Config{App: AppConfig{Env: "prod"}}
	if prod.ExposeAPIDocs() {
		t.Fatal("prod 未显式配置时应关闭 /docs 与 /openapi.yaml")
	}

	for _, env := range []string{"dev", "test"} {
		c := &Config{App: AppConfig{Env: env}}
		if !c.ExposeAPIDocs() {
			t.Fatalf("%s 未显式配置时应开放文档（本地与测试要用）", env)
		}
	}

	// 显式配置优先级最高：生产要临时给合作方看文档，改配置即可
	if (&Config{App: AppConfig{Env: "prod", ExposeDocs: &tru}}).ExposeAPIDocs() != true {
		t.Fatal("expose_docs: true 应在 prod 也生效")
	}
	if (&Config{App: AppConfig{Env: "test", ExposeDocs: &fals}}).ExposeAPIDocs() != false {
		t.Fatal("expose_docs: false 应在测试环境也生效")
	}
}
