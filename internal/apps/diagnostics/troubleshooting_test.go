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

package diagnostics

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTroubleshootingTestService(t *testing.T) *TroubleshootingService {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite in memory: %v", err)
	}
	if err := database.AutoMigrate(&TroubleshootingMemory{}); err != nil {
		t.Fatalf("auto migrate troubleshooting models: %v", err)
	}

	repo := NewTroubleshootingRepository(database)
	return NewTroubleshootingService(repo)
}

func TestTroubleshootingServicePresetAndCRUD(t *testing.T) {
	ctx := context.Background()
	svc := newTroubleshootingTestService(t)

	// 1. 初始列表查询会自动播种预置案例
	// 1. Initial list query should auto-seed preset troubleshooting cases
	presets, total, err := svc.ListMemories(ctx, nil)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if total < 4 || len(presets) < 4 {
		t.Fatalf("expected at least 4 preset memories, got %d", total)
	}

	// 2. 校验创建排障经验时解决方案非空约束
	// 2. Enforce non-empty solution validation on create
	_, err = svc.CreateMemory(ctx, &CreateTroubleshootingMemoryRequest{
		TargetType:   "error",
		Fingerprint:  "CustomErr",
		Title:        "测试标题",
		ErrorSummary: "测试摘要",
		Solution:     "   ", // 空白方案 / Empty whitespace
	})
	if err == nil {
		t.Fatalf("expected error on empty solution, got nil")
	}

	// 3. 正常创建用户排障经验
	// 3. Successfully create user-contributed memory entry
	item, err := svc.CreateMemory(ctx, &CreateTroubleshootingMemoryRequest{
		TargetType:     "error",
		Fingerprint:    "CustomKafkaOffsetOutOfRange",
		Title:          "Kafka 消费位点超出范围恢复方案",
		ErrorSummary:   "OffsetOutOfRangeException: The fetch offset is larger than the high watermark",
		RootCause:      "Topic 历史数据被主动保留策略删除，消费者 Offset 失效",
		Solution:       "1. 调整作业配置 auto.offset.reset 为 latest 或 earliest；2. 重启作业。",
		ActionsTaken:   []string{"设置 auto.offset.reset=latest", "重启作业"},
		PreventiveTips: "关注 Kafka topic 数据的 retention.ms 配置",
		Tags:           []string{"kafka", "offset", "consumer"},
		Author:         "SRE组-李四",
	})
	if err != nil {
		t.Fatalf("create memory failed: %v", err)
	}
	if item.ID == "" || item.IsPreset {
		t.Fatalf("expected valid non-preset memory item, got %+v", item)
	}

	// 4. 按指纹筛选应精确返回刚创建的数据
	// 4. Filter by fingerprint should return the newly created entry
	matched, count, err := svc.ListMemories(ctx, &TroubleshootingMemoryQuery{
		Fingerprint: "KafkaOffsetOutOfRange",
	})
	if err != nil {
		t.Fatalf("list with fingerprint filter failed: %v", err)
	}
	if count != 1 || len(matched) != 1 || matched[0].ID != item.ID {
		t.Fatalf("expected exactly 1 matched entry with ID %s, got %d", item.ID, count)
	}

	// 5. 更新排障经验
	// 5. Update troubleshooting memory
	updated, err := svc.UpdateMemory(ctx, uint(1), &UpdateTroubleshootingMemoryRequest{
		Title:    "MySQL 超时更新后方案",
		Solution: "更新后的详细解决措施",
	})
	if err != nil {
		t.Fatalf("update memory failed: %v", err)
	}
	if updated.Title != "MySQL 超时更新后方案" || updated.Solution != "更新后的详细解决措施" {
		t.Fatalf("update memory did not apply changes: %+v", updated)
	}

	// 6. 删除排障经验
	// 6. Delete troubleshooting memory
	if err := svc.DeleteMemory(ctx, uint(1)); err != nil {
		t.Fatalf("delete memory failed: %v", err)
	}
	_, err = svc.GetMemory(ctx, uint(1))
	if err == nil {
		t.Fatalf("expected not found after deletion, got nil error")
	}
}
