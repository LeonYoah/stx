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

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {useRouter} from 'next/navigation';
import Link from 'next/link';
import {
  Activity,
  AlertTriangle,
  BellRing,
  BookOpen,
  CalendarClock,
  ClipboardList,
  Layers,
  RefreshCw,
  Server,
  Variable,
  LayoutTemplate,
} from 'lucide-react';
import {OverviewService, OverviewData} from '@/lib/services/dashboard';
import type {RecentActivity} from '@/lib/services/dashboard';
import services from '@/lib/services';
import type {
  AlertInstance,
  PlatformHealthData,
} from '@/lib/services/monitoring';
import {MonitoringOverview} from '@/components/common/monitoring';
import {lookupAuditLabel} from '@/components/common/audit/audit-i18n';
import {useAuth} from '@/hooks/use-auth';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {STXMark} from '@/components/icons/logo';
import {
  WorkspaceHeader,
  StatPillsBar,
  type StatPillItem,
} from '@/components/common/layout';
import {cn} from '@/lib/utils';

// 四宫格面板固定高度，避免条目增减导致版面跳动；内容区内滚
// Fixed quad-panel height so list length does not shift layout; body scrolls
const PANEL_CARD_CLASS =
  'border-border/70 shadow-xs flex h-[18rem] flex-col gap-0 overflow-hidden py-0';
const PANEL_BODY_CLASS = 'min-h-0 flex-1 overflow-y-auto px-4 pb-4 pt-0';

/** 用户侧资源库存计数 / User-owned resource inventory counts */
type ResourceInventory = {
  syncTasksMine: number;
  syncTasksTotal: number;
  scheduledTasks: number;
  templatesUser: number;
  templatesBuiltin: number;
  variablesMine: number;
  variablesTotal: number;
  playbooksCustom: number;
  playbooksPreset: number;
};

// 控制台首屏胶囊焦点：资源统计 + 告警入口
// Dashboard first-viewport pill focus: resource stats + alert entries
type DashboardPillKey =
  | 'hosts'
  | 'clusters'
  | 'unhealthy'
  | 'offlineAgents'
  | 'alerts'
  | 'critical';

/** 可行动待办：异常优先，每条都带明确跳转 / Actionable todo with a clear drill-down */
type ActionTodo = {
  id: string;
  title: string;
  detail: string;
  href: string;
  severity: 'critical' | 'warning' | 'info';
};

export default function DashboardPage() {
  const t = useTranslations('dashboard');
  const tRoot = useTranslations();
  const router = useRouter();
  const {user} = useAuth();
  const currentUserId = user?.id;
  const [data, setData] = useState<OverviewData | null>(null);
  const [platformHealth, setPlatformHealth] =
    useState<PlatformHealthData | null>(null);
  const [alertEvents, setAlertEvents] = useState<AlertInstance[]>([]);
  const [firingAlertTotal, setFiringAlertTotal] = useState(0);
  const [inventory, setInventory] = useState<ResourceInventory>({
    syncTasksMine: 0,
    syncTasksTotal: 0,
    scheduledTasks: 0,
    templatesUser: 0,
    templatesBuiltin: 0,
    variablesMine: 0,
    variablesTotal: 0,
    playbooksCustom: 0,
    playbooksPreset: 0,
  });
  const [initialLoading, setInitialLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activePill, setActivePill] = useState<DashboardPillKey>('hosts');

  // 拉取概览、告警与用户侧资源库存；静默刷新不塌陷首屏
  // Fetch overview, alerts, and user-owned inventory; silent refresh keeps layout stable
  const fetchData = useCallback(async (opts?: {silent?: boolean}) => {
    const silent = Boolean(opts?.silent);
    if (silent) {
      setRefreshing(true);
    } else {
      setInitialLoading(true);
    }
    try {
      const [
        overviewResult,
        healthResult,
        alertsResult,
        syncResult,
        templatesResult,
        variablesResult,
        playbooksResult,
      ] = await Promise.all([
        OverviewService.getOverviewDataSafe(),
        services.monitoring.getPlatformHealthSafe(),
        services.monitoring.getAlertInstancesSafe({
          status: 'firing',
          page: 1,
          page_size: 20,
        }),
        services.sync.listTasks({current: 1, size: 500}).then(
          (payload) => ({success: true as const, data: payload}),
          () => ({success: false as const, data: undefined}),
        ),
        services.sync.listCuratedTemplates({}).then(
          (payload) => ({success: true as const, data: payload}),
          () => ({success: false as const, data: undefined}),
        ),
        services.sync.listGlobalVariables({current: 1, size: 500}).then(
          (payload) => ({success: true as const, data: payload}),
          () => ({success: false as const, data: undefined}),
        ),
        services.troubleshooting.fetchRemoteMemories().then(
          (items) => ({success: true as const, data: items}),
          () => ({success: false as const, data: undefined}),
        ),
      ]);

      if (overviewResult.success && overviewResult.data) {
        setData(overviewResult.data);
        setError(null);
      } else {
        setData(overviewResult.data ?? null);
        setError(overviewResult.error || t('loadError'));
      }

      if (healthResult.success && healthResult.data) {
        setPlatformHealth(healthResult.data);
      } else if (!silent) {
        setPlatformHealth(null);
      }

      if (alertsResult.success && alertsResult.data) {
        setAlertEvents(alertsResult.data.alerts ?? []);
        setFiringAlertTotal(
          alertsResult.data.stats?.firing ?? alertsResult.data.total ?? 0,
        );
      } else if (!silent) {
        setAlertEvents([]);
        setFiringAlertTotal(0);
      }

      // 同步任务：文件节点总数 / 我创建 / 已启用定时
      // Sync files: workspace total / mine / schedule-enabled
      if (syncResult.success && syncResult.data) {
        const files = (syncResult.data.items ?? []).filter(
          (item) => item.node_type === 'file',
        );
        const mine = currentUserId
          ? files.filter((item) => item.created_by === currentUserId)
          : [];
        setInventory((prev) => ({
          ...prev,
          syncTasksTotal: files.length,
          syncTasksMine: mine.length,
          scheduledTasks: files.filter((item) => Boolean(item.schedule_enabled))
            .length,
        }));
      } else if (!silent) {
        setInventory((prev) => ({
          ...prev,
          syncTasksTotal: 0,
          syncTasksMine: 0,
          scheduledTasks: 0,
        }));
      }

      // 精选模板：用户 / 内置（含 override）
      // Curated templates: user-origin vs builtin/override
      if (templatesResult.success && templatesResult.data) {
        const items = templatesResult.data.items ?? [];
        setInventory((prev) => ({
          ...prev,
          templatesUser: items.filter((item) => item.origin === 'user').length,
          templatesBuiltin: items.filter(
            (item) => item.origin === 'builtin' || item.origin === 'override',
          ).length,
        }));
      } else if (!silent) {
        setInventory((prev) => ({
          ...prev,
          templatesUser: 0,
          templatesBuiltin: 0,
        }));
      }

      // 全局变量：工作区合计 / 我创建
      // Global variables: workspace total / mine
      if (variablesResult.success && variablesResult.data) {
        const items = variablesResult.data.items ?? [];
        setInventory((prev) => ({
          ...prev,
          variablesTotal: variablesResult.data?.total ?? items.length,
          variablesMine: currentUserId
            ? items.filter((item) => item.created_by === currentUserId).length
            : 0,
        }));
      } else if (!silent) {
        setInventory((prev) => ({
          ...prev,
          variablesTotal: 0,
          variablesMine: 0,
        }));
      }

      // 经验库：区分用户沉淀与预置
      // Playbooks: split custom vs preset
      if (playbooksResult.success && playbooksResult.data) {
        const items = playbooksResult.data;
        setInventory((prev) => ({
          ...prev,
          playbooksCustom: items.filter((item) => !item.is_preset).length,
          playbooksPreset: items.filter((item) => Boolean(item.is_preset))
            .length,
        }));
      } else if (!silent) {
        setInventory((prev) => ({
          ...prev,
          playbooksCustom: 0,
          playbooksPreset: 0,
        }));
      }
    } finally {
      setInitialLoading(false);
      setRefreshing(false);
    }
  }, [currentUserId, t]);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => {
      void fetchData({silent: true});
    }, 30000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const stats = data?.stats;
  const offlineHosts = Math.max(
    0,
    (stats?.total_hosts ?? 0) - (stats?.online_hosts ?? 0),
  );
  const offlineAgents = Math.max(
    0,
    (stats?.total_agents ?? 0) - (stats?.online_agents ?? 0),
  );
  const unhealthyClusters =
    platformHealth?.unhealthy_clusters ?? stats?.error_clusters ?? 0;
  const degradedClusters = platformHealth?.degraded_clusters ?? 0;
  const attentionClusterCount = unhealthyClusters + degradedClusters;
  const activeAlerts = platformHealth?.active_alerts ?? firingAlertTotal;
  const criticalAlerts = platformHealth?.critical_alerts ?? 0;

  const recentActivities = data?.recent_activities ?? [];

  // 待办提醒：只列需要用户动手的异常，附带入口（业界 exception-first）
  // Todos: only actionable exceptions with entry points (exception-first)
  const actionTodos = useMemo((): ActionTodo[] => {
    const items: ActionTodo[] = [];

    if ((stats?.total_hosts ?? 0) === 0) {
      items.push({
        id: 'onboard-host',
        title: t('todos.onboardHostTitle'),
        detail: t('todos.onboardHostDetail'),
        href: '/hosts',
        severity: 'info',
      });
    } else if (offlineHosts > 0) {
      items.push({
        id: 'offline-hosts',
        title: t('todos.offlineHostsTitle', {count: offlineHosts}),
        detail: t('todos.offlineHostsDetail'),
        href: '/hosts',
        severity: 'warning',
      });
    }

    if (offlineAgents > 0) {
      items.push({
        id: 'offline-agents',
        title: t('todos.offlineAgentsTitle', {count: offlineAgents}),
        detail: t('todos.offlineAgentsDetail'),
        href: '/hosts',
        severity: 'warning',
      });
    }

    if (attentionClusterCount > 0) {
      items.push({
        id: 'unhealthy-clusters',
        title: t('todos.unhealthyClustersTitle', {
          count: attentionClusterCount,
        }),
        detail: t('todos.unhealthyClustersDetail'),
        href: '/clusters',
        severity: unhealthyClusters > 0 ? 'critical' : 'warning',
      });
    }

    if (criticalAlerts > 0) {
      items.push({
        id: 'critical-alerts',
        title: t('todos.criticalAlertsTitle', {count: criticalAlerts}),
        detail: t('todos.criticalAlertsDetail'),
        href: '/monitoring?tab=alerts',
        severity: 'critical',
      });
    } else if (activeAlerts > 0) {
      items.push({
        id: 'active-alerts',
        title: t('todos.activeAlertsTitle', {count: activeAlerts}),
        detail: t('todos.activeAlertsDetail'),
        href: '/monitoring?tab=alerts',
        severity: 'warning',
      });
    }

    return items.slice(0, 6);
  }, [
    activeAlerts,
    attentionClusterCount,
    criticalAlerts,
    offlineAgents,
    offlineHosts,
    stats?.total_hosts,
    t,
    unhealthyClusters,
  ]);

  const pillItems: StatPillItem[] = useMemo(
    () => [
      {
        key: 'hosts',
        label: t('pills.hosts'),
        count: `${stats?.online_hosts ?? 0}/${stats?.total_hosts ?? 0}`,
        icon: <Server className='h-3 w-3' />,
        variant:
          (stats?.total_hosts ?? 0) > 0 && offlineHosts > 0
            ? 'warning'
            : 'success',
      },
      {
        key: 'clusters',
        label: t('pills.clusters'),
        count: `${stats?.running_clusters ?? 0}/${stats?.total_clusters ?? 0}`,
        icon: <Layers className='h-3 w-3' />,
        variant: 'default',
      },
      {
        key: 'unhealthy',
        label: t('pills.unhealthyClusters'),
        count: attentionClusterCount,
        icon: <AlertTriangle className='h-3 w-3' />,
        variant: attentionClusterCount > 0 ? 'danger' : 'success',
        pulse: unhealthyClusters > 0,
      },
      {
        key: 'offlineAgents',
        label: t('pills.offlineAgents'),
        count: offlineAgents,
        icon: <Activity className='h-3 w-3' />,
        variant: offlineAgents > 0 ? 'warning' : 'success',
      },
      {
        key: 'alerts',
        label: t('pills.activeAlerts'),
        count: activeAlerts,
        icon: <BellRing className='h-3 w-3' />,
        variant: activeAlerts > 0 ? 'warning' : 'success',
        pulse: activeAlerts > 0,
      },
      {
        key: 'critical',
        label: t('pills.criticalAlerts'),
        count: criticalAlerts,
        icon: <BellRing className='h-3 w-3' />,
        variant: criticalAlerts > 0 ? 'danger' : 'default',
        pulse: criticalAlerts > 0,
      },
    ],
    [
      activeAlerts,
      attentionClusterCount,
      criticalAlerts,
      offlineAgents,
      offlineHosts,
      stats?.online_hosts,
      stats?.running_clusters,
      stats?.total_clusters,
      stats?.total_hosts,
      t,
      unhealthyClusters,
    ],
  );

  // 活动行：操作人 · 来源 · 操作名（本地化）
  // Activity row: operator · source · action (localized)
  const formatActivityLine = useCallback(
    (activity: RecentActivity): string => {
      const operatorRaw = (activity.operator || '').trim();
      const operator =
        !operatorRaw || operatorRaw === 'system'
          ? tRoot('audit.actorSystem')
          : operatorRaw;

      const sourceRaw = (activity.source || '').trim().toLowerCase();
      const source =
        sourceRaw === 'cli' || sourceRaw === 'web' || sourceRaw === 'api'
          ? lookupAuditLabel('clientTypes', sourceRaw, sourceRaw, tRoot)
          : tRoot('audit.actorSystem');

      const actionRaw = (activity.action || '').trim();
      const action = actionRaw
        ? lookupAuditLabel('actions', actionRaw, actionRaw, tRoot)
        : '-';

      if (operator || source || actionRaw) {
        return `${operator} · ${source} · ${action}`;
      }
      return activity.message?.trim() || '-';
    },
    [tRoot],
  );

  const severityBadgeVariant = (
    severity: string,
  ): 'default' | 'secondary' | 'destructive' | 'outline' => {
    const value = severity.toLowerCase();
    if (value === 'critical') return 'destructive';
    if (value === 'warning') return 'secondary';
    return 'outline';
  };

  const handlePillChange = useCallback(
    (key: string) => {
      const pill = key as DashboardPillKey;
      setActivePill(pill);
      switch (pill) {
        case 'hosts':
        case 'offlineAgents':
          router.push('/hosts');
          break;
        case 'clusters':
        case 'unhealthy':
          router.push('/clusters');
          break;
        case 'alerts':
        case 'critical':
          router.push('/monitoring?tab=alerts');
          break;
        default:
          break;
      }
    },
    [router],
  );

  // 资源明细：用户自定义资源库存（含我的 / 全部拆分）
  // Resource breakdown: user-owned inventory with mine / total splits
  const resourceRows = useMemo(() => {
    const s = stats;
    return [
      {
        key: 'hosts',
        href: '/hosts',
        label: t('resources.hosts'),
        icon: <Server className='h-3.5 w-3.5 text-muted-foreground' />,
        primary: String(s?.total_hosts ?? 0),
        primaryHint: t('resources.total'),
        secondary: [
          {label: t('resources.online'), value: s?.online_hosts ?? 0},
          {label: t('resources.offline'), value: offlineHosts},
        ],
      },
      {
        key: 'clusters',
        href: '/clusters',
        label: t('resources.clusters'),
        icon: <Layers className='h-3.5 w-3.5 text-muted-foreground' />,
        primary: String(s?.total_clusters ?? 0),
        primaryHint: t('resources.total'),
        secondary: [
          {label: t('resources.running'), value: s?.running_clusters ?? 0},
          {label: t('resources.error'), value: s?.error_clusters ?? 0},
        ],
      },
      {
        key: 'sync',
        href: '/workbench',
        label: t('resources.syncTasks'),
        icon: <CalendarClock className='h-3.5 w-3.5 text-muted-foreground' />,
        primary: String(inventory.syncTasksMine),
        primaryHint: t('resources.mine'),
        secondary: [
          {label: t('resources.all'), value: inventory.syncTasksTotal},
          {label: t('resources.scheduled'), value: inventory.scheduledTasks},
        ],
      },
      {
        key: 'templates',
        href: '/workbench',
        label: t('resources.templates'),
        icon: <LayoutTemplate className='h-3.5 w-3.5 text-muted-foreground' />,
        primary: String(inventory.templatesUser),
        primaryHint: t('resources.custom'),
        secondary: [
          {label: t('resources.builtin'), value: inventory.templatesBuiltin},
        ],
      },
      {
        key: 'variables',
        href: '/workbench',
        label: t('resources.variables'),
        icon: <Variable className='h-3.5 w-3.5 text-muted-foreground' />,
        primary: String(inventory.variablesMine),
        primaryHint: t('resources.mine'),
        secondary: [
          {label: t('resources.all'), value: inventory.variablesTotal},
        ],
      },
      {
        key: 'playbooks',
        href: '/diagnostics?tab=memories',
        label: t('resources.playbooks'),
        icon: <BookOpen className='h-3.5 w-3.5 text-muted-foreground' />,
        primary: String(inventory.playbooksCustom),
        primaryHint: t('resources.custom'),
        secondary: [
          {label: t('resources.preset'), value: inventory.playbooksPreset},
        ],
      },
    ];
  }, [inventory, offlineHosts, stats, t]);

  const showSkeleton = initialLoading && !data;

  return (
    <div className='space-y-3.5 flex-1 flex flex-col'>
      <WorkspaceHeader
        icon={<STXMark className='size-8 object-contain' />}
        iconTint={false}
        title={t('title')}
        actions={
          <Button
            variant='outline'
            size='sm'
            onClick={() => void fetchData({silent: true})}
            disabled={refreshing || initialLoading}
            className='h-8.5 text-xs'
          >
            <RefreshCw
              className={cn(
                'mr-1.5 h-3.5 w-3.5',
                (refreshing || initialLoading) && 'animate-spin',
              )}
            />
            {refreshing ? t('refreshing') : t('refresh')}
          </Button>
        }
      />

      {error ? (
        <div className='rounded-lg border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs text-destructive'>
          {error}
        </div>
      ) : null}

      {/* 资源统计胶囊：一眼扫盘，点击直达对应面 */}
      {/* Resource pills: scan health, click through to the matching surface */}
      <Card className='border-border/70 shadow-xs'>
        <CardContent className='p-3 sm:p-3.5'>
          <StatPillsBar
            items={pillItems}
            activeKey={activePill}
            onChange={handlePillChange}
            actions={
              <div className='flex items-center gap-1.5'>
                <Button
                  asChild
                  variant='outline'
                  size='sm'
                  className='h-7 px-2.5 text-xs'
                >
                  <Link href='/monitoring?tab=alerts'>{t('viewAllAlerts')}</Link>
                </Button>
                <Button
                  asChild
                  variant='ghost'
                  size='sm'
                  className='h-7 px-2.5 text-xs'
                >
                  <Link href='/clusters'>{t('viewClusters')}</Link>
                </Button>
              </div>
            }
          />
        </CardContent>
      </Card>

      {/* 异常优先：告警事件 + 可行动待办（等高、内滚） */}
      {/* Exception-first: alert events + todos (equal height, internal scroll) */}
      <div className='grid grid-cols-1 gap-3 lg:grid-cols-2'>
        <Card className={PANEL_CARD_CLASS}>
          <CardHeader className='flex shrink-0 flex-row items-center justify-between space-y-0 px-4 py-3'>
            <CardTitle className='text-sm font-semibold'>
              {t('alertEventsTitle')}
            </CardTitle>
            <div className='flex items-center gap-1.5'>
              <Badge
                variant={firingAlertTotal > 0 ? 'destructive' : 'secondary'}
              >
                {firingAlertTotal}
              </Badge>
              <Button
                asChild
                variant='ghost'
                size='sm'
                className='h-7 px-2 text-xs'
              >
                <Link href='/monitoring?tab=alerts'>{t('viewAllAlerts')}</Link>
              </Button>
            </div>
          </CardHeader>
          <CardContent className={PANEL_BODY_CLASS}>
            {showSkeleton ? (
              <div className='space-y-2'>
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
              </div>
            ) : alertEvents.length === 0 ? (
              <div className='flex h-full min-h-[8rem] items-center justify-center'>
                <p className='text-xs text-muted-foreground'>
                  {t('alertEventsEmpty')}
                </p>
              </div>
            ) : (
              <ul className='space-y-1.5'>
                {alertEvents.map((alert) => (
                  <li key={alert.alert_id}>
                    <Link
                      href={
                        alert.cluster_id
                          ? `/monitoring?tab=alerts&cluster_id=${encodeURIComponent(alert.cluster_id)}`
                          : '/monitoring?tab=alerts'
                      }
                      className='flex items-center justify-between gap-2 rounded-md border border-border/60 bg-muted/20 px-2.5 py-1.5 text-xs transition-colors hover:bg-muted/40'
                    >
                      <span className='min-w-0 flex-1 truncate'>
                        <span className='font-medium'>
                          {alert.alert_name || alert.rule_key || alert.alert_id}
                        </span>
                        {alert.cluster_name ? (
                          <span className='ml-1.5 text-muted-foreground'>
                            · {alert.cluster_name}
                          </span>
                        ) : null}
                      </span>
                      <Badge
                        variant={severityBadgeVariant(String(alert.severity))}
                        className='shrink-0 text-[10px]'
                      >
                        {String(alert.severity)}
                      </Badge>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card className={PANEL_CARD_CLASS}>
          <CardHeader className='flex shrink-0 flex-row items-center justify-between space-y-0 px-4 py-3'>
            <CardTitle className='flex items-center gap-1.5 text-sm font-semibold'>
              <ClipboardList className='h-3.5 w-3.5 text-muted-foreground' />
              {t('todosTitle')}
            </CardTitle>
            <Badge variant={actionTodos.length > 0 ? 'secondary' : 'outline'}>
              {actionTodos.length}
            </Badge>
          </CardHeader>
          <CardContent className={PANEL_BODY_CLASS}>
            {showSkeleton ? (
              <div className='space-y-2'>
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
              </div>
            ) : actionTodos.length === 0 ? (
              <div className='flex h-full min-h-[8rem] items-center justify-center'>
                <p className='text-xs text-muted-foreground'>{t('todosEmpty')}</p>
              </div>
            ) : (
              <ul className='space-y-1.5'>
                {actionTodos.map((todo) => (
                  <li key={todo.id}>
                    <Link
                      href={todo.href}
                      className='flex items-start justify-between gap-2 rounded-md border border-border/60 bg-muted/20 px-2.5 py-1.5 text-xs transition-colors hover:bg-muted/40'
                    >
                      <span className='min-w-0 flex-1'>
                        <span className='block truncate font-medium'>
                          {todo.title}
                        </span>
                        <span className='mt-0.5 block truncate text-muted-foreground'>
                          {todo.detail}
                        </span>
                      </span>
                      <Badge
                        variant={severityBadgeVariant(todo.severity)}
                        className='mt-0.5 shrink-0 text-[10px]'
                      >
                        {t(`todos.severity.${todo.severity}`)}
                      </Badge>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      {/* 最近操作 + 资源明细（等高、内滚） */}
      {/* Recent ops + resource inventory (equal height, internal scroll) */}
      <div className='grid grid-cols-1 gap-3 lg:grid-cols-2'>
        <Card className={PANEL_CARD_CLASS}>
          <CardHeader className='flex shrink-0 flex-row items-center justify-between space-y-0 px-4 py-3'>
            <CardTitle className='text-sm font-semibold'>
              {t('recentActivitiesTitle')}
            </CardTitle>
            <Button
              asChild
              variant='ghost'
              size='sm'
              className='h-7 px-2 text-xs'
            >
              <Link href='/audit-logs'>{t('viewAuditLogs')}</Link>
            </Button>
          </CardHeader>
          <CardContent className={PANEL_BODY_CLASS}>
            {showSkeleton ? (
              <div className='space-y-2'>
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
                <div className='h-8 animate-pulse rounded-md bg-muted/60' />
              </div>
            ) : recentActivities.length === 0 ? (
              <div className='flex h-full min-h-[8rem] items-center justify-center'>
                <p className='text-xs text-muted-foreground'>
                  {t('activitiesEmpty')}
                </p>
              </div>
            ) : (
              <ul className='space-y-1.5'>
                {recentActivities.map((activity) => (
                  <li
                    key={activity.id}
                    className='flex items-start justify-between gap-2 rounded-md border border-border/60 bg-muted/20 px-2.5 py-1.5 text-xs'
                  >
                    <span className='min-w-0 flex-1 truncate'>
                      {formatActivityLine(activity)}
                    </span>
                    <span className='shrink-0 font-mono text-[11px] text-muted-foreground'>
                      {activity.timestamp}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card className={PANEL_CARD_CLASS}>
          <CardHeader className='flex shrink-0 flex-row items-center justify-between space-y-0 px-4 py-3'>
            <CardTitle className='text-sm font-semibold'>
              {t('resourcesTitle')}
            </CardTitle>
          </CardHeader>
          <CardContent className={PANEL_BODY_CLASS}>
            {showSkeleton ? (
              <div className='space-y-2'>
                <div className='h-10 animate-pulse rounded-md bg-muted/60' />
                <div className='h-10 animate-pulse rounded-md bg-muted/60' />
              </div>
            ) : (
              <ul className='space-y-1.5'>
                {resourceRows.map((row) => (
                  <li key={row.key}>
                    <Link
                      href={row.href}
                      className='flex items-center gap-2 rounded-md border border-border/60 bg-muted/20 px-2.5 py-2 text-xs transition-colors hover:bg-muted/40'
                    >
                      {row.icon}
                      <span className='w-14 shrink-0 font-medium'>
                        {row.label}
                      </span>
                      <span className='min-w-0 flex-1'>
                        <span className='font-mono font-semibold tabular-nums'>
                          {row.primary}
                        </span>
                        <span className='ml-1.5 text-muted-foreground'>
                          {row.primaryHint}
                        </span>
                      </span>
                      <span className='flex shrink-0 flex-wrap justify-end gap-x-2 gap-y-0.5 text-[11px] text-muted-foreground'>
                        {row.secondary.map((item) => (
                          <span key={item.label} className='whitespace-nowrap'>
                            {item.label}{' '}
                            <span className='font-mono tabular-nums text-foreground/80'>
                              {item.value}
                            </span>
                          </span>
                        ))}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Grafana：按需展开，不抢首屏 */}
      {/* Grafana: on-demand expand, does not dominate first viewport */}
      <MonitoringOverview compact defaultGrafanaExpanded={false} />
    </div>
  );
}
