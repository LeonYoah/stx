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
 * See the License for the LICENSE file for more details.
 */

package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// QueryInstantData 是 GET /monitoring/query 的返回：即时点查。
// QueryInstantData is the payload of GET /monitoring/query: an instant / point query.
type QueryInstantData struct {
	Query       string             `json:"query"`
	AtMs        int64              `json:"at_ms"`
	ElapsedMs   float64            `json:"elapsed_ms"`
	SampleCount int                `json:"sample_count"`
	Samples     []promquery.Sample `json:"samples"`
}

// QueryRangeData 是 GET /monitoring/query_range 的返回：区间查询。
// QueryRangeData is the payload of GET /monitoring/query_range: a range query.
type QueryRangeData struct {
	Query       string             `json:"query"`
	StartMs     int64              `json:"start_ms"`
	EndMs       int64              `json:"end_ms"`
	StepSeconds int                `json:"step_seconds"`
	ElapsedMs   float64            `json:"elapsed_ms"`
	SeriesCount int                `json:"series_count"`
	Series      []promquery.Series `json:"series"`
}

// QueryInstant 执行一条 PromQL 即时查询（点查）。expr 为空返回错误。
// QueryInstant runs one PromQL instant (point) query. An empty expr is an error.
func (s *Service) QueryInstant(ctx context.Context, expr string, at time.Time) (*QueryInstantData, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("query is required")
	}
	if reason := promquery.UnavailableReason(); reason != "" {
		return nil, fmt.Errorf("%s", reason)
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	started := time.Now()
	samples, err := promquery.Instant(ctx, expr, at)
	elapsed := time.Since(started).Seconds() * 1000
	if err != nil {
		return nil, err
	}
	if samples == nil {
		samples = []promquery.Sample{}
	}
	return &QueryInstantData{
		Query:       expr,
		AtMs:        at.UTC().UnixMilli(),
		ElapsedMs:   elapsed,
		SampleCount: len(samples),
		Samples:     samples,
	}, nil
}

// QueryRange 执行一条 PromQL 区间查询。start/end 走 promquery.ParseWindow；step<=0 时按窗口自动选。
// QueryRange runs one PromQL range query. start/end use promquery.ParseWindow; step<=0 picks AutoStep.
func (s *Service) QueryRange(ctx context.Context, expr, startRaw, endRaw string, step int) (*QueryRangeData, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("query is required")
	}
	if reason := promquery.UnavailableReason(); reason != "" {
		return nil, fmt.Errorf("%s", reason)
	}
	window, err := promquery.ParseWindow(startRaw, endRaw, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if window == nil {
		end := time.Now().UTC()
		window = &promquery.Window{Start: end.Add(-time.Hour), End: end}
	}
	if step <= 0 {
		step = promquery.AutoStep(window.End.Sub(window.Start))
	}
	started := time.Now()
	series, err := promquery.Range(ctx, expr, window.Start, window.End, step)
	elapsed := time.Since(started).Seconds() * 1000
	if err != nil {
		return nil, err
	}
	if series == nil {
		series = []promquery.Series{}
	}
	return &QueryRangeData{
		Query:       expr,
		StartMs:     window.Start.UnixMilli(),
		EndMs:       window.End.UnixMilli(),
		StepSeconds: step,
		ElapsedMs:   elapsed,
		SeriesCount: len(series),
		Series:      series,
	}, nil
}

func queryHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if promquery.IsValidationError(err) {
		return http.StatusBadRequest
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, promquery.ReasonObservabilityDisabled),
		strings.Contains(msg, promquery.ReasonPrometheusUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// GetQuery 处理 GET /api/v1/monitoring/query：PromQL 即时点查。
// GetQuery handles GET /api/v1/monitoring/query: PromQL instant / point query.
// @Summary PromQL instant query
// @Tags Monitoring
// @Produce json
// @Param query query string true "PromQL expression"
// @Param time query string false "Evaluation instant: RFC3339 or unix timestamp"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /api/v1/monitoring/query [get]
func (h *Handler) GetQuery(c *gin.Context) {
	at := time.Time{}
	if raw := strings.TrimSpace(c.Query("time")); raw != "" {
		parsed, err := parseStandaloneInstant(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "invalid time: " + err.Error()})
			return
		}
		at = parsed
	}
	data, err := h.service.QueryInstant(c.Request.Context(), c.Query("query"), at)
	if err != nil {
		c.JSON(queryHTTPStatus(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

// GetQueryRange 处理 GET /api/v1/monitoring/query_range：PromQL 区间查询。
// GetQueryRange handles GET /api/v1/monitoring/query_range: PromQL range query.
// @Summary PromQL range query
// @Tags Monitoring
// @Produce json
// @Param query query string true "PromQL expression"
// @Param start query string false "Window start: RFC3339 or unix timestamp"
// @Param end query string false "Window end; default now"
// @Param step query int false "Step in seconds; 0 means auto"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /api/v1/monitoring/query_range [get]
func (h *Handler) GetQueryRange(c *gin.Context) {
	step := 0
	if raw := strings.TrimSpace(c.Query("step")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, Response{ErrorMsg: "step must be a non-negative integer"})
			return
		}
		step = parsed
	}
	data, err := h.service.QueryRange(c.Request.Context(), c.Query("query"), c.Query("start"), c.Query("end"), step)
	if err != nil {
		c.JSON(queryHTTPStatus(err), Response{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Data: data})
}

func parseStandaloneInstant(raw string) (time.Time, error) {
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if len(strings.TrimPrefix(raw, "-")) <= 10 {
			return time.Unix(n, 0).UTC(), nil
		}
		return time.UnixMilli(n).UTC(), nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("want RFC3339 or unix timestamp, got %q", raw)
	}
	return t.UTC(), nil
}
