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

import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Copy, Loader2, Pencil, Plus, Trash2, Wrench} from 'lucide-react';
import {toast} from 'sonner';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
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
import {cn} from '@/lib/utils';
import type {
  SyncCuratedSection,
  SyncCuratedTemplateView,
} from '@/lib/services/sync';
import services from '@/lib/services';

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
 * 模板管理：全量列表 + 检索/类型过滤；自建/改/删/复制；系统只读需复制后改。
 * Template manage: full list + filters; create/edit/delete/copy; builtins read-only (copy to edit).
 */
export function CuratedTemplatesManagePanel({
  refreshToken = 0,
  onChanged,
}: {
  refreshToken?: number;
  onChanged?: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<SyncCuratedTemplateView[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [section, setSection] = useState<string>('all');
  const [origin, setOrigin] = useState<string>('all');
  const [query, setQuery] = useState('');
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] =
    useState<SyncCuratedTemplateView | null>(null);

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
        ? t('curatedCopy')
        : t('curatedCreate');

  return (
    <div className='flex h-full min-h-0 w-full min-w-0 flex-col gap-3.5'>
      <div className='shrink-0 rounded-lg border border-border/50 bg-muted/10 p-3 space-y-2.5'>
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
        <p className='text-[11px] leading-5 text-muted-foreground'>
          {t('curatedManageHint')}
        </p>

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
            return (
              <div
                key={key}
                className={cn(
                  'rounded-md border border-border/50 bg-background/60 px-2.5 py-2',
                  !owned && 'opacity-95',
                )}
              >
                <div className='flex items-center gap-1.5 min-w-0'>
                  <span className='text-xs font-medium truncate'>
                    {item.name}
                  </span>
                  <Badge
                    variant='secondary'
                    className='text-[9px] px-1 py-0 shrink-0'
                  >
                    {item.origin}
                  </Badge>
                  <Badge
                    variant='outline'
                    className='text-[9px] px-1 py-0 shrink-0'
                  >
                    {item.section}
                  </Badge>
                  {item.has_override && item.origin === 'builtin' ? (
                    <Badge
                      variant='outline'
                      className='text-[9px] px-1 py-0 shrink-0'
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
                <div className='mt-1.5 flex flex-wrap items-center gap-1'>
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
                  ) : (
                    <span className='text-[10px] text-muted-foreground px-1'>
                      {t('curatedBuiltinReadonly')}
                    </span>
                  )}
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    className='h-6 px-1.5 text-[10px]'
                    disabled={busy}
                    onClick={() => void openCopy(item)}
                  >
                    {busy ? (
                      <Loader2 className='size-3 mr-1 animate-spin' />
                    ) : (
                      <Copy className='size-3 mr-1' />
                    )}
                    {t('curatedCopy')}
                  </Button>
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
                </div>
              </div>
            );
          })}
        </div>
      )}

      <Dialog
        open={editorOpen}
        onOpenChange={(open) => {
          if (!open && !saving) {
            setEditorOpen(false);
          }
        }}
      >
        <DialogContent className='max-w-xl'>
          <DialogHeader>
            <DialogTitle>{editorTitle}</DialogTitle>
            <DialogDescription>{t('curatedEditDialogDesc')}</DialogDescription>
          </DialogHeader>
          <div className='grid gap-3 py-1'>
            <div className='grid gap-1.5'>
              <Label htmlFor='curated-mgmt-name'>{t('curatedSaveName')}</Label>
              <Input
                id='curated-mgmt-name'
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                disabled={saving}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label htmlFor='curated-mgmt-desc'>
                {t('curatedSaveDescription')}
              </Label>
              <Input
                id='curated-mgmt-desc'
                value={editDescription}
                onChange={(e) => setEditDescription(e.target.value)}
                disabled={saving}
                placeholder={t('curatedSaveDescriptionPlaceholder')}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label>{t('curatedSaveSection')}</Label>
              <Select
                value={editSection}
                onValueChange={(value) =>
                  setEditSection(value as SyncCuratedSection)
                }
                disabled={saving}
              >
                <SelectTrigger className='h-9 w-full text-sm'>
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
            <div className='grid gap-1.5'>
              <Label htmlFor='curated-mgmt-content'>
                {t('curatedEditContent')}
              </Label>
              <Textarea
                id='curated-mgmt-content'
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                disabled={saving}
                className='min-h-48 font-mono text-xs'
              />
            </div>
          </div>
          <DialogFooter>
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
              {saving ? <Loader2 className='size-3.5 animate-spin' /> : null}
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
