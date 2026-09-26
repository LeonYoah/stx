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
import {useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  Archive,
  Bell,
  CheckCircle2,
  ExternalLink,
  Eye,
  Info,
  Lightbulb,
  RefreshCw,
  RotateCcw,
  Search,
  ShieldAlert,
  VolumeX,
  X,
  XCircle,
} from 'lucide-react';
import gsap from 'gsap';
import {useGSAP} from '@gsap/react';
import {toast} from 'sonner';

// 注册 GSAP 核心与 React 扩展插件
// Register GSAP core and React extension plugin
if (typeof window !== 'undefined') {
  gsap.registerPlugin(useGSAP);
}
import services from '@/lib/services';
import type {
  AlertDisplayStatus,
  AlertInstance,
  AlertInstanceStats,
  AlertSeverity,
  AlertSourceType,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {
  ExpandableTextPanel,
  StatPillsBar,
  type StatPillItem,
  CompactTimeFilter,
  TableLoadingBar,
  TableSkeletonRows,
} from '@/components/common/layout';
import {
  animateSheetSections,
  animateTableRows,
} from '@/lib/animations/gsap-motion';
import {
  TroubleshootingMemoryCard,
  SaveMemoryDialog,
} from '@/components/common/troubleshooting';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card} from '@/components/ui/card';
import {Input} from '@/components/ui/input';
import {Pagination} from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';

type ClusterOption = {
  id: string;
  name: string;
};

const EMPTY_STATS: AlertInstanceStats = {
  firing: 0,
  resolved: 0,
  closed: 0,
};

// 格式化日期时间
// Format ISO date string into readable local date time
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

// 转换为 RFC3339 字符串
// Convert date string to standard RFC3339 ISO string
function toRFC3339(value: string): string | undefined {
  if (!value) {
    return undefined;
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return undefined;
  }
  return parsed.toISOString();
}

// 格式化为 datetime-local 所需的 YYYY-MM-DDTHH:mm 本地时间字符串
// Format Date object into YYYY-MM-DDTHH:mm string required by HTML5 datetime-local input
function formatToDateTimeLocal(date: Date): string {
  const pad = (num: number) => String(num).padStart(2, '0');
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const minutes = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

// 检查静音是否处于活跃生效状态
// Check whether the silence period is still actively in effect
function isSilenceActive(value?: string | null): boolean {
  if (!value) {
    return false;
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return false;
  }
  return parsed.getTime() > Date.now();
}

// 计算告警最近发生变更的时间
// Calculate the latest timestamp of alert state change
function resolveLastChangedAt(alert: AlertInstance): string | null {
  return alert.closed_at || alert.resolved_at || alert.last_seen_at || null;
}

export function MonitoringAlertsCenter() {
  const t = useTranslations('monitoringCenter');
  const tsT = useTranslations('troubleshooting');
  const searchParams = useSearchParams();
  const router = useRouter();

  const [clusterOptions, setClusterOptions] = useState<ClusterOption[]>([]);
  const [clusterFilter, setClusterFilter] = useState<string>('all');
  const [sourceFilter, setSourceFilter] = useState<string>('all');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [searchKeyword, setSearchKeyword] = useState<string>('');
  const [startTimeFilter, setStartTimeFilter] = useState<string>('');
  const [endTimeFilter, setEndTimeFilter] = useState<string>('');
  // 当前选中的时间快捷预设（单位：分钟，为 null 表示自定义或未选）
  // Currently selected quick time preset (in minutes, null indicates custom or unset)
  const [activePreset, setActivePreset] = useState<number | null>(null);
  const [page, setPage] = useState<number>(1);
  const [pageSize, setPageSize] = useState<string>('50');

  const [alerts, setAlerts] = useState<AlertInstance[]>([]);
  const [stats, setStats] = useState<AlertInstanceStats>(EMPTY_STATS);
  const [total, setTotal] = useState<number>(0);
  const [loading, setLoading] = useState<boolean>(true);
  const [actingAlertId, setActingAlertId] = useState<string | null>(null);

  // 当前在侧边抽屉中查看详情的告警实例
  // Currently selected alert instance in the detail slide-over sheet
  const [selectedAlert, setSelectedAlert] = useState<AlertInstance | null>(null);
  // 抽屉里的告警不进入 loadAlerts 依赖，避免打开后请求死循环
  // Keep the open alert out of loadAlerts deps so opening the sheet cannot refetch forever
  const selectedAlertRef = useRef<AlertInstance | null>(null);
  selectedAlertRef.current = selectedAlert;

  // 排障经验记忆库弹窗状态与刷新版本号
  // Troubleshooting memory dialog open state and refresh version trigger
  const [memoryDialogOpen, setMemoryDialogOpen] = useState(false);
  const [memoriesVersion, setMemoriesVersion] = useState(0);

  const pageSizeNumber = useMemo(
    () => Number.parseInt(pageSize, 10) || 50,
    [pageSize],
  );

  // 加载集群列表选项
  // Fetch available cluster options for the filter dropdown
  const loadClusters = useCallback(async () => {
    const healthResult = await services.monitoring.getClustersHealthSafe();
    if (healthResult.success && healthResult.data) {
      const options = (healthResult.data.clusters || [])
        .map((cluster) => ({
          id: String(cluster.cluster_id),
          name: cluster.cluster_name || `Cluster-${cluster.cluster_id}`,
        }))
        .sort((a, b) => Number.parseInt(a.id, 10) - Number.parseInt(b.id, 10));
      setClusterOptions(options);
      return;
    }

    try {
      const data = await services.cluster.getClusters({
        current: 1,
        size: 100,
      });
      setClusterOptions(
        (data.clusters || []).map((cluster) => ({
          id: String(cluster.id),
          name: cluster.name,
        })),
      );
    } catch {
      setClusterOptions([]);
    }
  }, []);

  // 加载告警列表数据
  // Fetch alert instances list with current filters
  const loadAlerts = useCallback(async () => {
    setLoading(true);
    try {
      const result = await services.monitoring.getAlertInstancesSafe({
        cluster_id: clusterFilter === 'all' ? undefined : clusterFilter,
        source_type:
          sourceFilter === 'all'
            ? undefined
            : (sourceFilter as AlertSourceType),
        status:
          statusFilter === 'all'
            ? undefined
            : (statusFilter as AlertDisplayStatus),
        start_time: toRFC3339(startTimeFilter),
        end_time: toRFC3339(endTimeFilter),
        page,
        page_size: pageSizeNumber,
      });

      if (!result.success || !result.data) {
        toast.error(result.error || t('alerts.loadError'));
        setAlerts([]);
        setStats(EMPTY_STATS);
        setTotal(0);
        return;
      }

      setAlerts(result.data.alerts || []);
      setStats(result.data.stats || EMPTY_STATS);
      setTotal(result.data.total || 0);

      // 只刷新仍打开的那条，且用户已关闭时不要再写回去
      // Refresh only the alert still open; do not reopen after the user closed it
      const current = selectedAlertRef.current;
      if (current) {
        const updated = (result.data.alerts || []).find(
          (item) => item.alert_id === current.alert_id,
        );
        if (updated) {
          setSelectedAlert((prev) =>
            prev && prev.alert_id === updated.alert_id ? updated : prev,
          );
        }
      }
    } finally {
      setLoading(false);
    }
  }, [
    clusterFilter,
    endTimeFilter,
    page,
    pageSizeNumber,
    sourceFilter,
    startTimeFilter,
    statusFilter,
    t,
  ]);

  useEffect(() => {
    loadClusters();
  }, [loadClusters]);

  useEffect(() => {
    const clusterIDFromQuery = searchParams.get('cluster_id');
    if (!clusterIDFromQuery) {
      return;
    }
    setClusterFilter(clusterIDFromQuery);
    setPage(1);
  }, [searchParams]);

  useEffect(() => {
    loadAlerts();
  }, [loadAlerts]);

  // 根据关键字在前端进一步过滤高亮展示
  // Filter alerts by search keyword on name, summary, and rule_key
  const filteredAlerts = useMemo(() => {
    const kw = searchKeyword.trim().toLowerCase();
    if (!kw) {
      return alerts;
    }
    return alerts.filter((item) => {
      const name = (item.alert_name || '').toLowerCase();
      const summary = (item.summary || '').toLowerCase();
      const rule = (item.rule_key || '').toLowerCase();
      const cluster = (item.cluster_name || '').toLowerCase();
      return (
        name.includes(kw) ||
        summary.includes(kw) ||
        rule.includes(kw) ||
        cluster.includes(kw)
      );
    });
  }, [alerts, searchKeyword]);

  // 重置所有筛选条件
  // Reset all filters back to defaults
  const handleResetFilters = useCallback(() => {
    setClusterFilter('all');
    setSourceFilter('all');
    setStatusFilter('all');
    setSearchKeyword('');
    setStartTimeFilter('');
    setEndTimeFilter('');
    setActivePreset(null);
    setPage(1);
  }, []);

  // 应用时间快捷预设（分钟）或清除时间过滤
  // Apply quick time preset (in minutes) or clear time filter
  const handleApplyTimePreset = useCallback((minutes: number | null) => {
    setActivePreset(minutes);
    if (minutes === null) {
      setStartTimeFilter('');
      setEndTimeFilter('');
      setPage(1);
      return;
    }
    const now = new Date();
    const start = new Date(now.getTime() - minutes * 60 * 1000);
    setStartTimeFilter(formatToDateTimeLocal(start));
    setEndTimeFilter(formatToDateTimeLocal(now));
    setPage(1);
  }, []);

  const resolveSourceLabel = useCallback(
    (sourceType: AlertSourceType) => {
      if (sourceType === 'local_process_event') {
        return t('alerts.sourceTypes.local_process_event');
      }
      if (sourceType === 'remote_alertmanager') {
        return t('alerts.sourceTypes.remote_alertmanager');
      }
      return sourceType;
    },
    [t],
  );

  const resolveStatusLabel = useCallback(
    (status: AlertDisplayStatus) => {
      if (status === 'resolved') {
        return t('alerts.statuses.resolved');
      }
      if (status === 'closed') {
        return t('alerts.statuses.closed');
      }
      return t('alerts.statuses.firing');
    },
    [t],
  );

  const resolveSeverityLabel = useCallback(
    (severity: AlertSeverity | string) => {
      const normalized = String(severity || '')
        .trim()
        .toLowerCase();
      if (normalized === 'critical') {
        return t('alertSeverity.critical');
      }
      if (normalized === 'warning') {
        return t('alertSeverity.warning');
      }
      return severity || '-';
    },
    [t],
  );

  const totalPages = useMemo(() => {
    if (total <= 0) {
      return 1;
    }
    return Math.max(1, Math.ceil(total / pageSizeNumber));
  }, [pageSizeNumber, total]);

  // 检索选中告警命中的历史排障经验
  // Match historical troubleshooting solutions for selected alert
  const matchedMemories = useMemo(() => {
    if (!selectedAlert) {
      return [];
    }
    return services.troubleshooting.findMatchingMemories({
      fingerprint: selectedAlert.rule_key || selectedAlert.alert_name,
      title: selectedAlert.alert_name || selectedAlert.rule_key,
      target_type: 'alert',
    });
  }, [selectedAlert, memoriesVersion]);

  const primaryMemory = matchedMemories[0] || null;

  // 映射当前页告警是否有命中方案
  // Map whether alert instances in current list have matched solutions
  const alertMemoryMap = useMemo(() => {
    const map = new Map<string, boolean>();
    alerts.forEach((alert) => {
      const matches = services.troubleshooting.findMatchingMemories({
        fingerprint: alert.rule_key || alert.alert_name,
        title: alert.alert_name || alert.rule_key,
        target_type: 'alert',
      });
      if (matches.length > 0) {
        map.set(alert.alert_id, true);
      }
    });
    return map;
  }, [alerts, memoriesVersion]);

  // 静默单条告警 30 分钟
  // Silence single alert instance for 30 minutes
  const handleSilence = async (alert: AlertInstance) => {
    setActingAlertId(alert.alert_id);
    try {
      const result = await services.monitoring.silenceAlertInstanceSafe(
        alert.alert_id,
        {duration_minutes: 30},
      );
      if (!result.success) {
        toast.error(result.error || t('alerts.silenceError'));
        return;
      }
      toast.success(t('alerts.silenceSuccess'));
      await loadAlerts();
    } finally {
      setActingAlertId(null);
    }
  };

  // 手动关闭告警
  // Manually close an alert instance
  const handleClose = async (alert: AlertInstance) => {
    setActingAlertId(alert.alert_id);
    try {
      const result = await services.monitoring.closeAlertInstanceSafe(
        alert.alert_id,
      );
      if (!result.success) {
        toast.error(result.error || t('alerts.closeError'));
        return;
      }
      toast.success(t('alerts.closeSuccess'));
      await loadAlerts();
    } finally {
      setActingAlertId(null);
    }
  };

  // 跳转到诊断中心进行排查
  // Jump directly to Diagnostics Center with pre-filled context
  const handleNavigateToDiagnostics = (alert: AlertInstance) => {
    const params = new URLSearchParams();
    if (alert.cluster_id) {
      params.set('cluster_id', alert.cluster_id);
    }
    params.set('source', 'alerts');
    params.set('alert_id', alert.alert_id);
    router.push(`/diagnostics?${params.toString()}`);
  };

  // 渲染严重级别微章
  // Render stylized severity badge with icon
  const renderSeverityBadge = (severity: string) => {
    const normalized = severity.trim().toLowerCase();
    if (normalized === 'critical') {
      return (
        <span className='inline-flex items-center gap-1.5 rounded-md bg-rose-500/10 px-2 py-0.5 text-xs font-semibold text-rose-600 dark:text-rose-400 border border-rose-500/20'>
          <AlertCircle className='h-3.5 w-3.5' />
          {resolveSeverityLabel(severity)}
        </span>
      );
    }
    if (normalized === 'warning') {
      return (
        <span className='inline-flex items-center gap-1.5 rounded-md bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-600 dark:text-amber-400 border border-amber-500/20'>
          <AlertTriangle className='h-3.5 w-3.5' />
          {resolveSeverityLabel(severity)}
        </span>
      );
    }
    return (
      <span className='inline-flex items-center gap-1.5 rounded-md bg-blue-500/10 px-2 py-0.5 text-xs font-semibold text-blue-600 dark:text-blue-400 border border-blue-500/20'>
        <Info className='h-3.5 w-3.5' />
        {resolveSeverityLabel(severity)}
      </span>
    );
  };

  // 渲染状态微章
  // Render status badge with visual indicator
  const renderStatusBadge = (status: AlertDisplayStatus) => {
    if (status === 'firing') {
      return (
        <span className='inline-flex items-center gap-1.5 rounded-full bg-rose-500/10 px-2.5 py-0.5 text-xs font-semibold text-rose-600 dark:text-rose-400 border border-rose-500/20'>
          <span className='relative flex h-2 w-2'>
            <span className='animate-ping absolute inline-flex h-full w-full rounded-full bg-rose-400 opacity-75' />
            <span className='relative inline-flex rounded-full h-2 w-2 bg-rose-500' />
          </span>
          {resolveStatusLabel(status)}
        </span>
      );
    }
    if (status === 'resolved') {
      return (
        <span className='inline-flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2.5 py-0.5 text-xs font-semibold text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'>
          <CheckCircle2 className='h-3.5 w-3.5' />
          {resolveStatusLabel(status)}
        </span>
      );
    }
    return (
      <span className='inline-flex items-center gap-1.5 rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground border border-border'>
        <Archive className='h-3.5 w-3.5' />
        {resolveStatusLabel(status)}
      </span>
    );
  };

  // 渲染静音或备注标签
  // Render markers for silenced duration, note, and acknowledgment
  const renderMarkers = (alert: AlertInstance) => {
    const silenceActive = isSilenceActive(alert.silenced_until);
    const hasAcknowledged = Boolean(alert.acknowledged_at);
    const hasNote = Boolean(alert.latest_note?.trim());

    if (!silenceActive && !hasAcknowledged && !hasNote) {
      return null;
    }

    return (
      <div className='mt-1.5 flex flex-wrap gap-1.5'>
        {hasAcknowledged ? (
          <Badge
            variant='secondary'
            className='text-[11px] py-0 px-1.5 font-normal'
          >
            {t('alerts.markers.acknowledged')}
          </Badge>
        ) : null}
        {silenceActive ? (
          <Badge
            variant='outline'
            className='text-[11px] py-0 px-1.5 text-amber-600 dark:text-amber-400 border-amber-500/30'
          >
            <VolumeX className='mr-1 h-3 w-3 inline' />
            {t('alerts.markers.silencedUntil', {
              time: formatDateTime(alert.silenced_until),
            })}
          </Badge>
        ) : null}
        {hasNote ? (
          <Badge
            variant='outline'
            className='text-[11px] py-0 px-1.5 max-w-[240px] truncate'
            title={alert.latest_note || ''}
          >
            {t('alerts.markers.latestNote', {note: alert.latest_note || ''})}
          </Badge>
        ) : null}
      </div>
    );
  };

  // 状态指标胶囊项定义
  // Status pill items definition
  const pillItems: StatPillItem[] = useMemo(
    () => [
      {
        key: 'all',
        label: t('alerts.allStatuses'),
        count: total,
        icon: <Bell className='h-3.5 w-3.5' />,
      },
      {
        key: 'firing',
        label: t('alerts.firingCount'),
        count: stats.firing,
        variant: 'danger',
        pulse: stats.firing > 0,
      },
      {
        key: 'resolved',
        label: t('alerts.resolvedCount'),
        count: stats.resolved,
        variant: 'success',
        icon: (
          <CheckCircle2 className='h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400' />
        ),
      },
      {
        key: 'closed',
        label: t('alerts.closedCount'),
        count: stats.closed,
        variant: 'default',
        icon: <Archive className='h-3.5 w-3.5 text-muted-foreground' />,
      },
    ],
    [stats, total, t],
  );

  // 容器引用与 GSAP 动效绑定
  // Container refs for GSAP animation scopes
  const containerRef = useRef<HTMLDivElement>(null);
  const sheetContentRef = useRef<HTMLDivElement>(null);

  // GSAP 辅助动效：表格数据行交错入场
  // GSAP auxiliary animation: table rows smooth stagger entrance
  useGSAP(
    () => {
      if (loading || filteredAlerts.length === 0) {
        return;
      }
      animateTableRows('.alert-row-animate');
    },
    {scope: containerRef, dependencies: [filteredAlerts, loading]},
  );

  // GSAP 辅助动效：详情抽屉各区块平滑滑入
  // GSAP auxiliary animation: smooth slide-in for sheet sections
  useGSAP(
    () => {
      if (!selectedAlert) {
        return;
      }
      animateSheetSections('.sheet-section-animate');
    },
    {scope: sheetContentRef, dependencies: [selectedAlert?.alert_id]},
  );

  return (
    <div ref={containerRef} className='flex-1 flex flex-col'>
      {/* 紧凑型一体化工作台卡片 / Unified Compact Alert Workspace Container */}
      <Card className='border-border/70 shadow-xs overflow-hidden flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-270px)]'>
        {/* 顶部工具与状态分段栏 / Top Toolbar & Segmented Status Filter Bar */}
        <div className='p-3 sm:p-3.5 border-b bg-muted/20 space-y-2.5'>
          {/* 第一行：状态胶囊切换器 + 统计徽章 + 右侧操作 */}
          {/* Row 1: Status pills segmented controller + Quick actions */}
          <StatPillsBar
            items={pillItems}
            activeKey={statusFilter}
            onChange={(key: string) => {
              setStatusFilter(key as AlertDisplayStatus | 'all');
              setPage(1);
            }}
            actions={
              <div className='flex items-center gap-1.5'>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={handleResetFilters}
                  className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground cursor-pointer'
                >
                  <RotateCcw className='mr-1 h-3 w-3' />
                  {t('alerts.reset')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={loadAlerts}
                  disabled={loading}
                  className='h-7 px-2.5 text-xs cursor-pointer'
                >
                  <RefreshCw
                    className={cn('mr-1.5 h-3 w-3', loading && 'animate-spin')}
                  />
                  {t('refresh')}
                </Button>
              </div>
            }
          />

          {/* 第二行：高密度搜索与条件组合筛选 */}
          {/* Row 2: High density search and multi-dimensional filters */}
          <div className='flex flex-wrap items-center gap-2 pt-0.5'>
            {/* 关键字搜索 / Keyword Search */}
            <div className='relative flex-1 min-w-[220px]'>
              <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground' />
              <Input
                value={searchKeyword}
                onChange={(e) => setSearchKeyword(e.target.value)}
                placeholder={t('alerts.searchPlaceholder')}
                className='pl-8 pr-7 h-8 text-xs bg-background'
              />
              {searchKeyword && (
                <button
                  type='button'
                  onClick={() => setSearchKeyword('')}
                  className='absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
                >
                  <X className='h-3.5 w-3.5' />
                </button>
              )}
            </div>

            {/* 集群筛选 / Cluster Filter */}
            <Select
              value={clusterFilter}
              onValueChange={(val) => {
                setClusterFilter(val);
                setPage(1);
              }}
            >
              <SelectTrigger className='h-8 text-xs w-[145px] bg-background'>
                <SelectValue placeholder={t('alerts.clusterFilter')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('alerts.allClusters')}</SelectItem>
                {clusterOptions.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* 来源筛选 / Source Filter */}
            <Select
              value={sourceFilter}
              onValueChange={(val) => {
                setSourceFilter(val);
                setPage(1);
              }}
            >
              <SelectTrigger className='h-8 text-xs w-[135px] bg-background'>
                <SelectValue placeholder={t('alerts.sourceFilter')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('alerts.allSources')}</SelectItem>
                <SelectItem value='local_process_event'>
                  {t('alerts.sourceTypes.local_process_event')}
                </SelectItem>
                <SelectItem value='remote_alertmanager'>
                  {t('alerts.sourceTypes.remote_alertmanager')}
                </SelectItem>
              </SelectContent>
            </Select>

          </div>

          {/* 第三行：时间范围快速预设与自定义起止时间 */}
          {/* Row 3: Quick time presets and labeled datetime-local inputs */}
          <CompactTimeFilter
            startTime={startTimeFilter}
            endTime={endTimeFilter}
            activePreset={activePreset}
            onStartTimeChange={(val) => {
              setActivePreset(null);
              setStartTimeFilter(val);
              setPage(1);
            }}
            onEndTimeChange={(val) => {
              setActivePreset(null);
              setEndTimeFilter(val);
              setPage(1);
            }}
            onApplyPreset={handleApplyTimePreset}
            onClear={() => {
              setStartTimeFilter('');
              setEndTimeFilter('');
              setActivePreset(null);
              setPage(1);
            }}
          />
        </div>

        {/* 3. 告警列表表格（高密度排版） / Alert Instances High Density Table */}
        <TableLoadingBar loading={loading && filteredAlerts.length > 0} />
        <div className='overflow-x-auto flex-1'>
          <Table>
            <TableHeader>
              <TableRow className='bg-muted/30 hover:bg-muted/30 h-8'>
                <TableHead className='w-[120px] py-1.5 px-3 text-xs'>{t('alerts.cluster')}</TableHead>
                <TableHead className='w-[130px] py-1.5 px-3 text-xs'>{t('alerts.sourceType')}</TableHead>
                <TableHead className='min-w-[170px] py-1.5 px-3 text-xs'>{t('alerts.alertName')}</TableHead>
                <TableHead className='w-[90px] py-1.5 px-3 text-xs'>{t('alerts.severity')}</TableHead>
                <TableHead className='w-[95px] py-1.5 px-3 text-xs'>{t('alerts.status')}</TableHead>
                <TableHead className='min-w-[260px] py-1.5 px-3 text-xs'>{t('alerts.summary')}</TableHead>
                <TableHead className='w-[140px] py-1.5 px-3 text-xs whitespace-nowrap'>{t('alerts.firstFiredAt')}</TableHead>
                <TableHead className='w-[140px] py-1.5 px-3 text-xs whitespace-nowrap'>{t('alerts.lastChangedAt')}</TableHead>
                <TableHead className='w-[130px] py-1.5 px-3 text-xs text-right'>{t('actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && filteredAlerts.length === 0 ? (
                <TableSkeletonRows columns={9} rows={6} />
              ) : filteredAlerts.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={9}
                    className='h-40 text-center text-muted-foreground'
                  >
                    <div className='flex flex-col items-center justify-center gap-2'>
                      <ShieldAlert className='h-8 w-8 text-muted-foreground/50' />
                      <span className='font-medium'>{t('alerts.noAlerts')}</span>
                      <span className='text-xs text-muted-foreground'>
                        {t('alerts.noMatchingAlerts')}
                      </span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : (
                filteredAlerts.map((alert) => {
                  const busy = actingAlertId === alert.alert_id;
                  const silenceActive = isSilenceActive(alert.silenced_until);
                  const canSilence =
                    alert.status === 'firing' && !silenceActive;
                  // 已恢复的事件条件已结束，不再要求人工关闭
                  // Recovered incidents are done; do not ask for a manual close
                  const canClose = alert.status === 'firing';

                  return (
                    <TableRow
                      key={alert.alert_id}
                      className={cn(
                        'alert-row-animate cursor-pointer transition-all group',
                        loading && 'opacity-50 pointer-events-none',
                        alert.status === 'firing' &&
                          'hover:bg-rose-500/[0.03] border-l-2 border-l-rose-500',
                        selectedAlert?.alert_id === alert.alert_id &&
                          'bg-muted/60 font-medium',
                      )}
                      onClick={() => setSelectedAlert(alert)}
                    >
                        {/* 集群 / Cluster */}
                        <TableCell className='py-2 px-3 font-medium text-xs'>
                          <span
                            className='truncate max-w-[110px] inline-block'
                            title={alert.cluster_name || alert.cluster_id || '-'}
                          >
                            {alert.cluster_name || alert.cluster_id || '-'}
                          </span>
                        </TableCell>

                        {/* 来源 / Source */}
                        <TableCell className='py-2 px-3'>
                          <Badge variant='outline' className='text-[10px] py-0 px-1 font-normal'>
                            {resolveSourceLabel(alert.source_type)}
                          </Badge>
                        </TableCell>

                        {/* 告警名称与规则 / Alert Name & Rule */}
                        <TableCell className='py-2 px-3'>
                          <div className='space-y-0.5'>
                            <div className='flex items-center gap-1.5'>
                              <span className='font-semibold text-xs text-foreground line-clamp-1 group-hover:text-primary transition-colors'>
                                {alert.alert_name || '-'}
                              </span>
                              {alertMemoryMap.get(alert.alert_id) && (
                                <Badge
                                  variant='outline'
                                  className='text-[10px] py-0 px-1 border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 gap-0.5 shrink-0 font-normal'
                                >
                                  <Lightbulb className='h-2.5 w-2.5 text-emerald-500' />
                                  已有方案
                                </Badge>
                              )}
                            </div>
                            <div className='text-[10px] font-mono text-muted-foreground truncate max-w-[160px]'>
                              {alert.rule_key}
                            </div>
                          </div>
                        </TableCell>

                        {/* 严重级别 / Severity */}
                        <TableCell className='py-2 px-3'>{renderSeverityBadge(alert.severity)}</TableCell>

                        {/* 状态 / Status */}
                        <TableCell className='py-2 px-3'>{renderStatusBadge(alert.status)}</TableCell>

                        {/* 摘要与标记 / Summary & Markers */}
                        <TableCell className='max-w-[280px] py-2 px-3'>
                          <div
                            className='line-clamp-1 text-xs text-foreground/90'
                            title={alert.summary || alert.description || ''}
                          >
                            {alert.summary || alert.description || '-'}
                          </div>
                          {renderMarkers(alert)}
                        </TableCell>

                        {/* 首次触发 / First Fired At */}
                        <TableCell className='py-2 px-3 text-xs font-mono text-muted-foreground whitespace-nowrap'>
                          {formatDateTime(alert.firing_at)}
                        </TableCell>

                        {/* 最近变更 / Last Changed At */}
                        <TableCell className='py-2 px-3 text-xs font-mono text-muted-foreground whitespace-nowrap'>
                          {formatDateTime(resolveLastChangedAt(alert))}
                        </TableCell>

                        {/* 操作栏 / Action Bar */}
                        <TableCell className='py-2 px-3 text-right whitespace-nowrap'>
                          <div
                            className='flex items-center justify-end gap-1'
                            onClick={(e) => e.stopPropagation()}
                          >
                            {/* 查看详情 / View Details */}
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  size='icon'
                                  variant='ghost'
                                  className='h-6.5 w-6.5'
                                  onClick={() => setSelectedAlert(alert)}
                                >
                                  <Eye className='h-3.5 w-3.5' />
                                </Button>
                              </TooltipTrigger>
                              <TooltipContent>{t('viewDetails')}</TooltipContent>
                            </Tooltip>

                            {/* 静音 30m / Silence 30m */}
                            {canSilence && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    size='sm'
                                    variant='outline'
                                    className='h-6.5 px-2 text-[11px]'
                                    disabled={busy}
                                    onClick={() => handleSilence(alert)}
                                  >
                                    <VolumeX className='mr-1 h-3 w-3' />
                                    {t('alerts.silence30m')}
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  {t('alerts.actionHints.silenceReady')}
                                </TooltipContent>
                              </Tooltip>
                            )}

                            {/* 关闭告警 / Close Alert */}
                            {canClose && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    size='sm'
                                    variant='ghost'
                                    className='h-6.5 px-1.5 text-[11px] text-muted-foreground hover:text-foreground'
                                    disabled={busy}
                                    onClick={() => handleClose(alert)}
                                  >
                                    <XCircle className='mr-1 h-3 w-3' />
                                    {t('alerts.close')}
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  {t('alerts.actionHints.closeReady')}
                                </TooltipContent>
                              </Tooltip>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </div>

          {/* 底部分页栏 / Table Footer Pagination */}
          <div className='border-t bg-muted/10 px-4 py-2.5 mt-auto'>
            <Pagination
              currentPage={page}
              totalPages={totalPages}
              pageSize={pageSizeNumber}
              totalItems={total}
              onPageChange={setPage}
              onPageSizeChange={(newSize) => {
                setPageSize(String(newSize));
                setPage(1);
              }}
              showPageSizeSelector={true}
              pageSizeOptions={[20, 50, 100, 200]}
            />
          </div>
      </Card>

      {/* 4. 告警详情滑出抽屉 (Slide-over Sheet) / Alert Details Slide-over Sheet */}
      <Sheet
        open={Boolean(selectedAlert)}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedAlert(null);
          }
        }}
      >
        <SheetContent className='w-full sm:max-w-xl p-0 flex flex-col'>
          <SheetHeader className='p-6 pb-4 border-b space-y-2.5 bg-muted/15'>
            <div className='flex items-center justify-between gap-3'>
              <div className='flex items-center gap-2'>
                {selectedAlert && renderSeverityBadge(selectedAlert.severity)}
                {selectedAlert && renderStatusBadge(selectedAlert.status)}
              </div>
              <Badge variant='outline' className='font-mono text-xs'>
                {selectedAlert?.alert_id}
              </Badge>
            </div>
            <SheetTitle className='text-lg font-bold text-foreground text-left leading-snug'>
              {selectedAlert?.alert_name || t('alerts.detailsTitle')}
            </SheetTitle>
            <SheetDescription className='text-xs text-muted-foreground text-left'>
              {t('alerts.ruleKey')}: <span className='font-mono font-medium'>{selectedAlert?.rule_key}</span>
            </SheetDescription>
          </SheetHeader>

          {/* 抽屉正文内容 / Sheet Body */}
          <div ref={sheetContentRef} className='flex-1 overflow-y-auto p-6 space-y-6'>
            {selectedAlert && (
              <>
                {/* 历史排障经验置顶回显卡片 / Historical Troubleshooting Solution Card */}
                <div className='sheet-section-animate'>
                  <TroubleshootingMemoryCard
                    matchedMemory={primaryMemory}
                    totalMatches={matchedMemories.length}
                    onAddOrEdit={() => setMemoryDialogOpen(true)}
                  />
                </div>

                {/* 核心概要 / Summary */}
                <div className='sheet-section-animate space-y-2'>
                  <h4 className='text-xs font-semibold text-muted-foreground uppercase tracking-wider'>
                    {t('alerts.summary')}
                  </h4>
                  <div className='rounded-lg bg-muted/40 p-3.5 text-sm leading-relaxed border'>
                    {selectedAlert.summary || selectedAlert.description || '-'}
                  </div>
                </div>

                {/* 描述信息 (若与概要不同) / Description */}
                {selectedAlert.description &&
                  selectedAlert.description !== selectedAlert.summary && (
                    <div className='sheet-section-animate space-y-2'>
                      <h4 className='text-xs font-semibold text-muted-foreground uppercase tracking-wider'>
                        {t('alerts.detailedDescription')}
                      </h4>
                      {/* 长 JSON/标签串无空格，须 wrap+break-all，否则单行溢出裁切。 */}
                      {/* Long JSON/label blobs need wrap+break-all or they clip as one line. */}
                      <ExpandableTextPanel
                        content={selectedAlert.description}
                        height={220}
                        tone='plain'
                        wrap
                      />
                    </div>
                  )}

                {/* 告警时间线 / Timeline */}
                <div className='sheet-section-animate space-y-3'>
                  <h4 className='text-xs font-semibold text-muted-foreground uppercase tracking-wider'>
                    {t('alerts.eventTimeline')}
                  </h4>
                  <div className='grid grid-cols-2 gap-3 text-xs'>
                    <div className='rounded-md border p-2.5 space-y-1 bg-background'>
                      <span className='text-muted-foreground'>{t('alerts.firstFiredAt')}</span>
                      <div className='font-mono font-medium'>{formatDateTime(selectedAlert.firing_at)}</div>
                    </div>
                    <div className='rounded-md border p-2.5 space-y-1 bg-background'>
                      <span className='text-muted-foreground'>{t('alerts.lastChangedAt')}</span>
                      <div className='font-mono font-medium'>{formatDateTime(resolveLastChangedAt(selectedAlert))}</div>
                    </div>
                    {selectedAlert.resolved_at && (
                      <div className='rounded-md border p-2.5 space-y-1 bg-emerald-500/5 border-emerald-500/20'>
                        <span className='text-emerald-600 dark:text-emerald-400'>{t('alerts.resolvedTime')}</span>
                        <div className='font-mono font-medium text-emerald-700 dark:text-emerald-300'>
                          {formatDateTime(selectedAlert.resolved_at)}
                        </div>
                      </div>
                    )}
                    {selectedAlert.closed_at && (
                      <div className='rounded-md border p-2.5 space-y-1 bg-muted/30'>
                        <span className='text-muted-foreground'>{t('alerts.closedTime')}</span>
                        <div className='font-mono font-medium'>{formatDateTime(selectedAlert.closed_at)}</div>
                      </div>
                    )}
                  </div>
                </div>

                {/* 所属环境元数据 / Context & Metadata */}
                <div className='sheet-section-animate space-y-3'>
                  <h4 className='text-xs font-semibold text-muted-foreground uppercase tracking-wider'>
                    {t('alerts.contextAndTopology')}
                  </h4>
                  <div className='rounded-lg border divide-y text-xs'>
                    <div className='flex items-center justify-between p-3'>
                      <span className='text-muted-foreground'>{t('alerts.belongCluster')}</span>
                      <span className='font-medium'>{selectedAlert.cluster_name || selectedAlert.cluster_id || '-'}</span>
                    </div>
                    <div className='flex items-center justify-between p-3'>
                      <span className='text-muted-foreground'>{t('alerts.alertSource')}</span>
                      <span>{resolveSourceLabel(selectedAlert.source_type)}</span>
                    </div>
                    {selectedAlert.source_ref && (
                      <>
                        {selectedAlert.source_ref.hostname ? (
                          <div className='flex items-center justify-between p-3'>
                            <span className='text-muted-foreground'>{t('alerts.affectedHost')}</span>
                            <span className='font-mono font-medium'>{selectedAlert.source_ref.hostname}</span>
                          </div>
                        ) : null}
                        {selectedAlert.source_ref.process_name ? (
                          <div className='flex items-center justify-between p-3'>
                            <span className='text-muted-foreground'>{t('alerts.associatedProcess')}</span>
                            <span className='font-mono font-medium'>{selectedAlert.source_ref.process_name}</span>
                          </div>
                        ) : null}
                        {selectedAlert.source_ref.event_id ? (
                          <div className='flex items-center justify-between p-3'>
                            <span className='text-muted-foreground'>{t('alerts.associatedEventId')}</span>
                            <span className='font-mono font-medium'>#{selectedAlert.source_ref.event_id}</span>
                          </div>
                        ) : null}
                      </>
                    )}
                  </div>
                </div>

                {/* 备注与处理记录 / Notes & Acknowledgments */}
                {(selectedAlert.latest_note || selectedAlert.silenced_until || selectedAlert.acknowledged_at) && (
                  <div className='sheet-section-animate space-y-3'>
                    <h4 className='text-xs font-semibold text-muted-foreground uppercase tracking-wider'>
                      {t('alerts.handlingNotes')}
                    </h4>
                    <div className='rounded-lg border p-3.5 space-y-2 text-xs bg-muted/20'>
                      {selectedAlert.silenced_until && (
                        <div className='flex items-center gap-2'>
                          <span className='text-muted-foreground'>{t('alerts.silencedUntilLabel')}:</span>
                          <span className='font-mono font-medium'>{formatDateTime(selectedAlert.silenced_until)}</span>
                        </div>
                      )}
                      {selectedAlert.acknowledged_at && (
                        <div className='flex items-center gap-2'>
                          <span className='text-muted-foreground'>{t('alerts.acknowledgedTimeLabel')}:</span>
                          <span className='font-mono font-medium'>{formatDateTime(selectedAlert.acknowledged_at)}</span>
                        </div>
                      )}
                      {selectedAlert.latest_note && (
                        <div className='space-y-1 pt-1 border-t'>
                          <span className='text-muted-foreground'>{t('alerts.latestNoteLabel')}:</span>
                          <p className='text-foreground'>{selectedAlert.latest_note}</p>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </>
            )}
          </div>

          {/* 抽屉底部操作栏 / Sheet Footer Actions */}
          {selectedAlert && (
            <div className='p-4 border-t bg-muted/20 flex flex-wrap items-center justify-between gap-2.5'>
              {/* 联动跳转到诊断中心排查 / Deep link to Diagnostics */}
              <Button
                variant='default'
                size='sm'
                onClick={() => handleNavigateToDiagnostics(selectedAlert)}
                className='h-9 font-medium shadow-xs'
              >
                <Activity className='mr-1.5 h-4 w-4' />
                {t('alerts.navigateToDiagnostics')}
                <ExternalLink className='ml-1.5 h-3.5 w-3.5 opacity-70' />
              </Button>

              <div className='flex items-center gap-2'>
                {/* 沉淀/更新经验库方案 / Record or update playbook */}
                <Button
                  variant='outline'
                  size='sm'
                  className='h-9 text-xs gap-1 border-emerald-500/40 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/10'
                  onClick={() => setMemoryDialogOpen(true)}
                >
                  <Lightbulb className='h-3.5 w-3.5 text-emerald-500' />
                  {primaryMemory ? tsT('updateSolution') : tsT('recordSolution')}
                </Button>

                {selectedAlert.status === 'firing' &&
                  !isSilenceActive(selectedAlert.silenced_until) && (
                    <Button
                      variant='outline'
                      size='sm'
                      className='h-9 text-xs'
                      disabled={actingAlertId === selectedAlert.alert_id}
                      onClick={() => handleSilence(selectedAlert)}
                    >
                      <VolumeX className='mr-1.5 h-3.5 w-3.5' />
                      {t('alerts.silence30m')}
                    </Button>
                  )}

                {selectedAlert.status === 'firing' && (
                  <Button
                    variant='secondary'
                    size='sm'
                    className='h-9 text-xs'
                    disabled={actingAlertId === selectedAlert.alert_id}
                    onClick={() => handleClose(selectedAlert)}
                  >
                    <XCircle className='mr-1.5 h-3.5 w-3.5' />
                    {t('alerts.manualClose')}
                  </Button>
                )}
              </div>
            </div>
          )}
        </SheetContent>
      </Sheet>

      {/* 沉淀排障解决方案弹窗 / Save troubleshooting memory dialog */}
      <SaveMemoryDialog
        open={memoryDialogOpen}
        onOpenChange={setMemoryDialogOpen}
        initialData={
          selectedAlert
            ? {
                id: primaryMemory?.id,
                target_type: 'alert',
                fingerprint:
                  selectedAlert.rule_key ||
                  selectedAlert.alert_name,
                title:
                  primaryMemory?.title ||
                  `${selectedAlert.alert_name || selectedAlert.rule_key} ${tsT('defaultTitleSuffix')}`,
                error_summary:
                  selectedAlert.summary ||
                  selectedAlert.description,
                root_cause: primaryMemory?.root_cause,
                solution: primaryMemory?.solution || '',
                preventive_tips: primaryMemory?.preventive_tips,
                tags: primaryMemory?.tags || ['alert', selectedAlert.source_type || 'metric'].filter(Boolean),
                cluster_id: selectedAlert.cluster_id,
                cluster_name: selectedAlert.cluster_name,
                author: primaryMemory?.author,
              }
            : null
        }
        onSaved={() => {
          setMemoriesVersion((v) => v + 1);
        }}
      />
    </div>
  );
}
