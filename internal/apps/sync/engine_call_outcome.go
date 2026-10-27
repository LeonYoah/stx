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
	"io"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// ErrEngineCallOutcomeUnknown 表示写操作（提交/停止）可能已到达引擎但没有拿到回复。
// ErrEngineCallOutcomeUnknown means a write call (submit/stop) may have reached the engine but no reply was received.
var ErrEngineCallOutcomeUnknown = errors.New("sync: engine call outcome unknown")

// EngineCallUnknownError 记录结果未知时实际请求的端点，供对账按 jobId 查询。
// EngineCallUnknownError records the endpoint actually called so reconcile can look the job up by id.
type EngineCallUnknownError struct {
	Cause error
	// APIMode 是实际请求的接口族：v2 或 v1。
	// APIMode is the API family actually called: v2 or v1.
	APIMode string
	// BaseURL 是实际请求的基础地址：v2 为 endpoint.BaseURL，v1 为 endpoint.LegacyURL。
	// BaseURL is the base address actually called: endpoint.BaseURL for v2, endpoint.LegacyURL for v1.
	BaseURL   string
	EngineURL string
}

// Error 返回带原因的错误文本。
// Error returns the error text including the cause.
func (e *EngineCallUnknownError) Error() string {
	if e == nil || e.Cause == nil {
		return ErrEngineCallOutcomeUnknown.Error()
	}
	return ErrEngineCallOutcomeUnknown.Error() + ": " + e.Cause.Error()
}

// Unwrap 暴露底层原因，便于 errors.Is 判断超时等类型。
// Unwrap exposes the cause so errors.Is can still match timeouts and similar.
func (e *EngineCallUnknownError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is 让 errors.Is(err, ErrEngineCallOutcomeUnknown) 成立。
// Is makes errors.Is(err, ErrEngineCallOutcomeUnknown) hold.
func (e *EngineCallUnknownError) Is(target error) bool {
	return target == ErrEngineCallOutcomeUnknown
}

func newEngineCallUnknownError(cause error, apiMode, baseURL, engineURL string) *EngineCallUnknownError {
	return &EngineCallUnknownError{
		Cause:     cause,
		APIMode:   strings.TrimSpace(apiMode),
		BaseURL:   strings.TrimSpace(baseURL),
		EngineURL: strings.TrimSpace(engineURL),
	}
}

// engineTransportErrorIsAmbiguous 判断传输层错误是否意味着请求可能已经送达引擎。
// 拨号阶段失败（连接被拒、DNS、拨号超时）说明请求没发出去，属于确定失败；
// 已建立连接后的超时、EOF、连接重置说明请求可能已写出，引擎可能仍在处理。拿不准的按确定失败处理，保持原有行为。
// engineTransportErrorIsAmbiguous reports whether a transport error means the request may have reached the engine.
// Dial-phase failures (refused, DNS, dial timeout) mean nothing was sent, so they are definite failures; timeouts,
// EOF and resets after the connection is up mean the request may have been written and the engine may still be
// working. Anything unclassified is treated as a definite failure to keep the previous behavior.
func engineTransportErrorIsAmbiguous(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

// isEngineTransportError 判断引擎调用失败是否发生在传输层（超时、连不上、连接中断），用于对账止损。
// 引擎返回的非 2xx 状态、响应解析失败不算：它说明引擎还能回话，只是这个接口不可用。
// isEngineTransportError reports whether an engine call failed at the transport layer (timeout, unreachable,
// dropped connection); reconcile uses it to stop early. A non-2xx status or an unparsable body does not count:
// the engine still answers, only that endpoint is unusable.
func isEngineTransportError(err error) bool {
	if err == nil {
		return false
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}
