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

package monitoring

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	monitorapp "github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// componentsByName 把组件列表转成按名索引，便于断言。
// componentsByName indexes the component list by name for assertions.
func componentsByName(components []*IntegrationComponentStatus) map[string]*IntegrationComponentStatus {
	out := make(map[string]*IntegrationComponentStatus, len(components))
	for _, component := range components {
		if component == nil {
			continue
		}
		out[component.Name] = component
	}
	return out
}

// setupIntegrationStatusService 构造带真实 SQLite 的监控服务，并返回 DB 供测试造集群/节点数据。
// setupIntegrationStatusService builds a monitoring service on a real SQLite db and returns the
// db so tests can seed cluster/node rows.
func setupIntegrationStatusService(t *testing.T, hosts map[uint]*cluster.HostInfo) (*Service, *gorm.DB, context.Context) {
	t.Helper()
	database, cleanup := setupMonitoringNodeOfflineTestDB(t)
	t.Cleanup(cleanup)

	clusterRepo := cluster.NewRepository(database)
	clusterService := cluster.NewService(clusterRepo, &stubNodeOfflineHostProvider{hosts: hosts}, &cluster.ServiceConfig{
		HeartbeatTimeout: 30 * time.Second,
	})
	service := NewService(clusterService, monitorapp.NewService(monitorapp.NewRepository(database)), NewRepository(database))
	return service, database, context.Background()
}

func TestService_GetIntegrationStatus_disabled(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = false

	service := &Service{}
	data, err := service.GetIntegrationStatus(context.Background())
	if err != nil {
		t.Fatalf("GetIntegrationStatus returned error: %v", err)
	}

	components := componentsByName(data.Components)
	for _, name := range []string{"embedded_tsdb", "seatunnel_metrics", "remote_ingest"} {
		component, ok := components[name]
		if !ok {
			t.Fatalf("expected component %s in status payload", name)
		}
		if component.Healthy {
			t.Fatalf("expected component %s to be unhealthy when disabled", name)
		}
		if !strings.Contains(component.Error, "disabled by config") {
			t.Fatalf("component %s unexpected error: %q", name, component.Error)
		}
	}
	if len(data.Components) != 3 {
		t.Fatalf("expected exactly 3 components, got %d", len(data.Components))
	}
}

func TestService_GetIntegrationStatus_enabledEmbeddedComponents(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	// 测试环境内嵌 tsdb 不启动：embedded_tsdb 组件应降级为不健康但接口不报错。
	// The embedded tsdb is not started in tests: the embedded_tsdb component degrades to
	// unhealthy while the endpoint still returns data.
	_ = embedts.Close()

	service, _, ctx := setupIntegrationStatusService(t, nil)
	data, err := service.GetIntegrationStatus(ctx)
	if err != nil {
		t.Fatalf("GetIntegrationStatus returned error: %v", err)
	}
	components := componentsByName(data.Components)

	tsdbComponent := components["embedded_tsdb"]
	if tsdbComponent == nil {
		t.Fatal("missing embedded_tsdb component")
	}
	if tsdbComponent.Healthy {
		t.Fatal("expected embedded_tsdb to be unhealthy while not started")
	}
	if tsdbComponent.Meta["dir"] == "" {
		t.Fatalf("expected dir meta on embedded_tsdb, got %+v", tsdbComponent.Meta)
	}
	if tsdbComponent.Meta["retention"] == "" {
		t.Fatalf("expected retention meta on embedded_tsdb, got %+v", tsdbComponent.Meta)
	}

	metricsComponent := components["seatunnel_metrics"]
	if metricsComponent == nil {
		t.Fatal("missing seatunnel_metrics component")
	}
	if metricsComponent.Healthy {
		t.Fatal("expected seatunnel_metrics unhealthy with zero targets")
	}
	if !strings.Contains(metricsComponent.Error, "no managed cluster node") {
		t.Fatalf("unexpected seatunnel_metrics error: %q", metricsComponent.Error)
	}
	if metricsComponent.Meta["total"] != "0" {
		t.Fatalf("expected total=0 meta, got %+v", metricsComponent.Meta)
	}

	ingestComponent := components["remote_ingest"]
	if ingestComponent == nil {
		t.Fatal("missing remote_ingest component")
	}
	if !ingestComponent.Healthy {
		t.Fatalf("expected remote_ingest healthy, got error %q", ingestComponent.Error)
	}
	if ingestComponent.Meta["path"] == "" {
		t.Fatalf("expected path meta on remote_ingest, got %+v", ingestComponent.Meta)
	}
}

func TestService_ScrapeTargets_translatesOnlineNodes(t *testing.T) {
	originalPath := config.Config.Observability.SeatunnelMetric.Path
	t.Cleanup(func() { config.Config.Observability.SeatunnelMetric.Path = originalPath })
	config.Config.Observability.SeatunnelMetric.Path = "/hazelcast/rest/instance/metrics"

	now := time.Now().UTC()
	hosts := map[uint]*cluster.HostInfo{
		7: {
			ID:            7,
			Name:          "host-7",
			IPAddress:     "10.1.2.3",
			AgentID:       "agent-7",
			AgentStatus:   "installed",
			LastHeartbeat: &now,
		},
		// host-8 有心跳但对应节点 HazelcastPort=0，应被跳过。
		// host-8 heartbeats but its node has HazelcastPort=0 and must be skipped.
		8: {
			ID:            8,
			Name:          "host-8",
			IPAddress:     "10.1.2.4",
			AgentID:       "agent-8",
			AgentStatus:   "installed",
			LastHeartbeat: &now,
		},
	}
	service, database, ctx := setupIntegrationStatusService(t, hosts)

	testCluster := &cluster.Cluster{
		Name:           "scrape-targets-cluster",
		DeploymentMode: cluster.DeploymentModeHybrid,
		Version:        "2.3.11",
		Status:         cluster.ClusterStatusRunning,
		InstallDir:     "/opt/seatunnel",
		CreatedBy:      1,
		Config:         map[string]interface{}{"env": "prod"},
	}
	if err := database.WithContext(ctx).Create(testCluster).Error; err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}
	for _, node := range []*cluster.ClusterNode{
		{ClusterID: testCluster.ID, HostID: 7, Role: cluster.NodeRoleMasterWorker, InstallDir: "/opt/seatunnel", HazelcastPort: 5801, Status: cluster.NodeStatusRunning},
		{ClusterID: testCluster.ID, HostID: 8, Role: cluster.NodeRoleWorker, InstallDir: "/opt/seatunnel", HazelcastPort: 0, Status: cluster.NodeStatusRunning},
	} {
		if err := database.WithContext(ctx).Create(node).Error; err != nil {
			t.Fatalf("failed to create node: %v", err)
		}
	}

	targets, err := service.ScrapeTargets(ctx)
	if err != nil {
		t.Fatalf("ScrapeTargets returned error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 scrape target, got %d: %+v", len(targets), targets)
	}
	target := targets[0]
	if target.URL != "http://10.1.2.3:5801/hazelcast/rest/instance/metrics" {
		t.Fatalf("unexpected scrape URL: %q", target.URL)
	}
	if target.Labels["job"] != "seatunnel_engine_http" {
		t.Fatalf("unexpected job label: %q", target.Labels["job"])
	}
	if target.Labels["instance"] != "10.1.2.3:5801" {
		t.Fatalf("unexpected instance label: %q", target.Labels["instance"])
	}
	if target.Labels["cluster"] != "scrape-targets-cluster" || target.Labels["cluster_name"] != "scrape-targets-cluster" {
		t.Fatalf("unexpected cluster labels: %+v", target.Labels)
	}
	if target.Labels["env"] != "prod" {
		t.Fatalf("unexpected env label: %q", target.Labels["env"])
	}
}
