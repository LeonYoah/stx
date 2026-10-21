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

package grpc

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AgentReadyHook 在 Agent（重新）注册且命令流就绪后被调用，例如重推离线期间未下发的配置。
// AgentReadyHook is called after an Agent (re-)registers and its command stream is ready,
// e.g. to re-push configs that could not be delivered while it was offline.
type AgentReadyHook func(ctx context.Context, hostID uint)

var (
	agentReadyHooksMu sync.RWMutex
	agentReadyHooks   []AgentReadyHook
)

// RegisterAgentReadyHook 注册 Agent 就绪回调。
// RegisterAgentReadyHook registers an Agent-ready callback.
func RegisterAgentReadyHook(hook AgentReadyHook) {
	if hook == nil {
		return
	}
	agentReadyHooksMu.Lock()
	defer agentReadyHooksMu.Unlock()
	agentReadyHooks = append(agentReadyHooks, hook)
}

func snapshotAgentReadyHooks() []AgentReadyHook {
	agentReadyHooksMu.RLock()
	defer agentReadyHooksMu.RUnlock()
	return append([]AgentReadyHook(nil), agentReadyHooks...)
}

// agentStreamWaitAttempts / agentStreamWaitInterval 控制等待命令流就绪的重试；测试可调小。
// agentStreamWaitAttempts / agentStreamWaitInterval control the retries while waiting for the
// command stream; tests may shrink them.
var (
	agentStreamWaitAttempts = 10
	agentStreamWaitInterval = 3 * time.Second
)

// runAgentReadyHooks 等待 Agent 命令流就绪后依次执行回调；单个回调 panic 不影响其他回调。
// runAgentReadyHooks waits for the Agent command stream and then runs every hook;
// a panicking hook does not affect the others.
func (s *Server) runAgentReadyHooks(ctx context.Context, agentID string, hostID uint) {
	hooks := snapshotAgentReadyHooks()
	if len(hooks) == 0 || hostID == 0 {
		return
	}
	ready := false
	for i := 0; i < agentStreamWaitAttempts; i++ {
		if conn, ok := s.agentManager.GetAgent(agentID); ok && conn.Stream != nil {
			ready = true
			break
		}
		time.Sleep(agentStreamWaitInterval)
	}
	if !ready {
		s.logger.Warn("Agent command stream not ready, skipping agent-ready hooks / Agent 命令流未就绪，跳过就绪回调",
			zap.String("agent_id", agentID), zap.Uint("host_id", hostID))
		return
	}
	for _, hook := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Warn("agent-ready hook panic recovered / Agent 就绪回调 panic 已恢复",
						zap.String("agent_id", agentID), zap.Any("panic", r))
				}
			}()
			hookCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			hook(hookCtx, hostID)
		}()
	}
}
