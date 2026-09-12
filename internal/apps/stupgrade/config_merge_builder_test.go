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
package stupgrade

import (
	"strings"
	"testing"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
)

func TestBuildConfigMergeInputs_deduplicatesByConfigType(t *testing.T) {
	hostID := uint(101)
	inputs, issues := buildConfigMergeInputs([]*appconfig.ConfigInfo{
		{
			ConfigType: appconfig.ConfigTypeSeatunnel,
			FilePath:   appconfig.GetConfigFilePath(appconfig.ConfigTypeSeatunnel),
			Content:    "env: base",
			IsTemplate: true,
		},
		{
			ConfigType: appconfig.ConfigTypeSeatunnel,
			FilePath:   appconfig.GetConfigFilePath(appconfig.ConfigTypeSeatunnel),
			Content:    "env: local",
			HostID:     &hostID,
		},
		{
			ConfigType: appconfig.ConfigTypeHazelcast,
			FilePath:   appconfig.GetConfigFilePath(appconfig.ConfigTypeHazelcast),
			Content:    "cluster-name: stx",
			IsTemplate: true,
		},
	})

	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected 2 config merge inputs, got %d", len(inputs))
	}

	seatunnelInput := inputs[1]
	if seatunnelInput.ConfigType != string(appconfig.ConfigTypeSeatunnel) {
		t.Fatalf("expected seatunnel input, got %q", seatunnelInput.ConfigType)
	}
	if seatunnelInput.BaseContent != "env: base" {
		t.Fatalf("expected base content to come from template, got %q", seatunnelInput.BaseContent)
	}
	if seatunnelInput.LocalContent != "env: local" {
		t.Fatalf("expected local content to come from node override, got %q", seatunnelInput.LocalContent)
	}
	if seatunnelInput.TargetPath != "config/seatunnel.yaml" {
		t.Fatalf("expected normalized target path, got %q", seatunnelInput.TargetPath)
	}
}

func TestBuildConfigMergeInputs_multipleNodeVariantsReturnBlockingIssue(t *testing.T) {
	hostIDA := uint(101)
	hostIDB := uint(102)
	_, issues := buildConfigMergeInputs([]*appconfig.ConfigInfo{
		{
			ConfigType: appconfig.ConfigTypeSeatunnel,
			FilePath:   appconfig.GetConfigFilePath(appconfig.ConfigTypeSeatunnel),
			Content:    "env: node-a",
			HostID:     &hostIDA,
		},
		{
			ConfigType: appconfig.ConfigTypeSeatunnel,
			FilePath:   appconfig.GetConfigFilePath(appconfig.ConfigTypeSeatunnel),
			Content:    "env: node-b",
			HostID:     &hostIDB,
		},
	})

	assertIssueCode(t, issues, "config_node_variants")
}

func TestBuildConfigMergeFile_keepsIdenticalContentsWithoutConflict(t *testing.T) {
	file := buildConfigMergeFile(configMergeInput{
		ConfigType:   string(appconfig.ConfigTypeSeatunnel),
		TargetPath:   "config/seatunnel.yaml",
		BaseContent:  "parallelism: 1\njob.mode: batch",
		LocalContent: "parallelism: 1\njob.mode: batch",
	}, "parallelism: 1\njob.mode: batch")

	if file.ConflictCount != 0 {
		t.Fatalf("expected no conflicts, got %d", file.ConflictCount)
	}
	if !file.Resolved {
		t.Fatalf("expected file to stay resolved when old and new values are identical")
	}
	if file.MergedContent != "parallelism: 1\njob.mode: batch" {
		t.Fatalf("expected merged content to keep identical values, got %q", file.MergedContent)
	}
}

func TestBuildConfigMergeFile_marksTargetChangeAsPendingConflict(t *testing.T) {
	file := buildConfigMergeFile(configMergeInput{
		ConfigType:   string(appconfig.ConfigTypeSeatunnel),
		TargetPath:   "config/seatunnel.yaml",
		BaseContent:  "parallelism: 1\njob.mode: batch",
		LocalContent: "parallelism: 1\njob.mode: batch",
	}, "parallelism: 2\njob.mode: batch")

	if file.ConflictCount != 1 {
		t.Fatalf("expected 1 conflict when old and new values differ, got %d", file.ConflictCount)
	}
	if file.Resolved {
		t.Fatalf("expected unresolved file when target value changes")
	}
	if len(file.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict entry, got %d", len(file.Conflicts))
	}
	if file.Conflicts[0].Status != ConfigConflictPending {
		t.Fatalf("expected conflict status pending, got %q", file.Conflicts[0].Status)
	}
}

func TestBuildConfigMergeFile_marksDivergentLinesAsPendingConflict(t *testing.T) {
	file := buildConfigMergeFile(configMergeInput{
		ConfigType:   string(appconfig.ConfigTypeSeatunnel),
		TargetPath:   "config/seatunnel.yaml",
		BaseContent:  "parallelism: 1\njob.mode: batch",
		LocalContent: "parallelism: 4\njob.mode: batch",
	}, "parallelism: 2\njob.mode: batch")

	if file.ConflictCount != 1 {
		t.Fatalf("expected 1 conflict, got %d", file.ConflictCount)
	}
	if file.Resolved {
		t.Fatalf("expected unresolved file when local and target diverge")
	}
	if len(file.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict entry, got %d", len(file.Conflicts))
	}
	if file.Conflicts[0].Status != ConfigConflictPending {
		t.Fatalf("expected conflict status pending, got %q", file.Conflicts[0].Status)
	}
	if !strings.Contains(file.MergedContent, "<<<<<<< LOCAL") {
		t.Fatalf("expected merged content to include LOCAL marker, got %q", file.MergedContent)
	}
	if !strings.Contains(file.MergedContent, ">>>>>>> TARGET") {
		t.Fatalf("expected merged content to include TARGET marker, got %q", file.MergedContent)
	}
}
