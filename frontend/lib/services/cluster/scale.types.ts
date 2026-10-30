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

/**
 * 集群伸缩（扩容 / 缩容）类型，与后端 internal/apps/cluster/scale_*.go 的 JSON 契约一一对应。
 * Cluster scaling (scale-out / scale-in) types matching the JSON contract of
 * internal/apps/cluster/scale_*.go on the backend.
 */

import type {DeploymentMode, NodeRole} from './types';

/** 伸缩类型 / Scaling kind */
export type ScaleKind = 'scale_out' | 'scale_in';

/** 扩容模式：安装并加入 / 纳管已运行节点 / Scale-out mode: install and join, or register a running node */
export type ScaleMode = 'install' | 'register';

/** 预检项状态 / Precheck item status */
export type ScaleCheckStatus = 'passed' | 'failed' | 'warning' | 'skipped';

/** 步骤名称 / Step names */
export type ScaleStepName =
  | 'validate_plan'
  | 'fix_loopback'
  | 'install_files'
  | 'register_node'
  | 'apply_configs'
  | 'replay_plugins'
  | 'start_node'
  | 'patch_members'
  | 'verify_membership'
  | 'stop_node'
  | 'cleanup_records'
  | 'refresh_monitor'
  | 'follow_java_proxy'
  | 'verify'
  | 'rolling_restart';

/** 步骤状态 / Step status */
export type ScaleStepStatus =
  | 'pending'
  | 'running'
  | 'succeeded'
  | 'partial'
  | 'failed'
  | 'skipped';

/** 伸缩操作状态 / Scaling operation status */
export type ScaleOperationStatus = 'running' | 'succeeded' | 'failed';

/** 节点端口 / Node ports */
export interface ScaleNodePorts {
  hazelcast_port: number;
  api_port: number;
  worker_port: number;
}

/** 扩容计划请求 / Scale-out plan request */
export interface ScaleOutRequest {
  host_id: number;
  /** 为空时由后端自动识别 / Empty lets the backend detect the mode */
  mode?: ScaleMode;
  role?: NodeRole;
  install_dir?: string;
  ports?: Partial<ScaleNodePorts>;
  /** Node JDK; empty means auto detection / 节点 JDK，空表示自动探测 */
  java_home?: string;
}

/** 扩容提交请求 / Scale-out submit request */
export interface ScaleOutSubmitRequest extends ScaleOutRequest {
  plan_fingerprint: string;
  loopback_restart_ack: boolean;
  /** 入群后滚动重启现有节点（可选）/ Rolling-restart existing nodes after the join (optional) */
  rolling_restart?: boolean;
}

/** 缩容提交请求 / Scale-in submit request */
export interface ScaleInSubmitRequest {
  plan_fingerprint: string;
  jobs_risk_ack: boolean;
}

/** 预检项 / Precheck item */
export interface ScaleCheck {
  name: string;
  status: ScaleCheckStatus;
  /** 失败时是否阻断提交 / Whether a failure blocks submission */
  blocking: boolean;
  message: string;
  target?: string;
}

/** 计划目标节点 / Plan target node */
export interface ScaleTarget {
  node_id?: number;
  host_id: number;
  host_name: string;
  host_ip: string;
  role: NodeRole;
  install_dir: string;
  hazelcast_port: number;
  api_port: number;
  worker_port: number;
  address: string;
  /** Prefill / selected JDK for this target / 该目标节点的预填或已选 JDK */
  java_home?: string;
}

/** 回环成员节点 / Loopback member node */
export interface ScaleLoopbackNode {
  node_id: number;
  host_id: number;
  host_name: string;
  address: string;
  /** config：节点配置；runtime：运行时成员 / config: node config; runtime: runtime members */
  source: string;
}

/** 待补齐插件 / Plugin to replay */
export interface ScalePluginItem {
  name: string;
  artifact_id: string;
  version: string;
  status: string;
  profile_keys: string[];
  profile_unknown: boolean;
  /** 元数据缺失时由控制面缓存反推的画像 / Profile inferred from the Control Plane cache when metadata is missing */
  profile_inferred?: boolean;
}

/** 运行中作业 / Running job */
export interface ScaleRunningJob {
  job_id: string;
  job_name: string;
}

/** checkpoint 存储摘要 / Checkpoint storage summary */
export interface ScaleCheckpointStorage {
  type: string;
  location: string;
  local: boolean;
}

/** java-proxy 跟随迁移 / java-proxy follow */
export interface ScaleJavaProxyFollow {
  from_node_id: number;
  from_host_id: number;
  from_host_name: string;
  to_node_id: number;
  to_host_id: number;
  to_host_name: string;
}

/** 滚动重启候选节点 / Rolling-restart candidate node */
export interface ScaleRestartNode {
  node_id: number;
  host_id: number;
  host_name: string;
  address: string;
}

/**
 * 扩容后可选的滚动重启：Hazelcast 只在启动时读取 member-list，分裂后的合并只探测启动时的列表，
 * 现有节点重启后才能在分裂后主动找回新节点。
 * Optional rolling restart after scaling out: Hazelcast reads the member list only at startup and split-brain
 * merging only probes that list, so existing nodes can find the new node again after a split only once restarted.
 */
export interface ScaleRollingRestart {
  nodes: ScaleRestartNode[];
  available: boolean;
  recommended: boolean;
  blocked_reason?: string;
  note?: string;
}

/** 伸缩计划 / Scaling plan */
export interface ScalePlan {
  kind: ScaleKind;
  mode?: ScaleMode;
  detected_mode?: ScaleMode;
  cluster_id: number;
  deployment_mode: DeploymentMode;
  cluster_version: string;
  target: ScaleTarget;
  fingerprint: string;
  members_before: string[];
  members_after: string[];
  checks: ScaleCheck[];
  loopback_nodes: ScaleLoopbackNode[];
  requires_restart_ack: boolean;
  plugins_to_replay: ScalePluginItem[];
  running_jobs: ScaleRunningJob[];
  running_jobs_known: boolean;
  requires_jobs_ack: boolean;
  checkpoint_storage: ScaleCheckpointStorage | null;
  java_proxy_follow: ScaleJavaProxyFollow | null;
  /** 仅扩容 / scale-out only */
  rolling_restart?: ScaleRollingRestart | null;
  steps: ScaleStepName[];
  can_submit: boolean;
  blocking_reasons: string[];
  warnings: string[];
}

/** 步骤状态 / Step state */
export interface ScaleStepState {
  name: ScaleStepName;
  status: ScaleStepStatus;
  attempts: number;
  message?: string;
  error?: string;
  started_at?: string;
  finished_at?: string;
  result?: unknown;
}

/** 伸缩操作详情 / Scaling operation detail */
export interface ScaleOperationInfo {
  id: number;
  execution_id: string;
  cluster_id: number;
  host_id: number;
  node_ids: number[];
  kind: ScaleKind;
  mode?: ScaleMode;
  status: ScaleOperationStatus;
  current_step: ScaleStepName;
  loopback_restart_ack: boolean;
  jobs_risk_ack: boolean;
  rolling_restart?: boolean;
  error?: string;
  warnings: string[];
  retryable: boolean;
  plan: ScalePlan | null;
  steps: ScaleStepState[];
  created_by: number;
  created_by_name: string;
  created_at: string;
  updated_at: string;
  finished_at?: string;
}

/** 提交 / 重试结果 / Submit or retry result */
export interface ScaleSubmitResult {
  execution_id: string;
  operation_id: number;
  operation: ScaleOperationInfo | null;
}

/** 并发冲突详情（409 cluster_scale_in_progress 的 data）/ Conflict details (data of 409 cluster_scale_in_progress) */
export interface ScaleInProgressDetail {
  operation_id: number;
  kind: ScaleKind;
  created_by: number;
  created_by_name: string;
}
