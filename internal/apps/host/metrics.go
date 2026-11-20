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

package host

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// HostMetricSample 是一条主机指标样本（平面结构，与 pb.HostMetricSample 对应；
// host 包不依赖 proto，gRPC 层负责转换）。
// HostMetricSample is one host metric sample (flat shape mirroring pb.HostMetricSample;
// the host package stays proto-free, the gRPC layer converts).
type HostMetricSample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// hostMetricsLabelCap / hostMetricsSampleCap 是单轮上报的防御性上限，防止异常样本撑爆 tsdb 写入。
// hostMetricsLabelCap / hostMetricsSampleCap are defensive bounds per report round so a bad
// batch cannot flood the tsdb write path.
const (
	hostMetricsSampleCap = 512
	hostMetricsLabelCap  = 16
	hostMetricsJob       = "stx_host"
	hostMetricsPrefix    = "stx_host_"
	// agentMetricsPrefix 是 Agent 进程自指标前缀：与主机指标走同一上报通道，
	// server 侧白名单同样放行。
	// agentMetricsPrefix is the Agent self-metrics prefix: it rides the same report
	// channel as host metrics, so the server-side whitelist admits it too.
	agentMetricsPrefix = "stx_agent_"
)

var (
	// hostMetricNameRe 放行 stx_host_*（主机指标）与 stx_agent_*（Agent 进程自指标）两个前缀。
	// hostMetricNameRe admits both stx_host_* (host metrics) and stx_agent_*
	// (Agent process self-metrics) prefixes.
	hostMetricNameRe = regexp.MustCompile(`^stx_(host|agent)_[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	hostLabelNameRe  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	// reservedHostLabels 是身份标签，样本维度不允许覆盖（防 agent 侧注入污染）。
	// reservedHostLabels are identity labels a sample's dimensions may not override
	// (guards against agent-side label injection).
	reservedHostLabels = map[string]bool{
		"__name__": true, "job": true, "instance": true, "host_id": true, "host_name": true,
	}

	// hostTrackers 按主机追踪上一轮写入的序列标签集，用于消失序列的 stale marker 差分。
	// hostTrackers tracks the label sets each host wrote last round, to diff out series that
	// disappeared and need stale markers.
	hostTrackers sync.Map // hostID → *embedts.SeriesTracker

	// hostWatermark 是主机指标写入方的全局时间戳水位线（同毫秒多轮写入必须错开）。
	// hostWatermark is the global write-timestamp watermark for the host-metrics writer
	// (back-to-back rounds inside one millisecond must be staggered).
	hostWatermark embedts.WriteTsWatermark
)

// WriteHostMetrics 把一轮主机样本追加进内嵌 tsdb：注入 job/host_id/host_name/instance 身份标签，
// 维度标签做白名单校验；未开启内嵌可观测时静默丢弃。
// 返回实际写入条数与错误（写入失败返回 err，脏样本只跳过不报错）。
// WriteHostMetrics appends one round of host samples into the embedded tsdb: identity labels
// (job/host_id/host_name/instance) are injected, dimension labels are whitelist-checked;
// when the embedded tsdb is not ready the round is dropped silently.
// Returns the written count and error (write failures return err; dirty samples are skipped).
func WriteHostMetrics(ctx context.Context, hostID uint, hostName, instance string, samples []HostMetricSample) (int, error) {
	app := embedts.Appender(ctx)
	if app == nil {
		return 0, nil
	}
	if len(samples) > hostMetricsSampleCap {
		samples = samples[:hostMetricsSampleCap]
	}
	flat := make([]embedts.FlatSample, 0, len(samples))
	lsets := make([]labels.Labels, 0, len(samples))
	for _, sample := range samples {
		if !hostMetricNameRe.MatchString(sample.Name) || math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
			continue
		}
		builder := labels.NewBuilder(labels.EmptyLabels())
		builder.Set(labels.MetricName, sample.Name)
		builder.Set("job", hostMetricsJob)
		builder.Set("host_id", strconv.FormatUint(uint64(hostID), 10))
		builder.Set("host_name", hostName)
		builder.Set("instance", instance)
		dims := 0
		for key, value := range sample.Labels {
			if dims >= hostMetricsLabelCap {
				break
			}
			if reservedHostLabels[key] || !hostLabelNameRe.MatchString(key) {
				continue
			}
			builder.Set(key, value)
			dims++
		}
		lset := builder.Labels()
		flat = append(flat, embedts.FlatSample{Labels: lset, Value: sample.Value})
		lsets = append(lsets, lset)
	}
	if len(flat) == 0 {
		return 0, nil
	}
	tsMs := hostWatermark.Next(time.Now().UnixMilli())
	if err := embedts.AppendFlatSamples(app, flat, tsMs); err != nil {
		return 0, err
	}
	if err := app.Commit(); err != nil {
		return 0, err
	}
	// 消失的序列（如盘被卸载、网卡消失）补 stale marker，防幽灵续存。
	// Series that disappeared (an unmounted disk, a vanished NIC) get stale markers to
	// prevent ghost persistence.
	tracker := hostTrackerFor(hostID)
	if stales := tracker.Diff(lsets); len(stales) > 0 {
		_ = embedts.AppendStaleMarkers(ctx, stales, hostWatermark.Next(time.Now().UnixMilli()))
	}
	return len(flat), nil
}

// MarkHostMetricsStale 在主机离线时对它的全部已知序列写 stale marker（等价于整台主机的
// 序列一起消失：tracker.Diff(nil)）。
// MarkHostMetricsStale writes stale markers for every known series of a host going offline
// (equivalent to all its series vanishing at once: tracker.Diff(nil)).
func MarkHostMetricsStale(ctx context.Context, hostID uint) {
	value, ok := hostTrackers.Load(hostID)
	if !ok {
		return
	}
	tracker, _ := value.(*embedts.SeriesTracker)
	stales := tracker.Diff(nil)
	hostTrackers.Delete(hostID)
	if len(stales) > 0 {
		_ = embedts.AppendStaleMarkers(ctx, stales, hostWatermark.Next(time.Now().UnixMilli()))
	}
}

// hostTrackerFor 取（或新建）某主机的序列追踪器。
// hostTrackerFor returns (or creates) the series tracker of a host.
func hostTrackerFor(hostID uint) *embedts.SeriesTracker {
	value, _ := hostTrackers.LoadOrStore(hostID, embedts.NewSeriesTracker())
	tracker, _ := value.(*embedts.SeriesTracker)
	return tracker
}
