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
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// TestFlattenMetricFamiliesLimited_OverLimitSkipsMaterializing 超限时返回 nil 切片，且计数在 stopAt 处截断。
// TestFlattenMetricFamiliesLimited_OverLimitSkipsMaterializing returns a nil slice on overflow
// and stops the pre-count at stopAt instead of walking every series.
func TestFlattenMetricFamiliesLimited_OverLimitSkipsMaterializing(t *testing.T) {
	mfs := []*dto.MetricFamily{gaugeFamily("fat_metric", 100_000)}
	samples, err := FlattenMetricFamiliesLimited(mfs, map[string]string{"job": "t"}, 2)
	require.Error(t, err)
	require.Nil(t, samples)
	require.Contains(t, err.Error(), errSampleLimitMsg)
	require.Equal(t, 3, countFlattenedSamples(mfs, 3), "must stop counting at stopAt instead of walking every series")
}

// TestFlattenMetricFamiliesLimited_UnderLimitMatchesUnlimited 未超限时结果与无限制展平一致。
// TestFlattenMetricFamiliesLimited_UnderLimitMatchesUnlimited matches the unlimited flatten when under the cap.
func TestFlattenMetricFamiliesLimited_UnderLimitMatchesUnlimited(t *testing.T) {
	mfs := []*dto.MetricFamily{gaugeFamily("ok_metric", 3)}
	injected := map[string]string{"job": "t"}
	got, err := FlattenMetricFamiliesLimited(mfs, injected, 3)
	require.NoError(t, err)
	require.Equal(t, FlattenMetricFamilies(mfs, injected), got)
}

// TestFlattenMetricFamiliesLimited_UnlimitedWhenLimitNonPositive limit<=0 时不截断。
// TestFlattenMetricFamiliesLimited_UnlimitedWhenLimitNonPositive leaves the flatten unbounded when limit<=0.
func TestFlattenMetricFamiliesLimited_UnlimitedWhenLimitNonPositive(t *testing.T) {
	mfs := []*dto.MetricFamily{gaugeFamily("ok_metric", 4)}
	got, err := FlattenMetricFamiliesLimited(mfs, nil, 0)
	require.NoError(t, err)
	require.Len(t, got, 4)
}

// TestCountFlattenedSamples_HistogramAndSummary 直方图/摘要按 bucket|quantile + sum + count 计数。
// TestCountFlattenedSamples_HistogramAndSummary counts histogram/summary as buckets|quantiles plus sum and count.
func TestCountFlattenedSamples_HistogramAndSummary(t *testing.T) {
	histType := dto.MetricType_HISTOGRAM
	sumType := dto.MetricType_SUMMARY
	nameH := "h"
	nameS := "s"
	le := 1.0
	q := 0.9
	hist := &dto.MetricFamily{
		Name: &nameH,
		Type: &histType,
		Metric: []*dto.Metric{{
			Histogram: &dto.Histogram{
				Bucket: []*dto.Bucket{{CumulativeCount: uint64Ptr(1), UpperBound: &le}},
			},
		}},
	}
	summary := &dto.MetricFamily{
		Name: &nameS,
		Type: &sumType,
		Metric: []*dto.Metric{{
			Summary: &dto.Summary{
				Quantile: []*dto.Quantile{{Quantile: &q, Value: &le}},
			},
		}},
	}
	require.Equal(t, 3, countFlattenedSamples([]*dto.MetricFamily{hist}, 0)) // bucket + sum + count
	require.Equal(t, 3, countFlattenedSamples([]*dto.MetricFamily{summary}, 0))
	samples, err := FlattenMetricFamiliesLimited([]*dto.MetricFamily{hist}, nil, 2)
	require.Error(t, err)
	require.Nil(t, samples)
}

func gaugeFamily(name string, n int) *dto.MetricFamily {
	typ := dto.MetricType_GAUGE
	mf := &dto.MetricFamily{Name: &name, Type: &typ, Metric: make([]*dto.Metric, n)}
	for i := 0; i < n; i++ {
		v := float64(i)
		mf.Metric[i] = &dto.Metric{Gauge: &dto.Gauge{Value: &v}}
	}
	return mf
}

func uint64Ptr(v uint64) *uint64 { return &v }
