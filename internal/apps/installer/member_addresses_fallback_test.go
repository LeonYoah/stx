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

package installer

import (
	"context"
	"testing"
)

type fakeNodeAddressesResolver struct {
	nodes []ClusterNodeAddress
	err   error
}

func (f *fakeNodeAddressesResolver) ResolveClusterNodeAddresses(ctx context.Context, clusterID uint) ([]ClusterNodeAddress, error) {
	return f.nodes, f.err
}

type fakeHostProvider struct {
	host *HostInfo
	err  error
}

func (f *fakeHostProvider) GetHostByID(ctx context.Context, hostID uint) (*HostInfo, error) {
	return f.host, f.err
}

// 调用方未传地址时，应合并「目标主机 IP + 已登记节点 IP」并按角色归入 master/worker 列表。
// When the caller omits addresses, the fallback must merge the target host IP with
// registered node IPs and bucket them into master/worker lists by role.
func TestResolveMemberAddresses_fillsFromClusterNodes(t *testing.T) {
	s := &Service{
		hostProvider: &fakeHostProvider{host: &HostInfo{ID: 9, IPAddress: "10.0.0.9"}},
		nodeAddressesResolver: &fakeNodeAddressesResolver{nodes: []ClusterNodeAddress{
			{HostIP: "10.0.0.2", Role: "master/worker"},
			{HostIP: "10.0.0.3", Role: "master/worker"},
			{HostIP: "", Role: "master/worker"},
		}},
	}
	req := &InstallationRequest{
		HostID:         "9",
		ClusterID:      "5",
		DeploymentMode: DeploymentModeHybrid,
		NodeRole:       NodeRoleMasterWorker,
	}

	s.resolveMemberAddresses(context.Background(), req)

	want := []string{"10.0.0.2", "10.0.0.3", "10.0.0.9"}
	if len(req.MasterAddresses) != len(want) {
		t.Fatalf("masters = %v, want %v", req.MasterAddresses, want)
	}
	seen := map[string]bool{}
	for _, a := range req.MasterAddresses {
		seen[a] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("missing %s in %v", w, req.MasterAddresses)
		}
	}
}

func TestResolveMemberAddresses_separatedSplitsRoles(t *testing.T) {
	s := &Service{
		hostProvider: &fakeHostProvider{host: &HostInfo{ID: 9, IPAddress: "10.0.0.9"}},
		nodeAddressesResolver: &fakeNodeAddressesResolver{nodes: []ClusterNodeAddress{
			{HostIP: "10.0.0.2", Role: "master"},
			{HostIP: "10.0.0.3", Role: "worker"},
			{HostIP: "10.0.0.9", Role: "worker"},
		}},
	}
	req := &InstallationRequest{
		HostID:         "9",
		ClusterID:      "5",
		DeploymentMode: DeploymentModeSeparated,
		NodeRole:       NodeRoleWorker,
	}

	s.resolveMemberAddresses(context.Background(), req)

	if len(req.MasterAddresses) != 1 || req.MasterAddresses[0] != "10.0.0.2" {
		t.Fatalf("masters = %v, want [10.0.0.2]", req.MasterAddresses)
	}
	if len(req.WorkerAddresses) != 2 {
		t.Fatalf("workers = %v, want [10.0.0.3 10.0.0.9]", req.WorkerAddresses)
	}
}

func TestResolveMemberAddresses_keepsCallerProvided(t *testing.T) {
	s := &Service{
		hostProvider:          &fakeHostProvider{host: &HostInfo{ID: 9, IPAddress: "10.0.0.9"}},
		nodeAddressesResolver: &fakeNodeAddressesResolver{nodes: []ClusterNodeAddress{{HostIP: "10.0.0.2", Role: "master/worker"}}},
	}
	req := &InstallationRequest{
		HostID:          "9",
		ClusterID:       "5",
		DeploymentMode:  DeploymentModeHybrid,
		NodeRole:        NodeRoleMasterWorker,
		MasterAddresses: []string{"10.9.9.9"},
		WorkerAddresses: []string{"10.9.9.8"},
	}

	s.resolveMemberAddresses(context.Background(), req)

	if len(req.MasterAddresses) != 1 || req.MasterAddresses[0] != "10.9.9.9" {
		t.Fatalf("masters overwritten: %v", req.MasterAddresses)
	}
}
