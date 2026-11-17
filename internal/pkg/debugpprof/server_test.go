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

package debugpprof

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServeNamedHeap(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/pprof/heap", nil)
	rec := httptest.NewRecorder()
	ServeNamed(rec, req, "heap")
	if rec.Code != http.StatusOK {
		t.Fatalf("heap status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() < 16 {
		t.Fatalf("heap profile too small: %d bytes", rec.Body.Len())
	}
}

func TestServeNamedRejectsEmptyName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	ServeNamed(rec, req, "  ")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name status=%d", rec.Code)
	}
}

func TestStartLoopbackHeap(t *testing.T) {
	ctx := context.Background()
	if err := Start(ctx, Options{Enabled: true, Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("start pprof: %v", err)
	}
	t.Cleanup(Stop)
	addr := ListenAddr()
	if addr == "" {
		t.Fatal("expected listen address")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://" + addr + "/debug/pprof/heap"
	deadline := time.Now().Add(2 * time.Second)
	var resp *http.Response
	var err error
	// Serve 在 goroutine 里 Accept，首次 GET 可能打在监听刚就绪时；短轮询等到 200。
	// Serve Accepts in a goroutine, so the first GET may race the listener becoming ready; poll until 200.
	for {
		resp, err = client.Get(url)
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			break
		}
		status := 0
		if resp != nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("GET heap: %v", err)
			}
			t.Fatalf("heap status=%d", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read heap: %v", err)
	}
	if len(body) < 16 {
		t.Fatalf("heap profile too small: %d bytes", len(body))
	}
}

func TestStartDisabledIsNoop(t *testing.T) {
	if err := Start(context.Background(), Options{Enabled: false, Addr: "127.0.0.1:1"}); err != nil {
		t.Fatalf("disabled start: %v", err)
	}
}
