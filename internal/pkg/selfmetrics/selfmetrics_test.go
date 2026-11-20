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
package selfmetrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// gather 把采集器注册进独立 registry 后聚合，返回按名字索引的指标族。
// gather registers the collectors on a fresh registry and returns families keyed by name.
func gather(t *testing.T, m *Metrics) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	for _, c := range m.Collectors() {
		require.NoError(t, reg.Register(c))
	}
	mfs, err := reg.Gather()
	require.NoError(t, err)
	out := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

// labelValue 读取样本上某个标签的值，缺失返回空串。
// labelValue reads one label off a sample; missing labels yield "".
func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

// findMetric 在指标族内按标签键值匹配样本。
// findMetric locates a sample inside a family by exact label pairs.
func findMetric(mf *dto.MetricFamily, labels map[string]string) *dto.Metric {
	if mf == nil {
		return nil
	}
	for _, m := range mf.GetMetric() {
		match := true
		for k, v := range labels {
			if labelValue(m, k) != v {
				match = false
				break
			}
		}
		if match {
			return m
		}
	}
	return nil
}

func counterValue(t *testing.T, mf *dto.MetricFamily, labels map[string]string) float64 {
	t.Helper()
	m := findMetric(mf, labels)
	require.NotNil(t, m, "expected sample with labels %v", labels)
	return m.GetCounter().GetValue()
}

// serve 挂载中间件与给定路由后依次打请求。
// serve wires the middleware and routes, then performs the requests.
func serve(m *Metrics, register func(r *gin.Engine), requests ...*http.Request) {
	r := gin.New()
	r.Use(m.GinMiddleware())
	register(r)
	for _, req := range requests {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func TestGinMiddleware_RouteTemplateAndCodeClass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := New("1.0.0", nil)

	serve(m, func(r *gin.Engine) {
		r.GET("/api/v1/jobs/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
		r.POST("/api/v1/items", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
		r.GET("/api/v1/forbidden", func(c *gin.Context) { c.Status(http.StatusForbidden) })
	},
		httptest.NewRequest(http.MethodGet, "/api/v1/jobs/42?debug=1", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/jobs/7", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/items", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/forbidden", nil),
	)

	fams := gather(t, m)
	requests := fams["stx_server_http_requests_total"]
	require.NotNil(t, requests)
	// route 标签必须是模板而非原始路径，计数按请求聚合。
	// The route label must be the template, not the raw path; counts aggregate per route.
	require.Equal(t, 2.0, counterValue(t, requests, map[string]string{
		"method": http.MethodGet, "route": "/api/v1/jobs/:id", "code": "2xx",
	}))
	require.Equal(t, 1.0, counterValue(t, requests, map[string]string{
		"method": http.MethodPost, "route": "/api/v1/items", "code": "5xx",
	}))
	require.Equal(t, 1.0, counterValue(t, requests, map[string]string{
		"method": http.MethodGet, "route": "/api/v1/forbidden", "code": "4xx",
	}))

	// 正常路由进入延迟直方图。
	// Matched routes land in the latency histogram.
	duration := fams["stx_server_http_request_duration_seconds"]
	require.NotNil(t, duration)
	d := findMetric(duration, map[string]string{"method": http.MethodGet, "route": "/api/v1/jobs/:id"})
	require.NotNil(t, d)
	require.Equal(t, uint64(2), d.GetHistogram().GetSampleCount())
}

func TestGinMiddleware_UnmatchedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := New("1.0.0", nil)

	serve(m, func(r *gin.Engine) {
		r.GET("/api/v1/only", func(c *gin.Context) { c.Status(http.StatusOK) })
	},
		httptest.NewRequest(http.MethodGet, "/totally/unknown/path", nil),
	)

	fams := gather(t, m)
	require.Equal(t, 1.0, counterValue(t, fams["stx_server_http_requests_total"], map[string]string{
		"method": http.MethodGet, "route": RouteUnmatched, "code": "4xx",
	}))
}

func TestGinMiddleware_ExcludedRoutesSkipHistogram(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := New("1.0.0", nil)

	serve(m, func(r *gin.Engine) {
		r.GET("/api/v1/health", func(c *gin.Context) { c.Status(http.StatusOK) })
		r.GET("/api/v1/tasks/:id/events/stream", func(c *gin.Context) {
			c.Writer.Header().Set("Content-Type", "text/event-stream")
			c.Status(http.StatusOK)
		})
		r.GET("/api/v1/executions/:id/wait", func(c *gin.Context) { c.Status(http.StatusOK) })
	},
		httptest.NewRequest(http.MethodGet, "/api/v1/health", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks/9/events/stream", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/executions/3/wait", nil),
	)

	fams := gather(t, m)
	requests := fams["stx_server_http_requests_total"]
	// 排除路由仍计数，但不进延迟直方图。
	// Excluded routes still count but never reach the latency histogram.
	for _, route := range []string{"/api/v1/health", "/api/v1/tasks/:id/events/stream", "/api/v1/executions/:id/wait"} {
		require.Equal(t, 1.0, counterValue(t, requests, map[string]string{
			"method": http.MethodGet, "route": route, "code": "2xx",
		}))
		require.Nil(t, findMetric(fams["stx_server_http_request_duration_seconds"], map[string]string{
			"method": http.MethodGet, "route": route,
		}), "route %s must stay out of the histogram", route)
	}
}

func TestStatusCodeClass(t *testing.T) {
	require.Equal(t, "2xx", statusCodeClass(200))
	require.Equal(t, "2xx", statusCodeClass(204))
	require.Equal(t, "3xx", statusCodeClass(302))
	require.Equal(t, "4xx", statusCodeClass(404))
	require.Equal(t, "5xx", statusCodeClass(500))
	require.Equal(t, "other", statusCodeClass(100))
	require.Equal(t, "other", statusCodeClass(700))
}

// fakeServerStream 是满足 grpc.ServerStream 接口的最小实现。
// fakeServerStream is the smallest grpc.ServerStream implementation.
type fakeServerStream struct{ grpc.ServerStream }

func (fakeServerStream) Context() context.Context { return context.Background() }
func (fakeServerStream) SetHeader(metadata.MD) error {
	return nil
}
func (fakeServerStream) SendHeader(metadata.MD) error { return nil }
func (fakeServerStream) SetTrailer(metadata.MD)       {}
func (fakeServerStream) SendMsg(any) error            { return nil }
func (fakeServerStream) RecvMsg(any) error            { return nil }

func TestGRPCInterceptors_CountAndObserve(t *testing.T) {
	m := New("1.0.0", nil)

	_, err := m.UnaryServerInterceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/stx.agent.AgentService/Heartbeat"},
		func(ctx context.Context, req any) (any, error) { return nil, nil })
	require.NoError(t, err)

	_, err = m.UnaryServerInterceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/stx.agent.AgentService/Heartbeat"},
		func(ctx context.Context, req any) (any, error) {
			return nil, status.Error(codes.NotFound, "missing")
		})
	require.Error(t, err)

	err = m.StreamServerInterceptor(nil, fakeServerStream{},
		&grpc.StreamServerInfo{FullMethod: "/stx.agent.AgentService/CommandStream"},
		func(srv any, ss grpc.ServerStream) error { return nil })
	require.NoError(t, err)

	fams := gather(t, m)
	requests := fams["stx_server_grpc_requests_total"]
	require.Equal(t, 1.0, counterValue(t, requests, map[string]string{
		"method": "/stx.agent.AgentService/Heartbeat", "code": codes.OK.String(),
	}))
	require.Equal(t, 1.0, counterValue(t, requests, map[string]string{
		"method": "/stx.agent.AgentService/Heartbeat", "code": codes.NotFound.String(),
	}))
	require.Equal(t, 1.0, counterValue(t, requests, map[string]string{
		"method": "/stx.agent.AgentService/CommandStream", "code": codes.OK.String(),
	}))

	duration := fams["stx_server_grpc_request_duration_seconds"]
	d := findMetric(duration, map[string]string{"method": "/stx.agent.AgentService/Heartbeat"})
	require.NotNil(t, d)
	require.Equal(t, uint64(2), d.GetHistogram().GetSampleCount())
}

func TestBuildInfoAndAgentsConnected(t *testing.T) {
	m := New("2.5.0", func() float64 { return 3 })

	fams := gather(t, m)
	build := findMetric(fams["stx_server_build_info"], map[string]string{"version": "2.5.0"})
	require.NotNil(t, build)
	require.Equal(t, 1.0, build.GetGauge().GetValue())

	agents := fams["stx_server_agents_connected"]
	require.NotNil(t, agents)
	require.Equal(t, 3.0, agents.GetMetric()[0].GetGauge().GetValue())
}
