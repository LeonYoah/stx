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

import "errors"

var (
	// ErrDiagnosticsRepositoryUnavailable indicates diagnostics persistence is unavailable.
	// ErrDiagnosticsRepositoryUnavailable 表示诊断持久层不可用。
	ErrDiagnosticsRepositoryUnavailable = errors.New("diagnostics: repository unavailable")
	// ErrInvalidSeatunnelErrorRequest indicates the inbound seatunnel error request is invalid.
	// ErrInvalidSeatunnelErrorRequest 表示入站的 Seatunnel 错误请求非法。
	ErrInvalidSeatunnelErrorRequest = errors.New("diagnostics: invalid seatunnel error request")
	// ErrSeatunnelErrorGroupNotFound indicates the error group does not exist.
	// ErrSeatunnelErrorGroupNotFound 表示错误组不存在。
	ErrSeatunnelErrorGroupNotFound = errors.New("diagnostics: seatunnel error group not found")
	// ErrSeatunnelErrorEventNotFound indicates the error event does not exist.
	// ErrSeatunnelErrorEventNotFound 表示错误事件不存在。
	ErrSeatunnelErrorEventNotFound = errors.New("diagnostics: seatunnel error event not found")
	// ErrSeatunnelLogCursorNotFound indicates the log cursor does not exist.
	// ErrSeatunnelLogCursorNotFound 表示日志游标不存在。
	ErrSeatunnelLogCursorNotFound = errors.New("diagnostics: seatunnel log cursor not found")
	// ErrInvalidInspectionRequest indicates the inspection request is invalid.
	// ErrInvalidInspectionRequest 表示巡检请求非法。
	ErrInvalidInspectionRequest = errors.New("diagnostics: invalid inspection request")
	// ErrInspectionReportNotFound indicates the inspection report does not exist.
	// ErrInspectionReportNotFound 表示巡检报告不存在。
	ErrInspectionReportNotFound = errors.New("diagnostics: inspection report not found")
	// ErrInspectionFindingNotFound indicates the inspection finding does not exist.
	// ErrInspectionFindingNotFound 表示巡检发现项不存在。
	ErrInspectionFindingNotFound = errors.New("diagnostics: inspection finding not found")
	// ErrDiagnosticTaskNotFound indicates the diagnostics task does not exist.
	// ErrDiagnosticTaskNotFound 表示诊断任务不存在。
	ErrDiagnosticTaskNotFound = errors.New("diagnostics: diagnostic task not found")
	// ErrDiagnosticTaskStepNotFound indicates the diagnostics task step does not exist.
	// ErrDiagnosticTaskStepNotFound 表示诊断任务步骤不存在。
	ErrDiagnosticTaskStepNotFound = errors.New("diagnostics: diagnostic task step not found")
	// ErrDiagnosticNodeExecutionNotFound indicates the diagnostics node execution does not exist.
	// ErrDiagnosticNodeExecutionNotFound 表示诊断任务节点执行不存在。
	ErrDiagnosticNodeExecutionNotFound = errors.New("diagnostics: diagnostic node execution not found")
	// ErrInvalidDiagnosticTaskRequest indicates the diagnostics task request is invalid.
	// ErrInvalidDiagnosticTaskRequest 表示诊断任务请求非法。
	ErrInvalidDiagnosticTaskRequest = errors.New("diagnostics: invalid diagnostic task request")
	// ErrDiagnosticTaskAlreadyFinished 表示终态诊断任务不能再次启动或改写。
	// ErrDiagnosticTaskAlreadyFinished indicates that a terminal diagnostic task cannot be started or rewritten again.
	ErrDiagnosticTaskAlreadyFinished = errors.New("diagnostics: diagnostic task already finished")
	// ErrAutoPolicyNotFound indicates the auto-inspection policy does not exist.
	// ErrAutoPolicyNotFound 表示自动巡检策略不存在。
	ErrAutoPolicyNotFound = errors.New("diagnostics: auto-policy not found")
	// ErrInvalidAutoPolicyRequest indicates the auto-policy request is invalid.
	// ErrInvalidAutoPolicyRequest 表示自动策略请求非法。
	ErrInvalidAutoPolicyRequest = errors.New("diagnostics: invalid auto-policy request")
)
