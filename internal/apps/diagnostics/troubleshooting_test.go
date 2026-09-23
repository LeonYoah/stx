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

	// 1. 初始列表查询会自动播种中英文 8 条预置案例
	// 1. Initial list query should auto-seed 8 preset troubleshooting cases (4 zh, 4 en)
	allPresets, total, err := svc.ListMemories(ctx, nil)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if total != 8 || len(allPresets) != 8 {
		t.Fatalf("expected 8 preset memories (4 zh, 4 en), got total %d, len %d", total, len(allPresets))
	}
	for _, p := range allPresets {
		if p.IsPreset {
			if p.PresetKey == "" {
				t.Fatalf("expected preset memory %s to have PresetKey, got empty", p.Fingerprint)
			}
			if p.Language != "zh" && p.Language != "en" {
				t.Fatalf("expected preset memory %s to have valid language (zh/en), got %s", p.Fingerprint, p.Language)
			}
		}
	}

	// 2. 按语言筛选中文官方预置案例（跟随全局语言切分）
	// 2. Filter Chinese preset cases (partitioned by global language)
	zhList, zhTotal, err := svc.ListMemories(ctx, &TroubleshootingMemoryQuery{Language: "zh"})
	if err != nil {
		t.Fatalf("list zh memories failed: %v", err)
	}
	if zhTotal != 4 || len(zhList) != 4 {
		t.Fatalf("expected 4 zh preset memories, got total %d, len %d", zhTotal, len(zhList))
	}
	for _, item := range zhList {
		if item.Language != "zh" {
			t.Fatalf("expected item language to be zh, got %s", item.Language)
		}
	}

	// 3. 按语言筛选英文官方预置案例
	// 3. Filter English preset cases
	enList, enTotal, err := svc.ListMemories(ctx, &TroubleshootingMemoryQuery{Language: "en"})
	if err != nil {
		t.Fatalf("list en memories failed: %v", err)
	}
	if enTotal != 4 || len(enList) != 4 {
		t.Fatalf("expected 4 en preset memories, got total %d, len %d", enTotal, len(enList))
	}
	for _, item := range enList {
		if item.Language != "en" {
			t.Fatalf("expected item language to be en, got %s", item.Language)
		}
	}

	// 4. 校验创建排障经验时解决方案非空约束
	// 4. Enforce non-empty solution validation on create
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

	// 5. 正常创建用户排障经验（用户自定义经验不区分语言，全语言环境可见）
	// 5. Successfully create user-contributed memory entry (custom entries are visible across all languages)
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

	// 6. 验证用户自定义经验在 zh 和 en 语言查询下均能返回（用户自定义不考虑多语言切分）
	// 6. Verify user memory is returned under both zh and en queries (unconstrained by language filter)
	zhWithCustom, zhCountWithCustom, err := svc.ListMemories(ctx, &TroubleshootingMemoryQuery{Language: "zh"})
	if err != nil {
		t.Fatalf("list zh with custom failed: %v", err)
	}
	if zhCountWithCustom != 5 || len(zhWithCustom) != 5 { // 4 zh presets + 1 custom
		t.Fatalf("expected 5 items in zh list, got total %d, len %d", zhCountWithCustom, len(zhWithCustom))
	}
	hasCustomInZh := false
	for _, m := range zhWithCustom {
		if m.ID == item.ID {
			hasCustomInZh = true
			break
		}
	}
	if !hasCustomInZh {
		t.Fatalf("expected custom item %s to be present in zh list", item.ID)
	}

	enWithCustom, enCountWithCustom, err := svc.ListMemories(ctx, &TroubleshootingMemoryQuery{Language: "en"})
	if err != nil {
		t.Fatalf("list en with custom failed: %v", err)
	}
	if enCountWithCustom != 5 || len(enWithCustom) != 5 { // 4 en presets + 1 custom
		t.Fatalf("expected 5 items in en list, got total %d, len %d", enCountWithCustom, len(enWithCustom))
	}
	hasCustomInEn := false
	for _, m := range enWithCustom {
		if m.ID == item.ID {
			hasCustomInEn = true
			break
		}
	}
	if !hasCustomInEn {
		t.Fatalf("expected custom item %s to be present in en list", item.ID)
	}

	// 7. 按指纹筛选应精确返回刚创建的数据
	// 7. Filter by fingerprint should return the newly created entry
	matched, count, err := svc.ListMemories(ctx, &TroubleshootingMemoryQuery{
		Fingerprint: "KafkaOffsetOutOfRange",
	})
	if err != nil {
		t.Fatalf("list with fingerprint filter failed: %v", err)
	}
	if count != 1 || len(matched) != 1 || matched[0].ID != item.ID {
		t.Fatalf("expected exactly 1 matched entry with ID %s, got %d", item.ID, count)
	}

	// 8. 更新排障经验
	// 8. Update troubleshooting memory
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

	// 9. 删除排障经验
	// 9. Delete troubleshooting memory
	if err := svc.DeleteMemory(ctx, uint(1)); err != nil {
		t.Fatalf("delete memory failed: %v", err)
	}
	_, err = svc.GetMemory(ctx, uint(1))
	if err == nil {
		t.Fatalf("expected not found after deletion, got nil error")
	}
}
