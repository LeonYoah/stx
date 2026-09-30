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

func TestSupplementConfigMergeInputs_addsSeatunnelEnvWhenPackageHasIt(t *testing.T) {
	inputs := []configMergeInput{{
		ConfigType:   string(appconfig.ConfigTypeSeatunnel),
		TargetPath:   "config/seatunnel.yaml",
		LocalContent: "seatunnel: local",
	}}
	targetContents := map[string]string{
		"config/seatunnel.yaml":  "seatunnel: target",
		"config/seatunnel-env.sh": "export JAVA_OPTS=1\n",
	}

	got := supplementConfigMergeInputs(inputs, "hybrid", nil, targetContents)
	foundEnv := false
	for _, input := range got {
		if input.ConfigType == string(appconfig.ConfigTypeSeatunnelEnv) {
			foundEnv = true
			if input.TargetPath != "config/seatunnel-env.sh" {
				t.Fatalf("unexpected env target path: %q", input.TargetPath)
			}
		}
	}
	if !foundEnv {
		t.Fatalf("expected seatunnel-env.sh to be backfilled from package contents")
	}
}

func TestSupplementConfigMergeInputs_skipsMissingModeConfigs(t *testing.T) {
	inputs := []configMergeInput{{
		ConfigType:   string(appconfig.ConfigTypeSeatunnel),
		TargetPath:   "config/seatunnel.yaml",
		LocalContent: "seatunnel: local",
	}}
	got := supplementConfigMergeInputs(inputs, "hybrid", nil, map[string]string{
		"config/seatunnel.yaml": "seatunnel: target",
	})
	if len(got) != 1 {
		t.Fatalf("expected no extra mode configs when package/source lack them, got %d", len(got))
	}
}

func TestSoftenOptionalPackageConfigIssues(t *testing.T) {
	issues := []BlockingIssue{
		blockingIssue(CheckCategoryPackage, "package_config_missing", "missing env", map[string]string{
			"target_path": "config/seatunnel-env.sh",
		}),
		blockingIssue(CheckCategoryPackage, "package_config_missing", "missing yaml", map[string]string{
			"target_path": "config/seatunnel.yaml",
		}),
	}
	got := softenOptionalPackageConfigIssues(issues)
	if len(got) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(got))
	}
	if got[0].Blocking {
		t.Fatalf("expected seatunnel-env.sh missing to be non-blocking")
	}
	if !got[1].Blocking {
		t.Fatalf("expected seatunnel.yaml missing to stay blocking")
	}
}
