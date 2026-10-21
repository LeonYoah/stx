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
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/agent"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"go.uber.org/zap"
	grpclib "google.golang.org/grpc"
)

type fakeCommandStream struct {
	grpclib.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest]
}

func resetAgentReadyHooks(t *testing.T) {
	t.Helper()
	agentReadyHooksMu.Lock()
	saved := agentReadyHooks
	agentReadyHooks = nil
	agentReadyHooksMu.Unlock()
	savedAttempts, savedInterval := agentStreamWaitAttempts, agentStreamWaitInterval
	agentStreamWaitAttempts, agentStreamWaitInterval = 2, time.Millisecond
	t.Cleanup(func() {
		agentReadyHooksMu.Lock()
		agentReadyHooks = saved
		agentReadyHooksMu.Unlock()
		agentStreamWaitAttempts, agentStreamWaitInterval = savedAttempts, savedInterval
	})
}

func TestRunAgentReadyHooks_RunsAfterStreamReady(t *testing.T) {
	resetAgentReadyHooks(t)
	manager := agent.NewManager(nil)
	if _, err := manager.RegisterAgent(context.Background(), &pb.RegisterRequest{AgentId: "a1", IpAddress: "10.0.0.1"}); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if err := manager.SetAgentStream("a1", &fakeCommandStream{}); err != nil {
		t.Fatalf("SetAgentStream: %v", err)
	}
	var got []uint
	RegisterAgentReadyHook(func(_ context.Context, hostID uint) { got = append(got, hostID) })
	RegisterAgentReadyHook(func(_ context.Context, _ uint) { panic("boom") })
	RegisterAgentReadyHook(func(_ context.Context, hostID uint) { got = append(got, hostID+100) })

	server := &Server{agentManager: manager, logger: zap.NewNop()}
	server.runAgentReadyHooks(context.Background(), "a1", 7)

	if len(got) != 2 || got[0] != 7 || got[1] != 107 {
		t.Fatalf("expected both non-panicking hooks to run, got %v", got)
	}
}

func TestRunAgentReadyHooks_SkipsWhenStreamNeverReady(t *testing.T) {
	resetAgentReadyHooks(t)
	called := false
	RegisterAgentReadyHook(func(_ context.Context, _ uint) { called = true })
	server := &Server{agentManager: agent.NewManager(nil), logger: zap.NewNop()}
	server.runAgentReadyHooks(context.Background(), "missing", 7)
	if called {
		t.Fatal("hook must not run without a command stream")
	}
}
