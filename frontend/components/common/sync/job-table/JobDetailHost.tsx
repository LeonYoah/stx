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

'use client';

/**
 * JobDetailHost：可在工作台外使用的作业详情宿主（大盘行「详情」/ 抽屉「完整详情」入口）。
 * 内部实例化 useJobLogs / useJobCheckpoint 组装 ConsolePanel / CheckpointWorkspacePanel 插槽，
 * 再用 JobDetailDialog 渲染；作业活动时每 5s 调 getJob 刷新，保证弹窗里的状态是最新的。
 * 日志与 Checkpoint 只在对应页签打开时才加载（enabled 按页签门控），关闭详情后不再有任何轮询请求。
 *
 * JobDetailHost: a workbench-free host for the job detail dialog (dashboard row "Detail" / drawer
 * "Full detail" entry). It instantiates useJobLogs / useJobCheckpoint to assemble the ConsolePanel /
 * CheckpointWorkspacePanel slots, renders JobDetailDialog, and polls getJob every 5s while the job is
 * active so the dialog always shows a fresh job. Logs and checkpoint load only while their tab is open
 * (enabled is gated per tab); no polling requests remain after the detail closes.
 *
 * 决定记录：工作台的「大日志弹窗」是 DataSyncStudio 的内联 JSX（SearchField + VirtualizedLogViewer +
 * 混合模式横幅 + 关键字 / 级别状态），不是可复用组件，本期无法直接搬进 Host。因此 ConsolePanel 的
 * onExpand 改为可选属性，Host 不传时「展开」按钮隐藏；后续把该弹窗抽成共享组件后再接回。
 * 同理，Checkpoint 文件「检查」依赖工作台编辑器与自定义变量，Host 不传 onInspectCheckpointFile，
 * CheckpointWorkspacePanel 会回退为「在工作台中检查」链接。
 *
 * Decision log: the workbench's full log viewer dialog is inline JSX inside DataSyncStudio
 * (SearchField + VirtualizedLogViewer + mixed-mode banner + keyword/level state), not a reusable
 * component, so it cannot be reused here this iteration. ConsolePanel's onExpand is therefore optional
 * and the expand button stays hidden while unset; rewire once that dialog is extracted into a shared
 * component. Similarly, checkpoint file inspection depends on the workbench editor and custom
 * variables, so the Host omits onInspectCheckpointFile and CheckpointWorkspacePanel falls back to an
 * "inspect in workbench" link.
 */

import {useEffect, useState} from 'react';
import services from '@/lib/services';
import type {SyncJobInstance} from '@/lib/services/sync';
import {JobDetailDialog} from '../job-detail/JobDetailDialog';
import type {JobDetailTab} from '../job-detail/types';
import {ConsolePanel} from '../ConsolePanel';
import {CheckpointWorkspacePanel} from '../CheckpointPanels';
import {
  getDisplayJobLifecycleStatus,
  isJobLifecycleActive,
  type LogFilterMode,
} from '../sync-studio-utils';
import type {JobActorContext, JobRowAction} from './types';
import {useJobLogs} from './use-job-logs';
import {useJobCheckpoint} from './use-job-checkpoint';

// 活动作业的刷新间隔（详情弹窗内作业状态 5s 内更新）。/ Refresh interval for active jobs (detail state updates within 5s).
const JOB_DETAIL_POLL_MS = 5000;

export interface JobDetailHostProps {
  job: SyncJobInstance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tab: JobDetailTab;
  onTabChange: (tab: JobDetailTab) => void;
  /** 操作权限上下文（大盘逐行构造；与 JobDetailDialog 的 actionContext 同构）。/ Action permission context (the dashboard builds it per row; same shape as JobDetailDialog's actionContext). */
  actor: JobActorContext;
  /** 行操作透传给宿主：大盘负责确认弹窗与结果 toast。/ Row actions forwarded to the host: the dashboard owns confirmation + result toasts. */
  onAction: (action: JobRowAction, job: SyncJobInstance) => void;
  monacoTheme: string;
}

/**
 * 弹窗打开期间跟踪作业最新状态：作业活动时每 5s 调 getJob 刷新；其余时间直接用宿主传入的 job。
 * Track the job while the dialog is open: poll getJob every 5s while the job is active; otherwise the
 * job passed by the host is used as-is.
 */
function useJob(
  job: SyncJobInstance | null,
  enabled: boolean,
): SyncJobInstance | null {
  const [liveJob, setLiveJob] = useState<SyncJobInstance | null>(job);
  const jobId = job?.id ?? null;

  // 宿主传入的作业更新（如列表轮询拿到更新副本）时同步到本地。
  // Sync when the host passes an updated job (e.g. the list poll returns a fresher copy).
  useEffect(() => {
    setLiveJob(job);
  }, [job]);

  const active = liveJob
    ? isJobLifecycleActive(getDisplayJobLifecycleStatus(liveJob))
    : false;

  useEffect(() => {
    if (!enabled || !jobId) {
      return;
    }
    let disposed = false;
    const load = async () => {
      try {
        const next = await services.sync.getJob(jobId);
        if (!disposed) {
          setLiveJob(next);
        }
      } catch {
        // 瞬时轮询失败保留上一份数据。/ Transient poll failures keep the previous copy.
      }
    };
    // 打开即取一次全量：列表行的 result_preview 已被后端瘦身（只留展示键），拓扑 / 指标 / 日志页签
    // 需要完整字段；终态作业不再轮询，活动作业继续 5s 刷新。
    // Fetch once on open: the list row's result_preview is pruned by the backend (display keys only)
    // while the topology / metrics / logs tabs need the full record. Terminal jobs stop here;
    // active jobs keep polling every 5s.
    void load();
    if (!active) {
      return () => {
        disposed = true;
      };
    }
    const timer = window.setInterval(() => {
      void load();
    }, JOB_DETAIL_POLL_MS);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [enabled, jobId, active]);

  return liveJob;
}

export function JobDetailHost({
  job,
  open,
  onOpenChange,
  tab,
  onTabChange,
  actor,
  onAction,
  monacoTheme,
}: JobDetailHostProps) {
  const liveJob = useJob(job, open);
  const jobId = liveJob?.id ?? null;

  // 日志级别筛选由 Host 自持；关键字输入只存在于（本期未复用的）展开弹窗，固定传空串。
  // The host owns the log level filter; the keyword box only exists in the (unreused) expanded
  // dialog, so the keyword stays ''.
  const [logFilterMode, setLogFilterMode] = useState<LogFilterMode>('all');

  // 日志与 Checkpoint 只在对应页签打开时加载 / 轮询。/ Logs and checkpoint load / poll only while their tab is open.
  const logsEnabled = open && tab === 'logs';
  const checkpointEnabled = open && tab === 'checkpoint';
  const logs = useJobLogs({
    jobId,
    enabled: logsEnabled,
    keyword: '',
    level: logFilterMode,
  });
  const checkpoint = useJobCheckpoint({job: liveJob, enabled: checkpointEnabled});

  return (
    <JobDetailDialog
      job={liveJob}
      open={open}
      onOpenChange={onOpenChange}
      tab={tab}
      onTabChange={onTabChange}
      actionContext={actor}
      onRecover={() => {
        if (liveJob) {
          onAction('recover', liveJob);
        }
      }}
      onCancel={() => {
        if (liveJob) {
          onAction('stop', liveJob);
        }
      }}
      onSavepointStop={() => {
        if (liveJob) {
          onAction('savepointStop', liveJob);
        }
      }}
      monacoTheme={monacoTheme}
      logsSlot={
        <ConsolePanel
          job={liveJob}
          logsResult={logs.result}
          loading={logs.loading}
          filterMode={logFilterMode}
          onFilterChange={setLogFilterMode}
          // onExpand 不传 → 隐藏「展开」按钮（见文件头说明）。/ onExpand omitted → the expand button is hidden (see the header note).
        />
      }
      checkpointSlot={
        <CheckpointWorkspacePanel
          job={liveJob}
          checkpointSnapshot={checkpoint.snapshot}
          loading={checkpoint.loading}
          checkpointFiles={checkpoint.files}
          checkpointFilesLoading={checkpoint.filesLoading}
          // 不传 onInspectCheckpointFile → 「检查」按钮回退为「在工作台中检查」链接。
          // onInspectCheckpointFile omitted → the inspect action falls back to an "inspect in workbench" link.
          onRefresh={checkpoint.reload}
        />
      }
    />
  );
}
