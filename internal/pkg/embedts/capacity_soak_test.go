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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"
)

// soakConfig 是实时长时间运行基准的参数：墙钟间隔抓取，而不是把样本一次性灌进 head。
// soakConfig is the real-time soak: scrapes on a wall-clock interval instead of bursting samples into head.
type soakConfig struct {
	Duration    time.Duration
	Interval    time.Duration
	SampleEvery time.Duration
	Series      int
	Targets     int
	MinBlock    time.Duration
	Retention   time.Duration
	OutPath     string
}

// soakSnapshot 是一次采样：RSS / heap / GC / 抓取耗时 / 存储 / 首页与作业趋势查询。
// soakSnapshot is one sample: RSS / heap / GC / scrape cost / storage / home and job-trend queries.
type soakSnapshot struct {
	UnixMilli         int64   `json:"unix_ms"`
	ElapsedSec        float64 `json:"elapsed_sec"`
	ScrapeMs          float64 `json:"scrape_ms"`
	RSSBytes          int64   `json:"rss_bytes"`
	HeapInuse         uint64  `json:"heap_inuse"`
	NumGC             uint32  `json:"num_gc"`
	PauseTotalNs      uint64  `json:"pause_total_ns"`
	LastPauseNs       uint64  `json:"last_pause_ns"`
	HeadSeries        uint64  `json:"head_series"`
	HeadBytes         int64   `json:"head_bytes"`
	BlocksBytes       int64   `json:"blocks_bytes"`
	CompactionsFailed float64 `json:"compactions_failed"`
	CompactionsTotal  float64 `json:"compactions_total"`
	SizeRetentions    float64 `json:"size_retentions"`
	CompactionWindow  bool    `json:"compaction_window"`
	QueryHome1hMs     float64 `json:"query_home_1h_ms"`
	QueryHome7dMs     float64 `json:"query_home_7d_ms"`
	QueryHome30dMs    float64 `json:"query_home_30d_ms"`
	QueryJob1hMs      float64 `json:"query_job_1h_ms"`
	QueryPeakHeap     uint64  `json:"query_peak_heap"`
	TargetsHealthy    int     `json:"targets_healthy"`
	TargetsTotal      int     `json:"targets_total"`
	LastError         string  `json:"last_error,omitempty"`
	QueryErr          string  `json:"query_err,omitempty"`
}

type soakResult struct {
	Cfg       soakConfig
	Snapshots []soakSnapshot
	ScrapeMs  []float64
	Home1hMs  []float64
	Home7dMs  []float64
	Home30dMs []float64
	Job1hMs   []float64
	Compact1h []float64
}

// TestCapacity_SoakSmoke 用真实抓取循环做秒级冒烟，证明 soak harness 会写入、自抓取和查询。
// TestCapacity_SoakSmoke is a seconds-long real scrape loop so CI proves the soak harness writes, self-scrapes and queries.
func TestCapacity_SoakSmoke(t *testing.T) {
	result := runCapacitySoak(t, soakConfig{
		Duration:    2500 * time.Millisecond,
		Interval:    400 * time.Millisecond,
		SampleEvery: 400 * time.Millisecond,
		Series:      40,
		Targets:     2,
		MinBlock:    time.Second,
		Retention:   time.Hour,
	})
	require.GreaterOrEqual(t, len(result.Snapshots), 2)
	last := result.Snapshots[len(result.Snapshots)-1]
	require.GreaterOrEqual(t, last.HeadSeries, uint64(40))
	require.Equal(t, 2, last.TargetsHealthy)
	require.Greater(t, last.ScrapeMs, 0.0)
}

// TestCapacity_Soak 是 #150 的实时长时间运行入口；STX_TSDB_SOAK=48h 时按 15s 抓取至少两天。
// 必须带 -timeout，默认 go test 10 分钟会先杀掉。48h 请用 ./scripts/tsdb-soak.sh。
// TestCapacity_Soak is the #150 real-time soak; STX_TSDB_SOAK=48h scrapes every 15s for two days.
// Pass -timeout; the default 10m go test limit kills it first. Use ./scripts/tsdb-soak.sh for 48h.
func TestCapacity_Soak(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("STX_TSDB_SOAK"))
	if raw == "" {
		t.Skip("set STX_TSDB_SOAK=48h (or 2d / 3m / smoke) and go test -timeout longer than the soak")
	}
	cfg, err := soakConfigFromEnv(raw)
	require.NoError(t, err)
	if cfg.Duration >= 9*time.Minute {
		t.Logf("soak duration=%s; go test default timeout is 10m — use -timeout 50h for a 48h run", cfg.Duration)
	}
	result := runCapacitySoak(t, cfg)
	require.NotEmpty(t, result.Snapshots)
	last := result.Snapshots[len(result.Snapshots)-1]
	require.GreaterOrEqual(t, last.HeadSeries, uint64(cfg.Series))
	require.Equal(t, cfg.Targets, last.TargetsHealthy, "all soak targets should stay healthy")
	logSoakSummary(t, result)
}

func soakConfigFromEnv(raw string) (soakConfig, error) {
	duration, err := parseSoakDuration(raw)
	if err != nil {
		return soakConfig{}, err
	}
	cfg := soakConfig{
		Duration:    duration,
		Interval:    15 * time.Second,
		SampleEvery: time.Minute,
		Series:      20_000,
		Targets:     4,
		Retention:   31 * 24 * time.Hour,
	}
	if raw == "smoke" {
		cfg.Duration = 8 * time.Second
		cfg.Interval = time.Second
		cfg.SampleEvery = time.Second
		cfg.Series = 80
		cfg.Targets = 2
		cfg.MinBlock = 2 * time.Second
		cfg.Retention = time.Hour
	}
	if v := envDuration("STX_TSDB_SOAK_INTERVAL"); v > 0 {
		cfg.Interval = v
	}
	if v := envDuration("STX_TSDB_SOAK_SAMPLE"); v > 0 {
		cfg.SampleEvery = v
	}
	if v := envDuration("STX_TSDB_SOAK_BLOCK"); v > 0 {
		cfg.MinBlock = v
	}
	if n := envInt("STX_TSDB_SOAK_SERIES", 0); n > 0 {
		cfg.Series = n
	}
	if n := envInt("STX_TSDB_SOAK_TARGETS", 0); n > 0 {
		cfg.Targets = n
	}
	if out := strings.TrimSpace(os.Getenv("STX_TSDB_SOAK_OUT")); out != "" {
		cfg.OutPath = out
	}
	if cfg.SampleEvery <= 0 || cfg.Duration < 2*cfg.SampleEvery {
		cfg.SampleEvery = cfg.Interval
	}
	if cfg.Targets < 1 {
		cfg.Targets = 1
	}
	if cfg.Series < cfg.Targets {
		cfg.Series = cfg.Targets
	}
	return cfg, nil
}

func parseSoakDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "smoke" {
		return 8 * time.Second, nil
	}
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid STX_TSDB_SOAK %q", raw)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid STX_TSDB_SOAK %q", raw)
	}
	return d, nil
}

func runCapacitySoak(t *testing.T, cfg soakConfig) soakResult {
	t.Helper()
	ctx := context.Background()
	opts := testOptions(t)
	opts.MaxSamples = DefaultMaxSamples
	opts.QueryTimeout = 30 * time.Second
	opts.RetentionDuration = cfg.Retention
	opts.MinBlockDuration = cfg.MinBlock
	require.NoError(t, Start(ctx, opts))
	t.Cleanup(func() { _ = Close() })

	targets, servers := startSoakTargets(t, cfg.Targets, cfg.Series)
	t.Cleanup(func() {
		for _, srv := range servers {
			srv.Close()
		}
	})

	scraper := newTestScraper(staticProvider(targets))
	scraper.opts.Interval = cfg.Interval
	scraper.opts.SampleLimit = DefaultSampleLimit
	scraper.refreshTargets(ctx)

	var (
		outFile *os.File
		enc     *json.Encoder
	)
	if cfg.OutPath != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(cfg.OutPath), 0o755))
		f, err := os.Create(cfg.OutPath)
		require.NoError(t, err)
		t.Cleanup(func() { _ = f.Close() })
		outFile = f
		enc = json.NewEncoder(f)
	}

	result := soakResult{Cfg: cfg}
	started := time.Now()
	var lastCompact float64
	sampleDue := started
	round := 0

	doRound := func(force bool) {
		round++
		scrapeStart := time.Now()
		scraper.scrapeRound(ctx)
		scrapeMs := time.Since(scrapeStart).Seconds() * 1000
		result.ScrapeMs = append(result.ScrapeMs, scrapeMs)
		now := time.Now()
		sample := force || !now.Before(sampleDue)
		if !sample {
			return
		}
		for !sampleDue.After(now) {
			sampleDue = sampleDue.Add(cfg.SampleEvery)
		}
		snap := takeSoakSnapshot(ctx, scraper, started, scrapeMs)
		if len(result.Snapshots) > 0 && snap.CompactionsTotal > lastCompact {
			snap.CompactionWindow = true
		}
		if snap.CompactionsTotal > lastCompact {
			lastCompact = snap.CompactionsTotal
		}
		result.Snapshots = append(result.Snapshots, snap)
		result.Home1hMs = append(result.Home1hMs, snap.QueryHome1hMs)
		result.Home7dMs = append(result.Home7dMs, snap.QueryHome7dMs)
		result.Home30dMs = append(result.Home30dMs, snap.QueryHome30dMs)
		result.Job1hMs = append(result.Job1hMs, snap.QueryJob1hMs)
		if snap.CompactionWindow {
			result.Compact1h = append(result.Compact1h, snap.QueryHome1hMs)
		}
		t.Logf("soak round=%d elapsed=%.0fs scrape_ms=%.1f rss=%d heap=%d head_series=%d blocks=%d compact_total=%.0f home1h_ms=%.1f job1h_ms=%.1f compact_window=%v",
			round, snap.ElapsedSec, snap.ScrapeMs, snap.RSSBytes, snap.HeapInuse, snap.HeadSeries, snap.BlocksBytes, snap.CompactionsTotal, snap.QueryHome1hMs, snap.QueryJob1hMs, snap.CompactionWindow)
		if enc != nil {
			if err := enc.Encode(snap); err != nil {
				t.Errorf("write soak jsonl: %v", err)
			}
			_ = outFile.Sync()
		}
	}

	doRound(true)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	deadline := time.NewTimer(cfg.Duration)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			doRound(true)
			return result
		case <-ticker.C:
			doRound(false)
		}
	}
}

func startSoakTargets(t *testing.T, nTargets, series int) ([]ScrapeTarget, []*httptest.Server) {
	t.Helper()
	per := series / nTargets
	servers := make([]*httptest.Server, 0, nTargets)
	targets := make([]ScrapeTarget, 0, nTargets)
	for i := 0; i < nTargets; i++ {
		start := i * per
		end := start + per
		if i == nTargets-1 {
			end = series
		}
		seq := &atomic.Uint64{}
		from, to := start, end
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := seq.Add(1)
			w.Header().Set("Content-Type", "text/plain; version=0.0.4")
			_, _ = w.Write([]byte(renderSoakExposition(from, to, n)))
		}))
		servers = append(servers, srv)
		targets = append(targets, ScrapeTarget{
			URL: srv.URL + "/metrics",
			Labels: map[string]string{
				"job":        "seatunnel_engine_http",
				"instance":   fmt.Sprintf("soak-%d", i),
				"cluster":    "soak",
				"cluster_id": "1",
			},
		})
	}
	return targets, servers
}

func renderSoakExposition(from, to int, seq uint64) string {
	var b strings.Builder
	n := to - from
	if n < 0 {
		n = 0
	}
	b.Grow(n*90 + 80)
	b.WriteString("# TYPE stx_capacity_value gauge\n")
	for i := from; i < to; i++ {
		fmt.Fprintf(&b, "stx_capacity_value{series=%q} %d\n", strconv.Itoa(i), seq)
	}
	b.WriteString("# TYPE stx_job_source_received_rows_total counter\n")
	for i := from; i < to; i++ {
		fmt.Fprintf(&b, "stx_job_source_received_rows_total{series=%q} %d\n", strconv.Itoa(i), seq)
	}
	return b.String()
}

func takeSoakSnapshot(ctx context.Context, scraper *scraper, started time.Time, scrapeMs float64) soakSnapshot {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	now := time.Now().UTC()
	home1h, heap1 := soakRangeMs(ctx, `sum(stx_capacity_value)`, now.Add(-time.Hour), now, 15*time.Second)
	home7d, heap7 := soakRangeMs(ctx, `sum(stx_capacity_value)`, now.Add(-7*24*time.Hour), now, 30*time.Minute)
	home30d, heap30 := soakRangeMs(ctx, `sum(stx_capacity_value)`, now.Add(-30*24*time.Hour), now, 2*time.Hour)
	job1h, heapJ := soakRangeMs(ctx, `sum(rate(stx_job_source_received_rows_total[1m]))`, now.Add(-time.Hour), now, 15*time.Second)
	peak := heap1
	for _, h := range []uint64{heap7, heap30, heapJ} {
		if h > peak {
			peak = h
		}
	}
	status, _ := GetStorageStatus()
	compactions := soakInstantFloat(ctx, `sum(prometheus_tsdb_compactions_total{job="stx_embedded"})`)
	healthy, total, lastErr := soakTargetHealth(scraper)
	return soakSnapshot{
		UnixMilli:         now.UnixMilli(),
		ElapsedSec:        time.Since(started).Seconds(),
		ScrapeMs:          scrapeMs,
		RSSBytes:          processRSSBytes(),
		HeapInuse:         ms.HeapInuse,
		NumGC:             ms.NumGC,
		PauseTotalNs:      ms.PauseTotalNs,
		LastPauseNs:       ms.PauseNs[(ms.NumGC+255)%256],
		HeadSeries:        status.HeadSeries,
		HeadBytes:         status.HeadBytes,
		BlocksBytes:       status.BlocksBytes,
		CompactionsFailed: status.CompactionsFailed,
		CompactionsTotal:  compactions,
		SizeRetentions:    status.SizeRetentions,
		QueryHome1hMs:     home1h.ms,
		QueryHome7dMs:     home7d.ms,
		QueryHome30dMs:    home30d.ms,
		QueryJob1hMs:      job1h.ms,
		QueryPeakHeap:     peak,
		TargetsHealthy:    healthy,
		TargetsTotal:      total,
		LastError:         lastErr,
		QueryErr:          firstNonEmpty(home1h.err, home7d.err, home30d.err, job1h.err),
	}
}

type soakQuery struct {
	ms  float64
	err string
}

func soakRangeMs(ctx context.Context, expr string, start, end time.Time, step time.Duration) (soakQuery, uint64) {
	engine, queryable := QueryEngine()
	if engine == nil || queryable == nil {
		return soakQuery{err: "query engine not ready"}, 0
	}
	started := time.Now()
	qry, err := engine.NewRangeQuery(ctx, queryable, nil, expr, start, end, step)
	if err != nil {
		return soakQuery{err: err.Error()}, 0
	}
	defer qry.Close()
	res := qry.Exec(ctx)
	elapsed := time.Since(started).Seconds() * 1000
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	out := soakQuery{ms: elapsed}
	if res.Err != nil {
		out.err = res.Err.Error()
		return out, ms.HeapInuse
	}
	if _, ok := res.Value.(promql.Matrix); !ok {
		out.err = fmt.Sprintf("expected matrix, got %T", res.Value)
	}
	return out, ms.HeapInuse
}

func soakInstantFloat(ctx context.Context, expr string) float64 {
	engine, queryable := QueryEngine()
	if engine == nil || queryable == nil {
		return 0
	}
	qry, err := engine.NewInstantQuery(ctx, queryable, nil, expr, time.Now().UTC())
	if err != nil {
		return 0
	}
	defer qry.Close()
	res := qry.Exec(ctx)
	if res.Err != nil {
		return 0
	}
	vec, ok := res.Value.(promql.Vector)
	if !ok || len(vec) == 0 {
		return 0
	}
	return vec[0].F
}

func soakTargetHealth(s *scraper) (healthy, total int, lastErr string) {
	if s == nil {
		return 0, 0, ""
	}
	for _, st := range s.targetStates() {
		total++
		if st.Healthy {
			healthy++
			continue
		}
		if st.LastError != "" {
			lastErr = st.LastError
		}
	}
	return healthy, total, lastErr
}

func logSoakSummary(t *testing.T, result soakResult) {
	t.Helper()
	last := result.Snapshots[len(result.Snapshots)-1]
	t.Logf("soak summary duration=%s interval=%s series=%d targets=%d rounds=%d samples=%d rss=%d heap=%d head_series=%d head_bytes=%d blocks_bytes=%d scrape_p99_ms=%.1f home1h_p99_ms=%.1f home7d_p99_ms=%.1f home30d_p99_ms=%.1f job1h_p99_ms=%.1f compact_home1h_p99_ms=%.1f gc_pause_total_ms=%.1f",
		result.Cfg.Duration, result.Cfg.Interval, result.Cfg.Series, result.Cfg.Targets,
		len(result.ScrapeMs), len(result.Snapshots), last.RSSBytes, last.HeapInuse, last.HeadSeries, last.HeadBytes, last.BlocksBytes,
		percentile(result.ScrapeMs, 0.99), percentile(result.Home1hMs, 0.99), percentile(result.Home7dMs, 0.99), percentile(result.Home30dMs, 0.99), percentile(result.Job1hMs, 0.99), percentile(result.Compact1h, 0.99),
		float64(last.PauseTotalNs)/1e6)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	idx := int(float64(len(cp)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func TestParseSoakDuration(t *testing.T) {
	d, err := parseSoakDuration("2d")
	require.NoError(t, err)
	require.Equal(t, 48*time.Hour, d)
	d, err = parseSoakDuration("48h")
	require.NoError(t, err)
	require.Equal(t, 48*time.Hour, d)
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return d
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
