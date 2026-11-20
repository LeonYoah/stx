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

package promquery

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// setObservabilityEnabled 开关 observability.enabled 并在用例结束时还原。
// setObservabilityEnabled flips observability.enabled and restores it at test end.
func setObservabilityEnabled(t *testing.T, enabled bool) {
	t.Helper()
	previous := config.Config.Observability.Enabled
	config.Config.Observability.Enabled = enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = previous })
}

// startEmbeddedForTest 起一份指向 t.TempDir() 的内嵌实例，结束时关闭。
// startEmbeddedForTest starts an embedded instance rooted at t.TempDir() and closes it at test end.
func startEmbeddedForTest(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, embedts.Start(ctx, embedts.Options{
		Dir:               t.TempDir(),
		RetentionDuration: 24 * time.Hour,
		QueryTimeout:      5 * time.Second,
		MaxSamples:        1000,
	}))
	t.Cleanup(func() { _ = embedts.Close() })
}

func TestUnavailableReason_ObservabilityDisabled(t *testing.T) {
	setObservabilityEnabled(t, false)
	require.Equal(t, ReasonObservabilityDisabled, UnavailableReason())
}

func TestUnavailableReason_NotReady(t *testing.T) {
	setObservabilityEnabled(t, true)
	_ = embedts.Close()
	require.Equal(t, ReasonPrometheusUnavailable, UnavailableReason())
}

func TestUnavailableReason_Ready(t *testing.T) {
	setObservabilityEnabled(t, true)
	ctx := context.Background()
	startEmbeddedForTest(t, ctx)
	require.Empty(t, UnavailableReason())
}

func TestRangeInstant_NotReady(t *testing.T) {
	_ = embedts.Close()
	ctx := context.Background()
	_, err := Range(ctx, "up", time.Now().Add(-time.Hour), time.Now(), 60)
	require.Error(t, err)
	_, err = Instant(ctx, "up", time.Now())
	require.Error(t, err)
}

// TestRange_MatrixExpansion 验证 promql.Matrix 到 Series 的展开（时间戳毫秒、Metric 拍平成 map）。
// TestRange_MatrixExpansion verifies the promql.Matrix → Series expansion (ms timestamps,
// Metric flattened to map).
func TestRange_MatrixExpansion(t *testing.T) {
	setObservabilityEnabled(t, true)
	ctx := context.Background()
	startEmbeddedForTest(t, ctx)

	now := time.Now().UTC()
	baseMs := now.Add(-2 * time.Minute).UnixMilli()
	app := embedts.Appender(ctx)
	require.NotNil(t, app)
	for i := 0; i < 4; i++ {
		for _, instance := range []string{"a", "b"} {
			_, err := app.Append(0,
				labels.FromStrings(labels.MetricName, "stx_promquery_metric", "instance", instance, "job", "stx"),
				baseMs+int64(i)*15*1000, float64(i)+1)
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())

	// 窗口收紧到已写入的样本区间：PromQL 每步按 lookback 取最后样本，超出样本时段会继续延续末值。
	// The window is bounded to the appended span: PromQL evaluates every step with lookback, so
	// beyond the samples' lifetime the last value would keep carrying forward.
	series, err := Range(ctx, `sum by (instance) (stx_promquery_metric)`,
		time.UnixMilli(baseMs), time.UnixMilli(baseMs+45*1000), 15)
	require.NoError(t, err)
	require.Len(t, series, 2)
	require.Equal(t, "a", series[0].Metric["instance"])
	require.Equal(t, "b", series[1].Metric["instance"])
	require.Len(t, series[0].Points, 4)
	require.Equal(t, [2]float64{float64(baseMs), 1}, series[0].Points[0])
	require.Equal(t, [2]float64{float64(baseMs + 45*1000), 4}, series[0].Points[3])
}

// TestInstant_VectorExpansion 验证 promql.Vector 到 Sample 的展开。
// TestInstant_VectorExpansion verifies the promql.Vector → Sample expansion.
func TestInstant_VectorExpansion(t *testing.T) {
	setObservabilityEnabled(t, true)
	ctx := context.Background()
	startEmbeddedForTest(t, ctx)

	now := time.Now().UTC()
	app := embedts.Appender(ctx)
	require.NotNil(t, app)
	_, err := app.Append(0, labels.FromStrings(labels.MetricName, "stx_promquery_instant", "instance", "a"), now.Add(-30*time.Second).UnixMilli(), 7.5)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	samples, err := Instant(ctx, "stx_promquery_instant", now)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.Equal(t, "a", samples[0].Metric["instance"])
	require.Equal(t, 7.5, samples[0].Value)
	require.Equal(t, now.UnixMilli(), samples[0].TsMs)
}

// TestInstant_ScalarResultErrors 验证非标量/向量结果（如标量表达式）会走错误路径而不是静默空值。
// TestInstant_ScalarResultErrors verifies scalar expressions take the error path rather than
// silently returning an empty list.
func TestInstant_ScalarResultErrors(t *testing.T) {
	setObservabilityEnabled(t, true)
	ctx := context.Background()
	startEmbeddedForTest(t, ctx)
	_, err := Instant(ctx, "42", time.Now())
	require.Error(t, err)
}

func TestRangeWindow(t *testing.T) {
	t.Parallel()
	require.Equal(t, "1m", RangeWindow(30, time.Minute))
	require.Equal(t, "1m", RangeWindow(60, time.Minute))
	require.Equal(t, "5m", RangeWindow(60, 5*time.Minute))
	require.Equal(t, "10m", RangeWindow(600, 5*time.Minute))
	require.Equal(t, "2d", RangeWindow(172800, time.Minute))
}

func TestSampleGauge(t *testing.T) {
	t.Parallel()
	expr := `sum by (instance) (jvm_threads_current{a="1"})`
	// 步长 ≤5m 原样返回。/ Steps at or below 5m pass through verbatim.
	require.Equal(t, expr, SampleGauge(300, "avg", expr))
	require.Equal(t, expr, SampleGauge(60, "max", expr))
	require.Equal(t, `sum by (instance) (avg_over_time(jvm_threads_current{a="1"}[1h]))`, SampleGauge(3600, "avg", expr))
	// 多选择器表达式（如 offheap 差值）每个选择器都包装。/ Compound exprs wrap every selector.
	got := SampleGauge(172800, "max", `max by (instance) (process_resident_memory_bytes{a="1"}) - on (instance) max by (instance) (jvm_memory_bytes_used{a="1",area="heap"})`)
	require.Equal(t, `max by (instance) (max_over_time(process_resident_memory_bytes{a="1"}[2d])) - on (instance) max by (instance) (max_over_time(jvm_memory_bytes_used{a="1",area="heap"}[2d]))`, got)
}
