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

// AlertRuleCatalogItem 是告警规则目录的一项：规则键、显示名与所属告警类别，作为巡检条件、
// 通知路由 rule_key 与 CLI 的唯一取值来源。
// AlertRuleCatalogItem is one alert rule catalog entry: rule key, display name and category.
// It is the single source of truth for inspection conditions, notification route rule keys and the CLI.
type AlertRuleCatalogItem struct {
	RuleKey  string          `json:"rule_key"`
	Name     string          `json:"name"`
	Category AlertSourceType `json:"category"`
}

// AlertRuleCatalog 返回全部可选规则键的目录：平台健康与指标规则来自策略模板，作业规则来自
// syncalert 任务规则；按 rule_key 去重，集群级作业规则已随平台健康模板出现，不重复列出。
// AlertRuleCatalog lists every selectable rule key: platform-health and metric entries come
// from policy templates, job rules from syncalert task rules; entries are deduped by rule key
// and cluster-level sync rules already appear via the platform health templates.
func AlertRuleCatalog(lang string) []*AlertRuleCatalogItem {
	english := strings.EqualFold(strings.TrimSpace(lang), "en")
	items := make([]*AlertRuleCatalogItem, 0, 16)
	seen := make(map[string]bool, 16)
	for _, tpl := range allAlertPolicyTemplateSummaries() {
		if tpl == nil || tpl.Key == "" || seen[tpl.Key] {
			continue
		}
		var category AlertSourceType
		switch AlertPolicyBuilderKind(strings.TrimSpace(tpl.SourceKind)) {
		case AlertPolicyBuilderKindPlatformHealth:
			category = AlertSourceTypePlatformHealth
		case AlertPolicyBuilderKindMetricsTemplate:
			category = AlertSourceTypeMetric
		default:
			continue
		}
		seen[tpl.Key] = true
		name := strings.TrimSpace(tpl.Name)
		if !english {
			// 中文名与告警中心一致：作业类规则键走 sync 目录名，其余走模板中文名。
			// zh names match the alert center: sync rule keys take the sync catalog
			// name, everything else the template zh name.
			if syncName, ok := syncAlertRuleCatalog[tpl.Key]; ok {
				name = syncName.Name
			} else {
				name = alertPolicyTemplateDisplayNameZH(tpl.Key)
			}
		}
		items = append(items, &AlertRuleCatalogItem{RuleKey: tpl.Key, Name: name, Category: category})
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
		items = append(items, &AlertRuleCatalogItem{RuleKey: ruleKey, Name: name, Category: AlertSourceTypeJob})
	}
	return items
}
