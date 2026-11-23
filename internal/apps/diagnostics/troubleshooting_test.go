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

	// 1. 初始列表查询会自动播种中英文 10 条预置案例
	// 1. Initial list query should auto-seed 10 preset troubleshooting cases (5 zh, 5 en)
	allPresets, total, err := svc.ListMemories(ctx, nil)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if total != 10 || len(allPresets) != 10 {
		t.Fatalf("expected 10 preset memories (5 zh, 5 en), got total %d, len %d", total, len(allPresets))
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
	if zhTotal != 5 || len(zhList) != 5 {
		t.Fatalf("expected 5 zh preset memories, got total %d, len %d", zhTotal, len(zhList))
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
	if enTotal != 5 || len(enList) != 5 {
		t.Fatalf("expected 5 en preset memories, got total %d, len %d", enTotal, len(enList))
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
	if zhCountWithCustom != 6 || len(zhWithCustom) != 6 { // 5 条中文预置 + 1 条自定义 / 5 zh presets + 1 custom
		t.Fatalf("expected 6 items in zh list, got total %d, len %d", zhCountWithCustom, len(zhWithCustom))
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
	if enCountWithCustom != 6 || len(enWithCustom) != 6 { // 5 条英文预置 + 1 条自定义 / 5 en presets + 1 custom
		t.Fatalf("expected 6 items in en list, got total %d, len %d", enCountWithCustom, len(enWithCustom))
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

// TestSeedPresetMemoriesIdempotentTopUp 验证预置播种对已有库幂等：重复播种不产生重复行，
// 新增的预置条目（如 jdk24）也能补齐进旧库。
// TestSeedPresetMemoriesIdempotentTopUp verifies seeding is idempotent on an existing
// database: re-seeding creates no duplicates, and newly added presets (e.g. jdk24) are
// back-filled into databases that already hold the older presets.
func TestSeedPresetMemoriesIdempotentTopUp(t *testing.T) {
	ctx := context.Background()
	svc := newTroubleshootingTestService(t)
	repo := svc.repo

	// 模拟旧库：只播种过一次，再重复播种一次
	// Simulate an existing database: seed once, then seed again
	if err := repo.SeedPresetMemories(ctx); err != nil {
		t.Fatalf("first seed failed: %v", err)
	}
	if err := repo.SeedPresetMemories(ctx); err != nil {
		t.Fatalf("second seed failed: %v", err)
	}

	_, total, err := svc.ListMemories(ctx, nil)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if total != 10 {
		t.Fatalf("expected exactly 10 presets after repeated seeding, got %d", total)
	}

	// 逐语言校验 jdk24 预置条目存在且无重复
	// Verify the jdk24 preset exists exactly once per language
	for _, lang := range []string{"zh", "en"} {
		items, count, err := svc.ListMemories(ctx, &TroubleshootingMemoryQuery{Language: lang})
		if err != nil {
			t.Fatalf("list %s memories failed: %v", lang, err)
		}
		found := 0
		for _, item := range items {
			if item.PresetKey == "jdk24_subject_getsubject_unsupported" {
				found++
				if item.Fingerprint != "getSubject is not supported" {
					t.Fatalf("lang %s: unexpected fingerprint %q", lang, item.Fingerprint)
				}
			}
		}
		if found != 1 || count != 5 {
			t.Fatalf("lang %s: expected exactly 1 jdk24 preset among 5 presets, got %d/%d", lang, found, count)
		}
	}
}
