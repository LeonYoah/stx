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

package cluster

import (
	"context"
	"sync/atomic"
	"testing"
)

// 回归：单节点重启必须在阻塞的 RestartNode 之前发出 OperationRestart 钩子，
// 否则实机库中会缺 node_restart_requested（#186 AC1）。
// Regression: single-node restart must fire the OperationRestart hook before the
// blocking RestartNode body, or the DB misses node_restart_requested (#186 AC1).
func TestNotifyOperationExecutedInvokesHook(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := &Handler{}
	h.SetOnOperationExecuted(func(ctx context.Context, event *OperationEvent) error {
		if event == nil {
			t.Fatalf("event is nil")
		}
		if event.Operation != OperationRestart {
			t.Fatalf("operation=%s, want restart", event.Operation)
		}
		if event.NodeID != 4 {
			t.Fatalf("node_id=%d, want 4", event.NodeID)
		}
		calls.Add(1)
		return nil
	})

	h.notifyOperationExecuted(context.Background(), &OperationEvent{
		ClusterID: 2,
		NodeID:    4,
		Operation: OperationRestart,
		Success:   true,
		Trigger:   "manual_api",
	})

	if calls.Load() != 1 {
		t.Fatalf("hook calls=%d, want 1", calls.Load())
	}
}

func TestBuildNodeOperationEventCarriesNodeScope(t *testing.T) {
	t.Parallel()

	h := &Handler{}
	event := h.buildNodeOperationEvent(
		context.Background(),
		2,
		4,
		OperationRestart,
		&OperationResult{Success: true, Message: "restart accepted"},
		"admin",
	)
	// service 为空时仍应带上 cluster/node/operation，供 router 映射 node_restart_requested。
	// Even without a service, cluster/node/operation must be set for router → node_restart_requested.
	if event == nil {
		t.Fatal("event is nil")
	}
	if event.ClusterID != 2 || event.NodeID != 4 || event.Operation != OperationRestart {
		t.Fatalf("event=%+v", event)
	}
	if event.Operator != "admin" || event.Trigger != "manual_api" {
		t.Fatalf("operator/trigger=%s/%s", event.Operator, event.Trigger)
	}
}
