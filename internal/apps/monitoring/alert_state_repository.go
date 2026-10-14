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

package monitoring

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// GetAlertStateBySourceKey returns one unified alert state by source key.
// GetAlertStateBySourceKey 根据 source key 返回统一告警状态。
func (r *Repository) GetAlertStateBySourceKey(ctx context.Context, sourceKey string) (*AlertState, error) {
	var state AlertState
	if err := r.db.WithContext(ctx).Where("source_key = ?", sourceKey).First(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

// ListAlertStatesBySourceKeys returns unified alert states mapped by source_key.
// ListAlertStatesBySourceKeys 根据 source_key 列表返回统一告警状态映射。
func (r *Repository) ListAlertStatesBySourceKeys(ctx context.Context, sourceKeys []string) (map[string]*AlertState, error) {
	result := make(map[string]*AlertState)
	if len(sourceKeys) == 0 {
		return result, nil
	}

	var states []*AlertState
	if err := r.db.WithContext(ctx).
		Where("source_key IN ?", sourceKeys).
		Find(&states).Error; err != nil {
		return nil, err
	}

	for _, state := range states {
		if state == nil {
			continue
		}
		result[state.SourceKey] = state
	}
	return result, nil
}

// SaveAlertState creates or updates one unified alert state.
// SaveAlertState 创建或更新一条统一告警状态。
func (r *Repository) SaveAlertState(ctx context.Context, state *AlertState) error {
	if state == nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("source_key = ?", state.SourceKey).
		Assign(map[string]interface{}{
			"source_type":     state.SourceType,
			"cluster_id":      state.ClusterID,
			"handling_status": state.HandlingStatus,
			"acknowledged_by": state.AcknowledgedBy,
			"acknowledged_at": state.AcknowledgedAt,
			"silenced_by":     state.SilencedBy,
			"silenced_until":  state.SilencedUntil,
			"closed_by":       state.ClosedBy,
			"closed_at":       state.ClosedAt,
			"note":            state.Note,
			"updated_at":      timex.Now(),
		}).
		FirstOrCreate(&AlertState{SourceKey: state.SourceKey}).Error
}

// GetRemoteAlertByFingerprintAndStartsAt returns one remote alert record by unique key.
// GetRemoteAlertByFingerprintAndStartsAt 根据唯一键返回一条远程告警记录。
func (r *Repository) GetRemoteAlertByFingerprintAndStartsAt(ctx context.Context, fingerprint string, startsAt int64) (*RemoteAlertRecord, error) {
	var record RemoteAlertRecord
	if err := r.db.WithContext(ctx).
		Where("fingerprint = ? AND starts_at = ?", fingerprint, startsAt).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}
