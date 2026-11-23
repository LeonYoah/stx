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

package host

import (
	"context"
	"fmt"
	"strconv"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const (
	hostExecutionModule   = "host"
	hostDeleteOperationID = "host.delete"
	hostDeleteImpact      = "删除主机会移除控制面主机登记；不会卸载主机上的 Agent 或节点安装文件。若主机仍关联集群节点，须先停止并移出集群。"
)

// SetExecutionService 注入公共执行服务，供删除主机做 R2 确认。
// SetExecutionService injects the shared execution service for R2 confirmation on host delete.
func (h *Handler) SetExecutionService(svc *executionapp.Service) {
	if h == nil {
		return
	}
	h.executionService = svc
}

func (h *Handler) beginDeleteWrite(c *gin.Context, moduleRef, requestHash string) (*executionapp.Execution, bool, error) {
	if h == nil || h.executionService == nil {
		return nil, false, fmt.Errorf("host execution service is not configured")
	}
	metadata := executionapp.MetadataFromGin(c)
	user := auth.GetUserFromContext(c)
	actor := executionapp.Actor{UserID: auth.GetUserIDFromContext(c)}
	if user != nil {
		actor.IsAdmin = user.IsAdmin
	}
	return h.executionService.BeginSynchronous(c.Request.Context(), actor, executionapp.SynchronousInput{
		OperationID:    hostDeleteOperationID,
		Module:         hostExecutionModule,
		ModuleRef:      moduleRef,
		RequestID:      metadata.RequestID,
		IdempotencyKey: metadata.IdempotencyKey,
		RequestHash:    requestHash,
		RiskLevel:      executionapp.RiskLevelR2,
		Impact:         hostDeleteImpact,
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
		logger.WarnF(finishContext, "[Host] 更新同步删除执行记录失败: execution_id=%s err=%v", item.ExecutionID, err)
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

func hostDeleteRequestHash(hostID uint64) (string, error) {
	return executionapp.HashRequest(struct {
		ID uint64 `json:"id"`
	}{ID: hostID})
}

func hostModuleRef(id uint64) string {
	return strconv.FormatUint(id, 10)
}
