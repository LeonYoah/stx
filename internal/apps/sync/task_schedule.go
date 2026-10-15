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
	"fmt"
	"log"
	"strings"
	stdsync "sync"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/schedulex"
)

const (
	defaultTaskScheduleTimezone = schedulex.DefaultTimezone
	scheduleTriggerSource       = "schedule"
)

type taskScheduleConfig struct {
	Enabled  bool
	CronExpr string
	Timezone string
}

func parseTaskSchedule(definition JSONMap) (*taskScheduleConfig, error) {
	cfg := &taskScheduleConfig{Enabled: false, Timezone: defaultTaskScheduleTimezone}
	if len(definition) == 0 {
		return cfg, nil
	}
	var raw map[string]interface{}
	switch value := definition["schedule"].(type) {
	case JSONMap:
		raw = map[string]interface{}(value)
	case map[string]interface{}:
		raw = value
	}
	if len(raw) == 0 {
		return cfg, nil
	}
	if value, ok := raw["enabled"].(bool); ok {
		cfg.Enabled = value
	}
	cfg.CronExpr = strings.TrimSpace(stringValue(raw, "cron_expr", "cron", "expression"))
	if tz := strings.TrimSpace(stringValue(raw, "timezone", "time_zone")); tz != "" {
		cfg.Timezone = tz
	}
	if err := validateTaskScheduleConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateTaskScheduleConfig(cfg *taskScheduleConfig) error {
	if cfg == nil {
		return nil
	}
	if strings.TrimSpace(cfg.Timezone) == "" {
		cfg.Timezone = defaultTaskScheduleTimezone
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return fmt.Errorf("%w: invalid timezone %q", ErrInvalidTaskSchedule, cfg.Timezone)
	}
	if !cfg.Enabled {
		if strings.TrimSpace(cfg.CronExpr) == "" {
			return nil
		}
		if err := schedulex.Validate(cfg.CronExpr); err != nil {
			return fmt.Errorf("%w: invalid cron expression: %v", ErrInvalidTaskSchedule, err)
		}
		return nil
	}
	if strings.TrimSpace(cfg.CronExpr) == "" {
		return fmt.Errorf("%w: cron expression is required when schedule is enabled", ErrInvalidTaskSchedule)
	}
	if err := schedulex.Validate(cfg.CronExpr); err != nil {
		return fmt.Errorf("%w: invalid cron expression: %v", ErrInvalidTaskSchedule, err)
	}
	return nil
}

func (cfg *taskScheduleConfig) matches(now time.Time) (bool, time.Time, time.Time, error) {
	if cfg == nil || !cfg.Enabled {
		return false, time.Time{}, time.Time{}, nil
	}
	return schedulex.MatchMinuteWindow(cfg.CronExpr, now, cfg.Timezone)
}

func validateTaskDefinition(definition JSONMap) error {
	if err := validateVariableKeyConflicts(extractDefinitionVariables(definition, "custom_variables")); err != nil {
		return err
	}
	_, err := parseTaskSchedule(definition)
	return err
}

func triggerSourceForRunType(runType RunType) string {
	switch runType {
	case RunTypeSchedule:
		return scheduleTriggerSource
	default:
		return "manual"
	}
}

func taskFromVersionSnapshot(task *Task, version *TaskVersion) *Task {
	if task == nil || version == nil {
		return nil
	}
	return &Task{
		ID:             task.ID,
		ParentID:       task.ParentID,
		NodeType:       task.NodeType,
		Name:           version.NameSnapshot,
		Description:    version.DescriptionSnapshot,
		ClusterID:      version.ClusterIDSnapshot,
		EngineVersion:  version.EngineVersionSnapshot,
		Mode:           version.ModeSnapshot,
		Status:         TaskStatusPublished,
		ContentFormat:  version.ContentFormatSnapshot,
		Content:        version.ContentSnapshot,
		JobName:        version.JobNameSnapshot,
		Definition:     cloneJSONMap(version.DefinitionSnapshot),
		SortOrder:      task.SortOrder,
		CurrentVersion: version.Version,
		CreatedBy:      task.CreatedBy,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
}

const defaultScheduleMisfireGrace = 10 * time.Minute

// scheduleRuntimeState 记录调度器上次检查到的分钟，用于补跑错过的触发。
// scheduleRuntimeState tracks the last evaluated minute so missed fires can be caught up.
type scheduleRuntimeState struct {
	mu        stdsync.Mutex
	lastCheck time.Time
	// 测试覆盖项；nil/0 时读取全局配置。/ Test overrides; nil/0 falls back to global config.
	catchupOverride *bool
	graceOverride   time.Duration
}

func (s *Service) scheduleMisfireCatchupEnabled() bool {
	if s.schedule.catchupOverride != nil {
		return *s.schedule.catchupOverride
	}
	if config.Config != nil && config.Config.Sync.ScheduleMisfireCatchupEnabled != nil {
		return *config.Config.Sync.ScheduleMisfireCatchupEnabled
	}
	return true
}

func (s *Service) scheduleMisfireGrace() time.Duration {
	if s.schedule.graceOverride > 0 {
		return s.schedule.graceOverride
	}
	if config.Config != nil && config.Config.Sync.ScheduleMisfireGraceMinutes > 0 {
		return time.Duration(config.Config.Sync.ScheduleMisfireGraceMinutes) * time.Minute
	}
	return defaultScheduleMisfireGrace
}

func (s *Service) StartTaskScheduleRuntime(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	s.schedule.mu.Lock()
	s.schedule.lastCheck = time.Now().Truncate(time.Minute)
	s.schedule.mu.Unlock()
	go func() {
		// 对齐到整分钟 +1s，让 tick 落在 cron 分钟内的固定位置。
		// Align to minute boundary + 1s so ticks land at a stable spot inside each cron minute.
		now := time.Now()
		align := time.NewTimer(now.Truncate(time.Minute).Add(time.Minute + time.Second).Sub(now))
		select {
		case <-ctx.Done():
			align.Stop()
			return
		case first := <-align.C:
			if err := s.triggerScheduledTasks(ctx, first); err != nil {
				log.Printf("[SyncSchedule] tick failed: %v", err)
			}
		}
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := s.triggerScheduledTasks(ctx, now); err != nil {
					log.Printf("[SyncSchedule] tick failed: %v", err)
				}
			}
		}
	}()
}

// scheduleMinutesToEvaluate 返回本次 tick 需要检查的分钟（升序）。开启补跑时覆盖 (last, now]，
// 并截断到宽限期内；关闭时只检查当前分钟（旧行为）。
// scheduleMinutesToEvaluate returns minutes to evaluate (ascending). With catch-up it covers
// (last, now] capped by the grace window; without it only the current minute (legacy behavior).
func (s *Service) scheduleMinutesToEvaluate(now time.Time) []time.Time {
	current := now.Truncate(time.Minute)
	s.schedule.mu.Lock()
	last := s.schedule.lastCheck
	if current.After(last) {
		s.schedule.lastCheck = current
	}
	s.schedule.mu.Unlock()

	if !s.scheduleMisfireCatchupEnabled() || last.IsZero() || !current.After(last) {
		return []time.Time{current}
	}
	from := last.Add(time.Minute)
	if earliest := current.Add(-s.scheduleMisfireGrace()); from.Before(earliest) {
		skipped := int(earliest.Sub(from) / time.Minute)
		log.Printf("[SyncSchedule] scheduler stalled from %s to %s; %d missed minute(s) beyond grace %s are skipped",
			last.Format(time.RFC3339), current.Format(time.RFC3339), skipped, s.scheduleMisfireGrace())
		from = earliest
	}
	minutes := make([]time.Time, 0, int(current.Sub(from)/time.Minute)+1)
	for m := from; !m.After(current); m = m.Add(time.Minute) {
		minutes = append(minutes, m)
	}
	return minutes
}

func (s *Service) triggerScheduledTasks(ctx context.Context, now time.Time) error {
	minutes := s.scheduleMinutesToEvaluate(now)
	current := minutes[len(minutes)-1]
	tasks, err := s.repo.ListAllTasks(ctx)
	if err != nil {
		return err
	}
	for _, item := range tasks {
		if item == nil {
			continue
		}
		s.applyTaskDefaults(item)
		if item.NodeType != TaskNodeTypeFile || item.CurrentVersion <= 0 || item.Status != TaskStatusPublished {
			continue
		}
		cfg, err := parseTaskSchedule(item.Definition)
		if err != nil {
			log.Printf("[SyncSchedule] task %d schedule ignored: %v", item.ID, err)
			continue
		}
		// 多个错过分钟都命中时只补跑最近的一次，避免停顿恢复后集中触发。
		// When several missed minutes match, fire only the latest to avoid a burst after a stall.
		var fireAt time.Time
		for i := len(minutes) - 1; i >= 0; i-- {
			matched, windowStart, _, matchErr := cfg.matches(minutes[i])
			if matchErr != nil {
				log.Printf("[SyncSchedule] task %d schedule invalid: %v", item.ID, matchErr)
				break
			}
			if matched {
				fireAt = windowStart
				break
			}
		}
		if fireAt.IsZero() {
			continue
		}
		// 去重窗口从命中分钟一直到当前分钟结束：迟到补跑的实例创建于当前分钟。
		// Dedup window spans the matched minute through the end of the current minute,
		// because a late catch-up instance is created in the current minute.
		exists, err := s.repo.HasScheduledJobInstanceInWindow(ctx, item.ID, fireAt, current.Add(time.Minute))
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if !fireAt.Equal(current) {
			log.Printf("[SyncSchedule] task %d catching up missed fire at %s", item.ID, fireAt.Format(time.RFC3339))
		}
		if err := s.submitScheduledTask(ctx, item); err != nil {
			log.Printf("[SyncSchedule] task %d submit failed: %v", item.ID, err)
		}
	}
	return nil
}

func (s *Service) submitScheduledTask(ctx context.Context, task *Task) error {
	version, err := s.repo.GetTaskVersionByVersion(ctx, task.ID, task.CurrentVersion)
	if err != nil {
		return err
	}
	scheduledTask := taskFromVersionSnapshot(task, version)
	if scheduledTask == nil {
		return ErrTaskNotPublished
	}
	if err := validateTaskDefinition(scheduledTask.Definition); err != nil {
		return err
	}
	platformJobID := s.nextJobID()
	if taskExecutionMode(scheduledTask) == "local" {
		body, format, jobName, buildErr := s.buildSubmitPayload(ctx, scheduledTask, buildTaskVariableRuntime(scheduledTask, platformJobID))
		if buildErr != nil {
			return buildErr
		}
		_, err = s.submitLocalTaskInstance(ctx, scheduledTask, 0, RunTypeSchedule, platformJobID, body, format, jobName)
		return err
	}
	_, err = s.submitTaskInstance(ctx, scheduledTask, 0, RunTypeSchedule, platformJobID, JobRestorePlan{}, nil)
	return err
}
