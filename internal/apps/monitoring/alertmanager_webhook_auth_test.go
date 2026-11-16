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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/gin-gonic/gin"
)

// newAlertmanagerWebhookTestRouter 构造只挂 webhook 端点的测试路由。
// newAlertmanagerWebhookTestRouter builds a test router exposing only the webhook endpoint.
func newAlertmanagerWebhookTestRouter(service *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/monitoring/alertmanager/webhook", NewHandler(service).AlertmanagerWebhook)
	return router
}

// doWebhookRequest 发送一次 webhook POST，可选携带 Authorization 头。
// doWebhookRequest issues one webhook POST, optionally carrying an Authorization header.
func doWebhookRequest(t *testing.T, router http.Handler, authorization string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/monitoring/alertmanager/webhook", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func webhookTestPayload() map[string]interface{} {
	return map[string]interface{}{
		"version":  "4",
		"status":   "firing",
		"receiver": "ops",
		"alerts": []map[string]interface{}{
			{
				"status":      "firing",
				"fingerprint": "fp-auth-test",
				"startsAt":    time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
				"labels":      map[string]string{"alertname": "HighCpu", "severity": "critical"},
			},
		},
	}
}

// TestAlertmanagerWebhook_BearerAuth 覆盖 remote_ingest.token 的 Bearer 鉴权矩阵：
// 配置 token 后无头/错 token 一律 401，对 token 正常入流；token 为空维持放行兼容。
// TestAlertmanagerWebhook_BearerAuth covers the remote_ingest.token Bearer matrix: once a token
// is configured, a missing header or wrong token is a 401 while the right token flows through;
// an empty configured token keeps the legacy open behaviour.
func TestAlertmanagerWebhook_BearerAuth(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	originalToken := config.Config.Observability.RemoteIngest.Token
	t.Cleanup(func() {
		config.Config.Observability.Enabled = originalEnabled
		config.Config.Observability.RemoteIngest.Token = originalToken
	})
	config.Config.Observability.Enabled = true

	database, cleanup := setupMonitoringNotificationTestDB(t)
	t.Cleanup(cleanup)
	router := newAlertmanagerWebhookTestRouter(NewService(nil, nil, NewRepository(database)))

	t.Run("token configured requires valid bearer", func(t *testing.T) {
		config.Config.Observability.RemoteIngest.Token = "test-remote-ingest-token"

		cases := []struct {
			name          string
			authorization string
			wantStatus    int
		}{
			{"missing authorization header", "", http.StatusUnauthorized},
			{"wrong bearer token", "Bearer wrong-token", http.StatusUnauthorized},
			{"non-bearer scheme", "Basic dGVzdA==", http.StatusUnauthorized},
			{"bare token without scheme", "test-remote-ingest-token", http.StatusUnauthorized},
			{"correct bearer token", "Bearer test-remote-ingest-token", http.StatusOK},
			{"bearer scheme case-insensitive", "bearer test-remote-ingest-token", http.StatusOK},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := doWebhookRequest(t, router, tc.authorization, webhookTestPayload())
				if rec.Code != tc.wantStatus {
					t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
				}
				if tc.wantStatus == http.StatusOK {
					var resp struct {
						Data *AlertmanagerWebhookResult `json:"data"`
					}
					if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
						t.Fatalf("failed to decode response: %v", err)
					}
					if resp.Data == nil || resp.Data.Stored != 1 {
						t.Fatalf("expected 1 stored alert, got %+v", resp.Data)
					}
				}
			})
		}
	})

	t.Run("empty token keeps endpoint open", func(t *testing.T) {
		config.Config.Observability.RemoteIngest.Token = ""
		rec := doWebhookRequest(t, router, "", webhookTestPayload())
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d without token, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}
	})
}

// TestRemoteIngestBearerAuthorized 单元覆盖 Authorization 头解析与常量时间比较的边界。
// TestRemoteIngestBearerAuthorized unit-covers Authorization header parsing and the constant-time
// compare edge cases.
func TestRemoteIngestBearerAuthorized(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"Bearer secret", true},
		{"bearer secret", true},
		{"BEARER secret", true},
		{"  Bearer  secret  ", true},
		{"Bearer", false},
		{"Bearer ", false},
		{"Bearer  ", false},
		{"Bearer other", false},
		{"secret", false},
		{"", false},
		{"Basic c2VjcmV0", false},
	}
	for _, tc := range cases {
		if got := remoteIngestBearerAuthorized(tc.header, "secret"); got != tc.want {
			t.Fatalf("remoteIngestBearerAuthorized(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}
