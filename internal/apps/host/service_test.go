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

package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupServiceTestDB creates an in-memory SQLite database for service testing
// setupServiceTestDB 创建用于服务测试的内存 SQLite 数据库
func setupServiceTestDB(t *testing.T) (*gorm.DB, func()) {
	tempDir, err := os.MkdirTemp("", "host_service_test_*")
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
	if err := db.AutoMigrate(&Host{}, &cluster.Cluster{}, &cluster.ClusterNode{}); err != nil {
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

// genServiceValidHostName generates valid host names for service tests
// genServiceValidHostName 为服务测试生成有效的主机名
func genServiceValidHostName() gopter.Gen {
	return gen.RegexMatch("[a-zA-Z][a-zA-Z0-9_-]{0,99}").SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 100
	})
}

// genServiceValidIPv4 generates valid IPv4 addresses for service tests
// genServiceValidIPv4 为服务测试生成有效的 IPv4 地址
func genServiceValidIPv4() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 255),
		gen.IntRange(0, 255),
		gen.IntRange(0, 255),
		gen.IntRange(1, 254),
	).Map(func(vals []interface{}) string {
		return fmt.Sprintf("%d.%d.%d.%d",
			vals[0].(int), vals[1].(int), vals[2].(int), vals[3].(int))
	})
}

// genAgentID generates valid agent IDs
// genAgentID 生成有效的 Agent ID
func genAgentID() gopter.Gen {
	return gen.RegexMatch("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}")
}

// genAgentVersion generates valid agent versions
// genAgentVersion 生成有效的 Agent 版本
func genAgentVersion() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 10),
		gen.IntRange(0, 99),
		gen.IntRange(0, 99),
	).Map(func(vals []interface{}) string {
		return fmt.Sprintf("%d.%d.%d", vals[0].(int), vals[1].(int), vals[2].(int))
	})
}

// genResourceUsage generates valid resource usage percentages (0-100)
// genResourceUsage 生成有效的资源使用率百分比 (0-100)
func genResourceUsage() gopter.Gen {
	return gen.Float64Range(0.0, 100.0)
}

// **Feature: stx-agent, Property 3: Agent Registration IP Matching**
// **Validates: Requirements 3.2**
// For any Agent registration request, if the Agent's reported IP address matches
// a registered host's IP address, the system SHALL update that host's Agent status
// to "installed" and associate the Agent ID.

func TestProperty_AgentRegistrationIPMatching(t *testing.T) {
	// **Feature: stx-agent, Property 3: Agent Registration IP Matching**
	// **Validates: Requirements 3.2**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("agent registration updates host status when IP matches", prop.ForAll(
		func(hostName string, ipAddress string, agentID string, version string) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			clusterRepo := cluster.NewRepository(db)
			svc := NewService(repo, clusterRepo, nil)
			ctx := context.Background()

			// Create a host first
			// 首先创建一个主机
			host, err := svc.Create(ctx, &CreateHostRequest{
				Name:      hostName,
				IPAddress: ipAddress,
			})
			if err != nil {
				t.Logf("Failed to create host: %v", err)
				return false
			}

			// Verify initial status is not_installed
			// 验证初始状态为 not_installed
			if host.AgentStatus != AgentStatusNotInstalled {
				t.Logf("Initial status should be not_installed, got: %s", host.AgentStatus)
				return false
			}

			// Register agent with matching IP
			// 使用匹配的 IP 注册 Agent
			updatedHost, err := svc.UpdateAgentStatus(ctx, host.ID, ipAddress, agentID, version, nil, "")
			if err != nil {
				t.Logf("Failed to update agent status: %v", err)
				return false
			}

			// Verify status is now installed
			// 验证状态现在为 installed
			if updatedHost.AgentStatus != AgentStatusInstalled {
				t.Logf("Status should be installed after registration, got: %s", updatedHost.AgentStatus)
				return false
			}

			// Verify agent ID is associated
			// 验证 Agent ID 已关联
			if updatedHost.AgentID != agentID {
				t.Logf("Agent ID should be %s, got: %s", agentID, updatedHost.AgentID)
				return false
			}

			// Verify version is stored
			// 验证版本已存储
			if updatedHost.AgentVersion != version {
				t.Logf("Agent version should be %s, got: %s", version, updatedHost.AgentVersion)
				return false
			}

			return true
		},
		genServiceValidHostName(),
		genServiceValidIPv4(),
		genAgentID(),
		genAgentVersion(),
	))

	properties.Property("agent registration auto-creates host when IP does not match any host", prop.ForAll(
		func(hostName string, hostIP string, agentIP string, agentID string, version string) bool {
			// Skip if IPs are the same
			// 如果 IP 相同则跳过
			if hostIP == agentIP {
				return true
			}

			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			clusterRepo := cluster.NewRepository(db)
			svc := NewService(repo, clusterRepo, nil)
			ctx := context.Background()

			// Create a host with one IP
			// 创建一个具有特定 IP 的主机
			_, err := svc.Create(ctx, &CreateHostRequest{
				Name:      hostName,
				IPAddress: hostIP,
			})
			if err != nil {
				t.Logf("Failed to create host: %v", err)
				return false
			}

			// Register agent with different IP; service should auto-create host
			// 使用不同的 IP 注册 Agent；服务应自动创建主机
			updatedHost, err := svc.UpdateAgentStatus(ctx, 0, agentIP, agentID, version, nil, "")
			if err != nil {
				t.Logf("Failed to auto-create host during agent registration: %v", err)
				return false
			}

			if updatedHost.IPAddress != agentIP {
				t.Logf("Auto-created host IP mismatch: expected %s, got %s", agentIP, updatedHost.IPAddress)
				return false
			}
			if updatedHost.AgentStatus != AgentStatusInstalled {
				t.Logf("Auto-created host status should be installed, got: %s", updatedHost.AgentStatus)
				return false
			}
			if updatedHost.AgentID != agentID {
				t.Logf("Auto-created host agent ID should be %s, got: %s", agentID, updatedHost.AgentID)
				return false
			}
			if updatedHost.AgentVersion != version {
				t.Logf("Auto-created host version should be %s, got: %s", version, updatedHost.AgentVersion)
				return false
			}

			hostByIP, err := svc.GetByIP(ctx, agentIP)
			if err != nil {
				t.Logf("Failed to get auto-created host by IP: %v", err)
				return false
			}
			if hostByIP.ID != updatedHost.ID {
				t.Logf("Auto-created host ID mismatch: expected %d, got %d", updatedHost.ID, hostByIP.ID)
				return false
			}

			return true
		},
		genServiceValidHostName(),
		genServiceValidIPv4(),
		genServiceValidIPv4(),
		genAgentID(),
		genAgentVersion(),
	))

	properties.TestingRun(t)
}

// **Feature: stx-agent, Property 4: Heartbeat Data Persistence**
// **Validates: Requirements 3.3**
// For any heartbeat message received from an Agent, the system SHALL update
// the corresponding host's last_heartbeat timestamp and resource usage metrics
// (CPU, memory, disk) to match the heartbeat data.

func TestProperty_HeartbeatDataPersistence(t *testing.T) {
	// **Feature: stx-agent, Property 4: Heartbeat Data Persistence**
	// **Validates: Requirements 3.3**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("heartbeat updates host resource usage and timestamp", prop.ForAll(
		func(hostName string, ipAddress string, agentID string, cpuUsage float64, memoryUsage float64, diskUsage float64) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			clusterRepo := cluster.NewRepository(db)
			svc := NewService(repo, clusterRepo, nil)
			ctx := context.Background()

			// Create and register a host
			// 创建并注册一个主机
			host, err := svc.Create(ctx, &CreateHostRequest{
				Name:      hostName,
				IPAddress: ipAddress,
			})
			if err != nil {
				t.Logf("Failed to create host: %v", err)
				return false
			}

			// Register agent
			// 注册 Agent
			_, err = svc.UpdateAgentStatus(ctx, host.ID, ipAddress, agentID, "1.0.0", nil, "")
			if err != nil {
				t.Logf("Failed to register agent: %v", err)
				return false
			}

			// Record time before heartbeat
			// 记录心跳前的时间
			beforeHeartbeat := time.Now().Add(-time.Second)

			// Send heartbeat
			// 发送心跳
			err = svc.UpdateHeartbeat(ctx, agentID, cpuUsage, memoryUsage, diskUsage)
			if err != nil {
				t.Logf("Failed to update heartbeat: %v", err)
				return false
			}

			// Verify heartbeat data was persisted
			// 验证心跳数据已持久化
			updatedHost, err := svc.Get(ctx, host.ID)
			if err != nil {
				t.Logf("Failed to get updated host: %v", err)
				return false
			}

			// Check CPU usage matches (with small tolerance for float comparison)
			// 检查 CPU 使用率匹配（浮点比较允许小误差）
			if !floatEquals(updatedHost.CPUUsage, cpuUsage, 0.01) {
				t.Logf("CPU usage mismatch: expected %f, got %f", cpuUsage, updatedHost.CPUUsage)
				return false
			}

			// Check memory usage matches
			// 检查内存使用率匹配
			if !floatEquals(updatedHost.MemoryUsage, memoryUsage, 0.01) {
				t.Logf("Memory usage mismatch: expected %f, got %f", memoryUsage, updatedHost.MemoryUsage)
				return false
			}

			// Check disk usage matches
			// 检查磁盘使用率匹配
			if !floatEquals(updatedHost.DiskUsage, diskUsage, 0.01) {
				t.Logf("Disk usage mismatch: expected %f, got %f", diskUsage, updatedHost.DiskUsage)
				return false
			}

			// Check last heartbeat was updated
			// 检查最后心跳时间已更新
			if updatedHost.LastHeartbeat == nil {
				t.Logf("Last heartbeat should not be nil")
				return false
			}

			if updatedHost.LastHeartbeat.Before(beforeHeartbeat) {
				t.Logf("Last heartbeat should be after the heartbeat was sent")
				return false
			}

			return true
		},
		genServiceValidHostName(),
		genServiceValidIPv4(),
		genAgentID(),
		genResourceUsage(),
		genResourceUsage(),
		genResourceUsage(),
	))

	properties.TestingRun(t)
}

// floatEquals compares two floats with a tolerance
// floatEquals 使用容差比较两个浮点数
func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// **Feature: stx-agent, Property 5: Host Offline Detection**
// **Validates: Requirements 3.4**
// For any host that has not received a heartbeat for more than 30 seconds,
// the system SHALL mark the host's status as "offline".

// 执行太久了 暂时屏蔽
// func TestProperty_HostOfflineDetection(t *testing.T) {
// 	// **Feature: stx-agent, Property 5: Host Offline Detection**
// 	// **Validates: Requirements 3.4**

// 	parameters := gopter.DefaultTestParameters()
// 	parameters.MinSuccessfulTests = 100
// 	parameters.Rng.Seed(42)

// 	properties := gopter.NewProperties(parameters)

// 	properties.Property("host is marked offline when heartbeat timeout exceeded", prop.ForAll(
// 		func(hostName string, ipAddress string, agentID string) bool {
// 			db, cleanup := setupServiceTestDB(t)
// 			defer cleanup()

// 			repo := NewRepository(db)
// 			clusterRepo := cluster.NewRepository(db)
// 			// Use a very short timeout for testing (1 second)
// 			// 使用非常短的超时时间进行测试（1秒）
// 			svc := NewService(repo, clusterRepo, &ServiceConfig{
// 				HeartbeatTimeout: 1 * time.Second,
// 			})
// 			ctx := context.Background()

// 			// Create and register a host
// 			// 创建并注册一个主机
// 			host, err := svc.Create(ctx, &CreateHostRequest{
// 				Name:      hostName,
// 				IPAddress: ipAddress,
// 			})
// 			if err != nil {
// 				t.Logf("Failed to create host: %v", err)
// 				return false
// 			}

// 			// Register agent
// 			// 注册 Agent
// 			_, err = svc.UpdateAgentStatus(ctx, ipAddress, agentID, "1.0.0", nil)
// 			if err != nil {
// 				t.Logf("Failed to register agent: %v", err)
// 				return false
// 			}

// 			// Send initial heartbeat
// 			// 发送初始心跳
// 			err = svc.UpdateHeartbeat(ctx, agentID, 50.0, 50.0, 50.0)
// 			if err != nil {
// 				t.Logf("Failed to send heartbeat: %v", err)
// 				return false
// 			}

// 			// Verify host is online
// 			// 验证主机在线
// 			isOnline, err := svc.IsHostOnline(ctx, host.ID)
// 			if err != nil {
// 				t.Logf("Failed to check online status: %v", err)
// 				return false
// 			}
// 			if !isOnline {
// 				t.Logf("Host should be online after heartbeat")
// 				return false
// 			}

// 			// Wait for timeout to expire
// 			// 等待超时过期
// 			time.Sleep(2 * time.Second)

// 			// Check and mark offline
// 			// 检查并标记离线
// 			markedOffline, err := svc.CheckAndMarkOffline(ctx, host.ID)
// 			if err != nil {
// 				t.Logf("Failed to check and mark offline: %v", err)
// 				return false
// 			}

// 			if !markedOffline {
// 				t.Logf("Host should have been marked offline")
// 				return false
// 			}

// 			// Verify host status is now offline
// 			// 验证主机状态现在为离线
// 			updatedHost, err := svc.Get(ctx, host.ID)
// 			if err != nil {
// 				t.Logf("Failed to get updated host: %v", err)
// 				return false
// 			}

// 			if updatedHost.AgentStatus != AgentStatusOffline {
// 				t.Logf("Host status should be offline, got: %s", updatedHost.AgentStatus)
// 				return false
// 			}

// 			return true
// 		},
// 		genServiceValidHostName(),
// 		genServiceValidIPv4(),
// 		genAgentID(),
// 	))

// 	properties.Property("host remains online when heartbeat is within timeout", prop.ForAll(
// 		func(hostName string, ipAddress string, agentID string) bool {
// 			db, cleanup := setupServiceTestDB(t)
// 			defer cleanup()

// 			repo := NewRepository(db)
// 			clusterRepo := cluster.NewRepository(db)
// 			// Use a longer timeout (10 seconds)
// 			// 使用较长的超时时间（10秒）
// 			svc := NewService(repo, clusterRepo, &ServiceConfig{
// 				HeartbeatTimeout: 10 * time.Second,
// 			})
// 			ctx := context.Background()

// 			// Create and register a host
// 			// 创建并注册一个主机
// 			host, err := svc.Create(ctx, &CreateHostRequest{
// 				Name:      hostName,
// 				IPAddress: ipAddress,
// 			})
// 			if err != nil {
// 				t.Logf("Failed to create host: %v", err)
// 				return false
// 			}

// 			// Register agent
// 			// 注册 Agent
// 			_, err = svc.UpdateAgentStatus(ctx, ipAddress, agentID, "1.0.0", nil)
// 			if err != nil {
// 				t.Logf("Failed to register agent: %v", err)
// 				return false
// 			}

// 			// Send heartbeat
// 			// 发送心跳
// 			err = svc.UpdateHeartbeat(ctx, agentID, 50.0, 50.0, 50.0)
// 			if err != nil {
// 				t.Logf("Failed to send heartbeat: %v", err)
// 				return false
// 			}

// 			// Check immediately (should still be online)
// 			// 立即检查（应该仍然在线）
// 			markedOffline, err := svc.CheckAndMarkOffline(ctx, host.ID)
// 			if err != nil {
// 				t.Logf("Failed to check and mark offline: %v", err)
// 				return false
// 			}

// 			// Should NOT be marked offline
// 			// 不应该被标记为离线
// 			if markedOffline {
// 				t.Logf("Host should not be marked offline when heartbeat is recent")
// 				return false
// 			}

// 			// Verify host is still online
// 			// 验证主机仍然在线
// 			updatedHost, err := svc.Get(ctx, host.ID)
// 			if err != nil {
// 				t.Logf("Failed to get updated host: %v", err)
// 				return false
// 			}

// 			if updatedHost.AgentStatus != AgentStatusInstalled {
// 				t.Logf("Host status should still be installed, got: %s", updatedHost.AgentStatus)
// 				return false
// 			}

// 			return true
// 		},
// 		genServiceValidHostName(),
// 		genServiceValidIPv4(),
// 		genAgentID(),
// 	))

// 	properties.TestingRun(t)
// }

// **Feature: stx-agent, Property 6: Host Deletion Constraint**
// **Validates: Requirements 3.6**
// For any host deletion request, if the host is associated with any cluster,
// the system SHALL reject the deletion and return the list of associated clusters.

func TestProperty_HostDeletionConstraint(t *testing.T) {
	// **Feature: stx-agent, Property 6: Host Deletion Constraint**
	// **Validates: Requirements 3.6**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("host deletion is rejected when associated with cluster", prop.ForAll(
		func(hostName string, ipAddress string, clusterName string) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			clusterRepo := cluster.NewRepository(db)
			svc := NewService(repo, clusterRepo, nil)
			ctx := context.Background()

			// Create a host
			// 创建一个主机
			host, err := svc.Create(ctx, &CreateHostRequest{
				Name:      hostName,
				IPAddress: ipAddress,
			})
			if err != nil {
				t.Logf("Failed to create host: %v", err)
				return false
			}

			// Create a cluster
			// 创建一个集群
			clusterObj := &cluster.Cluster{
				Name:           clusterName,
				DeploymentMode: cluster.DeploymentModeHybrid,
			}
			err = clusterRepo.Create(ctx, clusterObj)
			if err != nil {
				t.Logf("Failed to create cluster: %v", err)
				return false
			}

			// Add host as a node to the cluster
			// 将主机作为节点添加到集群
			node := &cluster.ClusterNode{
				ClusterID: clusterObj.ID,
				HostID:    host.ID,
				Role:      cluster.NodeRoleMaster,
			}
			err = clusterRepo.AddNode(ctx, node)
			if err != nil {
				t.Logf("Failed to add node: %v", err)
				return false
			}

			// Try to delete the host
			// 尝试删除主机
			err = svc.Delete(ctx, host.ID)

			// Should fail with ErrHostHasCluster
			// 应该返回 ErrHostHasCluster 错误
			if err != ErrHostHasCluster {
				t.Logf("Expected ErrHostHasCluster, got: %v", err)
				return false
			}

			// Verify host still exists
			// 验证主机仍然存在
			_, err = svc.Get(ctx, host.ID)
			if err != nil {
				t.Logf("Host should still exist after failed deletion")
				return false
			}

			return true
		},
		genServiceValidHostName(),
		genServiceValidIPv4(),
		gen.RegexMatch("[a-zA-Z][a-zA-Z0-9_-]{0,99}").SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) <= 100
		}),
	))

	properties.Property("host deletion succeeds when not associated with any cluster", prop.ForAll(
		func(hostName string, ipAddress string) bool {
			db, cleanup := setupServiceTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			clusterRepo := cluster.NewRepository(db)
			svc := NewService(repo, clusterRepo, nil)
			ctx := context.Background()

			// Create a host
			// 创建一个主机
			host, err := svc.Create(ctx, &CreateHostRequest{
				Name:      hostName,
				IPAddress: ipAddress,
			})
			if err != nil {
				t.Logf("Failed to create host: %v", err)
				return false
			}

			// Delete the host (should succeed)
			// 删除主机（应该成功）
			err = svc.Delete(ctx, host.ID)
			if err != nil {
				t.Logf("Failed to delete host: %v", err)
				return false
			}

			// Verify host no longer exists
			// 验证主机不再存在
			_, err = svc.Get(ctx, host.ID)
			if err != ErrHostNotFound {
				t.Logf("Host should not exist after deletion")
				return false
			}

			return true
		},
		genServiceValidHostName(),
		genServiceValidIPv4(),
	))

	properties.TestingRun(t)
}

// TestGetAssociatedClusters tests the GetAssociatedClusters method
// TestGetAssociatedClusters 测试 GetAssociatedClusters 方法
func TestGetAssociatedClusters(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	clusterRepo := cluster.NewRepository(db)
	svc := NewService(repo, clusterRepo, nil)
	ctx := context.Background()

	// Create a host
	// 创建一个主机
	host, err := svc.Create(ctx, &CreateHostRequest{
		Name:      "test-host",
		IPAddress: "192.168.1.1",
	})
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}

	// Initially no clusters
	// 初始时没有集群
	clusters, err := svc.GetAssociatedClusters(ctx, host.ID)
	if err != nil {
		t.Fatalf("Failed to get associated clusters: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("Expected 0 clusters, got %d", len(clusters))
	}

	// Create and associate a cluster
	// 创建并关联一个集群
	clusterObj := &cluster.Cluster{
		Name:           "test-cluster",
		DeploymentMode: cluster.DeploymentModeHybrid,
	}
	err = clusterRepo.Create(ctx, clusterObj)
	if err != nil {
		t.Fatalf("Failed to create cluster: %v", err)
	}

	node := &cluster.ClusterNode{
		ClusterID: clusterObj.ID,
		HostID:    host.ID,
		Role:      cluster.NodeRoleMaster,
	}
	err = clusterRepo.AddNode(ctx, node)
	if err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	// Now should have one cluster
	// 现在应该有一个集群
	clusters, err = svc.GetAssociatedClusters(ctx, host.ID)
	if err != nil {
		t.Fatalf("Failed to get associated clusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Errorf("Expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Name != "test-cluster" {
		t.Errorf("Expected cluster name 'test-cluster', got '%s'", clusters[0].Name)
	}
}

// TestGetInstallCommand tests the GetInstallCommand method
// TestGetInstallCommand 测试 GetInstallCommand 方法
func TestGetInstallCommand(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	clusterRepo := cluster.NewRepository(db)
	svc := NewService(repo, clusterRepo, &ServiceConfig{
		ControlPlaneAddr: "control-plane.example.com:8000",
	})
	ctx := context.Background()

	// Create a host
	// 创建一个主机
	host, err := svc.Create(ctx, &CreateHostRequest{
		Name:      "test-host",
		IPAddress: "192.168.1.1",
	})
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}

	// Get install command
	// 获取安装命令
	cmd, err := svc.GetInstallCommand(ctx, host.ID)
	if err != nil {
		t.Fatalf("Failed to get install command: %v", err)
	}

	// Verify command contains the control plane address
	// 验证命令包含 Control Plane 地址
	expectedAddr := "control-plane.example.com:8000"
	if !containsString(cmd, expectedAddr) {
		t.Errorf("Install command should contain '%s', got: %s", expectedAddr, cmd)
	}

	// Verify command is a curl command with bash -s, default install-dir, and CLI host-id
	// 验证命令是 curl，并通过 bash -s 传入默认 install-dir 和 CLI host-id
	if !containsString(cmd, "curl") {
		t.Errorf("Install command should contain 'curl', got: %s", cmd)
	}
	if !containsString(cmd, "bash -s --") {
		t.Errorf("Install command should pipe to 'bash -s --', got: %s", cmd)
	}
	if !containsString(cmd, "--install-dir=$HOME/.stx/agent/") {
		t.Errorf("Install command should include default --install-dir=$HOME/.stx/agent/, got: %s", cmd)
	}
	expectedHostIDFlag := fmt.Sprintf("--host-id %d", host.ID)
	if !containsString(cmd, expectedHostIDFlag) {
		t.Errorf("Install command should include '%s', got: %s", expectedHostIDFlag, cmd)
	}
	if containsString(cmd, "?host_id=") {
		t.Errorf("Install command URL should not contain '?host_id=', got: %s", cmd)
	}
}

// containsString checks if a string contains a substring
// containsString 检查字符串是否包含子字符串
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestIsOnlineWithSince verifies Host.IsOnlineWithSince: when since is set, online requires
// last_heartbeat after since and within timeout.
func TestIsOnlineWithSince(t *testing.T) {
	timeout := 30 * time.Second
	now := time.Now()
	recent := now.Add(-5 * time.Second)        // within timeout
	old := now.Add(-60 * time.Second)          // outside timeout
	processStart := now.Add(-10 * time.Second) // e.g. platform started 10s ago

	// LastHeartbeat before processStart -> offline even if within timeout window from "now"
	hBefore := &Host{LastHeartbeat: ptrTime(processStart.Add(-1 * time.Second))}
	if hBefore.IsOnlineWithSince(timeout, processStart) {
		t.Error("expected offline when LastHeartbeat is before processStart")
	}

	// LastHeartbeat after processStart and within timeout -> online
	hAfter := &Host{LastHeartbeat: &recent}
	if !hAfter.IsOnlineWithSince(timeout, processStart) {
		t.Error("expected online when LastHeartbeat is after processStart and within timeout")
	}

	// LastHeartbeat after processStart but outside timeout -> offline
	hOld := &Host{LastHeartbeat: &old}
	if hOld.IsOnlineWithSince(timeout, processStart) {
		t.Error("expected offline when LastHeartbeat is outside timeout")
	}

	// Zero since: backward compat, only timeout matters
	if !hAfter.IsOnlineWithSince(timeout, time.Time{}) {
		t.Error("expected online with zero since when within timeout")
	}
	if hOld.IsOnlineWithSince(timeout, time.Time{}) {
		t.Error("expected offline with zero since when outside timeout")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// TestUpdateAgentStatusWithHostIDBinding tests host binding with hostID and IP auto-correction.
// TestUpdateAgentStatusWithHostIDBinding 测试通过 hostID 显式绑定及 IP 自动校正功能。
func TestUpdateAgentStatusWithHostIDBinding(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	// 1. Create a host with 127.0.0.1
	// 1. 创建 IP 为 127.0.0.1 的主机
	createdHost, err := svc.Create(ctx, &CreateHostRequest{
		Name:      "你瞅啥！",
		IPAddress: "127.0.0.1",
		HostType:  HostTypeBareMetal,
	})
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}

	// 2. Register Agent with hostID and real detected outbound IP 10.0.0.211
	// 2. 使用 hostID 和真实探测出口 IP 10.0.0.211 注册 Agent
	updatedHost, err := svc.UpdateAgentStatus(ctx, createdHost.ID, "10.0.0.211", "agent-12345", "1.0.0", nil, "panda001")
	if err != nil {
		t.Fatalf("UpdateAgentStatus failed: %v", err)
	}

	if updatedHost.ID != createdHost.ID {
		t.Errorf("Expected host ID %d, got %d", createdHost.ID, updatedHost.ID)
	}
	if updatedHost.AgentStatus != AgentStatusInstalled {
		t.Errorf("Expected status %s, got %s", AgentStatusInstalled, updatedHost.AgentStatus)
	}
	if updatedHost.IPAddress != "10.0.0.211" {
		t.Errorf("Expected IP to be auto-corrected to 10.0.0.211, got %s", updatedHost.IPAddress)
	}
}

// TestUpdateAgentStatusWithHostnameFallback tests host binding via hostname when uninstalled.
// TestUpdateAgentStatusWithHostnameFallback 测试未安装状态下通过主机名匹配的兜底机制。
func TestUpdateAgentStatusWithHostnameFallback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	// 1. Create a host with mismatched IP but matching name
	// 1. 创建 IP 不匹配但名称匹配的主机
	createdHost, err := svc.Create(ctx, &CreateHostRequest{
		Name:      "panda001",
		IPAddress: "127.0.0.1",
		HostType:  HostTypeBareMetal,
	})
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}

	// 2. Register Agent with hostID=0 and reported hostname panda001
	// 2. 使用 hostID=0 和上报的主机名 panda001 注册 Agent
	updatedHost, err := svc.UpdateAgentStatus(ctx, 0, "10.0.0.211", "agent-67890", "1.0.0", nil, "panda001")
	if err != nil {
		t.Fatalf("UpdateAgentStatus failed: %v", err)
	}

	if updatedHost.ID != createdHost.ID {
		t.Errorf("Expected host ID %d, got %d", createdHost.ID, updatedHost.ID)
	}
	if updatedHost.AgentStatus != AgentStatusInstalled {
		t.Errorf("Expected status %s, got %s", AgentStatusInstalled, updatedHost.AgentStatus)
	}
}
