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

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// AlertRuleCatalogItem 是告警规则目录的一项：规则键、显示名与所属告警类别，作为巡检条件
// 与 CLI 的唯一取值来源。
// AlertRuleCatalogItem is one alert rule catalog entry: rule key, display name and category.
// It is the single source of truth for inspection conditions and the CLI.
type AlertRuleCatalogItem struct {
	RuleKey  string          `json:"rule_key"`
	Name     string          `json:"name"`
	Category AlertSourceType `json:"category"`
	// Scope 由类别派生：job → job，其余 → cluster。
	// Scope derives from the category: job → job, everything else → cluster.
	Scope string `json:"scope"`
}

// alertScopeFromSourceType 把告警类别映射为聚合 scope。
// alertScopeFromSourceType maps an alert category onto its aggregate scope.
func alertScopeFromSourceType(sourceType AlertSourceType) string {
	if sourceType == AlertSourceTypeJob {
		return "job"
	}
	return clusterRuleScope
}

// alertCategoryFromClusterEngine 把集群规则的判定引擎映射回兼容的告警类别：
// tsdb 引擎 → metric，其余（availability / event / sync）→ platform_health。
// alertCategoryFromClusterEngine maps a cluster rule's engine back to the compatible alert
// category: tsdb → metric, everything else (availability / event / sync) → platform_health.
func alertCategoryFromClusterEngine(engine clusterRuleEngine) AlertSourceType {
	if engine == clusterRuleEngineTSDB {
		return AlertSourceTypeMetric
	}
	return AlertSourceTypePlatformHealth
}

// AlertRuleCatalog 返回全部可选规则键的目录：集群级规则按 clusterRuleCatalog 顺序输出，
// 名称取目录 Name/NameEN；作业级规则来自 syncalert.TaskRules()；按 rule_key 去重。
// AlertRuleCatalog lists every selectable rule key: cluster-level entries follow
// clusterRuleCatalog order and take their names from Name/NameEN; job-level entries come
// from syncalert.TaskRules(); entries are deduped by rule key.
func AlertRuleCatalog(lang string) []*AlertRuleCatalogItem {
	english := strings.EqualFold(strings.TrimSpace(lang), "en")
	items := make([]*AlertRuleCatalogItem, 0, len(clusterRuleCatalog)+len(syncalert.TaskRules()))
	seen := make(map[string]bool, len(clusterRuleCatalog)+len(syncalert.TaskRules()))
	for _, rule := range clusterRuleCatalog {
		if rule == nil || rule.Key == "" || seen[rule.Key] {
			continue
		}
		seen[rule.Key] = true
		name := rule.Name
		if english && strings.TrimSpace(rule.NameEN) != "" {
			name = rule.NameEN
		}
		items = append(items, &AlertRuleCatalogItem{
			RuleKey:  rule.Key,
			Name:     name,
			Category: alertCategoryFromClusterEngine(rule.Engine),
			Scope:    clusterRuleScope,
		})
	}
	for _, ruleKey := range syncalert.TaskRules() {
		if seen[ruleKey] {
			continue
		}
		seen[ruleKey] = true
		name := syncAlertRuleNameEN(ruleKey)
		if !english {
			name = syncAlertRuleName(ruleKey)
		}
		items = append(items, &AlertRuleCatalogItem{RuleKey: ruleKey, Name: name, Category: AlertSourceTypeJob, Scope: "job"})
	}
	return items
}
