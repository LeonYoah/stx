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
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// alertStoreKind 是告警的存储种类，只由告警 ID 前缀决定，仅用于内部分发
// （确认 / 静默 / 关闭 / 退避重试查表），不对外暴露。
// alertStoreKind is where an alert is stored, derived from the alert ID prefix;
// used only for internal dispatch (ack / silence / close / retry lookups) and never exposed.
type alertStoreKind string

const (
	// alertStoreLocalEvent 对应 local:event:<id>。
	// alertStoreLocalEvent matches local:event:<id>.
	alertStoreLocalEvent alertStoreKind = "local_event"
	// alertStoreMetricRecord 对应 remote:<fingerprint>:<startsAt>。
	// alertStoreMetricRecord matches remote:<fingerprint>:<startsAt>.
	alertStoreMetricRecord alertStoreKind = "metric_record"
	// alertStoreSyncEpisode 对应 sync:episode:<id>。
	// alertStoreSyncEpisode matches sync:episode:<id>.
	alertStoreSyncEpisode alertStoreKind = "sync_episode"
)

type parsedAlertSourceKey struct {
	Kind        alertStoreKind
	SourceKey   string
	EventID     uint
	Fingerprint string
	StartsAt    int64
	EpisodeID   uint
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

	now := timex.Now()
	items := make([]*AlertInstance, 0, 32)

	// 类别按规则归属判定：platform_health 同时包含本地事件与集群级作业阶段，
	// job 只含任务级作业阶段；sync 阶段在 buildSyncAlertInstances 内再按类别过滤。
	// Category follows rule ownership: platform_health covers both local events and
	// cluster-level sync episodes, job covers task-level episodes only; sync episodes
	// are filtered again by category inside buildSyncAlertInstances.
	includeLocal := filter.SourceType == "" || filter.SourceType == AlertSourceTypePlatformHealth
	includeRemote := filter.SourceType == "" || filter.SourceType == AlertSourceTypeMetric
	includeSync := filter.SourceType == "" || filter.SourceType == AlertSourceTypePlatformHealth || filter.SourceType == AlertSourceTypeJob

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
	if includeSync {
		syncItems, err := s.buildSyncAlertInstances(ctx, filter, now)
		if err != nil {
			return nil, err
		}
		items = append(items, syncItems...)
	}

	// scope 派生与过滤：job → job，其余 → cluster；与旧 source_type 过滤并存。
	// Scope derivation and filtering: job → job, everything else → cluster; it coexists with
	// the legacy source_type filter.
	scoped := items[:0]
	for _, item := range items {
		if item == nil {
			continue
		}
		item.Scope = alertScopeFromSourceType(item.SourceType)
		if filter.Scope != "" && item.Scope != filter.Scope {
			continue
		}
		scoped = append(scoped, item)
	}
	items = scoped

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
	if isAlertStateClosed(existingState, timex.Now()) {
		return nil, ErrAlertInstanceAlreadyClosed
	}

	now := timex.Now()
	switch parsed.Kind {
	case alertStoreLocalEvent:
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
		return s.alertInstanceActionOK(state), nil
	case alertStoreMetricRecord:
		record, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, parsed.Fingerprint, parsed.StartsAt)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, ErrAlertInstanceNotFound
		}
		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     AlertSourceTypeMetric,
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
		return s.alertInstanceActionOK(state), nil
	case alertStoreSyncEpisode:
		clusterID, ruleKey, err := s.syncAlertEpisodeClusterRule(ctx, parsed.EpisodeID)
		if err != nil {
			return nil, err
		}
		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     syncEpisodeCategory(ruleKey),
			SourceKey:      parsed.SourceKey,
			ClusterID:      clusterID,
			HandlingStatus: AlertHandlingStatusAcknowledged,
			AcknowledgedBy: operator,
			AcknowledgedAt: &now,
			Note:           note,
		})
		if err != nil {
			return nil, err
		}
		return s.alertInstanceActionOK(state), nil
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
	if isAlertStateClosed(existingState, timex.Now()) {
		return nil, ErrAlertInstanceAlreadyClosed
	}

	silencedUntil := timex.Now().Add(time.Duration(durationMinutes) * time.Minute)
	switch parsed.Kind {
	case alertStoreLocalEvent:
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
		return s.alertInstanceActionOK(state), nil
	case alertStoreMetricRecord:
		record, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, parsed.Fingerprint, parsed.StartsAt)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, ErrAlertInstanceNotFound
		}
		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     AlertSourceTypeMetric,
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
		return s.alertInstanceActionOK(state), nil
	case alertStoreSyncEpisode:
		clusterID, ruleKey, err := s.syncAlertEpisodeClusterRule(ctx, parsed.EpisodeID)
		if err != nil {
			return nil, err
		}
		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     syncEpisodeCategory(ruleKey),
			SourceKey:      parsed.SourceKey,
			ClusterID:      clusterID,
			HandlingStatus: AlertHandlingStatusSilenced,
			SilencedBy:     operator,
			SilencedUntil:  &silencedUntil,
			Note:           note,
		})
		if err != nil {
			return nil, err
		}
		return s.alertInstanceActionOK(state), nil
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
	if isAlertStateClosed(existingState, timex.Now()) {
		return toAlertInstanceActionResult(existingState), nil
	}

	now := timex.Now()
	switch parsed.Kind {
	case alertStoreLocalEvent:
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
			SourceType:     AlertSourceTypePlatformHealth,
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
		return s.alertInstanceActionOK(state), nil
	case alertStoreMetricRecord:
		record, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, parsed.Fingerprint, parsed.StartsAt)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, ErrAlertInstanceNotFound
		}

		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     AlertSourceTypeMetric,
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
		return s.alertInstanceActionOK(state), nil
	case alertStoreSyncEpisode:
		// 关闭作用于整个阶段：之后不再提醒；同一个键再出问题时开新阶段。
		// Closing applies to the whole episode: no more reminders, and the next problem on the key opens a new one.
		clusterID, ruleKey, err := s.syncAlertEpisodeClusterRule(ctx, parsed.EpisodeID)
		if err != nil {
			return nil, err
		}
		state, err := s.upsertAlertState(ctx, &AlertState{
			SourceType:     syncEpisodeCategory(ruleKey),
			SourceKey:      parsed.SourceKey,
			ClusterID:      clusterID,
			HandlingStatus: AlertHandlingStatusClosed,
			ClosedBy:       operator,
			ClosedAt:       &now,
			Note:           note,
		})
		if err != nil {
			return nil, err
		}
		return s.alertInstanceActionOK(state), nil
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

	recoveryIndex, err := s.loadLocalRecoveryIndex(ctx, rows)
	if err != nil {
		return nil, err
	}

	ruleCache := make(map[string]*resolvedRuleConfig)
	items := make([]*AlertInstance, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}

		ruleKey := eventTypeToRuleKey(row.EventType)
		cacheKey := strconv.FormatUint(uint64(row.ClusterID), 10) + "|" + ruleKey
		rule, ok := ruleCache[cacheKey]
		if !ok {
			resolved, err := s.ResolveClusterRule(ctx, ruleKey, row.ClusterID)
			if err != nil {
				return nil, err
			}
			rule = resolved
			ruleCache[cacheKey] = rule
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

		lifecycleStatus, resolvedAt, err := s.resolveLocalAlertLifecycle(ctx, row, recoveryIndex)
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

		ruleName := ""
		ruleNameEN := ""
		if rule.Definition != nil {
			ruleName = strings.TrimSpace(rule.Definition.Name)
			ruleNameEN = strings.TrimSpace(rule.Definition.NameEN)
		}
		item := &AlertInstance{
			AlertID:         sourceKey,
			SourceType:      AlertSourceTypePlatformHealth,
			ClusterID:       strconv.FormatUint(uint64(row.ClusterID), 10),
			ClusterName:     strings.TrimSpace(row.ClusterName),
			Severity:        rule.Severity,
			AlertName:       ruleName,
			AlertNameEN:     ruleNameEN,
			RuleKey:         strings.TrimSpace(rule.RuleKey),
			Summary:         buildLocalAlertSummary(row, ruleName),
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

func (s *Service) resolveLocalAlertLifecycle(ctx context.Context, row *AlertEventSource, recoveryIndex *localRecoveryIndex) (AlertLifecycleStatus, *time.Time, error) {
	if row == nil {
		return AlertLifecycleStatusFiring, nil, nil
	}
	if isInstantResolvedLocalEventType(row.EventType) {
		resolvedAt := row.CreatedAt.UTC()
		return AlertLifecycleStatusResolved, &resolvedAt, nil
	}
	if s.monitorService == nil && recoveryIndex == nil {
		return AlertLifecycleStatusFiring, nil, nil
	}

	lookup := func(types []monitor.ProcessEventType) (bool, *monitor.ProcessEvent, error) {
		if recoveryIndex != nil {
			event := recoveryIndex.firstAfter(row.NodeID, row.CreatedAt, types)
			if event == nil {
				return false, nil, nil
			}
			return true, event, nil
		}
		return s.monitorService.HasNodeEventAfter(ctx, row.NodeID, row.CreatedAt, types)
	}

	subjectLookup := func(types []monitor.ProcessEventType) (bool, *monitor.ProcessEvent, error) {
		if recoveryIndex != nil {
			event := recoveryIndex.firstSubjectAfter(row.ClusterID, row.NodeID, row.HostID, row.CreatedAt, types)
			if event == nil {
				return false, nil, nil
			}
			return true, event, nil
		}
		return false, nil, nil
	}

	switch row.EventType {
	case monitor.EventTypeNodeOffline:
		recovered, recoveryEvent, err := lookup([]monitor.ProcessEventType{
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
	case monitor.EventTypeAgentOffline, monitor.EventTypeMasterUnavailable, monitor.EventTypeWorkerInsufficient:
		recovered, recoveryEvent, err := subjectLookup([]monitor.ProcessEventType{
			availabilityFiringRecoveredTypes[row.EventType],
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
		recovered, recoveryEvent, err := lookup([]monitor.ProcessEventType{
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

// localRecoveryIndex 按节点缓存恢复类事件，供一次列表内判定生命周期，避免逐条查库；
// bySubject 额外按 cluster|node|host 元组索引主机 / 集群级恢复事件。
// localRecoveryIndex caches recovery events per node so one list can resolve lifecycles
// without per-row queries; bySubject additionally indexes host / cluster scoped recovery
// events by the cluster|node|host tuple.
type localRecoveryIndex struct {
	byNode    map[uint][]monitor.ProcessEvent
	bySubject map[string][]monitor.ProcessEvent
}

func alertSubjectKey(clusterID, nodeID, hostID uint) string {
	return strconv.FormatUint(uint64(clusterID), 10) + "|" +
		strconv.FormatUint(uint64(nodeID), 10) + "|" +
		strconv.FormatUint(uint64(hostID), 10)
}

func (idx *localRecoveryIndex) firstSubjectAfter(clusterID, nodeID, hostID uint, after time.Time, types []monitor.ProcessEventType) *monitor.ProcessEvent {
	if idx == nil {
		return nil
	}
	allowed := make(map[monitor.ProcessEventType]struct{}, len(types))
	for _, eventType := range types {
		allowed[eventType] = struct{}{}
	}
	for i := range idx.bySubject[alertSubjectKey(clusterID, nodeID, hostID)] {
		event := &idx.bySubject[alertSubjectKey(clusterID, nodeID, hostID)][i]
		if !event.CreatedAt.UTC().After(timex.Bound(after)) {
			continue
		}
		if _, ok := allowed[event.EventType]; ok {
			return event
		}
	}
	return nil
}

func (idx *localRecoveryIndex) firstAfter(nodeID uint, after time.Time, types []monitor.ProcessEventType) *monitor.ProcessEvent {
	if idx == nil {
		return nil
	}
	allowed := make(map[monitor.ProcessEventType]struct{}, len(types))
	for _, eventType := range types {
		allowed[eventType] = struct{}{}
	}
	for i := range idx.byNode[nodeID] {
		event := &idx.byNode[nodeID][i]
		if !event.CreatedAt.UTC().After(timex.Bound(after)) {
			continue
		}
		if _, ok := allowed[event.EventType]; ok {
			return event
		}
	}
	return nil
}

func (s *Service) loadLocalRecoveryIndex(ctx context.Context, rows []*AlertEventSource) (*localRecoveryIndex, error) {
	if s.monitorService == nil || len(rows) == 0 {
		return &localRecoveryIndex{byNode: map[uint][]monitor.ProcessEvent{}, bySubject: map[string][]monitor.ProcessEvent{}}, nil
	}
	nodeIDs := make([]uint, 0, len(rows))
	clusterIDs := make([]uint, 0, len(rows))
	seenNode := make(map[uint]struct{}, len(rows))
	seenCluster := make(map[uint]struct{}, len(rows))
	var minCreated time.Time
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.NodeID != 0 {
			if _, ok := seenNode[row.NodeID]; !ok {
				seenNode[row.NodeID] = struct{}{}
				nodeIDs = append(nodeIDs, row.NodeID)
			}
		}
		if row.ClusterID != 0 {
			if _, ok := seenCluster[row.ClusterID]; !ok {
				seenCluster[row.ClusterID] = struct{}{}
				clusterIDs = append(clusterIDs, row.ClusterID)
			}
		}
		if minCreated.IsZero() || row.CreatedAt.Before(minCreated) {
			minCreated = row.CreatedAt
		}
	}
	index := &localRecoveryIndex{
		byNode:    make(map[uint][]monitor.ProcessEvent, len(nodeIDs)),
		bySubject: map[string][]monitor.ProcessEvent{},
	}
	if len(nodeIDs) > 0 {
		events, err := s.monitorService.ListNodeEventsAfter(ctx, nodeIDs, minCreated, []monitor.ProcessEventType{
			monitor.EventTypeStarted,
			monitor.EventTypeRestarted,
			monitor.EventTypeNodeRecovered,
		})
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			index.byNode[event.NodeID] = append(index.byNode[event.NodeID], event)
		}
	}
	if len(clusterIDs) > 0 {
		// 主机 / 集群级主体（agent_offline / master_unavailable / worker_insufficient）的恢复事件
		// node_id 为 0，节点查询取不到；按集群取回后按 subject 元组建索引。
		// Recovery events of host / cluster scoped subjects (agent_offline /
		// master_unavailable / worker_insufficient) carry node_id 0 and are invisible to the
		// node query; fetch them per cluster and index them by the subject tuple.
		events, err := s.monitorService.ListClusterEventsAfter(ctx, clusterIDs, minCreated, []monitor.ProcessEventType{
			monitor.EventTypeNodeRecovered,
			monitor.EventTypeAgentRecovered,
			monitor.EventTypeMasterRecovered,
			monitor.EventTypeWorkerRecovered,
		})
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			key := alertSubjectKey(event.ClusterID, event.NodeID, event.HostID)
			index.bySubject[key] = append(index.bySubject[key], event)
		}
	}
	return index, nil
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

		lifecycleStatus, inferredResolvedAt := resolveRemoteLifecycleStatus(row.Status, row.EndsAt, row.LastReceivedAt, now)
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

		ruleKey := remoteRecordRuleKey(row)
		storedName := strings.TrimSpace(row.AlertName)
		if ruleKey == "" {
			ruleKey = legacyAlertNameRuleKeys[storedName]
		}
		alertName := storedName
		alertNameEN := ""
		originalAlertName := ""
		if def := lookupClusterRuleDefinition(ruleKey); def != nil {
			originalAlertName = storedName
			alertName = strings.TrimSpace(def.Name)
			alertNameEN = strings.TrimSpace(def.NameEN)
		}

		item := &AlertInstance{
			AlertID:         sourceKey,
			SourceType:      AlertSourceTypeMetric,
			ClusterID:       strings.TrimSpace(row.ClusterID),
			ClusterName:     strings.TrimSpace(row.ClusterName),
			Severity:        AlertSeverity(strings.TrimSpace(row.Severity)),
			AlertName:       alertName,
			AlertNameEN:     alertNameEN,
			RuleKey:         ruleKey,
			Summary:         strings.TrimSpace(row.Summary),
			Description:     strings.TrimSpace(row.Description),
			Status:          displayStatus,
			LifecycleStatus: lifecycleStatus,
			HandlingStatus:  handlingStatus,
			CreatedAt:       row.CreatedAt.UTC(),
			FiringAt:        firingAt,
			LastSeenAt:      lastSeenAt,
			ResolvedAt:      firstNonNilTime(toUTCTimePointer(row.ResolvedAt), inferredResolvedAt),
			SourceRef: &AlertInstanceSourceRef{
				Fingerprint:       strings.TrimSpace(row.Fingerprint),
				Receiver:          strings.TrimSpace(row.Receiver),
				Env:               strings.TrimSpace(row.Env),
				OriginalAlertName: originalAlertName,
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

	now := timex.Now()
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
		SourceType:     AlertSourceTypePlatformHealth,
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

// legacyAlertNameRuleKeys 把迁移前内置 YAML 规则的告警名映射到目录 rule_key。
// 仅影响展示与 rule_key 派生，不回写存储。
// legacyAlertNameRuleKeys maps pre-migration built-in YAML alert names onto
// catalog rule keys. Display/derivation only; stored records are untouched.
var legacyAlertNameRuleKeys = map[string]string{
	"SeaTunnelMetricsEndpointDown":       AlertRuleKeyNodeOffline,
	"SeaTunnelNodeStateDown":             AlertRuleKeyNodeOffline,
	"SeaTunnelPartitionUnsafe":           "split_brain_risk",
	"SeaTunnelJobThreadPoolQueueHigh":    "job_thread_pool_queue_backlog_high",
	"SeaTunnelCoordinatorBacklogGrowing": "coordinator_backlog_growing",
	"SeaTunnelCoordinatorRejectingTasks": "job_thread_pool_rejection_high",
	"SeaTunnelJVMHeapUsageHigh":          "memory_usage_high",
	"SeaTunnelProcessCpuHigh":            "cpu_usage_high",
	"SeaTunnelGCTimeHigh":                "gc_time_high",
	"SeaTunnelFDUsageHigh":               "fd_usage_high",
	"SeaTunnelThreadDeadlock":            "deadlocked_threads_detected",
}

// remoteRecordRuleKey 从告警记录的 labels 里取出规则键（评估器写入的 template_key），
// 解析失败或缺失时返回空串。
// remoteRecordRuleKey extracts the rule key (the template_key the evaluator writes into
// rule labels) from a stored alert record, returning "" when absent or unparsable.
func remoteRecordRuleKey(record *RemoteAlertRecord) string {
	return strings.TrimSpace(parseRemoteAlertLabels(record)["template_key"])
}

func parseAlertSourceKey(sourceKey string) (*parsedAlertSourceKey, error) {
	sourceKey = strings.TrimSpace(sourceKey)
	parts := strings.Split(sourceKey, ":")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid alert id")
	}

	if parts[0] == "local" && parts[1] == "event" {
		eventID, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid alert id")
		}
		return &parsedAlertSourceKey{
			Kind:      alertStoreLocalEvent,
			SourceKey: sourceKey,
			EventID:   uint(eventID),
		}, nil
	}

	if episodeID, ok := parseSyncAlertSourceKey(parts); ok {
		return &parsedAlertSourceKey{
			Kind:      alertStoreSyncEpisode,
			SourceKey: sourceKey,
			EpisodeID: episodeID,
		}, nil
	}

	if parts[0] == "remote" && len(parts) == 3 {
		startsAt, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid alert id")
		}
		return &parsedAlertSourceKey{
			Kind:        alertStoreMetricRecord,
			SourceKey:   sourceKey,
			Fingerprint: parts[1],
			StartsAt:    startsAt,
		}, nil
	}

	return nil, fmt.Errorf("invalid alert id")
}

// remoteAlertStaleThreshold 是没有 ends_at 的旧记录（外部 Alertmanager 时代写入、没有内嵌
// 评估器的 ValidUntil 概念）判恢复的兜底阈值。
// remoteAlertStaleThreshold is the fallback age after which a legacy record without ends_at
// (written by the external Alertmanager stack, which predates the embedded evaluator's
// ValidUntil) counts as resolved.
const remoteAlertStaleThreshold = time.Hour

// resolveRemoteLifecycleStatus 把记录状态映射成生命周期状态，第二返回值是按过期推断出的恢复
// 时间（无则 nil）。接入端点移除后，firing 记录不再保证能等到 resolved 记录：ends_at 已过
// 判为已恢复（内嵌评估器 firing 时持续以 ValidUntil 刷新它，过期说明重推已停止）；ends_at=0
// 的旧记录用 last_received_at 兜底。
// resolveRemoteLifecycleStatus maps the stored status to the lifecycle status; the second return
// is the resolved time inferred from expiry (nil when absent). With the ingest endpoint removed,
// a firing record is not guaranteed to meet a resolved twin: a record past ends_at counts as
// resolved (the embedded evaluator keeps refreshing it to ValidUntil while resending, so expiry
// means resends stopped), and legacy rows with ends_at=0 fall back to last_received_at.
func resolveRemoteLifecycleStatus(status string, endsAt int64, lastReceivedAt time.Time, now time.Time) (AlertLifecycleStatus, *time.Time) {
	if strings.EqualFold(strings.TrimSpace(status), string(AlertLifecycleStatusResolved)) {
		return AlertLifecycleStatusResolved, nil
	}
	if endsAt > 0 && time.Unix(endsAt, 0).Before(now) {
		t := time.Unix(endsAt, 0).UTC()
		return AlertLifecycleStatusResolved, &t
	}
	if endsAt == 0 && !lastReceivedAt.IsZero() && lastReceivedAt.Add(remoteAlertStaleThreshold).Before(now) {
		t := lastReceivedAt.Add(remoteAlertStaleThreshold).UTC()
		return AlertLifecycleStatusResolved, &t
	}
	return AlertLifecycleStatusFiring, nil
}

// firstNonNilTime 返回第一个非空时间指针。/ firstNonNilTime returns the first non-nil time pointer.
func firstNonNilTime(values ...*time.Time) *time.Time {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
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
		SourceType:     AlertSourceTypePlatformHealth,
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

// alertInstanceActionOK 在确认 / 静默 / 关闭成功后丢掉集群健康缓存，让首页和大盘告警数立刻对齐。
// alertInstanceActionOK drops the clusters-health cache after a successful ack / silence / close so home and
// dashboard alert counts update immediately.
func (s *Service) alertInstanceActionOK(state *AlertState) *AlertInstanceActionResult {
	s.invalidateClustersHealthCache()
	return toAlertInstanceActionResult(state)
}

func toAlertInstanceActionResult(state *AlertState) *AlertInstanceActionResult {
	if state == nil {
		return nil
	}
	return &AlertInstanceActionResult{
		AlertID:        state.SourceKey,
		Status:         resolveActionDisplayStatus(state),
		HandlingStatus: resolveAlertHandlingStatus(state, timex.Now()),
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
	if isAlertStateClosed(state, timex.Now()) {
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

func buildLocalAlertSummary(row *AlertEventSource, ruleName string) string {
	if row == nil {
		return ""
	}

	base := strings.TrimSpace(ruleName)
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
