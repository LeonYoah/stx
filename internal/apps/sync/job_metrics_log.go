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

package sync

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	jobMetricsLogInterval      = 30 * time.Second
	jobMetricsLogPreviewKey    = "stx_metrics_log"
	jobMetricsLogLastAtKey     = "stx_metrics_log_at"
	jobMetricsLogPreviewMaxLen = 64 * 1024
)

// FormatJobMetricsLogBlock 将 SeaTunnel / STX 已存 metrics 格式化为可读日志块（双语标签）。
// FormatJobMetricsLogBlock formats stored SeaTunnel/STX metrics into a readable log block (bilingual labels).
func FormatJobMetricsLogBlock(metrics map[string]interface{}, at time.Time, final bool) string {
	if len(metrics) == 0 {
		return ""
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	title := "STX metrics snapshot / 任务指标快照"
	if final {
		title = "STX metrics final / 任务终态指标"
	}
	readCount := metricNumber(metrics, "SourceReceivedCount")
	writeCount := metricNumber(metrics, "SinkWriteCount")
	if writeCount == nil {
		writeCount = metricNumber(metrics, "SinkCommittedCount")
	}
	readQPS := metricNumber(metrics, "SourceReceivedQPS")
	writeQPS := metricNumber(metrics, "SinkWriteQPS")
	if writeQPS == nil {
		writeQPS = metricNumber(metrics, "SinkCommittedQPS")
	}

	lines := []string{
		"---------- " + title + " ----------",
		fmt.Sprintf("time / 时间          : %s", at.UTC().Format(time.RFC3339)),
		fmt.Sprintf("read count / 读取行  : %s", formatMetricField(readCount)),
		fmt.Sprintf("write count / 写入行 : %s", formatMetricField(writeCount)),
		fmt.Sprintf("read QPS / 读取速率  : %s", formatMetricField(readQPS)),
		fmt.Sprintf("write QPS / 写入速率 : %s", formatMetricField(writeQPS)),
	}

	tableKeys := collectTableMetricKeys(metrics)
	if len(tableKeys) > 0 {
		lines = append(lines, fmt.Sprintf("tables / 表数量      : %d", len(tableKeys)))
		for _, key := range tableKeys {
			lines = append(lines, fmt.Sprintf("  - %-40s read=%s write=%s",
				key,
				formatMetricField(tableMetricNumber(metrics, "TableSourceReceivedCount", key)),
				formatMetricField(firstTableMetricNumber(metrics, key, "TableSinkWriteCount", "TableSinkCommittedCount")),
			))
		}
	}
	lines = append(lines, "----------------------------------------------")
	return strings.Join(lines, "\n")
}

// maybeRecordMetricsLog 在 result_preview 中按间隔追加指标日志块；终态强制再写一次。
// maybeRecordMetricsLog appends a metrics block into result_preview on an interval; always once more at terminal.
func maybeRecordMetricsLog(preview JSONMap, metrics map[string]interface{}, now time.Time, force bool) JSONMap {
	if len(metrics) == 0 {
		return preview
	}
	if preview == nil {
		preview = JSONMap{}
	}
	lastAt := parseMetricsLogAt(preview[jobMetricsLogLastAtKey])
	if !force && !lastAt.IsZero() && now.Sub(lastAt) < jobMetricsLogInterval {
		return preview
	}
	block := FormatJobMetricsLogBlock(metrics, now, force)
	if strings.TrimSpace(block) == "" {
		return preview
	}
	existing := strings.TrimSpace(fmt.Sprintf("%v", preview[jobMetricsLogPreviewKey]))
	combined := block
	if existing != "" && existing != "<nil>" {
		combined = existing + "\n" + block
	}
	if len(combined) > jobMetricsLogPreviewMaxLen {
		combined = combined[len(combined)-jobMetricsLogPreviewMaxLen:]
		if idx := strings.Index(combined, "\n"); idx >= 0 {
			combined = combined[idx+1:]
		}
	}
	preview[jobMetricsLogPreviewKey] = combined
	preview[jobMetricsLogLastAtKey] = now.UTC().Format(time.RFC3339Nano)
	return preview
}

func metricsLogFromPreview(preview JSONMap) string {
	if preview == nil {
		return ""
	}
	raw := strings.TrimSpace(fmt.Sprintf("%v", preview[jobMetricsLogPreviewKey]))
	if raw == "" || raw == "<nil>" {
		return ""
	}
	return normalizeLogNewlines(raw)
}

func latestMetricsBlockFromPreview(preview JSONMap) string {
	full := metricsLogFromPreview(preview)
	if full == "" {
		return ""
	}
	const marker = "---------- STX metrics"
	idx := strings.LastIndex(full, marker)
	if idx < 0 {
		return full
	}
	return strings.TrimSpace(full[idx:])
}

func parseMetricsLogAt(raw interface{}) time.Time {
	text := strings.TrimSpace(fmt.Sprintf("%v", raw))
	if text == "" || text == "<nil>" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, text); err == nil {
		return ts
	}
	return time.Time{}
}

func metricNumber(metrics map[string]interface{}, key string) *float64 {
	if metrics == nil {
		return nil
	}
	return coerceMetricNumber(metrics[key])
}

func tableMetricNumber(metrics map[string]interface{}, mapKey, tableKey string) *float64 {
	if metrics == nil {
		return nil
	}
	raw, ok := metrics[mapKey]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case map[string]interface{}:
		return coerceMetricNumber(typed[tableKey])
	case map[string]float64:
		if v, ok := typed[tableKey]; ok {
			out := v
			return &out
		}
	}
	return nil
}

func firstTableMetricNumber(metrics map[string]interface{}, tableKey string, mapKeys ...string) *float64 {
	for _, mapKey := range mapKeys {
		if v := tableMetricNumber(metrics, mapKey, tableKey); v != nil {
			return v
		}
	}
	return nil
}

func collectTableMetricKeys(metrics map[string]interface{}) []string {
	keys := map[string]struct{}{}
	for _, mapKey := range []string{"TableSourceReceivedCount", "TableSinkWriteCount", "TableSinkCommittedCount"} {
		raw, ok := metrics[mapKey]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case map[string]interface{}:
			for key := range typed {
				if strings.TrimSpace(key) != "" {
					keys[key] = struct{}{}
				}
			}
		case map[string]float64:
			for key := range typed {
				if strings.TrimSpace(key) != "" {
					keys[key] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func coerceMetricNumber(raw interface{}) *float64 {
	switch typed := raw.(type) {
	case nil:
		return nil
	case float64:
		v := typed
		return &v
	case float32:
		v := float64(typed)
		return &v
	case int:
		v := float64(typed)
		return &v
	case int64:
		v := float64(typed)
		return &v
	case json.Number:
		v, err := typed.Float64()
		if err != nil {
			return nil
		}
		return &v
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		var v float64
		if _, err := fmt.Sscanf(trimmed, "%f", &v); err != nil {
			return nil
		}
		return &v
	default:
		trimmed := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if trimmed == "" || trimmed == "<nil>" {
			return nil
		}
		var v float64
		if _, err := fmt.Sscanf(trimmed, "%f", &v); err != nil {
			return nil
		}
		return &v
	}
}

func formatMetricField(value *float64) string {
	if value == nil {
		return "-"
	}
	if *value == float64(int64(*value)) {
		return fmt.Sprintf("%d", int64(*value))
	}
	return fmt.Sprintf("%.3f", *value)
}
