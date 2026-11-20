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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// TestRegisterSelfCollectors_NotReady 锁定未启动时的哨兵错误契约。
// TestRegisterSelfCollectors_NotReady locks the sentinel-error contract while the store is down.
func TestRegisterSelfCollectors_NotReady(t *testing.T) {
	_ = Close()
	err := RegisterSelfCollectors(prometheus.NewCounter(prometheus.CounterOpts{Name: "stx_test_counter"}))
	require.ErrorIs(t, err, errSelfNotReady)
}

// TestRegisterSelfCollectors_WritesAfterScrape 验证后注册的采集器也能被自抓取写回 tsdb，
// 且重复注册静默跳过（Start 已把 registry 建好，注册可延迟到 Start 之后）。
// TestRegisterSelfCollectors_WritesAfterScrape proves collectors registered after Start are
// picked up by the next self-scrape, and duplicate registrations are skipped silently.
func TestRegisterSelfCollectors_WritesAfterScrape(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stx_server_build_info",
		Help: "test build info",
	}, []string{"version"})
	buildInfo.WithLabelValues("9.9.9").Set(1)

	require.NoError(t, RegisterSelfCollectors(buildInfo, nil))
	// 重复注册同一采集器应按幂等处理，不报错。
	// Re-registering the same collector is idempotent and must not error.
	require.NoError(t, RegisterSelfCollectors(buildInfo))

	require.NoError(t, SelfScrapeOnce(ctx))
	now := time.Now().Add(5 * time.Millisecond)

	samples := instantVector(t, ctx, `stx_server_build_info{job="stx_embedded",version="9.9.9"}`, now)
	require.Len(t, samples, 1)
	require.Equal(t, 1.0, samples[0].F)
	require.Equal(t, InstanceEmbedded, samples[0].Metric.Get("instance"))
}
