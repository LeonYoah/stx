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

import {useCallback, useEffect, useMemo, useRef, useState, type ReactNode} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {
  ArrowLeft,
  Clock3,
  History,
  ListOrdered,
  Package,
  PlayCircle,
  PlugZap,
  RefreshCw,
  Server,
  Terminal,
} from 'lucide-react';
import {Alert as ArcAlert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Badge} from '@/components/arc/badge/badge';
import {ButtonGroup} from '@/components/arc/button-group/button-group';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Select} from '@/components/arc/select/select';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {TableLoadingBar, WorkspaceHeader} from '@/components/common/layout';
import {Pagination} from '@/components/ui/pagination';
import {HELP_HINT_DELAY_MS, HelpHint} from '@/components/common/HelpHint';
import {Progress} from '@/components/ui/progress';
import {Skeleton} from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import services from '@/lib/services';
import {
  loadStUpgradeSession,
  patchStUpgradeSession,
} from '@/lib/st-upgrade-session';
import type {
  LogLevel,
  StepCode,
  TaskLogsData,
  TaskLogsQuery,
  TaskStepsData,
  UpgradePlanRecord,
  UpgradeTask,
} from '@/lib/services/st-upgrade';
import {UpgradeSteps} from './UpgradeSteps';
import {
  buildPlanStepMap,
  calculateExecutionProgress,
  getExecutionStatusLabel,
  upgradeStatusTone,
  getStepNodeCount,
} from './utils';
import {useRefreshOnSettle} from './useRefreshOnSettle';
import {formatDateTimeTz} from '@/lib/timezone';

interface ClusterUpgradeExecuteProps {
  clusterId: number;
}

const ACTIVE_TASK_STATUSES: UpgradeTask['status'][] = [
  'pending',
  'ready',
  'running',
];
const EXECUTION_PANES_HEIGHT_CLASS = 'xl:h-[680px]';

export function ClusterUpgradeExecute({clusterId}: ClusterUpgradeExecuteProps) {
  const t = useTranslations('stUpgrade');
  const commonT = useTranslations('common');
  const router = useRouter();
  const searchParams = useSearchParams();

  const [plan, setPlan] = useState<UpgradePlanRecord | null>(null);
  const [task, setTask] = useState<UpgradeTask | null>(null);
  const [stepsData, setStepsData] = useState<TaskStepsData | null>(null);
  const [logsData, setLogsData] = useState<TaskLogsData | null>(null);
  const [logsQuery, setLogsQuery] = useState<TaskLogsQuery>({
    page: 1,
    page_size: 100,
  });
  const [loading, setLoading] = useState(true);
  // planLoadError：读取失败（非 404）；missingPlan 仅用于「确实没有计划」。
  // planLoadError covers read failures (not 404); missingPlan is only for true absence.
  const [planLoadError, setPlanLoadError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [starting, setStarting] = useState(false);
  // 步骤 / 节点视图切换，释放横向空间 / Toggle steps vs nodes for horizontal space
  const [executionView, setExecutionView] = useState<'steps' | 'nodes'>(
    'steps',
  );
  const logEndRef = useRef<HTMLDivElement | null>(null);
  const initializedLogsTaskRef = useRef<number | null>(null);
  const previousLogsTotalPagesRef = useRef(1);
  const reqSeqRef = useRef(0);

  const planIdFromQuery = Number(searchParams.get('planId') || 0);
  const taskIdFromQuery = Number(searchParams.get('taskId') || 0);

  const loadTaskState = useCallback(
    async (taskId: number, query: TaskLogsQuery) => {
      const seq = ++reqSeqRef.current;
      setRefreshing(true);
      try {
        const [taskResult, stepsResult, logsResult] = await Promise.all([
          services.stUpgrade.getTask(taskId),
          services.stUpgrade.getTaskSteps(taskId),
          services.stUpgrade.getTaskLogs(taskId, query),
        ]);
        if (seq !== reqSeqRef.current) {
          return;
        }
        setPlan(taskResult.plan);
        setTask(taskResult);
        setStepsData(stepsResult);
        setLogsData(logsResult);
        setPlanLoadError(null);
        patchStUpgradeSession(clusterId, {
          clusterId,
          plan: taskResult.plan,
          task: taskResult,
        });
      } catch (error) {
        if (seq !== reqSeqRef.current) {
          return;
        }
        const message =
          error instanceof Error ? error.message : t('loadExecuteFailed');
        const status = (error as Error & {status?: number}).status;
        if (status === 404) {
          setPlan(null);
          setTask(null);
          setPlanLoadError(null);
        } else {
          setPlanLoadError(message);
        }
        toast.error(message);
      } finally {
        if (seq === reqSeqRef.current) {
          setRefreshing(false);
        }
      }
    },
    [clusterId, t],
  );

  const loadInitialState = useCallback(async () => {
    setLoading(true);
    try {
      const session = loadStUpgradeSession(clusterId);
      const sessionPlanId = session?.plan?.id || 0;
      const sessionTask = session?.task;

      let initialPlanId = 0;
      let initialTaskId = 0;

      if (taskIdFromQuery) {
        initialTaskId = taskIdFromQuery;
        initialPlanId =
          planIdFromQuery || sessionTask?.plan_id || sessionPlanId || 0;
      } else if (planIdFromQuery) {
        initialPlanId = planIdFromQuery;
        if (sessionTask?.plan_id === planIdFromQuery) {
          initialTaskId = sessionTask.id;
        }
      } else if (sessionTask?.id) {
        initialTaskId = sessionTask.id;
        initialPlanId = sessionTask.plan_id || sessionPlanId || 0;
      } else if (sessionPlanId) {
        initialPlanId = sessionPlanId;
      }

      if (!initialPlanId && !initialTaskId) {
        setPlan(null);
        setTask(null);
        setStepsData(null);
        setLogsData(null);
        setPlanLoadError(null);
        return;
      }

      if (initialTaskId) {
        await loadTaskState(initialTaskId, logsQuery);
        return;
      }

      const planResult = await services.stUpgrade.getPlan(initialPlanId);
      setPlan(planResult);
      setTask(null);
      setStepsData(null);
      setLogsData(null);
      setPlanLoadError(null);
      patchStUpgradeSession(clusterId, {
        clusterId,
        request: session?.request,
        precheck: session?.precheck,
        plan: planResult,
        task: undefined,
      });
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('loadExecuteFailed');
      const status = (error as Error & {status?: number}).status;
      // 404 → 计划不存在；其它（含 snapshot 扫描失败的 500）→ 读取失败错误态。
      // 404 → plan missing; other errors (incl. snapshot scan 500) → read-failure state.
      if (status === 404) {
        setPlan(null);
        setPlanLoadError(null);
      } else {
        setPlanLoadError(message);
      }
      toast.error(message);
    } finally {
      setLoading(false);
    }
  }, [
    clusterId,
    loadTaskState,
    logsQuery,
    planIdFromQuery,
    taskIdFromQuery,
    t,
  ]);

  useEffect(() => {
    void loadInitialState();
  }, [loadInitialState]);

  const hasActiveTask = useMemo(() => {
    if (!task) {
      return false;
    }

    return (
      ACTIVE_TASK_STATUSES.includes(task.status) ||
      task.rollback_status === 'rollback_running'
    );
  }, [task]);

  useEffect(() => {
    if (!task?.id) {
      return;
    }
    void loadTaskState(task.id, logsQuery);
  }, [logsQuery, loadTaskState, task?.id]);

  useEffect(() => {
    initializedLogsTaskRef.current = null;
    previousLogsTotalPagesRef.current = 1;
  }, [task?.id]);

  useEffect(() => {
    if (!task?.id || !hasActiveTask) {
      return;
    }

    const timer = setInterval(() => {
      void loadTaskState(task.id, logsQuery).catch(() => undefined);
    }, 3000);

    return () => {
      clearInterval(timer);
    };
  }, [hasActiveTask, loadTaskState, logsQuery, task?.id]);

  // 任务从活跃转为终态时再拉取一次，避免停在最后一次轮询的中间态（例如节点仍显示回滚中）。
  // Refresh once more when the task settles so the page never freezes on an intermediate snapshot.
  useRefreshOnSettle(hasActiveTask, () => {
    if (task?.id) {
      void loadTaskState(task.id, logsQuery).catch(() => undefined);
    }
  });

  const handleStartExecution = async () => {
    if (!plan) {
      toast.error(t('missingPlan'));
      return;
    }
    setStarting(true);
    try {
      const result = await services.stUpgrade.executePlanSafe({
        plan_id: plan.id,
      });
      if (!result.success || !result.data) {
        toast.error(result.error || t('executeFailed'));
        return;
      }
      setTask(result.data);
      setPlan(result.data.plan);
      patchStUpgradeSession(clusterId, {
        clusterId,
        plan: result.data.plan,
        task: result.data,
      });
      toast.success(t('executionStarted'));
      router.replace(
        `/clusters/${clusterId}/upgrade/execute?planId=${plan.id}&taskId=${result.data.id}`,
      );
      await loadTaskState(result.data.id, logsQuery);
    } finally {
      setStarting(false);
    }
  };

  const currentPlan = task?.plan || plan;
  const stepMap = useMemo(
    () => buildPlanStepMap(currentPlan?.snapshot.steps || []),
    [currentPlan?.snapshot.steps],
  );
  const progress = useMemo(
    () => calculateExecutionProgress(stepsData?.steps || task?.steps || []),
    [stepsData?.steps, task?.steps],
  );
  const canRestartExecution = useMemo(() => {
    if (!task) {
      return false;
    }

    return !hasActiveTask && task.status !== 'succeeded';
  }, [hasActiveTask, task]);
  const startExecutionLabel = task?.id
    ? canRestartExecution
      ? t('restartExecution')
      : t('executionCreated')
    : t('startExecution');

  const nodeOptions = useMemo(
    () => stepsData?.node_executions || task?.node_executions || [],
    [stepsData?.node_executions, task?.node_executions],
  );
  const logsTotalPages = useMemo(() => {
    if (!logsData?.page_size) {
      return 1;
    }
    return Math.max(1, Math.ceil((logsData.total || 0) / logsData.page_size));
  }, [logsData?.page_size, logsData?.total]);
  const showPrimaryExecutionButton = task?.status !== 'succeeded';

  useEffect(() => {
    if (!task?.id || !logsData) {
      return;
    }

    const currentPage = logsQuery.page || 1;
    const totalPages = Math.max(
      1,
      Math.ceil(logsData.total / Math.max(logsData.page_size || 1, 1)),
    );

    if (currentPage > totalPages) {
      setLogsQuery((current) => ({
        ...current,
        page: totalPages,
      }));
      previousLogsTotalPagesRef.current = totalPages;
      return;
    }

    if (initializedLogsTaskRef.current !== task.id) {
      initializedLogsTaskRef.current = task.id;
      previousLogsTotalPagesRef.current = totalPages;
      if (currentPage === 1 && totalPages > 1) {
        setLogsQuery((current) => ({
          ...current,
          page: totalPages,
        }));
      }
      return;
    }

    if (
      hasActiveTask &&
      currentPage === previousLogsTotalPagesRef.current &&
      totalPages > previousLogsTotalPagesRef.current
    ) {
      setLogsQuery((current) => ({
        ...current,
        page: totalPages,
      }));
      previousLogsTotalPagesRef.current = totalPages;
      return;
    }

    previousLogsTotalPagesRef.current = totalPages;
  }, [hasActiveTask, logsData, logsQuery.page, task?.id]);

  useEffect(() => {
    if (!logsData?.items.length) {
      return;
    }

    requestAnimationFrame(() => {
      logEndRef.current?.scrollIntoView({
        behavior: hasActiveTask ? 'smooth' : 'auto',
        block: 'end',
      });
    });
  }, [hasActiveTask, logsData?.items, logsQuery.page]);

  if (loading) {
    return (
      <div
        className='upgrade-page arc-compact'
        data-testid='upgrade-execute-page'
      >
        <WorkspaceHeader
          breadcrumbs={[
            {
              label: t('clusterDetailBreadcrumb'),
              href: `/clusters/${clusterId}`,
            },
            {
              label: t('configTitle'),
              href: `/clusters/${clusterId}/upgrade/config`,
            },
            {
              label: t('executeTitle'),
            },
          ]}
          icon={<PlayCircle />}
          title={t('executeTitle')}
          subtitle={<span>{t('executeDescription')}</span>}
        />
        <UpgradeSteps current='execute' clusterId={clusterId} />
        <div className='space-y-4'>
          <div className='grid grid-cols-2 divide-y-[0.5px] divide-border/80 rounded-(--radius-panel) border border-(--border) bg-card xl:grid-cols-4 xl:divide-x-[0.5px] xl:divide-y-0'>
            <div className='p-4'>
              <Skeleton className='h-4 w-20' />
              <Skeleton className='mt-2 h-5 w-28' />
            </div>
            <div className='p-4'>
              <Skeleton className='h-4 w-20' />
              <Skeleton className='mt-2 h-5 w-28' />
            </div>
            <div className='p-4'>
              <Skeleton className='h-4 w-20' />
              <Skeleton className='mt-2 h-5 w-28' />
            </div>
            <div className='p-4'>
              <Skeleton className='h-4 w-20' />
              <Skeleton className='mt-2 h-5 w-28' />
            </div>
          </div>
          <div className='grid gap-4 xl:grid-cols-2'>
            <Skeleton className='h-80 w-full rounded-(--radius-panel)' />
            <Skeleton className='h-80 w-full rounded-(--radius-panel)' />
          </div>
        </div>
      </div>
    );
  }

  if (!currentPlan) {
    return (
      <div
        className='upgrade-page arc-compact'
        data-testid='upgrade-execute-page'
      >
        <WorkspaceHeader
          breadcrumbs={[
            {
              label: t('clusterDetailBreadcrumb'),
              href: `/clusters/${clusterId}`,
            },
            {
              label: t('configTitle'),
              href: `/clusters/${clusterId}/upgrade/config`,
            },
            {
              label: t('executeTitle'),
            },
          ]}
          icon={<PlayCircle />}
          title={t('executeTitle')}
          subtitle={<span>{t('executeDescription')}</span>}
        />
        <UpgradeSteps current='execute' clusterId={clusterId} />
        <div className='rounded-(--radius-panel) border border-(--border) bg-card px-6 py-10'>
          {planLoadError ? (
            <div
              className='flex flex-col items-center justify-center gap-3'
              role='alert'
              data-testid='upgrade-plan-load-error'
            >
              <EmptyState
                icon={<PlayCircle />}
                title={t('loadPlanFailed')}
                description={planLoadError || t('loadPlanFailedDesc')}
                action={
                  <Button
                    size='sm'
                    variant='secondary'
                    onClick={() => void loadInitialState()}
                  >
                    <RefreshCw className='size-3.5' aria-hidden />
                    {commonT('retry')}
                  </Button>
                }
              />
            </div>
          ) : (
            <EmptyState
              icon={<PlayCircle />}
              title={t('executeTitle')}
              description={t('missingPlan')}
              action={
                <Button
                  size='sm'
                  onClick={() =>
                    router.push(`/clusters/${clusterId}/upgrade/prepare`)
                  }
                >
                  <ArrowLeft className='size-3.5' aria-hidden />
                  {t('startFromPrepare')}
                </Button>
              }
            />
          )}
        </div>
      </div>
    );
  }

  const currentLogsPage = logsQuery.page || 1;
  const canLoadEarlierLogs = currentLogsPage > 1;

  return (
    <div
      className='upgrade-page arc-compact'
      data-testid='upgrade-execute-page'
    >
      <WorkspaceHeader
        breadcrumbs={[
          {
            label: t('clusterDetailBreadcrumb'),
            href: `/clusters/${clusterId}`,
          },
          {
            label: t('configTitle'),
            href: `/clusters/${clusterId}/upgrade/config`,
          },
          {
            label: t('executeTitle'),
          },
        ]}
        icon={<PlayCircle />}
        title={t('executeTitle')}
        subtitle={
          <span className='inline-flex flex-wrap items-center gap-1.5 tabular-nums'>
            <span>
              {currentPlan
                ? `${currentPlan.source_version} → ${currentPlan.target_version}`
                : t('executeStage')}
            </span>
            <HelpHint
              label={t('executeTitle')}
              content={t('executeDescription')}
            />
          </span>
        }
        actions={
          <div className='flex flex-wrap items-center gap-2'>
            <ButtonGroup
              label={t('upgradeStagesAria')}
              size='sm'
              items={[
                {
                  id: 'back',
                  label: t('backToConfig'),
                  icon: <ArrowLeft />,
                  onSelect: () =>
                    router.push(`/clusters/${clusterId}/upgrade/config`),
                },
                {
                  id: 'refresh',
                  label: commonT('refresh'),
                  icon: <RefreshCw />,
                  disabled: !task?.id,
                  onSelect: () => {
                    if (task?.id) {
                      void loadTaskState(task.id, logsQuery);
                    }
                  },
                },
              ]}
            />
            {showPrimaryExecutionButton ? (
              <Button
                size='sm'
                data-testid='upgrade-execute-start'
                onClick={handleStartExecution}
                loading={starting}
                disabled={!plan || hasActiveTask}
              >
                <PlayCircle className='size-3.5' aria-hidden />
                {startExecutionLabel}
              </Button>
            ) : null}
          </div>
        }
      />

      <UpgradeSteps current='execute' clusterId={clusterId} />

      {/* 4 项执行状态收敛为单条扁平摘要栏（0.5px 竖线分隔）/ Converge 4 status cells into one flat hairline bar */}
      <div className='relative overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'>
        <TableLoadingBar loading={refreshing} />
        <dl className='grid grid-cols-2 divide-y-[0.5px] divide-border/80 xl:grid-cols-4 xl:divide-x-[0.5px] xl:divide-y-0'>
          <StatusSummaryCell
            title={t('taskStatus')}
            value={task ? getExecutionStatusLabel(task.status) : t('notStarted')}
          />
          <StatusSummaryCell
            title={t('rollbackStatus')}
            value={
              task
                ? getExecutionStatusLabel(task.rollback_status)
                : t('notStarted')
            }
          />
          <StatusSummaryCell
            title={t('currentStep')}
            value={task?.current_step || '-'}
            mono
          />
          <StatusSummaryCell
            title={t('refreshStatus')}
            value={hasActiveTask ? t('pollingActive') : t('pollingStopped')}
          />
        </dl>
        {task ? (
          <div className='space-y-2 border-t-[0.5px] border-border/80 px-5 py-3.5'>
            <div className='flex items-center justify-between text-xs text-muted-foreground tabular-nums'>
              <span className='inline-flex items-center gap-1.5 font-medium text-foreground'>
                {t('executionProgress')}
                <HelpHint
                  label={t('executionProgress')}
                  content={t('executionProgressDescription')}
                />
              </span>
              <span className='font-mono font-medium text-foreground'>
                {t('completedPercent')}: {progress}%
              </span>
            </div>
            <Progress value={progress} />
          </div>
        ) : null}
      </div>

      {task?.status === 'succeeded' ? (
        <div data-testid='upgrade-execute-success'>
          <ArcAlert tone='success' title={t('upgradeCompletedTitle')}>
            <div className='mt-2 flex flex-wrap gap-2'>
              <Button
                variant='secondary'
                size='sm'
                onClick={handleStartExecution}
                loading={starting}
                disabled={!plan}
              >
                <RefreshCw className='size-3.5' aria-hidden />
                {t('restartCurrentPlan')}
              </Button>
              <Button
                variant='secondary'
                size='sm'
                onClick={() => router.push(`/clusters/${clusterId}`)}
              >
                <ArrowLeft className='size-3.5' aria-hidden />
                {t('backToClusterDetail')}
              </Button>
            </div>
          </ArcAlert>
        </div>
      ) : null}

      {canRestartExecution ? (
        <ArcAlert tone='warning' title={t('restartExecutionTitle')}>
          <div className='mt-2 flex flex-wrap items-center gap-2'>
            <Button
              size='sm'
              data-testid='upgrade-execute-restart'
              onClick={handleStartExecution}
              loading={starting}
              disabled={!plan}
            >
              <RefreshCw className='size-3.5' aria-hidden />
              {t('restartCurrentPlan')}
            </Button>
            <HelpHint
              label={t('restartCurrentPlan')}
              content={t('restartExecutionDescription')}
              data-testid='upgrade-execute-restart-hint'
            />
          </div>
        </ArcAlert>
      ) : null}

      {/* 尚未启动任务：展示单层扁平计划复核视图，避免空壳点「开始」 / Before start: flat plan review */}
      {!task ? (
        <>
          <section
            className='overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card px-6 py-5'
            data-testid='upgrade-plan-review'
          >
            <div className='flex flex-wrap items-center justify-between gap-2 pb-4'>
              <h2 className='text-sm font-semibold text-foreground'>
                {t('planReviewTitle')}
              </h2>
              <div className='flex flex-wrap items-center gap-3 text-xs text-muted-foreground tabular-nums'>
                <Badge tone='info' outline size='sm'>
                  {currentPlan.source_version} → {currentPlan.target_version}
                </Badge>
                <span>
                  {t('nodeTargetCount')}:{' '}
                  {currentPlan.snapshot.node_targets?.length || 0}
                </span>
                <span>
                  {t('planStepCount')}: {currentPlan.snapshot.steps?.length || 0}
                </span>
                <span>
                  {t('planConfigFileCount')}:{' '}
                  {currentPlan.snapshot.config_merge_plan?.files?.length || 0}
                </span>
                <span>
                  {t('unresolvedConflicts')}:{' '}
                  {currentPlan.snapshot.config_merge_plan?.conflict_count || 0}
                </span>
              </div>
            </div>

            <div className='grid gap-6 border-t-[0.5px] border-border/80 py-5 xl:grid-cols-2 xl:gap-0 xl:divide-x-[0.5px] xl:divide-border/80'>
              <PlanSummarySection
                icon={<Package className='size-4 text-primary' aria-hidden />}
                title={t('packageInfo')}
                description={
                  currentPlan.snapshot.package_manifest?.file_name || '-'
                }
                lines={[
                  `${t('checksum')}: ${currentPlan.snapshot.package_manifest?.checksum || '-'}`,
                  `${t('packageArch')}: ${currentPlan.snapshot.package_manifest?.arch || '-'}`,
                ]}
                className='xl:pr-6'
              />
              <PlanSummarySection
                icon={<PlugZap className='size-4 text-primary' aria-hidden />}
                title={t('connectorInfo')}
                description={`${currentPlan.snapshot.connector_manifest?.connectors?.length || 0} / ${currentPlan.snapshot.connector_manifest?.libraries?.length || 0} / ${currentPlan.snapshot.connector_manifest?.plugin_dependencies?.length || 0}`}
                lines={[
                  `${t('connectorCount')}: ${currentPlan.snapshot.connector_manifest?.connectors?.length || 0}`,
                  `${t('libraryCount')}: ${currentPlan.snapshot.connector_manifest?.libraries?.length || 0}`,
                  `${t('pluginDependencyCount')}: ${currentPlan.snapshot.connector_manifest?.plugin_dependencies?.length || 0}`,
                ]}
                className='xl:pl-6'
              />
            </div>

            <p className='border-t-[0.5px] border-border/80 pt-4 text-xs text-muted-foreground'>
              {t('planReviewReadyHint')}
            </p>
          </section>

          <div className='grid items-stretch gap-4 xl:grid-cols-2'>
            <section
              className={`flex flex-col overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card ${EXECUTION_PANES_HEIGHT_CLASS}`}
              data-testid='upgrade-plan-steps'
            >
              <div className='flex items-center gap-2 border-b-[0.5px] border-border/80 px-5 py-3.5'>
                <ListOrdered className='size-4 text-muted-foreground' aria-hidden />
                <h2 className='text-sm font-semibold text-foreground'>
                  {t('plannedSteps')}
                </h2>
              </div>
              <div className='min-h-0 flex-1 overflow-auto px-5'>
                <div className='divide-y-[0.5px] divide-border/70'>
                  {(currentPlan.snapshot.steps || []).map((step) => (
                    <div
                      key={`${step.sequence}-${step.code}`}
                      className='py-3.5'
                    >
                      <div className='flex flex-wrap items-center justify-between gap-2'>
                        <div className='min-w-0'>
                          <div className='text-sm font-medium text-foreground'>
                            {step.title || step.code}
                          </div>
                          <div className='font-mono text-xs text-muted-foreground tabular-nums'>
                            {step.sequence}. {step.code}
                          </div>
                        </div>
                        <Badge
                          tone={step.required ? 'info' : 'neutral'}
                          outline
                          size='sm'
                        >
                          {step.required
                            ? t('requiredStep')
                            : t('optionalStep')}
                        </Badge>
                      </div>
                      {step.description ? (
                        <div className='mt-1.5 text-xs leading-relaxed text-muted-foreground'>
                          {step.description}
                        </div>
                      ) : null}
                    </div>
                  ))}
                </div>
              </div>
            </section>

            <section
              className={`flex flex-col overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card ${EXECUTION_PANES_HEIGHT_CLASS}`}
              data-testid='upgrade-plan-nodes'
            >
              <div className='border-b-[0.5px] border-border/80 px-5 py-3.5'>
                <h2 className='flex items-center gap-2 text-sm font-semibold text-foreground'>
                  <Server className='size-4 text-muted-foreground' aria-hidden />
                  {t('nodeTargets')}
                </h2>
                <p className='mt-0.5 text-xs text-muted-foreground'>
                  {t('nodeTargetsDescription')}
                </p>
              </div>
              <div className='min-h-0 flex-1 overflow-auto'>
                <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
                  <TableHeader>
                    <TableRow className='hover:bg-transparent'>
                      <TableHead>{t('host')}</TableHead>
                      <TableHead>{t('role')}</TableHead>
                      <TableHead>{t('sourceVersion')}</TableHead>
                      <TableHead>{t('sourceInstallDir')}</TableHead>
                      <TableHead>{t('targetDir')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(currentPlan.snapshot.node_targets || []).map((target) => (
                      <TableRow key={target.cluster_node_id}>
                        <TableCell>
                          <div className='font-medium whitespace-nowrap'>
                            {target.host_name || target.host_ip || '-'}
                          </div>
                          <div className='font-mono text-xs text-muted-foreground tabular-nums'>
                            {target.host_ip || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='whitespace-nowrap'>
                          {target.role || '-'}
                        </TableCell>
                        <TableCell className='font-mono text-xs tabular-nums whitespace-nowrap'>
                          {target.source_version || currentPlan.source_version}
                        </TableCell>
                        <TableCell>
                          <div
                            className='max-w-[200px] truncate font-mono text-xs'
                            title={target.source_install_dir || '-'}
                          >
                            {target.source_install_dir || '-'}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div
                            className='max-w-[200px] truncate font-mono text-xs'
                            title={target.target_install_dir || '-'}
                          >
                            {target.target_install_dir || '-'}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </section>
          </div>
        </>
      ) : (
        <div className='grid items-stretch gap-4 xl:grid-cols-[minmax(0,1fr)_440px]'>
          {/* 左侧步骤 / 节点执行详情单层扁平画布 / Left steps/nodes flat single-layer panel */}
          <section
            className={`flex flex-col overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card ${EXECUTION_PANES_HEIGHT_CLASS}`}
          >
            <div className='flex flex-wrap items-center justify-between gap-3 border-b-[0.5px] border-border/80 px-5 py-3.5'>
              <div className='min-w-0'>
                <h2 className='text-sm font-semibold text-foreground'>
                  {t('executionDetailTitle')}
                </h2>
                <p className='text-xs text-muted-foreground'>
                  {t('executionDetailDescription')}
                </p>
              </div>
              <SegmentedControl
                label={t('executionDetailTitle')}
                value={executionView}
                onValueChange={(value) =>
                  setExecutionView(value as 'steps' | 'nodes')
                }
                options={[
                  {
                    value: 'steps',
                    label: t('stepTree'),
                    testId: 'upgrade-view-steps',
                  },
                  {
                    value: 'nodes',
                    label: t('nodeExecutions'),
                    testId: 'upgrade-view-nodes',
                  },
                ]}
              />
            </div>

            {executionView === 'steps' ? (
              <div className='min-h-0 flex-1 overflow-auto px-5'>
                <div className='divide-y-[0.5px] divide-border/70'>
                  {(stepsData?.steps || task?.steps || []).map((step) => (
                    <div key={step.id} className='py-3.5'>
                      <div className='flex flex-wrap items-center justify-between gap-2'>
                        <div className='min-w-0'>
                          <div className='text-sm font-medium text-foreground'>
                            {stepMap[step.code]?.title || step.code}
                          </div>
                          <div className='font-mono text-xs text-muted-foreground tabular-nums'>
                            {step.sequence}. {step.code}
                          </div>
                        </div>
                        <div className='flex items-center gap-2'>
                          <Badge
                            tone={upgradeStatusTone(step.status)}
                            outline
                            dot
                            size='sm'
                          >
                            {getExecutionStatusLabel(step.status)}
                          </Badge>
                          <Badge tone='neutral' outline size='sm'>
                            {t('stepNodeCount')}:{' '}
                            {getStepNodeCount(step, stepsData)}
                          </Badge>
                        </div>
                      </div>
                      <div className='mt-1.5 text-xs leading-relaxed text-muted-foreground'>
                        {step.message || stepMap[step.code]?.description}
                      </div>
                      {step.error ? (
                        <div className='mt-1.5 text-xs text-destructive'>
                          {step.error}
                        </div>
                      ) : null}
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <div className='min-h-0 flex-1 overflow-auto'>
                <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
                  <TableHeader>
                    <TableRow className='hover:bg-transparent'>
                      <TableHead>{t('host')}</TableHead>
                      <TableHead>{t('role')}</TableHead>
                      <TableHead>{t('statusLabel')}</TableHead>
                      <TableHead>{t('currentStep')}</TableHead>
                      <TableHead>{t('messageLabel')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {nodeOptions.map((node) => {
                      const messageText = node.message || node.error || '-';
                      return (
                        <TableRow key={node.id}>
                          <TableCell className='font-medium whitespace-nowrap'>
                            {node.host_name}
                          </TableCell>
                          <TableCell className='whitespace-nowrap'>
                            {node.role}
                          </TableCell>
                          <TableCell>
                            <Badge
                              tone={upgradeStatusTone(node.status)}
                              outline
                              dot
                              size='sm'
                            >
                              {getExecutionStatusLabel(node.status)}
                            </Badge>
                          </TableCell>
                          <TableCell className='font-mono text-xs whitespace-nowrap'>
                            {node.current_step || '-'}
                          </TableCell>
                          <TableCell>
                            {/* 截断 + Tooltip 完整展示 / Truncate with full Tooltip */}
                            <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
                              <TooltipTrigger asChild>
                                <div className='max-w-[340px] cursor-help truncate text-left'>
                                  {messageText}
                                </div>
                              </TooltipTrigger>
                              <TooltipContent
                                side='bottom'
                                className='max-w-md break-words'
                              >
                                <p className='whitespace-pre-wrap'>
                                  {messageText}
                                </p>
                              </TooltipContent>
                            </Tooltip>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            )}
          </section>

          {/* 右侧执行日志单层扁平画布：原生 overflow-auto + 0.5px 细线日志行 + SegmentedControl 日志级别 */}
          {/* Right execution logs flat panel: native overflow-auto, 0.5px hairline log rows, SegmentedControl log level */}
          <section
            className={`flex flex-col overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card ${EXECUTION_PANES_HEIGHT_CLASS}`}
          >
            <div className='space-y-3 border-b-[0.5px] border-border/80 px-5 py-3.5'>
              <div>
                <h2 className='text-sm font-semibold text-foreground'>
                  {t('logsTitle')}
                </h2>
                <p className='text-xs text-muted-foreground'>
                  {t('logsDescription')}
                </p>
              </div>

              <SegmentedControl
                label={t('allLevels')}
                value={logsQuery.level || 'all'}
                onValueChange={(value) =>
                  setLogsQuery((current) => ({
                    ...current,
                    page: 1,
                    level: value === 'all' ? undefined : (value as LogLevel),
                  }))
                }
                options={[
                  {value: 'all', label: t('allLevels')},
                  {value: 'INFO', label: 'INFO'},
                  {value: 'WARN', label: 'WARN'},
                  {value: 'ERROR', label: 'ERROR'},
                ]}
              />

              <div className='grid gap-2 sm:grid-cols-2'>
                <Select
                  aria-label={t('allSteps')}
                  value={logsQuery.step_code || 'all'}
                  onValueChange={(value) =>
                    setLogsQuery((current) => ({
                      ...current,
                      page: 1,
                      step_code:
                        value === 'all' ? undefined : (value as StepCode),
                    }))
                  }
                  placeholder={t('allSteps')}
                  options={[
                    {value: 'all', label: t('allSteps')},
                    ...(stepsData?.steps || task?.steps || []).map((step) => ({
                      value: step.code,
                      label: step.code,
                    })),
                  ]}
                />

                <Select
                  aria-label={t('allNodes')}
                  value={
                    logsQuery.node_execution_id
                      ? String(logsQuery.node_execution_id)
                      : 'all'
                  }
                  onValueChange={(value) =>
                    setLogsQuery((current) => ({
                      ...current,
                      page: 1,
                      node_execution_id:
                        value === 'all' ? undefined : Number(value),
                    }))
                  }
                  placeholder={t('allNodes')}
                  options={[
                    {value: 'all', label: t('allNodes')},
                    ...nodeOptions.map((node) => ({
                      value: String(node.id),
                      label: `${node.host_name} · ${node.role}`,
                    })),
                  ]}
                />
              </div>

              {canLoadEarlierLogs || logsTotalPages > 1 ? (
                <div className='flex items-center justify-between gap-2 pt-0.5'>
                  {canLoadEarlierLogs ? (
                    <Button
                      variant='secondary'
                      size='sm'
                      data-testid='upgrade-load-earlier-logs'
                      onClick={() =>
                        setLogsQuery((current) => ({
                          ...current,
                          page: Math.max(1, (current.page || 1) - 1),
                        }))
                      }
                    >
                      <History className='size-3.5' aria-hidden />
                      {t('loadEarlierLogs')}
                    </Button>
                  ) : (
                    <span className='text-xs text-muted-foreground'>
                      {logsTotalPages > 1 ? t('noEarlierLogs') : null}
                    </span>
                  )}
                  {logsTotalPages > 1 ? (
                    <span className='text-xs text-muted-foreground tabular-nums'>
                      {t('logsPageIndicator', {
                        page: currentLogsPage,
                        total: logsTotalPages,
                      })}
                    </span>
                  ) : null}
                </div>
              ) : null}
            </div>

            <div className='min-h-0 flex-1 overflow-auto px-5'>
              {logsData?.items.length ? (
                <div className='divide-y-[0.5px] divide-border/70'>
                  {logsData.items.map((log) => (
                    <div key={log.id} className='py-3'>
                      <div className='flex flex-wrap items-center gap-2 text-xs text-muted-foreground'>
                        <Badge
                          tone={
                            log.level === 'ERROR'
                              ? 'danger'
                              : log.level === 'WARN'
                                ? 'warning'
                                : 'neutral'
                          }
                          outline
                          size='sm'
                        >
                          {log.level}
                        </Badge>
                        <span className='font-mono'>{log.step_code}</span>
                        <span className='ml-auto inline-flex items-center gap-1 font-mono tabular-nums'>
                          <Clock3 className='size-3' aria-hidden />
                          {formatDateTimeTz(log.created_at)}
                        </span>
                      </div>
                      <div className='mt-1.5 text-xs leading-relaxed text-foreground'>
                        {log.message}
                      </div>
                      {log.command_summary ? (
                        <div className='mt-1.5 flex items-start gap-1.5 overflow-x-auto rounded-md bg-muted/40 px-2.5 py-1.5 font-mono text-[11px] text-muted-foreground'>
                          <Terminal className='mt-0.5 size-3 shrink-0' aria-hidden />
                          <span className='whitespace-pre-wrap break-words'>
                            {log.command_summary}
                          </span>
                        </div>
                      ) : null}
                    </div>
                  ))}
                  <div ref={logEndRef} />
                </div>
              ) : (
                <div className='py-10'>
                  <EmptyState
                    icon={<Terminal />}
                    title={t('noLogs')}
                    description={t('logsDescription')}
                  />
                </div>
              )}
            </div>

            {logsTotalPages > 1 ? (
              <div className='border-t-[0.5px] border-border/80 px-4 py-2.5'>
                <Pagination
                  currentPage={currentLogsPage}
                  totalPages={logsTotalPages}
                  pageSize={logsData?.page_size || logsQuery.page_size || 100}
                  totalItems={logsData?.total || 0}
                  onPageChange={(page) =>
                    setLogsQuery((current) => ({
                      ...current,
                      page,
                    }))
                  }
                  showPageSizeSelector={false}
                  showTotalItems={true}
                />
              </div>
            ) : null}
          </section>
        </div>
      )}
    </div>
  );
}

interface StatusSummaryCellProps {
  title: string;
  value: string;
  mono?: boolean;
}

/**
 * 扁平状态摘要单元格：嵌入单层状态栏，以 0.5px 竖线分隔。
 * Flat status summary cell inside the single-layer summary bar, separated by 0.5px hairlines.
 */
function StatusSummaryCell({title, value, mono}: StatusSummaryCellProps) {
  return (
    <div className='min-w-0 px-5 py-3.5'>
      <dt className='truncate text-xs text-muted-foreground'>{title}</dt>
      <dd
        className={`mt-1 truncate text-sm font-semibold text-foreground tabular-nums ${mono ? 'font-mono' : ''}`}
        title={value}
      >
        {value}
      </dd>
    </div>
  );
}

interface PlanSummarySectionProps {
  icon: ReactNode;
  title: string;
  description: string;
  lines: string[];
  className?: string;
}

/**
 * 计划资产摘要分区：安装包 / 插件只读速览（单层无嵌套框）。
 * Flat plan asset summary section for package / connector read-only preview.
 */
function PlanSummarySection({
  icon,
  title,
  description,
  lines,
  className,
}: PlanSummarySectionProps) {
  return (
    <div className={className}>
      <div className='flex items-center gap-2 text-xs font-semibold text-foreground'>
        {icon}
        {title}
      </div>
      <div
        className='mt-1.5 truncate font-mono text-sm font-medium text-foreground tabular-nums'
        title={description}
      >
        {description}
      </div>
      <div className='mt-2.5 space-y-1 text-xs text-muted-foreground tabular-nums'>
        {lines.map((line) => (
          <div key={line} className='truncate' title={line}>
            {line}
          </div>
        ))}
      </div>
    </div>
  );
}
