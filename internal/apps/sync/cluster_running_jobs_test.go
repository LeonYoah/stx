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
	"testing"
)

// staticRuntimeResolver 返回固定的引擎地址。
// staticRuntimeResolver returns a fixed engine endpoint.
type staticRuntimeResolver struct{ endpoint *EngineEndpoint }

func (r staticRuntimeResolver) ResolveEngineEndpoint(context.Context, uint, JSONMap) (*EngineEndpoint, error) {
	return r.endpoint, nil
}

func TestListClusterRunningJobs(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	if items, err := service.ListClusterRunningJobs(ctx, 1); !errors.Is(err, ErrClusterEngineUnavailable) || items == nil {
		t.Fatalf("without engine client expected unavailable and [], got %v %v", items, err)
	}

	service.engineClient = &stubEngineClient{info: &EngineJobInfo{JobID: "900", JobName: "orders", JobStatus: "RUNNING"}}
	service.runtimeResolver = failRuntimeResolver{err: errors.New("no master")}
	if _, err := service.ListClusterRunningJobs(ctx, 1); !errors.Is(err, ErrClusterEngineUnavailable) {
		t.Fatalf("resolver failure should be ErrClusterEngineUnavailable, got %v", err)
	}

	service.runtimeResolver = staticRuntimeResolver{endpoint: &EngineEndpoint{BaseURL: "http://10.0.0.1:8080"}}
	items, err := service.ListClusterRunningJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ListClusterRunningJobs: %v", err)
	}
	if len(items) != 1 || items[0].JobID != "900" || items[0].JobName != "orders" {
		t.Fatalf("unexpected running jobs: %+v", items)
	}

	service.engineClient = &stubEngineClient{}
	items, err = service.ListClusterRunningJobs(ctx, 1)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("no running jobs should return [], got %v %v", items, err)
	}
}
