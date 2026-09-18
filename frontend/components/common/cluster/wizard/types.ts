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
    job_log_mode: 'mixed',
  },
  jvm: {
    hybrid_heap_size: 2,
    master_heap_size: 2,
    worker_heap_size: 2,
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
