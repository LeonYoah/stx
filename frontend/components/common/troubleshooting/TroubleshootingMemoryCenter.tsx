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

import {useState, useEffect, useMemo, useCallback} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertCircle,
  AlertTriangle,
  BookOpen,
  Calendar,
  Check,
  ChevronRight,
  Copy,
  Edit3,
  ExternalLink,
  FileCode,
  Fingerprint,
  Lightbulb,
  Plus,
  RefreshCw,
  Search,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Tag,
  Trash2,
  User,
  Users,
  X,
} from 'lucide-react';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import services from '@/lib/services';
import type {
  TroubleshootingMemoryEntry,
  TroubleshootingTargetType,
} from '@/lib/services/troubleshooting';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card} from '@/components/ui/card';
import {Input} from '@/components/ui/input';
import {Skeleton} from '@/components/ui/skeleton';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
import {ScrollArea} from '@/components/ui/scroll-area';
import {StatPillsBar, type StatPillItem} from '@/components/common/layout';
import {SaveMemoryDialog} from './SaveMemoryDialog';

export interface TroubleshootingMemoryCenterProps {
  // 可选过滤的集群 ID
  // Optional cluster ID filter
  clusterId?: number;
  // 可选过滤的集群名称
  // Optional cluster name filter
  clusterName?: string;
  // 选定某条排障经验时的外部回调
  // External callback when selecting a memory entry
  onSelectMemory?: (memory: TroubleshootingMemoryEntry) => void;
  // 自定义类名
  // Custom CSS class name
  className?: string;
}

// 格式化日期时间
// Format ISO date string into readable local date
function formatDateTime(value?: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleDateString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  });
}

/**
 * 统一排障经验库中心组件
 * Unified Troubleshooting Memory Center Component
 */
export function TroubleshootingMemoryCenter({
  clusterId,
  clusterName,
  onSelectMemory,
  className,
}: TroubleshootingMemoryCenterProps) {
  const t = useTranslations('troubleshooting');
  const commonT = useTranslations('common');

  // 数据列表与加载状态
  // Memory entries list and loading state
  const [memories, setMemories] = useState<TroubleshootingMemoryEntry[]>([]);
  const [loading, setLoading] = useState(true);

  // 搜索关键字与过滤类型
  // Search keyword and filter category
  const [searchKeyword, setSearchKeyword] = useState('');
  const [activeFilter, setActiveFilter] = useState('all');

  // 弹窗状态管理（新建 / 编辑）
  // Dialog state management (create / edit)
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [editingMemory, setEditingMemory] =
    useState<TroubleshootingMemoryEntry | null>(null);

  // 删除确认对话框状态
  // Delete confirmation dialog state
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deletingMemory, setDeletingMemory] =
    useState<TroubleshootingMemoryEntry | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  // 完整详情查看弹窗状态
  // Full detail viewing dialog state
  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const [selectedDetail, setSelectedDetail] =
    useState<TroubleshootingMemoryEntry | null>(null);

  // 复制反馈状态缓存
  // Copied feedback status cache
  const [copiedId, setCopiedId] = useState<string | null>(null);

  // 加载经验数据
  // Load memory entries from backend and cache
  const loadMemories = useCallback(async () => {
    setLoading(true);
    try {
      const data = await services.troubleshooting.fetchRemoteMemories({
        cluster_id: clusterId,
      });
      setMemories(data);
    } catch {
      toast.error('加载排障经验失败，请刷新重试');
    } finally {
      setLoading(false);
    }
  }, [clusterId]);

  // 初始化加载
  // Initial loading
  useEffect(() => {
    void loadMemories();
  }, [loadMemories]);

  // 计算各分类统计数量
  // Compute categorized statistics counts
  const stats = useMemo(() => {
    const total = memories.length;
    const preset = memories.filter((m) => m.is_preset).length;
    const custom = memories.filter((m) => !m.is_preset).length;
    const errorCount = memories.filter((m) => m.target_type === 'error').length;
    const alertCount = memories.filter((m) => m.target_type === 'alert').length;
    return {total, preset, custom, errorCount, alertCount};
  }, [memories]);

  // 胶囊分类项配置
  // Stat pills configuration
  const pillItems: StatPillItem[] = useMemo(
    () => [
      {
        key: 'all',
        label: t('allMemories'),
        count: stats.total,
        icon: <BookOpen className='size-3.5' />,
      },
      {
        key: 'custom',
        label: t('customMemories'),
        count: stats.custom,
        icon: <Users className='size-3.5 text-emerald-500' />,
        variant: 'success',
      },
      {
        key: 'preset',
        label: t('presetMemories'),
        count: stats.preset,
        icon: <Sparkles className='size-3.5 text-amber-500' />,
      },
      {
        key: 'error',
        label: t('errorType'),
        count: stats.errorCount,
        icon: <AlertTriangle className='size-3.5 text-red-500' />,
      },
      {
        key: 'alert',
        label: t('alertType'),
        count: stats.alertCount,
        icon: <ShieldAlert className='size-3.5 text-amber-500' />,
      },
    ],
    [stats, t],
  );

  // 综合过滤与搜索匹配逻辑
  // Combined filtering and search match logic
  const filteredMemories = useMemo(() => {
    return memories.filter((item) => {
      // 1. 胶囊状态过滤
      // 1. Pill category filter
      if (activeFilter === 'preset' && !item.is_preset) {
        return false;
      }
      if (activeFilter === 'custom' && item.is_preset) {
        return false;
      }
      if (activeFilter === 'error' && item.target_type !== 'error') {
        return false;
      }
      if (activeFilter === 'alert' && item.target_type !== 'alert') {
        return false;
      }

      // 2. 关键字搜索过滤
      // 2. Keyword search filter
      const keyword = searchKeyword.trim().toLowerCase();
      if (!keyword) {
        return true;
      }

      const matchTitle = item.title?.toLowerCase().includes(keyword);
      const matchFp = item.fingerprint?.toLowerCase().includes(keyword);
      const matchSummary = item.error_summary?.toLowerCase().includes(keyword);
      const matchSolution = item.solution?.toLowerCase().includes(keyword);
      const matchAuthor = item.author?.toLowerCase().includes(keyword);
      const matchTags = item.tags?.some((t) => t.toLowerCase().includes(keyword));

      return (
        matchTitle ||
        matchFp ||
        matchSummary ||
        matchSolution ||
        matchAuthor ||
        matchTags
      );
    });
  }, [activeFilter, memories, searchKeyword]);

  // 复制解决方案至剪贴板
  // Copy verified solution to clipboard
  const handleCopySolution = useCallback(
    (id: string, solutionText: string) => {
      if (!solutionText) {
        return;
      }
      navigator.clipboard.writeText(solutionText);
      setCopiedId(id);
      toast.success(t('solutionCopied'));
      setTimeout(() => setCopiedId(null), 2000);
    },
    [t],
  );

  // 打开新建经验弹窗
  // Open dialog to record new memory
  const handleOpenCreate = useCallback(() => {
    setEditingMemory(null);
    setSaveDialogOpen(true);
  }, []);

  // 打开编辑经验弹窗
  // Open dialog to edit existing memory
  const handleOpenEdit = useCallback((entry: TroubleshootingMemoryEntry) => {
    setEditingMemory(entry);
    setSaveDialogOpen(true);
  }, []);

  // 触发删除确认
  // Trigger delete confirmation prompt
  const handleRequestDelete = useCallback(
    (entry: TroubleshootingMemoryEntry) => {
      if (entry.is_preset) {
        toast.info(t('cannotDeletePreset'));
        return;
      }
      setDeletingMemory(entry);
      setDeleteConfirmOpen(true);
    },
    [t],
  );

  // 确认执行删除
  // Confirm and execute memory deletion
  const handleConfirmDelete = useCallback(async () => {
    if (!deletingMemory) {
      return;
    }
    setIsDeleting(true);
    try {
      await services.troubleshooting.deleteRemoteMemory(deletingMemory.id);
      toast.success(t('deleteSuccess'));
      setDeleteConfirmOpen(false);
      setDeletingMemory(null);
      await loadMemories();
    } catch {
      toast.error(t('deleteFailed'));
    } finally {
      setIsDeleting(false);
    }
  }, [deletingMemory, loadMemories, t]);

  // 打开完整详情弹窗
  // Open full detail modal
  const handleOpenDetail = useCallback((entry: TroubleshootingMemoryEntry) => {
    setSelectedDetail(entry);
    setDetailModalOpen(true);
  }, []);

  return (
    <div className={cn('space-y-3.5', className)}>
      {/* 顶部胶囊统计与快速筛选栏（高信息密度，符合 UI Spec） */}
      {/* Top Stat Pills Bar for distribution overview and quick category switching */}
      <StatPillsBar
        items={pillItems}
        activeKey={activeFilter}
        onChange={(key) => setActiveFilter(key)}
      />

      {/* 搜索与快捷操作工具条 */}
      {/* Search and Quick Action Toolbar */}
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between rounded-lg border bg-card/60 p-2.5 shadow-2xs'>
        <div className='relative flex-1 max-w-md'>
          <Search className='absolute left-2.5 top-2.5 size-3.5 text-muted-foreground' />
          <Input
            value={searchKeyword}
            onChange={(e) => setSearchKeyword(e.target.value)}
            placeholder={t('searchPlaceholder')}
            className='h-8 pl-8 pr-7 text-xs bg-background'
          />
          {searchKeyword && (
            <button
              type='button'
              onClick={() => setSearchKeyword('')}
              className='absolute right-2 top-2 text-muted-foreground hover:text-foreground cursor-pointer'
              title='清空搜索'
            >
              <X className='size-3.5' />
            </button>
          )}
        </div>

        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void loadMemories()}
            disabled={loading}
            className='h-8 text-xs'
          >
            <RefreshCw
              className={cn('mr-1.5 size-3.5', loading && 'animate-spin')}
            />
            {commonT('refresh')}
          </Button>

          <Button
            size='sm'
            onClick={handleOpenCreate}
            className='h-8 text-xs shadow-2xs'
          >
            <Plus className='mr-1.5 size-3.5' />
            {t('recordNewMemory')}
          </Button>
        </div>
      </div>

      {/* 经验卡片网格区域 */}
      {/* Troubleshooting Memories Card Grid */}
      {loading && memories.length === 0 ? (
        <div className='grid grid-cols-1 xl:grid-cols-2 gap-3.5'>
          {Array.from({length: 4}).map((_, idx) => (
            <Card key={`skeleton-${idx}`} className='p-4 space-y-3'>
              <div className='flex items-center justify-between'>
                <Skeleton className='h-5 w-32' />
                <Skeleton className='h-5 w-20' />
              </div>
              <Skeleton className='h-4 w-3/4' />
              <Skeleton className='h-20 w-full' />
              <div className='flex items-center justify-between pt-2'>
                <Skeleton className='h-4 w-24' />
                <Skeleton className='h-4 w-28' />
              </div>
            </Card>
          ))}
        </div>
      ) : filteredMemories.length === 0 ? (
        <Card className='p-10 text-center flex flex-col items-center justify-center border-dashed'>
          <div className='size-11 rounded-full bg-muted/70 flex items-center justify-center mb-3 text-muted-foreground'>
            <BookOpen className='size-5' />
          </div>
          <h3 className='text-sm font-semibold text-foreground mb-1'>
            {t('noMemoriesFound')}
          </h3>
          <p className='text-xs text-muted-foreground max-w-sm mb-4'>
            {t('noMemoriesFoundHint')}
          </p>
          <Button size='sm' onClick={handleOpenCreate} className='h-8 text-xs'>
            <Plus className='mr-1.5 size-3.5' />
            {t('recordNewMemory')}
          </Button>
        </Card>
      ) : (
        <div className='grid grid-cols-1 xl:grid-cols-2 gap-3.5'>
          {filteredMemories.map((entry) => {
            const isCopied = copiedId === entry.id;

            return (
              <Card
                key={entry.id}
                className={cn(
                  'group flex flex-col justify-between p-4 transition-all duration-150',
                  'border hover:border-primary/40 hover:shadow-xs bg-card',
                )}
              >
                <div className='space-y-2.5'>
                  {/* 卡片头部：分类属性、指纹胶囊与操作按钮 */}
                  {/* Card Header: Category badges, fingerprint capsule, and action buttons */}
                  <div className='flex items-start justify-between gap-2'>
                    <div className='flex flex-wrap items-center gap-1.5'>
                      {entry.is_preset ? (
                        <Badge
                          variant='secondary'
                          className='h-5 px-1.5 text-[10px] font-medium gap-1 bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20'
                        >
                          <Sparkles className='size-2.5 text-amber-500' />
                          {t('presetSolution')}
                        </Badge>
                      ) : (
                        <Badge
                          variant='secondary'
                          className='h-5 px-1.5 text-[10px] font-medium gap-1 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20'
                        >
                          <Users className='size-2.5 text-emerald-600 dark:text-emerald-400' />
                          {t('teamSolution')}
                        </Badge>
                      )}

                      <Badge
                        variant='outline'
                        className='h-5 px-1.5 text-[10px] font-normal text-muted-foreground'
                      >
                        {entry.target_type === 'error'
                          ? t('errorType')
                          : t('alertType')}
                      </Badge>

                      {entry.fingerprint && (
                        <span
                          className='inline-flex items-center gap-1 font-mono text-[10px] text-muted-foreground bg-muted/60 px-1.5 py-0.5 rounded border border-border/60 max-w-[200px] truncate'
                          title={`故障指纹: ${entry.fingerprint}`}
                        >
                          <Fingerprint className='size-2.5 shrink-0 text-primary' />
                          <span className='truncate'>{entry.fingerprint}</span>
                        </span>
                      )}
                    </div>

                    <div className='flex items-center gap-1 shrink-0'>
                      <Button
                        variant='ghost'
                        size='icon'
                        onClick={() => handleCopySolution(entry.id, entry.solution)}
                        className='size-7 text-muted-foreground hover:text-emerald-600'
                        title={t('copySolution')}
                      >
                        {isCopied ? (
                          <Check className='size-3.5 text-emerald-600' />
                        ) : (
                          <Copy className='size-3.5' />
                        )}
                      </Button>

                      <Button
                        variant='ghost'
                        size='icon'
                        onClick={() => handleOpenEdit(entry)}
                        className='size-7 text-muted-foreground hover:text-foreground'
                        title={t('edit')}
                      >
                        <Edit3 className='size-3.5' />
                      </Button>

                      {!entry.is_preset && (
                        <Button
                          variant='ghost'
                          size='icon'
                          onClick={() => handleRequestDelete(entry)}
                          className='size-7 text-muted-foreground hover:text-destructive'
                          title={t('delete')}
                        >
                          <Trash2 className='size-3.5' />
                        </Button>
                      )}
                    </div>
                  </div>

                  {/* 方案标题 */}
                  {/* Solution Title */}
                  <h4
                    onClick={() => handleOpenDetail(entry)}
                    className='text-sm font-semibold text-foreground tracking-tight line-clamp-1 hover:text-primary transition-colors cursor-pointer'
                  >
                    {entry.title}
                  </h4>

                  {/* 异常现象摘录 */}
                  {/* Symptom / Error excerpt */}
                  {entry.error_summary && (
                    <div className='font-mono text-[11px] text-muted-foreground bg-muted/40 rounded-md px-2.5 py-1.5 line-clamp-2 border border-border/40'>
                      {entry.error_summary}
                    </div>
                  )}

                  {/* 核心排障处理方案卡片 */}
                  {/* Core Verified Remediation Solution Box */}
                  <div className='rounded-lg border border-emerald-500/25 bg-emerald-500/[0.03] dark:bg-emerald-950/20 p-2.5 space-y-1'>
                    <div className='flex items-center justify-between text-[11px] font-semibold text-emerald-700 dark:text-emerald-400'>
                      <span className='flex items-center gap-1'>
                        <ShieldCheck className='size-3.5' />
                        {t('verifiedSolution')}
                      </span>
                      <button
                        type='button'
                        onClick={() => handleOpenDetail(entry)}
                        className='text-[10px] font-normal hover:underline cursor-pointer flex items-center gap-0.5'
                      >
                        {t('viewFullDetail')}
                        <ChevronRight className='size-3' />
                      </button>
                    </div>

                    <pre className='text-xs font-mono whitespace-pre-wrap text-foreground/90 leading-relaxed max-h-24 overflow-y-auto pr-1'>
                      {entry.solution}
                    </pre>
                  </div>

                  {/* 根因分析或防范建议摘要（渐进式轻量呈现） */}
                  {/* Compact Root Cause / Preventive Tips Snippets */}
                  {(entry.root_cause || entry.preventive_tips) && (
                    <div className='grid grid-cols-1 sm:grid-cols-2 gap-2 text-[11px]'>
                      {entry.root_cause && (
                        <div className='p-2 rounded bg-muted/30 border border-border/40 line-clamp-2'>
                          <span className='font-medium text-foreground mr-1'>
                            {t('rootCause')}:
                          </span>
                          <span className='text-muted-foreground'>
                            {entry.root_cause}
                          </span>
                        </div>
                      )}
                      {entry.preventive_tips && (
                        <div className='p-2 rounded bg-muted/30 border border-border/40 line-clamp-2'>
                          <span className='font-medium text-foreground mr-1'>
                            {t('preventiveTips')}:
                          </span>
                          <span className='text-muted-foreground'>
                            {entry.preventive_tips}
                          </span>
                        </div>
                      )}
                    </div>
                  )}
                </div>

                {/* 卡片底部：分类标签、记录人与更新时间 */}
                {/* Card Footer: Categorical tags, author, and updated timestamp */}
                <div className='flex items-center justify-between pt-3 mt-3 border-t border-border/50 text-[11px] text-muted-foreground'>
                  <div className='flex flex-wrap items-center gap-1 max-w-[60%] truncate'>
                    {(entry.tags || []).slice(0, 3).map((tag) => (
                      <Badge
                        key={tag}
                        variant='outline'
                        className='h-4 px-1 text-[10px] font-normal text-muted-foreground/80'
                      >
                        #{tag}
                      </Badge>
                    ))}
                    {(entry.tags || []).length > 3 && (
                      <span className='text-[10px] text-muted-foreground'>
                        +{(entry.tags || []).length - 3}
                      </span>
                    )}
                  </div>

                  <div className='flex items-center gap-3 shrink-0'>
                    <span className='flex items-center gap-1'>
                      <User className='size-3 text-muted-foreground/70' />
                      <span className='max-w-[80px] truncate'>{entry.author}</span>
                    </span>
                    <span className='flex items-center gap-1'>
                      <Calendar className='size-3 text-muted-foreground/70' />
                      <span>{formatDateTime(entry.updated_at)}</span>
                    </span>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {/* 方案新建与编辑弹窗（复用高审美 SaveMemoryDialog） */}
      {/* Save and Edit Memory Dialog */}
      <SaveMemoryDialog
        open={saveDialogOpen}
        onOpenChange={setSaveDialogOpen}
        initialData={
          editingMemory
            ? {
                id: editingMemory.id,
                target_type: editingMemory.target_type,
                fingerprint: editingMemory.fingerprint,
                title: editingMemory.title,
                error_summary: editingMemory.error_summary,
                root_cause: editingMemory.root_cause,
                solution: editingMemory.solution,
                preventive_tips: editingMemory.preventive_tips,
                cluster_id: editingMemory.cluster_id,
                cluster_name: editingMemory.cluster_name,
                tags: editingMemory.tags,
                author: editingMemory.author,
              }
            : null
        }
        onSaved={loadMemories}
      />

      {/* 删除确认对话框 */}
      {/* Delete Confirmation Alert Dialog */}
      <AlertDialog open={deleteConfirmOpen} onOpenChange={setDeleteConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('confirmDeleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('confirmDeleteDesc', {title: deletingMemory?.title || ''})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>
              {t('cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => void handleConfirmDelete()}
              disabled={isDeleting}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
            >
              {isDeleting ? '正在删除...' : t('confirmDelete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* 完整排障方案阅读弹窗 */}
      {/* Full Troubleshooting Solution Detail Dialog */}
      <Dialog open={detailModalOpen} onOpenChange={setDetailModalOpen}>
        <DialogContent className='max-w-2xl max-h-[85vh] flex flex-col p-0 gap-0'>
          <DialogHeader className='px-5 py-4 border-b shrink-0'>
            <div className='flex items-center gap-2'>
              <DialogTitle className='text-sm font-semibold tracking-tight'>
                {selectedDetail?.title}
              </DialogTitle>
              {selectedDetail?.is_preset ? (
                <Badge
                  variant='secondary'
                  className='h-5 px-1.5 text-[10px] font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400'
                >
                  {t('presetSolution')}
                </Badge>
              ) : (
                <Badge
                  variant='secondary'
                  className='h-5 px-1.5 text-[10px] font-medium bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                >
                  {t('teamSolution')}
                </Badge>
              )}
            </div>
            <DialogDescription className='text-xs font-mono text-muted-foreground flex items-center gap-1.5 mt-1'>
              <Fingerprint className='size-3 text-primary' />
              <span>指纹: {selectedDetail?.fingerprint || '-'}</span>
            </DialogDescription>
          </DialogHeader>

          {selectedDetail && (
            <ScrollArea className='flex-1 p-5'>
              <div className='space-y-4 text-xs'>
                {/* 故障现象摘录 */}
                {/* Fault Symptom */}
                {selectedDetail.error_summary && (
                  <div className='space-y-1.5'>
                    <div className='font-medium text-foreground flex items-center gap-1'>
                      <FileCode className='size-3.5 text-muted-foreground' />
                      <span>异常现象与错误日志</span>
                    </div>
                    <pre className='p-2.5 rounded bg-muted/50 font-mono text-[11px] text-muted-foreground whitespace-pre-wrap leading-relaxed border border-border/50'>
                      {selectedDetail.error_summary}
                    </pre>
                  </div>
                )}

                {/* 完整解决方案 */}
                {/* Complete Solution */}
                <div className='space-y-1.5 rounded-lg border border-emerald-500/30 bg-emerald-500/[0.03] dark:bg-emerald-950/20 p-3'>
                  <div className='flex items-center justify-between font-semibold text-emerald-700 dark:text-emerald-400'>
                    <span className='flex items-center gap-1.5'>
                      <ShieldCheck className='size-4' />
                      <span>已验证排障方案与操作步骤</span>
                    </span>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        handleCopySolution(
                          selectedDetail.id,
                          selectedDetail.solution,
                        )
                      }
                      className='h-6 text-[11px] px-2 gap-1 border-emerald-500/30 hover:bg-emerald-500/10'
                    >
                      {copiedId === selectedDetail.id ? (
                        <>
                          <Check className='size-3 text-emerald-600' />
                          <span>已复制</span>
                        </>
                      ) : (
                        <>
                          <Copy className='size-3' />
                          <span>复制方案</span>
                        </>
                      )}
                    </Button>
                  </div>

                  <pre className='text-xs font-mono whitespace-pre-wrap text-foreground leading-relaxed pt-1.5'>
                    {selectedDetail.solution}
                  </pre>
                </div>

                {/* 根因剖析 */}
                {/* Root Cause Analysis */}
                {selectedDetail.root_cause && (
                  <div className='space-y-1.5'>
                    <div className='font-medium text-foreground flex items-center gap-1'>
                      <AlertCircle className='size-3.5 text-amber-500' />
                      <span>{t('rootCause')}</span>
                    </div>
                    <p className='text-muted-foreground leading-relaxed p-2.5 rounded bg-muted/30 border border-border/40'>
                      {selectedDetail.root_cause}
                    </p>
                  </div>
                )}

                {/* 防范建议 */}
                {/* Prevention Tips */}
                {selectedDetail.preventive_tips && (
                  <div className='space-y-1.5'>
                    <div className='font-medium text-foreground flex items-center gap-1'>
                      <Lightbulb className='size-3.5 text-emerald-500' />
                      <span>{t('preventiveTips')}</span>
                    </div>
                    <p className='text-muted-foreground leading-relaxed p-2.5 rounded bg-muted/30 border border-border/40'>
                      {selectedDetail.preventive_tips}
                    </p>
                  </div>
                )}

                {/* 标签与元数据 */}
                {/* Tags and Metadata */}
                <div className='flex flex-wrap items-center justify-between gap-2 pt-2 border-t text-[11px] text-muted-foreground'>
                  <div className='flex flex-wrap items-center gap-1'>
                    <Tag className='size-3 text-muted-foreground/70' />
                    {(selectedDetail.tags || []).map((t) => (
                      <Badge
                        key={t}
                        variant='outline'
                        className='text-[10px] font-normal h-4 px-1'
                      >
                        #{t}
                      </Badge>
                    ))}
                  </div>
                  <div className='flex items-center gap-3'>
                    <span>记录人: {selectedDetail.author}</span>
                    <span>更新时间: {formatDateTime(selectedDetail.updated_at)}</span>
                  </div>
                </div>
              </div>
            </ScrollArea>
          )}

          <DialogFooter className='px-5 py-3 border-t shrink-0 flex items-center justify-between sm:justify-between'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setDetailModalOpen(false)}
              className='h-8 text-xs'
            >
              关闭
            </Button>
            <div className='flex items-center gap-2'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => {
                  setDetailModalOpen(false);
                  if (selectedDetail) {
                    handleOpenEdit(selectedDetail);
                  }
                }}
                className='h-8 text-xs'
              >
                <Edit3 className='mr-1 size-3.5' />
                {t('edit')}
              </Button>
              <Button
                size='sm'
                onClick={() => {
                  if (selectedDetail) {
                    handleCopySolution(
                      selectedDetail.id,
                      selectedDetail.solution,
                    );
                  }
                }}
                className='h-8 text-xs'
              >
                <Copy className='mr-1.5 size-3.5' />
                {t('copySolution')}
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
