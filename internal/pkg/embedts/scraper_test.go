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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"

	"github.com/LeonYoah/stx/internal/config"
)

// configObservability 构造只含抓取间隔的可观测配置。
// configObservability builds an observability config carrying only the scrape interval.
func configObservability(scrapeInterval string) config.ObservabilityConfig {
	return config.ObservabilityConfig{ScrapeInterval: scrapeInterval}
}

// expositionServer 返回一个内容可变的 exposition 测试服务器。
// expositionServer returns a test server whose exposition body can be swapped per case.
func expositionServer(t *testing.T, body *string, status *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *status != 200 {
			w.WriteHeader(*status)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(*body))
	}))
}

// staticProvider 返回固定目标集的 provider。
// staticProvider returns a provider with a fixed target set.
func staticProvider(targets []ScrapeTarget) MetricsTargetProvider {
	return MetricsTargetProviderFunc(func(ctx context.Context) ([]ScrapeTarget, error) {
		return targets, nil
	})
}

// newTestScraper 直接构造 scraper 内部结构（不走 goroutine），便于确定性单轮驱动测试。
// newTestScraper builds the scraper internals directly (no goroutine) so tests can drive
// single rounds deterministically.
func newTestScraper(provider MetricsTargetProvider) *scraper {
	return &scraper{
		provider: provider,
		opts:     ScrapeOptions{}.withDefaults(),
		targets:  make(map[string]ScrapeTarget),
		states:   make(map[string]*TargetState),
		trackers: make(map[string]*SeriesTracker),
	}
}

// instantVector 在当前实例上执行即时查询并返回 vector；查询失败直接失败测试。
// instantVector runs an instant query on the live instance and fails the test on error.
func instantVector(t *testing.T, ctx context.Context, expr string, at time.Time) promql.Vector {
	t.Helper()
	engine, queryable := QueryEngine()
	require.NotNil(t, engine)
	require.NotNil(t, queryable)
	qry, err := engine.NewInstantQuery(ctx, queryable, nil, expr, at.UTC())
	require.NoError(t, err)
	defer qry.Close()
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	vec, ok := res.Value.(promql.Vector)
	require.True(t, ok, "expected vector result, got %T", res.Value)
	return vec
}

func TestScraper_RoundWritesSamplesAndUp(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	body := `# HELP jvm_memory_bytes_used heap
# TYPE jvm_memory_bytes_used gauge
jvm_memory_bytes_used{area="heap"} 123.5
# TYPE requests_total counter
requests_total{path="/api"} 7
`
	status := 200
	server := expositionServer(t, &body, &status)
	defer server.Close()

	target := ScrapeTarget{
		URL: server.URL + "/metrics",
		Labels: map[string]string{
			"job":          "seatunnel_engine_http",
			"instance":     "10.0.0.1:5801",
			"cluster":      "c1",
			"cluster_id":   "1",
			"cluster_name": "c1",
			"env":          "prod",
		},
	}
	s := newTestScraper(staticProvider([]ScrapeTarget{target}))
	s.refreshTargets(ctx)
	s.scrapeRound(ctx)

	now := time.Now().Add(5 * time.Millisecond)
	vec := instantVector(t, ctx, `jvm_memory_bytes_used`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 123.5, vec[0].F)
	require.Equal(t, "seatunnel_engine_http", vec[0].Metric.Get("job"))
	require.Equal(t, "10.0.0.1:5801", vec[0].Metric.Get("instance"))
	require.Equal(t, "prod", vec[0].Metric.Get("env"))

	vec = instantVector(t, ctx, `requests_total`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 7.0, vec[0].F)

	// up 合成：健康目标应为 1。
	// Synthesised up: a healthy target reads 1.
	vec = instantVector(t, ctx, `up{instance="10.0.0.1:5801"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 1.0, vec[0].F)

	states := s.targetStates()
	require.Len(t, states, 1)
	require.True(t, states[0].Healthy)
	require.Empty(t, states[0].LastError)
}

func TestScraper_DownThenRemovedWritesStale(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	body := `# TYPE node_state gauge
node_state 1
`
	status := 200
	server := expositionServer(t, &body, &status)

	target := ScrapeTarget{
		URL:    server.URL + "/metrics",
		Labels: map[string]string{"job": "seatunnel_engine_http", "instance": "10.0.0.2:5801"},
	}
	s := newTestScraper(staticProvider([]ScrapeTarget{target}))
	s.refreshTargets(ctx)
	s.scrapeRound(ctx)

	now := time.Now().Add(5 * time.Millisecond)
	vec := instantVector(t, ctx, `up{instance="10.0.0.2:5801"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 1.0, vec[0].F)

	// 目标下线：下一轮 up=0 且 last_error 有记录。
	// Target down: next round reports up=0 and records last_error.
	status = 503
	s.scrapeRound(ctx)
	now = time.Now().Add(5 * time.Millisecond)
	vec = instantVector(t, ctx, `up{instance="10.0.0.2:5801"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 0.0, vec[0].F)
	states := s.targetStates()
	require.Len(t, states, 1)
	require.False(t, states[0].Healthy)
	require.Contains(t, states[0].LastError, "503")

	// 目标移除：历史序列（含 up）写 stale marker，lookback 内查询不再返回。
	// 写入水位线可能把 stale 推到 wall clock +1ms，查询点向后偏移避开边界。
	// Target removed: its historical series (including up) get stale markers and no
	// longer resolve inside the lookback window. The write watermark may push the stale
	// marker to wall-clock +1ms, so the query point is offset past that boundary.
	s.provider = staticProvider(nil)
	s.refreshTargets(ctx)
	now = time.Now().Add(5 * time.Millisecond)
	vec = instantVector(t, ctx, `up{instance="10.0.0.2:5801"}`, now)
	require.Empty(t, vec)
	vec = instantVector(t, ctx, `node_state{instance="10.0.0.2:5801"}`, now)
	require.Empty(t, vec)
	require.Empty(t, s.targetStates())
}

func TestScraper_ExportedLabelRename(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	// exposition 自带 job/instance 标签应按 honor_labels=false 改名 exported_*。
	// Exposition-side job/instance labels are renamed to exported_* per honor_labels=false.
	body := `# TYPE worker_jobs gauge
worker_jobs{job="from_target",instance="from_target"} 3
`
	status := 200
	server := expositionServer(t, &body, &status)
	defer server.Close()

	target := ScrapeTarget{
		URL:    server.URL + "/metrics",
		Labels: map[string]string{"job": "seatunnel_engine_http", "instance": "10.0.0.3:5801"},
	}
	s := newTestScraper(staticProvider([]ScrapeTarget{target}))
	s.refreshTargets(ctx)
	s.scrapeRound(ctx)

	vec := instantVector(t, ctx, `worker_jobs`, time.Now().Add(5*time.Millisecond))
	require.Len(t, vec, 1)
	require.Equal(t, "seatunnel_engine_http", vec[0].Metric.Get("job"))
	require.Equal(t, "10.0.0.3:5801", vec[0].Metric.Get("instance"))
	require.Equal(t, "from_target", vec[0].Metric.Get("exported_job"))
	require.Equal(t, "from_target", vec[0].Metric.Get("exported_instance"))
}

func TestScraper_SummaryAndHistogramFlatten(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	body := `# TYPE req_duration summary
req_duration{quantile="0.5"} 1.5
req_duration{quantile="0.9"} 2.5
req_duration_sum 12
req_duration_count 8
# TYPE lat histogram
lat_bucket{le="0.5"} 2
lat_bucket{le="1"} 5
lat_bucket{le="+Inf"} 9
lat_sum 20.5
lat_count 9
`
	status := 200
	server := expositionServer(t, &body, &status)
	defer server.Close()

	target := ScrapeTarget{
		URL:    server.URL + "/metrics",
		Labels: map[string]string{"job": "seatunnel_engine_http", "instance": "10.0.0.4:5801"},
	}
	s := newTestScraper(staticProvider([]ScrapeTarget{target}))
	s.refreshTargets(ctx)
	s.scrapeRound(ctx)

	now := time.Now().Add(5 * time.Millisecond)
	vec := instantVector(t, ctx, `req_duration{quantile="0.9"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 2.5, vec[0].F)
	vec = instantVector(t, ctx, `req_duration_sum`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 12.0, vec[0].F)
	vec = instantVector(t, ctx, `req_duration_count`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 8.0, vec[0].F)

	vec = instantVector(t, ctx, `lat_bucket{le="1"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 5.0, vec[0].F)
	vec = instantVector(t, ctx, `lat_bucket{le="+Inf"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 9.0, vec[0].F)
	vec = instantVector(t, ctx, `lat_count`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 9.0, vec[0].F)
}

func TestScraper_ProviderFailureKeepsPreviousTargets(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	body := `# TYPE node_state gauge
node_state 1
`
	status := 200
	server := expositionServer(t, &body, &status)
	defer server.Close()

	target := ScrapeTarget{
		URL:    server.URL + "/metrics",
		Labels: map[string]string{"job": "seatunnel_engine_http", "instance": "10.0.0.5:5801"},
	}
	var targets []ScrapeTarget = []ScrapeTarget{target}
	var providerErr error
	s := newTestScraper(MetricsTargetProviderFunc(func(ctx context.Context) ([]ScrapeTarget, error) {
		return targets, providerErr
	}))
	s.refreshTargets(ctx)
	s.scrapeRound(ctx)

	// provider 失败：目标集沿用上一轮，不写 stale。
	// Provider failure keeps the last-known set and writes no stale markers.
	providerErr = errors.New("list failed")
	s.refreshTargets(ctx)
	require.Len(t, s.currentTargets(), 1)
	s.scrapeRound(ctx)
	vec := instantVector(t, ctx, `node_state{instance="10.0.0.5:5801"}`, time.Now().Add(5*time.Millisecond))
	require.Len(t, vec, 1)
}

func TestStartScraper_Lifecycle(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	err := StartScraper(ctx, nil, ScrapeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider is nil")

	require.NoError(t, StartScraper(ctx, staticProvider(nil), ScrapeOptions{Interval: time.Hour, RefreshInterval: time.Hour}))
	// 重复启动返回错误，已运行实例不受影响。
	// A second start errors without disturbing the running loop.
	require.Error(t, StartScraper(ctx, staticProvider(nil), ScrapeOptions{}))
	require.NotNil(t, snapshot().scraper)

	// Close 连动停止抓取协程。
	// Close stops the scrape goroutine as part of teardown.
	require.NoError(t, Close())
	require.Nil(t, snapshot())
}

func TestStartScraper_NotReady(t *testing.T) {
	_ = Close()
	err := StartScraper(context.Background(), staticProvider(nil), ScrapeOptions{})
	require.ErrorIs(t, err, errScraperNotReady)
}

func TestScraper_EmptyTargetsStillSelfScrapes(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	s := newTestScraper(staticProvider(nil))
	s.refreshTargets(ctx)
	s.scrapeRound(ctx)
	s.scrapeRound(ctx)

	vec := instantVector(t, ctx, `prometheus_tsdb_head_series{job="stx_embedded"}`, time.Now().Add(5*time.Millisecond))
	require.NotEmpty(t, vec)
	require.Greater(t, vec[0].F, 0.0)
}

func TestScraper_SampleLimitDropsOneTargetKeepsOther(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	okBody := `# TYPE node_state gauge
node_state 1
`
	fatBody := `# TYPE a gauge
a 1
# TYPE b gauge
b 2
# TYPE c gauge
c 3
`
	okStatus, fatStatus := 200, 200
	okServer := expositionServer(t, &okBody, &okStatus)
	defer okServer.Close()
	fatServer := expositionServer(t, &fatBody, &fatStatus)
	defer fatServer.Close()

	okTarget := ScrapeTarget{
		URL:    okServer.URL + "/metrics",
		Labels: map[string]string{"job": "seatunnel_engine_http", "instance": "10.0.0.8:5801"},
	}
	fatTarget := ScrapeTarget{
		URL:    fatServer.URL + "/metrics",
		Labels: map[string]string{"job": "seatunnel_engine_http", "instance": "10.0.0.9:5801"},
	}
	s := newTestScraper(staticProvider([]ScrapeTarget{okTarget, fatTarget}))
	s.opts.SampleLimit = 2
	s.refreshTargets(ctx)
	s.scrapeRound(ctx)

	now := time.Now().Add(5 * time.Millisecond)
	okUp := instantVector(t, ctx, `up{instance="10.0.0.8:5801"}`, now)
	require.Len(t, okUp, 1)
	require.Equal(t, 1.0, okUp[0].F)
	okMetric := instantVector(t, ctx, `node_state{instance="10.0.0.8:5801"}`, now)
	require.Len(t, okMetric, 1)

	fatUp := instantVector(t, ctx, `up{instance="10.0.0.9:5801"}`, now)
	require.Len(t, fatUp, 1)
	require.Equal(t, 0.0, fatUp[0].F)
	fatMetric := instantVector(t, ctx, `a{instance="10.0.0.9:5801"}`, now)
	require.Empty(t, fatMetric)

	states := s.targetStates()
	require.Len(t, states, 2)
	var fatState TargetState
	for _, state := range states {
		if state.URL == fatTarget.URL {
			fatState = state
		}
	}
	require.False(t, fatState.Healthy)
	require.Contains(t, fatState.LastError, errSampleLimitMsg)
}

func TestScrapeOptionsFromObservability(t *testing.T) {
	// 空配置走默认 15s/30s。
	// Empty config falls back to 15s/30s.
	opts, err := ScrapeOptionsFromObservability(configObservability(""))
	require.NoError(t, err)
	require.Equal(t, DefaultScrapeInterval, opts.Interval)
	require.Equal(t, DefaultTargetRefreshInterval, opts.RefreshInterval)
	require.Equal(t, DefaultSampleLimit, opts.SampleLimit)
	require.Equal(t, 10*time.Second, opts.scrapeTimeout())

	opts, err = ScrapeOptionsFromObservability(configObservability("5s"))
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, opts.Interval)
	// 超时取 min(10s, interval)。
	// The per-target timeout is min(10s, interval).
	require.Equal(t, 5*time.Second, opts.scrapeTimeout())

	_, err = ScrapeOptionsFromObservability(configObservability("abc"))
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "scrape_interval"))
}
