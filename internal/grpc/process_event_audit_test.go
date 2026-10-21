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

package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestApplyManualFollowupAttributionUsesOriginalActor(t *testing.T) {
	dsn := fmt.Sprintf("file:process-followup-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(&auth.User{}, &audit.CommandLog{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &auth.User{Username: "ops", PasswordHash: "x", IsActive: true}
	if err := database.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	uid := uint(user.ID)
	repo := audit.NewRepository(database)
	if err := repo.CreateCommandLog(context.Background(), &audit.CommandLog{
		CommandID:   "cmd-manual-1",
		RequestID:   "req-manual-1",
		AgentID:     "agent-1",
		CommandType: "start",
		ClientType:  "web",
		Status:      audit.CommandStatusSuccess,
		CreatedBy:   &uid,
	}); err != nil {
		t.Fatalf("create command log: %v", err)
	}

	log := &audit.AuditLog{
		Username:     "agent",
		Action:       "restart",
		ResourceType: "cluster_node",
		Trigger:      "auto",
		ClientType:   "system",
		Details:      audit.AuditDetails{},
		UserAgent:    "stx-agent",
	}
	report := &pb.ProcessEventReport{
		Details: map[string]string{
			"trigger":    "manual_followup",
			"command_id": "cmd-manual-1",
			"request_id": "req-manual-1",
		},
	}
	applyManualFollowupAttribution(context.Background(), repo, log, report)

	if log.Trigger != "manual_followup" {
		t.Fatalf("trigger=%q", log.Trigger)
	}
	if log.CommandID != "cmd-manual-1" || log.RequestID != "req-manual-1" {
		t.Fatalf("ids not set: %+v", log)
	}
	if log.UserID == nil || *log.UserID != uid {
		t.Fatalf("expected original actor, got %+v", log.UserID)
	}
	if log.Username != "ops" {
		t.Fatalf("username=%q", log.Username)
	}
	if log.ClientType != "web" {
		t.Fatalf("client_type=%q", log.ClientType)
	}
}

func TestApplyManualFollowupAttributionLeavesCrashRestartAsSystem(t *testing.T) {
	dsn := fmt.Sprintf("file:process-crash-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(&audit.CommandLog{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := audit.NewRepository(database)
	log := &audit.AuditLog{
		Username:   "agent",
		Action:     "restart",
		Trigger:    "auto",
		ClientType: "system",
		UserAgent:  "stx-agent",
		Details:    audit.AuditDetails{},
	}
	applyManualFollowupAttribution(context.Background(), repo, log, &pb.ProcessEventReport{
		Details: map[string]string{"install_dir": "/opt/st"},
	})
	if log.Trigger != "auto" || log.Username != "agent" || log.UserID != nil {
		t.Fatalf("crash restart should stay system/auto: %+v", log)
	}
}
