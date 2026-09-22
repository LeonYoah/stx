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

package diagnostics

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const (
	diagnosticsInspectionExecutionModule = "diagnostics_inspection"
	diagnosticsAutoPolicyExecutionModule = "diagnostics_auto_policy"
	diagnosticsTaskStartExecutionModule  = "diagnostics_task_start"
	diagnosticsInspectionRunImpact       = "立即巡检会读取集群状态、进程事件、告警和近期错误，并保存巡检报告。"
	diagnosticsAutoPolicyCreateImpact    = "创建自动巡检策略后，启用的条件可能自动发起巡检和诊断任务。"
	diagnosticsAutoPolicyUpdateImpact    = "修改自动巡检策略会改变后续自动巡检和诊断任务的触发方式。"
	diagnosticsTaskStartImpact           = "启动诊断任务会执行任务中已选择的资源采集步骤，部分步骤可能增加节点负载。"
)

// beginDiagnosticsSynchronousWrite 为 CLI 和直接 API 写请求创建同步执行记录，网页请求保持现有行为。
// beginDiagnosticsSynchronousWrite creates a synchronous execution for CLI and direct API writes while preserving web behavior.
func (h *Handler) beginDiagnosticsSynchronousWrite(c *gin.Context, operationID, module, moduleRef string, risk executionapp.RiskLevel, impact string, payload any) (*executionapp.Execution, bool, error) {
	metadata := executionapp.MetadataFromGin(c)
	if h == nil || h.service == nil || h.service.executionService == nil || metadata.ClientType == "web" {
		return nil, false, nil
	}
	requestHash, err := executionapp.HashRequest(payload)
	if err != nil {
		return nil, false, err
	}
	return h.service.executionService.BeginSynchronous(c.Request.Context(), currentDiagnosticActor(c), executionapp.SynchronousInput{
		OperationID: operationID, Module: module, ModuleRef: moduleRef,
		RequestID: metadata.RequestID, IdempotencyKey: metadata.IdempotencyKey, RequestHash: requestHash,
		RiskLevel: risk, Impact: impact, Confirmed: metadata.Confirmed,
		ConfirmationID: metadata.ConfirmationID, ClientType: metadata.ClientType,
	})
}

// finishDiagnosticsSynchronousWrite 在客户端断开后仍保存写操作的最终状态。
// finishDiagnosticsSynchronousWrite stores the final write status even after client cancellation.
func (h *Handler) finishDiagnosticsSynchronousWrite(c *gin.Context, item *executionapp.Execution, resultRef string, runErr error) {
	if h == nil || h.service == nil || h.service.executionService == nil || item == nil {
		return
	}
	finishContext := context.WithoutCancel(c.Request.Context())
	if runErr == nil && strings.TrimSpace(item.ModuleRef) == "" && strings.TrimSpace(resultRef) != "" {
		if err := h.service.executionService.BindModuleRef(finishContext, item.ExecutionID, resultRef); err != nil {
			logger.WarnF(finishContext, "[Diagnostics] 绑定同步执行资源编号失败: execution_id=%s err=%v", item.ExecutionID, err)
		} else {
			item.ModuleRef = strings.TrimSpace(resultRef)
		}
	}
	if err := h.service.executionService.FinishSynchronous(finishContext, item, resultRef, runErr); err != nil {
		logger.WarnF(finishContext, "[Diagnostics] 更新同步执行记录失败: execution_id=%s err=%v", item.ExecutionID, err)
	}
}

// writeExistingDiagnosticsSynchronousWrite 返回第一次幂等请求产生的安全结果。
// writeExistingDiagnosticsSynchronousWrite returns the safe result produced by the first idempotent request.
func (h *Handler) writeExistingDiagnosticsSynchronousWrite(c *gin.Context, item *executionapp.Execution) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		status := executionapp.Status("")
		if item != nil {
			status = item.Status
		}
		h.writeDiagnosticsError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(item.ResultRef), 10, 32)
	if err != nil || id == 0 {
		h.writeDiagnosticsError(c, executionapp.ErrConcurrentUpdate)
		return
	}

	switch item.OperationID {
	case "diagnostics.inspection.run":
		data, loadErr := h.service.GetInspectionReportDetail(c.Request.Context(), uint(id))
		if loadErr != nil {
			h.writeDiagnosticsError(c, loadErr)
			return
		}
		c.JSON(http.StatusCreated, Response{Data: localizeInspectionReportDetailData(data, diagnosticsLanguageFromRequest(c))})
	case "diagnostics.auto-policy.create", "diagnostics.auto-policy.update":
		data, loadErr := h.service.GetAutoPolicy(c.Request.Context(), uint(id))
		if loadErr != nil {
			h.writeDiagnosticsError(c, loadErr)
			return
		}
		status := http.StatusOK
		if item.OperationID == "diagnostics.auto-policy.create" {
			status = http.StatusCreated
		}
		c.JSON(status, Response{Data: data})
	case "diagnostics.task.start":
		data, loadErr := h.service.GetDiagnosticTaskForActor(c.Request.Context(), currentDiagnosticActor(c), uint(id))
		if loadErr != nil {
			h.writeDiagnosticsError(c, loadErr)
			return
		}
		c.JSON(http.StatusOK, Response{Data: localizeDiagnosticTask(data, diagnosticsLanguageFromRequest(c))})
	default:
		h.writeDiagnosticsError(c, executionapp.ErrConcurrentUpdate)
	}
}
