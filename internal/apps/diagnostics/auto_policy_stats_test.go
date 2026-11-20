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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAutoPolicyStatsTestService(t *testing.T) (*Service, *Repository) {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&ClusterInspectionReport{}, &ClusterInspectionFinding{}, &InspectionAutoPolicy{}))

	repo := NewRepository(database)
	return NewServiceWithRepository(repo, nil, nil, nil), repo
}

func seedAutoTriggerReport(t *testing.T, repo *Repository, policyID uint, reason string, createdAt time.Time) *ClusterInspectionReport {
	t.Helper()

	report := &ClusterInspectionReport{
		ClusterID:         7,
		Status:            InspectionReportStatusCompleted,
		TriggerSource:     InspectionTriggerSourceAuto,
		AutoTriggerReason: reason,
		AutoPolicyID:      policyID,
	}
	require.NoError(t, repo.db.Create(report).Error)
	// autoCreateTime 会把 CreatedAt 写成当前时间，统一回写为测试时间。
	// autoCreateTime stamps CreatedAt with "now", so rewrite it to the test timestamp.
	require.NoError(t, repo.db.Model(&ClusterInspectionReport{}).Where("id = ?", report.ID).Update("created_at", createdAt).Error)
	return report
}

func TestSetInspectionReportAutoTriggerWritesPolicyIDAndReason(t *testing.T) {
	_, repo := newAutoPolicyStatsTestService(t)
	ctx := t.Context()

	report := &ClusterInspectionReport{
		ClusterID:     7,
		Status:        InspectionReportStatusRunning,
		TriggerSource: InspectionTriggerSourceAuto,
	}
	require.NoError(t, repo.db.Create(report).Error)

	require.NoError(t, repo.SetInspectionReportAutoTrigger(ctx, report.ID, 42, "policy:42|JAVA_OOM: boom"))

	got, err := repo.GetInspectionReportByID(ctx, report.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(42), got.AutoPolicyID)
	assert.Equal(t, "policy:42|JAVA_OOM: boom", got.AutoTriggerReason)

	require.ErrorIs(t, repo.SetInspectionReportAutoTrigger(ctx, 999, 1, "policy:1|x"), ErrInspectionReportNotFound)
}

func TestListInspectionReportsFiltersByAutoPolicyID(t *testing.T) {
	_, repo := newAutoPolicyStatsTestService(t)
	ctx := t.Context()

	seedAutoTriggerReport(t, repo, 42, "policy:42|JAVA_OOM: boom", time.Now().UTC())
	seedAutoTriggerReport(t, repo, 9, "policy:9|ERROR_SPIKE: spike", time.Now().UTC())
	seedAutoTriggerReport(t, repo, 0, "", time.Now().UTC())

	reports, total, err := repo.ListInspectionReports(ctx, &ClusterInspectionReportFilter{AutoPolicyID: 42, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, reports, 1)
	assert.Equal(t, uint(42), reports[0].AutoPolicyID)

	// auto_policy_id=0 不过滤，保持既有语义。/ auto_policy_id=0 disables the filter, preserving prior behavior.
	all, allTotal, err := repo.ListInspectionReports(ctx, &ClusterInspectionReportFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(3), allTotal)
	require.Len(t, all, 3)
}

func TestListAutoPolicyTriggerStatsAggregatesLastAndSevenDayCount(t *testing.T) {
	service, repo := newAutoPolicyStatsTestService(t)
	ctx := t.Context()

	now := time.Now().UTC()
	seedAutoTriggerReport(t, repo, 42, "policy:42|a", now.Add(-2*time.Hour))
	seedAutoTriggerReport(t, repo, 42, "policy:42|b", now.Add(-26*time.Hour))
	// 8 天前的旧触发计入最近时间但不计入近 7 天。/ The 8-day-old trigger feeds last-triggered but not the 7d count.
	seedAutoTriggerReport(t, repo, 42, "policy:42|c", now.Add(-8*24*time.Hour))
	seedAutoTriggerReport(t, repo, 9, "policy:9|x", now.Add(-48*time.Hour))
	// 手动巡检不带归因，不应进入聚合。/ Manual reports carry no attribution and must stay out of the aggregate.
	manual := &ClusterInspectionReport{ClusterID: 7, Status: InspectionReportStatusCompleted, TriggerSource: InspectionTriggerSourceDiagnosticsWorkspace}
	require.NoError(t, repo.db.Create(manual).Error)

	stats, err := repo.ListAutoPolicyTriggerStats(ctx)
	require.NoError(t, err)
	require.Len(t, stats, 2)

	stat42 := stats[42]
	require.NotNil(t, stat42.LastTriggeredAt)
	assert.WithinDuration(t, now.Add(-2*time.Hour), *stat42.LastTriggeredAt, 2*time.Second)
	assert.Equal(t, int64(2), stat42.TriggerCount7d)

	stat9 := stats[9]
	require.NotNil(t, stat9.LastTriggeredAt)
	assert.WithinDuration(t, now.Add(-48*time.Hour), *stat9.LastTriggeredAt, 2*time.Second)
	assert.Equal(t, int64(1), stat9.TriggerCount7d)

	// 服务层把聚合写入列表响应：有触发的策略回填时间/次数，未触发为 null/0。
	// The service decorates list items: triggered policies get time/count, never-triggered stays null/0.
	policy42 := &InspectionAutoPolicy{ID: 42, ClusterID: 0, Name: "p42", Enabled: true, Conditions: InspectionConditionItems{}}
	require.NoError(t, repo.db.Create(policy42).Error)
	policy99 := &InspectionAutoPolicy{ID: 99, ClusterID: 0, Name: "p99", Enabled: true, Conditions: InspectionConditionItems{}}
	require.NoError(t, repo.db.Create(policy99).Error)

	data, err := service.ListAutoPolicies(ctx, nil, 1, 20)
	require.NoError(t, err)
	var info42, info99 *InspectionAutoPolicyInfo
	for _, item := range data.Items {
		switch item.ID {
		case 42:
			info42 = item
		default:
			info99 = item
		}
	}
	require.NotNil(t, info42)
	require.NotNil(t, info42.LastTriggeredAt)
	assert.Equal(t, int64(2), info42.TriggerCount7d)
	require.NotNil(t, info99)
	assert.Nil(t, info99.LastTriggeredAt)
	assert.Equal(t, int64(0), info99.TriggerCount7d)
}

func TestListInspectionReportsAutoPolicyIDHandlerParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, repo := newAutoPolicyStatsTestService(t)
	seedAutoTriggerReport(t, repo, 42, "policy:42|JAVA_OOM: boom", time.Now().UTC())
	seedAutoTriggerReport(t, repo, 0, "", time.Now().UTC())

	handler := NewHandler(service)
	router := gin.New()
	router.GET("/api/v1/diagnostics/inspections", handler.ListInspectionReports)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/inspections?auto_policy_id=42", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, "body=%s", recorder.Body.String())

	var response struct {
		Data struct {
			Items []struct {
				ID           uint `json:"id"`
				AutoPolicyID uint `json:"auto_policy_id"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, int64(1), response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, uint(42), response.Data.Items[0].AutoPolicyID)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/inspections?auto_policy_id=abc", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code, "body=%s", recorder.Body.String())
}
