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

package config

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const (
	configExecutionModule      = "config"
	configUpdateImpact         = "修改配置会创建新版本；节点配置还会尝试写入对应 SeaTunnel 节点。"
	configRollbackImpact       = "回滚会把指定历史版本复制为新版本；节点配置还会尝试写入对应 SeaTunnel 节点。"
	configPromoteImpact        = "提升节点配置会替换同类型集群模板，并更新数据库中的所有节点配置。"
	configSyncImpact           = "同步会用集群模板替换节点配置，并尝试写入对应 SeaTunnel 节点。"
	configPushImpact           = "推送会直接覆盖目标节点安装目录中的配置文件，可能影响后续启动或重启。"
	configClusterInitImpact    = "初始化会从目标节点读取配置，并在 STX 中创建或更新集群模板和节点配置。"
	configClusterSyncAllImpact = "批量同步会更新所有节点的同类型配置，并逐台写入 SeaTunnel 安装目录。"
)

// beginConfigWrite 为 CLI 和直接 API 写请求执行确认与幂等检查，网页请求保持原有行为。
// beginConfigWrite applies confirmation and idempotency checks to CLI and direct API writes while preserving web behavior.
func (h *Handler) beginConfigWrite(c *gin.Context, operationID, moduleRef string, risk executionapp.RiskLevel, impact string, payload any) (*executionapp.Execution, bool, error) {
	metadata := executionapp.MetadataFromGin(c)
	if h == nil || h.executionService == nil || metadata.ClientType == "web" {
		return nil, false, nil
	}
	requestHash, err := executionapp.HashRequest(payload)
	if err != nil {
		return nil, false, err
	}
	user := auth.GetUserFromContext(c)
	actor := executionapp.Actor{UserID: auth.GetUserIDFromContext(c)}
	if user != nil {
		actor.IsAdmin = user.IsAdmin
	}
	return h.executionService.BeginSynchronous(c.Request.Context(), actor, executionapp.SynchronousInput{OperationID: operationID, Module: configExecutionModule, ModuleRef: moduleRef, RequestID: metadata.RequestID, IdempotencyKey: metadata.IdempotencyKey, RequestHash: requestHash, RiskLevel: risk, Impact: impact, Confirmed: metadata.Confirmed, ConfirmationID: metadata.ConfirmationID, ClientType: metadata.ClientType})
}

// finishConfigWrite 在客户端断开后仍完成公共执行状态记录。
// finishConfigWrite completes the shared execution record even after client cancellation.
func (h *Handler) finishConfigWrite(c *gin.Context, item *executionapp.Execution, resultRef string, runErr error) {
	if h == nil || h.executionService == nil || item == nil {
		return
	}
	finishContext := context.WithoutCancel(c.Request.Context())
	if err := h.executionService.FinishSynchronous(finishContext, item, resultRef, runErr); err != nil {
		logger.WarnF(finishContext, "[Config] 更新公共执行记录失败: execution_id=%s err=%v", item.ExecutionID, err)
	}
}

// writeExistingConfigWrite 返回第一次幂等请求产生的安全结果。
// writeExistingConfigWrite returns the safe result produced by the first idempotent request.
func (h *Handler) writeExistingConfigWrite(c *gin.Context, item *executionapp.Execution) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		status := executionapp.Status("")
		if item != nil {
			status = item.Status
		}
		executionapp.WriteError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
		return
	}
	resultRef := strings.TrimSpace(item.ResultRef)
	switch item.OperationID {
	case "config.update", "config.rollback", "config.sync":
		id, err := strconv.ParseUint(resultRef, 10, 32)
		if err != nil {
			executionapp.WriteError(c, executionapp.ErrConcurrentUpdate)
			return
		}
		item, err := h.service.Get(c.Request.Context(), uint(id))
		if err != nil {
			executionapp.WriteError(c, err)
			return
		}
		c.JSON(http.StatusOK, Response{Data: item})
	case "config.promote":
		c.JSON(http.StatusOK, Response{Data: map[string]string{"message": "config promoted to cluster successfully"}})
	case "config.push":
		c.JSON(http.StatusOK, Response{Data: map[string]string{"message": "config pushed to node successfully"}})
	case "config.cluster.init":
		c.JSON(http.StatusOK, Response{Data: map[string]string{"message": "cluster configs initialized successfully"}})
	case "config.cluster.sync-all":
		parts := strings.SplitN(resultRef, ":", 2)
		count := 0
		if len(parts) == 2 {
			count, _ = strconv.Atoi(parts[1])
		}
		c.JSON(http.StatusOK, Response{Data: map[string]interface{}{"message": "template synced to all nodes", "synced_count": count, "push_errors": []*PushError{}}})
	default:
		executionapp.WriteError(c, executionapp.ErrConcurrentUpdate)
	}
}
