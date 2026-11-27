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

package installer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRegistryHandlerRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newRegistryTestService(t)
	h := NewHandler(svc)
	r := gin.New()
	g := r.Group("/api/v1/packages")
	g.GET("/registries", h.ListContainerRegistries)
	g.POST("/registries", h.CreateContainerRegistry)
	g.PUT("/registries/:id", h.UpdateContainerRegistry)
	g.DELETE("/registries/:id", h.DeleteContainerRegistry)
	g.POST("/registries/:id/test", h.TestContainerRegistry)
	g.GET("/registries/:id/tags", h.ListContainerRegistryTags)
	g.POST("/registries/probe", h.ProbeRegistryTransient)
	return r, svc
}

func assertEnvelope(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw), "body=%s", string(body))
	_, hasErrorMsg := raw["error_msg"]
	_, hasLegacyError := raw["error"]
	assert.True(t, hasErrorMsg, "must expose unified error_msg envelope: %s", string(body))
	assert.False(t, hasLegacyError, "must not expose legacy top-level error field: %s", string(body))
	return raw
}

// TestContainerRegistryHandlersEnvelopeAndStatus 验证全部 7 个仓库端点统一使用
// {error_msg, data} 信封与精确 HTTP 状态码（400/404/409）。
// TestContainerRegistryHandlersEnvelopeAndStatus verifies all 7 registry endpoints
// use the {error_msg, data} envelope and accurate HTTP status codes.
func TestContainerRegistryHandlersEnvelopeAndStatus(t *testing.T) {
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/v2/_catalog"):
			_, _ = w.Write([]byte(`{"repositories":["team/st"]}`))
		case r.URL.Path == "/v2/team/st/tags/list":
			_, _ = w.Write([]byte(`{"tags":["3.1.0","3.1.1","latest"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer regServer.Close()
	host := strings.TrimPrefix(regServer.URL, "http://")

	r, svc := setupRegistryHandlerRouter(t)
	require.NoError(t, svc.db.Create(&ContainerRegistry{Host: DockerHubRegistryHost, IsSystem: true, ProbeStatus: "public"}).Error)

	// 1. POST create -> 200 + data envelope
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/packages/registries", bytes.NewBufferString(`{"host":"`+host+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	env := assertEnvelope(t, rec.Body.Bytes())
	var created ContainerRegistry
	require.NoError(t, json.Unmarshal(env["data"], &created))
	assert.Equal(t, "ok", created.ProbeStatus)

	// 2. Duplicate create -> 409
	recDup := httptest.NewRecorder()
	reqDup := httptest.NewRequest(http.MethodPost, "/api/v1/packages/registries", bytes.NewBufferString(`{"host":"`+host+`"}`))
	reqDup.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recDup, reqDup)
	assert.Equal(t, http.StatusConflict, recDup.Code)
	assertEnvelope(t, recDup.Body.Bytes())

	// 3. GET list -> 200 + data.items
	recList := httptest.NewRecorder()
	r.ServeHTTP(recList, httptest.NewRequest(http.MethodGet, "/api/v1/packages/registries", nil))
	require.Equal(t, http.StatusOK, recList.Code)
	envList := assertEnvelope(t, recList.Body.Bytes())
	var listData struct {
		Items []ContainerRegistry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(envList["data"], &listData))
	assert.Len(t, listData.Items, 2)

	// 4. GET tags -> 200 + semver descending order
	recTags := httptest.NewRecorder()
	r.ServeHTTP(recTags, httptest.NewRequest(http.MethodGet, "/api/v1/packages/registries/2/tags?repo=team/st", nil))
	require.Equal(t, http.StatusOK, recTags.Code)
	envTags := assertEnvelope(t, recTags.Body.Bytes())
	var tagData struct {
		Items []string `json:"items"`
	}
	require.NoError(t, json.Unmarshal(envTags["data"], &tagData))
	assert.Equal(t, []string{"3.1.1", "3.1.0", "latest"}, tagData.Items)

	// 5. POST /probe -> 200 + data
	recProbe := httptest.NewRecorder()
	reqProbe := httptest.NewRequest(http.MethodPost, "/api/v1/packages/registries/probe", bytes.NewBufferString(`{"host":"`+host+`"}`))
	reqProbe.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recProbe, reqProbe)
	require.Equal(t, http.StatusOK, recProbe.Code)
	assertEnvelope(t, recProbe.Body.Bytes())

	// 6. POST /test -> 200 + data; missing id -> 404
	recTest := httptest.NewRecorder()
	r.ServeHTTP(recTest, httptest.NewRequest(http.MethodPost, "/api/v1/packages/registries/2/test", nil))
	require.Equal(t, http.StatusOK, recTest.Code)
	assertEnvelope(t, recTest.Body.Bytes())

	recMissing := httptest.NewRecorder()
	r.ServeHTTP(recMissing, httptest.NewRequest(http.MethodPost, "/api/v1/packages/registries/999/test", nil))
	assert.Equal(t, http.StatusNotFound, recMissing.Code)
	assertEnvelope(t, recMissing.Body.Bytes())

	// 7. PUT /registries/1 (docker.io) changing host -> 400; DELETE /registries/1 -> 400
	recSysPut := httptest.NewRecorder()
	reqSysPut := httptest.NewRequest(http.MethodPut, "/api/v1/packages/registries/1", bytes.NewBufferString(`{"host":"other.io"}`))
	reqSysPut.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recSysPut, reqSysPut)
	assert.Equal(t, http.StatusBadRequest, recSysPut.Code)
	assertEnvelope(t, recSysPut.Body.Bytes())

	recSysDel := httptest.NewRecorder()
	r.ServeHTTP(recSysDel, httptest.NewRequest(http.MethodDelete, "/api/v1/packages/registries/1", nil))
	assert.Equal(t, http.StatusBadRequest, recSysDel.Code)
	assertEnvelope(t, recSysDel.Body.Bytes())

	// 8. DELETE /registries/2 -> 200
	recDel := httptest.NewRecorder()
	r.ServeHTTP(recDel, httptest.NewRequest(http.MethodDelete, "/api/v1/packages/registries/2", nil))
	require.Equal(t, http.StatusOK, recDel.Code)
	assertEnvelope(t, recDel.Body.Bytes())
}
