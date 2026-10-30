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
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/LeonYoah/stx/internal/apps/auth"
	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// newMonitoringTaskAlertStore 返回真实的 monitoring 任务告警存储（独立 sqlite 库）和一个启用的 webhook 渠道 ID。
// newMonitoringTaskAlertStore returns the real monitoring task alert store (on its own sqlite database) and the ID
// of an enabled webhook channel.
func newMonitoringTaskAlertStore(t *testing.T) (*monitoring.Service, uint) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "monitoring.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open monitoring db: %v", err)
	}
	if sqlDB, err := database.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := database.AutoMigrate(&auth.User{}, &clusterapp.Cluster{}, &clusterapp.ClusterNode{}, &monitor.ProcessEvent{},
		&monitoring.AlertRule{}, &monitoring.AlertPolicy{}, &monitoring.AlertState{}, &monitoring.NotificationChannel{},
		&monitoring.NotificationDelivery{}, &monitoring.SyncAlertEpisode{}); err != nil {
		t.Fatalf("migrate monitoring db: %v", err)
	}
	repo := monitoring.NewRepository(database)
	service := monitoring.NewService(clusterapp.NewService(clusterapp.NewRepository(database), nil, nil), monitor.NewService(monitor.NewRepository(database)), repo)
	channel := &monitoring.NotificationChannel{Name: "ops-webhook", Type: monitoring.NotificationChannelTypeWebhook, Enabled: true, Endpoint: "http://127.0.0.1:1/hook"}
	if err := repo.CreateNotificationChannel(context.Background(), channel); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return service, channel.ID
}

// t1 实测 B1 的真实路径：sync 的 PUT 经真实 monitoring 存储保存，去掉 B4（不带 cdc_delay_*）再勾回，阈值和持续时间
// 仍是 120 / 2。内存桩会原样保存参数，挡不住这个问题。
// The real path of t1 live test B1: sync PUTs go through the real monitoring store; removing B4 (without
// cdc_delay_*) and adding it back still yields 120 / 2. An in-memory stub stores the params verbatim and cannot catch
// the bug.
func TestTaskAlertRemoveAndReaddCDCDelayKeepsParamsWithMonitoringStore(t *testing.T) {
	service, _, task := newOutcomeTestService(t, nil)
	store, channelID := newMonitoringTaskAlertStore(t)
	service.SetTaskAlertPolicyStore(store)
	ctx := context.Background()
	owner := executionapp.Actor{UserID: 1}
	withB4 := []string{syncalert.RuleJobFailed, syncalert.RuleCDCDelayHigh}
	threshold, duration := 120, 2

	cfg, err := service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: withB4, ChannelIDs: []uint{channelID}, CDCDelayThresholdSeconds: &threshold, CDCDelayDurationMinutes: &duration})
	if err != nil || cfg.CDCDelayThresholdSeconds != 120 || cfg.CDCDelayDurationMinutes != 2 {
		t.Fatalf("create with 120 / 2: %+v err=%v", cfg, err)
	}
	cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{channelID}})
	if err != nil || cfg.HasRule(syncalert.RuleCDCDelayHigh) || cfg.CDCDelayThresholdSeconds != 120 || cfg.CDCDelayDurationMinutes != 2 {
		t.Fatalf("removing B4 without params must keep 120 / 2: %+v err=%v", cfg, err)
	}
	if got, err := service.GetTaskAlertForActor(ctx, owner, task.ID); err != nil || got.CDCDelayThresholdSeconds != 120 || got.CDCDelayDurationMinutes != 2 {
		t.Fatalf("GET while B4 is off: %+v err=%v", got, err)
	}
	cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: withB4, ChannelIDs: []uint{channelID}})
	if err != nil || !cfg.HasRule(syncalert.RuleCDCDelayHigh) || cfg.CDCDelayThresholdSeconds != 120 || cfg.CDCDelayDurationMinutes != 2 {
		t.Fatalf("adding B4 back without params must restore 120 / 2: %+v err=%v", cfg, err)
	}
	// 去掉 B4 时明确填 0：恢复默认，再勾回也是默认。 / Explicit 0 while removing B4 resets to the defaults.
	zero := 0
	if cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{channelID}, CDCDelayThresholdSeconds: &zero, CDCDelayDurationMinutes: &zero}); err != nil ||
		cfg.CDCDelayThresholdSeconds != syncalert.DefaultCDCDelayThresholdSeconds || cfg.CDCDelayDurationMinutes != syncalert.DefaultCDCDelayDurationMinutes {
		t.Fatalf("explicit 0 resets to the defaults: %+v err=%v", cfg, err)
	}
}
