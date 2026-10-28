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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type fakeTimeoutError struct{}

func (fakeTimeoutError) Error() string   { return "i/o timeout" }
func (fakeTimeoutError) Timeout() bool   { return true }
func (fakeTimeoutError) Temporary() bool { return true }

func TestEngineTransportErrorIsAmbiguous(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
		{name: "dns", err: &net.DNSError{Err: "no such host", Name: "engine", IsTimeout: true}, want: false},
		{name: "dial timeout", err: &net.OpError{Op: "dial", Net: "tcp", Err: fakeTimeoutError{}}, want: false},
		{name: "connection refused", err: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, want: false},
		{name: "deadline exceeded", err: fmt.Errorf("wrap: %w", context.DeadlineExceeded), want: true},
		{name: "canceled", err: context.Canceled, want: true},
		{name: "eof", err: &neturl.Error{Op: "Post", URL: "http://x", Err: io.EOF}, want: true},
		{name: "unexpected eof", err: io.ErrUnexpectedEOF, want: true},
		{name: "reset after connect", err: &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, want: true},
		{name: "broken pipe", err: &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.EPIPE)}, want: true},
		{name: "client timeout", err: &neturl.Error{Op: "Post", URL: "http://x", Err: fakeTimeoutError{}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := engineTransportErrorIsAmbiguous(tc.err); got != tc.want {
				t.Fatalf("engineTransportErrorIsAmbiguous(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// newShortTimeoutEngineClient 构造超时很短的客户端，让测试快速触发超时。
// newShortTimeoutEngineClient builds a client with a tiny timeout so tests hit timeouts quickly.
func newShortTimeoutEngineClient() *SeaTunnelEngineClient {
	return &SeaTunnelEngineClient{httpClient: &http.Client{Timeout: 50 * time.Millisecond}}
}

// hangingHandler 读完请求体后一直等到客户端断开，模拟引擎迟迟不回复。
// 必须先读完请求体，服务端才会检测连接断开；另加 2 秒兜底，避免 server.Close 卡住。
// hangingHandler drains the body and waits until the client gives up, simulating an engine that never replies.
// The body must be drained before the server notices the disconnect; a 2-second fallback keeps server.Close from hanging.
func hangingHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	select {
	case <-r.Context().Done():
	case <-time.After(2 * time.Second):
	}
}

// closedURL 返回一个当前没有进程监听的本地地址，用于制造连接被拒绝。
// closedURL returns a local address with no listener so connections are refused.
func closedURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return "http://" + addr
}

func legacySubmitServer(t *testing.T, calls *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/hazelcast/rest/maps/submit-job", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		_, _ = w.Write([]byte(`{"jobId":"901","jobName":"legacy"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func submitRequest(endpoint *EngineEndpoint) *EngineSubmitRequest {
	return &EngineSubmitRequest{Endpoint: endpoint, Format: "json", JobID: "901", JobName: "demo", Body: []byte(`{"env":{}}`)}
}

func TestSubmit_TimeoutIsUnknownAndSkipsLegacy(t *testing.T) {
	var legacyCalls int32
	legacy := legacySubmitServer(t, &legacyCalls)
	v2 := httptest.NewServer(http.HandlerFunc(hangingHandler))
	t.Cleanup(v2.Close)

	_, err := newShortTimeoutEngineClient().Submit(context.Background(), submitRequest(&EngineEndpoint{BaseURL: v2.URL, LegacyURL: legacy.URL}))
	if !errors.Is(err, ErrEngineCallOutcomeUnknown) {
		t.Fatalf("expected unknown outcome, got %v", err)
	}
	var unknown *EngineCallUnknownError
	if !errors.As(err, &unknown) || unknown.APIMode != "v2" || unknown.BaseURL != v2.URL {
		t.Fatalf("expected v2 unknown error carrying the base url, got %#v", unknown)
	}
	if got := atomic.LoadInt32(&legacyCalls); got != 0 {
		t.Fatalf("legacy submit must not be called after an ambiguous v2 failure, got %d calls", got)
	}
}

func TestSubmit_ConnectionRefusedStillFallsBackToLegacy(t *testing.T) {
	var legacyCalls int32
	legacy := legacySubmitServer(t, &legacyCalls)

	resp, err := newShortTimeoutEngineClient().Submit(context.Background(), submitRequest(&EngineEndpoint{BaseURL: closedURL(t), LegacyURL: legacy.URL}))
	if err != nil {
		t.Fatalf("expected legacy fallback to succeed, got %v", err)
	}
	if resp.APIMode != "v1" || atomic.LoadInt32(&legacyCalls) != 1 {
		t.Fatalf("expected one legacy submit, got mode=%s calls=%d", resp.APIMode, legacyCalls)
	}
}

func TestSubmit_GatewayTimeoutIsUnknown(t *testing.T) {
	var legacyCalls int32
	legacy := legacySubmitServer(t, &legacyCalls)
	v2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream timed out", http.StatusGatewayTimeout)
	}))
	t.Cleanup(v2.Close)

	_, err := newShortTimeoutEngineClient().Submit(context.Background(), submitRequest(&EngineEndpoint{BaseURL: v2.URL, LegacyURL: legacy.URL}))
	if !errors.Is(err, ErrEngineCallOutcomeUnknown) {
		t.Fatalf("expected unknown outcome for 504, got %v", err)
	}
	if atomic.LoadInt32(&legacyCalls) != 0 {
		t.Fatalf("504 must not fall back to legacy")
	}
}

func TestSubmit_ServiceUnavailableStillFallsBackToLegacy(t *testing.T) {
	var legacyCalls int32
	legacy := legacySubmitServer(t, &legacyCalls)
	v2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(v2.Close)

	resp, err := newShortTimeoutEngineClient().Submit(context.Background(), submitRequest(&EngineEndpoint{BaseURL: v2.URL, LegacyURL: legacy.URL}))
	if err != nil || resp.APIMode != "v1" {
		t.Fatalf("expected 503 to fall back to legacy, got resp=%v err=%v", resp, err)
	}
}

func TestSubmit_DefiniteRejectionIsNotUnknown(t *testing.T) {
	v2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "plugin not found", http.StatusInternalServerError)
	}))
	t.Cleanup(v2.Close)

	_, err := newShortTimeoutEngineClient().Submit(context.Background(), submitRequest(&EngineEndpoint{BaseURL: v2.URL}))
	if err == nil || errors.Is(err, ErrEngineCallOutcomeUnknown) {
		t.Fatalf("expected a definite failure for 500, got %v", err)
	}
}

func TestSubmit_AcceptedNonJSONBodyUsesRequestedJobID(t *testing.T) {
	v2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("accepted"))
	}))
	t.Cleanup(v2.Close)

	resp, err := newShortTimeoutEngineClient().Submit(context.Background(), submitRequest(&EngineEndpoint{BaseURL: v2.URL}))
	if err != nil {
		t.Fatalf("expected success for a 2xx non-JSON body, got %v", err)
	}
	if resp.JobID != "901" || resp.APIMode != "v2" {
		t.Fatalf("expected requested job id and v2, got %#v", resp)
	}
}

func TestSubmitV1_TimeoutIsUnknownWithLegacyBase(t *testing.T) {
	legacy := httptest.NewServer(http.HandlerFunc(hangingHandler))
	t.Cleanup(legacy.Close)

	_, err := newShortTimeoutEngineClient().Submit(context.Background(), submitRequest(&EngineEndpoint{BaseURL: legacy.URL, LegacyURL: legacy.URL, APIMode: "v1"}))
	var unknown *EngineCallUnknownError
	if !errors.As(err, &unknown) || unknown.APIMode != "v1" || unknown.BaseURL != legacy.URL {
		t.Fatalf("expected v1 unknown error with legacy base, got %v", err)
	}
}

func TestStopJob_TimeoutIsUnknownAndRefusedIsDefinite(t *testing.T) {
	v2 := httptest.NewServer(http.HandlerFunc(hangingHandler))
	t.Cleanup(v2.Close)
	client := newShortTimeoutEngineClient()

	err := client.StopJob(context.Background(), &EngineEndpoint{BaseURL: v2.URL}, "901", false)
	if !errors.Is(err, ErrEngineCallOutcomeUnknown) {
		t.Fatalf("expected unknown outcome for stop timeout, got %v", err)
	}

	err = client.StopJob(context.Background(), &EngineEndpoint{BaseURL: closedURL(t)}, "901", false)
	if err == nil || errors.Is(err, ErrEngineCallOutcomeUnknown) {
		t.Fatalf("expected a definite failure when the connection is refused, got %v", err)
	}

	legacy := httptest.NewServer(http.HandlerFunc(hangingHandler))
	t.Cleanup(legacy.Close)
	err = client.StopJob(context.Background(), &EngineEndpoint{BaseURL: legacy.URL, LegacyURL: legacy.URL, APIMode: "v1"}, "901", true)
	var unknown *EngineCallUnknownError
	if !errors.As(err, &unknown) || unknown.APIMode != "v1" {
		t.Fatalf("expected v1 unknown stop error, got %v", err)
	}
}
