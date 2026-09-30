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

import Link from 'next/link';
import {useCallback, useEffect, useMemo, useRef, useState, type ReactNode} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import {
  AlertTriangle,
  ArrowLeft,
  Clock3,
  History,
  ListOrdered,
  Loader2,
  Package,
  PlayCircle,
  PlugZap,
  RefreshCw,
  Server,
  Terminal,
} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {WorkspaceHeader} from '@/components/common/layout';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Pagination} from '@/components/ui/pagination';
import {Progress} from '@/components/ui/progress';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Skeleton} from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Tabs, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {ScrollArea} from '@/components/ui/scroll-area';
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
  getStatusBadgeVariant,
  getStepNodeCount,
} from './utils';

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
  const [starting, setStarting] = useState(false);
  // 步骤 / 节点视图切换，释放横向空间 / Toggle steps vs nodes for horizontal space
  const [executionView, setExecutionView] = useState<'steps' | 'nodes'>(
    'steps',
  );
  const logEndRef = useRef<HTMLDivElement | null>(null);
  const initializedLogsTaskRef = useRef<number | null>(null);
  const previousLogsTotalPagesRef = useRef(1);

  const planIdFromQuery = Number(searchParams.get('planId') || 0);
  const taskIdFromQuery = Number(searchParams.get('taskId') || 0);

  const loadTaskState = useCallback(
    async (taskId: number, query: TaskLogsQuery) => {
      const [taskResult, stepsResult, logsResult] = await Promise.all([
        services.stUpgrade.getTask(taskId),
        services.stUpgrade.getTaskSteps(taskId),
        services.stUpgrade.getTaskLogs(taskId, query),
      ]);
      setPlan(taskResult.plan);
      setTask(taskResult);
      setStepsData(stepsResult);
      setLogsData(logsResult);
      patchStUpgradeSession(clusterId, {
        clusterId,
        plan: taskResult.plan,
        task: taskResult,
      });
    },
    [clusterId],
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
      patchStUpgradeSession(clusterId, {
        clusterId,
        request: session?.request,
        precheck: session?.precheck,
        plan: planResult,
        task: undefined,
      });
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('loadExecuteFailed'),
      );
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
      <div className='space-y-6' data-testid='upgrade-execute-page'>
        <WorkspaceHeader
          icon={<PlayCircle />}
          title={t('executeTitle')}
          subtitle={<span>{t('executeDescription')}</span>}
        />
        <UpgradeSteps current='execute' clusterId={clusterId} />
        <div className='space-y-6'>
          <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
            <Skeleton className='h-24 w-full' />
            <Skeleton className='h-24 w-full' />
            <Skeleton className='h-24 w-full' />
            <Skeleton className='h-24 w-full' />
          </div>
          <Card>
            <CardHeader>
              <Skeleton className='h-6 w-48' />
            </CardHeader>
            <CardContent>
              <Skeleton className='h-4 w-full' />
            </CardContent>
          </Card>
          <div className='grid gap-6 xl:grid-cols-2'>
            <Skeleton className='h-96 w-full' />
            <Skeleton className='h-96 w-full' />
          </div>
        </div>
      </div>
    );
  }

  if (!currentPlan) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('executeTitle')}</CardTitle>
          <CardDescription>{t('missingPlan')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            onClick={() =>
              router.push(`/clusters/${clusterId}/upgrade/prepare`)
            }
          >
            <ArrowLeft className='mr-2 h-4 w-4' />
            {t('startFromPrepare')}
          </Button>
        </CardContent>
      </Card>
    );
  }

  const currentLogsPage = logsQuery.page || 1;
  const canLoadEarlierLogs = currentLogsPage > 1;

  return (
    <div className='space-y-6' data-testid='upgrade-execute-page'>
      <div className='space-y-3'>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link href={`/clusters/${clusterId}`}>
                  {t('clusterDetailBreadcrumb')}
                </Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t('executeTitle')}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <WorkspaceHeader
          icon={<PlayCircle />}
          title={t('executeTitle')}
          subtitle={
            <>
              <span>{t('executeDescription')}</span>
              {currentPlan ? (
                <span className='mt-0.5 block'>
                  {currentPlan.source_version} → {currentPlan.target_version}
                </span>
              ) : null}
            </>
          }
          actions={
            <>
              <Button
                variant='outline'
                onClick={() =>
                  router.push(`/clusters/${clusterId}/upgrade/config`)
                }
              >
                <ArrowLeft className='mr-2 h-4 w-4' />
                {t('backToConfig')}
              </Button>
              <Button
                variant='outline'
                onClick={() => task?.id && void loadTaskState(task.id, logsQuery)}
                disabled={!task?.id}
              >
                <RefreshCw className='mr-2 h-4 w-4' />
                {commonT('refresh')}
              </Button>
              {showPrimaryExecutionButton ? (
                <Button
                  data-testid='upgrade-execute-start'
                  onClick={handleStartExecution}
                  disabled={starting || !plan || hasActiveTask}
                >
                  {starting ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <PlayCircle className='mr-2 h-4 w-4' />
                  )}
                  {startExecutionLabel}
                </Button>
              ) : null}
            </>
          }
        />
      </div>

      <UpgradeSteps current='execute' clusterId={clusterId} />

      <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
        <StatusCard
          title={t('taskStatus')}
          value={task ? getExecutionStatusLabel(task.status) : t('notStarted')}
        />
        <StatusCard
          title={t('rollbackStatus')}
          value={
            task
              ? getExecutionStatusLabel(task.rollback_status)
              : t('notStarted')
          }
        />
        <StatusCard
          title={t('currentStep')}
          value={task?.current_step || '-'}
        />
        <StatusCard
          title={t('refreshStatus')}
          value={hasActiveTask ? t('pollingActive') : t('pollingStopped')}
        />
      </div>

      {task?.status === 'succeeded' ? (
        <Card
          className='border-emerald-500/40 bg-emerald-500/5'
          data-testid='upgrade-execute-success'
        >
          <CardContent className='flex flex-col gap-4 p-4 lg:flex-row lg:items-center lg:justify-between'>
            <div className='space-y-1'>
              <div className='font-medium'>{t('upgradeCompletedTitle')}</div>
              <div className='text-sm text-muted-foreground'>
                {t('upgradeCompletedDescription')}
              </div>
            </div>
            <div className='flex flex-wrap gap-2'>
              <Button
                variant='outline'
                onClick={handleStartExecution}
                disabled={starting || !plan}
              >
                {starting ? (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                ) : (
                  <RefreshCw className='mr-2 h-4 w-4' />
                )}
                {t('restartCurrentPlan')}
              </Button>
              <Button
                variant='outline'
                onClick={() => router.push(`/clusters/${clusterId}`)}
              >
                <ArrowLeft className='mr-2 h-4 w-4' />
                {t('backToClusterDetail')}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {canRestartExecution ? (
        <Card className='border-amber-500/40 bg-amber-500/5'>
          <CardContent className='flex flex-col gap-4 p-4 lg:flex-row lg:items-center lg:justify-between'>
            <div className='flex items-start gap-3'>
              <AlertTriangle className='mt-0.5 h-5 w-5 shrink-0 text-amber-600' />
              <div className='space-y-1'>
                <div className='font-medium'>{t('restartExecutionTitle')}</div>
                <div className='text-sm text-muted-foreground'>
                  {t('restartExecutionDescription')}
                </div>
              </div>
            </div>
            <Button onClick={handleStartExecution} disabled={starting || !plan}>
              {starting ? (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <RefreshCw className='mr-2 h-4 w-4' />
              )}
              {t('restartCurrentPlan')}
            </Button>
          </CardContent>
        </Card>
      ) : null}

      {/* 尚未启动任务：展示计划内容供确认，避免空壳点「开始」 / Before start: show plan review instead of empty execution panes */}
      {!task ? (
        <>
          <Card data-testid='upgrade-plan-review'>
            <CardHeader>
              <CardTitle>{t('planReviewTitle')}</CardTitle>
              <CardDescription>{t('planReviewDescription')}</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex flex-wrap items-center gap-2 text-sm text-muted-foreground'>
                <Badge variant='default'>
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

              <div className='grid gap-4 xl:grid-cols-2'>
                <PlanSummaryCard
                  icon={<Package className='h-4 w-4 text-primary' />}
                  title={t('packageInfo')}
                  description={
                    currentPlan.snapshot.package_manifest?.file_name || '-'
                  }
                  lines={[
                    `${t('checksum')}: ${currentPlan.snapshot.package_manifest?.checksum || '-'}`,
                    `${t('packageArch')}: ${currentPlan.snapshot.package_manifest?.arch || '-'}`,
                  ]}
                />
                <PlanSummaryCard
                  icon={<PlugZap className='h-4 w-4 text-primary' />}
                  title={t('connectorInfo')}
                  description={`${currentPlan.snapshot.connector_manifest?.connectors?.length || 0} / ${currentPlan.snapshot.connector_manifest?.libraries?.length || 0} / ${currentPlan.snapshot.connector_manifest?.plugin_dependencies?.length || 0}`}
                  lines={[
                    `${t('connectorCount')}: ${currentPlan.snapshot.connector_manifest?.connectors?.length || 0}`,
                    `${t('libraryCount')}: ${currentPlan.snapshot.connector_manifest?.libraries?.length || 0}`,
                    `${t('pluginDependencyCount')}: ${currentPlan.snapshot.connector_manifest?.plugin_dependencies?.length || 0}`,
                  ]}
                />
              </div>

              <p className='text-sm text-muted-foreground'>
                {t('planReviewReadyHint')}
              </p>
            </CardContent>
          </Card>

          <div className='grid items-stretch gap-6 xl:grid-cols-2'>
            <Card
              className={`flex flex-col ${EXECUTION_PANES_HEIGHT_CLASS}`}
              data-testid='upgrade-plan-steps'
            >
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <ListOrdered className='h-4 w-4' />
                  {t('plannedSteps')}
                </CardTitle>
                <CardDescription>
                  {t('plannedStepsDescription')}
                </CardDescription>
              </CardHeader>
              <CardContent className='flex min-h-0 flex-1 flex-col'>
                <ScrollArea className='min-h-0 flex-1 pr-4'>
                  <div className='space-y-3'>
                    {(currentPlan.snapshot.steps || []).map((step) => (
                      <div
                        key={`${step.sequence}-${step.code}`}
                        className='rounded-lg border p-4'
                      >
                        <div className='flex flex-wrap items-center justify-between gap-3'>
                          <div>
                            <div className='font-medium'>
                              {step.title || step.code}
                            </div>
                            <div className='text-xs text-muted-foreground'>
                              {step.sequence}. {step.code}
                            </div>
                          </div>
                          <Badge variant={step.required ? 'default' : 'outline'}>
                            {step.required
                              ? t('requiredStep')
                              : t('optionalStep')}
                          </Badge>
                        </div>
                        {step.description ? (
                          <div className='mt-3 text-sm text-muted-foreground'>
                            {step.description}
                          </div>
                        ) : null}
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              </CardContent>
            </Card>

            <Card
              className={`flex flex-col ${EXECUTION_PANES_HEIGHT_CLASS}`}
              data-testid='upgrade-plan-nodes'
            >
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <Server className='h-4 w-4' />
                  {t('nodeTargets')}
                </CardTitle>
                <CardDescription>{t('nodeTargetsDescription')}</CardDescription>
              </CardHeader>
              <CardContent className='flex min-h-0 flex-1 flex-col'>
                <ScrollArea className='min-h-0 flex-1 pr-4'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('host')}</TableHead>
                        <TableHead>{t('role')}</TableHead>
                        <TableHead>{t('sourceVersion')}</TableHead>
                        <TableHead>{t('sourceInstallDir')}</TableHead>
                        <TableHead>{t('targetDir')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {(currentPlan.snapshot.node_targets || []).map(
                        (target) => (
                          <TableRow key={target.cluster_node_id}>
                            <TableCell>
                              <div className='font-medium'>
                                {target.host_name || target.host_ip || '-'}
                              </div>
                              <div className='text-xs text-muted-foreground'>
                                {target.host_ip || '-'}
                              </div>
                            </TableCell>
                            <TableCell>{target.role || '-'}</TableCell>
                            <TableCell>
                              {target.source_version ||
                                currentPlan.source_version}
                            </TableCell>
                            <TableCell>
                              <div className='max-w-[200px] truncate font-mono text-xs'>
                                {target.source_install_dir || '-'}
                              </div>
                            </TableCell>
                            <TableCell>
                              <div className='max-w-[200px] truncate font-mono text-xs'>
                                {target.target_install_dir || '-'}
                              </div>
                            </TableCell>
                          </TableRow>
                        ),
                      )}
                    </TableBody>
                  </Table>
                </ScrollArea>
              </CardContent>
            </Card>
          </div>
        </>
      ) : (
        <>
          <Card>
            <CardHeader>
              <CardTitle>{t('executionProgress')}</CardTitle>
              <CardDescription>
                {t('executionProgressDescription')}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-3'>
              <div className='flex items-center justify-between text-sm text-muted-foreground'>
                <span>{t('completedPercent')}</span>
                <span>{progress}%</span>
              </div>
              <Progress value={progress} />
            </CardContent>
          </Card>

          <div className='grid items-stretch gap-6 xl:grid-cols-[minmax(0,1fr)_420px]'>
            <Card className={`flex flex-col ${EXECUTION_PANES_HEIGHT_CLASS}`}>
              <CardHeader className='space-y-3'>
                <div>
                  <CardTitle>{t('executionDetailTitle')}</CardTitle>
                  <CardDescription>
                    {t('executionDetailDescription')}
                  </CardDescription>
                </div>
                {/* 步骤 / 节点 Tabs，释放横向空间 / Steps↔nodes tabs for more room */}
                <Tabs
                  value={executionView}
                  onValueChange={(value) =>
                    setExecutionView(value as 'steps' | 'nodes')
                  }
                >
                  <TabsList>
                    <TabsTrigger
                      value='steps'
                      data-testid='upgrade-view-steps'
                    >
                      {t('stepTree')}
                    </TabsTrigger>
                    <TabsTrigger
                      value='nodes'
                      data-testid='upgrade-view-nodes'
                    >
                      {t('nodeExecutions')}
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
              </CardHeader>
              <CardContent className='flex min-h-0 flex-1 flex-col'>
                {executionView === 'steps' ? (
                  <ScrollArea className='min-h-0 flex-1 pr-4'>
                    <div className='space-y-3'>
                      {(stepsData?.steps || task?.steps || []).map((step) => (
                        <div key={step.id} className='rounded-lg border p-4'>
                          <div className='flex flex-wrap items-center justify-between gap-3'>
                            <div>
                              <div className='font-medium'>
                                {stepMap[step.code]?.title || step.code}
                              </div>
                              <div className='text-xs text-muted-foreground'>
                                {step.sequence}. {step.code}
                              </div>
                            </div>
                            <div className='flex items-center gap-2'>
                              <Badge
                                variant={getStatusBadgeVariant(step.status)}
                              >
                                {getExecutionStatusLabel(step.status)}
                              </Badge>
                              <Badge variant='outline'>
                                {t('stepNodeCount')}:{' '}
                                {getStepNodeCount(step, stepsData)}
                              </Badge>
                            </div>
                          </div>
                          <div className='mt-3 text-sm text-muted-foreground'>
                            {step.message || stepMap[step.code]?.description}
                          </div>
                          {step.error ? (
                            <div className='mt-2 text-sm text-destructive'>
                              {step.error}
                            </div>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  </ScrollArea>
                ) : (
                  <ScrollArea className='min-h-0 flex-1 pr-4'>
                    <TooltipProvider delayDuration={300}>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>{t('host')}</TableHead>
                            <TableHead>{t('role')}</TableHead>
                            <TableHead>{t('statusLabel')}</TableHead>
                            <TableHead>{t('currentStep')}</TableHead>
                            <TableHead>{t('messageLabel')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {nodeOptions.map((node) => {
                            const messageText =
                              node.message || node.error || '-';
                            return (
                              <TableRow key={node.id}>
                                <TableCell>{node.host_name}</TableCell>
                                <TableCell>{node.role}</TableCell>
                                <TableCell>
                                  <Badge
                                    variant={getStatusBadgeVariant(
                                      node.status,
                                    )}
                                  >
                                    {getExecutionStatusLabel(node.status)}
                                  </Badge>
                                </TableCell>
                                <TableCell>
                                  {node.current_step || '-'}
                                </TableCell>
                                <TableCell>
                                  {/* 截断 + Tooltip 完整展示 / Truncate with full Tooltip */}
                                  <Tooltip>
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
                    </TooltipProvider>
                  </ScrollArea>
                )}
              </CardContent>
            </Card>

            <Card className={`flex flex-col ${EXECUTION_PANES_HEIGHT_CLASS}`}>
              <CardHeader>
                <CardTitle>{t('logsTitle')}</CardTitle>
                <CardDescription>{t('logsDescription')}</CardDescription>
              </CardHeader>
              <CardContent className='flex min-h-0 flex-1 flex-col space-y-4'>
                <div className='grid gap-3'>
                  <div className='grid gap-3 sm:grid-cols-3'>
                    <Select
                      value={logsQuery.step_code || 'all'}
                      onValueChange={(value) =>
                        setLogsQuery((current) => ({
                          ...current,
                          step_code:
                            value === 'all' ? undefined : (value as StepCode),
                        }))
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder={t('allSteps')} />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='all'>{t('allSteps')}</SelectItem>
                        {(stepsData?.steps || task?.steps || []).map((step) => (
                          <SelectItem key={step.id} value={step.code}>
                            {step.code}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>

                    <Select
                      value={logsQuery.level || 'all'}
                      onValueChange={(value) =>
                        setLogsQuery((current) => ({
                          ...current,
                          level:
                            value === 'all' ? undefined : (value as LogLevel),
                        }))
                      }
                    >
                  <SelectTrigger>
                    <SelectValue placeholder={t('allLevels')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>{t('allLevels')}</SelectItem>
                    <SelectItem value='INFO'>INFO</SelectItem>
                    <SelectItem value='WARN'>WARN</SelectItem>
                    <SelectItem value='ERROR'>ERROR</SelectItem>
                  </SelectContent>
                </Select>

                <Select
                  value={
                    logsQuery.node_execution_id
                      ? String(logsQuery.node_execution_id)
                      : 'all'
                  }
                  onValueChange={(value) =>
                    setLogsQuery((current) => ({
                      ...current,
                      node_execution_id:
                        value === 'all' ? undefined : Number(value),
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('allNodes')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>{t('allNodes')}</SelectItem>
                    {nodeOptions.map((node) => (
                      <SelectItem key={node.id} value={String(node.id)}>
                        {node.host_name} · {node.role}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* 显式加载更早日志，替代隐式分页感知 / Explicit earlier-logs control */}
            <div className='flex items-center justify-between gap-3'>
              {canLoadEarlierLogs ? (
                <Button
                  variant='outline'
                  size='sm'
                  data-testid='upgrade-load-earlier-logs'
                  onClick={() =>
                    setLogsQuery((current) => ({
                      ...current,
                      page: Math.max(1, (current.page || 1) - 1),
                    }))
                  }
                >
                  <History className='mr-2 h-3.5 w-3.5' />
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

            <ScrollArea className='min-h-0 flex-1 rounded-lg border'>
              <div className='space-y-3 p-4'>
                {logsData?.items.length ? (
                  logsData.items.map((log) => (
                    <div
                      key={log.id}
                      className='rounded-lg border bg-muted/20 p-3'
                    >
                      <div className='flex flex-wrap items-center gap-2 text-xs text-muted-foreground'>
                        <Badge
                          variant={
                            log.level === 'ERROR'
                              ? 'destructive'
                              : log.level === 'WARN'
                                ? 'outline'
                                : 'secondary'
                          }
                        >
                          {log.level}
                        </Badge>
                        <span>{log.step_code}</span>
                        <span className='inline-flex items-center gap-1'>
                          <Clock3 className='h-3 w-3' />
                          {new Date(log.created_at).toLocaleString()}
                        </span>
                      </div>
                      <div className='mt-2 text-sm'>{log.message}</div>
                      {log.command_summary ? (
                        <div className='mt-2 flex items-start gap-2 rounded-md bg-background p-2 text-xs font-mono text-muted-foreground'>
                          <Terminal className='mt-0.5 h-3.5 w-3.5 shrink-0' />
                          <span className='break-all'>
                            {log.command_summary}
                          </span>
                        </div>
                      ) : null}
                    </div>
                  ))
                ) : (
                  <div className='rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                    {t('noLogs')}
                  </div>
                )}
                <div ref={logEndRef} />
              </div>
            </ScrollArea>

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
          </CardContent>
        </Card>
      </div>
        </>
      )}
    </div>
  );
}

interface StatusCardProps {
  title: string;
  value: string;
}

/**
 * 状态摘要卡：仅展示文本值，避免 title/badge 重复
 * Status summary card: text value only, no duplicate badge
 */
function StatusCard({title, value}: StatusCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle className='text-base'>{value}</CardTitle>
      </CardHeader>
    </Card>
  );
}

interface PlanSummaryCardProps {
  icon: ReactNode;
  title: string;
  description: string;
  lines: string[];
}

/**
 * 计划资产摘要卡：安装包 / 插件等只读速览
 * Plan asset summary card for package / connector read-only preview
 */
function PlanSummaryCard({
  icon,
  title,
  description,
  lines,
}: PlanSummaryCardProps) {
  return (
    <div className='rounded-lg border p-4'>
      <div className='flex items-start gap-3'>
        <div className='mt-0.5'>{icon}</div>
        <div className='min-w-0 space-y-1'>
          <div className='font-medium'>{title}</div>
          <div className='truncate text-sm text-muted-foreground'>
            {description}
          </div>
          <div className='space-y-0.5 pt-1 text-xs text-muted-foreground'>
            {lines.map((line) => (
              <div key={line} className='truncate'>
                {line}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
