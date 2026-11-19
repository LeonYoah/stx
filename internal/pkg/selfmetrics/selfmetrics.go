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

// Package selfmetrics 提供 STX server 自身的进程级可观测指标采集器：HTTP / gRPC
// 请求计数与延迟直方图、在线 Agent 数、构建信息。
// 采集器只依赖 prometheus client，与 apps 层无反向依赖；observability 关闭时
// 采集器照常计数但不写入 tsdb，embedts.Start 成功后由路由层调用
// embedts.RegisterSelfCollectors 完成注册。
// Package selfmetrics provides process-level self-observability collectors for the
// STX server: HTTP / gRPC request counters and latency histograms, connected-agent
// gauge and build info.
// Collectors depend only on the prometheus client and never import apps packages;
// with observability disabled they still count, and the router registers them via
// embedts.RegisterSelfCollectors once embedts.Start succeeds.
package selfmetrics

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const (
	// RouteUnmatched 是未匹配任何 gin 路由（404）时计数用的 route 标签值。
	// RouteUnmatched is the route label used when no gin route matched (404).
	RouteUnmatched = "unmatched"

	// namespace / subsystem 固定为 stx_server_*，保证指标名与文档契约一致。
	// namespace / subsystem are pinned so series keep the documented stx_server_* names.
	namespace = "stx"
	subsystem = "server"
)

// httpDurationBuckets 是 HTTP 延迟直方图的固定 bucket 清单（秒），
// 控制序列基数并保证 p95/p99 在不同部署间可比。
// httpDurationBuckets is the fixed bucket layout (seconds) of the HTTP latency
// histogram, keeping cardinality bounded and p95/p99 comparable across deployments.
var httpDurationBuckets = []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Metrics 是一组平台自监控采集器：先创建并接到中间件/拦截器上计数，
// 待内嵌 tsdb 就绪后再注册进 registry。
// Metrics bundles the platform self-monitoring collectors: create them early so
// middleware/interceptors already count, then register into the embedded registry
// once the tsdb is ready.
type Metrics struct {
	httpRequests    *prometheus.CounterVec
	httpDuration    *prometheus.HistogramVec
	grpcRequests    *prometheus.CounterVec
	grpcDuration    *prometheus.HistogramVec
	agentsConnected prometheus.GaugeFunc
	buildInfo       prometheus.Gauge
}

// New 创建平台自监控采集器集合。
// version 写入 stx_server_build_info{version} 常量标签（值恒为 1）；
// agentsConnected 为惰性回调，抓取时才求值，允许在 gRPC/Agent 管理器之后才注入。
// New builds the platform self-monitoring collector set.
// version lands on the constant label of stx_server_build_info{version} (value 1);
// agentsConnected is a lazy callback evaluated at gather time, so it may capture
// the gRPC/Agent manager before it is assigned.
func New(version string, agentsConnected func() float64) *Metrics {
	if agentsConnected == nil {
		agentsConnected = func() float64 { return 0 }
	}
	m := &Metrics{
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name: "http_requests_total",
			Help: "Total HTTP requests handled by the STX server, labeled by method, route template and status class.",
		}, []string{"method", "route", "code"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency of the STX server in seconds, excluding SSE / long-poll / health / static routes.",
			Buckets: httpDurationBuckets,
		}, []string{"method", "route"}),
		grpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name: "grpc_requests_total",
			Help: "Total gRPC calls handled by the STX agent server, labeled by full method and status code.",
		}, []string{"method", "code"}),
		grpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name:    "grpc_request_duration_seconds",
			Help:    "gRPC call latency of the STX agent server in seconds.",
			Buckets: httpDurationBuckets,
		}, []string{"method"}),
		agentsConnected: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name: "agents_connected",
			Help: "Number of STX Agents currently holding an online connection to the control plane.",
		}, agentsConnected),
		buildInfo: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: subsystem,
			Name:        "build_info",
			Help:        "Build information of the STX server; constant 1 with the version label.",
			ConstLabels: prometheus.Labels{"version": version},
		}),
	}
	m.buildInfo.Set(1)
	return m
}

// Collectors 返回需要注册进内嵌 registry 的全部采集器。
// Collectors returns every collector that must be registered into the embedded registry.
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.httpRequests,
		m.httpDuration,
		m.grpcRequests,
		m.grpcDuration,
		m.agentsConnected,
		m.buildInfo,
	}
}

// GinMiddleware 返回统计 HTTP 请求的 gin 中间件。
// route 标签取 c.FullPath() 的路由模板（如 /api/v1/sync/jobs/:id），未匹配路由
// 归为 unmatched，从源头卡死基数；状态码只按 2xx/3xx/4xx/5xx 类别计数。
// GinMiddleware returns a gin middleware that counts HTTP requests.
// The route label is c.FullPath() — the route template (e.g. /api/v1/sync/jobs/:id);
// unmatched paths collapse to "unmatched", bounding cardinality. Status codes are
// bucketed into the 2xx/3xx/4xx/5xx classes only.
func (m *Metrics) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = RouteUnmatched
		}
		code := statusCodeClass(c.Writer.Status())
		m.httpRequests.WithLabelValues(c.Request.Method, route, code).Inc()

		if observeDuration(c, route) {
			m.httpDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
		}
	}
}

// observeDuration 判定该请求是否计入延迟直方图。
// SSE / 长连接 / 长轮询会把 p99 拉成分钟级，健康探活与静态资源下载没有延迟价值，
// 它们只在 *_requests_total 计数，不进直方图。
// observeDuration decides whether the request feeds the latency histogram.
// SSE / long-lived / long-poll requests would stretch p99 to minutes, and health
// probes or static downloads carry no latency signal — they still count in
// *_requests_total but stay out of the histogram.
func observeDuration(c *gin.Context, route string) bool {
	if strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "text/event-stream") {
		return false
	}
	switch {
	case strings.HasSuffix(route, "/v1/health"),
		strings.HasSuffix(route, "/events/stream"),
		strings.HasSuffix(route, "/:id/wait"),
		strings.HasSuffix(route, "/agent/download"),
		strings.HasSuffix(route, "/agent/install.sh"),
		strings.HasSuffix(route, "/agent/uninstall.sh"),
		strings.HasSuffix(route, "/agent/assets/stx-java-proxy.jar"),
		strings.Contains(route, "/debug/pprof/"),
		strings.Contains(route, "/swagger/"),
		strings.Contains(route, "/webui"):
		return false
	}
	return true
}

// statusCodeClass 把 HTTP 状态码折叠成 2xx/3xx/4xx/5xx 类别；异常值归 other。
// statusCodeClass folds an HTTP status code into the 2xx/3xx/4xx/5xx classes;
// out-of-range values land in "other".
func statusCodeClass(status int) string {
	class := status / 100
	if class >= 2 && class <= 5 {
		return string(rune('0'+class)) + "xx"
	}
	return "other"
}

// UnaryServerInterceptor 返回 gRPC unary 拦截器：按完整方法名与 gRPC code 计数、
// 观测延迟。挂在 internal/grpc 的服务端拦截器链上，不依赖 go-grpc-middleware。
// UnaryServerInterceptor returns a gRPC unary interceptor counting calls by full
// method name and gRPC code while observing latency. It is chained into the
// server interceptors in internal/grpc without pulling in go-grpc-middleware.
func (m *Metrics) UnaryServerInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	m.recordGRPC(info.FullMethod, err, start)
	return resp, err
}

// StreamServerInterceptor 返回 gRPC stream 拦截器，语义同 UnaryServerInterceptor：
// 对命令流/心跳流按整个生命周期记一次请求与总时长。
// StreamServerInterceptor returns a gRPC stream interceptor with the same
// semantics as UnaryServerInterceptor: one request and one duration sample per
// command/heartbeat stream lifetime.
func (m *Metrics) StreamServerInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	err := handler(srv, ss)
	m.recordGRPC(info.FullMethod, err, start)
	return err
}

// recordGRPC 统一落 gRPC 计数与延迟样本。
// recordGRPC writes the shared gRPC counter and latency observation.
func (m *Metrics) recordGRPC(method string, err error, start time.Time) {
	code := status.Code(err).String()
	m.grpcRequests.WithLabelValues(method, code).Inc()
	m.grpcDuration.WithLabelValues(method).Observe(time.Since(start).Seconds())
}
