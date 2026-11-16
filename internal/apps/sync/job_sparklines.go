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
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// jobSparklinesMaxIDs 是一次 sparklines 请求最多的作业数（与列表每页上限一致）。
// jobSparklinesMaxIDs caps job IDs per sparklines request (matching the list page size cap).
const jobSparklinesMaxIDs = 100

// JobSparklinesResult 是 GET /sync/jobs/sparklines 的返回：作业 ID → [毫秒时间戳, 值] 序列。
// JobSparklinesResult is the payload of GET /sync/jobs/sparklines: job ID → [epoch ms, value] points.
type JobSparklinesResult struct {
	Source      string                  `json:"source"`
	Reason      string                  `json:"reason,omitempty"`
	Range       string                  `json:"range"`
	Metric      string                  `json:"metric"`
	StepSeconds int                     `json:"step_seconds"`
	Items       map[string][][2]float64 `json:"items"`
}

// ParseJobIDList 解析逗号分隔的作业 ID，去重并校验上限。
// ParseJobIDList parses comma-separated job IDs, de-duplicates them and enforces the cap.
func ParseJobIDList(csv string) ([]uint, error) {
	seen := map[uint]struct{}{}
	var ids []uint
	for _, raw := range strings.Split(csv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || parsed == 0 {
			return nil, fmt.Errorf("%w: invalid job id %q", ErrInvalidJobListQuery, raw)
		}
		if _, ok := seen[uint(parsed)]; ok {
			continue
		}
		seen[uint(parsed)] = struct{}{}
		ids = append(ids, uint(parsed))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: ids is required", ErrInvalidJobListQuery)
	}
	if len(ids) > jobSparklinesMaxIDs {
		return nil, fmt.Errorf("%w: at most %d ids", ErrInvalidJobListQuery, jobSparklinesMaxIDs)
	}
	return ids, nil
}

// GetJobSparklinesForActor 用一条 PromQL 批量取多个作业的读或写速率；越权或不存在的 ID 静默丢弃。
// GetJobSparklinesForActor fetches read or write rates for many jobs with one PromQL; IDs the caller cannot view (or
// that do not exist) are silently dropped.
func (s *Service) GetJobSparklinesForActor(ctx context.Context, actor executionapp.Actor, ids []uint, rangeKey, metric string) (*JobSparklinesResult, error) {
	rangeKey = strings.ToLower(strings.TrimSpace(rangeKey))
	if rangeKey == "" {
		rangeKey = "1h"
	}
	dur, ok := timeseriesRangeDurations[rangeKey]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported range %q (want 15m|1h|6h|24h|7d)", ErrInvalidJobListQuery, rangeKey)
	}
	metric = strings.ToLower(strings.TrimSpace(metric))
	counter := ""
	switch metric {
	case "", "write":
		metric, counter = "write", "stx_job_sink_write_rows_total"
	case "read":
		counter = "stx_job_source_received_rows_total"
	default:
		return nil, fmt.Errorf("%w: metric must be read or write", ErrInvalidJobListQuery)
	}
	step := timeseriesRangeSteps[rangeKey]
	out := &JobSparklinesResult{Range: rangeKey, Metric: metric, StepSeconds: step, Items: map[string][][2]float64{}}

	baseURL, reason := promquery.BaseURL()
	if baseURL == "" {
		out.Source, out.Reason = promquery.SourceNone, reason
		return out, nil
	}
	filter := &JobFilter{IDs: ids}
	if err := s.scopeJobFilterForActor(ctx, actor, filter); err != nil {
		return nil, err
	}
	visible, _, err := s.ListJobs(ctx, filter)
	if err != nil {
		return nil, err
	}
	out.Source = promquery.SourcePrometheus
	if len(visible) == 0 {
		return out, nil
	}
	visibleIDs := make([]uint, 0, len(visible))
	for _, job := range visible {
		visibleIDs = append(visibleIDs, job.ID)
	}
	query := fmt.Sprintf(`sum by (job_id) (rate(%s{job_id=~%s}[1m]))`, counter, promquery.QuoteLabelRegex(visibleIDs))
	end := time.Now().UTC()
	series, err := promquery.Range(ctx, baseURL, query, end.Add(-dur), end, step)
	if err != nil {
		// Prometheus 查询失败不让列表报错：返回空结果并标明原因。
		// A failed Prometheus query must not break the list: return empty items with a reason.
		out.Reason = "prometheus_error"
		return out, nil
	}
	for _, item := range series {
		if id := strings.TrimSpace(item.Metric["job_id"]); id != "" {
			out.Items[id] = item.Points
		}
	}
	return out, nil
}
