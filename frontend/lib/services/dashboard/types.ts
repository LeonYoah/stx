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

import {DistributionType} from '../project/types';
import {ApiResponse} from '../core/types';
import {ReactNode} from 'react';
import {
  NameType,
  ValueType,
} from 'recharts/types/component/DefaultTooltipContent';

/**
 * 时间序列数据
 */
export interface TimeSeriesData {
  date: string;
  value: number;
}

/**
 * 用户增长数据
 */
export interface UserGrowthData extends TimeSeriesData {
  date: string;
  value: number;
}

/**
 * 活动数据
 */
export interface ActivityData extends TimeSeriesData {
  date: string;
  value: number;
}

/**
 * 统计图表数据
 */
export interface ChartDataItem {
  name: string | DistributionType;
  value: number;
}

/**
 * 项目分类数据
 */
export interface ProjectTagsData extends ChartDataItem {
  name: string;
  value: number;
}

/**
 * 分发模式数据
 */
export interface DistributeModeData extends ChartDataItem {
  name: DistributionType;
  value: number;
}

/**
 * 热门项目数据
 */
export interface HotProjectData {
  name: string;
  tags: string[]; // 后端返回的是标签数组
  receiveCount: number;
}

/**
 * 用户活跃度数据
 */
export interface UserActivityData {
  name: string;
  avatar?: string;
}

/**
 * 活跃创建者原始数据
 */
export interface RawActiveCreatorData {
  avatar: string | null;
  nickname: string;
  username: string;
  projectCount: number;
}

/**
 * 活跃创建者数据
 */
export interface ActiveCreatorData {
  avatar: string | null;
  name: string;
  projectCount: number;
}

/**
 * 活跃领取者原始数据
 */
export interface RawActiveReceiverData {
  avatar: string | null;
  nickname: string;
  username: string;
  receiveCount: number;
}

/**
 * 活跃领取者数据
 */
export interface ActiveReceiverData {
  avatar: string | null;
  name: string;
  receiveCount: number;
}

/**
 * 热门标签数据
 */
export interface HotTagData {
  name: string;
  count: number;
}

/**
 * 统计数据
 */
export interface StatsSummary {
  totalUsers: number;
  newUsers: number;
  totalProjects: number;
  totalReceived: number;
  recentReceived: number;
}

/**
 * 后端原始数据结构
 */
export interface RawDashboardData {
  userGrowth: string | UserGrowthData[];
  activityData: string | ActivityData[];
  projectTags: string | ProjectTagsData[];
  distributeModes: string | DistributeModeData[];
  hotProjects: string | HotProjectData[];
  activeCreators: string | RawActiveCreatorData[];
  activeReceivers: string | RawActiveReceiverData[];
  summary: string | StatsSummary;
  [key: string]: unknown;
}

/**
 * 仪表盘数据响应
 */
export interface DashboardResponse {
  userGrowth: UserGrowthData[];
  activityData: ActivityData[];
  projectTags: ProjectTagsData[];
  distributeModes: DistributeModeData[];
  hotProjects: HotProjectData[];
  activeCreators: ActiveCreatorData[];
  activeReceivers: ActiveReceiverData[];
  summary: StatsSummary;
}

/**
 * 后端API响应类型
 */
export type DashboardApiResponse = ApiResponse<RawDashboardData>;

// ==================== 组件相关类型定义 ====================

/**
 * 统计卡片组件属性
 */
export interface StatCardProps {
  title: string;
  value?: number | string;
  icon: ReactNode;
  desc?: string;
  descColor?: string;
}

/**
 * 列表项数据类型（支持所有列表类型）
 */
export type ListItemData =
  | HotProjectData
  | ActiveCreatorData
  | ActiveReceiverData;

/**
 * 卡片列表组件属性
 */
export interface CardListProps {
  title: string;
  iconBg: string;
  icon: ReactNode;
  list: ListItemData[];
  type: 'project' | 'creator' | 'receiver';
}

/**
 * 标签展示组件属性
 */
export interface TagsDisplayProps {
  title: string;
  iconBg: string;
  tags?: {name: string; count: number}[];
  icon?: ReactNode;
}

// ==================== 图表组件相关类型定义 ====================

/**
 * 图表容器组件属性
 */
export interface ChartContainerProps {
  title: string;
  icon?: ReactNode;
  iconBg?: string;
  isLoading: boolean;
  children: ReactNode;
}

/**
 * 用户增长图表组件属性
 */
export interface UserGrowthChartProps {
  data?: UserGrowthData[];
  isLoading: boolean;
  icon?: ReactNode;
  range?: number;
}

/**
 * 活动趋势图表组件属性
 */
export interface ActivityChartProps {
  data?: ActivityData[];
  isLoading: boolean;
  icon?: ReactNode;
  range?: number;
}

/**
 * 项目分类图表组件属性
 */
export interface CategoryChartProps {
  data?: ProjectTagsData[];
  isLoading: boolean;
  icon?: ReactNode;
}

/**
 * 分发模式图表组件属性
 */
export interface DistributeModeChartProps {
  data?: DistributeModeData[];
  isLoading: boolean;
  icon?: ReactNode;
}

/**
 * 自定义工具提示属性
 */
export interface TooltipProps {
  active?: boolean;
  payload?: Array<{
    value: ValueType;
    name?: NameType;
    dataKey?: string;
    color?: string;
    payload?: Record<string, unknown>;
  }>;
  label?: string;
  labelFormatter?: (
    label: string,
    payload?: Record<string, unknown>[],
  ) => ReactNode;
}


// ==================== Overview 概览相关类型定义 ====================

/**
 * Overview statistics / 概览统计数据
 */
export interface OverviewStats {
  total_hosts: number;
  online_hosts: number;
  total_clusters: number;
  running_clusters: number;
  stopped_clusters: number;
  error_clusters: number;
  total_nodes: number;
  running_nodes: number;
  stopped_nodes: number;
  error_nodes: number;
  total_agents: number;
  online_agents: number;
}

/**
 * Cluster summary for dashboard / 仪表盘集群摘要
 */
export interface ClusterSummary {
  id: number;
  name: string;
  /** DB status; may be "unhealthy" when running but 0 nodes online */
  status: string;
  deployment_mode: string;
  total_nodes: number;
  master_nodes: number;
  worker_nodes: number;
  /** Nodes with status running AND host online */
  running_nodes: number;
  /** Number of nodes whose host is online */
  online_nodes?: number;
}

/**
 * Host summary for dashboard / 仪表盘主机摘要
 */
export interface HostSummary {
  id: number;
  name: string;
  ip_address: string;
  is_online: boolean;
  agent_status: string;
  node_count: number;
}

/**
 * Recent activity / 最近活动
 * Prefer operator + source + action; message kept for compatibility.
 * 优先展示操作人 + 来源 + 操作名；message 仅作兼容。
 */
export interface RecentActivity {
  id: number;
  type: 'success' | 'warning' | 'info' | 'error';
  operator?: string;
  source?: string;
  action?: string;
  message?: string;
  timestamp: string;
}

/**
 * Complete overview data / 完整概览数据
 */
export interface OverviewData {
  stats: OverviewStats;
  cluster_summaries: ClusterSummary[];
  host_summaries: HostSummary[];
  recent_activities: RecentActivity[];
}

/**
 * Overview API response / 概览 API 响应
 */
export type OverviewApiResponse = ApiResponse<OverviewData>;
export type OverviewStatsApiResponse = ApiResponse<OverviewStats>;
export type ClusterSummariesApiResponse = ApiResponse<ClusterSummary[]>;
export type HostSummariesApiResponse = ApiResponse<HostSummary[]>;
export type RecentActivitiesApiResponse = ApiResponse<RecentActivity[]>;
