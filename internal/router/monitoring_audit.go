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

package router

import (
	"net/http"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

type monitoringAuditTarget struct {
	operationID  string
	action       string
	resourceType string
	resourceID   string
}

// monitoringAuditMiddleware 在监控写请求成功后保存统一审计记录。
// monitoringAuditMiddleware stores a common audit row after a successful monitoring write.
func monitoringAuditMiddleware(repo *audit.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if repo == nil || c.Writer.Status() >= http.StatusBadRequest {
			return
		}
		target, ok := monitoringAuditTargetFor(c)
		if !ok {
			return
		}
		_ = audit.RecordFromGin(c, repo, auth.GetUserIDFromContext(c), auth.GetUsernameFromContext(c),
			target.action, target.resourceType, target.resourceID, target.operationID, audit.AuditDetails{
				"trigger":       "manual",
				"operation_id":  target.operationID,
				"risk_level":    "R1",
				"result_status": "succeeded",
			})
	}
}

// monitoringAuditTargetFor 将监控路由转换为稳定的审计动作和资源标识。
// monitoringAuditTargetFor maps a monitoring route to stable audit action and resource identifiers.
func monitoringAuditTargetFor(c *gin.Context) (monitoringAuditTarget, bool) {
	if c == nil || c.Request == nil {
		return monitoringAuditTarget{}, false
	}
	method := strings.ToUpper(c.Request.Method)
	path := c.FullPath()
	targets := map[string]monitoringAuditTarget{
		http.MethodPost + " /api/v1/monitoring/alert-policies":                        {operationID: "monitoring.alert-policy.create", action: "create", resourceType: "monitoring_alert_policy"},
		http.MethodPut + " /api/v1/monitoring/alert-policies/:id":                     {operationID: "monitoring.alert-policy.update", action: "update", resourceType: "monitoring_alert_policy"},
		http.MethodDelete + " /api/v1/monitoring/alert-policies/:id":                  {operationID: "monitoring.alert-policy.delete", action: "delete", resourceType: "monitoring_alert_policy"},
		http.MethodPost + " /api/v1/monitoring/alert-instances/:id/ack":               {operationID: "monitoring.alert-instance.ack", action: "ack", resourceType: "monitoring_alert_instance"},
		http.MethodPost + " /api/v1/monitoring/alert-instances/:id/silence":           {operationID: "monitoring.alert-instance.silence", action: "silence", resourceType: "monitoring_alert_instance"},
		http.MethodPost + " /api/v1/monitoring/alert-instances/:id/close":             {operationID: "monitoring.alert-instance.close", action: "close", resourceType: "monitoring_alert_instance"},
		http.MethodPost + " /api/v1/monitoring/alerts/:eventId/ack":                   {operationID: "monitoring.alert.ack", action: "ack", resourceType: "monitoring_alert"},
		http.MethodPost + " /api/v1/monitoring/alerts/:eventId/silence":               {operationID: "monitoring.alert.silence", action: "silence", resourceType: "monitoring_alert"},
		http.MethodPut + " /api/v1/monitoring/clusters/:id/rules/:ruleId":             {operationID: "monitoring.cluster.rule.update", action: "update", resourceType: "monitoring_cluster_rule"},
		http.MethodPost + " /api/v1/monitoring/notification-channels":                 {operationID: "monitoring.notification-channel.create", action: "create", resourceType: "monitoring_notification_channel"},
		http.MethodPut + " /api/v1/monitoring/notification-channels/:id":              {operationID: "monitoring.notification-channel.update", action: "update", resourceType: "monitoring_notification_channel"},
		http.MethodDelete + " /api/v1/monitoring/notification-channels/:id":           {operationID: "monitoring.notification-channel.delete", action: "delete", resourceType: "monitoring_notification_channel"},
		http.MethodPost + " /api/v1/monitoring/notification-channels/:id/test":        {operationID: "monitoring.notification-channel.test", action: "test", resourceType: "monitoring_notification_channel"},
		http.MethodPost + " /api/v1/monitoring/notification-channels/test":            {operationID: "monitoring.notification-channel.test-draft", action: "test", resourceType: "monitoring_notification_channel_draft", resourceID: "draft"},
		http.MethodPost + " /api/v1/monitoring/notification-channels/test-connection": {operationID: "monitoring.notification-channel.test-connection", action: "test_connection", resourceType: "monitoring_notification_channel_draft", resourceID: "connection"},
		http.MethodPost + " /api/v1/monitoring/notification-routes":                   {operationID: "monitoring.notification-route.create", action: "create", resourceType: "monitoring_notification_route"},
		http.MethodPut + " /api/v1/monitoring/notification-routes/:id":                {operationID: "monitoring.notification-route.update", action: "update", resourceType: "monitoring_notification_route"},
		http.MethodDelete + " /api/v1/monitoring/notification-routes/:id":             {operationID: "monitoring.notification-route.delete", action: "delete", resourceType: "monitoring_notification_route"},
	}
	target, ok := targets[method+" "+path]
	if !ok {
		return monitoringAuditTarget{}, false
	}
	if target.resourceID == "" {
		switch target.resourceType {
		case "monitoring_cluster_rule":
			target.resourceID = strings.TrimSpace(c.Param("id")) + "/" + strings.TrimSpace(c.Param("ruleId"))
		case "monitoring_alert":
			target.resourceID = strings.TrimSpace(c.Param("eventId"))
		default:
			target.resourceID = strings.TrimSpace(c.Param("id"))
		}
	}
	return target, true
}
