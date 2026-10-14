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
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 本地时区写入的实例必须能被 UTC 窗口查到，否则调度去重失效。
// Rows written in a local zone must match a UTC window, or schedule dedup breaks.
func TestHasScheduledJobInstanceInWindowAcrossTimezones(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	// autoCreateTime 写入本地时区 / autoCreateTime writes time.Local
	created := time.Date(2026, 10, 14, 12, 54, 29, 0, time.Local)
	instance := &JobInstance{TaskID: 7, RunType: RunTypeSchedule, PlatformJobID: "1", Status: JobStatusRunning, CreatedAt: created}
	if err := service.repo.CreateJobInstance(ctx, instance); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	windowStart := time.Date(2026, 10, 14, 12, 54, 0, 0, time.Local)
	for _, tc := range []struct {
		name  string
		start time.Time
		want  bool
	}{
		{"utc window", windowStart.UTC(), true},
		{"local window", windowStart, true},
		{"next minute utc", windowStart.Add(time.Minute).UTC(), false},
		{"previous minute utc", windowStart.Add(-time.Minute).UTC(), false},
	} {
		got, err := service.repo.HasScheduledJobInstanceInWindow(ctx, 7, tc.start, tc.start.Add(time.Minute))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func newScheduledTestTask(t *testing.T, service *Service, cron string) *Task {
	t.Helper()
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "schedule_root",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	task, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "schedule_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { job.mode = \"BATCH\" }\nsource { FakeSource { plugin_output = \"fake\" } }\nsink { Console {} }",
		Definition: JSONMap{
			"execution_mode": "cluster",
			"schedule": map[string]interface{}{
				"enabled":   true,
				"cron_expr": cron,
				"timezone":  "Asia/Shanghai",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, _, err := service.PublishTask(ctx, task.ID, "initial", 1); err != nil {
		t.Fatalf("publish task: %v", err)
	}
	return task
}

func countScheduledInstances(t *testing.T, service *Service, taskID uint) int {
	t.Helper()
	var count int64
	if err := service.repo.db.Model(&JobInstance{}).
		Where("task_id = ? AND run_type = ?", taskID, RunTypeSchedule).Count(&count).Error; err != nil {
		t.Fatalf("count instances: %v", err)
	}
	return int(count)
}

func shanghaiTime(t *testing.T, hour, minute, second int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return time.Date(2026, 10, 14, hour, minute, second, 0, loc)
}

func boolPtr(v bool) *bool { return &v }

// scheduleTestClock 让 GORM autoCreateTime 跟随模拟 tick 时间，使去重窗口可测。
// scheduleTestClock makes GORM autoCreateTime follow the simulated tick so dedup windows are testable.
type scheduleTestClock struct{ now time.Time }

func newScheduleTestService(t *testing.T) (*Service, *scheduleTestClock) {
	t.Helper()
	clock := &scheduleTestClock{now: time.Now()}
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{NowFunc: func() time.Time { return clock.now.In(time.Local) }})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&Task{}, &TaskVersion{}, &JobInstance{}, &GlobalVariable{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(NewRepository(database)), clock
}

func tickAt(t *testing.T, service *Service, clock *scheduleTestClock, now time.Time) {
	t.Helper()
	clock.now = now
	if err := service.triggerScheduledTasks(context.Background(), now); err != nil {
		t.Fatalf("tick at %s: %v", now, err)
	}
}

// 复现 run1：12:54 的 tick 因整机停顿丢失，13:02 恢复。
// Reproduces run1: the 12:54 tick is lost to a box stall and ticking resumes at 13:02.
func TestScheduleCatchupFiresMissedMinute(t *testing.T) {
	service, clock := newScheduleTestService(t)
	task := newScheduledTestTask(t, service, "54 12 * * *")
	service.schedule.graceOverride = 10 * time.Minute
	service.schedule.lastCheck = shanghaiTime(t, 12, 53, 0)
	tickAt(t, service, clock, shanghaiTime(t, 13, 2, 8))
	if got := countScheduledInstances(t, service, task.ID); got != 1 {
		t.Fatalf("expected missed minute to be caught up once, got %d", got)
	}
}

func TestScheduleCatchupDisabledKeepsLegacyBehavior(t *testing.T) {
	service, clock := newScheduleTestService(t)
	task := newScheduledTestTask(t, service, "54 12 * * *")
	service.schedule.catchupOverride = boolPtr(false)
	service.schedule.lastCheck = shanghaiTime(t, 12, 53, 0)
	tickAt(t, service, clock, shanghaiTime(t, 13, 2, 8))
	if got := countScheduledInstances(t, service, task.ID); got != 0 {
		t.Fatalf("catch-up disabled must not fire missed minute, got %d", got)
	}
	// 当前分钟命中时仍按旧行为触发 / On-time minute still fires.
	tickAt(t, service, clock, shanghaiTime(t, 12, 54, 29).AddDate(0, 0, 1))
	if got := countScheduledInstances(t, service, task.ID); got != 1 {
		t.Fatalf("expected on-time fire with catch-up disabled, got %d", got)
	}
}

func TestScheduleCatchupSkipsBeyondGrace(t *testing.T) {
	service, clock := newScheduleTestService(t)
	task := newScheduledTestTask(t, service, "54 12 * * *")
	service.schedule.graceOverride = 5 * time.Minute
	service.schedule.lastCheck = shanghaiTime(t, 12, 53, 0)
	tickAt(t, service, clock, shanghaiTime(t, 13, 2, 8))
	if got := countScheduledInstances(t, service, task.ID); got != 0 {
		t.Fatalf("misfire older than grace must be skipped, got %d", got)
	}
}

func TestScheduleCatchupDoesNotDoubleFire(t *testing.T) {
	service, clock := newScheduleTestService(t)
	task := newScheduledTestTask(t, service, "* * * * *")
	service.schedule.graceOverride = 10 * time.Minute
	// 先准点触发 12:54，再模拟同一分钟内的重复 tick / Fire 12:54 on time, then a duplicate tick in the same minute.
	service.schedule.lastCheck = shanghaiTime(t, 12, 53, 0)
	for _, now := range []time.Time{shanghaiTime(t, 12, 54, 1), shanghaiTime(t, 12, 54, 40)} {
		tickAt(t, service, clock, now)
	}
	if got := countScheduledInstances(t, service, task.ID); got != 1 {
		t.Fatalf("expected single fire within a minute, got %d", got)
	}
	// 停顿 5 分钟后恢复：多个错过分钟只补跑最近一次 / After a 5-minute stall only the latest missed minute fires.
	service.schedule.lastCheck = shanghaiTime(t, 12, 54, 0)
	tickAt(t, service, clock, shanghaiTime(t, 12, 59, 30))
	if got := countScheduledInstances(t, service, task.ID); got != 2 {
		t.Fatalf("expected one coalesced catch-up fire, got %d", got)
	}
}
