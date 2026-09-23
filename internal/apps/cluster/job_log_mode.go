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

package cluster

import (
	"context"
	"fmt"
	"strings"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
)

// SwitchJobLogModeRequest represents input to change job log mode.
type SwitchJobLogModeRequest struct {
	Mode string `json:"mode" binding:"required"` // "per_job" or "mixed"
}

// SwitchJobLogModeResult represents the result of changing job log mode.
type SwitchJobLogModeResult struct {
	Saved           bool   `json:"saved"`
	RestartRequired bool   `json:"restart_required"`
	Mode            string `json:"mode"`
	Message         string `json:"message"`
	ConfigVersion   int    `json:"config_version,omitempty"`
}

const defaultRoutingAppenderBlock = `
############################ routing appender for per-job logs #############################
appender.routing.name = routingAppender
appender.routing.type = Routing
appender.routing.purge.type = IdlePurgePolicy
appender.routing.purge.timeToLive = 60
appender.routing.purge.checkInterval = 1
appender.routing.route.type = Routes
appender.routing.route.pattern = $${ctx:ST-JID}
appender.routing.route.system.type = Route
appender.routing.route.system.key = $${ctx:ST-JID}
appender.routing.route.system.ref = fileAppender
appender.routing.route.job.type = Route
appender.routing.route.job.appender.type = File
appender.routing.route.job.appender.name = job-${ctx:ST-JID}
appender.routing.route.job.appender.fileName = ${file_path}/job-${ctx:ST-JID}.log
appender.routing.route.job.appender.layout.type = PatternLayout
appender.routing.route.job.appender.layout.pattern = %d{yyyy-MM-dd HH:mm:ss,SSS} %-5p [%-30.30c{1.}] [%t] - %m%n
#############################################################################################
`

// patchLog4j2JobLogMode replaces rootLogger.appenderRef.file.ref with routingAppender or fileAppender,
// and ensures the routing appender definition block exists when switching to per_job mode.
func patchLog4j2JobLogMode(content string, targetMode string) (string, error) {
	targetMode = strings.ToLower(strings.TrimSpace(targetMode))
	targetRef := "routingAppender"
	if targetMode == "mixed" {
		targetRef = "fileAppender"
	}

	lines := strings.Split(content, "\n")
	foundRef := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "rootLogger.appenderRef.file.ref") {
			lines[i] = fmt.Sprintf("rootLogger.appenderRef.file.ref = %s", targetRef)
			foundRef = true
			break
		}
	}
	if !foundRef {
		lines = append(lines, fmt.Sprintf("rootLogger.appenderRef.file.ref = %s", targetRef))
	}

	result := strings.Join(lines, "\n")

	if targetMode == "per_job" && !strings.Contains(result, "appender.routing.name = routingAppender") {
		result = strings.TrimRight(result, "\r\n") + "\n" + defaultRoutingAppenderBlock
	}

	return result, nil
}

// SwitchJobLogMode switches the cluster's log4j2.properties between per_job and mixed mode,
// creates a new version of the template configuration, and syncs it to all cluster nodes.
func (s *Service) SwitchJobLogMode(
	ctx context.Context,
	clusterID uint,
	req *SwitchJobLogModeRequest,
	userID uint,
) (*SwitchJobLogModeResult, error) {
	if s.runtimeConfigStore == nil {
		return nil, fmt.Errorf("runtime config store is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "per_job" && mode != "mixed" {
		return nil, fmt.Errorf("invalid mode: %s, expected 'per_job' or 'mixed'", mode)
	}

	configs, err := s.runtimeConfigStore.GetByCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	var targetTemplate *appconfig.ConfigInfo
	for _, cfg := range configs {
		if cfg != nil && cfg.ConfigType == appconfig.ConfigTypeLog4j2 && cfg.IsTemplate {
			targetTemplate = cfg
			break
		}
	}
	if targetTemplate == nil {
		return nil, fmt.Errorf("log4j2.properties template not found for cluster %d", clusterID)
	}

	patchedContent, err := patchLog4j2JobLogMode(targetTemplate.Content, mode)
	if err != nil {
		return nil, err
	}

	comment := "一键切换为单 Job 独立日志模式 (routingAppender)"
	if mode == "mixed" {
		comment = "一键切换为共享混合日志模式 (fileAppender)"
	}

	updated, err := s.runtimeConfigStore.Update(ctx, targetTemplate.ID, &appconfig.UpdateConfigRequest{
		Content: patchedContent,
		Comment: comment,
	}, userID)
	if err != nil {
		return nil, err
	}

	// 同步模板至所有节点
	_, _ = s.runtimeConfigStore.SyncTemplateToAllNodes(ctx, clusterID, appconfig.ConfigTypeLog4j2, userID)

	msg := "已切换为单 Job 独立日志模式，生成了新配置版本并同步到各节点，需重启集群后生效。"
	if mode == "mixed" {
		msg = "已切换为共享混合日志模式，生成了新配置版本并同步到各节点，需重启集群后生效。"
	}

	return &SwitchJobLogModeResult{
		Saved:           true,
		RestartRequired: true,
		Mode:            mode,
		Message:         msg,
		ConfigVersion:   updated.Version,
	}, nil
}

// GetJobLogMode returns the configured runtime job log mode for the cluster.
func (s *Service) GetJobLogMode(ctx context.Context, clusterID uint) (string, error) {
	clusterObj, err := s.Get(ctx, clusterID)
	if err != nil {
		return "", err
	}
	if clusterObj != nil && clusterObj.Config != nil {
		if runtimeCfg, ok := clusterObj.Config["runtime"].(map[string]interface{}); ok {
			if mode, ok := runtimeCfg["job_log_mode"].(string); ok && strings.TrimSpace(mode) != "" {
				return mode, nil
			}
		}
	}
	return "mixed", nil
}
