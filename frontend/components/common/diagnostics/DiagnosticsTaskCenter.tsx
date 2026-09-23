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

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  Download,
  ExternalLink,
  Loader2,
  PlayCircle,
  RefreshCw,
  Workflow,
} from 'lucide-react';
import {toast} from 'sonner';
import services from '@/lib/services';
import type {
  CreateDiagnosticsTaskRequest,
  DiagnosticsLogLevel,
  DiagnosticsTask,
  DiagnosticsTaskEvent,
  DiagnosticsTaskEventType,
  DiagnosticsTaskLog,
  DiagnosticsTaskOptions,
  DiagnosticsTaskSourceRef,
  DiagnosticsTaskSourceType,
  DiagnosticsTaskStatus,
  DiagnosticsTaskStepCode,
  DiagnosticsTaskSummary,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Pagination} from '@/components/ui/pagination';
import {Progress} from '@/components/ui/progress';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Skeleton} from '@/components/ui/skeleton';
import {Switch} from '@/components/ui/switch';
import {localizeDiagnosticsText} from './text-utils';

const DEFAULT_TASK_OPTIONS: DiagnosticsTaskOptions = {
  include_thread_dump: false,
  include_jvm_dump: false,
  jvm_dump_min_free_mb: 2048,
};

const ACTIVE_TASK_STATUS: DiagnosticsTaskStatus[] = ['running'];
const TASK_PAGE_SIZE = 10;
const LOG_PAGE_SIZE = 200;
const STREAM_EVENT_TYPES: DiagnosticsTaskEventType[] = [
  'snapshot',
  'task_updated',
  'step_updated',
  'node_updated',
  'log_appended',
];

type DiagnosticsTaskCenterProps = {
  clusterId?: number;
  clusterName?: string;
  groupId?: number;
  reportId?: number;
  findingId?: number;
  alertId?: string;
  taskId?: number;
  source?: string;
  onSelectTask?: (taskId: number | null) => void;
};

type CreateContext = {
  triggerSource: DiagnosticsTaskSourceType;
  sourceRef: DiagnosticsTaskSourceRef;
  summaryKey: string;
};

type CreateContextMap = Record<DiagnosticsTaskSourceType, CreateContext | null>;

type LogFilters = {
  stepCode: 'all' | DiagnosticsTaskStepCode;
  nodeExecutionId: 'all' | number;
  level: 'all' | DiagnosticsLogLevel;
};

function formatDateTime(value?: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

function getStatusVariant(
  status: DiagnosticsTaskStatus,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (status) {
    case 'succeeded':
      return 'default';
    case 'running':
    case 'ready':
      return 'secondary';
    case 'failed':
      return 'destructive';
    case 'pending':
    case 'skipped':
    case 'cancelled':
    default:
      return 'outline';
  }
}

function getLogLevelVariant(
  level: DiagnosticsLogLevel,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (level) {
    case 'ERROR':
      return 'destructive';
    case 'WARN':
      return 'secondary';
    case 'INFO':
    default:
      return 'outline';
  }
}

function isTaskActive(status?: DiagnosticsTaskStatus): boolean {
  return !!status && ACTIVE_TASK_STATUS.includes(status);
}

function calculateProgress(task: DiagnosticsTask | null): number {
  if (!task || !Array.isArray(task.steps) || task.steps.length === 0) {
    return 0;
  }
  if (['succeeded', 'failed', 'cancelled'].includes(task.status)) {
    return 100;
  }
  const completed = task.steps.filter((step) =>
    ['succeeded', 'skipped'].includes(step.status),
  ).length;
  return Math.round((completed / task.steps.length) * 100);
}

function toTaskSummary(task: DiagnosticsTask): DiagnosticsTaskSummary {
  return {
    id: task.id,
    cluster_id: task.cluster_id,
    trigger_source: task.trigger_source,
    status: task.status,
    current_step: task.current_step,
    failure_step: task.failure_step,
    failure_reason: task.failure_reason,
    summary: task.summary,
    started_at: task.started_at,
    completed_at: task.completed_at,
    created_by: task.created_by,
    created_by_name: task.created_by_name,
    created_at: task.created_at,
    updated_at: task.updated_at,
  };
}

function patchTaskWithEvent(
  previous: DiagnosticsTask | null,
  event: DiagnosticsTaskEvent,
): DiagnosticsTask | null {
  if (!previous || previous.id !== event.task_id) {
    return previous;
  }

  if (event.event_type === 'snapshot' || event.event_type === 'task_updated') {
    return {
      ...previous,
      status: event.task_status || previous.status,
      current_step: event.current_step || previous.current_step,
      summary: event.message ?? previous.summary,
      failure_reason: event.failure_reason ?? previous.failure_reason,
    };
  }

  if (event.event_type === 'step_updated' && event.step_id) {
    return {
      ...previous,
      steps: previous.steps.map((step) =>
        step.id === event.step_id
          ? {
              ...step,
              status: event.step_status || step.status,
              message: event.message ?? step.message,
              error: event.error ?? step.error,
            }
          : step,
      ),
    };
  }

  if (event.event_type === 'node_updated' && event.node_execution_id) {
    return {
      ...previous,
      node_executions: previous.node_executions.map((node) =>
        node.id === event.node_execution_id
          ? {
              ...node,
              task_step_id:
                event.step_id !== undefined ? event.step_id : node.task_step_id,
              status: event.node_status || node.status,
              current_step: event.current_step || node.current_step,
              host_id: event.host_id || node.host_id,
              host_name: event.host_name || node.host_name,
              message: event.message ?? node.message,
              error: event.error ?? node.error,
            }
          : node,
      ),
    };
  }

  return previous;
}

function matchesLogFilters(
  event: DiagnosticsTaskEvent,
  filters: LogFilters,
): boolean {
  if (filters.stepCode !== 'all' && event.step_code !== filters.stepCode) {
    return false;
  }
  if (
    filters.nodeExecutionId !== 'all' &&
    event.node_execution_id !== filters.nodeExecutionId
  ) {
    return false;
  }
  if (filters.level !== 'all' && event.level !== filters.level) {
    return false;
  }
  return true;
}

function buildCreateContext(input: {
  groupId?: number;
  reportId?: number;
  findingId?: number;
  alertId?: string;
}): CreateContext {
  if (input.findingId && input.findingId > 0) {
    return {
      triggerSource: 'inspection_finding',
      sourceRef: {
        inspection_finding_id: input.findingId,
        inspection_report_id: input.reportId,
      },
      summaryKey: 'inspection_finding',
    };
  }
  if (input.groupId && input.groupId > 0) {
    return {
      triggerSource: 'error_group',
      sourceRef: {error_group_id: input.groupId},
      summaryKey: 'error_group',
    };
  }
  if (input.alertId) {
    return {
      triggerSource: 'alert',
      sourceRef: {alert_id: input.alertId},
      summaryKey: 'alert',
    };
  }
  return {
    triggerSource: 'manual',
    sourceRef: {},
    summaryKey: 'manual',
  };
}

function buildCreateContextMap(input: {
  groupId?: number;
  reportId?: number;
  findingId?: number;
  alertId?: string;
}): CreateContextMap {
  return {
    manual: {
      triggerSource: 'manual',
      sourceRef: {},
      summaryKey: 'manual',
    },
    error_group:
      input.groupId && input.groupId > 0
        ? {
            triggerSource: 'error_group',
            sourceRef: {error_group_id: input.groupId},
            summaryKey: 'error_group',
          }
        : null,
    inspection_finding:
      input.findingId && input.findingId > 0
        ? {
            triggerSource: 'inspection_finding',
            sourceRef: {
              inspection_finding_id: input.findingId,
              inspection_report_id: input.reportId,
            },
            summaryKey: 'inspection_finding',
          }
        : null,
    alert: input.alertId
      ? {
          triggerSource: 'alert',
          sourceRef: {alert_id: input.alertId},
          summaryKey: 'alert',
        }
      : null,
  };
}

function resolveInitialCreateSource(
  contexts: CreateContextMap,
): DiagnosticsTaskSourceType {
  if (contexts.inspection_finding) {
    return 'inspection_finding';
  }
  if (contexts.error_group) {
    return 'error_group';
  }
  if (contexts.alert) {
    return 'alert';
  }
  return 'manual';
}

function hasPrefilledCreateSource(contexts: CreateContextMap): boolean {
  return !!(contexts.error_group || contexts.inspection_finding || contexts.alert);
}

export function DiagnosticsTaskCenter({
  clusterId,
  clusterName,
  groupId,
  reportId,
  findingId,
  alertId,
  taskId,
  source,
  onSelectTask,
}: DiagnosticsTaskCenterProps) {
  const t = useTranslations('diagnosticsCenter');
  const commonT = useTranslations('common');
  const syntheticLogIDRef = useRef(-1);
  const logEndRef = useRef<HTMLDivElement | null>(null);

  const [statusFilter, setStatusFilter] = useState<
    'all' | DiagnosticsTaskStatus
  >('all');
  const [page, setPage] = useState(1);
  const [loadingTasks, setLoadingTasks] = useState(true);
  const [tasks, setTasks] = useState<DiagnosticsTaskSummary[]>([]);
  const [taskTotal, setTaskTotal] = useState(0);
  const [selectedTaskId, setSelectedTaskId] = useState<number | null>(
    taskId ?? null,
  );
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [taskDetail, setTaskDetail] = useState<DiagnosticsTask | null>(null);
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [logs, setLogs] = useState<DiagnosticsTaskLog[]>([]);
  const [logTotal, setLogTotal] = useState(0);
  const [createOptions, setCreateOptions] =
    useState<DiagnosticsTaskOptions>(DEFAULT_TASK_OPTIONS);
  const [autoStart, setAutoStart] = useState(true);
  const [creatingTask, setCreatingTask] = useState(false);
  const [startingTask, setStartingTask] = useState(false);
  const [logFilters, setLogFilters] = useState<LogFilters>({
    stepCode: 'all',
    nodeExecutionId: 'all',
    level: 'all',
  });

  const contextMap = useMemo(
    () => buildCreateContextMap({groupId, reportId, findingId, alertId}),
    [alertId, findingId, groupId, reportId],
  );
  const initialCreateSource = useMemo(
    () => resolveInitialCreateSource(contextMap),
    [contextMap],
  );
  const [selectedCreateSource, setSelectedCreateSource] =
    useState<DiagnosticsTaskSourceType>(initialCreateSource);

  useEffect(() => {
    setSelectedCreateSource(initialCreateSource);
  }, [initialCreateSource]);

  const createContext = contextMap[selectedCreateSource] || buildCreateContext({});
  const availableCreateSources = useMemo(
    () =>
      (
        ['manual', 'error_group', 'inspection_finding', 'alert'] as const
      ).filter((item) => item === 'manual' || !!contextMap[item]),
    [contextMap],
  );
  const hasSourcePrefill = useMemo(
    () => hasPrefilledCreateSource(contextMap),
    [contextMap],
  );
  const onlyManualCreateMode = availableCreateSources.length === 1;

  const totalPages = Math.max(1, Math.ceil(taskTotal / TASK_PAGE_SIZE));
  const progress = useMemo(() => calculateProgress(taskDetail), [taskDetail]);
  const selectedClusterLabel =
    clusterName || (clusterId ? `#${clusterId}` : '');
  const createRequiresCluster =
    createContext.triggerSource === 'manual' ||
    createContext.triggerSource === 'alert';

  const logQuery = useMemo(() => {
    return {
      page: 1,
      page_size: LOG_PAGE_SIZE,
      step_code:
        logFilters.stepCode !== 'all' ? logFilters.stepCode : undefined,
      node_execution_id:
        logFilters.nodeExecutionId !== 'all'
          ? logFilters.nodeExecutionId
          : undefined,
      level: logFilters.level !== 'all' ? logFilters.level : undefined,
    };
  }, [logFilters]);

  const loadTasks = useCallback(async () => {
    setLoadingTasks(true);
    try {
      const result = await services.diagnostics.listTasksSafe({
        cluster_id: clusterId,
        status: statusFilter !== 'all' ? statusFilter : undefined,
        page,
        page_size: TASK_PAGE_SIZE,
      });
      if (!result.success || !result.data) {
        toast.error(result.error || t('tasks.loadTasksError'));
        setTasks([]);
        setTaskTotal(0);
        return;
      }
      setTasks(result.data.items || []);
      setTaskTotal(result.data.total || 0);
    } finally {
      setLoadingTasks(false);
    }
  }, [clusterId, page, statusFilter, t]);

  const loadTaskLogs = useCallback(
    async (nextTaskId: number) => {
      setLoadingLogs(true);
      try {
        const result = await services.diagnostics.getTaskLogsSafe(
          nextTaskId,
          logQuery,
        );
        if (!result.success || !result.data) {
          toast.error(result.error || t('tasks.loadLogsError'));
          setLogs([]);
          setLogTotal(0);
          return;
        }
        setLogs(result.data.items || []);
        setLogTotal(result.data.total || 0);
      } finally {
        setLoadingLogs(false);
      }
    },
    [logQuery, t],
  );

  const loadTaskDetail = useCallback(
    async (nextTaskId: number) => {
      setLoadingDetail(true);
      try {
        const result = await services.diagnostics.getTaskSafe(nextTaskId);
        if (!result.success || !result.data) {
          toast.error(result.error || t('tasks.loadDetailError'));
          setTaskDetail(null);
          return;
        }
        const nextTask = result.data;
        setTaskDetail(nextTask);
        setTasks((current) => {
          const summary = toTaskSummary(nextTask);
          if (current.some((item) => item.id === summary.id)) {
            return current.map((item) =>
              item.id === summary.id ? summary : item,
            );
          }
          return current;
        });
      } finally {
        setLoadingDetail(false);
      }
    },
    [t],
  );

  const reloadSelectedTaskState = useCallback(async () => {
    if (!selectedTaskId) {
      return;
    }
    await Promise.all([
      loadTaskDetail(selectedTaskId),
      loadTaskLogs(selectedTaskId),
    ]);
  }, [loadTaskDetail, loadTaskLogs, selectedTaskId]);

  useEffect(() => {
    void loadTasks();
  }, [loadTasks]);

  useEffect(() => {
    setSelectedTaskId(taskId ?? null);
  }, [taskId]);

  useEffect(() => {
    if (tasks.length === 0) {
      if (!taskId) {
        setTaskDetail(null);
        setLogs([]);
        setSelectedTaskId(null);
      }
      return;
    }
    if (selectedTaskId) {
      return;
    }
    setSelectedTaskId(tasks[0].id);
    onSelectTask?.(tasks[0].id);
  }, [onSelectTask, selectedTaskId, taskId, tasks]);

  useEffect(() => {
    setLogFilters({
      stepCode: 'all',
      nodeExecutionId: 'all',
      level: 'all',
    });
    syntheticLogIDRef.current = -1;
  }, [selectedTaskId]);

  useEffect(() => {
    if (!selectedTaskId) {
      setTaskDetail(null);
      setLogs([]);
      setLogTotal(0);
      return;
    }
    void reloadSelectedTaskState();
  }, [reloadSelectedTaskState, selectedTaskId]);

  useEffect(() => {
    if (!taskDetail?.id || !isTaskActive(taskDetail.status)) {
      return;
    }
    const timer = setInterval(() => {
      void reloadSelectedTaskState();
      void loadTasks();
    }, 5000);
    return () => clearInterval(timer);
  }, [loadTasks, reloadSelectedTaskState, taskDetail?.id, taskDetail?.status]);

  useEffect(() => {
    if (!taskDetail?.id || !isTaskActive(taskDetail.status)) {
      return;
    }

    const eventSource = new EventSource(
      services.diagnostics.getTaskEventsUrl(taskDetail.id),
    );

    const handleEvent = (messageEvent: MessageEvent<string>) => {
      try {
        const payload = JSON.parse(messageEvent.data) as DiagnosticsTaskEvent;
        setTaskDetail((current) => patchTaskWithEvent(current, payload));
        if (
          (payload.event_type === 'snapshot' ||
            payload.event_type === 'task_updated') &&
          selectedTaskId === payload.task_id
        ) {
          setTasks((current) =>
            current.map((item) =>
              item.id === payload.task_id
                ? {
                    ...item,
                    status: payload.task_status || item.status,
                    current_step: payload.current_step || item.current_step,
                    summary: payload.message ?? item.summary,
                    failure_reason:
                      payload.failure_reason ?? item.failure_reason,
                  }
                : item,
            ),
          );
        }
        if (
          payload.event_type === 'log_appended' &&
          matchesLogFilters(payload, logFilters)
        ) {
          const syntheticID = syntheticLogIDRef.current;
          syntheticLogIDRef.current -= 1;
          setLogs((current) => {
            const nextItems = [
              ...current,
              {
                id: syntheticID,
                task_id: payload.task_id,
                task_step_id: payload.step_id,
                node_execution_id: payload.node_execution_id,
                step_code: payload.step_code || 'COLLECT_ERROR_CONTEXT',
                level: payload.level || 'INFO',
                event_type: payload.log_event_type || 'progress',
                message: payload.message || '',
                command_summary: payload.command_summary || '',
                created_at: payload.timestamp,
              },
            ];
            return nextItems.slice(-LOG_PAGE_SIZE);
          });
          setLogTotal((current) => current + 1);
        }
      } catch {
        return;
      }
    };

    eventSource.onerror = () => eventSource.close();

    STREAM_EVENT_TYPES.forEach((eventType) => {
      eventSource.addEventListener(eventType, handleEvent as EventListener);
    });

    return () => {
      STREAM_EVENT_TYPES.forEach((eventType) => {
        eventSource.removeEventListener(
          eventType,
          handleEvent as EventListener,
        );
      });
      eventSource.close();
    };
  }, [logFilters, selectedTaskId, taskDetail?.id, taskDetail?.status]);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({block: 'end'});
  }, [logs]);

  const handleCreateTask = useCallback(async () => {
    if (createRequiresCluster && !clusterId) {
      toast.error(t('tasks.chooseClusterFirst'));
      return;
    }

    const payload: CreateDiagnosticsTaskRequest = {
      cluster_id: clusterId,
      trigger_source: createContext.triggerSource,
      source_ref: createContext.sourceRef,
      options: createOptions,
      auto_start: autoStart,
    };

    setCreatingTask(true);
    try {
      const result = await services.diagnostics.createTaskSafe(payload);
      if (!result.success || !result.data) {
        toast.error(result.error || t('tasks.createError'));
        return;
      }
      toast.success(t('tasks.createSuccess'));
      setStatusFilter('all');
      setPage(1);
      setSelectedTaskId(result.data.id);
      onSelectTask?.(result.data.id);
      setTaskDetail(result.data);
      await loadTaskLogs(result.data.id);
      await loadTasks();
    } finally {
      setCreatingTask(false);
    }
  }, [
    autoStart,
    clusterId,
    createContext.sourceRef,
    createContext.triggerSource,
    createOptions,
    createRequiresCluster,
    loadTaskLogs,
    loadTasks,
    onSelectTask,
    t,
  ]);

  const handleStartTask = useCallback(async () => {
    if (!selectedTaskId) {
      return;
    }
    setStartingTask(true);
    try {
      const result = await services.diagnostics.startTaskSafe(selectedTaskId);
      if (!result.success || !result.data) {
        toast.error(result.error || t('tasks.startError'));
        return;
      }
      toast.success(t('tasks.startSuccess'));
      setTaskDetail(result.data);
      setTasks((current) =>
        current.map((item) =>
          item.id === result.data?.id ? toTaskSummary(result.data) : item,
        ),
      );
    } finally {
      setStartingTask(false);
    }
  }, [selectedTaskId, t]);

  const sourceDetailItems = useMemo(() => {
    const items: string[] = [];
    if (createContext.sourceRef.error_group_id) {
      items.push(
        t('tasks.sourceRefs.errorGroup', {
          id: createContext.sourceRef.error_group_id,
        }),
      );
    }
    if (createContext.sourceRef.inspection_report_id) {
      items.push(
        t('tasks.sourceRefs.report', {
          id: createContext.sourceRef.inspection_report_id,
        }),
      );
    }
    if (createContext.sourceRef.inspection_finding_id) {
      items.push(
        t('tasks.sourceRefs.finding', {
          id: createContext.sourceRef.inspection_finding_id,
        }),
      );
    }
    if (createContext.sourceRef.alert_id) {
      items.push(
        t('tasks.sourceRefs.alert', {id: createContext.sourceRef.alert_id}),
      );
    }
    return items;
  }, [
    createContext.sourceRef.alert_id,
    createContext.sourceRef.error_group_id,
    createContext.sourceRef.inspection_finding_id,
    createContext.sourceRef.inspection_report_id,
    t,
  ]);
  const entrySourceLabel = useMemo(() => {
    switch (source) {
      case 'alerts':
        return t('tasks.entrySource.alerts');
      case 'inspection-finding':
        return t('tasks.entrySource.inspectionFinding');
      case 'cluster-detail':
        return t('tasks.entrySource.clusterDetail');
      case 'cluster-detail-summary':
        return t('tasks.entrySource.clusterDetailSummary');
      default:
        return source || '';
    }
  }, [source, t]);

  const startButtonLabel =
    taskDetail?.status === 'failed' || taskDetail?.status === 'cancelled'
      ? t('tasks.retryTask')
      : t('tasks.startTask');

  const selectedStepTitle = useMemo(() => {
    if (!taskDetail?.current_step) {
      return '-';
    }
    return (
      taskDetail.steps.find((step) => step.code === taskDetail.current_step)
        ?.title || taskDetail.current_step
    );
  }, [taskDetail]);

  const renderTaskDetail = () => {
    if (loadingDetail && !taskDetail) {
      return (
        <Card className='flex flex-col overflow-hidden xl:col-start-2 xl:row-start-1 xl:self-start'>
          <CardContent className='space-y-4 py-6'>
            <Skeleton className='h-8 w-56' />
            <Skeleton className='h-28 w-full' />
            <Skeleton className='h-64 w-full' />
            <Skeleton className='h-56 w-full' />
          </CardContent>
        </Card>
      );
    }

    if (!taskDetail) {
      return (
        <Card className='flex flex-col overflow-hidden xl:col-start-2 xl:row-start-1 xl:self-start'>
          <CardContent className='flex flex-1 items-center justify-center py-10 text-sm text-muted-foreground'>
            {t('tasks.selectTask')}
          </CardContent>
        </Card>
      );
    }

    return (
      <Card className='flex flex-col overflow-hidden xl:col-start-2 xl:row-start-1 xl:self-start'>
        <CardHeader className='shrink-0 space-y-4 border-b xl:h-[332px]'>
          <div className='flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between'>
            <div className='space-y-3'>
              <div className='flex flex-wrap items-center gap-2'>
                <CardTitle>{t('tasks.detailTitle')}</CardTitle>
                <Badge variant={getStatusVariant(taskDetail.status)}>
                  {t(`tasks.status.${taskDetail.status}`)}
                </Badge>
                <Badge variant='outline'>
                  {t(`tasks.source.${taskDetail.trigger_source}`)}
                </Badge>
                <Badge variant='outline'>#{taskDetail.id}</Badge>
              </div>
              <div
                className='max-w-[72ch] line-clamp-2 text-sm leading-6 text-muted-foreground'
                title={
                  localizeDiagnosticsText(taskDetail.summary) ||
                  t('tasks.summaryFallback')
                }
              >
                {localizeDiagnosticsText(taskDetail.summary) ||
                  t('tasks.summaryFallback')}
              </div>
            </div>

            <div className='flex flex-wrap items-center gap-2'>
              <Button
                variant='outline'
                onClick={() => void reloadSelectedTaskState()}
              >
                <RefreshCw className='mr-2 h-4 w-4' />
                {commonT('refresh')}
              </Button>
              {taskDetail.index_path ? (
                <>
                  <Button asChild variant='outline'>
                    <a
                      href={services.diagnostics.getTaskHTMLUrl(taskDetail.id)}
                      target='_blank'
                      rel='noreferrer'
                    >
                      <ExternalLink className='mr-2 h-4 w-4' />
                      {t('tasks.previewHTML')}
                    </a>
                  </Button>
                  <Button asChild variant='outline'>
                    <a
                      href={services.diagnostics.getTaskHTMLUrl(
                        taskDetail.id,
                        true,
                      )}
                    >
                      <Download className='mr-2 h-4 w-4' />
                      {t('tasks.downloadHTML')}
                    </a>
                  </Button>
                </>
              ) : null}
              {taskDetail.bundle_dir ? (
                <Button asChild variant='outline'>
                  <a href={services.diagnostics.getTaskBundleUrl(taskDetail.id)}>
                    <Download className='mr-2 h-4 w-4' />
                    {t('tasks.downloadBundle')}
                  </a>
                </Button>
              ) : null}
              {taskDetail.status !== 'running' &&
              taskDetail.status !== 'succeeded' ? (
                <Button
                  onClick={() => void handleStartTask()}
                  disabled={startingTask}
                >
                  {startingTask ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <PlayCircle className='mr-2 h-4 w-4' />
                  )}
                  {startButtonLabel}
                </Button>
              ) : null}
            </div>
          </div>

          <div className='space-y-2'>
            <div className='flex items-center justify-between text-sm'>
              <span>{t('tasks.progress')}</span>
              <span>{progress}%</span>
            </div>
            <Progress value={progress} className='h-2' />
          </div>

          <div className='grid auto-rows-fr gap-3 text-sm md:grid-cols-2 xl:grid-cols-4'>
            <div className='grid min-h-[96px] grid-rows-[20px_minmax(0,1fr)] overflow-hidden rounded-lg border p-3'>
              <div className='text-muted-foreground'>{t('tasks.currentStep')}</div>
              <div className='mt-1 truncate font-medium' title={selectedStepTitle}>
                {selectedStepTitle}
              </div>
            </div>
            <div className='grid min-h-[96px] grid-rows-[20px_minmax(0,1fr)] overflow-hidden rounded-lg border p-3'>
              <div className='text-muted-foreground'>{t('tasks.createdBy')}</div>
              <div
                className='mt-1 truncate font-medium'
                title={String(
                  taskDetail.created_by_name || taskDetail.created_by || '-',
                )}
              >
                {taskDetail.created_by_name || taskDetail.created_by || '-'}
              </div>
            </div>
            <div className='grid min-h-[96px] grid-rows-[20px_minmax(0,1fr)] overflow-hidden rounded-lg border p-3'>
              <div className='text-muted-foreground'>{t('tasks.startedAt')}</div>
              <div
                className='mt-1 truncate font-medium'
                title={formatDateTime(taskDetail.started_at)}
              >
                {formatDateTime(taskDetail.started_at)}
              </div>
            </div>
            <div className='grid min-h-[96px] grid-rows-[20px_minmax(0,1fr)] overflow-hidden rounded-lg border p-3'>
              <div className='text-muted-foreground'>{t('tasks.completedAt')}</div>
              <div
                className='mt-1 truncate font-medium'
                title={formatDateTime(taskDetail.completed_at)}
              >
                {formatDateTime(taskDetail.completed_at)}
              </div>
            </div>
            <div className='grid min-h-[96px] grid-rows-[20px_minmax(0,1fr)] overflow-hidden rounded-lg border p-3 md:col-span-2'>
              <div className='text-muted-foreground'>{t('tasks.bundleDir')}</div>
              <code
                className='mt-1 block truncate text-xs'
                title={taskDetail.bundle_dir || '-'}
              >
                {taskDetail.bundle_dir || '-'}
              </code>
            </div>
            <div className='grid min-h-[96px] grid-rows-[20px_minmax(0,1fr)] overflow-hidden rounded-lg border p-3'>
              <div className='text-muted-foreground'>{t('tasks.manifestPath')}</div>
              <code
                className='mt-1 block truncate text-xs'
                title={taskDetail.manifest_path || '-'}
              >
                {taskDetail.manifest_path || '-'}
              </code>
            </div>
            <div className='grid min-h-[96px] grid-rows-[20px_minmax(0,1fr)] overflow-hidden rounded-lg border p-3'>
              <div className='text-muted-foreground'>{t('tasks.indexPath')}</div>
              <code
                className='mt-1 block truncate text-xs'
                title={taskDetail.index_path || '-'}
              >
                {taskDetail.index_path || '-'}
              </code>
            </div>
          </div>
        </CardHeader>

        <CardContent className='flex min-h-0 flex-1 flex-col gap-4 p-6 xl:grid xl:grid-cols-2 xl:grid-rows-[minmax(0,1fr)_minmax(0,1fr)] xl:gap-4'>
          <section className='flex min-h-[380px] flex-col overflow-hidden rounded-lg border xl:min-h-0 xl:h-full'>
              <div className='border-b px-4 py-3'>
                <div className='font-medium'>{t('tasks.stepsTitle')}</div>
              </div>
              <div className='flex min-h-0 flex-1 flex-col p-4'>
                {taskDetail.steps.length === 0 ? (
                  <div className='flex flex-1 items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                    {t('tasks.noSteps')}
                  </div>
                ) : (
                  <ScrollArea className='min-h-0 flex-1 pr-3'>
                    <div className='space-y-3'>
                      {taskDetail.steps.map((step) => (
                        <div
                          key={step.id}
                          className='grid min-h-[184px] grid-rows-[auto_auto_minmax(0,1fr)_auto] overflow-hidden rounded-lg border p-4'
                        >
                          <div className='flex min-h-[24px] flex-wrap items-center gap-2'>
                            <Badge variant={getStatusVariant(step.status)}>
                              {t(`tasks.status.${step.status}`)}
                            </Badge>
                            <Badge variant='outline'>{step.code}</Badge>
                          </div>
                          <div
                            className='mt-2 min-h-[28px] truncate font-medium leading-7'
                            title={step.title}
                          >
                            {step.title}
                          </div>
                          <div
                            className='mt-2 line-clamp-3 text-sm leading-6 text-muted-foreground'
                            title={
                              localizeDiagnosticsText(step.message) ||
                              step.description ||
                              '-'
                            }
                          >
                            {localizeDiagnosticsText(step.message) ||
                              step.description ||
                              '-'}
                          </div>
                          <div className='mt-auto pt-3'>
                            {step.error &&
                            localizeDiagnosticsText(step.error) !==
                              localizeDiagnosticsText(step.message) ? (
                              <div
                                className='mb-3 line-clamp-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive'
                                title={localizeDiagnosticsText(step.error)}
                              >
                                {localizeDiagnosticsText(step.error)}
                              </div>
                            ) : null}
                            <div className='flex flex-wrap gap-4 text-xs text-muted-foreground'>
                              <span>
                                {t('tasks.startedAt')}:{' '}
                                {formatDateTime(step.started_at)}
                              </span>
                              <span>
                                {t('tasks.completedAt')}:{' '}
                                {formatDateTime(step.completed_at)}
                              </span>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </ScrollArea>
                )}
              </div>
            </section>

            <section className='flex min-h-[380px] flex-col overflow-hidden rounded-lg border xl:min-h-0 xl:h-full'>
              <div className='border-b px-4 py-3'>
                <div className='font-medium'>{t('tasks.nodesTitle')}</div>
              </div>
              <div className='flex min-h-0 flex-1 flex-col p-4'>
                {taskDetail.node_executions.length === 0 ? (
                  <div className='flex flex-1 items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                    {t('tasks.noNodes')}
                  </div>
                ) : (
                  <ScrollArea className='min-h-0 flex-1 pr-3'>
                    <div className='space-y-3'>
                      {taskDetail.node_executions.map((node) => (
                        <div
                          key={node.id}
                          className='grid min-h-[184px] grid-rows-[auto_auto_minmax(0,1fr)] overflow-hidden rounded-lg border p-4'
                        >
                          <div className='flex min-h-[24px] flex-wrap items-center gap-2'>
                            <Badge variant={getStatusVariant(node.status)}>
                              {t(`tasks.status.${node.status}`)}
                            </Badge>
                            <Badge variant='outline'>{node.role || '-'}</Badge>
                          </div>
                          <div
                            className='mt-2 min-h-[28px] truncate font-medium leading-7'
                            title={
                              node.host_name || node.host_ip || `#${node.host_id}`
                            }
                          >
                            {node.host_name || node.host_ip || `#${node.host_id}`}
                          </div>
                          <div className='mt-2 grid gap-2 text-sm text-muted-foreground'>
                            <div>
                              {t('tasks.hostIP')}: {node.host_ip || '-'}
                            </div>
                            <div>
                              {t('tasks.currentStep')}: {node.current_step || '-'}
                            </div>
                            <div
                              className='line-clamp-3 leading-6'
                              title={localizeDiagnosticsText(node.message) || '-'}
                            >
                              {localizeDiagnosticsText(node.message) || '-'}
                            </div>
                            {node.error &&
                            localizeDiagnosticsText(node.error) !==
                              localizeDiagnosticsText(node.message) ? (
                              <div
                                className='line-clamp-3 rounded-md bg-destructive/10 p-3 text-sm text-destructive'
                                title={localizeDiagnosticsText(node.error)}
                              >
                                {localizeDiagnosticsText(node.error)}
                              </div>
                            ) : null}
                          </div>
                        </div>
                      ))}
                    </div>
                  </ScrollArea>
                )}
              </div>
            </section>
          <section className='flex min-h-[320px] flex-col overflow-hidden rounded-lg border xl:col-span-2 xl:min-h-0 xl:h-full'>
            <div className='border-b px-4 py-3'>
              <div className='flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between'>
                <div className='font-medium'>{t('tasks.logsTitle')}</div>
                <Badge variant='outline'>
                  {t('tasks.logCount', {count: logTotal})}
                </Badge>
              </div>
            </div>
            <div className='flex min-h-0 flex-1 flex-col p-4'>
              <div className='grid gap-3 md:grid-cols-3'>
                <div className='space-y-2'>
                  <Label>{t('tasks.logFilters.step')}</Label>
                  <Select
                    value={String(logFilters.stepCode)}
                    onValueChange={(value) =>
                      setLogFilters((current) => ({
                        ...current,
                        stepCode:
                          value === 'all'
                            ? 'all'
                            : (value as DiagnosticsTaskStepCode),
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue
                        placeholder={t('tasks.logFilters.allSteps')}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='all'>
                        {t('tasks.logFilters.allSteps')}
                      </SelectItem>
                      {taskDetail.steps.map((step) => (
                        <SelectItem key={step.id} value={step.code}>
                          {step.title}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className='space-y-2'>
                  <Label>{t('tasks.logFilters.node')}</Label>
                  <Select
                    value={String(logFilters.nodeExecutionId)}
                    onValueChange={(value) =>
                      setLogFilters((current) => ({
                        ...current,
                        nodeExecutionId:
                          value === 'all' ? 'all' : Number.parseInt(value, 10),
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue
                        placeholder={t('tasks.logFilters.allNodes')}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='all'>
                        {t('tasks.logFilters.allNodes')}
                      </SelectItem>
                      {taskDetail.node_executions.map((node) => (
                        <SelectItem key={node.id} value={String(node.id)}>
                          {node.host_name || node.host_ip || `#${node.host_id}`}{' '}
                          · {node.role}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className='space-y-2'>
                  <Label>{t('tasks.logFilters.level')}</Label>
                  <Select
                    value={String(logFilters.level)}
                    onValueChange={(value) =>
                      setLogFilters((current) => ({
                        ...current,
                        level:
                          value === 'all'
                            ? 'all'
                            : (value as DiagnosticsLogLevel),
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue
                        placeholder={t('tasks.logFilters.allLevels')}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='all'>
                        {t('tasks.logFilters.allLevels')}
                      </SelectItem>
                      <SelectItem value='INFO'>INFO</SelectItem>
                      <SelectItem value='WARN'>WARN</SelectItem>
                      <SelectItem value='ERROR'>ERROR</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className='mt-4 flex min-h-0 flex-1 flex-col'>
                {loadingLogs ? (
                  <div className='space-y-3'>
                    <Skeleton className='h-10 w-full' />
                    <Skeleton className='h-10 w-full' />
                    <Skeleton className='h-10 w-full' />
                  </div>
                ) : logs.length === 0 ? (
                  <div className='flex flex-1 items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                    {t('tasks.noLogs')}
                  </div>
                ) : (
                  <ScrollArea className='min-h-0 flex-1 rounded-lg border bg-muted/20 p-4'>
                    <div className='space-y-3 font-mono text-xs'>
                      {logs.map((log) => (
                        <div
                          key={log.id}
                          className='space-y-2 rounded-md border bg-background p-3'
                        >
                          <div className='flex flex-wrap items-center gap-2'>
                            <Badge variant={getLogLevelVariant(log.level)}>
                              {log.level}
                            </Badge>
                            <Badge variant='outline'>{log.step_code}</Badge>
                            <span className='text-muted-foreground'>
                              {formatDateTime(log.created_at)}
                            </span>
                          </div>
                          <div className='whitespace-pre-wrap break-words text-sm'>
                            {localizeDiagnosticsText(log.message) || '-'}
                          </div>
                          {log.command_summary ? (
                            <div className='rounded bg-muted/40 p-2 text-muted-foreground'>
                              {log.command_summary}
                            </div>
                          ) : null}
                        </div>
                      ))}
                      <div ref={logEndRef} />
                    </div>
                  </ScrollArea>
                )}
              </div>
            </div>
          </section>
        </CardContent>
      </Card>
    );
  };

  return (
    <div className='grid gap-4 xl:grid-cols-[minmax(340px,0.78fr)_minmax(0,1.22fr)] xl:items-start'>
      <div className='space-y-4 xl:contents'>
        <Card className='flex flex-col overflow-hidden xl:col-start-1 xl:row-start-1 xl:self-start'>
          <CardHeader className='space-y-3'>
            <div className='flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between'>
              <div>
                <CardTitle>{t('tasks.title')}</CardTitle>
                <div className='mt-1 text-sm text-muted-foreground'>
                  {clusterId
                    ? t('tasks.clusterScopedHint', {name: selectedClusterLabel})
                    : t('tasks.globalHint')}
                </div>
              </div>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge variant='outline'>
                  {t('tasks.matchedTasks', {count: taskTotal})}
                </Badge>
                <Button variant='outline' onClick={() => void loadTasks()}>
                  <RefreshCw className='mr-2 h-4 w-4' />
                  {commonT('refresh')}
                </Button>
                <Button
                  onClick={() => void handleCreateTask()}
                  disabled={creatingTask}
                >
                  {creatingTask ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <Workflow className='mr-2 h-4 w-4' />
                  )}
                  {t('tasks.createTask')}
                </Button>
              </div>
            </div>

            <div className='rounded-lg border bg-muted/20 p-4'>
              <div className='grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start'>
                <div className='space-y-2'>
                  <Label htmlFor='diagnostics-task-trigger-source'>
                    {t('tasks.createModeLabel')}
                  </Label>
                  <Select
                    value={selectedCreateSource}
                    onValueChange={(value) =>
                      setSelectedCreateSource(value as DiagnosticsTaskSourceType)
                    }
                  >
                    <SelectTrigger id='diagnostics-task-trigger-source'>
                      <SelectValue
                        placeholder={t('tasks.createModePlaceholder')}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {availableCreateSources.map((item) => (
                        <SelectItem key={item} value={item}>
                          {t(`tasks.source.${item}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <div className='text-xs text-muted-foreground'>
                    {t('tasks.createModeHint')}
                  </div>
                  {onlyManualCreateMode ? (
                    <div className='text-xs text-muted-foreground'>
                      {t('tasks.manualOnlyHint')}
                    </div>
                  ) : null}
                </div>
              </div>

              <div className='mt-4 flex flex-wrap items-center gap-2'>
                <Badge variant='secondary'>
                  {t(`tasks.source.${createContext.triggerSource}`)}
                </Badge>
                {entrySourceLabel ? (
                  <Badge variant='outline'>
                    {t('tasks.entryRouteLabel', {source: entrySourceLabel})}
                  </Badge>
                ) : null}
              </div>
              <div className='mt-3 text-sm text-muted-foreground'>
                {t(`tasks.createContext.${createContext.summaryKey}`)}
              </div>
              {hasSourcePrefill && createContext.triggerSource === 'manual' ? (
                <div className='mt-2 text-xs text-muted-foreground'>
                  {t('tasks.prefillAvailableHint')}
                </div>
              ) : null}
              <div className='mt-4 text-xs font-medium text-muted-foreground'>
                {t('tasks.diagnosisBasisLabel')}
              </div>
              {sourceDetailItems.length > 0 ? (
                <div className='mt-3 flex flex-wrap gap-2'>
                  {sourceDetailItems.map((item) => (
                    <Badge key={item} variant='outline'>
                      {item}
                    </Badge>
                  ))}
                </div>
              ) : (
                <div className='mt-2 text-sm text-muted-foreground'>
                  {t('tasks.noDiagnosisBasis')}
                </div>
              )}

              <div className='mt-4 grid gap-4 md:grid-cols-2'>
                <div className='flex items-center justify-between rounded-lg border bg-background p-3'>
                  <div>
                    <div className='font-medium'>
                      {t('tasks.includeThreadDump')}
                    </div>
                    <div className='text-xs text-muted-foreground'>
                      {t('tasks.includeThreadDumpHint')}
                    </div>
                  </div>
                  <Switch
                    checked={createOptions.include_thread_dump}
                    onCheckedChange={(checked) =>
                      setCreateOptions((current) => ({
                        ...current,
                        include_thread_dump: checked,
                      }))
                    }
                  />
                </div>

                <div className='flex items-center justify-between rounded-lg border bg-background p-3'>
                  <div>
                    <div className='font-medium'>
                      {t('tasks.includeJVMDump')}
                    </div>
                    <div className='text-xs text-muted-foreground'>
                      {t('tasks.includeJVMDumpHint')}
                    </div>
                  </div>
                  <Switch
                    checked={createOptions.include_jvm_dump}
                    onCheckedChange={(checked) =>
                      setCreateOptions((current) => ({
                        ...current,
                        include_jvm_dump: checked,
                      }))
                    }
                  />
                </div>

                <div className='space-y-2'>
                  <Label htmlFor='diagnostics-task-jvm-space'>
                    {t('tasks.jvmDumpMinFreeMB')}
                  </Label>
                  <Input
                    id='diagnostics-task-jvm-space'
                    type='number'
                    min={256}
                    step={256}
                    value={
                      createOptions.jvm_dump_min_free_mb ||
                      DEFAULT_TASK_OPTIONS.jvm_dump_min_free_mb
                    }
                    onChange={(event) =>
                      setCreateOptions((current) => ({
                        ...current,
                        jvm_dump_min_free_mb:
                          Number.parseInt(event.target.value, 10) ||
                          DEFAULT_TASK_OPTIONS.jvm_dump_min_free_mb,
                      }))
                    }
                    disabled={!createOptions.include_jvm_dump}
                  />
                </div>
              </div>

              <div className='mt-4 flex items-center gap-3'>
                <Switch checked={autoStart} onCheckedChange={setAutoStart} />
                <div className='text-sm text-muted-foreground'>
                  {t('tasks.autoStart')}
                </div>
              </div>
            </div>
          </CardHeader>
        </Card>

        <Card className='flex h-full min-h-[560px] flex-col overflow-hidden xl:col-start-1 xl:row-start-2 xl:min-h-0'>
          <CardHeader className='space-y-4'>
            <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
              <CardTitle>{t('tasks.listTitle')}</CardTitle>
              <div className='w-full max-w-[220px]'>
                <Select
                  value={statusFilter}
                  onValueChange={(value) => {
                    setStatusFilter(value as 'all' | DiagnosticsTaskStatus);
                    setPage(1);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('tasks.filters.status')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('tasks.filters.allStatuses')}
                    </SelectItem>
                    {(
                      [
                        'pending',
                        'ready',
                        'running',
                        'succeeded',
                        'failed',
                        'skipped',
                        'cancelled',
                      ] as const
                    ).map((status) => (
                      <SelectItem key={status} value={status}>
                        {t(`tasks.status.${status}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </CardHeader>
          <CardContent className='flex flex-1 min-h-0 flex-col space-y-4'>
            {loadingTasks ? (
              <div className='space-y-3'>
                <Skeleton className='h-20 w-full' />
                <Skeleton className='h-20 w-full' />
                <Skeleton className='h-20 w-full' />
              </div>
            ) : tasks.length === 0 ? (
              <div className='flex flex-1 items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                {t('tasks.empty')}
              </div>
            ) : (
              <ScrollArea className='min-h-0 flex-1 pr-3'>
                <div className='space-y-3'>
                  {tasks.map((task) => (
                    <button
                      key={task.id}
                      type='button'
                      className={
                        selectedTaskId === task.id
                          ? 'flex min-h-[164px] w-full flex-col overflow-hidden rounded-lg border border-primary bg-muted/40 p-4 text-left'
                          : 'flex min-h-[164px] w-full flex-col overflow-hidden rounded-lg border p-4 text-left transition-colors hover:bg-muted/20'
                      }
                      onClick={() => {
                        setSelectedTaskId(task.id);
                        onSelectTask?.(task.id);
                      }}
                    >
                      <div className='flex min-h-[24px] flex-wrap items-center gap-2'>
                        <Badge variant={getStatusVariant(task.status)}>
                          {t(`tasks.status.${task.status}`)}
                        </Badge>
                        <Badge variant='outline'>
                          {t(`tasks.source.${task.trigger_source}`)}
                        </Badge>
                        <span className='font-medium'>#{task.id}</span>
                      </div>
                      <div
                        className='mt-3 min-h-[48px] line-clamp-2 text-sm font-medium leading-6'
                        title={
                          localizeDiagnosticsText(task.summary) ||
                          t('tasks.summaryFallback')
                        }
                      >
                        {localizeDiagnosticsText(task.summary) ||
                          t('tasks.summaryFallback')}
                      </div>
                      <div
                        className='mt-2 min-h-[24px] truncate text-sm text-muted-foreground'
                        title={task.current_step || '-'}
                      >
                        {t('tasks.currentStep')}: {task.current_step || '-'}
                      </div>
                      {task.failure_reason ? (
                        <div
                          className='mt-2 min-h-[44px] line-clamp-2 text-sm leading-6 text-destructive'
                          title={task.failure_reason}
                        >
                          {task.failure_reason}
                        </div>
                      ) : (
                        <div className='mt-2 min-h-[44px]' />
                      )}
                      <div className='mt-auto pt-3'>
                        <div className='flex flex-wrap gap-4 text-xs text-muted-foreground'>
                          <span>
                            {t('tasks.clusterLabel')}: #{task.cluster_id}
                          </span>
                          <span>
                            {t('tasks.createdAt')}:{' '}
                            {formatDateTime(task.created_at)}
                          </span>
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              </ScrollArea>
            )}

            {taskTotal > TASK_PAGE_SIZE ? (
              <Pagination
                currentPage={page}
                totalPages={totalPages}
                pageSize={TASK_PAGE_SIZE}
                totalItems={taskTotal}
                onPageChange={setPage}
                showPageSizeSelector={false}
              />
            ) : null}
          </CardContent>
        </Card>
      </div>

      {renderTaskDetail()}
    </div>
  );
}
