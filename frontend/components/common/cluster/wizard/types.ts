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
 * Cluster Deploy Wizard Types
 * 集群部署向导类型定义
 */

import React from 'react';
import {
  Settings,
  Server,
  Sliders,
  CheckCircle2,
  Package,
  PlayCircle,
  PartyPopper,
} from 'lucide-react';
import {HostInfo} from '@/lib/services/host/types';
import {DeploymentMode, NodeRole} from '@/lib/services/cluster/types';
import type {
  MirrorSource,
  JVMConfig,
  CheckpointConfig,
  IMAPConfig,
  RuntimeEngineConfig,
  PrecheckResult,
} from '@/lib/services/installer/types';
import {buildSeatunnelInstallDir} from '@/lib/seatunnel-version';

/**
 * Wizard step types
 * 向导步骤类型
 */
export type DeployWizardStep =
  | 'basic'
  | 'hosts'
  | 'config'
  | 'precheck'
  | 'plugins'
  | 'deploy'
  | 'complete';

/**
 * Step configuration item
 * 步骤配置项
 */
export interface StepConfig {
  id: DeployWizardStep;
  titleKey: string;
  descKey: string;
  icon: React.ComponentType<{className?: string}>;
}

/**
 * Wizard step definition list
 * 向导步骤配置列表
 */
export const WIZARD_STEPS: StepConfig[] = [
  {
    id: 'basic',
    titleKey: 'cluster.wizard.basic',
    descKey: 'cluster.wizard.basicDesc',
    icon: Settings,
  },
  {
    id: 'hosts',
    titleKey: 'cluster.wizard.hosts',
    descKey: 'cluster.wizard.hostsDesc',
    icon: Server,
  },
  {
    id: 'config',
    titleKey: 'cluster.wizard.config',
    descKey: 'cluster.wizard.configDesc',
    icon: Sliders,
  },
  {
    id: 'precheck',
    titleKey: 'cluster.wizard.precheck',
    descKey: 'cluster.wizard.precheckDesc',
    icon: CheckCircle2,
  },
  {
    id: 'plugins',
    titleKey: 'cluster.wizard.plugins',
    descKey: 'cluster.wizard.pluginsDesc',
    icon: Package,
  },
  {
    id: 'deploy',
    titleKey: 'cluster.wizard.deploy',
    descKey: 'cluster.wizard.deployDesc',
    icon: PlayCircle,
  },
  {
    id: 'complete',
    titleKey: 'cluster.wizard.complete',
    descKey: 'cluster.wizard.completeDesc',
    icon: PartyPopper,
  },
];

/**
 * Host with role assignment
 * 带角色分配的主机项
 */
export interface HostWithRole {
  host: HostInfo;
  selected: boolean;
  /** Single role for hybrid; one or both of MASTER/WORKER for separated / 混合模式为单角色；分离模式为 MASTER/WORKER 之一或两者 */
  roles: NodeRole[];
  /** Per-host JDK path; empty means auto detection (AC1) / 每主机 JDK 路径，空表示自动探测（AC1） */
  javaHome?: string;
  /** True when javaHome was auto-prefilled rather than explicitly chosen / 标识当前 javaHome 是否由探测器自动预填而非手动选择 */
  javaHomeAutoPrefilled?: boolean;
}

/**
 * Host precheck result
 * 主机预检查结果
 */
export interface HostPrecheckResult {
  hostId: number;
  hostName: string;
  loading: boolean;
  result: PrecheckResult | null;
  error: string | null;
}

/**
 * Detailed deploy step item
 * 部署过程单个步骤状态项
 */
export interface DeployStepItem {
  step: string;
  status: 'pending' | 'running' | 'success' | 'failed';
  message: string;
  hostName?: string;
  progress?: number;
}

/**
 * Cluster deploy configuration
 * 集群部署全量配置
 */
export interface ClusterDeployConfig {
  // Basic info / 基本信息
  name: string;
  /** Physical cluster code identifier / 物理集群唯一标识 */
  clusterCode: string;
  description: string;
  deploymentMode: DeploymentMode;
  // Install config / 安装配置
  version: string;
  installDir: string;
  mirror: MirrorSource;
  // Port config / 端口配置
  clusterPort: number;
  httpPort: number;
  workerPort: number;
  /** Managed stx-java-proxy listen port / 托管 stx-java-proxy 监听端口 */
  javaProxyPort: number;
  runtime: RuntimeEngineConfig;
  jvm: JVMConfig;
  checkpoint: CheckpointConfig;
  imap: IMAPConfig;
  // Plugins / 插件
  selectedPlugins: string[];
  selectedPluginProfiles: Record<string, string[]>;
}

/**
 * Default cluster deploy configuration
 * 默认集群部署配置
 */
export const defaultClusterDeployConfig: ClusterDeployConfig = {
  name: '',
  clusterCode: '',
  description: '',
  deploymentMode: DeploymentMode.SEPARATED,
  version: '',
  installDir: buildSeatunnelInstallDir(),
  mirror: 'aliyun',
  clusterPort: 5801,
  httpPort: 8080,
  workerPort: 5802,
  javaProxyPort: 18080,
  runtime: {
    dynamic_slot: true,
    slot_num: 2,
    slot_allocation_strategy: 'RANDOM',
    job_schedule_strategy: 'REJECT',
    history_job_expire_minutes: 1440,
    scheduled_deletion_enable: true,
    enable_http: true,
    enable_event_channel: true,
    job_log_mode: 'per_job',
  },
  jvm: {
    hybrid_heap_size: 2048,
    master_heap_size: 2048,
    worker_heap_size: 2048,
  },
  checkpoint: {
    storage_type: 'LOCAL_FILE',
    namespace: '/tmp/seatunnel/checkpoint/',
  },
  imap: {
    storage_type: 'DISABLED',
    namespace: '/tmp/seatunnel/imap/',
  },
  selectedPlugins: [],
  selectedPluginProfiles: {},
};

/**
 * K8s 部署向导表单状态（模块6统一向导扩展版）。
 * Kubernetes deploy wizard form state (module-6 unified wizard edition).
 */
export interface K8sWizardFormState {
  host_id: number;
  namespace: string;
  name: string;
  cluster_code: string;
  description: string;
  image: string;
  version: string;
  image_pull_policy: string;
  image_pull_secrets: string;
  service_account: string;
  deployment_mode: 'separated' | 'hybrid';
  master_replicas: number;
  worker_replicas: number;
  hybrid_replicas: number;
  master_http_port: number;
  hazelcast_port: number;
  master_cpu: string;
  master_memory: string;
  worker_cpu: string;
  worker_memory: string;
  hybrid_cpu: string;
  hybrid_memory: string;
  checkpoint_pvc: string;
  data_storage_class: string;
  data_size: string;
  jvm_xms: string;
  jvm_xmx: string;
  master_heap_mb: string;
  worker_heap_mb: string;
  hybrid_heap_mb: string;
  java_proxy_mode: string;
  java_proxy_port: string;
  java_proxy_heap: string;
  runtime_dynamic_slot: boolean;
  runtime_slot_num: string;
  runtime_slot_allocation_strategy: string;
  runtime_job_schedule_strategy: string;
  runtime_history_job_expire_minutes: string;
  runtime_scheduled_deletion_enable: boolean;
  runtime_job_log_mode: string;
  event_report_enabled: boolean;
  stx_callback_url: string;
  checkpoint: {
    enabled: boolean;
    storage_type: string;
    namespace: string;
    endpoint: string;
    bucket: string;
    access_key: string;
    secret_key: string;
  };
  imap: {
    enabled: boolean;
    storage_type: string;
    namespace: string;
    endpoint: string;
    bucket: string;
    access_key: string;
    secret_key: string;
  };
}

export const INITIAL_K8S_WIZARD_FORM: K8sWizardFormState = {
  host_id: 0,
  namespace: 'default',
  name: '',
  cluster_code: '',
  description: '',
  image: '',
  version: '',
  image_pull_policy: 'IfNotPresent',
  image_pull_secrets: '',
  service_account: '',
  deployment_mode: 'separated',
  master_replicas: 1,
  worker_replicas: 2,
  hybrid_replicas: 2,
  master_http_port: 8080,
  hazelcast_port: 5801,
  master_cpu: '0.5',
  master_memory: '2Gi',
  worker_cpu: '1',
  worker_memory: '2Gi',
  hybrid_cpu: '1',
  hybrid_memory: '4Gi',
  checkpoint_pvc: '',
  data_storage_class: '',
  data_size: '',
  jvm_xms: '512m',
  jvm_xmx: '1536m',
  master_heap_mb: '',
  worker_heap_mb: '',
  hybrid_heap_mb: '',
  java_proxy_mode: 'managed',
  java_proxy_port: '18080',
  java_proxy_heap: '512m',
  runtime_dynamic_slot: true,
  runtime_slot_num: '',
  runtime_slot_allocation_strategy: '',
  runtime_job_schedule_strategy: '',
  runtime_history_job_expire_minutes: '',
  runtime_scheduled_deletion_enable: true,
  runtime_job_log_mode: 'mixed',
  event_report_enabled: false,
  stx_callback_url: '',
  checkpoint: {
    enabled: false,
    storage_type: 'LOCAL_FILE',
    namespace: '',
    endpoint: '',
    bucket: '',
    access_key: '',
    secret_key: '',
  },
  imap: {
    enabled: false,
    storage_type: 'DISABLED',
    namespace: '',
    endpoint: '',
    bucket: '',
    access_key: '',
    secret_key: '',
  },
};

/** K8s 向导步骤：跳过 plugins / Kubernetes wizard steps: plugins skipped */
export const K8S_WIZARD_STEPS: StepConfig[] = WIZARD_STEPS.filter(
  (step) => step.id !== 'plugins',
);
