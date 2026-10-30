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

import dynamic from 'next/dynamic';
import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {useTheme} from 'next-themes';
import {
  ChevronDown,
  ChevronUp,
  Copy,
  CopyPlus,
  Eye,
  FileCode,
  Loader2,
  Maximize2,
  Pencil,
  Plus,
  Trash2,
  Wrench,
} from 'lucide-react';
import {toast} from 'sonner';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {HelpHint} from '@/components/common/HelpHint';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {cn, copyToClipboard} from '@/lib/utils';
import type {
  SyncCuratedSection,
  SyncCuratedTemplateView,
} from '@/lib/services/sync';
import services from '@/lib/services';

const MonacoEditor = dynamic(() => import('@monaco-editor/react'), {
  ssr: false,
});

type EditorMode = 'create' | 'edit' | 'copy';

function itemKey(item: SyncCuratedTemplateView) {
  return `${item.origin}-${item.builtin_id || item.id}-${item.name}`;
}

function isOwned(item: SyncCuratedTemplateView) {
  return (
    (item.origin === 'user' || item.origin === 'override') && !!item.id
  );
}

/**
 * 模板管理：全量列表 + 检索/类型过滤；自建/改/删/创建副本/脚本预览/脚本复制；宽屏 Monaco 代码编辑与脚本预览。
 * Template manage: full list + filters; create/edit/delete/fork/preview/copy; wide Monaco editor and preview.
 */
export function CuratedTemplatesManagePanel({
  refreshToken = 0,
  onChanged,
  clusterId,
}: {
  refreshToken?: number;
  onChanged?: () => void;
  clusterId?: string;
}) {
  const t = useTranslations('workbenchStudio');
  const {resolvedTheme} = useTheme();
  const monacoTheme = resolvedTheme === 'light' ? 'vs' : 'vs-dark';

  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<SyncCuratedTemplateView[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [section, setSection] = useState<string>('all');
  const [origin, setOrigin] = useState<string>('all');
  const [query, setQuery] = useState('');
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] =
    useState<SyncCuratedTemplateView | null>(null);

  // 展开速览脚本的模板 Key 集合
  // Set of template keys with expanded inline script preview
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());

  // 预览弹窗状态
  // Preview modal dialog state
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewItem, setPreviewItem] =
    useState<SyncCuratedTemplateView | null>(null);
  const [previewContent, setPreviewContent] = useState('');
  const [previewLoading, setPreviewLoading] = useState(false);

  // 编辑 / 新建 / 副本弹窗状态
  // Edit / create / copy draft modal dialog state
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorMode, setEditorMode] = useState<EditorMode>('create');
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editName, setEditName] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [editSection, setEditSection] = useState<SyncCuratedSection>('source');
  const [editContent, setEditContent] = useState('');
  const [saving, setSaving] = useState(false);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await services.sync.listCuratedTemplates({
        section: section === 'all' ? undefined : section,
        q: query.trim() || undefined,
        origin: origin === 'all' ? undefined : origin,
      });
      setItems(data.items || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('loadCuratedFailed'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [section, origin, refreshToken]);

  const visibleItems = useMemo(() => {
    // API 已按 section/origin/q 过滤；本地再按 origin 兜底（builtin 带 has_override 仍属 builtin）
    // API already filters; client keeps origin guard for builtin+has_override rows
    if (origin === 'all') {
      return items;
    }
    return items.filter((item) => item.origin === origin);
  }, [items, origin]);

  /**
   * 复制 HOCON 脚本到剪贴板
   * Copy HOCON script to clipboard
   */
  const handleCopyScript = async (content: string) => {
    if (!content) {
      toast.info(t('curatedPreviewEmpty'));
      return;
    }
    try {
      await copyToClipboard(content);
      toast.success(t('curatedScriptCopied'));
    } catch {
      toast.error(t('curatedCopyScriptFailed'));
    }
  };

  /**
   * 切换卡片内联脚本速览的展开/收起
   * Toggle inline script preview expand/collapse for a card
   */
  const toggleExpanded = (key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  /**
   * 打开模板完整预览弹窗
   * Open full template preview modal dialog
   */
  const openPreview = (item: SyncCuratedTemplateView) => {
    setPreviewItem(item);
    const fallback = (item.content || '').trim();
    setPreviewContent(fallback);
    setPreviewOpen(true);

    const clusterNum = clusterId ? Number(clusterId) : 0;
    if (Number.isFinite(clusterNum) && clusterNum > 0) {
      setPreviewLoading(true);
      const renderPromise = services.sync.renderCuratedTemplate?.({
        builtin_id: item.builtin_id,
        id: item.id,
        cluster_id: clusterNum,
      });
      if (renderPromise && typeof renderPromise.then === 'function') {
        void renderPromise
          .then((res) => {
            if (res?.content) {
              setPreviewContent(res.content.trim());
            }
          })
          .catch(() => {
            // 降级使用原始模板 content
            // Fallback to raw template content
            setPreviewContent(fallback);
          })
          .finally(() => {
            setPreviewLoading(false);
          });
      } else {
        setPreviewLoading(false);
      }
    }
  };

  const openCreate = () => {
    setEditorMode('create');
    setEditingId(null);
    setEditName('');
    setEditDescription('');
    setEditSection('source');
    setEditContent('');
    setEditorOpen(true);
  };

  const openEdit = (item: SyncCuratedTemplateView) => {
    if (!isOwned(item)) {
      toast.info(t('curatedBuiltinReadonly'));
      return;
    }
    setEditorMode('edit');
    setEditingId(item.id || null);
    setEditName(item.name || '');
    setEditDescription(item.description || '');
    setEditSection((item.section || 'source') as SyncCuratedSection);
    setEditContent(item.content || '');
    setEditorOpen(true);
  };

  /**
   * 基于现有模板创建副本（系统模板 fork，自建模板生成副本草稿）
   * Create copy from existing template (fork builtin, or create draft for user templates)
   */
  const openCopy = async (item: SyncCuratedTemplateView) => {
    const key = itemKey(item);
    setBusyKey(key);
    try {
      if (item.origin === 'builtin' && item.builtin_id) {
        if (item.has_override) {
          toast.info(t('curatedHasOverrideHint'));
          const override = items.find(
            (row) =>
              row.origin === 'override' && row.builtin_id === item.builtin_id,
          );
          if (override) {
            openEdit(override);
          }
          return;
        }
        const forked = await services.sync.forkCuratedTemplate({
          builtin_id: item.builtin_id,
          name: `${item.name} · copy`,
        });
        toast.success(t('curatedForked'));
        onChanged?.();
        await load();
        openEdit({...forked, origin: 'override'});
        return;
      }

      // 我的精选：复制为新建草稿
      // Mine: duplicate into a create draft
      setEditorMode('copy');
      setEditingId(null);
      setEditName(`${item.name || 'template'} · copy`);
      setEditDescription(item.description || '');
      setEditSection((item.section || 'source') as SyncCuratedSection);
      setEditContent(item.content || '');
      setEditorOpen(true);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('curatedForkFailed'));
    } finally {
      setBusyKey(null);
    }
  };

  const handleSave = async () => {
    const name = editName.trim();
    const content = editContent.trim();
    if (!name || !content) {
      toast.error(t('curatedSaveEmpty'));
      return;
    }
    setSaving(true);
    try {
      if (editorMode === 'edit' && editingId) {
        await services.sync.updateCuratedTemplate(editingId, {
          name,
          description: editDescription.trim() || undefined,
          section: editSection,
          content,
        });
        toast.success(t('curatedUpdated'));
      } else {
        await services.sync.createCuratedTemplate({
          name,
          description: editDescription.trim() || undefined,
          section: editSection,
          content,
        });
        toast.success(t('curatedSaved'));
      }
      setEditorOpen(false);
      onChanged?.();
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('curatedSaveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (item: SyncCuratedTemplateView) => {
    if (!isOwned(item) || !item.id) {
      toast.error(t('curatedBuiltinReadonly'));
      return;
    }
    const key = itemKey(item);
    setBusyKey(key);
    try {
      await services.sync.deleteCuratedTemplate(item.id);
      toast.success(t('curatedDeleted'));
      onChanged?.();
      await load();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('curatedDeleteFailed'),
      );
    } finally {
      setBusyKey(null);
    }
  };

  const editorTitle =
    editorMode === 'edit'
      ? t('curatedEdit')
      : editorMode === 'copy'
        ? t('curatedCreateCopy')
        : t('curatedCreate');

  const getOriginLabel = (orig: string) => {
    if (orig === 'builtin') return t('curatedOriginBuiltin');
    if (orig === 'user') return t('curatedOriginUser');
    if (orig === 'override') return t('curatedOriginOverride');
    return orig;
  };

  return (
    <div className='flex h-full min-h-0 w-full min-w-0 flex-col gap-3.5'>
      <div className='shrink-0 rounded-lg border border-border/60 bg-muted/10 p-3 space-y-2.5'>
        <div className='flex items-center justify-between gap-2'>
          <div className='flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-foreground'>
            <Wrench className='size-3.5 text-primary' />
            <span>{t('curatedManage')}</span>
          </div>
          <Button
            type='button'
            size='sm'
            className='h-7 px-2 text-[11px]'
            onClick={openCreate}
          >
            <Plus className='size-3.5 mr-1' />
            {t('curatedCreate')}
          </Button>
        </div>

        <div className='flex flex-col gap-2'>
          <div className='flex gap-2'>
            <Select value={section} onValueChange={setSection}>
              <SelectTrigger className='h-8 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('curatedSectionAll')}</SelectItem>
                <SelectItem value='env'>env</SelectItem>
                <SelectItem value='source'>source</SelectItem>
                <SelectItem value='transform'>transform</SelectItem>
                <SelectItem value='sink'>sink</SelectItem>
                <SelectItem value='combo'>combo</SelectItem>
              </SelectContent>
            </Select>
            <Select value={origin} onValueChange={setOrigin}>
              <SelectTrigger className='h-8 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='all'>{t('curatedOriginAll')}</SelectItem>
                <SelectItem value='builtin'>
                  {t('curatedOriginBuiltin')}
                </SelectItem>
                <SelectItem value='user'>{t('curatedOriginUser')}</SelectItem>
                <SelectItem value='override'>
                  {t('curatedOriginOverride')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                void load();
              }
            }}
            placeholder={t('searchCurated')}
            className='h-8 text-xs'
          />
        </div>
      </div>

      {loading ? (
        <div className='flex items-center gap-2 text-xs text-muted-foreground py-3'>
          <Loader2 className='size-3.5 animate-spin' />
          {t('loadingCurated')}
        </div>
      ) : error ? (
        <p className='text-[11px] text-destructive'>{error}</p>
      ) : visibleItems.length === 0 ? (
        <p className='text-[11px] text-muted-foreground px-1'>
          {t('noCuratedTemplates')}
        </p>
      ) : (
        <div className='min-h-0 flex-1 space-y-1.5 overflow-y-auto pr-1'>
          {visibleItems.map((item) => {
            const key = itemKey(item);
            const owned = isOwned(item);
            const busy = busyKey === key;
            const isExpanded = expandedKeys.has(key);
            return (
              <div
                key={key}
                className={cn(
                  'rounded-md border border-border/60 bg-background/60 px-2.5 py-2 transition-colors',
                  !owned && 'opacity-95',
                )}
              >
                <div className='flex items-center gap-1.5 min-w-0'>
                  <span className='text-xs font-medium truncate' title={item.name}>
                    {item.name}
                  </span>
                  {item.origin === 'builtin' ? (
                    // 系统模板不渲染禁用的编辑/删除，「只读、创建副本后可编辑」挂在来源 Badge 上；包成 button 以便键盘聚焦触发 Tooltip
                    // Builtin rows render no disabled edit/delete; the read-only hint lives on the origin Badge, wrapped in a button so keyboard focus opens the Tooltip
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type='button'
                          className='inline-flex shrink-0 cursor-help rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50'
                        >
                          <Badge
                            variant='secondary'
                            className='text-[9px] px-1 py-0'
                          >
                            {getOriginLabel(item.origin)}
                          </Badge>
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-[240px] text-xs'>
                        {t('curatedReadonlyEditHint')}
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    <Badge
                      variant='secondary'
                      className='text-[9px] px-1 py-0 shrink-0'
                    >
                      {getOriginLabel(item.origin)}
                    </Badge>
                  )}
                  <Badge
                    variant='outline'
                    className='text-[9px] px-1 py-0 shrink-0 font-mono'
                  >
                    {item.section}
                  </Badge>
                  {item.has_override && item.origin === 'builtin' ? (
                    <Badge
                      variant='outline'
                      className='text-[9px] px-1 py-0 shrink-0 text-primary border-primary/40'
                    >
                      {t('curatedHasOverride')}
                    </Badge>
                  ) : null}
                </div>
                {item.description ? (
                  <p className='mt-0.5 text-[10px] text-muted-foreground line-clamp-2'>
                    {item.description}
                  </p>
                ) : null}

                {/* 操作栏：预览、复制脚本、创建副本、编辑、删除、展开速览 */}
                {/* Action toolbar: preview, copy script, create copy, edit, delete, toggle inline preview */}
                <div className='mt-1.5 flex flex-wrap items-center gap-1'>
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    className='h-6 px-1.5 text-[10px] text-foreground hover:text-primary'
                    onClick={() => openPreview(item)}
                    title={t('curatedPreviewTitle')}
                  >
                    <Eye className='size-3 mr-1 text-primary' />
                    {t('curatedPreview')}
                  </Button>
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    className='h-6 px-1.5 text-[10px]'
                    onClick={() => void handleCopyScript(item.content || '')}
                    title={t('curatedCopyScript')}
                  >
                    <Copy className='size-3 mr-1' />
                    {t('curatedCopyScript')}
                  </Button>
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    className='h-6 px-1.5 text-[10px]'
                    disabled={busy}
                    onClick={() => void openCopy(item)}
                    title={t('curatedCreateCopyHint')}
                  >
                    {busy ? (
                      <Loader2 className='size-3 mr-1 animate-spin' />
                    ) : (
                      <CopyPlus className='size-3 mr-1' />
                    )}
                    {t('curatedCreateCopy')}
                  </Button>
                  {owned ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='ghost'
                      className='h-6 px-1.5 text-[10px]'
                      disabled={busy}
                      onClick={() => openEdit(item)}
                    >
                      <Pencil className='size-3 mr-1' />
                      {t('curatedEdit')}
                    </Button>
                  ) : null}
                  {owned ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='ghost'
                      className='h-6 px-1.5 text-[10px] text-destructive hover:text-destructive'
                      disabled={busy}
                      onClick={() => setPendingDelete(item)}
                    >
                      <Trash2 className='size-3 mr-1' />
                      {t('delete')}
                    </Button>
                  ) : null}
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    className='h-6 px-1.5 text-[10px] text-muted-foreground ml-auto'
                    onClick={() => toggleExpanded(key)}
                    title={
                      isExpanded
                        ? t('curatedHideInlinePreview')
                        : t('curatedToggleInlinePreview')
                    }
                  >
                    {isExpanded ? (
                      <>
                        <ChevronUp className='size-3 mr-0.5' />
                        <span>{t('curatedHideInlinePreview')}</span>
                      </>
                    ) : (
                      <>
                        <ChevronDown className='size-3 mr-0.5' />
                        <span>{t('curatedToggleInlinePreview')}</span>
                      </>
                    )}
                  </Button>
                </div>

                {/* 卡片内联速览区 */}
                {/* Inline script preview card */}
                {isExpanded ? (
                  <div className='mt-2 rounded-md border border-border/60 bg-muted/20 overflow-hidden'>
                    <div className='flex items-center justify-between border-b border-border/40 bg-muted/30 px-2.5 py-1 text-[10px] text-muted-foreground'>
                      <span className='font-mono font-medium'>
                        HOCON ({item.section})
                      </span>
                      <div className='flex items-center gap-1'>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='h-5 px-1.5 text-[10px]'
                          onClick={() => void handleCopyScript(item.content || '')}
                        >
                          <Copy className='size-2.5 mr-1' />
                          {t('curatedCopyScript')}
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='h-5 px-1.5 text-[10px]'
                          onClick={() => openPreview(item)}
                        >
                          <Maximize2 className='size-2.5 mr-1' />
                          {t('curatedPreview')}
                        </Button>
                      </div>
                    </div>
                    <pre className='max-h-36 overflow-auto p-2.5 font-mono text-[10px] leading-4 text-foreground/90 whitespace-pre-wrap break-all'>
                      {item.content || t('curatedPreviewEmpty')}
                    </pre>
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      )}

      {/* 完整脚本预览弹窗（遵循重内容 4xl/5xl 规范，Monaco 只读高亮） */}
      {/* Full script preview modal dialog (following 4xl/5xl heavy content spec, Monaco read-only) */}
      <Dialog
        open={previewOpen}
        onOpenChange={(open) => {
          if (!open) {
            setPreviewOpen(false);
            setPreviewItem(null);
          }
        }}
      >
        <DialogContent
          className='w-full sm:max-w-4xl lg:max-w-5xl h-[85vh] max-h-[85vh] flex flex-col p-0 gap-0 overflow-hidden border border-border/80 shadow-2xl'
          {...(previewItem?.description ? {} : {'aria-describedby': undefined})}
        >
          <DialogHeader className='p-6 pb-4 border-b border-border/60 shrink-0 bg-background'>
            <div className='flex flex-wrap items-center justify-between gap-2 pr-6'>
              <div className='flex items-center gap-2 min-w-0'>
                <DialogTitle className='text-base sm:text-lg font-semibold flex items-center gap-2 truncate'>
                  <Eye className='size-4 text-primary shrink-0' />
                  <span className='truncate'>
                    {previewItem?.name || t('curatedPreviewTitle')}
                  </span>
                </DialogTitle>
                {previewItem ? (
                  <div className='flex items-center gap-1.5 shrink-0'>
                    <Badge variant='secondary' className='text-xs'>
                      {getOriginLabel(previewItem.origin)}
                    </Badge>
                    <Badge variant='outline' className='text-xs font-mono'>
                      {previewItem.section}
                    </Badge>
                    {previewItem.has_override &&
                    previewItem.origin === 'builtin' ? (
                      <Badge
                        variant='outline'
                        className='text-xs text-primary border-primary/40'
                      >
                        {t('curatedHasOverride')}
                      </Badge>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </div>
            {previewItem?.description ? (
              <DialogDescription className='text-xs text-muted-foreground mt-1'>
                {previewItem.description}
              </DialogDescription>
            ) : null}
          </DialogHeader>

          <div className='flex-1 min-h-0 p-6 flex flex-col gap-2 overflow-hidden bg-muted/5'>
            <div className='flex items-center justify-between gap-2 shrink-0'>
              <span className='text-xs font-medium text-foreground flex items-center gap-1.5'>
                <FileCode className='size-3.5 text-muted-foreground' />
                <span>{t('curatedPreviewScriptHeading')}</span>
                {previewLoading ? (
                  <Loader2 className='size-3 animate-spin text-muted-foreground ml-1' />
                ) : null}
              </span>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-7 px-2.5 text-xs'
                onClick={() => void handleCopyScript(previewContent)}
              >
                <Copy className='size-3 mr-1.5' />
                {t('curatedCopyScript')}
              </Button>
            </div>
            <div className='flex-1 min-h-0 rounded-lg border border-border/60 overflow-hidden bg-background shadow-xs'>
              <MonacoEditor
                height='100%'
                theme={monacoTheme}
                language='shell'
                value={previewContent || ''}
                options={{
                  readOnly: true,
                  minimap: {enabled: false},
                  automaticLayout: true,
                  fontSize: 13,
                  lineNumbers: 'on',
                  scrollBeyondLastLine: false,
                  wordWrap: 'on',
                  renderWhitespace: 'selection',
                }}
              />
            </div>
          </div>

          <DialogFooter className='p-4 border-t border-border/60 bg-muted/15 flex items-center justify-between sm:justify-between shrink-0'>
            <div className='flex items-center gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='text-xs'
                onClick={() => void handleCopyScript(previewContent)}
              >
                <Copy className='size-3.5 mr-1.5' />
                {t('curatedCopyScript')}
              </Button>
            </div>
            <div className='flex items-center gap-2'>
              {previewItem && isOwned(previewItem) ? (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='text-xs'
                  onClick={() => {
                    const item = previewItem;
                    setPreviewOpen(false);
                    openEdit(item);
                  }}
                >
                  <Pencil className='size-3.5 mr-1.5' />
                  {t('curatedEdit')}
                </Button>
              ) : null}
              {previewItem ? (
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  className='text-xs'
                  onClick={() => {
                    const item = previewItem;
                    setPreviewOpen(false);
                    void openCopy(item);
                  }}
                >
                  <CopyPlus className='size-3.5 mr-1.5' />
                  {t('curatedCreateCopy')}
                </Button>
              ) : null}
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='text-xs'
                onClick={() => setPreviewOpen(false)}
              >
                {t('close')}
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑 / 新建 / 副本弹窗（放宽至 4xl/5xl，Monaco 专业编辑器支持） */}
      {/* Edit / create / copy draft dialog (widened to 4xl/5xl with Monaco editor) */}
      <Dialog
        open={editorOpen}
        onOpenChange={(open) => {
          if (!open && !saving) {
            setEditorOpen(false);
          }
        }}
      >
        <DialogContent
          className='w-full sm:max-w-4xl lg:max-w-5xl h-[85vh] max-h-[85vh] flex flex-col p-0 gap-0 overflow-hidden border border-border/80 shadow-2xl'
          aria-describedby={undefined}
        >
          <DialogHeader className='p-6 pb-4 border-b border-border/60 shrink-0 bg-background'>
            <div className='flex items-center justify-between gap-2 pr-6'>
              <DialogTitle className='text-base sm:text-lg font-semibold flex items-center gap-2'>
                <Pencil className='size-4 text-primary shrink-0' />
                <span>{editorTitle}</span>
              </DialogTitle>
              {editorMode === 'copy' ? (
                <Badge variant='secondary' className='text-xs'>
                  {t('curatedCreateCopy')}
                </Badge>
              ) : null}
            </div>
          </DialogHeader>

          <div className='flex-1 min-h-0 p-6 flex flex-col gap-3.5 overflow-hidden bg-muted/5'>
            <div className='grid grid-cols-1 sm:grid-cols-3 gap-3.5 shrink-0'>
              <div className='grid gap-1.5 sm:col-span-2'>
                <Label
                  htmlFor='curated-mgmt-name'
                  className='text-xs font-medium'
                >
                  {t('curatedSaveName')}
                </Label>
                <Input
                  id='curated-mgmt-name'
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  disabled={saving}
                  className='h-8 text-xs'
                />
              </div>
              <div className='grid gap-1.5 sm:col-span-1'>
                <Label
                  htmlFor='curated-mgmt-section'
                  className='text-xs font-medium'
                >
                  {t('curatedSaveSection')}
                </Label>
                <Select
                  value={editSection}
                  onValueChange={(value) =>
                    setEditSection(value as SyncCuratedSection)
                  }
                  disabled={saving}
                >
                  <SelectTrigger
                    id='curated-mgmt-section'
                    className='h-8 w-full text-xs'
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='env'>env</SelectItem>
                    <SelectItem value='source'>source</SelectItem>
                    <SelectItem value='transform'>transform</SelectItem>
                    <SelectItem value='sink'>sink</SelectItem>
                    <SelectItem value='combo'>combo</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className='grid gap-1.5 shrink-0'>
              <Label
                htmlFor='curated-mgmt-desc'
                className='text-xs font-medium'
              >
                {t('curatedSaveDescription')}
              </Label>
              <Input
                id='curated-mgmt-desc'
                value={editDescription}
                onChange={(e) => setEditDescription(e.target.value)}
                disabled={saving}
                placeholder={t('curatedSaveDescriptionPlaceholder')}
                className='h-8 text-xs'
              />
            </div>

            <div className='flex-1 min-h-0 flex flex-col gap-2'>
              <div className='flex items-center justify-between gap-2 shrink-0'>
                <div className='flex items-center gap-2'>
                  <Label
                    htmlFor='curated-mgmt-content'
                    className='text-xs font-medium flex items-center gap-1.5'
                  >
                    <FileCode className='size-3.5 text-muted-foreground' />
                    <span>{t('curatedEditContent')}</span>
                  </Label>
                  <HelpHint
                    label={t('curatedEditContent')}
                    content={t('curatedVarsHint')}
                    data-testid='curated-mgmt-content-hint'
                  />
                </div>
                <div className='flex items-center gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    className='h-6 px-2 text-[11px]'
                    disabled={!editContent.trim()}
                    onClick={() => void handleCopyScript(editContent)}
                  >
                    <Copy className='size-3 mr-1' />
                    {t('curatedCopyScript')}
                  </Button>
                </div>
              </div>
              <div className='flex-1 min-h-[280px] rounded-lg border border-border/60 overflow-hidden bg-background shadow-xs'>
                <MonacoEditor
                  height='100%'
                  theme={monacoTheme}
                  language='shell'
                  value={editContent}
                  onChange={(val) => setEditContent(val ?? '')}
                  options={{
                    readOnly: saving,
                    minimap: {enabled: false},
                    automaticLayout: true,
                    fontSize: 13,
                    lineNumbers: 'on',
                    scrollBeyondLastLine: false,
                    wordWrap: 'on',
                  }}
                />
              </div>
            </div>
          </div>

          <DialogFooter className='p-4 border-t border-border/60 bg-muted/15 flex items-center justify-end gap-2 shrink-0'>
            <Button
              type='button'
              variant='outline'
              disabled={saving}
              onClick={() => setEditorOpen(false)}
            >
              {t('cancel')}
            </Button>
            <Button
              type='button'
              disabled={saving || !editName.trim() || !editContent.trim()}
              onClick={() => void handleSave()}
            >
              {saving ? (
                <Loader2 className='size-3.5 mr-1.5 animate-spin' />
              ) : null}
              <span>{t('confirm')}</span>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={!!pendingDelete}
        onOpenChange={(open) => {
          if (!open) {
            setPendingDelete(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('curatedDeleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('curatedDeleteConfirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
              onClick={() => {
                const item = pendingDelete;
                setPendingDelete(null);
                if (item) {
                  void handleDelete(item);
                }
              }}
            >
              {t('curatedDeleteConfirmAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
