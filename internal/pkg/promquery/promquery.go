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
// Package promquery 是 STX 后端代查 Prometheus 的公共客户端：区间 / 即时查询、时间窗解析与可用性判定。
// PromQL 模板由各业务模块持有，前端不直连 Prometheus。
// Package promquery is the shared client STX uses to query Prometheus on the user's behalf: range / instant queries,
// window parsing and availability checks. PromQL templates stay in each feature module; the browser never talks to
// Prometheus directly.
package promquery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

// 数据来源与不可用原因，和作业趋势接口的 source / reason 取值一致。
// Data source and unavailability reasons, matching the job timeseries source / reason values.
const (
	SourcePrometheus = "prometheus"
	SourceNone       = "none"

	ReasonObservabilityDisabled = "observability_disabled"
	ReasonPrometheusUnavailable = "prometheus_unavailable"
)

// Series 是一条区间查询结果；Points 为 [毫秒时间戳, 值]。
// Series is one range query result; Points are [epoch ms, value].
type Series struct {
	Metric map[string]string
	Points [][2]float64
}

// Sample 是一条即时查询结果。
// Sample is one instant query result.
type Sample struct {
	Metric map[string]string
	TsMs   int64
	Value  float64
}

// BaseURL 返回已配置的 Prometheus 地址；不可用时返回空串与原因（observability 关闭或未配置地址）。
// BaseURL returns the configured Prometheus URL, or "" plus a reason when observability is off or no URL is set.
func BaseURL() (string, string) {
	if config.Config == nil || !config.Config.Observability.Enabled {
		return "", ReasonObservabilityDisabled
	}
	base := strings.TrimSpace(config.Config.Observability.Prometheus.URL)
	if base == "" {
		return "", ReasonPrometheusUnavailable
	}
	return base, ""
}

type apiResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// httpClient 统一 8 秒超时，单个大盘面板不应拖住整页。
// httpClient uses an 8s timeout so one slow panel cannot hold up the whole page.
var httpClient = &http.Client{Timeout: 8 * time.Second}

func call(ctx context.Context, baseURL, path string, params url.Values) (*apiResponse, error) {
	apiURL := strings.TrimRight(strings.TrimSpace(baseURL), "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("prometheus %s status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var payload apiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Status), "success") {
		return nil, fmt.Errorf("prometheus %s status %q: %s", path, payload.Status, strings.TrimSpace(payload.Error))
	}
	return &payload, nil
}

// Range 执行 query_range，跳过无法解析的样本。
// Range runs query_range and skips samples that cannot be parsed.
func Range(ctx context.Context, baseURL, promQL string, start, end time.Time, stepSeconds int) ([]Series, error) {
	params := url.Values{}
	params.Set("query", promQL)
	params.Set("start", strconv.FormatInt(start.Unix(), 10))
	params.Set("end", strconv.FormatInt(end.Unix(), 10))
	params.Set("step", strconv.Itoa(stepSeconds))
	payload, err := call(ctx, baseURL, "/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}
	out := make([]Series, 0, len(payload.Data.Result))
	for _, item := range payload.Data.Result {
		series := Series{Metric: item.Metric, Points: make([][2]float64, 0, len(item.Values))}
		for _, sample := range item.Values {
			if len(sample) < 2 {
				continue
			}
			ts, tsOK := CoerceSample(sample[0])
			val, valOK := CoerceSample(sample[1])
			if !tsOK || !valOK {
				continue
			}
			series.Points = append(series.Points, [2]float64{ts * 1000, val})
		}
		out = append(out, series)
	}
	return out, nil
}

// Instant 执行即时查询（time 为零值时取服务端当前时间）。
// Instant runs an instant query (server time when at is zero).
func Instant(ctx context.Context, baseURL, promQL string, at time.Time) ([]Sample, error) {
	params := url.Values{}
	params.Set("query", promQL)
	if !at.IsZero() {
		params.Set("time", strconv.FormatInt(at.Unix(), 10))
	}
	payload, err := call(ctx, baseURL, "/api/v1/query", params)
	if err != nil {
		return nil, err
	}
	out := make([]Sample, 0, len(payload.Data.Result))
	for _, item := range payload.Data.Result {
		if len(item.Value) < 2 {
			continue
		}
		ts, tsOK := CoerceSample(item.Value[0])
		val, valOK := CoerceSample(item.Value[1])
		if !tsOK || !valOK {
			continue
		}
		out = append(out, Sample{Metric: item.Metric, TsMs: int64(ts * 1000), Value: val})
	}
	return out, nil
}

// CoerceSample 把 Prometheus 返回的时间戳或值（数字或字符串）转成 float64。
// CoerceSample converts a Prometheus timestamp or value (number or string) to float64.
func CoerceSample(raw interface{}) (float64, bool) {
	switch typed := raw.(type) {
	case float64:
		return typed, true
	case json.Number:
		v, err := typed.Float64()
		return v, err == nil
	case string:
		v, err := strconv.ParseFloat(typed, 64)
		return v, err == nil
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

// QuoteLabelRegex 把一组 ID 拼成 PromQL 正则标签值（"1|2|3"），只接受数字，杜绝注入。
// QuoteLabelRegex joins IDs into a PromQL regex label value ("1|2|3"); numbers only, so nothing can be injected.
func QuoteLabelRegex(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strconv.Quote(strings.Join(parts, "|"))
}
