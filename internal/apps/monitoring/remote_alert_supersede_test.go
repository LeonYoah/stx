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

// seedRemoteAlertRow 直接落一条远程告警记录，模拟评估器重启前遗留的旧实例。
// seedRemoteAlertRow persists one remote alert row directly, simulating a stale instance left
// behind by a previous evaluator lifecycle.
func seedRemoteAlertRow(t *testing.T, ctx context.Context, repo *Repository, fingerprint string, startsAt int64, status string) {
	t.Helper()
	record := &RemoteAlertRecord{
		Fingerprint:    fingerprint,
		StartsAt:       startsAt,
		Status:         status,
		Receiver:       "stx",
		AlertName:      "DeadlockedThreads",
		Severity:       "warning",
		ClusterID:      "7",
		ClusterName:    "beta",
		LabelsJSON:     `{"alertname":"DeadlockedThreads","severity":"warning","cluster_id":"7"}`,
		LastReceivedAt: time.Now().UTC(),
	}
	if err := repo.UpsertRemoteAlert(ctx, record); err != nil {
		t.Fatalf("seedRemoteAlertRow upsert failed: %v", err)
	}
}

// unresolvedRemoteAlertStarts 返回集群内未恢复记录的 starts_at 集合，便于断言只剩一个实例。
// unresolvedRemoteAlertStarts returns the starts_at set of unresolved records for the cluster,
// making it easy to assert that only one instance remains.
func unresolvedRemoteAlertStarts(t *testing.T, ctx context.Context, repo *Repository, clusterID string) map[int64]bool {
	t.Helper()
	rows, err := repo.ListUnresolvedRemoteAlerts(ctx, []string{clusterID})
	if err != nil {
		t.Fatalf("ListUnresolvedRemoteAlerts failed: %v", err)
	}
	set := make(map[int64]bool, len(rows))
	for _, row := range rows {
		set[row.StartsAt] = true
	}
	return set
}

// TestIngestRemoteAlertRecord_supersedesStaleInstances 验证「同指纹只留一个未恢复实例」：
// 评估器重启/规则重载会用新 starts_at 重新上报相同告警，旧的 firing 行必须静默收编为 resolved，
// 否则 alert-instances 与集群健康计数会出现重复的幽灵活跃告警。
// TestIngestRemoteAlertRecord_supersedesStaleInstances verifies "one unresolved instance per
// fingerprint": after evaluator restarts / rule reloads the same alert is re-reported with a new
// starts_at, and stale firing rows must be silently folded into resolved, otherwise alert
// instances and cluster health counts show duplicate ghost-active alerts.
func TestIngestRemoteAlertRecord_supersedesStaleInstances(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)
	ctx := context.Background()

	const fingerprint = "fp-dup"
	const clusterID = "7"
	s1 := int64(1000)
	s2 := int64(2000)
	s3 := int64(3000)

	// 两条同指纹 firing 旧行：模拟多次评估器重启遗留的幽灵实例。
	// Two stale firing rows of the same fingerprint simulate ghost instances from repeated
	// evaluator restarts.
	seedRemoteAlertRow(t, ctx, repo, fingerprint, s1, "firing")
	seedRemoteAlertRow(t, ctx, repo, fingerprint, s2, "firing")

	firing := &RemoteAlertRecord{
		Fingerprint:    fingerprint,
		StartsAt:       s3,
		Status:         "firing",
		Receiver:       "stx",
		AlertName:      "DeadlockedThreads",
		Severity:       "warning",
		ClusterID:      clusterID,
		ClusterName:    "beta",
		LabelsJSON:     `{"alertname":"DeadlockedThreads","severity":"warning","cluster_id":"7"}`,
		LastReceivedAt: time.Now().UTC(),
	}
	if err := service.ingestRemoteAlertRecord(ctx, firing); err != nil {
		t.Fatalf("ingestRemoteAlertRecord firing returned error: %v", err)
	}

	unresolved := unresolvedRemoteAlertStarts(t, ctx, repo, clusterID)
	if len(unresolved) != 1 || !unresolved[s3] {
		t.Fatalf("expected only starts_at=%d unresolved, got %v", s3, unresolved)
	}
	// 旧实例按新实例的 starts_at 收编为 resolved。
	// Stale instances fold into resolved at the new instance's starts_at.
	stale, err := repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, fingerprint, s2)
	if err != nil || stale == nil {
		t.Fatalf("expected stale row s2 to exist: %v", err)
	}
	if stale.Status != "resolved" || stale.ResolvedAt == nil || stale.ResolvedAt.Unix() != s3 {
		t.Fatalf("expected stale row resolved at %d, got status=%s resolved_at=%v", s3, stale.Status, stale.ResolvedAt)
	}

	// 再造一条幽灵行，验证 resolved 记录同样收编其它未恢复实例。
	// Seed another ghost row and verify a resolved record also folds other unresolved instances.
	s4 := int64(4000)
	seedRemoteAlertRow(t, ctx, repo, fingerprint, s4, "firing")
	resolvedAt := time.Unix(s3+500, 0).UTC()
	resolved := &RemoteAlertRecord{
		Fingerprint:    fingerprint,
		StartsAt:       s3,
		Status:         "resolved",
		Receiver:       "stx",
		AlertName:      "DeadlockedThreads",
		Severity:       "warning",
		ClusterID:      clusterID,
		ClusterName:    "beta",
		LabelsJSON:     firing.LabelsJSON,
		EndsAt:         resolvedAt.Unix(),
		ResolvedAt:     &resolvedAt,
		LastReceivedAt: time.Now().UTC(),
	}
	if err := service.ingestRemoteAlertRecord(ctx, resolved); err != nil {
		t.Fatalf("ingestRemoteAlertRecord resolved returned error: %v", err)
	}
	if unresolved := unresolvedRemoteAlertStarts(t, ctx, repo, clusterID); len(unresolved) != 0 {
		t.Fatalf("expected no unresolved instances, got %v", unresolved)
	}
	ghost, err := repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, fingerprint, s4)
	if err != nil || ghost == nil {
		t.Fatalf("expected ghost row s4 to exist: %v", err)
	}
	if ghost.Status != "resolved" {
		t.Fatalf("expected ghost row s4 resolved, got %s", ghost.Status)
	}
}
