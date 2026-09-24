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
  ArrowUpRight,
  Check,
  Copy,
  Eye,
  FileCode,
  Lightbulb,
  RefreshCw,
  Search,
  Server,
  X,
} from 'lucide-react';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import services from '@/lib/services';
import type {
  DiagnosticsErrorEvent,
  DiagnosticsErrorGroup,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card} from '@/components/ui/card';
import {Input} from '@/components/ui/input';
import {Pagination} from '@/components/ui/pagination';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Skeleton} from '@/components/ui/skeleton';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
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
import {
  StatPillsBar,
  type StatPillItem,
  TableLoadingBar,
  TableSkeletonRows,
} from '@/components/common/layout';
import {animateTableRows, animateSheetSections} from '@/lib/animations/gsap-motion';
import {
  TroubleshootingMemoryCard,
  SaveMemoryDialog,
} from '@/components/common/troubleshooting';

type DiagnosticsErrorCenterProps = {
  clusterId?: number;
  clusterName?: string;
  groupId?: number;
  onSelectGroup?: (groupId: number | null) => void;
};

type ErrorHeatCategory = 'all' | 'critical' | 'warning' | 'normal';

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

// 获取错误频次热度样式（高频突出警示，中频预警，低频常规）
// Get error occurrence heat styling (high: prominent red, mid: warning amber, low: subtle gray)
function getOccurrenceHeatClass(count: number): string {
  if (count >= 10) {
    return 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border-rose-500/30 font-semibold';
  }
  if (count >= 3) {
    return 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30 font-medium';
  }
  return 'bg-muted text-muted-foreground border-border/80';
}

// 格式化节点来源信息
// Format origin node topology information
function formatNodeOrigin(options: {
  nodeId?: number | null;
  hostId?: number | null;
  hostName?: string | null;
  hostIp?: string | null;
  role?: string | null;
}): string {
  const parts: string[] = [];
  if (options.hostName?.trim()) {
    parts.push(options.hostName.trim());
  } else if (options.hostIp?.trim()) {
    parts.push(options.hostIp.trim());
  } else if (options.hostId) {
    parts.push(`#${options.hostId}`);
  }
  if (options.role?.trim()) {
    parts.push(options.role.trim());
  }
  if (options.nodeId) {
    parts.push(`node #${options.nodeId}`);
  }
  return parts.length > 0 ? parts.join(' · ') : '-';
}

export function DiagnosticsErrorCenter({
  clusterId,
  clusterName,
  groupId,
  onSelectGroup,
}: DiagnosticsErrorCenterProps) {
  const t = useTranslations('diagnosticsCenter');
  const commonT = useTranslations('common');
  const tsT = useTranslations('troubleshooting');

  const [keywordInput, setKeywordInput] = useState('');
  const [keyword, setKeyword] = useState('');
  const [heatFilter, setHeatFilter] = useState<ErrorHeatCategory>('all');
  const [page, setPage] = useState(1);
  const [loadingGroups, setLoadingGroups] = useState(true);
  const [groups, setGroups] = useState<DiagnosticsErrorGroup[]>([]);
  const [groupTotal, setGroupTotal] = useState(0);

  // 抽屉中查看详情的错误组
  // Selected error group open in slide-over sheet
  const [selectedGroupId, setSelectedGroupId] = useState<number | null>(
    groupId ?? null,
  );
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [selectedGroup, setSelectedGroup] = useState<DiagnosticsErrorGroup | null>(
    null,
  );
  const [groupEvents, setGroupEvents] = useState<DiagnosticsErrorEvent[]>([]);
  const [selectedEventId, setSelectedEventId] = useState<number | null>(null);
  const [copiedEvidence, setCopiedEvidence] = useState(false);
  const groupsRequestIdRef = useRef(0);
  const detailRequestIdRef = useRef(0);

  // 排障经验记忆库弹窗状态与刷新版本号
  // Troubleshooting memory dialog open state and refresh version trigger
  const [memoryDialogOpen, setMemoryDialogOpen] = useState(false);
  const [memoriesVersion, setMemoriesVersion] = useState(0);

  // 复制异常堆栈到剪贴板
  // Copy exception evidence stack trace to clipboard
  const handleCopyEvidence = useCallback((text?: string | null) => {
    if (!text) {
      return;
    }
    navigator.clipboard.writeText(text);
    setCopiedEvidence(true);
    toast.success('堆栈信息已复制到剪贴板');
    setTimeout(() => setCopiedEvidence(false), 2000);
  }, []);

  const clearSelectedGroupDetail = useCallback(() => {
    detailRequestIdRef.current += 1;
    setLoadingDetail(false);
    setSelectedGroup(null);
    setGroupEvents([]);
    setSelectedEventId(null);
  }, []);

  // 加载错误组列表
  // Fetch error groups list
  const loadGroups = useCallback(async () => {
    const requestId = groupsRequestIdRef.current + 1;
    groupsRequestIdRef.current = requestId;
    setLoadingGroups(true);
    try {
      const result = await services.diagnostics.getErrorGroupsSafe({
        cluster_id: clusterId,
        keyword: keyword || undefined,
        page,
        page_size: 20,
      });
      if (groupsRequestIdRef.current !== requestId) {
        return;
      }
      if (!result.success || !result.data) {
        toast.error(result.error || t('errors.loadGroupsError'));
        setGroups([]);
        setGroupTotal(0);
        return;
      }
      setGroups(result.data.items || []);
      setGroupTotal(result.data.total || 0);
    } finally {
      if (groupsRequestIdRef.current === requestId) {
        setLoadingGroups(false);
      }
    }
  }, [clusterId, keyword, page, t]);

  // 加载错误组详情与事件样本
  // Fetch error group detail and event evidence samples
  const loadGroupDetail = useCallback(
    async (targetGroupId: number) => {
      const requestId = detailRequestIdRef.current + 1;
      detailRequestIdRef.current = requestId;
      setLoadingDetail(true);
      try {
        const result = await services.diagnostics.getErrorGroupDetailSafe(
          targetGroupId,
          20,
          {
            cluster_id: clusterId,
          },
        );
        if (detailRequestIdRef.current !== requestId) {
          return;
        }
        if (!result.success || !result.data) {
          toast.error(result.error || t('errors.loadDetailError'));
          setSelectedGroup(null);
          setGroupEvents([]);
          setSelectedEventId(null);
          return;
        }
        setSelectedGroup(result.data.group);
        setGroupEvents(result.data.events || []);
        setSelectedEventId(result.data.events?.[0]?.id ?? null);
      } finally {
        if (detailRequestIdRef.current === requestId) {
          setLoadingDetail(false);
        }
      }
    },
    [clusterId, t],
  );

  useEffect(() => {
    void loadGroups();
  }, [loadGroups]);

  // 表格行入场动效
  // Stagger animation for table rows on load
  useEffect(() => {
    if (!loadingGroups && groups.length > 0) {
      animateTableRows('.data-row-animate');
    }
  }, [groups, loadingGroups]);

  // 抽屉内区块滑入动效
  // Stagger animation for sections in detail sheet
  useEffect(() => {
    if (selectedGroup && !loadingDetail) {
      animateSheetSections('.sheet-section-animate');
    }
  }, [loadingDetail, selectedGroup]);

  // 当外部传入 groupId 时自动打开对应错误组详情
  // Open group detail when external groupId prop is updated
  useEffect(() => {
    if (groupId) {
      setSelectedGroupId(groupId);
      void loadGroupDetail(groupId);
    }
  }, [groupId, loadGroupDetail]);

  const handleOpenGroupDetail = useCallback(
    (targetGroup: DiagnosticsErrorGroup) => {
      setSelectedGroupId(targetGroup.id);
      setSelectedGroup(targetGroup);
      onSelectGroup?.(targetGroup.id);
      void loadGroupDetail(targetGroup.id);
    },
    [loadGroupDetail, onSelectGroup],
  );

  const handleCloseDrawer = useCallback(() => {
    setSelectedGroupId(null);
    clearSelectedGroupDetail();
    onSelectGroup?.(null);
  }, [clearSelectedGroupDetail, onSelectGroup]);

  // 计算当前列表内的严重程度统计
  // Compute error severity and occurrence statistics
  const stats = useMemo(() => {
    let critical = 0;
    let warning = 0;
    let normal = 0;
    groups.forEach((g) => {
      if (g.occurrence_count >= 10) {
        critical += 1;
      } else if (g.occurrence_count >= 3) {
        warning += 1;
      } else {
        normal += 1;
      }
    });
    return {
      total: groupTotal,
      critical,
      warning,
      normal,
    };
  }, [groupTotal, groups]);

  // 前端按频次分类筛选
  // Filter groups based on selected occurrence heat level
  const displayedGroups = useMemo(() => {
    if (heatFilter === 'critical') {
      return groups.filter((g) => g.occurrence_count >= 10);
    }
    if (heatFilter === 'warning') {
      return groups.filter(
        (g) => g.occurrence_count >= 3 && g.occurrence_count < 10,
      );
    }
    if (heatFilter === 'normal') {
      return groups.filter((g) => g.occurrence_count < 3);
    }
    return groups;
  }, [groups, heatFilter]);

  const pillItems: StatPillItem[] = useMemo(
    () => [
      {
        key: 'all',
        label: '全部错误组',
        count: stats.total,
      },
      {
        key: 'critical',
        label: '高频严重 (≥10次)',
        count: stats.critical,
        variant: 'danger',
        pulse: stats.critical > 0,
      },
      {
        key: 'warning',
        label: '中频预警 (3-9次)',
        count: stats.warning,
        variant: 'warning',
      },
      {
        key: 'normal',
        label: '低频偶发 (<3次)',
        count: stats.normal,
        variant: 'default',
      },
    ],
    [stats],
  );

  const selectedEvent = useMemo(
    () =>
      groupEvents.find((item) => item.id === selectedEventId) ??
      groupEvents[0],
    [groupEvents, selectedEventId],
  );

  // 检索选中错误组命中的历史排障经验
  // Match historical troubleshooting solutions for selected error group
  const matchedMemories = useMemo(() => {
    if (!selectedGroup) {
      return [];
    }
    return services.troubleshooting.findMatchingMemories({
      fingerprint: selectedGroup.fingerprint,
      exception_class: selectedGroup.exception_class,
      title: selectedGroup.title,
      target_type: 'error',
    });
  }, [selectedGroup, memoriesVersion]);

  const primaryMemory = matchedMemories[0] || null;

  // 映射当前页错误组是否有命中方案
  // Map whether error groups in current list have matched solutions
  const groupMemoryMap = useMemo(() => {
    const map = new Map<number, boolean>();
    displayedGroups.forEach((group) => {
      const matches = services.troubleshooting.findMatchingMemories({
        fingerprint: group.fingerprint,
        exception_class: group.exception_class,
        title: group.title,
        target_type: 'error',
      });
      if (matches.length > 0) {
        map.set(group.id, true);
      }
    });
    return map;
  }, [displayedGroups, memoriesVersion]);

  const totalPages = Math.max(1, Math.ceil(groupTotal / 20));

  return (
    <div className='space-y-3.5'>
      {/* 统一全宽卡片（彻底消除左右不对称问题，释放完整可用空间） */}
      {/* Unified full-width card container (eliminates asymmetry, maximizes horizontal space) */}
      <Card className='border border-border/70 shadow-xs overflow-hidden flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-270px)]'>
        {/* 顶部胶囊栏与搜索筛选工具区 / Top Stat Pills & Filter Toolbar */}
        <div className='p-3 border-b bg-card/60 space-y-2.5'>
          {/* 第一行：状态胶囊分段与刷新操作 */}
          {/* Row 1: Stat pills segments and refresh button */}
          <StatPillsBar
            items={pillItems}
            activeKey={heatFilter}
            onChange={(key) => setHeatFilter(key as ErrorHeatCategory)}
            actions={
              <Button
                variant='outline'
                size='sm'
                onClick={() => void loadGroups()}
                disabled={loadingGroups}
                className='h-7 px-2.5 text-xs'
              >
                <RefreshCw
                  className={cn(
                    'mr-1.5 h-3 w-3',
                    loadingGroups && 'animate-spin',
                  )}
                />
                {commonT('refresh')}
              </Button>
            }
          />

          {/* 第二行：高密度关键字搜索 */}
          {/* Row 2: High density keyword search */}
          <form
            className='flex items-center gap-2 pt-0.5'
            onSubmit={(event) => {
              event.preventDefault();
              setPage(1);
              setKeyword(keywordInput.trim());
            }}
          >
            <div className='relative flex-1 min-w-[220px]'>
              <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground' />
              <Input
                value={keywordInput}
                onChange={(event) => setKeywordInput(event.target.value)}
                placeholder={t('errors.keywordPlaceholder')}
                className='pl-8 pr-7 h-8 text-xs bg-background'
              />
              {keywordInput ? (
                <button
                  type='button'
                  onClick={() => {
                    setKeywordInput('');
                    setKeyword('');
                    setPage(1);
                  }}
                  className='absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
                >
                  <X className='h-3.5 w-3.5' />
                </button>
              ) : null}
            </div>
            <Button type='submit' size='sm' className='h-8 px-3 text-xs'>
              {t('errors.search')}
            </Button>
          </form>
        </div>

        {/* 全宽对称高密度数据表格 / Full-width Symmetric High-Density Data Table */}
        <TableLoadingBar loading={loadingGroups && displayedGroups.length > 0} />
        <div className='overflow-x-auto flex-1'>
          <Table>
            <TableHeader>
              <TableRow className='bg-muted/30 hover:bg-muted/30 h-8'>
                <TableHead className='min-w-[280px] py-1.5 px-3 text-xs'>
                  {t('errors.columns.group')}
                </TableHead>
                <TableHead className='w-[220px] py-1.5 px-3 text-xs'>
                  {t('errors.columns.exception')}
                </TableHead>
                <TableHead className='w-[190px] py-1.5 px-3 text-xs'>
                  {t('errors.columns.node')}
                </TableHead>
                <TableHead className='w-[110px] py-1.5 px-3 text-xs'>
                  {t('errors.columns.occurrences')}
                </TableHead>
                <TableHead className='w-[160px] py-1.5 px-3 text-xs whitespace-nowrap'>
                  {t('errors.columns.lastSeen')}
                </TableHead>
                <TableHead className='w-[90px] py-1.5 px-3 text-xs text-right'>
                  {commonT('actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loadingGroups && displayedGroups.length === 0 ? (
                <TableSkeletonRows columns={6} rows={6} />
              ) : displayedGroups.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={6}
                    className='h-36 text-center text-muted-foreground text-xs'
                  >
                    {t('errors.empty')}
                  </TableCell>
                </TableRow>
              ) : (
                displayedGroups.map((group) => (
                  <TableRow
                    key={group.id}
                    className={cn(
                      'data-row-animate cursor-pointer transition-all hover:bg-muted/40 h-10',
                      loadingGroups && 'opacity-50 pointer-events-none',
                      selectedGroupId === group.id &&
                        'bg-primary/5 font-medium border-l-2 border-l-primary',
                    )}
                    onClick={() => handleOpenGroupDetail(group)}
                  >
                    {/* 错误标题与摘要 */}
                    <TableCell className='py-2 px-3'>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <div className='overflow-hidden max-w-[420px]'>
                            <div className='flex items-center gap-1.5'>
                              <span className='truncate font-medium text-xs text-foreground'>
                                {group.title || '-'}
                              </span>
                              {groupMemoryMap.get(group.id) && (
                                <Badge
                                  variant='outline'
                                  className='text-[10px] py-0 px-1 border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 gap-0.5 shrink-0 font-normal'
                                >
                                  <Lightbulb className='h-2.5 w-2.5 text-emerald-500' />
                                  已有方案
                                </Badge>
                              )}
                            </div>
                            <div className='truncate text-[11px] text-muted-foreground font-mono mt-0.5'>
                              {group.sample_message || group.fingerprint}
                            </div>
                          </div>
                        </TooltipTrigger>
                        <TooltipContent
                          side='top'
                          align='start'
                          className='max-w-[720px] break-all text-xs'
                        >
                          <div className='space-y-1'>
                            <div className='font-semibold'>{group.title || '-'}</div>
                            <div className='text-muted-foreground font-mono text-[11px]'>
                              {group.sample_message || group.fingerprint}
                            </div>
                          </div>
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>

                    {/* 异常类名 */}
                    <TableCell className='py-2 px-3 text-xs font-mono text-muted-foreground'>
                      <div
                        className='truncate max-w-[210px]'
                        title={group.exception_class || '-'}
                      >
                        {group.exception_class || '-'}
                      </div>
                    </TableCell>

                    {/* 来源节点 */}
                    <TableCell className='py-2 px-3 text-xs text-muted-foreground'>
                      <div
                        className='truncate max-w-[180px]'
                        title={formatNodeOrigin({
                          nodeId: group.last_node_id,
                          hostId: group.last_host_id,
                          hostName: group.last_host_name,
                          hostIp: group.last_host_ip,
                        })}
                      >
                        {formatNodeOrigin({
                          nodeId: group.last_node_id,
                          hostId: group.last_host_id,
                          hostName: group.last_host_name,
                          hostIp: group.last_host_ip,
                        })}
                      </div>
                    </TableCell>

                    {/* 发生频次 */}
                    <TableCell className='py-2 px-3 whitespace-nowrap'>
                      <Badge
                        variant='outline'
                        className={cn(
                          'text-xs font-mono px-2 py-0.5',
                          getOccurrenceHeatClass(group.occurrence_count),
                        )}
                      >
                        {group.occurrence_count} 次
                      </Badge>
                    </TableCell>

                    {/* 最近出现时间 */}
                    <TableCell className='py-2 px-3 whitespace-nowrap text-xs text-muted-foreground font-mono'>
                      {formatDateTime(group.last_seen_at)}
                    </TableCell>

                    {/* 操作按钮 */}
                    <TableCell className='py-2 px-3 text-right whitespace-nowrap'>
                      <Button
                        variant='ghost'
                        size='sm'
                        className='h-6.5 px-2 text-xs text-primary hover:text-primary'
                        onClick={(e) => {
                          e.stopPropagation();
                          handleOpenGroupDetail(group);
                        }}
                      >
                        <Eye className='mr-1 h-3.5 w-3.5' />
                        详情
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {/* 底部分页栏 / Table Footer Pagination */}
        <div className='border-t bg-muted/10 px-4 py-2.5 mt-auto'>
          <Pagination
            currentPage={page}
            totalPages={totalPages}
            pageSize={20}
            totalItems={groupTotal}
            onPageChange={setPage}
            showPageSizeSelector={false}
          />
        </div>
      </Card>

      {/* 侧边滑出式深度排查抽屉（告警中心同款架构，消除左右割裂） */}
      {/* Slide-over Sheet for Deep Error Diagnostics (aligned with Alert Center design) */}
      <Sheet
        open={Boolean(selectedGroupId && selectedGroup)}
        onOpenChange={(open) => {
          if (!open) {
            handleCloseDrawer();
          }
        }}
      >
        <SheetContent
          side='right'
          className='w-full sm:max-w-2xl p-0 flex flex-col overflow-hidden bg-background'
        >
          {/* 抽屉头部 / Sheet Header */}
          <SheetHeader className='p-4 pb-3 border-b bg-muted/20'>
            <div className='flex items-center gap-2 flex-wrap'>
              <Badge
                variant='outline'
                className={cn(
                  'text-xs font-mono',
                  selectedGroup
                    ? getOccurrenceHeatClass(selectedGroup.occurrence_count)
                    : '',
                )}
              >
                {t('errors.columns.occurrences')}:{' '}
                {selectedGroup?.occurrence_count ?? 0}
              </Badge>
              {selectedGroup?.last_seen_at && (
                <Badge variant='outline' className='text-xs'>
                  {t('errors.columns.lastSeen')}:{' '}
                  {formatDateTime(selectedGroup.last_seen_at)}
                </Badge>
              )}
            </div>
            <SheetTitle className='text-base font-bold tracking-tight text-foreground break-all mt-1'>
              {selectedGroup?.title || t('errors.detailTitle')}
            </SheetTitle>
            <SheetDescription className='text-xs font-mono text-muted-foreground break-all'>
              {selectedGroup?.exception_class || selectedGroup?.fingerprint}
            </SheetDescription>
          </SheetHeader>

          {/* 抽屉滚动内容区 / Sheet Scrollable Body */}
          <div className='flex-1 min-h-0 overflow-y-auto p-4'>
            {loadingDetail ? (
              <div className='space-y-3'>
                <Skeleton className='h-16 w-full' />
                <Skeleton className='h-36 w-full' />
                <Skeleton className='h-64 w-full' />
              </div>
            ) : selectedGroup ? (
              <div className='space-y-4 text-xs'>
                {/* 历史排障经验置顶回显卡片 / Historical Troubleshooting Solution Card */}
                <div className='sheet-section-animate'>
                  <TroubleshootingMemoryCard
                    matchedMemory={primaryMemory}
                    totalMatches={matchedMemories.length}
                    onAddOrEdit={() => setMemoryDialogOpen(true)}
                  />
                </div>

                {/* 拓扑上下文卡片 / Topology Context */}
                <div className='sheet-section-animate rounded-lg border p-3.5 space-y-2 bg-muted/15'>
                  <div className='font-semibold text-foreground flex items-center gap-1.5'>
                    <Server className='h-3.5 w-3.5 text-primary' />
                    <span>受影响拓扑与来源</span>
                  </div>
                  <div className='grid gap-1.5 text-muted-foreground sm:grid-cols-2'>
                    <div>
                      <span className='text-foreground font-medium'>
                        {t('errors.columns.node')}:{' '}
                      </span>
                      {formatNodeOrigin({
                        nodeId: selectedGroup.last_node_id,
                        hostId: selectedGroup.last_host_id,
                        hostName: selectedGroup.last_host_name,
                        hostIp: selectedGroup.last_host_ip,
                      })}
                    </div>
                    <div>
                      <span className='text-foreground font-medium'>
                        所属集群:{' '}
                      </span>
                      <span>{clusterName || (clusterId ? `#${clusterId}` : '全局')}</span>
                    </div>
                  </div>
                </div>

                {/* 错误摘要与样本信息 / Sample Message */}
                <div className='sheet-section-animate space-y-1.5'>
                  <div className='font-semibold text-foreground flex items-center justify-between'>
                    <span>错误摘要信息</span>
                    <Button
                      variant='ghost'
                      size='sm'
                      className='h-6 text-xs px-1.5 text-muted-foreground hover:text-foreground'
                      onClick={() =>
                        handleCopyEvidence(
                          selectedGroup.sample_message || selectedGroup.title,
                        )
                      }
                    >
                      <Copy className='mr-1 h-3 w-3' />
                      复制摘要
                    </Button>
                  </div>
                  <div className='rounded-md bg-muted/40 p-3 text-xs leading-relaxed text-foreground font-mono break-all'>
                    {selectedGroup.sample_message || t('errors.noSampleMessage')}
                  </div>
                </div>

                {/* 近期事件样本列表 / Recent Events Samples */}
                <div className='sheet-section-animate space-y-2 pt-1 border-t'>
                  <div className='flex items-center justify-between'>
                    <div className='font-semibold text-foreground'>
                      {t('errors.recentEventsTitle')}
                    </div>
                    <Badge variant='secondary' className='text-xs'>
                      {groupEvents.length} 条记录
                    </Badge>
                  </div>
                  <div className='rounded-lg border overflow-hidden'>
                    <Table>
                      <TableHeader>
                        <TableRow className='bg-muted/20 h-7 text-xs'>
                          <TableHead className='py-1 px-2.5'>
                            {t('errors.columns.time')}
                          </TableHead>
                          <TableHead className='py-1 px-2.5'>
                            {t('errors.columns.node')}
                          </TableHead>
                          <TableHead className='py-1 px-2.5'>Job</TableHead>
                          <TableHead className='py-1 px-2.5'>
                            {t('errors.columns.source')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {groupEvents.length === 0 ? (
                          <TableRow>
                            <TableCell
                              colSpan={4}
                              className='text-center py-4 text-muted-foreground text-xs'
                            >
                              {t('errors.noEvents')}
                            </TableCell>
                          </TableRow>
                        ) : (
                          groupEvents.map((event) => (
                            <TableRow
                              key={event.id}
                              className={cn(
                                'cursor-pointer text-xs transition-colors hover:bg-muted/40 h-8',
                                selectedEventId === event.id &&
                                  'bg-primary/5 font-medium border-l-2 border-l-primary',
                              )}
                              onClick={() => setSelectedEventId(event.id)}
                            >
                              <TableCell className='py-1.5 px-2.5 whitespace-nowrap text-muted-foreground font-mono'>
                                {formatDateTime(event.occurred_at)}
                              </TableCell>
                              <TableCell className='py-1.5 px-2.5 max-w-[140px] truncate text-muted-foreground'>
                                {formatNodeOrigin({
                                  nodeId: event.node_id,
                                  hostId: event.host_id,
                                  hostName: event.host_name,
                                  hostIp: event.host_ip,
                                  role: event.role,
                                })}
                              </TableCell>
                              <TableCell className='py-1.5 px-2.5 font-mono text-foreground'>
                                {event.job_id || '-'}
                              </TableCell>
                              <TableCell className='py-1.5 px-2.5 max-w-[140px] truncate font-mono text-muted-foreground'>
                                {event.source_file || '-'}
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </div>

                {/* 选中事件详情与异常堆栈阅读器 / Exception Evidence Viewer */}
                <div className='sheet-section-animate space-y-2 pt-1 border-t'>
                  <div className='flex items-center justify-between'>
                    <div className='font-semibold text-foreground flex items-center gap-1.5'>
                      <FileCode className='h-4 w-4 text-primary' />
                      <span>{t('errors.evidenceTitle')}</span>
                    </div>
                    {selectedEvent?.evidence && (
                      <Button
                        variant='outline'
                        size='sm'
                        className='h-7 text-xs gap-1'
                        onClick={() =>
                          handleCopyEvidence(selectedEvent.evidence)
                        }
                      >
                        {copiedEvidence ? (
                          <>
                            <Check className='h-3.5 w-3.5 text-emerald-500' />
                            <span>已复制</span>
                          </>
                        ) : (
                          <>
                            <Copy className='h-3.5 w-3.5' />
                            <span>复制完整堆栈</span>
                          </>
                        )}
                      </Button>
                    )}
                  </div>

                  <div className='relative rounded-lg border border-zinc-800 bg-zinc-950 dark:bg-zinc-900/95 text-zinc-200 overflow-hidden shadow-inner'>
                    <div className='flex items-center justify-between px-3 py-1.5 border-b border-zinc-800/80 bg-zinc-900/70 text-[11px] text-zinc-400 font-mono'>
                      <span>Stack Trace / Exception Snapshot</span>
                      <span className='truncate max-w-[260px]'>
                        {selectedEvent?.source_file || 'Standard Error Stream'}
                      </span>
                    </div>
                    <ScrollArea className='h-[260px]'>
                      <pre className='p-3.5 font-mono text-[11px] leading-relaxed text-zinc-300 whitespace-pre-wrap break-words select-text'>
                        {selectedEvent?.evidence || t('errors.noEvidence')}
                      </pre>
                    </ScrollArea>
                  </div>
                </div>
              </div>
            ) : null}
          </div>

          {/* 抽屉底部操作栏 / Sheet Sticky Footer */}
          {selectedGroup && (
            <div className='p-3 border-t bg-muted/20 flex flex-wrap items-center justify-between gap-2'>
              <div className='flex items-center gap-2'>
                {clusterId ? (
                  <Button
                    asChild
                    variant='outline'
                    size='sm'
                    className='h-8 text-xs gap-1'
                  >
                    <a
                      href={`/clusters/${clusterId}`}
                      target='_blank'
                      rel='noreferrer'
                    >
                      <ArrowUpRight className='h-3.5 w-3.5' />
                      前往受影响集群
                    </a>
                  </Button>
                ) : null}

                {/* 沉淀/更新经验库方案 / Record or update playbook */}
                <Button
                  variant='outline'
                  size='sm'
                  className='h-8 text-xs gap-1 border-emerald-500/40 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/10'
                  onClick={() => setMemoryDialogOpen(true)}
                >
                  <Lightbulb className='h-3.5 w-3.5 text-emerald-500' />
                  {primaryMemory ? tsT('updateSolution') : tsT('recordSolution')}
                </Button>
              </div>

              <Button
                variant='secondary'
                size='sm'
                className='h-8 text-xs'
                onClick={handleCloseDrawer}
              >
                关闭
              </Button>
            </div>
          )}
        </SheetContent>
      </Sheet>

      {/* 沉淀排障解决方案弹窗 / Save troubleshooting memory dialog */}
      <SaveMemoryDialog
        open={memoryDialogOpen}
        onOpenChange={setMemoryDialogOpen}
        initialData={
          selectedGroup
            ? {
                id: primaryMemory?.id,
                target_type: 'error',
                fingerprint:
                  selectedGroup.fingerprint ||
                  selectedGroup.exception_class ||
                  selectedGroup.title,
                title:
                  primaryMemory?.title ||
                  `${selectedGroup.title || selectedGroup.exception_class || '异常'} ${tsT('defaultTitleSuffix')}`,
                error_summary:
                  selectedGroup.sample_message || selectedGroup.title,
                root_cause: primaryMemory?.root_cause,
                solution: primaryMemory?.solution || '',
                preventive_tips: primaryMemory?.preventive_tips,
                tags:
                  primaryMemory?.tags ||
                  [
                    selectedGroup.exception_class
                      ? selectedGroup.exception_class
                          .split('.')
                          .pop()
                          ?.toLowerCase() || ''
                      : '',
                  ].filter(Boolean),
                cluster_id: clusterId,
                cluster_name: clusterName,
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
