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

import type {ReactNode} from 'react';
import type {useFormatter, useTranslations} from 'next-intl';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';
import type {
  SyncJobInstance,
  SyncJobListSort,
  SyncJobSparklines,
  SyncJobStatusGroup,
} from '@/lib/services/sync';
import type {JobDetailTab} from '../job-detail/types';

/** 表格预设：global = 大盘全局列表；task = 工作台单任务列表 / Table presets: global dashboard list vs. single-task workbench list. */
export type JobTablePreset = 'global' | 'task';

/** 列标识 / Column identifiers. */
export type JobColumnId =
  | 'select'
  | 'job'
  | 'cluster'
  | 'run'
  | 'status'
  | 'initiator'
  | 'rates'
  | 'totals'
  | 'cdc'
  | 'timeline'
  | 'actions';

/**
 * 列表范围：大盘全局（固定排除预览运行）或工作台当前任务（含预览运行）。
 * List scope: the global dashboard (preview runs always excluded) or one task in the workbench (previews included).
 */
export type JobTableScope =
  | {kind: 'all'}
  | {kind: 'task'; taskId: number};

/**
 * 受控查询状态：URL（大盘）与本地状态（工作台）共用一个形状，patch 语义一致。
 * Controlled query state shared by the URL-driven dashboard and the workbench's local state; both use patch semantics.
 */
export interface JobListQuery {
  /** '' = 全部 / '' means unfiltered. */
  statusGroup: SyncJobStatusGroup | '';
  /** 仅 global 预设暴露 / Exposed by the global preset only. */
  mode: 'streaming' | 'batch' | '';
  keyword: string;
  /** 仅 global / Global preset only. */
  clusterId?: number;
  sort: SyncJobListSort;
  order: 'asc' | 'desc';
  /**
   * 活跃窗口预设（'' = 不筛；'15m'|'1h'|'6h'|'24h'|'7d' = 窗口内运行过的作业，服务端按请求时刻滚动求窗）。
   * Active window preset ('' = unfiltered; presets match jobs that ran during the window, resolved server-side
   * per request so polling sees a rolling window).
   */
  activeRange?: string;
  /** 自定义活跃窗口（毫秒），两端齐备时优先于 activeRange。/ Custom active window (ms); wins over activeRange. */
  activeFrom?: number;
  activeTo?: number;
  /** 从 1 开始 / 1-based page. */
  page: number;
}

/** 行内操作：详情之外的作业动作，宿主负责确认弹窗与接口调用 / Row actions beyond opening the detail. */
export type JobRowAction = 'stop' | 'savepointStop' | 'recover';

/**
 * 操作判定的权限上下文：工作台固定一份，大盘按行构造（isOwner 逐行看 created_by）。
 * Permission context for action availability: fixed per workbench task, built per row on the dashboard.
 */
export interface JobActorContext {
  isAdmin: boolean;
  isOwner: boolean;
  canRun: boolean;
  currentUserId?: number;
  disableRecover: boolean;
}

/** jobTable 命名空间的翻译函数 / Translator bound to the jobTable namespace. */
export type JobTableTranslator = ReturnType<typeof useTranslations>;

/**
 * 列渲染上下文：列注册表的 header / cell / visibleWhen 共用这一份。
 * Shared context for the column registry's header / cell / visibleWhen callbacks.
 */
export interface JobTableContext {
  preset: JobTablePreset;
  query: JobListQuery;
  /** 当前页行（已合并 pinned / overlay）/ Rows of the current page (pinned / overlay already merged). */
  items: SyncJobInstance[];
  /** 本次刷新时刻，作为运行中作业时长的基准 / Refresh instant used as the base of running durations. */
  updatedAt: number | null;
  sparklines: SyncJobSparklines | null;
  clusterNames?: Map<number, string>;
  /** 逐行解析权限上下文（props 的函数形态已归一）/ Per-row actor context (normalized from the prop). */
  actorOf: (job: SyncJobInstance) => JobActorContext;
  initiatorUsers: NotificationRecipientUser[];
  currentUserId?: number;
  currentUsername?: string;
  selection?: {ids: Set<number>; onChange: (ids: Set<number>) => void};
  t: JobTableTranslator;
  format: ReturnType<typeof useFormatter>;
  onRowActivate: (job: SyncJobInstance) => void;
  onOpenDetail: (job: SyncJobInstance, tab?: JobDetailTab) => void;
  onAction: (action: JobRowAction, job: SyncJobInstance) => void;
  testIdPrefix: string;
}

/**
 * 列定义：唯一一份列注册表，两预设共用（见父 design §4）。
 * Column definition: the single registry shared by both presets (parent design §4).
 */
export interface JobColumnDef {
  id: JobColumnId;
  header: (ctx: JobTableContext) => ReactNode;
  /** 有值则可排序（映射到服务端排序键）/ Server sort key when sortable. */
  sortKey?: SyncJobListSort;
  /** 额外排序条件：rate / cdc_delay 只在 running 组可排 / Extra gating, e.g. rate / cdc_delay only inside the running group. */
  sortableWhen?: (query: JobListQuery) => boolean;
  align?: 'start' | 'end';
  minWidth?: number;
  /** 固定宽度（复选框列用）/ Fixed width (used by the checkbox column). */
  width?: number;
  presets: JobTablePreset[];
  /** 动态显示（如 task 预设的 CDC 列在本页有值时才出现）/ Dynamic visibility (e.g. task CDC column shows only when any row has a value). */
  visibleWhen?: (items: SyncJobInstance[], ctx: JobTableContext) => boolean;
  cell: (job: SyncJobInstance, ctx: JobTableContext) => ReactNode;
}

/** 状态胶囊顺序（与后端 status_group 对齐）/ Status pill order (matches the backend status_group values). */
export const JOB_STATUS_GROUPS: SyncJobStatusGroup[] = [
  'failed',
  'running',
  'pending',
  'stopping',
  'canceled',
  'finished',
];

/** 默认查询状态 / Default query state. */
export const DEFAULT_JOB_LIST_QUERY: JobListQuery = {
  statusGroup: '',
  mode: '',
  keyword: '',
  clusterId: undefined,
  sort: 'started_at',
  order: 'desc',
  page: 1,
};
