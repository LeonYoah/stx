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

import {useCallback, useMemo, useRef, useState} from 'react';
import {useTheme} from 'next-themes';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {useAuth} from '@/hooks/use-auth';
import {Button} from '@/components/arc/button/button';
import {BatchBar, usePagedSelection} from '@/components/common/layout';
import services from '@/lib/services';
import type {
  SyncJobListSort,
  SyncJobStatusGroup,
  SyncJobSummary,
} from '@/lib/services/sync';
import {
  JobDetailHost,
  JobTable,
  JobTableToolbar,
  mergeKnownJobs,
  useJobBulkActions,
  useJobList,
  useJobSparklines,
  useNotifiableUsers,
  JOB_STATUS_GROUPS,
  type JobActorContext,
  type JobBulkAction,
  type JobListQuery,
  type JobRowAction,
} from '@/components/common/sync/job-table';
import type {JobDetailTab} from '@/components/common/sync/job-detail/types';
import type {DashboardQuery} from './use-dashboard-query';

const PAGE_SIZE = 50;
const LIST_POLL_MS = 30_000;
const DEFAULT_SORT: SyncJobListSort = 'started_at';
const VALID_SORTS: readonly string[] = [
  'attention',
  'started_at',
  'created_at',
  'rate',
  'cdc_delay',
];

/** 待确认的批量 / 单行作业操作；bulk=true 时走批量汇总，否则逐条 toast。/ Pending bulk or row job action. */
interface PendingJobConfirm {
  action: JobRowAction;
  jobs: SyncJobSummary[];
  bulk: boolean;
}

/**
 * 大盘「作业」页签宿主层：URL 查询状态 ↔ JobListQuery 映射、批量确认、
 * 集群名映射与行操作回调；表格与工具栏全部由 sync/job-table 呈现，详情由 JobDetailHost 就地打开。
 * Dashboard jobs tab host: maps the URL query state to JobListQuery, owns the bulk / row confirm dialog,
 * cluster name mapping and row action callbacks; the table and toolbar live in sync/job-table and the
 * in-place detail is hosted by JobDetailHost.
 */
export function JobListTab({
  query,
  update,
  clusterNames,
}: {
  query: DashboardQuery;
  update: (
    patch: Partial<Record<keyof DashboardQuery, string | number | undefined>>,
  ) => void;
  clusterNames: Map<number, string>;
}) {
  const t = useTranslations('jobTable');
  const {resolvedTheme} = useTheme();
  const monacoTheme = resolvedTheme === 'light' ? 'vs' : 'vs-dark';
  const {user: currentUser} = useAuth();
  const initiatorUsers = useNotifiableUsers();
  // 完整详情弹窗：点击行 / 行操作「详情」/ 多表徽章都就地打开工作台同款 JobDetailDialog（不再有轻量预览抽屉）。
  // Full detail dialog: row click, the row "details" action, and the multi-table badge all open the same
  // workbench JobDetailDialog in place (no lightweight preview drawer).
  const [detail, setDetail] = useState<{
    job: SyncJobSummary;
    tab: JobDetailTab;
  } | null>(null);
  const [confirm, setConfirm] = useState<PendingJobConfirm | null>(null);

  // URL 查询 → 受控 JobListQuery（排序 / 状态非法值在这里归一）。
  // URL query → controlled JobListQuery (invalid sort / status values normalized here).
  const jobQuery = useMemo<JobListQuery>(
    () => ({
      statusGroup: (JOB_STATUS_GROUPS as readonly string[]).includes(
        query.status,
      )
        ? (query.status as SyncJobStatusGroup)
        : '',
      mode:
        query.mode === 'streaming' || query.mode === 'batch' ? query.mode : '',
      source:
        query.source === 'stx' || query.source === 'external'
          ? query.source
          : '',
      keyword: query.q,
      clusterId: query.clusterId,
      sort: VALID_SORTS.includes(query.sort)
        ? (query.sort as SyncJobListSort)
        : DEFAULT_SORT,
      order:
        query.order === 'asc' || query.order === 'desc' ? query.order : 'desc',
      // 顶栏时间窗 → 「窗口内活跃」筛选：自定义窗口传毫秒端点，预设传 range（'all' 不筛）。
      // The header range becomes the "active in window" filter: a custom window sends ms ends, a preset
      // sends range ('all' stays unfiltered).
      activeRange:
        query.start || query.listRange === 'all' ? '' : query.listRange,
      activeFrom: query.start,
      activeTo: query.end,
      page: query.page,
    }),
    [query],
  );

  // patch 语义统一映射回 URL（空值删除参数）。/ Patch semantics mapped back onto the URL (empty drops the key).
  const onQueryChange = useCallback(
    (patch: Partial<JobListQuery>) => {
      const urlPatch: Partial<
        Record<keyof DashboardQuery, string | number | undefined>
      > = {};
      if (patch.statusGroup !== undefined) {
        urlPatch.status = patch.statusGroup || undefined;
      }
      if (patch.mode !== undefined) {
        urlPatch.mode = patch.mode || undefined;
      }
      if (patch.source !== undefined) {
        urlPatch.source = patch.source || undefined;
      }
      if (patch.keyword !== undefined) {
        urlPatch.q = patch.keyword || undefined;
      }
      if (patch.clusterId !== undefined) {
        urlPatch.clusterId = patch.clusterId;
      }
      if (patch.sort !== undefined) {
        urlPatch.sort = patch.sort;
      }
      if (patch.order !== undefined) {
        urlPatch.order = patch.order;
      }
      if (patch.page !== undefined) {
        urlPatch.page = patch.page;
      }
      update(urlPatch);
    },
    [update],
  );

  const list = useJobList({
    scope: {kind: 'all'},
    query: jobQuery,
    pageSize: PAGE_SIZE,
    pollMs: LIST_POLL_MS,
  });
  const sparklines = useJobSparklines(list.items, list.updatedAt);
  const bulk = useJobBulkActions(list.reload);

  // 跨页勾选在执行/手动清空前都保留；已消失的作业交给批量动作逐条回报。
  // Selections survive paging until executed or cleared; jobs that vanish are
  // reported per-item by the bulk action.
  const pageJobIds = useMemo(() => list.items.map((job) => job.id), [list.items]);
  const {ids: selected, setIds: setSelected} = usePagedSelection(pageJobIds);
  const knownJobsRef = useRef<ReadonlyMap<number, SyncJobSummary>>(new Map());

  // 大盘逐行构造权限上下文：isOwner = 作业发起人；后端仍是最终裁决。
  // Per-row actor context on the dashboard: isOwner means the job initiator; the backend stays the final arbiter.
  const actor = useCallback(
    (job: SyncJobSummary): JobActorContext => ({
      isAdmin: currentUser?.is_admin ?? false,
      isOwner:
        currentUser !== null &&
        currentUser !== undefined &&
        job.created_by === currentUser.id,
      canRun: true,
      currentUserId: currentUser?.id,
      disableRecover: false,
    }),
    [currentUser],
  );

  // 单行操作：确认后直接调接口，toast 提示结果并刷新列表。
  // Row actions: call the endpoint after confirmation, toast the outcome and reload the list.
  const runRowAction = useCallback(
    async (action: JobRowAction, job: SyncJobSummary) => {
      try {
        if (action === 'recover') {
          await services.sync.recoverJob(job.id);
          toast.success(t('action.recoverOk', {id: job.id}));
        } else {
          await services.sync.cancelJob(job.id, {
            stop_with_savepoint: action === 'savepointStop',
          });
          toast.success(
            t(
              action === 'savepointStop'
                ? 'action.savepointOk'
                : 'action.stopOk',
              {
                id: job.id,
              },
            ),
          );
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t('action.failed'));
      } finally {
        list.reload();
      }
    },
    [list.reload, t],
  );

  // 跨页勾选的作业可能不在当前页 items 里：快照累积所有见过的作业，按 id 解析。
  // Jobs picked on other pages are absent from the current items: resolve them
  // through an accumulating snapshot of every job seen so far.
  const knownJobs = useMemo(() => {
    knownJobsRef.current = mergeKnownJobs(
      [],
      list.items,
      knownJobsRef.current,
    );
    return knownJobsRef.current;
  }, [list.items]);
  const selectedJobs = useMemo(
    () =>
      [...selected]
        .map((id) => knownJobs.get(id))
        .filter((job): job is SyncJobSummary => job != null),
    [selected, knownJobs],
  );

  const handleConfirm = () => {
    const pending = confirm;
    setConfirm(null);
    if (!pending || pending.jobs.length === 0) {
      return;
    }
    if (pending.bulk) {
      // 批量：走 useJobBulkActions 的串行汇总，结果展示在批量条里。
      // Bulk: run through useJobBulkActions' serial aggregation; the result shows in the bulk bar.
      void bulk
        .run(pending.action as JobBulkAction, pending.jobs)
        .then(() => setSelected(new Set()));
    } else {
      void runRowAction(pending.action, pending.jobs[0]);
    }
  };

  return (
    <div className='flex flex-col gap-3' data-testid='home-job-list'>
      <JobTableToolbar
        preset='global'
        query={jobQuery}
        onQueryChange={onQueryChange}
        statusCounts={list.statusCounts}
        testIdPrefix='home-job'
      />

      <JobTable
        preset='global'
        data={list}
        query={jobQuery}
        onQueryChange={onQueryChange}
        pageSize={PAGE_SIZE}
        sparklines={sparklines}
        clusterNames={clusterNames}
        actor={actor}
        initiatorUsers={initiatorUsers}
        currentUserId={currentUser?.id}
        currentUsername={currentUser?.username || currentUser?.nickname}
        selection={{ids: selected, onChange: setSelected}}
        bulkBar={
          <BatchBar
            placement='center'
            selectedCount={selected.size}
            selectedText={t('selected', {count: selected.size})}
            clearLabel={t('clearSelection')}
            busy={Boolean(bulk.running)}
            testId='home-job-bulk'
            result={
              bulk.result ? (
                <>
                  {t('bulk.result', {
                    ok: bulk.result.ok,
                    failed: bulk.result.failures.length,
                    skipped: bulk.result.skipped,
                  })}
                  {bulk.result.failures.slice(0, 3).map((item) => (
                    <span key={item.id} className='ml-2 text-destructive'>
                      {t('bulk.itemFailed', {
                        id: item.id,
                        message: item.message,
                      })}
                    </span>
                  ))}
                </>
              ) : null
            }
            onClear={() => {
              setSelected(new Set());
              bulk.clearResult();
            }}
          >
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={Boolean(bulk.running)}
              onClick={() =>
                setConfirm({action: 'stop', jobs: selectedJobs, bulk: true})
              }
            >
              {t('stop')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={Boolean(bulk.running)}
              onClick={() =>
                setConfirm({action: 'recover', jobs: selectedJobs, bulk: true})
              }
            >
              {t('recover')}
            </Button>
          </BatchBar>
        }
        onRowActivate={(job) => setDetail({job, tab: 'topology'})}
        onOpenDetail={(job, tab) => setDetail({job, tab: tab ?? 'topology'})}
        onAction={(action, job) =>
          setConfirm({action, jobs: [job], bulk: false})
        }
        testIdPrefix='home-job'
      />

      <AlertDialog
        open={confirm !== null}
        onOpenChange={(open) => !open && setConfirm(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirm
                ? t(`confirm.${confirm.action}Title`, {
                    count: confirm.jobs.length,
                  })
                : ''}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirm ? t(`confirm.${confirm.action}Desc`) : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('confirm.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirm}>
              {t('confirm.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <JobDetailHost
        job={detail?.job ?? null}
        open={detail !== null}
        onOpenChange={(open) => !open && setDetail(null)}
        tab={detail?.tab ?? 'topology'}
        onTabChange={(tab) =>
          setDetail((prev) => (prev ? {...prev, tab} : prev))
        }
        actor={
          detail
            ? actor(detail.job)
            : {
                isAdmin: false,
                isOwner: false,
                canRun: false,
                disableRecover: false,
              }
        }
        onAction={(action, job) =>
          setConfirm({action, jobs: [job], bulk: false})
        }
        monacoTheme={monacoTheme}
      />
    </div>
  );
}
