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

package operation

import "fmt"

// monitoringWriteOperationSpecs 登记监控中心非删除写操作。
// monitoringWriteOperationSpecs registers non-delete monitoring write operations.
func monitoringWriteOperationSpecs() []OperationSpec {
	return []OperationSpec{
		monitoringWriteOperation("monitoring.alert-policy.create", []string{"monitoring", "alert-policy", "create"}, "Create an alert policy", "POST", "/api/v1/monitoring/alert-policies", "新增策略会开始参与告警判断和通知。", "stx monitoring alert-policy create --request-file ./policy.json --confirm"),
		monitoringWriteOperation("monitoring.alert-policy.update", []string{"monitoring", "alert-policy", "update"}, "Update an alert policy", "PUT", "/api/v1/monitoring/alert-policies/:id", "修改策略会改变后续告警判断和通知。", "stx monitoring alert-policy update 1 --request-file ./policy.json --confirm"),
		monitoringWriteOperation("monitoring.alert-instance.ack", []string{"monitoring", "alert-instance", "ack"}, "Acknowledge an alert instance", "POST", "/api/v1/monitoring/alert-instances/:id/ack", "确认会记录当前用户和处理说明。", "stx monitoring alert-instance ack alert-1 --note checked --confirm"),
		monitoringWriteOperation("monitoring.alert-instance.silence", []string{"monitoring", "alert-instance", "silence"}, "Silence an alert instance", "POST", "/api/v1/monitoring/alert-instances/:id/silence", "静默期间该告警实例不会继续发送通知。", "stx monitoring alert-instance silence alert-1 --duration-minutes 30 --confirm"),
		monitoringWriteOperation("monitoring.alert-instance.close", []string{"monitoring", "alert-instance", "close"}, "Close an alert instance", "POST", "/api/v1/monitoring/alert-instances/:id/close", "关闭会结束当前告警实例的处理状态。", "stx monitoring alert-instance close alert-1 --note resolved --confirm"),
		monitoringWriteOperation("monitoring.alert.ack", []string{"monitoring", "alert", "ack"}, "Acknowledge a local alert", "POST", "/api/v1/monitoring/alerts/:eventId/ack", "确认会修改本地告警事件的处理状态。", "stx monitoring alert ack 1 --note checked --confirm"),
		monitoringWriteOperation("monitoring.alert.silence", []string{"monitoring", "alert", "silence"}, "Silence a local alert", "POST", "/api/v1/monitoring/alerts/:eventId/silence", "静默期间该本地告警不会继续发送通知。", "stx monitoring alert silence 1 --duration-minutes 30 --confirm"),
		monitoringWriteOperation("monitoring.cluster.rule.update", []string{"monitoring", "cluster", "rule", "update"}, "Update a cluster alert rule", "PUT", "/api/v1/monitoring/clusters/:id/rules/:ruleId", "修改规则会改变集群告警阈值、窗口或启用状态。", "stx monitoring cluster rule update 6 1 --request-file ./rule.json --confirm"),
		monitoringWriteOperation("monitoring.notification-channel.create", []string{"monitoring", "notification-channel", "create"}, "Create a notification channel", "POST", "/api/v1/monitoring/notification-channels", "新增渠道会保存通知地址和相关配置。", "stx monitoring notification-channel create --request-file ./channel.json --confirm"),
		monitoringWriteOperation("monitoring.notification-channel.update", []string{"monitoring", "notification-channel", "update"}, "Update a notification channel", "PUT", "/api/v1/monitoring/notification-channels/:id", "修改渠道会影响后续通知发送。", "stx monitoring notification-channel update 1 --request-file ./channel.json --confirm"),
		monitoringWriteOperation("monitoring.notification-channel.test", []string{"monitoring", "notification-channel", "test"}, "Test a saved notification channel", "POST", "/api/v1/monitoring/notification-channels/:id/test", "测试会向指定渠道真实发送一条测试通知。", "stx monitoring notification-channel test 1 --confirm"),
		monitoringWriteOperation("monitoring.notification-channel.test-draft", []string{"monitoring", "notification-channel", "test-draft"}, "Test a notification channel draft", "POST", "/api/v1/monitoring/notification-channels/test", "测试会使用未保存的渠道配置真实发送一条测试通知。", "stx monitoring notification-channel test-draft --request-file ./draft-test.json --confirm"),
		monitoringWriteOperation("monitoring.notification-channel.test-connection", []string{"monitoring", "notification-channel", "test-connection"}, "Test notification channel connectivity", "POST", "/api/v1/monitoring/notification-channels/test-connection", "连接测试会访问渠道配置中的远端服务。", "stx monitoring notification-channel test-connection --request-file ./channel.json --confirm"),
		monitoringWriteOperation("monitoring.notification-route.create", []string{"monitoring", "notification-route", "create"}, "Create a notification route", "POST", "/api/v1/monitoring/notification-routes", "新增路由会改变告警匹配到通知渠道的方式。", "stx monitoring notification-route create --request-file ./route.json --confirm"),
		monitoringWriteOperation("monitoring.notification-route.update", []string{"monitoring", "notification-route", "update"}, "Update a notification route", "PUT", "/api/v1/monitoring/notification-routes/:id", "修改路由会改变告警匹配到通知渠道的方式。", "stx monitoring notification-route update 1 --request-file ./route.json --confirm"),
	}
}

// monitoringWriteOperation 构造由专用 CLI 处理的监控写操作登记。
// monitoringWriteOperation builds a monitoring write registration handled by dedicated CLI code.
func monitoringWriteOperation(id string, commandPath []string, summary, method, route, impact, example string) OperationSpec {
	return OperationSpec{
		ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: false, Method: method, Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: RiskR1, Revision: 1, SupportsPick: true,
		Impact: &ImpactSpec{Level: RiskR1, Message: impact},
		Input: []InputSpec{
			{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key"},
			{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation"},
		},
		Example:       example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{},"result_meta":{"complete":true}}`, id),
	}
}
