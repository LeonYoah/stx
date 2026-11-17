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

package router

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/debugpprof"
)

func TestDebugPprofHandlerDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := config.Config.App.Pprof.Enabled
	config.Config.App.Pprof.Enabled = false
	t.Cleanup(func() { config.Config.App.Pprof.Enabled = original })

	engine := gin.New()
	engine.GET("/api/v1/debug/pprof/:name", debugPprofHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/pprof/heap", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled pprof status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDebugPprofHandlerHeapWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := config.Config.App.Pprof.Enabled
	config.Config.App.Pprof.Enabled = true
	t.Cleanup(func() { config.Config.App.Pprof.Enabled = original })

	engine := gin.New()
	engine.GET("/api/v1/debug/pprof/:name", debugPprofHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/pprof/heap", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enabled heap status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() < 16 {
		t.Fatalf("heap profile too small: %d", rec.Body.Len())
	}
}

func TestDebugPprofHandlerListenFailureReturnsReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := config.Config.App.Pprof.Enabled
	config.Config.App.Pprof.Enabled = true
	t.Cleanup(func() {
		config.Config.App.Pprof.Enabled = original
		debugpprof.Stop()
	})

	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold port: %v", err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	if err := debugpprof.Start(context.Background(), debugpprof.Options{Enabled: true, Addr: holder.Addr().String()}); err == nil {
		t.Fatal("expected pprof listen failure")
	}

	engine := gin.New()
	engine.GET("/api/v1/debug/pprof/:name", debugPprofHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/pprof/heap", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("busy pprof port status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		ErrorMsg string `json:"error_msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if !strings.Contains(body.ErrorMsg, "listen pprof") {
		t.Fatalf("expected listen failure reason, got %q", body.ErrorMsg)
	}
}
