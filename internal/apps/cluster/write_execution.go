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
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const (
	clusterExecutionModule = "cluster"

	clusterDeleteOperationID     = "cluster.delete"
	clusterNodeRemoveOperationID = "cluster.node.remove"

	clusterDeleteImpact = "删除集群会移除集群定义和节点记录；集群必须先停止，force_delete 还会请求 Agent 删除节点安装目录。"
	clusterNodeRemoveImpact = "移除节点会删除节点定义；如果节点仍在运行，应先停止节点。"
)

// SetExecutionService 注入公共执行服务，供删除类写接口做 R2 确认。
// SetExecutionService injects the shared execution service for R2 confirmation on delete writes.
func (h *Handler) SetExecutionService(svc *executionapp.Service) {
	if h == nil {
		return
	}
	h.executionService = svc
}

func (h *Handler) beginDeleteWrite(c *gin.Context, operationID, moduleRef, impact string, requestHash string) (*executionapp.Execution, bool, error) {
	if h == nil || h.executionService == nil {
		return nil, false, fmt.Errorf("cluster execution service is not configured")
	}
	metadata := executionapp.MetadataFromGin(c)
	user := auth.GetUserFromContext(c)
	actor := executionapp.Actor{UserID: auth.GetUserIDFromContext(c)}
	if user != nil {
		actor.IsAdmin = user.IsAdmin
	}
	// Web 与 CLI/API 一律走 R2：缺确认编号只返回 confirmation_id，不执行删除。
	// Web and CLI/API both use R2: without a confirmation ID only confirmation_id is returned; delete does not run.
	return h.executionService.BeginSynchronous(c.Request.Context(), actor, executionapp.SynchronousInput{
		OperationID:    operationID,
		Module:         clusterExecutionModule,
		ModuleRef:      moduleRef,
		RequestID:      metadata.RequestID,
		IdempotencyKey: metadata.IdempotencyKey,
		RequestHash:    requestHash,
		RiskLevel:      executionapp.RiskLevelR2,
		Impact:         impact,
		Confirmed:      metadata.Confirmed,
		ConfirmationID: metadata.ConfirmationID,
		ClientType:     metadata.ClientType,
	})
}

func (h *Handler) finishDeleteWrite(c *gin.Context, item *executionapp.Execution, resultRef string, runErr error) {
	if h == nil || h.executionService == nil || item == nil {
		return
	}
	finishContext := context.WithoutCancel(c.Request.Context())
	if err := h.executionService.FinishSynchronous(finishContext, item, resultRef, runErr); err != nil {
		logger.WarnF(finishContext, "[Cluster] 更新同步删除执行记录失败: execution_id=%s err=%v", item.ExecutionID, err)
	}
}

func (h *Handler) writeExistingDelete(c *gin.Context, item *executionapp.Execution, respondOK func()) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		status := executionapp.Status("")
		if item != nil {
			status = item.Status
		}
		executionapp.WriteError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
		return
	}
	respondOK()
}

func clusterDeleteRequestHash(clusterID uint64, forceDelete bool) (string, error) {
	return executionapp.HashRequest(struct {
		ID          uint64 `json:"id"`
		ForceDelete bool   `json:"force_delete"`
	}{ID: clusterID, ForceDelete: forceDelete})
}

func clusterNodeRemoveRequestHash(clusterID, nodeID uint64) (string, error) {
	return executionapp.HashRequest(struct {
		ID     uint64 `json:"id"`
		NodeID uint64 `json:"node_id"`
	}{ID: clusterID, NodeID: nodeID})
}

func moduleRefUint(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func moduleRefClusterNode(clusterID, nodeID uint64) string {
	return strings.TrimSpace(moduleRefUint(clusterID) + "/" + moduleRefUint(nodeID))
}
