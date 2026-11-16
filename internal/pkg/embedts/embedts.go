/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package embedts 管理 STX 进程内嵌的 Prometheus tsdb + PromQL 引擎生命周期：打开/关闭数据目录、
// 构造查询引擎、向查询层（internal/pkg/promquery）与未来的抓取器/评估器暴露就绪探测与 Appender。
//
// Package embedts manages the lifecycle of the Prometheus tsdb + PromQL engine embedded inside the
// STX process: opening/closing the data directory, building the query engine, and exposing readiness
// probes plus an Appender to the query layer (internal/pkg/promquery) and future scraper/evaluator.
//
// 包级单例是刻意为之：进程内同一时间只允许打开一份 tsdb 目录（同目录双开会被 lockfile 拒绝），
// 与 internal/db 的 DB 全局例外同理——它本质上是进程级资源而非可注入的 service。
// The package-level singleton is deliberate: a process may only ever hold one tsdb open on a directory
// (a second open is rejected by the lockfile). Like internal/db's DB global exception, it is a
// process-level resource rather than an injectable service.
package embedts

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
)

// 内嵌存储与查询的默认值；与 config 侧的默认值保持一致（config 负责填默认值，这里做兜底防御）。
// Defaults for embedded storage and query; mirrors the config-side defaults (config fills them
// during load, these are defensive fallbacks when Options bypasses config).
const (
	// DefaultDir 是默认 tsdb 数据目录。 / DefaultDir is the default tsdb data directory.
	DefaultDir = "./data/tsdb"
	// DefaultRetention 是默认时间保留期（31 天）。 / DefaultRetention is the default time retention (31 days).
	DefaultRetention = 31 * 24 * time.Hour
	// DefaultQueryTimeout 是默认单次查询超时，与原远程 HTTP 客户端 8s 一致。
	// DefaultQueryTimeout is the default per-query timeout, matching the legacy 8s HTTP client timeout.
	DefaultQueryTimeout = 8 * time.Second
	// DefaultMaxSamples 是默认单次查询样本上限，与 Prometheus 默认一致。
	// DefaultMaxSamples is the default per-query sample limit, same as upstream Prometheus.
	DefaultMaxSamples = 50000000
	// DefaultLookbackDelta 是 PromQL 瞬时查询回看窗口，与 Prometheus 默认 5m 一致。
	// DefaultLookbackDelta is the PromQL instant-query lookback window, same as the Prometheus 5m default.
	DefaultLookbackDelta = 5 * time.Minute
)

// Options 是内嵌存储与查询引擎的全部启动参数。
// Options holds every startup parameter for the embedded storage and query engine.
type Options struct {
	// Dir 是 tsdb 数据目录（blocks + WAL + head）。
	// Dir is the tsdb data directory (blocks + WAL + head).
	Dir string

	// RetentionDuration 是时间保留期；映射 tsdb.Options.RetentionDuration（毫秒）。
	// RetentionDuration is the time-based retention; maps to tsdb.Options.RetentionDuration (ms).
	RetentionDuration time.Duration

	// MaxBytes 是块存储字节数上限，0 表示不限；映射 tsdb.Options.MaxBytes。
	// MaxBytes caps retained block bytes (0 = unlimited); maps to tsdb.Options.MaxBytes.
	MaxBytes int64

	// QueryTimeout 是单次 PromQL 查询超时；映射 promql.EngineOpts.Timeout。
	// QueryTimeout is the per-query timeout; maps to promql.EngineOpts.Timeout.
	QueryTimeout time.Duration

	// MaxSamples 是单次查询允许加载的最大样本数；映射 promql.EngineOpts.MaxSamples。
	// MaxSamples is the per-query sample limit; maps to promql.EngineOpts.MaxSamples.
	MaxSamples int

	// NoLockfile 透传 tsdb.Options.NoLockfile，为 Windows 等不支持锁文件的平台留口。
	// NoLockfile passes through to tsdb.Options.NoLockfile as an escape hatch for platforms
	// without lockfile support (e.g. Windows).
	NoLockfile bool
}

// withDefaults 填充零值字段，保证绕过 config 直接构造 Options 的调用方也能工作。
// withDefaults fills zero-value fields so callers constructing Options without config still work.
func (o Options) withDefaults() Options {
	if strings.TrimSpace(o.Dir) == "" {
		o.Dir = DefaultDir
	}
	if o.RetentionDuration <= 0 {
		o.RetentionDuration = DefaultRetention
	}
	if o.MaxBytes < 0 {
		o.MaxBytes = 0
	}
	if o.QueryTimeout <= 0 {
		o.QueryTimeout = DefaultQueryTimeout
	}
	if o.MaxSamples <= 0 {
		o.MaxSamples = DefaultMaxSamples
	}
	return o
}

// OptionsFromObservability 把可观测配置翻译为启动参数；时长字符串按 Prometheus duration 语法解析。
// OptionsFromObservability translates observability config into startup Options; duration strings
// are parsed with the Prometheus duration syntax (s/m/h/d/w/y).
func OptionsFromObservability(obs config.ObservabilityConfig) (Options, error) {
	opts := Options{
		Dir:        strings.TrimSpace(obs.Storage.Dir),
		MaxBytes:   obs.Storage.MaxSize,
		MaxSamples: int(obs.MaxSamples),
	}
	if trimmed := strings.TrimSpace(obs.Storage.Retention); trimmed != "" {
		d, err := model.ParseDuration(trimmed)
		if err != nil {
			return Options{}, fmt.Errorf("observability.storage.retention %q: %w", trimmed, err)
		}
		opts.RetentionDuration = time.Duration(d)
	}
	if trimmed := strings.TrimSpace(obs.QueryTimeout); trimmed != "" {
		d, err := model.ParseDuration(trimmed)
		if err != nil {
			return Options{}, fmt.Errorf("observability.query_timeout %q: %w", trimmed, err)
		}
		opts.QueryTimeout = time.Duration(d)
	}
	return opts.withDefaults(), nil
}

// instance 持有一份已打开的内嵌存储与查询引擎。
// instance holds one opened embedded storage + query engine pair.
type instance struct {
	opts      Options
	db        *tsdb.DB
	engine    *promql.Engine
	queryable storage.Queryable
	registry  *prometheus.Registry
}

// 进程级单例：见包注释。读路径走 RLock，启停走 Lock。
// Process-level singleton: see package doc. Read paths take RLock, start/stop take Lock.
var (
	instanceMu sync.RWMutex
	current    *instance
)

// Start 按参数打开 tsdb 并构造 PromQL 引擎，登记为包级实例；重复 Start 返回错误。
// 打开失败不 panic——调用方应只记日志并让大盘走降级路径（Ready()=false）。
// Start opens the tsdb and builds the PromQL engine per Options, registering the package-level
// instance; a second Start returns an error. Open failures never panic—callers should only log
// and let dashboards degrade (Ready()=false).
func Start(ctx context.Context, opts Options) error {
	opts = opts.withDefaults()
	reg := prometheus.NewRegistry()
	tsdbOpts := tsdb.DefaultOptions()
	tsdbOpts.RetentionDuration = opts.RetentionDuration.Milliseconds()
	tsdbOpts.MaxBytes = opts.MaxBytes
	tsdbOpts.NoLockfile = opts.NoLockfile
	// 内部日志走 NopLogger：错误经返回值与 our logger.*F 上报，避免 zap/slog 双日志体系打架。
	// Internal logs go to a NopLogger: failures surface via return values and our logger.*F,
	// avoiding a second slog pipeline next to zap.
	db, err := tsdb.Open(opts.Dir, promslog.NewNopLogger(), reg, tsdbOpts, nil)
	if err != nil {
		return fmt.Errorf("open embedded tsdb at %q: %w", opts.Dir, err)
	}
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:               promslog.NewNopLogger(),
		Reg:                  reg,
		MaxSamples:           opts.MaxSamples,
		Timeout:              opts.QueryTimeout,
		LookbackDelta:        DefaultLookbackDelta,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})
	inst := &instance{
		opts:      opts,
		db:        db,
		engine:    engine,
		queryable: storage.QueryableFunc(db.Querier),
		registry:  reg,
	}

	instanceMu.Lock()
	defer instanceMu.Unlock()
	if current != nil {
		// 已存在实例时拒绝覆盖并回收刚打开的目录锁，避免泄漏。
		// Refuse to override a live instance and release the just-acquired directory lock.
		_ = db.Close()
		return fmt.Errorf("embedded tsdb already started at %q", current.opts.Dir)
	}
	current = inst
	logger.InfoF(ctx, "[Observability] 内嵌 tsdb 已启动: dir=%s retention=%s max_bytes=%d / embedded tsdb started: dir=%s retention=%s max_bytes=%d",
		opts.Dir, opts.RetentionDuration, opts.MaxBytes, opts.Dir, opts.RetentionDuration, opts.MaxBytes)
	return nil
}

// Close 关闭并摘除包级实例；幂等，未启动时为空操作。Close 会触发 head compact + WAL flush。
// Close closes and unregisters the package-level instance; idempotent, a no-op when not started.
// Closing triggers a head compaction and WAL flush.
func Close() error {
	instanceMu.Lock()
	inst := current
	current = nil
	instanceMu.Unlock()
	if inst == nil {
		return nil
	}
	return inst.db.Close()
}

// Stop 是 Close 的便捷别名，供偏好 Stop 语义的停机序列使用。
// Stop is a convenience alias of Close for shutdown sequences that prefer Stop semantics.
func Stop() error {
	return Close()
}

// Ready 报告内嵌存储与查询引擎是否已就绪。
// Ready reports whether the embedded storage and query engine are up.
func Ready() bool {
	inst := snapshot()
	return inst != nil
}

// Available 是 Ready 的别名，为后续「就绪 vs 可用」语义细分留口（如引擎健康检查）。
// Available is an alias of Ready, reserved for a future readiness-vs-availability split
// (e.g. engine health checks).
func Available() bool {
	return Ready()
}

// Appender 暴露 tsdb 写入入口供抓取器/进程内生产者使用；未就绪时返回 nil。
// Appender exposes the tsdb write path for the scraper / in-process producers; nil when not ready.
func Appender(ctx context.Context) storage.Appender {
	inst := snapshot()
	if inst == nil {
		return nil
	}
	return inst.db.Appender(ctx)
}

// QueryEngine 原子地返回查询引擎与数据源，供 promquery 构造区间/即时查询；未就绪时返回 (nil, nil)。
// QueryEngine atomically returns the query engine and queryable for promquery to build
// range/instant queries; (nil, nil) when not ready.
func QueryEngine() (*promql.Engine, storage.Queryable) {
	inst := snapshot()
	if inst == nil {
		return nil, nil
	}
	return inst.engine, inst.queryable
}

// snapshot 读取当前实例；未启动时返回 nil。
// snapshot reads the current instance; nil when not started.
func snapshot() *instance {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return current
}
