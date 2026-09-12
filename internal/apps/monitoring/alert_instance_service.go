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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/monitor"
)

type parsedAlertSourceKey struct {
	SourceType  AlertSourceType
	SourceKey   string
	EventID     uint
	Fingerprint string
	StartsAt    int64
}

// ListAlertInstances returns unified alert instances merged from local and remote sources.
// ListAlertInstances 返回本地与远程来源合并后的统一告警实例列表。
func (s *Service) ListAlertInstances(ctx context.Context, filter *AlertInstanceFilter) (*AlertInstanceListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if filter == nil {
		filter = &AlertInstanceFilter{}
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	now := time.Now().UTC()
	items := make([]*AlertInstance, 0, 32)

	includeLocal := filter.SourceType == "" || filter.SourceType == AlertSourceTypeLocalProcessEvent
	includeRemote := filter.SourceType == "" || filter.SourceType == AlertSourceTypeRemoteAlertmanager

	if includeLocal {
		localItems, err := s.buildLocalAlertInstances(ctx, filter, now)
		if err != nil {
			return nil, err
		}
		items = append(items, localItems...)
	}
	if includeRemote {
		remoteItems, err := s.buildRemoteAlertInstances(ctx, filter, now)
		if err != nil {
			return nil, err
		}
		items = append(items, remoteItems...)
	}

	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]

		leftWeight := alertDisplayStatusWeight(left.Status)
		rightWeight := alertDisplayStatusWeight(right.Status)
		if leftWeight != rightWeight {
			return leftWeight < rightWeight
		}

		leftCritical := strings.EqualFold(string(left.Severity), string(AlertSeverityCritical))
		rightCritical := strings.EqualFold(string(right.Severity), string(AlertSeverityCritical))
		if leftCritical != rightCritical {
			return leftCritical
		}

		leftSilenced := left.HandlingStatus == AlertHandlingStatusSilenced
		rightSilenced := right.HandlingStatus == AlertHandlingStatusSilenced
		if leftSilenced != rightSilenced {
			return !leftSilenced
		}

		if !left.LastSeenAt.Equal(right.LastSeenAt) {
			return left.LastSeenAt.After(right.LastSeenAt)
		}
		return left.AlertID > right.AlertID
	})

	stats := &AlertInstanceStats{}
	for _, item := range items {
		if item == nil {
			continue
		}
		switch item.Status {
		case AlertDisplayStatusResolved:
			stats.Resolved++
		case AlertDisplayStatusClosed:
			stats.Closed++
		default:
			stats.Firing++
		}
	}

	total := int64(len(items))
	start := (filter.Page - 1) * filter.PageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + filter.PageSize
	if end > len(items) {
		end = len(items)
	}

	pageItems := make([]*AlertInstance, 0, end-start)
	pageItems = append(pageItems, items[start:end]...)

	return &AlertInstanceListData{
		GeneratedAt: now,
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Total:       total,
		Stats:       stats,
		Alerts:      pageItems,
	}, nil
}

// AcknowledgeAlertInstance acknowledges one unified alert instance.
// AcknowledgeAlertInstance 确认一条统一告警实例。
func (s *Service) AcknowledgeAlertInstance(ctx context.Context, alertID, operator, note string) (*AlertInstanceActionResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if strings.TrimSpace(alertID) == "" {
		return nil, ErrAlertInstanceInvalidID
	}
	if strings.TrimSpace(operator) == "" {
		operator = "unknown"
	}

	parsed, err := parseAlertSourceKey(alertID)
	if err != nil {
		return nil, ErrAlertInstanceInvalidID
	}
	existingState, err := s.repo.GetAlertStateBySourceKey(ctx, parsed.SourceKey)
	if err != nil {
		return nil, err
	}
	if isAlertStateClosed(existingState, time.Now().UTC()) {
		return nil, ErrAlertInstanceAlreadyClosed
	}

	now := time.Now().UTC()
	switch parsed.SourceType {
	case AlertSourceTypeLocalProcessEvent:
		event, err := s.monitorService.GetEvent(ctx, parsed.EventID)
		if err != nil {
			if errors.Is(err, monitor.ErrEventNotFound) {
				return nil, ErrAlertInstanceNotFound
			}
			return nil, err
		}
		if !isAlertableEventType(event.EventType) {
			return nil, ErrAlertInstanceNotFound
		}
		state, _, err := s.persistLocalAlertHandlingState(
			ctx,
			event,
			AlertHandlingStatusAcknowledged,
			operator,
			nil,
			note,
		)
		if err != nil {
			return nil, err
		}
		return toAlertInstanceActionResult(state), nil
	case AlertSourceTypeRemoteAlertmanager:
		record, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, parsed.Fingerprint, parsed.StartsAt)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, ErrAlertInstanceNotFound
		}
		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     AlertSourceTypeRemoteAlertmanager,
			SourceKey:      parsed.SourceKey,
			ClusterID:      strings.TrimSpace(record.ClusterID),
			HandlingStatus: AlertHandlingStatusAcknowledged,
			AcknowledgedBy: operator,
			AcknowledgedAt: &now,
			SilencedBy:     "",
			SilencedUntil:  nil,
			Note:           note,
		})
		if err != nil {
			return nil, err
		}
		return toAlertInstanceActionResult(state), nil
	default:
		return nil, ErrAlertInstanceInvalidID
	}
}

// SilenceAlertInstance silences one unified alert instance for a duration in minutes.
// SilenceAlertInstance 将一条统一告警实例静默指定分钟数。
func (s *Service) SilenceAlertInstance(ctx context.Context, alertID, operator string, durationMinutes int, note string) (*AlertInstanceActionResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if strings.TrimSpace(alertID) == "" {
		return nil, ErrAlertInstanceInvalidID
	}
	if durationMinutes < 1 || durationMinutes > 7*24*60 {
		return nil, fmt.Errorf("duration_minutes must be between 1 and 10080")
	}
	if strings.TrimSpace(operator) == "" {
		operator = "unknown"
	}

	parsed, err := parseAlertSourceKey(alertID)
	if err != nil {
		return nil, ErrAlertInstanceInvalidID
	}
	existingState, err := s.repo.GetAlertStateBySourceKey(ctx, parsed.SourceKey)
	if err != nil {
		return nil, err
	}
	if isAlertStateClosed(existingState, time.Now().UTC()) {
		return nil, ErrAlertInstanceAlreadyClosed
	}

	silencedUntil := time.Now().UTC().Add(time.Duration(durationMinutes) * time.Minute)
	switch parsed.SourceType {
	case AlertSourceTypeLocalProcessEvent:
		event, err := s.monitorService.GetEvent(ctx, parsed.EventID)
		if err != nil {
			if errors.Is(err, monitor.ErrEventNotFound) {
				return nil, ErrAlertInstanceNotFound
			}
			return nil, err
		}
		if !isAlertableEventType(event.EventType) {
			return nil, ErrAlertInstanceNotFound
		}
		state, _, err := s.persistLocalAlertHandlingState(
			ctx,
			event,
			AlertHandlingStatusSilenced,
			operator,
			&silencedUntil,
			note,
		)
		if err != nil {
			return nil, err
		}
		return toAlertInstanceActionResult(state), nil
	case AlertSourceTypeRemoteAlertmanager:
		record, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, parsed.Fingerprint, parsed.StartsAt)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, ErrAlertInstanceNotFound
		}
		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     AlertSourceTypeRemoteAlertmanager,
			SourceKey:      parsed.SourceKey,
			ClusterID:      strings.TrimSpace(record.ClusterID),
			HandlingStatus: AlertHandlingStatusSilenced,
			SilencedBy:     operator,
			SilencedUntil:  &silencedUntil,
			Note:           note,
		})
		if err != nil {
			return nil, err
		}
		return toAlertInstanceActionResult(state), nil
	default:
		return nil, ErrAlertInstanceInvalidID
	}
}

// CloseAlertInstance manually closes one alert incident and suppresses follow-up notifications for the same incident.
// CloseAlertInstance 人工关闭一条告警事件，并抑制该事件后续通知。
func (s *Service) CloseAlertInstance(ctx context.Context, alertID, operator, note string) (*AlertInstanceActionResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if strings.TrimSpace(alertID) == "" {
		return nil, ErrAlertInstanceInvalidID
	}
	if strings.TrimSpace(operator) == "" {
		operator = "unknown"
	}

	parsed, err := parseAlertSourceKey(alertID)
	if err != nil {
		return nil, ErrAlertInstanceInvalidID
	}

	existingState, err := s.repo.GetAlertStateBySourceKey(ctx, parsed.SourceKey)
	if err != nil {
		return nil, err
	}
	if isAlertStateClosed(existingState, time.Now().UTC()) {
		return toAlertInstanceActionResult(existingState), nil
	}

	now := time.Now().UTC()
	switch parsed.SourceType {
	case AlertSourceTypeLocalProcessEvent:
		if s.monitorService == nil {
			return nil, ErrAlertInstanceNotFound
		}
		event, err := s.monitorService.GetEvent(ctx, parsed.EventID)
		if err != nil {
			if errors.Is(err, monitor.ErrEventNotFound) {
				return nil, ErrAlertInstanceNotFound
			}
			return nil, err
		}
		if !isAlertableEventType(event.EventType) {
			return nil, ErrAlertInstanceNotFound
		}

		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     AlertSourceTypeLocalProcessEvent,
			SourceKey:      parsed.SourceKey,
			ClusterID:      strconv.FormatUint(uint64(event.ClusterID), 10),
			HandlingStatus: AlertHandlingStatusClosed,
			ClosedBy:       operator,
			ClosedAt:       &now,
			Note:           note,
		})
		if err != nil {
			return nil, err
		}
		return toAlertInstanceActionResult(state), nil
	case AlertSourceTypeRemoteAlertmanager:
		record, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, parsed.Fingerprint, parsed.StartsAt)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, ErrAlertInstanceNotFound
		}

		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     AlertSourceTypeRemoteAlertmanager,
			SourceKey:      parsed.SourceKey,
			ClusterID:      strings.TrimSpace(record.ClusterID),
			HandlingStatus: AlertHandlingStatusClosed,
			ClosedBy:       operator,
			ClosedAt:       &now,
			Note:           note,
		})
		if err != nil {
			return nil, err
		}
		return toAlertInstanceActionResult(state), nil
	default:
		return nil, ErrAlertInstanceInvalidID
	}
}

func (s *Service) buildLocalAlertInstances(ctx context.Context, filter *AlertInstanceFilter, now time.Time) ([]*AlertInstance, error) {
	localFilter := &AlertEventQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	}
	if filter.ClusterID != "" {
		clusterID, err := strconv.ParseUint(strings.TrimSpace(filter.ClusterID), 10, 32)
		if err != nil {
			return []*AlertInstance{}, nil
		}
		localFilter.ClusterID = uint(clusterID)
	}

	rows, _, err := s.repo.ListCriticalEvents(ctx, localFilter)
	if err != nil {
		return nil, err
	}

	eventIDs := make([]uint, 0, len(rows))
	sourceKeys := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		eventIDs = append(eventIDs, row.ID)
		sourceKeys = append(sourceKeys, buildLocalAlertSourceKey(row.ID))
	}

	legacyStateMap, err := s.repo.ListEventStatesByEventIDs(ctx, eventIDs)
	if err != nil {
		return nil, err
	}
	stateMap, err := s.repo.ListAlertStatesBySourceKeys(ctx, sourceKeys)
	if err != nil {
		return nil, err
	}

	ruleCache := make(map[uint]map[string]*AlertRule)
	items := make([]*AlertInstance, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}

		rules, err := s.getClusterRuleMap(ctx, row.ClusterID, ruleCache)
		if err != nil {
			return nil, err
		}

		ruleKey := eventTypeToRuleKey(row.EventType)
		rule := rules[ruleKey]
		if rule == nil {
			rule = defaultRuleByKey(row.ClusterID, ruleKey)
		}
		if rule == nil || !rule.Enabled {
			continue
		}
		if filter.Severity != "" && !strings.EqualFold(string(rule.Severity), string(filter.Severity)) {
			continue
		}

		sourceKey := buildLocalAlertSourceKey(row.ID)
		state := stateMap[sourceKey]
		if state == nil {
			state = alertStateFromLegacyState(sourceKey, legacyStateMap[row.ID])
		}

		lifecycleStatus, resolvedAt, err := s.resolveLocalAlertLifecycle(ctx, row)
		if err != nil {
			return nil, err
		}
		if filter.LifecycleStatus != "" && lifecycleStatus != filter.LifecycleStatus {
			continue
		}

		handlingStatus := resolveAlertHandlingStatus(state, now)
		if filter.HandlingStatus != "" && handlingStatus != filter.HandlingStatus {
			continue
		}
		displayStatus := resolveAlertDisplayStatus(lifecycleStatus, handlingStatus)
		if filter.Status != "" && displayStatus != filter.Status {
			continue
		}

		lastSeenAt := row.CreatedAt.UTC()
		if resolvedAt != nil {
			lastSeenAt = resolvedAt.UTC()
		}

		item := &AlertInstance{
			AlertID:         sourceKey,
			SourceType:      AlertSourceTypeLocalProcessEvent,
			ClusterID:       strconv.FormatUint(uint64(row.ClusterID), 10),
			ClusterName:     strings.TrimSpace(row.ClusterName),
			Severity:        rule.Severity,
			AlertName:       strings.TrimSpace(rule.RuleName),
			RuleKey:         strings.TrimSpace(rule.RuleKey),
			Summary:         buildLocalAlertSummary(row, rule),
			Description:     strings.TrimSpace(row.Details),
			Status:          displayStatus,
			LifecycleStatus: lifecycleStatus,
			HandlingStatus:  handlingStatus,
			CreatedAt:       row.CreatedAt.UTC(),
			FiringAt:        row.CreatedAt.UTC(),
			LastSeenAt:      lastSeenAt,
			ResolvedAt:      resolvedAt,
			SourceRef: &AlertInstanceSourceRef{
				EventID:     row.ID,
				EventType:   string(row.EventType),
				ProcessName: strings.TrimSpace(row.ProcessName),
				Hostname:    strings.TrimSpace(row.Hostname),
			},
		}
		applyAlertStateToInstance(item, state)
		items = append(items, item)
	}

	return items, nil
}

func (s *Service) resolveLocalAlertLifecycle(ctx context.Context, row *AlertEventSource) (AlertLifecycleStatus, *time.Time, error) {
	if row == nil {
		return AlertLifecycleStatusFiring, nil, nil
	}
	if isInstantResolvedLocalEventType(row.EventType) {
		resolvedAt := row.CreatedAt.UTC()
		return AlertLifecycleStatusResolved, &resolvedAt, nil
	}
	if s.monitorService == nil {
		return AlertLifecycleStatusFiring, nil, nil
	}

	switch row.EventType {
	case monitor.EventTypeNodeOffline:
		recovered, recoveryEvent, err := s.monitorService.HasNodeEventAfter(ctx, row.NodeID, row.CreatedAt, []monitor.ProcessEventType{
			monitor.EventTypeNodeRecovered,
		})
		if err != nil {
			return AlertLifecycleStatusFiring, nil, err
		}
		if !recovered || recoveryEvent == nil {
			return AlertLifecycleStatusFiring, nil, nil
		}

		resolvedAt := recoveryEvent.CreatedAt.UTC()
		return AlertLifecycleStatusResolved, &resolvedAt, nil
	case monitor.EventTypeCrashed,
		monitor.EventTypeRestartFailed,
		monitor.EventTypeRestartLimitReached:
		recovered, recoveryEvent, err := s.monitorService.HasNodeEventAfter(ctx, row.NodeID, row.CreatedAt, []monitor.ProcessEventType{
			monitor.EventTypeStarted,
			monitor.EventTypeRestarted,
			monitor.EventTypeNodeRecovered,
		})
		if err != nil {
			return AlertLifecycleStatusFiring, nil, err
		}
		if !recovered || recoveryEvent == nil {
			return AlertLifecycleStatusFiring, nil, nil
		}

		resolvedAt := recoveryEvent.CreatedAt.UTC()
		return AlertLifecycleStatusResolved, &resolvedAt, nil
	default:
		return AlertLifecycleStatusFiring, nil, nil
	}
}

func (s *Service) buildRemoteAlertInstances(ctx context.Context, filter *AlertInstanceFilter, now time.Time) ([]*AlertInstance, error) {
	remoteFilter := &RemoteAlertFilter{
		ClusterID: strings.TrimSpace(filter.ClusterID),
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	}

	rows, _, err := s.repo.ListRemoteAlerts(ctx, remoteFilter)
	if err != nil {
		return nil, err
	}

	sourceKeys := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		sourceKeys = append(sourceKeys, buildRemoteAlertSourceKey(row.Fingerprint, row.StartsAt))
	}
	stateMap, err := s.repo.ListAlertStatesBySourceKeys(ctx, sourceKeys)
	if err != nil {
		return nil, err
	}

	items := make([]*AlertInstance, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}

		if filter.Severity != "" && !strings.EqualFold(strings.TrimSpace(row.Severity), string(filter.Severity)) {
			continue
		}

		lifecycleStatus := resolveRemoteLifecycleStatus(row.Status)
		if filter.LifecycleStatus != "" && lifecycleStatus != filter.LifecycleStatus {
			continue
		}

		sourceKey := buildRemoteAlertSourceKey(row.Fingerprint, row.StartsAt)
		state := stateMap[sourceKey]
		handlingStatus := resolveAlertHandlingStatus(state, now)
		if filter.HandlingStatus != "" && handlingStatus != filter.HandlingStatus {
			continue
		}
		displayStatus := resolveAlertDisplayStatus(lifecycleStatus, handlingStatus)
		if filter.Status != "" && displayStatus != filter.Status {
			continue
		}

		firingAt := time.Unix(row.StartsAt, 0).UTC()
		lastSeenAt := row.LastReceivedAt.UTC()
		if lastSeenAt.IsZero() {
			lastSeenAt = firingAt
		}

		item := &AlertInstance{
			AlertID:         sourceKey,
			SourceType:      AlertSourceTypeRemoteAlertmanager,
			ClusterID:       strings.TrimSpace(row.ClusterID),
			ClusterName:     strings.TrimSpace(row.ClusterName),
			Severity:        AlertSeverity(strings.TrimSpace(row.Severity)),
			AlertName:       strings.TrimSpace(row.AlertName),
			Summary:         strings.TrimSpace(row.Summary),
			Description:     strings.TrimSpace(row.Description),
			Status:          displayStatus,
			LifecycleStatus: lifecycleStatus,
			HandlingStatus:  handlingStatus,
			CreatedAt:       row.CreatedAt.UTC(),
			FiringAt:        firingAt,
			LastSeenAt:      lastSeenAt,
			ResolvedAt:      toUTCTimePointer(row.ResolvedAt),
			SourceRef: &AlertInstanceSourceRef{
				Fingerprint: strings.TrimSpace(row.Fingerprint),
				Receiver:    strings.TrimSpace(row.Receiver),
				Env:         strings.TrimSpace(row.Env),
			},
		}
		applyAlertStateToInstance(item, state)
		items = append(items, item)
	}

	return items, nil
}

func (s *Service) persistLocalAlertHandlingState(
	ctx context.Context,
	event *monitor.ProcessEvent,
	handlingStatus AlertHandlingStatus,
	operator string,
	silencedUntil *time.Time,
	note string,
) (*AlertState, *AlertEventState, error) {
	if event == nil {
		return nil, nil, fmt.Errorf("empty event")
	}
	if strings.TrimSpace(operator) == "" {
		operator = "unknown"
	}

	legacyState, err := s.repo.GetEventStateByEventID(ctx, event.ID)
	if err != nil {
		return nil, nil, err
	}
	if legacyState == nil {
		legacyState = &AlertEventState{EventID: event.ID}
	}

	now := time.Now().UTC()
	legacyState.ClusterID = event.ClusterID
	legacyState.Note = note
	switch handlingStatus {
	case AlertHandlingStatusSilenced:
		legacyState.Status = AlertStatusSilenced
		legacyState.SilencedBy = operator
		legacyState.SilencedUntil = silencedUntil
		legacyState.AcknowledgedBy = ""
		legacyState.AcknowledgedAt = nil
	case AlertHandlingStatusClosed:
		legacyState.Status = AlertStatusAcknowledged
		legacyState.AcknowledgedBy = operator
		legacyState.AcknowledgedAt = &now
		legacyState.SilencedBy = ""
		legacyState.SilencedUntil = nil
	case AlertHandlingStatusAcknowledged:
		legacyState.Status = AlertStatusAcknowledged
		legacyState.AcknowledgedBy = operator
		legacyState.AcknowledgedAt = &now
		legacyState.SilencedBy = ""
		legacyState.SilencedUntil = nil
	default:
		legacyState.Status = AlertStatusFiring
		legacyState.AcknowledgedBy = ""
		legacyState.AcknowledgedAt = nil
		legacyState.SilencedBy = ""
		legacyState.SilencedUntil = nil
	}

	if err := s.repo.SaveEventState(ctx, legacyState); err != nil {
		return nil, nil, err
	}

	state := &AlertState{
		SourceType:     AlertSourceTypeLocalProcessEvent,
		SourceKey:      buildLocalAlertSourceKey(event.ID),
		ClusterID:      strconv.FormatUint(uint64(event.ClusterID), 10),
		HandlingStatus: handlingStatus,
		Note:           note,
	}
	switch handlingStatus {
	case AlertHandlingStatusSilenced:
		state.SilencedBy = operator
		state.SilencedUntil = silencedUntil
	case AlertHandlingStatusClosed:
		state.ClosedBy = operator
		state.ClosedAt = &now
	case AlertHandlingStatusAcknowledged:
		state.AcknowledgedBy = operator
		state.AcknowledgedAt = &now
	}

	state, err = s.upsertAlertState(ctx, state)
	if err != nil {
		return nil, nil, err
	}
	return state, legacyState, nil
}

func (s *Service) upsertAlertState(ctx context.Context, state *AlertState) (*AlertState, error) {
	if state == nil {
		return nil, fmt.Errorf("empty alert state")
	}
	if err := s.repo.SaveAlertState(ctx, state); err != nil {
		return nil, err
	}
	return s.repo.GetAlertStateBySourceKey(ctx, state.SourceKey)
}

func buildLocalAlertSourceKey(eventID uint) string {
	return fmt.Sprintf("local:event:%d", eventID)
}

func buildRemoteAlertSourceKey(fingerprint string, startsAt int64) string {
	return fmt.Sprintf("remote:%s:%d", strings.TrimSpace(fingerprint), startsAt)
}

func parseAlertSourceKey(sourceKey string) (*parsedAlertSourceKey, error) {
	parts := strings.Split(strings.TrimSpace(sourceKey), ":")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid alert id")
	}

	if parts[0] == "local" && parts[1] == "event" {
		eventID, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid alert id")
		}
		return &parsedAlertSourceKey{
			SourceType: AlertSourceTypeLocalProcessEvent,
			SourceKey:  sourceKey,
			EventID:    uint(eventID),
		}, nil
	}

	if parts[0] == "remote" && len(parts) == 3 {
		startsAt, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid alert id")
		}
		return &parsedAlertSourceKey{
			SourceType:  AlertSourceTypeRemoteAlertmanager,
			SourceKey:   sourceKey,
			Fingerprint: parts[1],
			StartsAt:    startsAt,
		}, nil
	}

	return nil, fmt.Errorf("invalid alert id")
}

func resolveRemoteLifecycleStatus(status string) AlertLifecycleStatus {
	if strings.EqualFold(strings.TrimSpace(status), string(AlertLifecycleStatusResolved)) {
		return AlertLifecycleStatusResolved
	}
	return AlertLifecycleStatusFiring
}

func resolveAlertHandlingStatus(state *AlertState, now time.Time) AlertHandlingStatus {
	if state == nil {
		return AlertHandlingStatusPending
	}
	switch state.HandlingStatus {
	case AlertHandlingStatusClosed:
		return AlertHandlingStatusClosed
	case AlertHandlingStatusSilenced:
		if state.SilencedUntil != nil && state.SilencedUntil.After(now) {
			return AlertHandlingStatusSilenced
		}
		return AlertHandlingStatusPending
	case AlertHandlingStatusAcknowledged:
		return AlertHandlingStatusAcknowledged
	default:
		return AlertHandlingStatusPending
	}
}

func alertStateFromLegacyState(sourceKey string, legacy *AlertEventState) *AlertState {
	if legacy == nil {
		return nil
	}
	handlingStatus := AlertHandlingStatusPending
	switch legacy.Status {
	case AlertStatusAcknowledged:
		handlingStatus = AlertHandlingStatusAcknowledged
	case AlertStatusSilenced:
		handlingStatus = AlertHandlingStatusSilenced
	}
	return &AlertState{
		SourceType:     AlertSourceTypeLocalProcessEvent,
		SourceKey:      sourceKey,
		ClusterID:      strconv.FormatUint(uint64(legacy.ClusterID), 10),
		HandlingStatus: handlingStatus,
		AcknowledgedBy: legacy.AcknowledgedBy,
		AcknowledgedAt: legacy.AcknowledgedAt,
		SilencedBy:     legacy.SilencedBy,
		SilencedUntil:  legacy.SilencedUntil,
		Note:           legacy.Note,
		CreatedAt:      legacy.CreatedAt,
		UpdatedAt:      legacy.UpdatedAt,
	}
}

func applyAlertStateToInstance(item *AlertInstance, state *AlertState) {
	if item == nil || state == nil {
		return
	}
	item.AcknowledgedBy = state.AcknowledgedBy
	item.AcknowledgedAt = toUTCTimePointer(state.AcknowledgedAt)
	item.SilencedBy = state.SilencedBy
	item.SilencedUntil = toUTCTimePointer(state.SilencedUntil)
	item.ClosedBy = state.ClosedBy
	item.ClosedAt = toUTCTimePointer(state.ClosedAt)
	item.LatestNote = state.Note
}

func toAlertInstanceActionResult(state *AlertState) *AlertInstanceActionResult {
	if state == nil {
		return nil
	}
	return &AlertInstanceActionResult{
		AlertID:        state.SourceKey,
		Status:         resolveActionDisplayStatus(state),
		HandlingStatus: resolveAlertHandlingStatus(state, time.Now().UTC()),
		AcknowledgedBy: state.AcknowledgedBy,
		AcknowledgedAt: toUTCTimePointer(state.AcknowledgedAt),
		SilencedBy:     state.SilencedBy,
		SilencedUntil:  toUTCTimePointer(state.SilencedUntil),
		ClosedBy:       state.ClosedBy,
		ClosedAt:       toUTCTimePointer(state.ClosedAt),
		LatestNote:     state.Note,
	}
}

func resolveAlertDisplayStatus(lifecycleStatus AlertLifecycleStatus, handlingStatus AlertHandlingStatus) AlertDisplayStatus {
	if handlingStatus == AlertHandlingStatusClosed {
		return AlertDisplayStatusClosed
	}
	if lifecycleStatus == AlertLifecycleStatusResolved {
		return AlertDisplayStatusResolved
	}
	return AlertDisplayStatusFiring
}

func resolveActionDisplayStatus(state *AlertState) AlertDisplayStatus {
	if isAlertStateClosed(state, time.Now().UTC()) {
		return AlertDisplayStatusClosed
	}
	return ""
}

func isAlertStateClosed(state *AlertState, now time.Time) bool {
	if state == nil {
		return false
	}
	return resolveAlertHandlingStatus(state, now) == AlertHandlingStatusClosed
}

func alertDisplayStatusWeight(status AlertDisplayStatus) int {
	switch status {
	case AlertDisplayStatusFiring:
		return 0
	case AlertDisplayStatusResolved:
		return 1
	case AlertDisplayStatusClosed:
		return 2
	default:
		return 3
	}
}

func buildLocalAlertSummary(row *AlertEventSource, rule *AlertRule) string {
	if row == nil {
		return ""
	}

	base := ""
	if rule != nil {
		base = strings.TrimSpace(rule.RuleName)
	}
	if row.EventType == monitor.EventTypeClusterRestartRequested {
		switch {
		case base != "" && strings.TrimSpace(row.ClusterName) != "":
			return fmt.Sprintf("%s · %s", base, strings.TrimSpace(row.ClusterName))
		case base != "":
			return base
		case strings.TrimSpace(row.ClusterName) != "":
			return fmt.Sprintf("集群重启 · %s", strings.TrimSpace(row.ClusterName))
		default:
			return "集群重启事件"
		}
	}
	if row.EventType == monitor.EventTypeNodeRestartRequested || row.EventType == monitor.EventTypeNodeStopRequested {
		hostDisplay := strings.TrimSpace(firstNonEmpty(row.Hostname, row.ProcessName))
		switch {
		case base != "" && hostDisplay != "":
			return fmt.Sprintf("%s · %s", base, hostDisplay)
		case base != "":
			return base
		case row.EventType == monitor.EventTypeNodeStopRequested && hostDisplay != "":
			return fmt.Sprintf("节点停止 · %s", hostDisplay)
		case row.EventType == monitor.EventTypeNodeRestartRequested && hostDisplay != "":
			return fmt.Sprintf("节点重启 · %s", hostDisplay)
		case row.EventType == monitor.EventTypeNodeStopRequested:
			return "节点停止事件"
		default:
			return "节点重启事件"
		}
	}
	if row.EventType == monitor.EventTypeNodeOffline {
		switch {
		case base != "" && strings.TrimSpace(row.Hostname) != "":
			return fmt.Sprintf("%s · %s", base, strings.TrimSpace(row.Hostname))
		case base != "" && strings.TrimSpace(row.ProcessName) != "":
			return fmt.Sprintf("%s · %s", base, strings.TrimSpace(row.ProcessName))
		case base != "":
			return base
		case strings.TrimSpace(row.Hostname) != "":
			return fmt.Sprintf("节点离线 · %s", strings.TrimSpace(row.Hostname))
		default:
			return "节点离线"
		}
	}
	processName := strings.TrimSpace(row.ProcessName)
	hostname := strings.TrimSpace(row.Hostname)
	switch {
	case base != "" && processName != "" && hostname != "":
		return fmt.Sprintf("%s · %s @ %s", base, processName, hostname)
	case base != "" && processName != "":
		return fmt.Sprintf("%s · %s", base, processName)
	case base != "" && hostname != "":
		return fmt.Sprintf("%s · %s", base, hostname)
	case base != "":
		return base
	case processName != "":
		return processName
	default:
		return string(row.EventType)
	}
}

func isInstantResolvedLocalEventType(eventType monitor.ProcessEventType) bool {
	switch eventType {
	case monitor.EventTypeClusterRestartRequested,
		monitor.EventTypeNodeRestartRequested,
		monitor.EventTypeNodeStopRequested:
		return true
	default:
		return false
	}
}

func toUTCTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	t := value.UTC()
	return &t
}
