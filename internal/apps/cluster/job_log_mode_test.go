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

package cluster

import (
	"context"
	"strings"
	"testing"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
)

type stubRuntimeConfigStore struct {
	configs        []*appconfig.ConfigInfo
	lastUpdatedID  uint
	lastUpdateReq  *appconfig.UpdateConfigRequest
	lastSyncedType appconfig.ConfigType
}

func (s *stubRuntimeConfigStore) GetByCluster(ctx context.Context, clusterID uint) ([]*appconfig.ConfigInfo, error) {
	return s.configs, nil
}

func (s *stubRuntimeConfigStore) Update(ctx context.Context, id uint, req *appconfig.UpdateConfigRequest, userID uint) (*appconfig.ConfigInfo, error) {
	s.lastUpdatedID = id
	s.lastUpdateReq = req
	for _, c := range s.configs {
		if c.ID == id {
			c.Content = req.Content
			c.Version += 1
			return c, nil
		}
	}
	return &appconfig.ConfigInfo{ID: id, Content: req.Content, Version: 2}, nil
}

func (s *stubRuntimeConfigStore) SyncTemplateToAllNodes(ctx context.Context, clusterID uint, configType appconfig.ConfigType, userID uint) (*appconfig.SyncAllResult, error) {
	s.lastSyncedType = configType
	return &appconfig.SyncAllResult{SyncedCount: 1}, nil
}

func TestPatchLog4j2JobLogMode(t *testing.T) {
	mixedContent := `
rootLogger.level = INFO
rootLogger.appenderRef.file.ref = fileAppender

appender.routing.name = routingAppender
appender.routing.type = Routing
appender.routing.route.job.appender.fileName = ${file_path}/job-${ctx:ST-JID}.log
`

	perJob, err := patchLog4j2JobLogMode(mixedContent, "per_job")
	if err != nil {
		t.Fatalf("patchLog4j2JobLogMode failed: %v", err)
	}
	if !strings.Contains(perJob, "rootLogger.appenderRef.file.ref = routingAppender") {
		t.Fatalf("expected routingAppender in perJob content, got:\n%s", perJob)
	}

	backToMixed, err := patchLog4j2JobLogMode(perJob, "mixed")
	if err != nil {
		t.Fatalf("patchLog4j2JobLogMode back to mixed failed: %v", err)
	}
	if !strings.Contains(backToMixed, "rootLogger.appenderRef.file.ref = fileAppender") {
		t.Fatalf("expected fileAppender in mixed content, got:\n%s", backToMixed)
	}

	// Test with missing routing appender block
	minimalContent := `rootLogger.level = INFO`
	perJobWithAppendedBlock, err := patchLog4j2JobLogMode(minimalContent, "per_job")
	if err != nil {
		t.Fatalf("patchLog4j2JobLogMode failed: %v", err)
	}
	if !strings.Contains(perJobWithAppendedBlock, "rootLogger.appenderRef.file.ref = routingAppender") {
		t.Fatalf("expected routingAppender ref, got:\n%s", perJobWithAppendedBlock)
	}
	if !strings.Contains(perJobWithAppendedBlock, "appender.routing.name = routingAppender") {
		t.Fatalf("expected default routing appender block to be appended, got:\n%s", perJobWithAppendedBlock)
	}
}

func TestSwitchJobLogModeService(t *testing.T) {
	service := &Service{}
	store := &stubRuntimeConfigStore{
		configs: []*appconfig.ConfigInfo{
			{
				ID:         10,
				ClusterID:  1,
				ConfigType: appconfig.ConfigTypeLog4j2,
				IsTemplate: true,
				Content:    "rootLogger.appenderRef.file.ref = fileAppender",
				Version:    1,
			},
		},
	}
	service.SetRuntimeConfigStore(store)

	result, err := service.SwitchJobLogMode(context.Background(), 1, &SwitchJobLogModeRequest{Mode: "per_job"}, 42)
	if err != nil {
		t.Fatalf("SwitchJobLogMode failed: %v", err)
	}
	if !result.Saved || !result.RestartRequired || result.Mode != "per_job" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if store.lastUpdatedID != 10 {
		t.Fatalf("expected update on config 10, got %d", store.lastUpdatedID)
	}
	if !strings.Contains(store.lastUpdateReq.Content, "routingAppender") {
		t.Fatalf("expected updated content to have routingAppender, got: %s", store.lastUpdateReq.Content)
	}
	if store.lastSyncedType != appconfig.ConfigTypeLog4j2 {
		t.Fatalf("expected sync on ConfigTypeLog4j2, got %s", store.lastSyncedType)
	}
}
