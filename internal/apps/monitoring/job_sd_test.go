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
	"testing"

	"github.com/LeonYoah/stx/internal/config"
)

func TestBuildSTXSyncJobsSDTarget(t *testing.T) {
	original := config.Config.App.ExternalURL
	t.Cleanup(func() { config.Config.App.ExternalURL = original })

	config.Config.App.ExternalURL = ""
	if got := buildSTXSyncJobsSDTarget(); got != nil {
		t.Fatalf("expected nil without external_url, got %+v", got)
	}

	config.Config.App.ExternalURL = "https://stx.example.com:17800"
	got := buildSTXSyncJobsSDTarget()
	if got == nil || len(got.Targets) != 1 || got.Targets[0] != "stx.example.com:17800" {
		t.Fatalf("unexpected targets: %+v", got)
	}
	if got.Labels["job"] != "stx_sync_jobs" {
		t.Fatalf("job label=%q", got.Labels["job"])
	}
	if got.Labels["__metrics_path__"] != "/api/v1/monitoring/prometheus/jobs/metrics" {
		t.Fatalf("metrics path=%q", got.Labels["__metrics_path__"])
	}
	if got.Labels["__scheme__"] != "https" {
		t.Fatalf("scheme=%q", got.Labels["__scheme__"])
	}
}
