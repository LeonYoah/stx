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

package config

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testNodeInfoProvider struct {
	installDir string
}

func (p *testNodeInfoProvider) GetNodeInstallDir(_ context.Context, _ uint, _ uint) (string, error) {
	return p.installDir, nil
}

type testAgentClient struct {
	pushCalls int
}

func (c *testAgentClient) PullConfig(_ context.Context, _ uint, _ string, _ ConfigType) (string, error) {
	return "", nil
}

func (c *testAgentClient) PushConfig(_ context.Context, _ uint, _ string, _ ConfigType, _ string) error {
	c.pushCalls++
	return nil
}

type portUpdateCall struct {
	clusterID  uint
	hostID     uint
	port       int
	configType ConfigType
	mode       string
}

type testPortMetadataUpdater struct {
	calls []portUpdateCall
}

func (u *testPortMetadataUpdater) UpdateSeatunnelAPIPortByHost(_ context.Context, clusterID uint, hostID uint, port int) error {
	u.calls = append(u.calls, portUpdateCall{clusterID: clusterID, hostID: hostID, port: port})
	return nil
}

func (u *testPortMetadataUpdater) UpdateHazelcastPortByHost(_ context.Context, clusterID uint, hostID uint, configType ConfigType, port int) error {
	u.calls = append(u.calls, portUpdateCall{clusterID: clusterID, hostID: hostID, port: port, configType: configType})
	return nil
}

func (u *testPortMetadataUpdater) UpdateClusterJobLogMode(_ context.Context, clusterID uint, mode string) error {
	u.calls = append(u.calls, portUpdateCall{clusterID: clusterID, mode: mode})
	return nil
}

func newConfigTestService(t *testing.T) (*Service, *gorm.DB, *testAgentClient, *testPortMetadataUpdater) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&Config{}, &ConfigVersion{}); err != nil {
		t.Fatalf("failed to migrate config models: %v", err)
	}
	repo := NewRepository(db)
	agent := &testAgentClient{}
	updater := &testPortMetadataUpdater{}
	service := NewService(repo, nil, &testNodeInfoProvider{installDir: "/tmp/seatunnel"}, agent)
	service.SetPortMetadataUpdater(updater)
	return service, db, agent, updater
}

func TestExtractSeatunnelHTTPPort(t *testing.T) {
	content := `seatunnel:
  engine:
    http:
      enable-http: true
      port: 18081
`
	port, ok, err := extractSeatunnelHTTPPort(content)
	if err != nil {
		t.Fatalf("extractSeatunnelHTTPPort returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected port to be found")
	}
	if port != 18081 {
		t.Fatalf("expected port 18081, got %d", port)
	}
}

func TestExtractHazelcastNetworkPort(t *testing.T) {
	content := `hazelcast:
  network:
    port:
      port: 5809
`
	port, ok, err := extractHazelcastNetworkPort(content)
	if err != nil {
		t.Fatalf("extractHazelcastNetworkPort returned error: %v", err)
	}
	if !ok || port != 5809 {
		t.Fatalf("expected hazelcast port 5809, got ok=%v port=%d", ok, port)
	}
}

func TestExtractJobLogMode(t *testing.T) {
	mode, ok := extractJobLogMode("rootLogger.appenderRef.file.ref = routingAppender\n")
	if !ok || mode != "per_job" {
		t.Fatalf("expected per_job, got ok=%v mode=%q", ok, mode)
	}
	mode, ok = extractJobLogMode("rootLogger.appenderRef.file.ref = fileAppender\n")
	if !ok || mode != "mixed" {
		t.Fatalf("expected mixed, got ok=%v mode=%q", ok, mode)
	}
}

func TestUpdateNodeConfigSyncsSeatunnelAPIPortMetadata(t *testing.T) {
	service, db, agent, updater := newConfigTestService(t)
	ctx := context.Background()
	hostID := uint(11)
	config := &Config{
		ClusterID:  7,
		HostID:     &hostID,
		ConfigType: ConfigTypeSeatunnel,
		FilePath:   GetConfigFilePath(ConfigTypeSeatunnel),
		Content: `seatunnel:
  engine:
    http:
      enable-http: true
      port: 8080
`,
		Version:   1,
		UpdatedBy: 1,
	}
	if err := db.WithContext(ctx).Create(config).Error; err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	updated, err := service.Update(ctx, config.ID, &UpdateConfigRequest{
		Content: `seatunnel:
  engine:
    http:
      enable-http: true
      port: 18081
`,
	}, 2)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated config info")
	}
	if agent.pushCalls != 1 {
		t.Fatalf("expected one push call, got %d", agent.pushCalls)
	}
	if len(updater.calls) != 1 {
		t.Fatalf("expected one metadata update call, got %d", len(updater.calls))
	}
	if updater.calls[0].clusterID != 7 || updater.calls[0].hostID != hostID || updater.calls[0].port != 18081 {
		t.Fatalf("unexpected metadata update call: %+v", updater.calls[0])
	}
}

func TestSyncTemplateToAllNodesSyncsSeatunnelAPIPortMetadata(t *testing.T) {
	service, db, agent, updater := newConfigTestService(t)
	ctx := context.Background()
	hostA := uint(21)
	hostB := uint(22)
	template := &Config{
		ClusterID:  9,
		ConfigType: ConfigTypeSeatunnel,
		FilePath:   GetConfigFilePath(ConfigTypeSeatunnel),
		Content: `seatunnel:
  engine:
    http:
      enable-http: true
      port: 19090
`,
		Version:   1,
		UpdatedBy: 1,
	}
	nodeA := &Config{
		ClusterID:  9,
		HostID:     &hostA,
		ConfigType: ConfigTypeSeatunnel,
		FilePath:   GetConfigFilePath(ConfigTypeSeatunnel),
		Content:    "old-a",
		Version:    1,
		UpdatedBy:  1,
	}
	nodeB := &Config{
		ClusterID:  9,
		HostID:     &hostB,
		ConfigType: ConfigTypeSeatunnel,
		FilePath:   GetConfigFilePath(ConfigTypeSeatunnel),
		Content:    "old-b",
		Version:    1,
		UpdatedBy:  1,
	}
	for _, item := range []*Config{template, nodeA, nodeB} {
		if err := db.WithContext(ctx).Create(item).Error; err != nil {
			t.Fatalf("failed to create config: %v", err)
		}
	}

	result, err := service.SyncTemplateToAllNodes(ctx, 9, ConfigTypeSeatunnel, 3)
	if err != nil {
		t.Fatalf("SyncTemplateToAllNodes returned error: %v", err)
	}
	if result == nil || result.SyncedCount != 2 {
		t.Fatalf("expected synced count 2, got %+v", result)
	}
	if agent.pushCalls != 2 {
		t.Fatalf("expected two push calls, got %d", agent.pushCalls)
	}
	if len(updater.calls) != 2 {
		t.Fatalf("expected two metadata update calls, got %d", len(updater.calls))
	}
	expected := map[uint]int{hostA: 19090, hostB: 19090}
	for _, call := range updater.calls {
		if call.clusterID != 9 {
			t.Fatalf("unexpected cluster id in call: %+v", call)
		}
		if expected[call.hostID] != call.port {
			t.Fatalf("unexpected call payload: %+v", call)
		}
	}
}

func TestUpdateNodeConfigSyncsHazelcastPortMetadata(t *testing.T) {
	service, db, agent, updater := newConfigTestService(t)
	ctx := context.Background()
	hostID := uint(31)
	config := &Config{
		ClusterID:  12,
		HostID:     &hostID,
		ConfigType: ConfigTypeHazelcastMaster,
		FilePath:   GetConfigFilePath(ConfigTypeHazelcastMaster),
		Content: `hazelcast:
  network:
    port:
      port: 5801
`,
		Version:   1,
		UpdatedBy: 1,
	}
	if err := db.WithContext(ctx).Create(config).Error; err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	if _, err := service.Update(ctx, config.ID, &UpdateConfigRequest{Content: `hazelcast:
  network:
    port:
      port: 5901
`}, 2); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if agent.pushCalls != 1 {
		t.Fatalf("expected one push call, got %d", agent.pushCalls)
	}
	if len(updater.calls) != 1 {
		t.Fatalf("expected one metadata update call, got %d", len(updater.calls))
	}
	if updater.calls[0].configType != ConfigTypeHazelcastMaster || updater.calls[0].port != 5901 {
		t.Fatalf("unexpected hazelcast metadata sync: %+v", updater.calls[0])
	}
}

func TestUpdateNodeConfigSyncsJobLogModeMetadata(t *testing.T) {
	service, db, _, updater := newConfigTestService(t)
	ctx := context.Background()
	config := &Config{
		ClusterID:  15,
		ConfigType: ConfigTypeLog4j2,
		FilePath:   GetConfigFilePath(ConfigTypeLog4j2),
		Content:    "rootLogger.appenderRef.file.ref = fileAppender\n",
		Version:    1,
		UpdatedBy:  1,
	}
	if err := db.WithContext(ctx).Create(config).Error; err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	if _, err := service.Update(ctx, config.ID, &UpdateConfigRequest{
		Content: "rootLogger.appenderRef.file.ref = routingAppender\n",
	}, 2); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(updater.calls) != 1 {
		t.Fatalf("expected one metadata update call, got %d", len(updater.calls))
	}
	if updater.calls[0].clusterID != 15 || updater.calls[0].mode != "per_job" {
		t.Fatalf("unexpected job log mode sync: %+v", updater.calls[0])
	}
}
