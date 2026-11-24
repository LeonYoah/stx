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
 * 统一作业表公共出口（大盘 global / 工作台 task 两预设共用）。
 * 文件尾部「子任务 C」段是 job-detail-host 子任务产出的导出，由该任务维护。
 * Public surface of the unified job table shared by the dashboard (global) and workbench (task) presets.
 * The trailing "Subtask C" section holds the job-detail-host exports and is maintained by that subtask.
 */
export {JobTable} from './JobTable';
export type {JobTableProps} from './JobTable';
export {JobTableToolbar} from './JobTableToolbar';
export type {JobTableToolbarProps} from './JobTableToolbar';
export {JobRowActions} from './JobRowActions';
export {JOB_COLUMNS} from './job-table-columns';
export {
  formatRatePair,
  formatTotalPair,
  isRunningStatus,
  jobDisplayName,
  jobDurationSeconds,
  jobListRates,
  jobTimeBounds,
  jobWorkbenchHref,
  resolveJobInitiatorName,
} from './job-table-format';
export {
  canApplyJobBulkAction,
  useJobBulkActions,
} from './use-job-bulk-actions';
export type {JobBulkAction, JobBulkResult} from './use-job-bulk-actions';
export {useJobList, buildJobListParams, resolveJobListSort} from './use-job-list';
export type {UseJobListOptions, UseJobListResult} from './use-job-list';
export {mergeKnownJobs, retainedActiveJobIds} from './known-jobs';
export {useJobSparklines} from './use-job-sparklines';
export {useJobTableQuery} from './use-job-table-query';
export {useNotifiableUsers} from './use-notifiable-users';
export {
  DEFAULT_JOB_LIST_QUERY,
  JOB_STATUS_GROUPS,
} from './types';
export type {
  JobActorContext,
  JobColumnDef,
  JobColumnId,
  JobListQuery,
  JobRowAction,
  JobTableContext,
  JobTablePreset,
  JobTableScope,
  JobTableTranslator,
} from './types';
// —— 子任务 C（job-detail-host）产物：作业详情宿主与日志 / Checkpoint 数据 hooks ——
// —— Subtask C (job-detail-host): the detail host plus the logs / checkpoint data hooks ——
export {useJobDetail} from './use-job-detail';
export type {UseJobDetailOptions} from './use-job-detail';
export {useJobLogs} from './use-job-logs';
export type {UseJobLogsOptions, UseJobLogsResult} from './use-job-logs';
export {useJobCheckpoint} from './use-job-checkpoint';
export type {
  UseJobCheckpointOptions,
  UseJobCheckpointResult,
} from './use-job-checkpoint';
export {JobDetailHost} from './JobDetailHost';
export type {JobDetailHostProps} from './JobDetailHost';
