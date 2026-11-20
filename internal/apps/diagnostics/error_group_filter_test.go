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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedErrorGroupForFilter(t *testing.T, service *Service, occurrences int64, fingerprint string) {
	t.Helper()
	now := time.Now().UTC()
	repo := service.repo
	require.NotNil(t, repo)
	require.NoError(t, repo.db.Create(&SeatunnelErrorGroup{
		Fingerprint:        fingerprint,
		FingerprintVersion: "v1",
		Title:              "Job failed " + fingerprint,
		ExceptionClass:     "org.apache.seatunnel.TaskExecuteException",
		SampleMessage:      "sample " + fingerprint,
		OccurrenceCount:    occurrences,
		FirstSeenAt:        now,
		LastSeenAt:         now,
		LastClusterID:      1,
		LastNodeID:         1,
		LastHostID:         1,
	}).Error)
}

// TestListErrorGroupsOccurrenceRange 验证 min/max_occurrences 只约束组内聚合频次（闭区间）。
// TestListErrorGroupsOccurrenceRange verifies min/max_occurrences bound the aggregated count (closed range).
func TestListErrorGroupsOccurrenceRange(t *testing.T) {
	service, _ := newDiagnosticsTestService(t)
	seedErrorGroupForFilter(t, service, 1, "fp-low")
	seedErrorGroupForFilter(t, service, 5, "fp-mid")
	seedErrorGroupForFilter(t, service, 12, "fp-high")

	ctx := t.Context()
	intp := func(v int) *int { return &v }

	high, highTotal, err := service.repo.ListErrorGroups(ctx, &SeatunnelErrorGroupFilter{MinOccurrences: intp(10), Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), highTotal)
	require.Len(t, high, 1)
	assert.Equal(t, "fp-high", high[0].Fingerprint)

	mid, midTotal, err := service.repo.ListErrorGroups(ctx, &SeatunnelErrorGroupFilter{MinOccurrences: intp(3), MaxOccurrences: intp(9), Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), midTotal)
	require.Len(t, mid, 1)
	assert.Equal(t, "fp-mid", mid[0].Fingerprint)

	// 闭区间边界：3 与 9 都应命中。
	// Closed range: both bounds are inclusive.
	bounds, boundsTotal, err := service.repo.ListErrorGroups(ctx, &SeatunnelErrorGroupFilter{MinOccurrences: intp(5), MaxOccurrences: intp(5), Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), boundsTotal)
	require.Len(t, bounds, 1)
	assert.Equal(t, "fp-mid", bounds[0].Fingerprint)

	low, lowTotal, err := service.repo.ListErrorGroups(ctx, &SeatunnelErrorGroupFilter{MaxOccurrences: intp(2), Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), lowTotal)
	require.Len(t, low, 1)
	assert.Equal(t, "fp-low", low[0].Fingerprint)
}

// TestListErrorGroupsRejectsInvalidOccurrenceParams 验证非法频次参数按既有解析风格返回 400。
// TestListErrorGroupsRejectsInvalidOccurrenceParams verifies invalid occurrence params return 400 like the other query params.
func TestListErrorGroupsRejectsInvalidOccurrenceParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newDiagnosticsTestService(t)
	handler := NewHandler(service)
	router := gin.New()
	router.GET("/api/v1/diagnostics/errors/groups", handler.ListSeatunnelErrorGroups)

	for _, query := range []string{
		"min_occurrences=abc",
		"min_occurrences=-1",
		"min_occurrences=1.5",
		"max_occurrences=xyz",
		"max_occurrences=-3",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/errors/groups?"+query, nil)
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, "query=%s body=%s", query, recorder.Body.String())
	}
}

// TestListErrorGroupsAcceptsOccurrenceParams 验证合法频次参数透传到过滤并返回 200。
// TestListErrorGroupsAcceptsOccurrenceParams verifies valid occurrence params reach the filter and return 200.
func TestListErrorGroupsAcceptsOccurrenceParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newDiagnosticsTestService(t)
	seedErrorGroupForFilter(t, service, 15, "fp-hot")
	seedErrorGroupForFilter(t, service, 2, "fp-cold")
	handler := NewHandler(service)
	router := gin.New()
	router.GET("/api/v1/diagnostics/errors/groups", handler.ListSeatunnelErrorGroups)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/errors/groups?min_occurrences=10", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, "body=%s", recorder.Body.String())

	var response struct {
		Data struct {
			Items []struct {
				Fingerprint string `json:"fingerprint"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, int64(1), response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, "fp-hot", response.Data.Items[0].Fingerprint)
}
