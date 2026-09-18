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

package execution

import (
	"context"
	"sync"
)

// CancelResult 描述业务模块实际接收取消请求后的状态。
// CancelResult describes the state confirmed by a business module after receiving a cancellation request.
type CancelResult struct {
	Status            Status
	Cancellable       bool
	CancellableReason string
}

// CancellationProvider 由业务模块实现，用于执行真实停止或确认安全取消点。
// CancellationProvider is implemented by a business module to perform a real stop or confirm a safe cancellation point.
type CancellationProvider interface {
	RequestCancel(ctx context.Context, execution *Execution, actor Actor) (CancelResult, error)
}

// ProviderRegistry 按模块保存取消实现。
// ProviderRegistry stores cancellation implementations by module.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]CancellationProvider
}

// NewProviderRegistry 创建空的业务取消实现登记表。
// NewProviderRegistry creates an empty business cancellation provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{providers: make(map[string]CancellationProvider)}
}

// Register 注册或替换一个模块的取消实现。
// Register registers or replaces the cancellation provider for a module.
func (r *ProviderRegistry) Register(module string, provider CancellationProvider) {
	if r == nil || module == "" || provider == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[module] = provider
}

// Get 返回模块对应的取消实现。
// Get returns the cancellation provider registered for a module.
func (r *ProviderRegistry) Get(module string) (CancellationProvider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[module]
	return provider, ok
}
