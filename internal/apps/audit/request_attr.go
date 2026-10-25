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

package audit

import (
	"context"
	"strings"
)

type requestAttributionContextKey struct{}

// RequestAttribution 保存 HTTP 请求的客户端 IP 与 User-Agent，供异步审计复用。
// RequestAttribution stores client IP and User-Agent so async audits can reuse them.
type RequestAttribution struct {
	ClientIP  string
	UserAgent string
}

// WithRequestAttribution 把客户端 IP / UA 写入上下文；空值不会清掉已有字段。
// WithRequestAttribution stores client IP / UA on the context without clearing existing fields with empty values.
func WithRequestAttribution(ctx context.Context, attr RequestAttribution) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	current := RequestAttributionFromContext(ctx)
	if ip := strings.TrimSpace(attr.ClientIP); ip != "" {
		current.ClientIP = ip
	}
	if ua := strings.TrimSpace(attr.UserAgent); ua != "" {
		if len(ua) > 500 {
			ua = ua[:500]
		}
		current.UserAgent = ua
	}
	return context.WithValue(ctx, requestAttributionContextKey{}, current)
}

// RequestAttributionFromContext 读取请求归因信息。
// RequestAttributionFromContext reads request attribution from a context.
func RequestAttributionFromContext(ctx context.Context) RequestAttribution {
	if ctx == nil {
		return RequestAttribution{}
	}
	attr, _ := ctx.Value(requestAttributionContextKey{}).(RequestAttribution)
	return attr
}
