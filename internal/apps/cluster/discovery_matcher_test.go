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
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/discovery"
)

// TestCreateClusterFromDiscoveryPersistsJavaHome 覆盖 AC1e：发现集群确认时把 java_home 写入 cluster_nodes。
// TestCreateClusterFromDiscoveryPersistsJavaHome covers AC1e: discover-confirm persists java_home on cluster_nodes.
func TestCreateClusterFromDiscoveryPersistsJavaHome(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	hosts.AddHost(&HostInfo{ID: 1, Name: "h1", IPAddress: "10.0.0.1", AgentID: "a1", AgentStatus: "installed", LastHeartbeat: &now})
	svc := NewService(repo, hosts, &ServiceConfig{})

	dc := &discovery.DiscoveredCluster{
		Name:       "discovered-st",
		InstallDir: "/opt/seatunnel",
		Version:    "2.3.8",
		JavaHome:   "/opt/jdk11/",
		Nodes: []*discovery.DiscoveredNode{
			{PID: 4242, Role: "hybrid", HazelcastPort: 5801, APIPort: 8080, JavaHome: "/proc/jdk"},
		},
	}
	clusterID, err := svc.CreateClusterFromDiscovery(context.Background(), 1, dc)
	if err != nil {
		t.Fatalf("CreateClusterFromDiscovery: %v", err)
	}
	if clusterID == 0 {
		t.Fatal("expected cluster id")
	}

	nodes, err := repo.GetNodesByHostID(context.Background(), 1)
	if err != nil || len(nodes) == 0 {
		t.Fatalf("nodes: %v len=%d", err, len(nodes))
	}
	if nodes[0].JavaHome != "/opt/jdk11" {
		t.Fatalf("java_home = %q, want /opt/jdk11 (confirm path wins over process)", nodes[0].JavaHome)
	}
	if nodes[0].InstallDir != "/opt/seatunnel" {
		t.Fatalf("install_dir = %q", nodes[0].InstallDir)
	}
}

// TestCreateClusterFromDiscoveryEmptyJavaHomeKeepsAuto 覆盖 AC1e：空路径保持自动探测，不落库非空 java_home。
// TestCreateClusterFromDiscoveryEmptyJavaHomeKeepsAuto covers AC1e: empty path keeps auto-detect semantics (empty java_home).
func TestCreateClusterFromDiscoveryEmptyJavaHomeKeepsAuto(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	hosts.AddHost(&HostInfo{ID: 2, Name: "h2", IPAddress: "10.0.0.2", AgentID: "a2", AgentStatus: "installed", LastHeartbeat: &now})
	svc := NewService(repo, hosts, &ServiceConfig{})

	dc := &discovery.DiscoveredCluster{
		Name:       "auto-st",
		InstallDir: "/data/seatunnel",
		Nodes: []*discovery.DiscoveredNode{
			{PID: 100, Role: "hybrid", HazelcastPort: 5801},
		},
		// JavaHome empty → auto
	}
	clusterID, err := svc.CreateClusterFromDiscovery(context.Background(), 2, dc)
	if err != nil {
		t.Fatalf("CreateClusterFromDiscovery: %v", err)
	}
	nodes, err := repo.GetNodesByClusterID(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("GetNodesByClusterID: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len(nodes)=%d", len(nodes))
	}
	if nodes[0].JavaHome != "" {
		t.Fatalf("empty confirm must keep auto (empty java_home), got %q", nodes[0].JavaHome)
	}
}

// TestCreateClusterFromDiscoveryUsesProcessJavaHomeWhenConfirmEmpty 覆盖 AC1e/AC1f：确认未指定时用进程 JDK。
// TestCreateClusterFromDiscoveryUsesProcessJavaHomeWhenConfirmEmpty covers AC1e/AC1f: process JDK when confirm omits path.
func TestCreateClusterFromDiscoveryUsesProcessJavaHomeWhenConfirmEmpty(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	hosts.AddHost(&HostInfo{ID: 3, Name: "h3", IPAddress: "10.0.0.3", AgentID: "a3", AgentStatus: "installed", LastHeartbeat: &now})
	svc := NewService(repo, hosts, &ServiceConfig{})

	dc := &discovery.DiscoveredCluster{
		Name:       "proc-st",
		InstallDir: "/opt/st-proc",
		Nodes: []*discovery.DiscoveredNode{
			{PID: 200, Role: "hybrid", HazelcastPort: 5801, JavaHome: "/usr/lib/jvm/java-11"},
		},
	}
	_, err := svc.CreateClusterFromDiscovery(context.Background(), 3, dc)
	if err != nil {
		t.Fatalf("CreateClusterFromDiscovery: %v", err)
	}
	nodes, err := repo.GetNodesByHostID(context.Background(), 3)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes: %v len=%d", err, len(nodes))
	}
	if nodes[0].JavaHome != "/usr/lib/jvm/java-11" {
		t.Fatalf("java_home = %q, want process JDK", nodes[0].JavaHome)
	}
}

// TestFindClusterByInstallDirMatchesExisting 覆盖发现匹配已有安装目录。
// TestFindClusterByInstallDirMatchesExisting covers matching an already-managed install dir.
func TestFindClusterByInstallDirMatchesExisting(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	hosts.AddHost(&HostInfo{ID: 4, Name: "h4", IPAddress: "10.0.0.4", AgentID: "a4", AgentStatus: "installed", LastHeartbeat: &now})
	svc := NewService(repo, hosts, &ServiceConfig{})

	created, err := svc.Create(context.Background(), &CreateClusterRequest{
		Name:           "existing",
		DeploymentMode: DeploymentModeHybrid,
		InstallDir:     "/opt/existing",
		Nodes: []CreateNodeFromDiscovery{{
			HostID: 4, InstallDir: "/opt/existing", Role: "hybrid", HazelcastPort: 5801, JavaHome: "/opt/jdk8",
		}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cid, nid, found, err := svc.FindClusterByInstallDir(context.Background(), 4, "/opt/existing")
	if err != nil || !found || cid != created.ID || nid == 0 {
		t.Fatalf("FindClusterByInstallDir: cid=%d nid=%d found=%v err=%v", cid, nid, found, err)
	}
	_, _, found, err = svc.FindClusterByInstallDir(context.Background(), 4, "/opt/other")
	if err != nil || found {
		t.Fatalf("expected not found, found=%v err=%v", found, err)
	}
}
