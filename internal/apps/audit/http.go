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

package audit

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// RecordFromGin writes an audit log entry from an HTTP request (user from session, IP, User-Agent from request).
// RecordFromGin 根据 HTTP 请求写入审计日志（用户来自 session，IP、User-Agent 来自请求）。
// userID and username should be obtained via auth.GetUserIDFromContext(c) and auth.GetUsernameFromContext(c).
// If repo is nil, the function no-ops and returns nil.
func RecordFromGin(c *gin.Context, repo *Repository, userID uint64, username, action, resourceType, resourceID, resourceName string, details AuditDetails) error {
	if repo == nil {
		return nil
	}
	if action == "" || resourceType == "" {
		return nil
	}
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	if len(ua) > 500 {
		ua = ua[:500]
	}
	var uid *uint
	if userID > 0 {
		u := uint(userID)
		uid = &u
	}
	requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
	if requestID == "" && c.Request != nil {
		requestID = CommandMetadataFromContext(c.Request.Context()).RequestID
	}
	log := &AuditLog{
		UserID:       uid,
		Username:     username,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		RequestID:    requestID,
		ClientType:   auditClientType(c.GetHeader("X-STX-Client"), ua),
		Details:      RedactDetails(details),
		IPAddress:    ip,
		UserAgent:    ua,
	}
	if details != nil {
		if v, ok := details["trigger"]; ok {
			if s, ok := v.(string); ok && (s == "auto" || s == "manual") {
				log.Trigger = s
			}
		}
		applyAuditDetailLinks(log, details)
	}
	return repo.CreateAuditLog(c.Request.Context(), log)
}

// RecordFromGinNoUser is like RecordFromGin but accepts *http.Request for context (e.g. when no gin context).
// Prefer RecordFromGin when you have gin.Context.
func RecordFromGinNoUser(repo *Repository, req *http.Request, userID uint64, username, action, resourceType, resourceID, resourceName string, details AuditDetails) error {
	if repo == nil || req == nil {
		return nil
	}
	if action == "" || resourceType == "" {
		return nil
	}
	ip := req.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = req.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = req.RemoteAddr
	}
	ua := req.Header.Get("User-Agent")
	if len(ua) > 500 {
		ua = ua[:500]
	}
	var uid *uint
	if userID > 0 {
		u := uint(userID)
		uid = &u
	}
	log := &AuditLog{
		UserID:       uid,
		Username:     username,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		RequestID:    strings.TrimSpace(req.Header.Get("X-Request-ID")),
		ClientType:   auditClientType(req.Header.Get("X-STX-Client"), ua),
		Details:      RedactDetails(details),
		IPAddress:    ip,
		UserAgent:    ua,
	}
	if details != nil {
		if v, ok := details["trigger"]; ok {
			if s, ok := v.(string); ok && (s == "auto" || s == "manual") {
				log.Trigger = s
			}
		}
		applyAuditDetailLinks(log, details)
	}
	return repo.CreateAuditLog(req.Context(), log)
}

// UintID formats a uint as string for ResourceID.
func UintID(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func applyAuditDetailLinks(log *AuditLog, details AuditDetails) {
	if log == nil || details == nil {
		return
	}
	log.ExecutionID = auditDetailString(details, "execution_id")
	log.CommandID = auditDetailString(details, "command_id")
	log.RiskLevel = auditDetailString(details, "risk_level")
	log.ResultStatus = auditDetailString(details, "result_status")
	if log.RequestID == "" {
		log.RequestID = auditDetailString(details, "request_id")
	}
}

func auditDetailString(details AuditDetails, key string) string {
	value, ok := details[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func auditClientType(marker, userAgent string) string {
	if strings.EqualFold(strings.TrimSpace(marker), "cli") {
		return "cli"
	}
	if strings.TrimSpace(userAgent) == "" {
		return "api"
	}
	return "web"
}
