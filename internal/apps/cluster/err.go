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

import "errors"

// Error definitions for cluster management operations.
var (
	// ErrClusterNotFound indicates the requested cluster does not exist.
	ErrClusterNotFound = errors.New("cluster: cluster not found")
	// ErrClusterNameDuplicate indicates a cluster with the same name already exists.
	ErrClusterNameDuplicate = errors.New("cluster: cluster name already exists")
	// ErrClusterNameEmpty indicates the cluster name is empty.
	ErrClusterNameEmpty = errors.New("cluster: cluster name cannot be empty")
	// ErrClusterHasRunningTask indicates the cluster has running tasks and cannot be deleted.
	ErrClusterHasRunningTask = errors.New("cluster: cluster has running tasks and cannot be deleted")
	// ErrNodeNotFound indicates the requested cluster node does not exist.
	ErrNodeNotFound = errors.New("cluster: node not found")
	// ErrNodeAlreadyExists indicates the host already has a node with the same role in the cluster.
	// ErrNodeAlreadyExists 表示主机在集群中已有相同角色的节点。
	ErrNodeAlreadyExists = errors.New("cluster: host already has a node with this role in the cluster / 主机在此集群中已有相同角色的节点")
	// ErrNodeAgentNotInstalled indicates the host's agent is not installed.
	ErrNodeAgentNotInstalled = errors.New("cluster: host agent is not installed")
	// ErrInvalidDeploymentMode indicates an invalid deployment mode was specified.
	ErrInvalidDeploymentMode = errors.New("cluster: invalid deployment mode")
	// ErrInvalidNodeRole indicates an invalid node role was specified.
	ErrInvalidNodeRole = errors.New("cluster: invalid node role")
	// ErrInvalidHazelcastPort indicates an invalid or missing hazelcast port.
	// ErrInvalidHazelcastPort 表示无效或缺失的 Hazelcast 端口。
	ErrInvalidHazelcastPort = errors.New("cluster: hazelcast port is required and must be valid")
	// ErrInvalidAPIPort indicates an invalid API port.
	// ErrInvalidAPIPort 表示无效的 API 端口。
	ErrInvalidAPIPort = errors.New("cluster: api port must be valid when provided")
	// ErrInvalidWorkerPort indicates an invalid worker port.
	// ErrInvalidWorkerPort 表示无效的 worker 端口。
	ErrInvalidWorkerPort = errors.New("cluster: worker port must be valid when provided")
	// ErrNodeBatchEntriesRequired indicates batch add request has no entries.
	// ErrNodeBatchEntriesRequired 表示批量加节点请求未提供任何条目。
	ErrNodeBatchEntriesRequired = errors.New("cluster: at least one node entry is required")
	// ErrInvalidNodeJVMOverride indicates node-level JVM override is invalid.
	// ErrInvalidNodeJVMOverride 表示节点级 JVM override 非法。
	ErrInvalidNodeJVMOverride = errors.New("cluster: jvm override must be greater than 0 when provided")
	// ErrPrecheckFailed indicates the node precheck failed.
	// ErrPrecheckFailed 表示节点预检查失败。
	ErrPrecheckFailed = errors.New("cluster: node precheck failed")
	// ErrSeparatedClusterRequiresMasterAndWorker indicates separated mode cluster must have at least one master and one worker node.
	// ErrSeparatedClusterRequiresMasterAndWorker 表示分离模式集群必须至少包含一个 master 节点和一个 worker 节点。
	ErrSeparatedClusterRequiresMasterAndWorker = errors.New("cluster: separated deployment mode requires at least one master and one worker node / 分离模式集群必须至少包含一个 master 节点和一个 worker 节点")

	// ErrClusterScaleInProgress 集群已有进行中的伸缩操作（409 cluster_scale_in_progress）。
	// ErrClusterScaleInProgress means the cluster already has a running scaling operation (409 cluster_scale_in_progress).
	ErrClusterScaleInProgress = errors.New("cluster: another scaling operation is running on this cluster")
	// ErrScaleOperationNotFound 伸缩操作不存在或无权查看。
	// ErrScaleOperationNotFound means the scaling operation does not exist or is not visible to the caller.
	ErrScaleOperationNotFound = errors.New("cluster: scaling operation not found")
	// ErrScaleOperationCorrupted 伸缩操作的计划快照无法解析。
	// ErrScaleOperationCorrupted means the scaling operation's plan snapshot cannot be decoded.
	ErrScaleOperationCorrupted = errors.New("cluster: scaling operation plan snapshot is invalid")
	// ErrScaleOperationNotRetryable 只有失败的伸缩操作可以重试。
	// ErrScaleOperationNotRetryable means only failed scaling operations can be retried.
	ErrScaleOperationNotRetryable = errors.New("cluster: only failed scaling operations can be retried")
	// ErrScaleInvalidRequest 伸缩请求参数非法（如模式、角色）。
	// ErrScaleInvalidRequest means the scaling request is invalid (e.g. mode or role).
	ErrScaleInvalidRequest = errors.New("cluster: invalid scaling request")
	// ErrScalePlanBlocked 计划存在阻断项，不能提交。
	// ErrScalePlanBlocked means the plan has blocking items and cannot be submitted.
	ErrScalePlanBlocked = errors.New("cluster: scaling plan has blocking items")
	// ErrScalePlanChanged 提交的计划指纹与当前重算结果不一致，需要重新预检。
	// ErrScalePlanChanged means the submitted plan fingerprint differs from the recomputed plan; precheck again.
	ErrScalePlanChanged = errors.New("cluster: scaling plan changed since precheck, please precheck again")
	// ErrScaleRestartAckRequired 存在回环成员但未确认重启现有节点。
	// ErrScaleRestartAckRequired means loopback members exist but the restart of existing nodes was not acknowledged.
	ErrScaleRestartAckRequired = errors.New("cluster: restarting existing nodes with loopback member addresses must be acknowledged")
	// ErrScaleJobsRiskAckRequired 存在运行中作业（或无法确认）但未确认风险。
	// ErrScaleJobsRiskAckRequired means running jobs exist (or are unknown) but the risk was not acknowledged.
	ErrScaleJobsRiskAckRequired = errors.New("cluster: running jobs risk must be acknowledged before removing the node")
	// ErrScaleNotConfigured 伸缩编排缺少必要依赖。
	// ErrScaleNotConfigured means the scaling orchestrator lacks a required dependency.
	ErrScaleNotConfigured = errors.New("cluster: scaling orchestrator is not fully configured")
)

// 伸缩操作对外错误编码，前端据此本地化。
// Public error codes of scaling operations, localized by the frontend.
const (
	ErrorCodeClusterScaleInProgress  = "cluster_scale_in_progress"
	ErrorCodeScaleOperationNotFound  = "scale_operation_not_found"
	ErrorCodeScaleNotRetryable       = "scale_operation_not_retryable"
	ErrorCodeScaleInvalidRequest     = "scale_invalid_request"
	ErrorCodeScalePlanBlocked        = "scale_plan_blocked"
	ErrorCodeScalePlanChanged        = "scale_plan_changed"
	ErrorCodeScaleRestartAckRequired = "scale_restart_ack_required"
	ErrorCodeScaleJobsAckRequired    = "scale_jobs_risk_ack_required"
)

// Error codes for cluster management operations.
const (
	ErrCodeClusterNotFound       = 3001
	ErrCodeClusterNameDuplicate  = 3002
	ErrCodeClusterHasRunningTask = 3003
	ErrCodeNodeAgentNotInstalled = 3004
	ErrCodeNodeNotFound          = 3005
	ErrCodeNodeAlreadyExists     = 3006
)
