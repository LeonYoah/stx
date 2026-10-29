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
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetEngineEventChannel 读取集群的事件通道记录，不存在时返回 ErrEventChannelNotFound。
// GetEngineEventChannel reads a cluster event channel record and returns ErrEventChannelNotFound when absent.
func (r *Repository) GetEngineEventChannel(ctx context.Context, clusterID uint) (*EngineEventChannel, error) {
	var channel EngineEventChannel
	if err := r.db.WithContext(ctx).First(&channel, "cluster_id = ?", clusterID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEventChannelNotFound
		}
		return nil, err
	}
	return &channel, nil
}

// ListEngineEventChannels 列出全部事件通道记录。
// ListEngineEventChannels lists all event channel records.
func (r *Repository) ListEngineEventChannels(ctx context.Context) ([]*EngineEventChannel, error) {
	var channels []*EngineEventChannel
	if err := r.db.WithContext(ctx).Order("cluster_id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

// SaveEngineEventChannel 按 cluster_id 插入或整体更新通道记录。
// SaveEngineEventChannel inserts or fully updates a channel record keyed by cluster_id.
func (r *Repository) SaveEngineEventChannel(ctx context.Context, channel *EngineEventChannel) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cluster_id"}},
		UpdateAll: true,
	}).Create(channel).Error
}

// UpdateEngineEventChannelStats 原子累加计数并更新时间字段，避免并发批次互相覆盖。
// UpdateEngineEventChannelStats atomically adds counters and updates timestamps so concurrent batches do not overwrite each other.
func (r *Repository) UpdateEngineEventChannelStats(ctx context.Context, clusterID uint, counters map[string]int64, fields map[string]any) error {
	updates := map[string]any{}
	for column, delta := range counters {
		if delta == 0 {
			continue
		}
		updates[column] = gorm.Expr(column+" + ?", delta)
	}
	for column, value := range fields {
		updates[column] = value
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&EngineEventChannel{}).Where("cluster_id = ?", clusterID).Updates(updates).Error
}

// ExistingEngineEventDedupeKeys 返回已入库的去重键集合，用于判断一批事件里哪些是新的。
// ExistingEngineEventDedupeKeys returns which dedupe keys are already stored so a batch can tell new events apart.
func (r *Repository) ExistingEngineEventDedupeKeys(ctx context.Context, keys []string) (map[string]bool, error) {
	existing := map[string]bool{}
	for start := 0; start < len(keys); start += 500 {
		end := start + 500
		if end > len(keys) {
			end = len(keys)
		}
		var found []string
		if err := r.db.WithContext(ctx).Model(&EngineEvent{}).Where("dedupe_key IN ?", keys[start:end]).Pluck("dedupe_key", &found).Error; err != nil {
			return nil, err
		}
		for _, key := range found {
			existing[key] = true
		}
	}
	return existing, nil
}

// BumpEngineEventDelayIfGreater 在同一分钟桶已有 READER_MESSAGE_DELAYED 时，若新样本延迟更大则更新 payload.delay_ms，
// 避免跨批次入库时只保留第一批、界面时间线系统性低估。返回实际更新的行数。
// BumpEngineEventDelayIfGreater updates payload.delay_ms when an existing READER_MESSAGE_DELAYED minute bucket has a
// smaller delay, so later batches do not leave the timeline stuck on the first sample. Returns how many rows changed.
func (r *Repository) BumpEngineEventDelayIfGreater(ctx context.Context, events []*EngineEvent) (int, error) {
	updated := 0
	for _, event := range events {
		if event == nil || event.EventType != EngineEventTypeMessageDelayed || event.DedupeKey == "" {
			continue
		}
		newDelay := int64Field(event.Payload, "delay_ms")
		if newDelay <= 0 {
			continue
		}
		var existing EngineEvent
		if err := r.db.WithContext(ctx).Where("dedupe_key = ?", event.DedupeKey).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return updated, err
		}
		oldDelay := int64Field(existing.Payload, "delay_ms")
		if newDelay <= oldDelay {
			continue
		}
		payload := existing.Payload
		if payload == nil {
			payload = JSONMap{}
		}
		payload["delay_ms"] = newDelay
		if err := r.db.WithContext(ctx).Model(&existing).Update("payload", payload).Error; err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

// InsertEngineEventsIgnoreDuplicates 批量写入事件，去重键冲突的行直接忽略（并发与重放安全）。
// InsertEngineEventsIgnoreDuplicates bulk-inserts events and ignores rows whose dedupe key already exists
// (safe under concurrency and replays).
func (r *Repository) InsertEngineEventsIgnoreDuplicates(ctx context.Context, events []*EngineEvent) error {
	if len(events) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "dedupe_key"}},
		DoNothing: true,
	}).CreateInBatches(events, 200).Error
}

// ListEngineEventsByJob 按事件时间倒序分页列出某作业的事件，可按类型过滤。
// ListEngineEventsByJob lists one job's events by event time descending with paging and optional type filters.
func (r *Repository) ListEngineEventsByJob(ctx context.Context, jobInstanceID uint, eventTypes []string, page, size int) ([]*EngineEvent, int64, error) {
	query := r.db.WithContext(ctx).Model(&EngineEvent{}).Where("job_instance_id = ?", jobInstanceID)
	if len(eventTypes) > 0 {
		query = query.Where("event_type IN ?", eventTypes)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 50
	}
	var events []*EngineEvent
	if err := query.Order("event_time_ms DESC").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// PurgeEngineEventsBefore 删除早于 cutoff 收到的事件。
// PurgeEngineEventsBefore deletes events received before cutoff.
func (r *Repository) PurgeEngineEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("received_at < ?", cutoff).Delete(&EngineEvent{})
	return result.RowsAffected, result.Error
}

// FindLatestJobInstanceByEngineJobID 按引擎作业 ID 找最新的作业实例（恢复作业会复用同一个引擎作业 ID）。
// FindLatestJobInstanceByEngineJobID finds the newest job instance for an engine job ID (recovered jobs reuse the
// engine job ID).
func (r *Repository) FindLatestJobInstanceByEngineJobID(ctx context.Context, engineJobID string) (*JobInstance, error) {
	var instance JobInstance
	if err := r.db.WithContext(ctx).Where("engine_job_id = ?", engineJobID).Order("id DESC").First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobInstanceNotFound
		}
		return nil, err
	}
	return &instance, nil
}
