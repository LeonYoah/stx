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

import "testing"

func TestDiagnosticResourceRegistryExcludesInternalSteps(t *testing.T) {
	resources := ListDiagnosticResources()
	if len(resources) != 7 {
		t.Fatalf("公开诊断资源数量错误 / unexpected public diagnostics resource count: %d", len(resources))
	}
	for _, resource := range resources {
		if resource.StepCode == DiagnosticStepCodeAssembleManifest || resource.StepCode == DiagnosticStepCodeRenderHTMLSummary || resource.StepCode == DiagnosticStepCodeComplete {
			t.Fatalf("内部流程步骤不应公开为诊断资源 / internal workflow step must not be public: %+v", resource)
		}
	}
}

func TestDefaultDiagnosticTaskOptionsMatchesLegacyBundle(t *testing.T) {
	options := DefaultDiagnosticTaskOptions()
	if len(options.SelectedResources) != 6 {
		t.Fatalf("默认资源数量错误 / unexpected default resource count: %d", len(options.SelectedResources))
	}
	if !options.IncludeThreadDump {
		t.Fatal("默认应采集线程快照 / thread dump should be selected by default")
	}
	if options.IncludeJVMDump || containsDiagnosticResource(options.SelectedResources, DiagnosticResourceJVMDump) {
		t.Fatal("JVM Dump 不应默认执行 / JVM dump must not be selected by default")
	}
	for _, resource := range ListDiagnosticResources() {
		if resource.DefaultSelected != containsDiagnosticResource(options.SelectedResources, resource.Code) {
			t.Fatalf("资源默认标记与任务选项不一致 / resource default does not match task options: %s", resource.Code)
		}
	}
}

func TestResourceOnlySelectionSkipsUnselectedAndBundleSteps(t *testing.T) {
	options := DiagnosticTaskOptions{
		SelectedResources: []DiagnosticResourceCode{DiagnosticResourceThreadDump},
		ResourceOnly:      true,
	}.Normalize()
	if !options.IncludeThreadDump {
		t.Fatal("选择线程快照后应自动启用对应步骤 / selecting thread dump must enable its step")
	}
	if _, skipped := shouldSkipDiagnosticPlanStep(DiagnosticStepCodeCollectThreadDump, options); skipped {
		t.Fatal("已选择的线程快照不应跳过 / selected thread dump must not be skipped")
	}
	for _, code := range []DiagnosticStepCode{DiagnosticStepCodeCollectLogSample, DiagnosticStepCodeAssembleManifest, DiagnosticStepCodeRenderHTMLSummary} {
		if _, skipped := shouldSkipDiagnosticPlanStep(code, options); !skipped {
			t.Fatalf("单项资源任务应跳过步骤 / resource-only task should skip step: %s", code)
		}
	}
	if _, skipped := shouldSkipDiagnosticPlanStep(DiagnosticStepCodeComplete, options); skipped {
		t.Fatal("完成步骤仍需执行 / completion step must still run")
	}
}

func TestValidateDiagnosticResourceSelectionRejectsUnknownCode(t *testing.T) {
	err := validateDiagnosticResourceSelection(DiagnosticTaskOptions{SelectedResources: []DiagnosticResourceCode{"unknown"}})
	if err == nil {
		t.Fatal("未知诊断资源应被拒绝 / unknown diagnostics resource must be rejected")
	}
}
