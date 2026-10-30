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

package discovery

import (
	"context"
	"testing"
)

type fakeHostProvider struct {
	host *HostInfo
}

func (f *fakeHostProvider) GetHostByID(_ context.Context, id uint) (*HostInfo, error) {
	if f.host == nil || f.host.ID != id {
		return nil, ErrHostNotFound
	}
	return f.host, nil
}

func (f *fakeHostProvider) GetHostByAgentID(_ context.Context, agentID string) (*HostInfo, error) {
	if f.host == nil || f.host.AgentID != agentID {
		return nil, ErrHostNotFound
	}
	return f.host, nil
}

type fakeAgentDiscoverer struct {
	clusters []*DiscoveredCluster
}

func (f *fakeAgentDiscoverer) TriggerDiscovery(_ context.Context, _ string) ([]*DiscoveredCluster, error) {
	return f.clusters, nil
}

func (f *fakeAgentDiscoverer) DiscoverProcesses(_ context.Context, _ string) ([]*DiscoveredProcess, error) {
	return nil, nil
}

type recordingMatcher struct {
	created []*DiscoveredCluster
}

func (m *recordingMatcher) FindClusterByInstallDir(_ context.Context, _ uint, _ string) (uint, uint, bool, error) {
	return 0, 0, false, nil
}

func (m *recordingMatcher) CreateClusterFromDiscovery(_ context.Context, _ uint, dc *DiscoveredCluster) (uint, error) {
	m.created = append(m.created, dc)
	return uint(len(m.created)), nil
}

func (m *recordingMatcher) UpdateNodeFromDiscovery(_ context.Context, _ uint, _ *DiscoveredNode) error {
	return nil
}

// TestConfirmDiscoveryAppliesJavaHomes 覆盖 AC1e：确认请求的 java_homes 进入 CreateClusterFromDiscovery。
// TestConfirmDiscoveryAppliesJavaHomes covers AC1e: confirm java_homes reaches CreateClusterFromDiscovery.
func TestConfirmDiscoveryAppliesJavaHomes(t *testing.T) {
	matcher := &recordingMatcher{}
	svc := NewService()
	svc.SetHostProvider(&fakeHostProvider{host: &HostInfo{ID: 9, Name: "h9", AgentID: "ag9", AgentStatus: "online"}})
	svc.SetAgentDiscoverer(&fakeAgentDiscoverer{clusters: []*DiscoveredCluster{{
		Name: "c1", InstallDir: "/opt/st", IsNew: true,
		Nodes: []*DiscoveredNode{{PID: 1, Role: "hybrid", JavaHome: "/proc/jdk"}},
	}}})
	svc.SetClusterMatcher(matcher)

	ids, err := svc.ConfirmDiscovery(context.Background(), &ConfirmDiscoveryRequest{
		HostID:      9,
		InstallDirs: []string{"/opt/st"},
		JavaHomes:   map[string]string{"/opt/st": "/opt/custom-jdk"},
	})
	if err != nil {
		t.Fatalf("ConfirmDiscovery: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("created ids = %v", ids)
	}
	if len(matcher.created) != 1 {
		t.Fatalf("matcher calls = %d", len(matcher.created))
	}
	got := matcher.created[0]
	if got.JavaHome != "/opt/custom-jdk" {
		t.Fatalf("cluster.JavaHome = %q", got.JavaHome)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].JavaHome != "/opt/custom-jdk" {
		t.Fatalf("node.JavaHome = %#v", got.Nodes)
	}
}

// TestConfirmDiscoveryEmptyJavaHomeLeavesAuto 覆盖 AC1e：未传 java_homes 时保持空/进程值（自动探测语义由 matcher 落库）。
// TestConfirmDiscoveryEmptyJavaHomeLeavesAuto covers AC1e: omitted java_homes leaves empty/process value for matcher.
func TestConfirmDiscoveryEmptyJavaHomeLeavesAuto(t *testing.T) {
	matcher := &recordingMatcher{}
	svc := NewService()
	svc.SetHostProvider(&fakeHostProvider{host: &HostInfo{ID: 10, Name: "h10", AgentID: "ag10", AgentStatus: "online"}})
	svc.SetAgentDiscoverer(&fakeAgentDiscoverer{clusters: []*DiscoveredCluster{{
		Name: "c2", InstallDir: "/opt/auto", IsNew: true,
		Nodes: []*DiscoveredNode{{PID: 2, Role: "hybrid"}},
	}}})
	svc.SetClusterMatcher(matcher)

	_, err := svc.ConfirmDiscovery(context.Background(), &ConfirmDiscoveryRequest{
		HostID:      10,
		InstallDirs: []string{"/opt/auto"},
	})
	if err != nil {
		t.Fatalf("ConfirmDiscovery: %v", err)
	}
	if matcher.created[0].JavaHome != "" {
		t.Fatalf("expected empty cluster JavaHome for auto, got %q", matcher.created[0].JavaHome)
	}
}

// TestBuildClustersFromProcessesGroupsByInstallDir 覆盖进程聚合为发现集群且带上进程 JDK。
// TestBuildClustersFromProcessesGroupsByInstallDir covers grouping processes into clusters with process JDK.
func TestBuildClustersFromProcessesGroupsByInstallDir(t *testing.T) {
	clusters := BuildClustersFromProcesses([]*DiscoveredProcess{
		{PID: 1, Role: "hybrid", InstallDir: "/opt/a", Version: "2.3.8", JavaHome: "/jdk/a"},
		{PID: 2, Role: "master", InstallDir: "/opt/b", HazelcastPort: 1},
		{PID: 3, Role: "worker", InstallDir: "/opt/b", HazelcastPort: 2},
		{PID: 4, Role: "hybrid", InstallDir: ""}, // skipped
	})
	if len(clusters) != 2 {
		t.Fatalf("len=%d", len(clusters))
	}
	if clusters[0].InstallDir != "/opt/a" || clusters[0].Nodes[0].JavaHome != "/jdk/a" || clusters[0].DeploymentMode != "hybrid" {
		t.Fatalf("cluster0 = %+v nodes=%+v", clusters[0], clusters[0].Nodes)
	}
	if clusters[1].InstallDir != "/opt/b" || len(clusters[1].Nodes) != 2 || clusters[1].DeploymentMode != "separated" {
		t.Fatalf("cluster1 = %+v", clusters[1])
	}
}
