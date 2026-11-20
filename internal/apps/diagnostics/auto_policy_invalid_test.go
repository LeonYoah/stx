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
		wantReason string
		wantDetail string
	}{
		{
			name: "no conditions",
		},
		{
			name: "legacy PROM template",
			conditions: InspectionConditionItems{
				{TemplateCode: "PROM_CPU_HIGH", Enabled: true},
			},
			wantReason: AutoPolicyInvalidLegacyCondition,
			wantDetail: "PROM_CPU_HIGH",
		},
		{
			name: "legacy NODE_UNHEALTHY",
			conditions: InspectionConditionItems{
				{TemplateCode: "NODE_UNHEALTHY", Enabled: true},
			},
			wantReason: AutoPolicyInvalidLegacyCondition,
			wantDetail: "NODE_UNHEALTHY",
		},
		{
			name: "ALERT_FIRING without rule keys",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true},
			},
			wantReason: AutoPolicyInvalidAlertFiringNoRules,
			wantDetail: "ALERT_FIRING",
		},
		{
			name: "ALERT_FIRING blank rule keys",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{" ", ""}},
			},
			wantReason: AutoPolicyInvalidAlertFiringNoRules,
			wantDetail: "ALERT_FIRING",
		},
		{
			name: "ALERT_FIRING with retired stx_ rule key",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"stx_server_memory_high"}},
			},
			wantReason: AutoPolicyInvalidUnknownAlertRuleKey,
			wantDetail: "stx_server_memory_high",
		},
		{
			name: "ALERT_FIRING first unknown key wins",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"node_offline", "stx_agent_memory_high", "stx_tsdb_compaction_failed"}},
			},
			wantReason: AutoPolicyInvalidUnknownAlertRuleKey,
			wantDetail: "stx_agent_memory_high",
		},
		{
			name: "ALERT_FIRING with rule keys",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"node_offline"}},
			},
		},
		{
			name: "ALERT_FIRING with new catalog keys",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeAlertFiring, Enabled: true, RuleKeys: []string{"gc_time_high", "coordinator_backlog_growing"}},
			},
		},
		{
			name: "scheduled only",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeScheduled, Enabled: true},
			},
		},
		{
			name: "valid plus invalid mix",
			conditions: InspectionConditionItems{
				{TemplateCode: ConditionCodeScheduled, Enabled: true},
				{TemplateCode: "PROM_MEM_HIGH", Enabled: true},
			},
			wantReason: AutoPolicyInvalidLegacyCondition,
			wantDetail: "PROM_MEM_HIGH",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := &InspectionAutoPolicy{ID: 1, Name: "p", Conditions: tc.conditions}
			require.Equal(t, tc.wantReason, policy.InvalidReason())
			require.Equal(t, tc.wantDetail, policy.InvalidDetail())
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
	require.Equal(t, AutoPolicyInvalidLegacyCondition, info.InvalidReason)
	require.Equal(t, "PROM_CPU_HIGH", info.InvalidDetail)

	valid := &InspectionAutoPolicy{
		ID:         4,
		Name:       "ok",
		Conditions: InspectionConditionItems{{TemplateCode: ConditionCodeScheduled, Enabled: true}},
	}
	info = valid.ToInfo()
	require.NotNil(t, info)
	require.Empty(t, info.InvalidReason)
	require.Empty(t, info.InvalidDetail)
}
