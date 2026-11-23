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

// Package debugpprof 提供进程剖析：本机 loopback 监听（go tool pprof）以及管理员 API 用的命名 profile 写出。
// Package debugpprof serves process profiles: a loopback listener for go tool pprof, and named-profile
// writers for the admin API.
package debugpprof

import (
	"context"
	"fmt"
	"net"
	"net/http"
	httppprof "net/http/pprof"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/logger"
)

const (
	// DefaultAddr 是默认本机剖析地址，只绑 loopback。
	// DefaultAddr is the default local profile address, loopback only.
	DefaultAddr = "127.0.0.1:6060"
	// blockProfileRate 传给 runtime.SetBlockProfileRate，单位是纳秒：平均每阻塞 1µs 采样一次。
	// 配置开启后一直生效，不是“每 1000 次事件记一次”，开销不是零。
	// blockProfileRate is passed to runtime.SetBlockProfileRate in nanoseconds: sample once per 1µs of blocking on average.
	// It stays on while pprof is enabled; it is not “1 in 1000 events”, and the cost is not zero.
	blockProfileRate = 1000
	// mutexProfileFraction 每 5 次 mutex 争用记一次。
	// mutexProfileFraction records one of every 5 mutex contentions.
	mutexProfileFraction = 5
)

// Options 是本机 pprof 监听参数。
// Options holds the local pprof listener settings.
type Options struct {
	Enabled bool
	Addr    string
}

// withDefaults 填默认监听地址。
// withDefaults fills the default listen address.
func (o Options) withDefaults() Options {
	if strings.TrimSpace(o.Addr) == "" {
		o.Addr = DefaultAddr
	}
	return o
}

var (
	serverMu   sync.Mutex
	server     *http.Server
	listenAddr string
	listenErr  error
)

// ListenAddr 返回本机 pprof 实际监听地址；未启动或绑失败时为空。
// ListenAddr returns the local pprof bind address, or empty when it is not running or bind failed.
func ListenAddr() string {
	serverMu.Lock()
	defer serverMu.Unlock()
	return listenAddr
}

// ListenError 返回最近一次本机监听失败原因；未尝试或已成功时为 nil。
// ListenError returns the last local listen failure; nil when it was not attempted or succeeded.
func ListenError() error {
	serverMu.Lock()
	defer serverMu.Unlock()
	return listenErr
}

// NewMux 注册标准 /debug/pprof 路由，不污染 DefaultServeMux。
// NewMux registers the standard /debug/pprof routes without touching DefaultServeMux.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", httppprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", httppprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", httppprof.Trace)
	return mux
}

// ServeNamed 写出一个命名 profile（heap / goroutine / profile / trace 等）。
// ServeNamed writes one named profile (heap / goroutine / profile / trace, and so on).
func ServeNamed(w http.ResponseWriter, r *http.Request, name string) {
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" {
		http.Error(w, "profile name is required", http.StatusBadRequest)
		return
	}
	switch name {
	case "cmdline":
		httppprof.Cmdline(w, r)
	case "profile":
		httppprof.Profile(w, r)
	case "symbol":
		httppprof.Symbol(w, r)
	case "trace":
		httppprof.Trace(w, r)
	default:
		httppprof.Handler(name).ServeHTTP(w, r)
	}
}

// Start 在 Addr 上启动本机 pprof；未开启则为空操作。重复 Start 先停旧监听。
// Start serves local pprof on Addr; a no-op when disabled. A second Start stops the previous listener first.
func Start(ctx context.Context, opts Options) error {
	opts = opts.withDefaults()
	if !opts.Enabled {
		return nil
	}
	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		listenFail := fmt.Errorf("listen pprof on %s: %w", opts.Addr, err)
		serverMu.Lock()
		listenAddr = ""
		listenErr = listenFail
		serverMu.Unlock()
		return listenFail
	}
	if host, _, splitErr := net.SplitHostPort(ln.Addr().String()); splitErr == nil && !isLoopbackHost(host) {
		logger.WarnF(ctx, "listening on %s is not loopback; the profile endpoint is reachable beyond this host", ln.Addr().String())
	}
	srv := &http.Server{Handler: NewMux(), ReadHeaderTimeout: 5 * time.Second}
	actual := ln.Addr().String()
	serverMu.Lock()
	previous := server
	server = srv
	listenAddr = actual
	listenErr = nil
	serverMu.Unlock()
	if previous != nil {
		_ = shutdownServer(previous)
	}
	runtime.SetBlockProfileRate(blockProfileRate)
	runtime.SetMutexProfileFraction(mutexProfileFraction)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			logger.ErrorF(context.Background(), "local pprof server exited: %v", serveErr)
		}
	}()
	logger.InfoF(ctx, "[pprof] 本机剖析已监听 %s （go tool pprof http://%s/debug/pprof/heap） / local pprof listening on %s (go tool pprof http://%s/debug/pprof/heap)", ln.Addr().String(), ln.Addr().String(), ln.Addr().String(), ln.Addr().String())
	return nil
}

// Stop 关闭本机 pprof 并关掉 block/mutex 采样；未启动则为空操作。
// Stop shuts down local pprof and disables block/mutex sampling; a no-op when it was never started.
func Stop() {
	serverMu.Lock()
	current := server
	server = nil
	listenAddr = ""
	listenErr = nil
	serverMu.Unlock()
	if current != nil {
		_ = shutdownServer(current)
	}
	runtime.SetBlockProfileRate(0)
	runtime.SetMutexProfileFraction(0)
}

func shutdownServer(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
