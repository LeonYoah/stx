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
	"strings"
	"testing"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
	"github.com/stretchr/testify/require"
)

// TestAlertRuleCatalogKeysMatchClusterAndTaskRules 锁定目录键集合：等于集群规则目录
// ∪ syncalert.TaskRules()，不含已停用的 stx_* 平台自监控模板键。
// TestAlertRuleCatalogKeysMatchClusterAndTaskRules locks the catalog key set: it equals
// clusterRuleCatalog ∪ syncalert.TaskRules() and excludes the retired stx_* platform
// self-monitoring template keys.
func TestAlertRuleCatalogKeysMatchClusterAndTaskRules(t *testing.T) {
	items := AlertRuleCatalog("")

	want := make(map[string]struct{}, len(clusterRuleCatalog)+len(syncalert.TaskRules()))
	for _, rule := range clusterRuleCatalog {
		want[rule.Key] = struct{}{}
	}
	for _, ruleKey := range syncalert.TaskRules() {
		want[ruleKey] = struct{}{}
	}

	got := make(map[string]struct{}, len(items))
	for _, item := range items {
		require.NotNil(t, item)
		require.False(t, strings.HasPrefix(item.RuleKey, "stx_"), "retired platform template %s must not be in the catalog", item.RuleKey)
		_, dup := got[item.RuleKey]
		require.False(t, dup, "duplicate rule key %s", item.RuleKey)
		got[item.RuleKey] = struct{}{}
	}
	require.Equal(t, want, got)
}

// TestAlertRuleCatalogNamesAndScope 校验名称与 scope/category 派生：英文名取目录 NameEN，
// 中文名取目录 Name；集群规则 scope=cluster，tsdb 引擎归为 metric、其余归为
// platform_health；作业级规则 scope=job 且 category=job。
// TestAlertRuleCatalogNamesAndScope verifies name and scope/category derivation: English
// names come from NameEN and zh names from Name; cluster rules carry scope=cluster with
// tsdb engines mapped to metric and the rest to platform_health; task rules carry
// scope=job and category=job.
func TestAlertRuleCatalogNamesAndScope(t *testing.T) {
	zh := AlertRuleCatalog("")
	en := AlertRuleCatalog("en")
	require.Len(t, zh, len(en))
	zhByKey := make(map[string]*AlertRuleCatalogItem, len(zh))
	for _, item := range zh {
		zhByKey[item.RuleKey] = item
	}
	enByKey := make(map[string]*AlertRuleCatalogItem, len(en))
	for _, item := range en {
		enByKey[item.RuleKey] = item
	}

	taskRules := make(map[string]struct{}, len(syncalert.TaskRules()))
	for _, ruleKey := range syncalert.TaskRules() {
		taskRules[ruleKey] = struct{}{}
	}

	for _, rule := range clusterRuleCatalog {
		zhItem := zhByKey[rule.Key]
		require.NotNil(t, zhItem, "cluster rule %s missing", rule.Key)
		require.Equal(t, rule.Name, zhItem.Name)
		require.Equal(t, "cluster", zhItem.Scope)
		wantCategory := AlertSourceTypePlatformHealth
		if rule.Engine == clusterRuleEngineTSDB {
			wantCategory = AlertSourceTypeMetric
		}
		require.Equal(t, wantCategory, zhItem.Category, "cluster rule %s category", rule.Key)

		enItem := enByKey[rule.Key]
		require.NotNil(t, enItem, "cluster rule %s missing in en catalog", rule.Key)
		wantNameEN := strings.TrimSpace(rule.NameEN)
		if wantNameEN == "" {
			wantNameEN = rule.Name
		}
		require.Equal(t, wantNameEN, enItem.Name)
	}

	for _, ruleKey := range syncalert.TaskRules() {
		zhItem := zhByKey[ruleKey]
		require.NotNil(t, zhItem, "task rule %s missing", ruleKey)
		require.Equal(t, syncAlertRuleName(ruleKey), zhItem.Name)
		require.Equal(t, "job", zhItem.Scope)
		require.Equal(t, AlertSourceTypeJob, zhItem.Category)

		enItem := enByKey[ruleKey]
		require.NotNil(t, enItem, "task rule %s missing in en catalog", ruleKey)
		require.Equal(t, syncAlertRuleNameEN(ruleKey), enItem.Name)
	}
}

// TestAlertRuleCatalogOrderKeepsCatalogFirst 校验输出顺序：集群目录在前、作业级规则在后。
// TestAlertRuleCatalogOrderKeepsCatalogFirst verifies the output order: cluster catalog
// entries first, task rules last.
func TestAlertRuleCatalogOrderKeepsCatalogFirst(t *testing.T) {
	items := AlertRuleCatalog("")
	var want []string
	for _, rule := range clusterRuleCatalog {
		want = append(want, rule.Key)
	}
	want = append(want, syncalert.TaskRules()...)
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.RuleKey)
	}
	require.Equal(t, want, got)
}
