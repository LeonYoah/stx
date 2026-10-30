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

package agent

import (
	"context"
	"testing"

	pb "github.com/LeonYoah/stx/internal/proto/agent"
)

// TestEnrichParamsOnlyAddsMissingKeys 覆盖 AC4c：补全器只新增调用方未给出的键，且不修改调用方的 map。
// TestEnrichParamsOnlyAddsMissingKeys covers AC4c: the enricher only adds keys the caller did not set and never
// mutates the caller's map.
func TestEnrichParamsOnlyAddsMissingKeys(t *testing.T) {
	m := NewManager(nil)
	original := map[string]string{"install_dir": "/opt/st"}
	if got := m.enrichParams(context.Background(), "agent-1", pb.CommandType_START, original); len(got) != 1 {
		t.Fatalf("no enricher must return params unchanged, got %v", got)
	}

	m.SetCommandParamEnricher(func(_ context.Context, agentID string, _ pb.CommandType, params map[string]string) map[string]string {
		if agentID != "agent-1" {
			t.Fatalf("agentID = %s", agentID)
		}
		return map[string]string{"java_home": "/opt/jdk8", "install_dir": "/should/not/override"}
	})
	got := m.enrichParams(context.Background(), "agent-1", pb.CommandType_START, original)
	if got["java_home"] != "/opt/jdk8" || got["install_dir"] != "/opt/st" {
		t.Fatalf("enriched params = %v", got)
	}
	if _, mutated := original["java_home"]; mutated {
		t.Fatalf("caller map must not be mutated: %v", original)
	}

	m.SetCommandParamEnricher(nil)
	if got := m.enrichParams(context.Background(), "agent-1", pb.CommandType_START, original); got["java_home"] != "" {
		t.Fatalf("disabled enricher must not add keys: %v", got)
	}
}
