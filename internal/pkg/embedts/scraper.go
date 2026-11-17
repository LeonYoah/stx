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

package embedts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"golang.org/x/sync/errgroup"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
)

// 抓取循环默认值：与退役的外部 prometheus.yml（scrape_interval=15s、SD refresh_interval=30s）一致。
// Scrape loop defaults, matching the retired external prometheus.yml (scrape_interval=15s,
// SD refresh_interval=30s).
const (
	// DefaultScrapeInterval 是默认抓取间隔。 / DefaultScrapeInterval is the default scrape interval.
	DefaultScrapeInterval = 15 * time.Second
	// DefaultTargetRefreshInterval 是默认目标集刷新间隔。 / DefaultTargetRefreshInterval is the default target-set refresh interval.
	DefaultTargetRefreshInterval = 30 * time.Second
	// maxScrapeTimeout 是单目标抓取超时上限；实际超时为 min(10s, interval)。
	// maxScrapeTimeout caps the per-target scrape timeout; the effective timeout is min(10s, interval).
	maxScrapeTimeout = 10 * time.Second
	// maxExpositionBytes 限制单次 exposition 响应体大小，防止异常目标耗尽内存。
	// maxExpositionBytes bounds one exposition response body so a rogue target cannot exhaust memory.
	maxExpositionBytes = 64 << 20
)

// errScraperNotReady 是内嵌 tsdb 未就绪时 StartScraper 返回的哨兵错误。
// errScraperNotReady is the sentinel error StartScraper returns while the embedded tsdb is not ready.
var errScraperNotReady = errors.New("embedded tsdb is not ready")

// errSampleLimitMsg 是超限目标 last_error 的稳定前缀，便于状态接口识别。
// errSampleLimitMsg is the stable last_error prefix for over-limit targets so status can match it.
const errSampleLimitMsg = "sample limit exceeded"

// ScrapeTarget 是一个抓取目标：完整 URL + 需要注入每条序列的标签集（job/instance/cluster 等）。
// ScrapeTarget is one scrape target: a full URL plus the label set injected into every
// scraped series (job/instance/cluster etc.).
type ScrapeTarget struct {
	// URL 是 exposition 完整地址（如 http://host:5801/hazelcast/rest/instance/metrics）。
	// URL is the full exposition address (e.g. http://host:5801/hazelcast/rest/instance/metrics).
	URL string
	// Labels 是注入标签集：job/instance/cluster/cluster_id/cluster_name/env 等。
	// Labels is the injected label set: job/instance/cluster/cluster_id/cluster_name/env etc.
	Labels map[string]string
}

// MetricsTargetProvider 提供抓取目标集；由 internal/apps 侧实现（embedts 不反向依赖应用层）。
// MetricsTargetProvider supplies the scrape target set; implemented by internal/apps
// (embedts never imports application packages).
type MetricsTargetProvider interface {
	Targets(ctx context.Context) ([]ScrapeTarget, error)
}

// MetricsTargetProviderFunc 把普通函数适配为 MetricsTargetProvider。
// MetricsTargetProviderFunc adapts a plain function into a MetricsTargetProvider.
type MetricsTargetProviderFunc func(ctx context.Context) ([]ScrapeTarget, error)

// Targets 实现 MetricsTargetProvider。
// Targets implements MetricsTargetProvider.
func (f MetricsTargetProviderFunc) Targets(ctx context.Context) ([]ScrapeTarget, error) {
	return f(ctx)
}

// ScrapeOptions 是抓取循环的启动参数。
// ScrapeOptions holds the scrape loop startup parameters.
type ScrapeOptions struct {
	// Interval 是抓取间隔（observability.scrape_interval，默认 15s）。
	// Interval is the scrape interval (observability.scrape_interval, default 15s).
	Interval time.Duration
	// RefreshInterval 是目标集刷新间隔（沿用原 HTTP SD refresh_interval=30s 语义）。
	// RefreshInterval is the target-set refresh interval (keeps the old HTTP SD refresh_interval=30s semantics).
	RefreshInterval time.Duration
	// HTTPClient 允许测试注入自定义 http.Client；nil 时使用内部默认（按目标超时构造）。
	// HTTPClient lets tests inject a custom http.Client; nil falls back to an internal default
	// built around the per-target timeout.
	HTTPClient *http.Client
	// SampleLimit 是单目标单轮允许写入的最大样本数；0 表示使用 DefaultSampleLimit。
	// 超限时丢弃本轮样本、up=0，并在 TargetState.LastError 记录，不影响其他目标。
	// SampleLimit is the per-target per-round sample cap; 0 means DefaultSampleLimit.
	// Over-limit scrapes drop the round, set up=0 and record TargetState.LastError without
	// affecting other targets.
	SampleLimit int
}

// scrapeTimeout 返回单目标抓取超时 min(10s, interval)。
// scrapeTimeout returns the per-target scrape timeout min(10s, interval).
func (o ScrapeOptions) scrapeTimeout() time.Duration {
	if o.Interval > 0 && o.Interval < maxScrapeTimeout {
		return o.Interval
	}
	return maxScrapeTimeout
}

// withDefaults 填充零值字段。
// withDefaults fills zero-value fields.
func (o ScrapeOptions) withDefaults() ScrapeOptions {
	if o.Interval <= 0 {
		o.Interval = DefaultScrapeInterval
	}
	if o.RefreshInterval <= 0 {
		o.RefreshInterval = DefaultTargetRefreshInterval
	}
	if o.HTTPClient == nil {
		// Client.Timeout 留零：超时由每请求的 context 控制，避免双重超时语义。
		// Client.Timeout stays zero: timeouts ride on the per-request context instead,
		// avoiding two overlapping timeout semantics.
		o.HTTPClient = &http.Client{}
	}
	if o.SampleLimit <= 0 {
		o.SampleLimit = DefaultSampleLimit
	}
	return o
}

// ScrapeOptionsFromObservability 把可观测配置翻译为抓取循环参数。
// ScrapeOptionsFromObservability translates observability config into scrape loop options.
func ScrapeOptionsFromObservability(obs config.ObservabilityConfig) (ScrapeOptions, error) {
	opts := ScrapeOptions{}
	if trimmed := strings.TrimSpace(obs.ScrapeInterval); trimmed != "" {
		d, err := model.ParseDuration(trimmed)
		if err != nil {
			return ScrapeOptions{}, fmt.Errorf("observability.scrape_interval %q: %w", trimmed, err)
		}
		opts.Interval = time.Duration(d)
	}
	opts.SampleLimit = int(obs.SampleLimit)
	return opts.withDefaults(), nil
}

// TargetState 是一个抓取目标的内存状态，供 integration status 读取（替代原 Prometheus /api/v1/targets 查询）。
// TargetState is the in-memory state of one scrape target, read by integration status
// (replacing the old Prometheus /api/v1/targets query).
type TargetState struct {
	// URL 是抓取目标完整地址。 / URL is the full scrape target address.
	URL string
	// Labels 是该目标注入的标签集快照。 / Labels is a snapshot of the target's injected label set.
	Labels map[string]string
	// Healthy 表示最近一次抓取成功。 / Healthy reports whether the latest scrape succeeded.
	Healthy bool
	// StatusCode 是最近一次抓取得到的 HTTP 状态码（0 表示请求未发出/未得到响应）。
	// StatusCode is the last HTTP status code observed (0 when the request never got a response).
	StatusCode int
	// LastError 是最近一次失败的错误信息。 / LastError is the latest failure message.
	LastError string
	// LastScrapeAt 是最近一次抓取发生的时间。 / LastScrapeAt is when the latest scrape happened.
	LastScrapeAt time.Time
}

// scrapeResult 是一个目标单轮抓取的结果。
// scrapeResult is the outcome of scraping one target in a round.
type scrapeResult struct {
	samples    []FlatSample
	statusCode int
	err        error
}

// scraper 是抓取循环：周期并发 GET 全部目标 → expfmt 解析 → 展平 → 单 appender 写入 tsdb。
// scraper is the scrape loop: it concurrently GETs every target each interval, parses the
// exposition with expfmt, flattens it, and writes everything to the tsdb via one appender.
type scraper struct {
	provider MetricsTargetProvider
	opts     ScrapeOptions

	mu       sync.Mutex
	targets  map[string]ScrapeTarget   // key 为 URL / keyed by URL
	states   map[string]*TargetState   // key 为 URL / keyed by URL
	trackers map[string]*SeriesTracker // key 为 URL；目标消失时对其历史序列写 stale marker / stale markers on removal
	// ts 是本写入方的样本时间戳水位线：同一毫秒内连续两轮写（如抓取轮与目标移除 stale）必须错开，
	// 否则 tsdb 以 duplicate sample 拒绝。
	// ts is this writer's sample timestamp watermark: two consecutive writes inside one
	// millisecond (e.g. a scrape round then removal stales) must be staggered or the tsdb
	// rejects them as duplicate samples.
	ts WriteTsWatermark

	cancel context.CancelFunc
	done   chan struct{}
}

// StartScraper 启动抓取循环并挂到当前内嵌实例上；未就绪返回哨兵错误，重复启动返回错误。
// 循环随 embedts.Close()/Stop() 联动停止（同时也会响应传入 ctx 的取消）。
// StartScraper launches the scrape loop on the live embedded instance; it returns the
// not-ready sentinel when the tsdb is down and an error on double start. The loop is
// stopped together with embedts.Close()/Stop() (and also honours cancellation of ctx).
func StartScraper(ctx context.Context, provider MetricsTargetProvider, opts ScrapeOptions) error {
	if provider == nil {
		return errors.New("metrics target provider is nil")
	}
	inst := snapshot()
	if inst == nil {
		return errScraperNotReady
	}

	scrapeCtx, cancel := context.WithCancel(ctx)
	s := &scraper{
		provider: provider,
		opts:     opts.withDefaults(),
		targets:  make(map[string]ScrapeTarget),
		states:   make(map[string]*TargetState),
		trackers: make(map[string]*SeriesTracker),
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	instanceMu.Lock()
	if current == nil {
		instanceMu.Unlock()
		cancel()
		return errScraperNotReady
	}
	if current.scraper != nil {
		instanceMu.Unlock()
		cancel()
		return errors.New("embedded tsdb scraper already started")
	}
	current.scraper = s
	instanceMu.Unlock()

	go s.run(scrapeCtx)
	logger.InfoF(ctx, "[Observability] 内嵌抓取循环已启动: interval=%s refresh=%s / embedded scrape loop started: interval=%s refresh=%s",
		s.opts.Interval, s.opts.RefreshInterval, s.opts.Interval, s.opts.RefreshInterval)
	return nil
}

// TargetStates 返回全部抓取目标的当前状态快照（按 URL 排序）；抓取循环未启动时返回 nil。
// TargetStates returns a snapshot of all scrape target states sorted by URL; nil while
// the scrape loop is not running.
func TargetStates() []TargetState {
	inst := snapshot()
	if inst == nil || inst.scraper == nil {
		return nil
	}
	return inst.scraper.targetStates()
}

// stopScraper 停掉抓取循环（幂等）；由 Close 在关闭 db 之前调用，保证写入路径先停。
// stopScraper stops the scrape loop (idempotent); Close calls it before closing the db
// so the write path quiesces first.
func (i *instance) stopScraper() {
	if i == nil || i.scraper == nil {
		return
	}
	i.scraper.cancel()
	<-i.scraper.done
}

// run 是抓取循环主协程：立即刷新一次目标集并开始周期抓取。
// run is the scrape loop goroutine: it refreshes the target set immediately, then ticks.
func (s *scraper) run(ctx context.Context) {
	defer close(s.done)

	s.refreshTargets(ctx)

	scrapeTicker := time.NewTicker(s.opts.Interval)
	refreshTicker := time.NewTicker(s.opts.RefreshInterval)
	defer scrapeTicker.Stop()
	defer refreshTicker.Stop()

	s.scrapeRound(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-refreshTicker.C:
			s.refreshTargets(ctx)
		case <-scrapeTicker.C:
			s.scrapeRound(ctx)
		}
	}
}

// refreshTargets 重新拉取目标集：新增/更新目标进入调度；消失目标的历史序列写 stale marker 并移出状态。
// provider 失败时沿用上一轮目标集继续抓取（与 SD 语义一致——目标集按最后一次成功刷新为准）。
// refreshTargets re-pulls the target set: new/updated targets enter the schedule; removed
// targets get stale markers written on their historical series and drop out of the state map.
// A provider failure keeps the last known set (same as SD semantics—the last successful
// refresh wins).
func (s *scraper) refreshTargets(ctx context.Context) {
	next, err := s.provider.Targets(ctx)
	if err != nil {
		logger.WarnF(ctx, "[Observability] 抓取目标刷新失败，沿用上一轮目标集: %v / scrape target refresh failed, keeping previous set: %v", err, err)
		return
	}
	nextByURL := make(map[string]ScrapeTarget, len(next))
	for _, target := range next {
		url := strings.TrimSpace(target.URL)
		if url == "" {
			continue
		}
		target.URL = url
		nextByURL[url] = target
	}

	s.mu.Lock()
	var staleLsets []labels.Labels
	for url := range s.targets {
		if _, ok := nextByURL[url]; ok {
			continue
		}
		if tracker := s.trackers[url]; tracker != nil {
			staleLsets = append(staleLsets, tracker.snapshot()...)
		}
		delete(s.trackers, url)
		delete(s.states, url)
	}
	s.targets = nextByURL
	for url, target := range s.targets {
		if _, ok := s.states[url]; !ok {
			s.states[url] = &TargetState{URL: url, Labels: cloneLabels(target.Labels)}
		} else {
			s.states[url].Labels = cloneLabels(target.Labels)
		}
	}
	s.mu.Unlock()

	// 消失目标的历史序列（含 up）统一写 stale marker，防 PromQL 查询里幽灵续存。
	// Historical series of removed targets (including up) get stale markers so PromQL
	// queries do not keep seeing a ghosted last value.
	if err := AppendStaleMarkers(ctx, staleLsets, s.ts.NextNow()); err != nil {
		logger.ErrorF(ctx, "[Observability] 消失目标写入 stale marker 失败: %v / writing stale markers for removed targets failed: %v", err, err)
	}
}

// scrapeRound 执行一轮抓取：并发拉取并解析全部目标，然后在一个 appender 上按目标分段提交，
// 并为每个目标合成 up=1/0。
// scrapeRound performs one round: it fetches and parses all targets concurrently, then commits
// everything on a single appender (samples grouped per target) and synthesises up=1/0 per target.
func (s *scraper) scrapeRound(ctx context.Context) {
	targets := s.currentTargets()
	if len(targets) > 0 {
		s.scrapeTargets(ctx, targets)
	}
	// 即使没有 SeaTunnel 目标也要自抓取：否则空集群时 prometheus_tsdb_* / go_* 永远写不进 tsdb。
	// Self-scrape even with zero SeaTunnel targets, otherwise prometheus_tsdb_* / go_* never
	// land in the tsdb on an empty cluster.
	if err := SelfScrapeOnce(ctx); err != nil {
		logSelfScrapeError(ctx, err)
	}
}

// scrapeTargets 抓取并提交一轮 SeaTunnel 目标样本（不含自抓取）。
// scrapeTargets scrapes and commits one round of SeaTunnel target samples (self-scrape excluded).
func (s *scraper) scrapeTargets(ctx context.Context, targets []ScrapeTarget) {
	tsMs := s.ts.NextNow()

	results := make([]scrapeResult, len(targets))
	group, groupCtx := errgroup.WithContext(ctx)
	for i, target := range targets {
		i, target := i, target
		group.Go(func() error {
			results[i] = s.scrapeTarget(groupCtx, target)
			// 单目标失败不传染整轮：错误经 results 记录，errgroup 恒返回 nil。
			// One bad target must not poison the round: errors land in results and the
			// errgroup itself never fails.
			return nil
		})
	}
	_ = group.Wait()

	app := Appender(ctx)
	if app == nil {
		return
	}
	appendErr := false
	for i, target := range targets {
		result := results[i]
		upValue := 0.0
		if result.err == nil {
			upValue = 1.0
			if err := AppendFlatSamples(app, result.samples, tsMs); err != nil {
				appendErr = true
				result.err = err
				upValue = 0.0
			}
		}
		results[i] = result
		upLset := buildSeriesLabels("up", nil, nil, target.Labels)
		if _, err := app.Append(0, upLset, tsMs, upValue); err != nil {
			appendErr = true
		}

		// 追踪本轮写入的序列集合：消失序列写 stale marker。
		// Track this round's written series: disappeared ones get stale markers.
		written := make([]labels.Labels, 0, len(result.samples)+1)
		for _, sample := range result.samples {
			written = append(written, sample.Labels)
		}
		written = append(written, upLset)
		tracker := s.trackerFor(target.URL)
		stale := tracker.Diff(written)
		for _, lset := range stale {
			if _, err := app.Append(0, lset, tsMs, StaleNaNValue); err != nil {
				appendErr = true
			}
		}

		s.recordState(target, result)
	}
	if err := app.Commit(); err != nil {
		logger.ErrorF(ctx, "[Observability] 抓取轮次提交失败: %v / scrape round commit failed: %v", err, err)
	} else if appendErr {
		logger.WarnF(ctx, "[Observability] 抓取轮次部分样本写入失败（已提交其余样本）/ scrape round had partial append failures (remaining samples committed)")
	}
}

// scrapeTarget 抓取并解析单个目标的 exposition；错误只影响该目标的 up 与 last_error。
// scrapeTarget fetches and parses one target's exposition; failures only affect that
// target's up and last_error.
func (s *scraper) scrapeTarget(ctx context.Context, target ScrapeTarget) scrapeResult {
	reqCtx, cancel := context.WithTimeout(ctx, s.opts.scrapeTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		return scrapeResult{err: err}
	}
	resp, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		return scrapeResult{err: err}
	}
	defer resp.Body.Close()

	result := scrapeResult{statusCode: resp.StatusCode}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return scrapeResult{statusCode: resp.StatusCode, err: fmt.Errorf("http status %d", resp.StatusCode)}
	}
	parser := expfmt.NewTextParser(model.LegacyValidation)
	mfs, err := parser.TextToMetricFamilies(io.LimitReader(resp.Body, maxExpositionBytes))
	if err != nil {
		return scrapeResult{statusCode: resp.StatusCode, err: fmt.Errorf("parse exposition: %w", err)}
	}
	result.samples = FlattenMetricFamilies(metricFamiliesToSlice(mfs), target.Labels)
	if limit := s.opts.SampleLimit; limit > 0 && len(result.samples) > limit {
		return scrapeResult{
			statusCode: resp.StatusCode,
			err:        fmt.Errorf("%s: got %d samples, limit %d", errSampleLimitMsg, len(result.samples), limit),
		}
	}
	return result
}

// metricFamiliesToSlice 把 TextParser 返回的 map 转成按名称排序的切片，保证写入顺序稳定。
// metricFamiliesToSlice converts the TextParser map into a name-sorted slice so the write
// order stays deterministic.
func metricFamiliesToSlice(mfs map[string]*dto.MetricFamily) []*dto.MetricFamily {
	out := make([]*dto.MetricFamily, 0, len(mfs))
	for _, mf := range mfs {
		out = append(out, mf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetName() < out[j].GetName() })
	return out
}

// currentTargets 返回当前目标集快照（按 URL 排序，保证 appender 上按目标分段提交顺序稳定）。
// currentTargets snapshots the current target set sorted by URL so the per-target
// segments on the shared appender stay deterministic.
func (s *scraper) currentTargets() []ScrapeTarget {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScrapeTarget, 0, len(s.targets))
	for _, target := range s.targets {
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

// trackerFor 返回某目标的序列追踪器（惰性创建）。
// trackerFor lazily returns the series tracker of one target.
func (s *scraper) trackerFor(url string) *SeriesTracker {
	s.mu.Lock()
	defer s.mu.Unlock()
	tracker, ok := s.trackers[url]
	if !ok {
		tracker = NewSeriesTracker()
		s.trackers[url] = tracker
	}
	return tracker
}

// recordState 记录单目标本轮结果到内存状态。
// recordState stores one target's round outcome in the in-memory state.
func (s *scraper) recordState(target ScrapeTarget, result scrapeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[target.URL]
	if !ok {
		state = &TargetState{URL: target.URL}
		s.states[target.URL] = state
	}
	state.Labels = cloneLabels(target.Labels)
	state.Healthy = result.err == nil
	state.StatusCode = result.statusCode
	state.LastScrapeAt = time.Now()
	if result.err != nil {
		state.LastError = result.err.Error()
	} else {
		state.LastError = ""
	}
}

// targetStates 返回状态快照。
// targetStates returns the state snapshot.
func (s *scraper) targetStates() []TargetState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TargetState, 0, len(s.states))
	for _, state := range s.states {
		if state == nil {
			continue
		}
		cloned := *state
		cloned.Labels = cloneLabels(state.Labels)
		out = append(out, cloned)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

// cloneLabels 复制标签集，避免调用方持有内部 map。
// cloneLabels copies a label map so callers never share the internal map.
func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
