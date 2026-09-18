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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type mockAgentCommand struct {
	agentID     string
	commandType string
	params      map[string]string
}

type mockOperationAgentSender struct {
	commands []mockAgentCommand
}

func (m *mockOperationAgentSender) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	copied := make(map[string]string, len(params))
	for key, value := range params {
		copied[key] = value
	}
	m.commands = append(m.commands, mockAgentCommand{
		agentID:     agentID,
		commandType: commandType,
		params:      copied,
	})

	if commandType == "check_process" {
		return true, "SeaTunnel process found: PID=4321, role=hybrid", nil
	}

	return true, "ok", nil
}

type scriptedAgentSender struct {
	send func(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error)
}

func (s *scriptedAgentSender) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	if s != nil && s.send != nil {
		return s.send(ctx, agentID, commandType, params)
	}
	return true, "ok", nil
}

type mockConfigAgentClient struct {
	pullContent string
	pushed      string
	pullCalls   int
	pushCalls   int
}

func (m *mockConfigAgentClient) PullConfig(ctx context.Context, hostID uint, installDir string, configType appconfig.ConfigType) (string, error) {
	m.pullCalls++
	return m.pullContent, nil
}

func (m *mockConfigAgentClient) PushConfig(ctx context.Context, hostID uint, installDir string, configType appconfig.ConfigType, content string) error {
	m.pushCalls++
	m.pushed = content
	return nil
}

// MockHostProvider implements HostProvider for testing
// MockHostProvider 实现用于测试的 HostProvider 接口
type MockHostProvider struct {
	hosts map[uint]*HostInfo
}

// NewMockHostProvider creates a new MockHostProvider
// NewMockHostProvider 创建一个新的 MockHostProvider
func NewMockHostProvider() *MockHostProvider {
	return &MockHostProvider{
		hosts: make(map[uint]*HostInfo),
	}
}

// AddHost adds a host to the mock provider
// AddHost 向模拟提供者添加主机
func (m *MockHostProvider) AddHost(host *HostInfo) {
	m.hosts[host.ID] = host
}

// GetHostByID retrieves host information by ID
// GetHostByID 根据 ID 获取主机信息
func (m *MockHostProvider) GetHostByID(ctx context.Context, id uint) (*HostInfo, error) {
	host, ok := m.hosts[id]
	if !ok {
		return nil, fmt.Errorf("host not found: %d", id)
	}
	return host, nil
}

// setupServiceTestDB creates an in-memory SQLite database for service testing
// setupServiceTestDB 创建用于服务测试的内存 SQLite 数据库
func setupServiceTestDB(t *testing.T) (*gorm.DB, func()) {
	tempDir, err := os.MkdirTemp("", "cluster_service_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	// Auto-migrate models
	// 自动迁移模型
	if err := db.AutoMigrate(&Cluster{}, &ClusterNode{}); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to migrate: %v", err)
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.RemoveAll(tempDir)
	}

	return db, cleanup
}

func TestUpdateSeatunnelHTTPPort(t *testing.T) {
	content := "seatunnel:\n  engine:\n    backup-count: 1\n    http:\n      enable-http: true\n      port: 8080\n      enable-dynamic-port: true\n"
	updated, err := updateSeatunnelHTTPPort(content, 18081)
	if err != nil {
		t.Fatalf("updateSeatunnelHTTPPort returned error: %v", err)
	}
	if !strings.Contains(updated, "engine:\n        backup-count: 1\n        http:") {
		t.Fatalf("expected nested seatunnel.engine.http block, got: %s", updated)
	}
	if !strings.Contains(updated, "port: 18081") {
		t.Fatalf("expected updated port in content, got: %s", updated)
	}
	if !strings.Contains(updated, "enable-dynamic-port: false") {
		t.Fatalf("expected dynamic port disabled, got: %s", updated)
	}
}

func TestBuildNodeForCreateUsesClusterPortConfigDefaults(t *testing.T) {
	cluster := &Cluster{
		DeploymentMode: DeploymentModeHybrid,
		InstallDir:     "/opt/seatunnel",
		Config: ClusterConfig{
			"ports": map[string]interface{}{
				"master_hazelcast_port": 15801,
				"master_api_port":       18080,
				"worker_port":           15802,
			},
		},
	}

	node, err := buildNodeForCreate(1, 2, cluster, NodeRoleMasterWorker, "", 0, 0, 0, nil)
	if err != nil {
		t.Fatalf("buildNodeForCreate returned error: %v", err)
	}
	if node.HazelcastPort != 15801 {
		t.Fatalf("expected hazelcast port 15801, got %d", node.HazelcastPort)
	}
	if node.APIPort != 18080 {
		t.Fatalf("expected api port 18080 from cluster config, got %d", node.APIPort)
	}
	if node.WorkerPort != 15802 {
		t.Fatalf("expected worker port 15802, got %d", node.WorkerPort)
	}
}

func TestBuildNodeForCreateExplicitPortsOverrideClusterPortConfig(t *testing.T) {
	cluster := &Cluster{
		DeploymentMode: DeploymentModeHybrid,
		InstallDir:     "/opt/seatunnel",
		Config: ClusterConfig{
			"ports": map[string]interface{}{
				"master_hazelcast_port": 15801,
				"master_api_port":       18080,
				"worker_port":           15802,
			},
		},
	}

	node, err := buildNodeForCreate(1, 2, cluster, NodeRoleMasterWorker, "", 25801, 28080, 25802, nil)
	if err != nil {
		t.Fatalf("buildNodeForCreate returned error: %v", err)
	}
	if node.HazelcastPort != 25801 {
		t.Fatalf("expected explicit hazelcast port 25801, got %d", node.HazelcastPort)
	}
	if node.APIPort != 28080 {
		t.Fatalf("expected explicit api port 28080, got %d", node.APIPort)
	}
	if node.WorkerPort != 25802 {
		t.Fatalf("expected explicit worker port 25802, got %d", node.WorkerPort)
	}
}

func TestUpdateNodeSyncsSeatunnelHTTPPortAndRestartsNode(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hostProvider := NewMockHostProvider()
	now := time.Now()
	hostProvider.AddHost(&HostInfo{ID: 1, Name: "host-1", IPAddress: "127.0.0.1", AgentID: "agent-1", LastHeartbeat: &now})

	service := NewService(repo, hostProvider, &ServiceConfig{})
	configClient := &mockConfigAgentClient{
		pullContent: "seatunnel:\n  engine:\n    backup-count: 1\n    http:\n      enable-http: true\n      port: 8080\n      enable-dynamic-port: true\n",
	}
	service.SetConfigAgentClient(configClient)
	agentSender := &mockOperationAgentSender{}
	service.SetAgentCommandSender(agentSender)

	ctx := context.Background()
	cluster := &Cluster{Name: "demo", DeploymentMode: DeploymentModeHybrid, InstallDir: "/opt/seatunnel", Status: ClusterStatusRunning}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster failed: %v", err)
	}
	node := &ClusterNode{
		ClusterID:     cluster.ID,
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		APIPort:       8080,
		WorkerPort:    5802,
		Status:        NodeStatusRunning,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("create node failed: %v", err)
	}

	newPort := 18081
	updated, err := service.UpdateNode(ctx, cluster.ID, node.ID, &UpdateNodeRequest{APIPort: &newPort})
	if err != nil {
		t.Fatalf("UpdateNode returned error: %v", err)
	}
	if updated.APIPort != newPort {
		t.Fatalf("expected api port %d, got %d", newPort, updated.APIPort)
	}
	if configClient.pullCalls != 1 || configClient.pushCalls != 1 {
		t.Fatalf("expected pull/push once, got pull=%d push=%d", configClient.pullCalls, configClient.pushCalls)
	}
	if !strings.Contains(configClient.pushed, "port: 18081") {
		t.Fatalf("expected pushed config to contain new port, got: %s", configClient.pushed)
	}
	foundRestart := false
	for _, command := range agentSender.commands {
		if command.commandType == string(OperationRestart) {
			foundRestart = true
			break
		}
	}
	if !foundRestart {
		t.Fatalf("expected restart command after api port sync")
	}
}

// genValidHostID generates valid host IDs for tests
// genValidHostID 为测试生成有效的主机 ID
func genValidHostID() gopter.Gen {
	return gen.UIntRange(1, 10000).Map(func(v uint) uint {
		return v
	})
}

// genNodeRole generates valid node roles
// genNodeRole 生成有效的节点角色
func genNodeRole() gopter.Gen {
	return gen.OneConstOf(NodeRoleMaster, NodeRoleWorker)
}

// **Feature: stx-agent, Property 11: Node Association Validation**
// **Validates: Requirements 7.2**
// For any request to add a node to a cluster, if the host's Agent status is not "installed",
// the system SHALL reject the association and return an error.

func TestProperty_NodeAssociationValidation(t *testing.T) {
	// **Feature: stx-agent, Property 11: Node Association Validation**
	// **Validates: Requirements 7.2**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("node association is rejected when agent is not installed", prop.ForAll(
		func(clusterName string, hostID uint, role NodeRole) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			mockHostProvider := NewMockHostProvider()

			// Add a host with agent NOT installed
			// 添加一个未安装 Agent 的主机
			mockHostProvider.AddHost(&HostInfo{
				ID:          hostID,
				Name:        fmt.Sprintf("host-%d", hostID),
				HostType:    "bare_metal",
				IPAddress:   fmt.Sprintf("192.168.1.%d", hostID%255),
				AgentStatus: "not_installed", // Agent not installed
			})

			svc := NewService(repo, mockHostProvider, nil)
			ctx := context.Background()

			// Create a cluster first
			// 首先创建一个集群
			cluster, err := svc.Create(ctx, &CreateClusterRequest{
				Name:           clusterName,
				DeploymentMode: DeploymentModeHybrid,
			})
			if err != nil {
				t.Logf("Failed to create cluster: %v", err)
				return false
			}

			// Try to add node with agent not installed
			// 尝试添加未安装 Agent 的节点
			_, err = svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
				HostID: hostID,
				Role:   role,
			})

			// Should fail with ErrNodeAgentNotInstalled
			// 应该返回 ErrNodeAgentNotInstalled 错误
			if err != ErrNodeAgentNotInstalled {
				t.Logf("Expected ErrNodeAgentNotInstalled, got: %v", err)
				return false
			}

			return true
		},
		genValidClusterName(),
		genValidHostID(),
		genNodeRole(),
	))

	properties.Property("node association succeeds when agent is installed", prop.ForAll(
		func(clusterName string, hostID uint, role NodeRole) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			mockHostProvider := NewMockHostProvider()

			// Add a host with agent installed
			// 添加一个已安装 Agent 的主机
			now := time.Now()
			mockHostProvider.AddHost(&HostInfo{
				ID:            hostID,
				Name:          fmt.Sprintf("host-%d", hostID),
				HostType:      "bare_metal",
				IPAddress:     fmt.Sprintf("192.168.1.%d", hostID%255),
				AgentStatus:   "installed", // Agent installed
				LastHeartbeat: &now,
			})

			svc := NewService(repo, mockHostProvider, nil)
			ctx := context.Background()

			// Create a cluster first
			// 首先创建一个集群
			cluster, err := svc.Create(ctx, &CreateClusterRequest{
				Name:           clusterName,
				DeploymentMode: DeploymentModeHybrid,
			})
			if err != nil {
				t.Logf("Failed to create cluster: %v", err)
				return false
			}

			// Add node with agent installed
			// 添加已安装 Agent 的节点
			node, err := svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
				HostID: hostID,
				Role:   role,
			})
			if err != nil {
				t.Logf("Failed to add node: %v", err)
				return false
			}

			// Verify node was created
			// 验证节点已创建
			if node.ClusterID != cluster.ID {
				t.Logf("Node cluster ID mismatch: expected %d, got %d", cluster.ID, node.ClusterID)
				return false
			}

			if node.HostID != hostID {
				t.Logf("Node host ID mismatch: expected %d, got %d", hostID, node.HostID)
				return false
			}

			expectedRole := NodeRoleMasterWorker
			if node.Role != expectedRole {
				t.Logf("Node role mismatch: expected %s, got %s", expectedRole, node.Role)
				return false
			}

			return true
		},
		genValidClusterName(),
		genValidHostID(),
		genNodeRole(),
	))

	properties.Property("node association succeeds for docker hosts without agent check", prop.ForAll(
		func(clusterName string, hostID uint, role NodeRole) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			mockHostProvider := NewMockHostProvider()

			// Add a Docker host (no agent required)
			// 添加一个 Docker 主机（不需要 Agent）
			mockHostProvider.AddHost(&HostInfo{
				ID:          hostID,
				Name:        fmt.Sprintf("docker-host-%d", hostID),
				HostType:    "docker",
				AgentStatus: "", // No agent status for Docker hosts
			})

			svc := NewService(repo, mockHostProvider, nil)
			ctx := context.Background()

			// Create a cluster first
			// 首先创建一个集群
			cluster, err := svc.Create(ctx, &CreateClusterRequest{
				Name:           clusterName,
				DeploymentMode: DeploymentModeHybrid,
			})
			if err != nil {
				t.Logf("Failed to create cluster: %v", err)
				return false
			}

			// Add Docker host as node (should succeed without agent check)
			// 添加 Docker 主机作为节点（应该成功，无需 Agent 检查）
			node, err := svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
				HostID: hostID,
				Role:   role,
			})
			if err != nil {
				t.Logf("Failed to add Docker node: %v", err)
				return false
			}

			// Verify node was created
			// 验证节点已创建
			if node == nil {
				t.Logf("Node should not be nil")
				return false
			}

			return true
		},
		genValidClusterName(),
		genValidHostID(),
		genNodeRole(),
	))

	properties.TestingRun(t)
}

// **Feature: stx-agent, Property 12: Cluster Health Status Propagation**
// **Validates: Requirements 7.6**
// For any cluster with at least one node whose host status is "offline",
// the cluster's health status SHALL be marked as "unhealthy".

func TestProperty_ClusterHealthStatusPropagation(t *testing.T) {
	// **Feature: stx-agent, Property 12: Cluster Health Status Propagation**
	// **Validates: Requirements 7.6**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("cluster is unhealthy when any node is offline", prop.ForAll(
		func(clusterName string, onlineHostID uint, offlineHostID uint) bool {
			// Ensure different host IDs
			// 确保不同的主机 ID
			if onlineHostID == offlineHostID {
				offlineHostID = onlineHostID + 1
			}

			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			mockHostProvider := NewMockHostProvider()

			// Add an online host (recent heartbeat)
			// 添加一个在线主机（最近有心跳）
			recentTime := time.Now()
			mockHostProvider.AddHost(&HostInfo{
				ID:            onlineHostID,
				Name:          fmt.Sprintf("online-host-%d", onlineHostID),
				HostType:      "bare_metal",
				IPAddress:     fmt.Sprintf("192.168.1.%d", onlineHostID%255),
				AgentStatus:   "installed",
				LastHeartbeat: &recentTime,
			})

			// Add an offline host (old heartbeat)
			// 添加一个离线主机（旧心跳）
			oldTime := time.Now().Add(-60 * time.Second) // 60 seconds ago, exceeds 30s timeout
			mockHostProvider.AddHost(&HostInfo{
				ID:            offlineHostID,
				Name:          fmt.Sprintf("offline-host-%d", offlineHostID),
				HostType:      "bare_metal",
				IPAddress:     fmt.Sprintf("192.168.2.%d", offlineHostID%255),
				AgentStatus:   "installed",
				LastHeartbeat: &oldTime,
			})

			svc := NewService(repo, mockHostProvider, &ServiceConfig{
				HeartbeatTimeout: 30 * time.Second,
			})
			ctx := context.Background()

			// Create a cluster
			// 创建一个集群
			cluster, err := svc.Create(ctx, &CreateClusterRequest{
				Name:           clusterName,
				DeploymentMode: DeploymentModeHybrid,
			})
			if err != nil {
				t.Logf("Failed to create cluster: %v", err)
				return false
			}

			// Add online host as master node
			// 添加在线主机作为 master 节点
			_, err = svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
				HostID: onlineHostID,
				Role:   NodeRoleMaster,
			})
			if err != nil {
				t.Logf("Failed to add online node: %v", err)
				return false
			}

			// Add offline host as worker node
			// 添加离线主机作为 worker 节点
			_, err = svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
				HostID: offlineHostID,
				Role:   NodeRoleWorker,
			})
			if err != nil {
				t.Logf("Failed to add offline node: %v", err)
				return false
			}

			// Get cluster status
			// 获取集群状态
			status, err := svc.GetStatus(ctx, cluster.ID)
			if err != nil {
				t.Logf("Failed to get cluster status: %v", err)
				return false
			}

			// Verify cluster is unhealthy because one node is offline
			// 验证集群不健康，因为有一个节点离线
			if status.HealthStatus != HealthStatusUnhealthy {
				t.Logf("Cluster should be unhealthy when any node is offline, got: %s", status.HealthStatus)
				return false
			}

			// Verify node counts
			// 验证节点计数
			if status.TotalNodes != 2 {
				t.Logf("Expected 2 total nodes, got: %d", status.TotalNodes)
				return false
			}

			if status.OnlineNodes != 1 {
				t.Logf("Expected 1 online node, got: %d", status.OnlineNodes)
				return false
			}

			if status.OfflineNodes != 1 {
				t.Logf("Expected 1 offline node, got: %d", status.OfflineNodes)
				return false
			}

			return true
		},
		genValidClusterName(),
		genValidHostID(),
		genValidHostID(),
	))

	properties.Property("cluster is healthy when all nodes are online", prop.ForAll(
		func(clusterName string, hostID1 uint, hostID2 uint) bool {
			// Ensure different host IDs
			// 确保不同的主机 ID
			if hostID1 == hostID2 {
				hostID2 = hostID1 + 1
			}

			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			mockHostProvider := NewMockHostProvider()

			// Add two online hosts (recent heartbeats)
			// 添加两个在线主机（最近有心跳）
			recentTime := time.Now()
			mockHostProvider.AddHost(&HostInfo{
				ID:            hostID1,
				Name:          fmt.Sprintf("host-%d", hostID1),
				HostType:      "bare_metal",
				IPAddress:     fmt.Sprintf("192.168.1.%d", hostID1%255),
				AgentStatus:   "installed",
				LastHeartbeat: &recentTime,
			})

			mockHostProvider.AddHost(&HostInfo{
				ID:            hostID2,
				Name:          fmt.Sprintf("host-%d", hostID2),
				HostType:      "bare_metal",
				IPAddress:     fmt.Sprintf("192.168.2.%d", hostID2%255),
				AgentStatus:   "installed",
				LastHeartbeat: &recentTime,
			})

			svc := NewService(repo, mockHostProvider, &ServiceConfig{
				HeartbeatTimeout: 30 * time.Second,
			})
			ctx := context.Background()

			// Create a cluster
			// 创建一个集群
			cluster, err := svc.Create(ctx, &CreateClusterRequest{
				Name:           clusterName,
				DeploymentMode: DeploymentModeHybrid,
			})
			if err != nil {
				t.Logf("Failed to create cluster: %v", err)
				return false
			}

			// Add both hosts as nodes
			// 添加两个主机作为节点
			_, err = svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
				HostID: hostID1,
				Role:   NodeRoleMaster,
			})
			if err != nil {
				t.Logf("Failed to add node 1: %v", err)
				return false
			}

			_, err = svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
				HostID: hostID2,
				Role:   NodeRoleWorker,
			})
			if err != nil {
				t.Logf("Failed to add node 2: %v", err)
				return false
			}

			// Get cluster status
			// 获取集群状态
			status, err := svc.GetStatus(ctx, cluster.ID)
			if err != nil {
				t.Logf("Failed to get cluster status: %v", err)
				return false
			}

			// Verify cluster is healthy because all nodes are online
			// 验证集群健康，因为所有节点都在线
			if status.HealthStatus != HealthStatusHealthy {
				t.Logf("Cluster should be healthy when all nodes are online, got: %s", status.HealthStatus)
				return false
			}

			// Verify node counts
			// 验证节点计数
			if status.TotalNodes != 2 {
				t.Logf("Expected 2 total nodes, got: %d", status.TotalNodes)
				return false
			}

			if status.OnlineNodes != 2 {
				t.Logf("Expected 2 online nodes, got: %d", status.OnlineNodes)
				return false
			}

			if status.OfflineNodes != 0 {
				t.Logf("Expected 0 offline nodes, got: %d", status.OfflineNodes)
				return false
			}

			return true
		},
		genValidClusterName(),
		genValidHostID(),
		genValidHostID(),
	))

	properties.Property("cluster health is unknown when no nodes exist", prop.ForAll(
		func(clusterName string) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			mockHostProvider := NewMockHostProvider()

			svc := NewService(repo, mockHostProvider, nil)
			ctx := context.Background()

			// Create a cluster without nodes
			// 创建一个没有节点的集群
			cluster, err := svc.Create(ctx, &CreateClusterRequest{
				Name:           clusterName,
				DeploymentMode: DeploymentModeHybrid,
			})
			if err != nil {
				t.Logf("Failed to create cluster: %v", err)
				return false
			}

			// Get cluster status
			// 获取集群状态
			status, err := svc.GetStatus(ctx, cluster.ID)
			if err != nil {
				t.Logf("Failed to get cluster status: %v", err)
				return false
			}

			// Verify cluster health is unknown when no nodes
			// 验证没有节点时集群健康状态为未知
			if status.HealthStatus != HealthStatusUnknown {
				t.Logf("Cluster health should be unknown when no nodes, got: %s", status.HealthStatus)
				return false
			}

			if status.TotalNodes != 0 {
				t.Logf("Expected 0 total nodes, got: %d", status.TotalNodes)
				return false
			}

			return true
		},
		genValidClusterName(),
	))

	properties.TestingRun(t)
}

// TestClusterServiceBasicOperations tests basic CRUD operations
// TestClusterServiceBasicOperations 测试基本的 CRUD 操作
func TestClusterServiceBasicOperations(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	mockHostProvider := NewMockHostProvider()
	svc := NewService(repo, mockHostProvider, nil)
	ctx := context.Background()

	// Test Create
	// 测试创建
	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "test-cluster",
		Description:    "Test cluster description",
		DeploymentMode: DeploymentModeHybrid,
		Version:        "2.3.0",
	})
	if err != nil {
		t.Fatalf("Failed to create cluster: %v", err)
	}
	if cluster.Name != "test-cluster" {
		t.Errorf("Expected name 'test-cluster', got '%s'", cluster.Name)
	}
	if cluster.Status != ClusterStatusCreated {
		t.Errorf("Expected status 'created', got '%s'", cluster.Status)
	}

	// Test Get
	// 测试获取
	retrieved, err := svc.Get(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("Failed to get cluster: %v", err)
	}
	if retrieved.Name != cluster.Name {
		t.Errorf("Retrieved cluster name mismatch")
	}

	// Test Update
	// 测试更新
	newName := "updated-cluster"
	updated, err := svc.Update(ctx, cluster.ID, &UpdateClusterRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("Failed to update cluster: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("Expected updated name '%s', got '%s'", newName, updated.Name)
	}

	// Test List
	// 测试列表
	clusters, total, err := svc.List(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list clusters: %v", err)
	}
	if total != 1 {
		t.Errorf("Expected 1 cluster, got %d", total)
	}
	if len(clusters) != 1 {
		t.Errorf("Expected 1 cluster in list, got %d", len(clusters))
	}

	// Test Delete
	// 测试删除
	err = svc.Delete(ctx, cluster.ID, false)
	if err != nil {
		t.Fatalf("Failed to delete cluster: %v", err)
	}

	// Verify deletion
	// 验证删除
	_, err = svc.Get(ctx, cluster.ID)
	if err != ErrClusterNotFound {
		t.Errorf("Expected ErrClusterNotFound after deletion, got: %v", err)
	}
}

// TestClusterServiceDeleteConstraint tests that running clusters cannot be deleted
// TestClusterServiceDeleteConstraint 测试运行中的集群不能被删除
func TestClusterServiceDeleteConstraint(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	mockHostProvider := NewMockHostProvider()
	svc := NewService(repo, mockHostProvider, nil)
	ctx := context.Background()

	// Create a cluster
	// 创建一个集群
	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "running-cluster",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("Failed to create cluster: %v", err)
	}

	// Set cluster status to running
	// 设置集群状态为运行中
	err = svc.UpdateStatus(ctx, cluster.ID, ClusterStatusRunning)
	if err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	// Try to delete running cluster
	// 尝试删除运行中的集群
	err = svc.Delete(ctx, cluster.ID, false)
	if err != ErrClusterHasRunningTask {
		t.Errorf("Expected ErrClusterHasRunningTask, got: %v", err)
	}

	// Verify cluster still exists
	// 验证集群仍然存在
	_, err = svc.Get(ctx, cluster.ID)
	if err != nil {
		t.Errorf("Cluster should still exist after failed deletion")
	}
}

func TestClusterServiceStartUsesNodeInstallDirAndRefreshesProcess(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	mockHostProvider := NewMockHostProvider()
	now := time.Now()
	mockHostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "host-1",
		HostType:      "bare_metal",
		IPAddress:     "127.0.0.1",
		AgentID:       "agent-1",
		AgentStatus:   "installed",
		LastHeartbeat: &now,
	})

	svc := NewService(repo, mockHostProvider, nil)
	agentSender := &mockOperationAgentSender{}
	svc.SetAgentCommandSender(agentSender)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "upgrade-cluster",
		DeploymentMode: DeploymentModeHybrid,
		Version:        "2.3.11",
		InstallDir:     "/opt/seatunnel-2.3.11",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	node, err := svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel-2.3.12",
		HazelcastPort: 5801,
		APIPort:       8080,
		WorkerPort:    5802,
		SkipPrecheck:  true,
	})
	if err != nil {
		t.Fatalf("AddNode returned error: %v", err)
	}

	result, err := svc.Start(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful start result, got %+v", result)
	}

	foundStartCommand := false
	for _, command := range agentSender.commands {
		if command.commandType != string(OperationStart) {
			continue
		}
		foundStartCommand = true
		if got := command.params["install_dir"]; got != "/opt/seatunnel-2.3.12" {
			t.Fatalf("expected cluster start to use node install dir, got %q", got)
		}
	}
	if !foundStartCommand {
		t.Fatalf("expected start command to be sent, got %+v", agentSender.commands)
	}

	updatedNode, err := repo.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetNodeByID returned error: %v", err)
	}
	if updatedNode.ProcessPID != 4321 {
		t.Fatalf("expected process PID to be refreshed to 4321, got %d", updatedNode.ProcessPID)
	}
	if updatedNode.Status != NodeStatusRunning {
		t.Fatalf("expected node status running after PID refresh, got %q", updatedNode.Status)
	}
}

func intPtr(v int) *int {
	return &v
}

func TestService_AddNode_hybridNormalizesRoleToMasterWorker(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	mockHostProvider := NewMockHostProvider()
	now := time.Now()
	mockHostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "hybrid-host",
		HostType:      "bare_metal",
		IPAddress:     "127.0.0.1",
		AgentStatus:   "installed",
		LastHeartbeat: &now,
	})

	svc := NewService(repo, mockHostProvider, nil)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "hybrid-normalize",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	node, err := svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
		HostID: 1,
		Role:   NodeRoleMaster,
	})
	if err != nil {
		t.Fatalf("AddNode returned error: %v", err)
	}

	if node.Role != NodeRoleMasterWorker {
		t.Fatalf("expected hybrid node role to normalize to %q, got %q", NodeRoleMasterWorker, node.Role)
	}
	if node.HazelcastPort != DefaultPorts.MasterHazelcast {
		t.Fatalf("expected hybrid hazelcast port %d, got %d", DefaultPorts.MasterHazelcast, node.HazelcastPort)
	}
	if node.WorkerPort != DefaultPorts.WorkerHazelcast {
		t.Fatalf("expected hybrid worker port %d, got %d", DefaultPorts.WorkerHazelcast, node.WorkerPort)
	}
}

func TestService_GetStatus_refreshesStoppedProcessToStoppedCluster(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	mockHostProvider := NewMockHostProvider()
	now := time.Now()
	mockHostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "t13",
		HostType:      "bare_metal",
		IPAddress:     "127.0.0.1",
		AgentStatus:   "installed",
		AgentID:       "agent-t13",
		LastHeartbeat: &now,
	})

	agentSender := &scriptedAgentSender{
		send: func(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
			if commandType == "check_process" {
				return false, "SeaTunnel process not found", nil
			}
			return true, "ok", nil
		},
	}

	svc := NewService(repo, mockHostProvider, nil)
	svc.SetAgentCommandSender(agentSender)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "t13-cluster",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	node, err := svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		APIPort:       8080,
		WorkerPort:    5802,
		SkipPrecheck:  true,
	})
	if err != nil {
		t.Fatalf("AddNode returned error: %v", err)
	}

	if err := repo.UpdateNodeProcess(ctx, node.ID, 4321, "running"); err != nil {
		t.Fatalf("UpdateNodeProcess returned error: %v", err)
	}
	if err := repo.UpdateStatus(ctx, cluster.ID, ClusterStatusRunning); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	status, err := svc.GetStatus(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}

	if status.Status != ClusterStatusStopped {
		t.Fatalf("expected cluster status stopped after refresh, got %q", status.Status)
	}
	if len(status.Nodes) != 1 {
		t.Fatalf("expected 1 node status, got %d", len(status.Nodes))
	}
	if status.Nodes[0].Status != NodeStatusStopped {
		t.Fatalf("expected node status stopped after refresh, got %q", status.Nodes[0].Status)
	}

	updatedNode, err := repo.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetNodeByID returned error: %v", err)
	}
	if updatedNode.ProcessPID != 0 {
		t.Fatalf("expected process PID reset to 0, got %d", updatedNode.ProcessPID)
	}
	if updatedNode.Status != NodeStatusStopped {
		t.Fatalf("expected persisted node status stopped, got %q", updatedNode.Status)
	}
}

func TestService_ListWithInfo_usesRefreshedRuntimeStatus(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	mockHostProvider := NewMockHostProvider()
	now := time.Now()
	mockHostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "t13",
		HostType:      "bare_metal",
		IPAddress:     "127.0.0.1",
		AgentStatus:   "installed",
		AgentID:       "agent-t13",
		LastHeartbeat: &now,
	})

	agentSender := &scriptedAgentSender{
		send: func(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
			if commandType == "check_process" {
				return false, "SeaTunnel process not found", nil
			}
			return true, "ok", nil
		},
	}

	svc := NewService(repo, mockHostProvider, nil)
	svc.SetAgentCommandSender(agentSender)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "list-info-refresh",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	node, err := svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		APIPort:       8080,
		WorkerPort:    5802,
		SkipPrecheck:  true,
	})
	if err != nil {
		t.Fatalf("AddNode returned error: %v", err)
	}

	if err := repo.UpdateNodeProcess(ctx, node.ID, 4321, "running"); err != nil {
		t.Fatalf("UpdateNodeProcess returned error: %v", err)
	}
	if err := repo.UpdateStatus(ctx, cluster.ID, ClusterStatusRunning); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}

	clusters, total, err := svc.ListWithInfo(ctx, &ClusterFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListWithInfo returned error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster info, got %d", len(clusters))
	}
	if clusters[0].Status != ClusterStatusStopped {
		t.Fatalf("expected refreshed cluster status stopped, got %q", clusters[0].Status)
	}
}

func TestService_AddNodes_sameHostSeparatedCreatesMasterAndWorkerAtomically(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	mockHostProvider := NewMockHostProvider()
	now := time.Now()
	mockHostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "separated-host",
		HostType:      "bare_metal",
		IPAddress:     "127.0.0.2",
		AgentStatus:   "installed",
		LastHeartbeat: &now,
	})

	svc := NewService(repo, mockHostProvider, nil)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "separated-batch",
		DeploymentMode: DeploymentModeSeparated,
		Config: ClusterConfig{
			"jvm": map[string]interface{}{
				"master_heap_size": 2,
				"worker_heap_size": 4,
			},
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	nodes, err := svc.AddNodes(ctx, cluster.ID, &AddNodesRequest{
		HostID:     1,
		InstallDir: "/opt/seatunnel",
		Entries: []AddNodeEntryRequest{
			{
				Role:          NodeRoleMaster,
				HazelcastPort: 5801,
				APIPort:       8080,
				Overrides: &NodeOverrides{
					JVM: &NodeJVMOverrides{MasterHeapSize: intPtr(6)},
				},
			},
			{
				Role:          NodeRoleWorker,
				HazelcastPort: 5802,
				Overrides: &NodeOverrides{
					JVM: &NodeJVMOverrides{WorkerHeapSize: intPtr(8)},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("AddNodes returned error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 created nodes, got %d", len(nodes))
	}

	storedNodes, err := repo.GetNodesByClusterID(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("GetNodesByClusterID returned error: %v", err)
	}
	if len(storedNodes) != 2 {
		t.Fatalf("expected 2 stored nodes, got %d", len(storedNodes))
	}

	roleMap := make(map[NodeRole]*ClusterNode, len(storedNodes))
	for _, node := range storedNodes {
		roleMap[node.Role] = node
	}
	if roleMap[NodeRoleMaster] == nil || roleMap[NodeRoleWorker] == nil {
		t.Fatalf("expected both master and worker nodes, got %+v", roleMap)
	}
	if roleMap[NodeRoleMaster].Overrides.JVM == nil || roleMap[NodeRoleMaster].Overrides.JVM.MasterHeapSize == nil || *roleMap[NodeRoleMaster].Overrides.JVM.MasterHeapSize != 6 {
		t.Fatalf("expected master JVM override 6GB, got %+v", roleMap[NodeRoleMaster].Overrides)
	}
	if roleMap[NodeRoleWorker].Overrides.JVM == nil || roleMap[NodeRoleWorker].Overrides.JVM.WorkerHeapSize == nil || *roleMap[NodeRoleWorker].Overrides.JVM.WorkerHeapSize != 8 {
		t.Fatalf("expected worker JVM override 8GB, got %+v", roleMap[NodeRoleWorker].Overrides)
	}
}

func TestClusterNode_ResolveJVM_usesNodeOverrideOverClusterDefault(t *testing.T) {
	clusterConfig := ClusterConfig{
		"jvm": map[string]interface{}{
			"hybrid_heap_size": 3,
			"master_heap_size": 2,
			"worker_heap_size": 4,
		},
	}
	node := &ClusterNode{
		Role: NodeRoleWorker,
		Overrides: NodeOverrides{
			JVM: &NodeJVMOverrides{
				WorkerHeapSize: intPtr(10),
			},
		},
	}

	resolved := node.ResolveJVM(clusterConfig)
	if resolved == nil {
		t.Fatalf("expected resolved JVM config")
	}
	if resolved.MasterHeapSize != 2 {
		t.Fatalf("expected master heap to inherit 2GB, got %d", resolved.MasterHeapSize)
	}
	if resolved.WorkerHeapSize != 10 {
		t.Fatalf("expected worker heap override 10GB, got %d", resolved.WorkerHeapSize)
	}
}
