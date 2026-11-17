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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInspectionAutoPolicyInvalidReason 覆盖失效判定：旧 PROM_* / NODE_UNHEALTHY 模板、
// 未选规则的 ALERT_FIRING 判失效；合法模板与选了规则的 ALERT_FIRING 有效。
// TestInspectionAutoPolicyInvalidReason covers the invalid judgment: retired PROM_* /
// NODE_UNHEALTHY templates and ALERT_FIRING without rule keys are invalid; supported
// templates and ALERT_FIRING with rule keys stay valid.
func TestInspectionAutoPolicyInvalidReason(t *testing.T) {
	cases := []struct {
		name       string
		conditions InspectionConditionItems
		wantValid  bool
		wantSubstr string
	}{
		{
			name:      "no conditions",
			wantValid: true,
		},
		{
			name: "legacy PROM template",
			conditions: InspectionConditionItems{
				{TemplateCode: "PROM_CPU_HIGH", Enabled: true},
			},
			wantSubstr: "PROM_CPU_HIGH",
		},
		{
			name: "legacy NODE_UNHEALTHY",
			conditions: InspectionConditionItems{
				{TemplateCode: "NODE_UNHEALTHY", Enabled: true},
			},
			wantSubstr: "NODE_UNHEALTHY",
		},
		{
			name: "ALERT_FIRING without rule keys",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true},
			},
			wantSubstr: "ALERT_FIRING",
		},
		{
			name: "ALERT_FIRING blank rule keys",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{" ", ""}},
			},
			wantSubstr: "ALERT_FIRING",
		},
		{
			name: "ALERT_FIRING with rule keys",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"node_offline"}},
			},
			wantValid: true,
		},
		{
			name: "scheduled only",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeScheduled, Enabled: true},
			},
			wantValid: true,
		},
		{
			name: "valid plus invalid mix",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeScheduled, Enabled: true},
				{TemplateCode: "PROM_MEM_HIGH", Enabled: true},
			},
			wantSubstr: "PROM_MEM_HIGH",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := &InspectionAutoPolicy{ID: 1, Name: "p", Conditions: tc.conditions}
			reason := policy.InvalidReason()
			if tc.wantValid {
				require.Empty(t, reason)
			} else {
				require.NotEmpty(t, reason)
				require.Contains(t, reason, tc.wantSubstr)
			}
		})
	}
}

// TestToInfoCarriesInvalidReason 验证 API 视图携带失效原因。
// TestToInfoCarriesInvalidReason verifies the API view carries the invalid reason.
func TestToInfoCarriesInvalidReason(t *testing.T) {
	policy := &InspectionAutoPolicy{
		ID:         3,
		Name:       "legacy",
		Conditions: InspectionConditionItems{{TemplateCode: "PROM_CPU_HIGH", Enabled: true}},
	}
	info := policy.ToInfo()
	require.NotNil(t, info)
	require.NotEmpty(t, info.InvalidReason)
	require.Contains(t, info.InvalidReason, "PROM_CPU_HIGH")

	valid := &InspectionAutoPolicy{
		ID:         4,
		Name:       "ok",
		Conditions: InspectionConditionItems{{TemplateCode: ConditionCodeScheduled, Enabled: true}},
	}
	info = valid.ToInfo()
	require.NotNil(t, info)
	require.Empty(t, info.InvalidReason)
}
