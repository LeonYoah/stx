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
	"time"

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

func (s *Service) StartTaskScheduleRuntime(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	go func() {
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

func (s *Service) triggerScheduledTasks(ctx context.Context, now time.Time) error {
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
		matched, windowStart, windowEnd, err := cfg.matches(now)
		if err != nil {
			log.Printf("[SyncSchedule] task %d schedule invalid: %v", item.ID, err)
			continue
		}
		if !matched {
			continue
		}
		exists, err := s.repo.HasScheduledJobInstanceInWindow(ctx, item.ID, windowStart.UTC(), windowEnd.UTC())
		if err != nil {
			return err
		}
		if exists {
			continue
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
	_, err = s.submitTaskInstance(ctx, scheduledTask, 0, RunTypeSchedule, platformJobID, false, nil)
	return err
}
