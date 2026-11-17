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
	"testing"
	"time"
)

func TestService_ListAlertInstances_usesDisplayStatusAndCloseFlow(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(nil, nil, repo)

	startsAt := time.Date(2026, 3, 10, 1, 0, 0, 0, time.UTC)
	resolvedAt := startsAt.Add(8 * time.Minute)
	record := &RemoteAlertRecord{
		Fingerprint:    "fp-resolved-demo",
		StartsAt:       startsAt.Unix(),
		Status:         "resolved",
		AlertName:      "CPU usage high",
		Severity:       "critical",
		ClusterID:      "6",
		ClusterName:    "t3",
		Summary:        "CPU usage high on worker node",
		Description:    "cpu > 80% for 5m",
		ResolvedAt:     &resolvedAt,
		LastReceivedAt: resolvedAt,
	}
	if err := database.WithContext(ctx).Create(record).Error; err != nil {
		t.Fatalf("failed to create remote alert record: %v", err)
	}

	alerts, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{
		SourceType: AlertSourceTypeRemoteAlertmanager,
	})
	if err != nil {
		t.Fatalf("ListAlertInstances returned error: %v", err)
	}
	if len(alerts.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts.Alerts))
	}
	if alerts.Alerts[0].Status != AlertDisplayStatusResolved {
		t.Fatalf("expected resolved display status, got %s", alerts.Alerts[0].Status)
	}
	if alerts.Stats.Resolved != 1 || alerts.Stats.Closed != 0 || alerts.Stats.Firing != 0 {
		t.Fatalf("unexpected stats before close: %+v", alerts.Stats)
	}

	action, err := service.CloseAlertInstance(ctx, buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt), "admin", "handled")
	if err != nil {
		t.Fatalf("CloseAlertInstance returned error: %v", err)
	}
	if action.Status != AlertDisplayStatusClosed {
		t.Fatalf("expected action status closed, got %s", action.Status)
	}
	if action.ClosedBy != "admin" || action.ClosedAt == nil {
		t.Fatalf("expected close metadata, got %+v", action)
	}

	alerts, err = service.ListAlertInstances(ctx, &AlertInstanceFilter{
		SourceType: AlertSourceTypeRemoteAlertmanager,
	})
	if err != nil {
		t.Fatalf("ListAlertInstances after close returned error: %v", err)
	}
	if len(alerts.Alerts) != 1 {
		t.Fatalf("expected 1 alert after close, got %d", len(alerts.Alerts))
	}
	if alerts.Alerts[0].Status != AlertDisplayStatusClosed {
		t.Fatalf("expected closed display status after close, got %s", alerts.Alerts[0].Status)
	}
	if alerts.Alerts[0].ClosedBy != "admin" || alerts.Alerts[0].ClosedAt == nil {
		t.Fatalf("expected closed metadata on alert item, got %+v", alerts.Alerts[0])
	}
	if alerts.Stats.Closed != 1 || alerts.Stats.Resolved != 0 || alerts.Stats.Firing != 0 {
		t.Fatalf("unexpected stats after close: %+v", alerts.Stats)
	}

	filtered, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{
		SourceType: AlertSourceTypeRemoteAlertmanager,
		Status:     AlertDisplayStatusClosed,
	})
	if err != nil {
		t.Fatalf("ListAlertInstances closed filter returned error: %v", err)
	}
	if len(filtered.Alerts) != 1 || filtered.Alerts[0].Status != AlertDisplayStatusClosed {
		t.Fatalf("expected one closed alert in closed filter, got %+v", filtered.Alerts)
	}
}

func TestService_CloseAlertInstance_allowsFiringAlert(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(nil, nil, repo)

	startsAt := time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC)
	record := &RemoteAlertRecord{
		Fingerprint:    "fp-firing-demo",
		StartsAt:       startsAt.Unix(),
		Status:         "firing",
		AlertName:      "Memory usage high",
		Severity:       "warning",
		ClusterID:      "6",
		ClusterName:    "t3",
		Summary:        "Memory usage high on worker node",
		Description:    "memory > 80% for 5m",
		LastReceivedAt: startsAt.Add(2 * time.Minute),
	}
	if err := database.WithContext(ctx).Create(record).Error; err != nil {
		t.Fatalf("failed to create firing remote alert record: %v", err)
	}

	action, err := service.CloseAlertInstance(ctx, buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt), "admin", "handled by human")
	if err != nil {
		t.Fatalf("expected firing alert to be manually closable, got %v", err)
	}
	if action.Status != AlertDisplayStatusClosed {
		t.Fatalf("expected closed status, got %s", action.Status)
	}
	if action.ClosedBy != "admin" || action.ClosedAt == nil {
		t.Fatalf("expected close metadata, got %+v", action)
	}
}

func TestParseAlertSourceKeyAcceptsNumericEventID(t *testing.T) {
	parsed, err := parseAlertSourceKey("9")
	if err != nil {
		t.Fatalf("parse numeric alert id: %v", err)
	}
	if parsed.SourceType != AlertSourceTypeLocalProcessEvent || parsed.EventID != 9 || parsed.SourceKey != "local:event:9" {
		t.Fatalf("parsed = %+v, want local:event:9", parsed)
	}

	canonical, err := parseAlertSourceKey("local:event:9")
	if err != nil {
		t.Fatalf("parse canonical alert id: %v", err)
	}
	if canonical.SourceKey != "local:event:9" || canonical.EventID != 9 {
		t.Fatalf("canonical = %+v", canonical)
	}
}
