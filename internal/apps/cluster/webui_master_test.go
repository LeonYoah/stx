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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// engineNode 启动一个模拟 SeaTunnel 节点：isMaster=true 时实时接口返回 200，否则返回 404 "Not a master node"。
// engineNode starts a fake SeaTunnel node: the realtime API answers 200 when isMaster is true and 404 otherwise.
func engineNode(t *testing.T, id uint, isMaster *atomic.Bool, hits *atomic.Int32) *NodeInfo {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == webUIMasterProbePath && isMaster.Load() {
			_, _ = w.Write([]byte(`{"jobs":[]}`))
			return
		}
		http.Error(w, "Not a master node", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	port, _ := strconv.Atoi(parsed.Port())
	return &NodeInfo{ID: id, HostIP: parsed.Hostname(), APIPort: port, Role: NodeRoleMasterWorker, IsOnline: true}
}

func TestWebUIMasterSelector_picksActiveMasterAndCaches(t *testing.T) {
	var firstMaster, secondMaster atomic.Bool
	var firstHits, secondHits atomic.Int32
	secondMaster.Store(true)
	first := engineNode(t, 1, &firstMaster, &firstHits)
	second := engineNode(t, 2, &secondMaster, &secondHits)

	now := time.Unix(1000, 0)
	selector := &webUIMasterSelector{now: func() time.Time { return now }}
	ctx := context.Background()

	if got := selector.pick(ctx, 7, []*NodeInfo{first, second}); got.ID != 2 {
		t.Fatalf("expected active master node 2, got %d", got.ID)
	}
	probes := firstHits.Load() + secondHits.Load()
	if got := selector.pick(ctx, 7, []*NodeInfo{first, second}); got.ID != 2 {
		t.Fatalf("expected cached node 2, got %d", got.ID)
	}
	if firstHits.Load()+secondHits.Load() != probes {
		t.Fatalf("cached pick must not probe again")
	}

	// 切主后缓存过期，重新探测到新 master。
	// After failover and cache expiry the new master is probed.
	firstMaster.Store(true)
	secondMaster.Store(false)
	now = now.Add(webUIMasterCacheTTL + time.Second)
	if got := selector.pick(ctx, 7, []*NodeInfo{first, second}); got.ID != 1 {
		t.Fatalf("expected new master node 1, got %d", got.ID)
	}
}

func TestWebUIMasterSelector_fallbacks(t *testing.T) {
	var never atomic.Bool
	var hits atomic.Int32
	first := engineNode(t, 1, &never, &hits)
	second := engineNode(t, 2, &never, &hits)
	selector := &webUIMasterSelector{}
	ctx := context.Background()

	// 2.3.x 没有实时接口：全部探测失败时退回第一个在线节点（与旧行为一致）。
	// 2.3.x lacks the realtime API: when every probe fails fall back to the first online node (previous behavior).
	if got := selector.pick(ctx, 1, []*NodeInfo{first, second}); got.ID != 1 {
		t.Fatalf("expected fallback to first online node, got %d", got.ID)
	}

	single := *first
	hits.Store(0)
	if got := (&webUIMasterSelector{}).pick(ctx, 2, []*NodeInfo{&single}); got.ID != 1 || hits.Load() != 0 {
		t.Fatalf("single online master must be returned without probing, got %d hits=%d", got.ID, hits.Load())
	}

	offline := &NodeInfo{ID: 5, HostIP: "127.0.0.1", APIPort: 1, Role: NodeRoleMaster}
	worker := &NodeInfo{ID: 6, HostIP: "127.0.0.1", APIPort: 2, Role: NodeRoleWorker, IsOnline: true}
	if got := (&webUIMasterSelector{}).pick(ctx, 3, []*NodeInfo{worker, offline}); got == nil || got.ID != 5 {
		t.Fatalf("expected offline master fallback when no master is online, got %+v", got)
	}
	if got := (&webUIMasterSelector{}).pick(ctx, 4, []*NodeInfo{worker}); got != nil {
		t.Fatalf("worker-only clusters have no Web UI target, got %+v", got)
	}
}
