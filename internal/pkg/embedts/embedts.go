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
	"math"
	"os"
	"runtime"
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
	// DefaultRetention 是默认时间保留期：0 表示不按时间删除（默认只按容量清理）。
	// DefaultRetention is the default time retention: 0 disables time-based deletion
	// (capacity-only retention is the default).
	DefaultRetention = 0
	// DefaultMaxBytesCap 是容量上限的天花板：未配置 max_size 时实际上限取
	// min(30GiB, 数据目录所在磁盘总容量 × 20%)，下限 minAutoMaxBytes。
	// DefaultMaxBytesCap is the ceiling of the size cap: when max_size is unset the effective
	// cap is min(30GiB, 20% of the data-dir disk total), floored at minAutoMaxBytes.
	DefaultMaxBytesCap = int64(30) << 30
	// maxSizeDiskFraction 是自动容量上限占磁盘总容量的比例（20%）。
	// maxSizeDiskFraction is the share of the disk total used for the auto size cap (20%).
	maxSizeDiskFraction = 0.20
	// minAutoMaxBytes 是自动容量上限的下限（1GiB），小磁盘也至少保留这么多数据。
	// minAutoMaxBytes floors the auto size cap at 1GiB so small disks still keep some history.
	minAutoMaxBytes = int64(1) << 30
	// DefaultMaxBlockDuration 是缺省的最大块时长（7 天）：更长窗口下减少块文件数量；
	// 有显式 retention 时收紧到 retention/10，下限 minMaxBlockDuration。
	// DefaultMaxBlockDuration is the default max block range (7 days) to keep block counts low
	// on long windows; an explicit retention tightens it to retention/10, floored at minMaxBlockDuration.
	DefaultMaxBlockDuration = 7 * 24 * time.Hour
	// minMaxBlockDuration 是最大块时长的下限（2 小时，即 tsdb 默认块长）。
	// minMaxBlockDuration floors the max block range at 2h (the tsdb default block range).
	minMaxBlockDuration = 2 * time.Hour
	// DefaultQueryTimeout 是默认单次查询超时，与原远程 HTTP 客户端 8s 一致。
	// DefaultQueryTimeout is the default per-query timeout, matching the legacy 8s HTTP client timeout.
	DefaultQueryTimeout = 8 * time.Second
	// DefaultMaxSamples 是默认单次查询样本上限：约 160MB（按 16B/样本），覆盖约 2 万活跃序列 × 7d 面板点数。
	// 独立 Prometheus 默认 5000 万在 8G 部署机上一次查询就能打满 API 进程堆，见 embedded-tsdb-capacity.md。
	// DefaultMaxSamples is the per-query sample cap: ~160MB at 16B/sample, covering ~20k active
	// series × 7d dashboard points. Upstream Prometheus' 50M default can fill an 8G API heap on
	// one query; see embedded-tsdb-capacity.md.
	DefaultMaxSamples = 10000000
	// DefaultSampleLimit 是每个抓取目标单轮样本上限，超限丢弃本轮并记 last_error（Prometheus sample_limit 语义）。
	// DefaultSampleLimit is the per-target per-round sample cap; over-limit scrapes are dropped
	// and recorded in last_error (Prometheus sample_limit semantics).
	DefaultSampleLimit = 50000
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

	// MaxBytesSource 记录 MaxBytes 的取值来源（config / auto:20% / auto:cap / fallback），
	// 写进启动日志与组件状态 meta；直接构造 Options 时可为空。
	// MaxBytesSource records where MaxBytes came from (config / auto:20% / auto:cap / fallback)
	// for the startup log and component meta; empty for hand-built Options.
	MaxBytesSource string

	// MaxBlockDuration 覆盖最大块时长上限；0 表示按 retention 自适应（默认 7 天、
	// retention/10 收紧、下限 2 小时）。
	// MaxBlockDuration overrides the max block range; 0 derives it from retention
	// (default 7d, tightened to retention/10, floored at 2h).
	MaxBlockDuration time.Duration

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

	// MinBlockDuration 覆盖 tsdb 默认 2h 块时长；仅基准测试用来在短窗口内触发 compaction，生产保持 0。
	// MinBlockDuration overrides the default 2h tsdb block range; capacity benches use a short
	// window so compaction can fire, production leaves it 0.
	MinBlockDuration time.Duration
}

// withDefaults 填充零值字段，保证绕过 config 直接构造 Options 的调用方也能工作。
// withDefaults fills zero-value fields so callers constructing Options without config still work.
func (o Options) withDefaults() Options {
	if strings.TrimSpace(o.Dir) == "" {
		o.Dir = DefaultDir
	}
	// 0 表示不按时间删除（默认只按容量清理）；负值防御性归零。
	// 0 disables time-based deletion (capacity-only retention is the default); negatives clamp to 0.
	if o.RetentionDuration < 0 {
		o.RetentionDuration = 0
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

// MaxBytes 取值来源：写进启动日志与组件状态 meta。
// MaxBytes provenance values reported in the startup log and component meta.
const (
	// MaxBytesSourceConfig 表示 max_size 由用户显式配置（含 0=不限）。
	// MaxBytesSourceConfig marks an explicitly configured max_size (including 0 = unlimited).
	MaxBytesSourceConfig = "config"
	// MaxBytesSourceAuto20 表示按磁盘总容量 20% 自适应。
	// MaxBytesSourceAuto20 marks a cap derived as 20% of the disk total.
	MaxBytesSourceAuto20 = "auto:20%"
	// MaxBytesSourceAutoCap 表示磁盘 20% 超过 30GiB，取天花板 30GiB。
	// MaxBytesSourceAutoCap marks the 30GiB ceiling winning over the 20% derivation.
	MaxBytesSourceAutoCap = "auto:cap"
	// MaxBytesSourceFallback 表示 statfs 失败或平台不支持，退回 30GiB。
	// MaxBytesSourceFallback marks the 30GiB fallback after a statfs failure or unsupported platform.
	MaxBytesSourceFallback = "fallback"
)

// fsTotalBytes 是磁盘总容量探测入口；包级变量便于测试替换（实现按平台分文件）。
// fsTotalBytes probes the disk total; a package var so tests can stub it (per-platform impls).
var fsTotalBytes = defaultFsTotalBytes

// autoMaxBytes 计算未配置 max_size 时的容量上限：min(DefaultMaxBytesCap, 磁盘总容量 × 20%)，
// 下限 1GiB；目录不可建或 statfs 失败时退回 30GiB 并记 WARN。
// autoMaxBytes derives the size cap when max_size is unset: min(DefaultMaxBytesCap, 20% of the
// disk total) floored at 1GiB; an uncreatable dir or a statfs failure falls back to 30GiB with a WARN.
func autoMaxBytes(ctx context.Context, dir string, statfs func(string) (int64, error)) (int64, string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.WarnF(ctx, "[Observability] 无法创建 tsdb 目录 %s，容量上限退回 30GiB: %v / cannot create tsdb dir %s, size cap falls back to 30GiB: %v",
			dir, err, dir, err)
		return DefaultMaxBytesCap, MaxBytesSourceFallback
	}
	total, err := statfs(dir)
	if err != nil || total <= 0 {
		logger.WarnF(ctx, "[Observability] 读取 %s 磁盘容量失败或不支持，容量上限退回 30GiB: %v / statfs on %s failed or unsupported, size cap falls back to 30GiB: %v",
			dir, err, dir, err)
		return DefaultMaxBytesCap, MaxBytesSourceFallback
	}
	auto := int64(float64(total) * maxSizeDiskFraction)
	if auto > DefaultMaxBytesCap {
		return DefaultMaxBytesCap, MaxBytesSourceAutoCap
	}
	if auto < minAutoMaxBytes {
		return minAutoMaxBytes, MaxBytesSourceAuto20
	}
	return auto, MaxBytesSourceAuto20
}

// resolveMaxBlockDuration 计算最大块时长：未显式指定时默认 7 天；retention>0 且 retention/10 更小时
// 取它（Prometheus 压实器需要约 10 倍块长才能压实旧块）；下限 2 小时（tsdb 默认块长）。
// resolveMaxBlockDuration derives the max block range: 7 days when unspecified; a smaller
// retention/10 wins when retention > 0 (the compactor needs ~10x the block range to compact old
// blocks); floored at 2h (the tsdb default block range).
func resolveMaxBlockDuration(opts Options) time.Duration {
	maxBlock := opts.MaxBlockDuration
	if maxBlock <= 0 {
		maxBlock = DefaultMaxBlockDuration
		if r := opts.RetentionDuration; r > 0 && r/10 < maxBlock {
			maxBlock = r / 10
		}
		if maxBlock < minMaxBlockDuration {
			maxBlock = minMaxBlockDuration
		}
	}
	return maxBlock
}

// OptionsFromObservability 把可观测配置翻译为启动参数；时长字符串按 Prometheus duration 语法解析。
// OptionsFromObservability translates observability config into startup Options; duration strings
// are parsed with the Prometheus duration syntax (s/m/h/d/w/y).
func OptionsFromObservability(ctx context.Context, obs config.ObservabilityConfig) (Options, error) {
	opts := Options{
		Dir:        strings.TrimSpace(obs.Storage.Dir),
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
	opts = opts.withDefaults()
	// max_size 显式配置时使用配置值（含 0=不限）；未配置时按数据目录所在磁盘自适应。
	// Use the configured max_size when explicitly set (0 = unlimited); otherwise derive it
	// from the data-dir disk.
	if obs.Storage.MaxSizeSet {
		opts.MaxBytes = obs.Storage.MaxSize
		opts.MaxBytesSource = MaxBytesSourceConfig
	} else {
		opts.MaxBytes, opts.MaxBytesSource = autoMaxBytes(ctx, opts.Dir, fsTotalBytes)
	}
	return opts, nil
}

// instance 持有一份已打开的内嵌存储与查询引擎。
// instance holds one opened embedded storage + query engine pair.
type instance struct {
	opts      Options
	db        *tsdb.DB
	engine    *promql.Engine
	queryable storage.Queryable
	registry  *prometheus.Registry
	// scraper 是挂在实例上的抓取循环；Close 时先停它再关 db。
	// scraper is the scrape loop attached to the instance; Close stops it before closing the db.
	scraper *scraper
}

// 进程级单例：见包注释。读路径走 RLock，启停走 Lock。
// Process-level singleton: see package doc. Read paths take RLock, start/stop take Lock.
// maxConcurrentQueries 返回引擎同时执行的查询上限：max(4, GOMAXPROCS)。大盘、作业趋势、诊断与告警评估
// 共用同一引擎，超出上限的查询在引擎内排队（仍受 query_timeout 约束），避免多人打开首页时把 CPU 打满。
// maxConcurrentQueries returns the engine's concurrent query cap: max(4, GOMAXPROCS). Dashboards, job
// trends, diagnostics and alert evaluation share one engine; queries beyond the cap queue inside the
// engine (still bounded by query_timeout) so many open home pages cannot saturate the CPU.
func maxConcurrentQueries() int {
	if n := runtime.GOMAXPROCS(0); n > 4 {
		return n
	}
	return 4
}

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
	// 时间与容量保留都不限时数据会一直涨到磁盘写满，启动时显式提示。
	// Warn when both retentions are unbounded: data grows until the disk fills.
	if opts.RetentionDuration == 0 && opts.MaxBytes == 0 {
		logger.WarnF(ctx, "[Observability] retention 与 max_size 均未限制，tsdb 数据会一直增长直到磁盘写满 / neither retention nor max_size limits storage, tsdb data grows until the disk fills up")
	}
	reg := prometheus.NewRegistry()
	// Go runtime / 进程采集器与 tsdb/engine 指标共用同一 registry，由自抓取写回 tsdb。
	// Go runtime / process collectors share this registry with tsdb/engine metrics; self-scrape writes them back.
	registerRuntimeCollectors(reg)
	tsdbOpts := tsdb.DefaultOptions()
	tsdbOpts.RetentionDuration = opts.RetentionDuration.Milliseconds()
	tsdbOpts.MaxBytes = opts.MaxBytes
	tsdbOpts.NoLockfile = opts.NoLockfile
	// 最大块时长按 retention 自适应：长窗口下让块合并到 7 天档位，减少块文件与查询打开数。
	// Max block range follows retention: long windows compact up to the 7-day tier, keeping block
	// counts (and files opened per query) low.
	maxBlock := resolveMaxBlockDuration(opts)
	tsdbOpts.MaxBlockDuration = maxBlock.Milliseconds()
	if opts.MinBlockDuration > 0 {
		ms := opts.MinBlockDuration.Milliseconds()
		tsdbOpts.MinBlockDuration = ms
		// 最短块不能超过最大块；压短窗口的基准测试里两者可能相近。
		// Min block range cannot exceed the max; short-window benches may bring them close.
		if ms > tsdbOpts.MaxBlockDuration {
			tsdbOpts.MaxBlockDuration = ms
		}
	}
	// 记录实际生效值，CurrentOptions 供组件状态展示。/ Store the effective value for CurrentOptions.
	opts.MaxBlockDuration = time.Duration(tsdbOpts.MaxBlockDuration) * time.Millisecond
	// 内部日志走 NopLogger：错误经返回值与 our logger.*F 上报，避免 zap/slog 双日志体系打架。
	// Internal logs go to a NopLogger: failures surface via return values and our logger.*F,
	// avoiding a second slog pipeline next to zap.
	db, err := tsdb.Open(opts.Dir, promslog.NewNopLogger(), reg, tsdbOpts, nil)
	if err != nil {
		return fmt.Errorf("open embedded tsdb at %q: %w", opts.Dir, err)
	}
	// 查询并发上限：跟踪文件 queries.active 放在 tsdb 目录（与 Prometheus 同做法），随 engine.Close 释放。
	// Concurrent query cap: the queries.active tracking file lives in the tsdb dir (as Prometheus
	// does) and is released by engine.Close.
	tracker, err := promql.NewActiveQueryTracker(opts.Dir, maxConcurrentQueries(), promslog.NewNopLogger())
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("create embedded query tracker at %q: %w", opts.Dir, err)
	}
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:               promslog.NewNopLogger(),
		Reg:                  reg,
		MaxSamples:           opts.MaxSamples,
		Timeout:              opts.QueryTimeout,
		ActiveQueryTracker:   tracker,
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
		_ = engine.Close()
		_ = db.Close()
		return fmt.Errorf("embedded tsdb already started at %q", current.opts.Dir)
	}
	current = inst
	// retention=0 在日志里展示为“不限 / unlimited”，与组件状态 meta 的 "unlimited" 一致。
	// retention=0 logs as 不限 / unlimited, matching the "unlimited" component meta value.
	retentionZh, retentionEn := "不限", "unlimited"
	if opts.RetentionDuration > 0 {
		retentionZh = model.Duration(opts.RetentionDuration).String()
		retentionEn = retentionZh
	}
	source := opts.MaxBytesSource
	if source == "" {
		source = MaxBytesSourceConfig
	}
	maxBlockLabel := model.Duration(opts.MaxBlockDuration).String()
	logger.InfoF(ctx, "[Observability] 内嵌 tsdb 已启动: dir=%s retention=%s max_bytes=%d(%s) max_block=%s / embedded tsdb started: dir=%s retention=%s max_bytes=%d(%s) max_block=%s",
		opts.Dir, retentionZh, opts.MaxBytes, source, maxBlockLabel,
		opts.Dir, retentionEn, opts.MaxBytes, source, maxBlockLabel)
	return nil
}

// CurrentOptions 返回运行中实例实际生效的启动参数（含自适应 max_bytes / max_block）；未启动时 ok=false。
// CurrentOptions returns the effective Options of the running instance (including derived
// max_bytes / max_block); ok=false when not started.
func CurrentOptions() (Options, bool) {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	if current == nil {
		return Options{}, false
	}
	return current.opts, true
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
	// 先停抓取循环再关 db，避免关闭瞬间仍有写入线程在 appender 上。
	// Stop the scrape loop before closing the db so no writer still sits on an appender.
	inst.stopScraper()
	resetSelfMetricsTracker()
	_ = inst.engine.Close()
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

// AppendableV2 暴露 tsdb 的 V2 追加入口，供 rules.Manager 回写录制序列与 ALERTS/ALERTS_FOR_STATE
// 状态序列（后者是重启后恢复告警 for 计时态的数据源）；未就绪时返回 nil。
// AppendableV2 exposes the tsdb V2 append path so rules.Manager can write back recording
// series and the ALERTS/ALERTS_FOR_STATE state series (the latter is the data source for
// restoring alert 'for' state after a restart); nil when not ready.
func AppendableV2() storage.AppendableV2 {
	inst := snapshot()
	if inst == nil {
		return nil
	}
	return inst.db
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

// OldestSampleTime 返回 tsdb 中最早样本的时间：持久化 blocks 的 Meta().MinTime 与 head 的
// MinTime 取最小（两者单位都是毫秒）。保留期被时间或容量截断后该值会随之变新，因此调用方
// 应把它当作「数据实际能回看到哪里」的事实来源，而不是按配置推算。未就绪或没有任何样本时返回 false。
// OldestSampleTime returns the timestamp of the oldest sample in the tsdb: the minimum of every
// persisted block's Meta().MinTime and the head's MinTime (all in milliseconds). When retention
// truncates data by time or size this moves forward, so callers should treat it as the factual
// "how far back does data actually go" rather than deriving it from config. It returns false when
// the store is not ready or holds no samples at all.
func OldestSampleTime() (time.Time, bool) {
	inst := snapshot()
	if inst == nil {
		return time.Time{}, false
	}
	oldest := int64(math.MaxInt64)
	for _, block := range inst.db.Blocks() {
		if meta := block.Meta(); meta.MinTime < oldest {
			oldest = meta.MinTime
		}
	}
	if head := inst.db.Head(); head != nil {
		if minTime := head.MinTime(); minTime < oldest {
			oldest = minTime
		}
	}
	if oldest == math.MaxInt64 {
		return time.Time{}, false
	}
	return time.UnixMilli(oldest).UTC(), true
}
