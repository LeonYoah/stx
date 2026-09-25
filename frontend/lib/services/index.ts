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

import {AuthService} from './auth/index';
import {ProjectService} from './project/index';
import {DashboardService} from './dashboard/index';
import {AdminUserService} from './admin/index';
import {HostService} from './host/index';
import {ClusterService} from './cluster/index';
import {AuditService} from './audit/index';
import {installerService} from './installer/index';
import {pluginService} from './plugin/index';
import {ConfigService} from './config/index';
import * as MonitorService from './monitor/index';
import {MonitoringService} from './monitoring/index';
import * as DiscoveryService from './discovery/index';
import {StUpgradeService} from './st-upgrade/index';
import {DiagnosticsService} from './diagnostics/index';
import {SyncService} from './sync/index';
import {troubleshootingService} from './troubleshooting/index';
import {CapabilityService} from './capability/index';

/**
 * 服务层架构说明：
 *
 * 服务层是应用程序与API交互的统一入口，遵循以下原则：
 * 1. 关注点分离 - 每个服务负责一个具体的业务领域
 * 2. 统一入口 - 通过services对象统一导出所有服务
 * 3. 类型安全 - 所有服务的请求和响应都有明确的类型定义
 *
 * 服务层目录结构：
 *
 * /services
 *   /core - 核心基础工具
 *     - api-client.ts - Axios实例配置
 *     - base.service.ts - 服务基类
 *     - types.ts - 通用类型定义
 *   /auth - 认证相关服务
 *     - auth.service.ts - 认证服务实现
 *     - types.ts - 认证相关类型
 *     - index.ts - 导出服务
 *   /project - 项目相关服务
 *     - project.service.ts - 项目服务实现
 *     - types.ts - 项目相关类型
 *     - index.ts - 导出服务
 *   /dashboard - 仪表盘服务
 *     - dashboard.service.ts - 仪表盘服务实现
 *     - types.ts - 仪表盘相关类型
 *     - index.ts - 导出服务
 *
 * 使用示例：
 *
 * // 调用认证服务登录方法
 * import services from '@/lib/services';
 * await services.auth.login('/redirect-path');
 *
 * // 获取用户信息
 * const userInfo = await services.auth.getUserInfo();
 *
 * // 创建项目
 * const result = await services.project.createProjectSafe({
 *   name: '测试项目',
 *   start_time: '2024-01-01T00:00:00Z',
 *   end_time: '2024-12-31T23:59:59Z',
 *   minimum_trust_level: TrustLevel.NEW_USER,
 *   distribution_type: DistributionType.ONE_FOR_EACH,
 *   project_items: ['item1', 'item2', 'item3']
 * });
 *
 * 扩展指南：
 *
 * 1. 创建新服务目录结构
 *    - 在services目录下创建新的目录，如 `/services/project`
 *    - 创建types.ts定义类型
 *    - 创建service.ts实现服务
 *    - 创建index.ts导出服务
 *
 * 2. 实现服务类
 *    - 继承BaseService基类
 *    - 设置basePath指向对应API路径
 *    - 实现服务方法，处理业务逻辑
 *
 * 3. 在此文件中注册服务
 *    - 导入新服务
 *    - 添加到services对象中
 */

/**
 * 服务层统一入口
 */
const services = {
  /**
   * 认证服务
   */
  auth: AuthService,

  /**
   * 产品版本服务
   * Product version service
   */
  capability: CapabilityService,

  /**
   * 项目服务
   */
  project: ProjectService,

  /**
   * 仪表盘服务
   */
  dashboard: DashboardService,

  /**
   * 管理员用户管理服务
   */
  adminUser: AdminUserService,

  /**
   * 主机管理服务
   */
  host: HostService,

  /**
   * 集群管理服务
   */
  cluster: ClusterService,

  /**
   * 审计日志服务
   */
  audit: AuditService,

  /**
   * 安装管理服务
   * Installer management service
   */
  installer: installerService,

  /**
   * 插件市场服务
   * Plugin marketplace service
   */
  plugin: pluginService,

  /**
   * 配置管理服务
   * Config management service
   */
  config: ConfigService,

  /**
   * 监控配置服务
   * Monitor configuration service
   * Requirements: 5.1, 5.4, 6.4
   */
  monitor: MonitorService,

  /**
   * 监控中心服务
   * Monitoring center service
   */
  monitoring: MonitoringService,

  /**
   * 诊断中心服务
   * Diagnostics center service
   */
  diagnostics: DiagnosticsService,

  /**
   * 数据同步 Studio 服务
   * Data sync studio service
   */
  sync: SyncService,

  /**
   * 集群发现服务
   * Cluster discovery service
   * Requirements: 1.2, 1.9, 9.3, 9.4
   */
  discovery: DiscoveryService,

  /**
   * SeaTunnel 升级服务
   * SeaTunnel upgrade service
   */
  stUpgrade: StUpgradeService,

  /**
   * 排障经验记忆库服务
   * Troubleshooting memory bank service
   */
  troubleshooting: troubleshootingService,
};

export default services;
