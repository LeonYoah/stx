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

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react';
import {useTranslations} from 'next-intl';
import {Container, Loader2, MoreHorizontal, Pencil, PlugZap, Trash2} from 'lucide-react';
import {useAuth} from '@/hooks/use-auth';
import {ActionButton} from '@/components/arc/action-button/action-button';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {DropdownMenu, type DropdownItem} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import {
  BatchBar,
  DetailHost,
  DirtyCloseDialog,
  TableCard,
  TruncatedText,
  useBeforeUnload,
  useDetailNarrow,
  usePagedSelection,
  type DetailNav,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {Badge} from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
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
import {cn} from '@/lib/utils';
import {toast} from '@/lib/toast';
import {
  createContainerRegistry,
  deleteContainerRegistry,
  listContainerRegistries,
  probeRegistry,
  testContainerRegistry,
  updateContainerRegistry,
  type ContainerRegistry,
  type RegistryProbeResult,
} from '@/lib/services/installer/registry.service';

type PendingEditTransition =
  | {type: 'close'}
  | {type: 'switch'; row: ContainerRegistry; origin?: HTMLElement | null};

// ContainerRegistryPanel 镜像仓库管理面板：TableCard + DetailHost（#178 零间距联动、
// 行浮起凹圆角、名称飞入与 J/K 切换）、居中 Dialog 注册、默认镜像名配置、未保存拦截与
// docker.io 锁定、BatchBar + AlertDialog 批量删除及管理员门控。
// ContainerRegistryPanel manages container registries with TableCard + DetailHost (#178
// zero-gap linked motion, active row lift with concave corners, name fly-in, and J/K nav),
// a centered create Dialog, optional default repository path, dirty-close guard + locked
// docker.io host, BatchBar + AlertDialog bulk deletion, and admin capability gating.
export function ContainerRegistryPanel() {
  const t = useTranslations('packages.registry');
  const tCommon = useTranslations('common');
  const {user} = useAuth();
  const isAdmin = Boolean(user?.is_admin);

  const [items, setItems] = useState<ContainerRegistry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // 新建弹窗状态 / Create dialog state
  const [createOpen, setCreateOpen] = useState(false);
  const [createHost, setCreateHost] = useState('');
  const [createDefaultRepo, setCreateDefaultRepo] = useState('');
  const [createUsername, setCreateUsername] = useState('');
  const [createPassword, setCreatePassword] = useState('');
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [createProbeResult, setCreateProbeResult] = useState<{ok: boolean; text: string} | null>(null);

  // 右侧编辑常驻栏状态 / Edit DetailHost rail state
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editHost, setEditHost] = useState('');
  const [editDefaultRepo, setEditDefaultRepo] = useState('');
  const [editUsername, setEditUsername] = useState('');
  const [editPassword, setEditPassword] = useState('');
  const [editSubmitting, setEditSubmitting] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [editProbeResult, setEditProbeResult] = useState<{ok: boolean; text: string} | null>(null);
  const [pendingTransition, setPendingTransition] = useState<PendingEditTransition | null>(null);
  const flyOriginRef = useRef<HTMLElement | null>(null);

  // 多选批量删除状态 / Selection & batch delete state
  const pageIds = useMemo(() => items.map((row) => row.id), [items]);
  const {
    ids: selectedIds,
    toggle: toggleSelect,
    togglePage: toggleSelectPage,
    clear: clearSelection,
    pageAllSelected,
    pageSomeSelected,
  } = usePagedSelection(pageIds);
  const [batchConfirmOpen, setBatchConfirmOpen] = useState(false);
  const [batchDeleting, setBatchDeleting] = useState(false);

  // 行内操作状态（单行探测中集合 + 单行删除确认弹窗）/ Row action state (probing set + single delete confirm)
  const [testingIds, setTestingIds] = useState<Set<number>>(() => new Set());
  const [deleteTarget, setDeleteTarget] = useState<ContainerRegistry | null>(null);
  const [deletingSingle, setDeletingSingle] = useState(false);

  const editingRow = editingId != null ? items.find((row) => row.id === editingId) ?? null : null;
  const retained = useDrawerItem(editingRow);
  const narrow = useDetailNarrow();

  const isEditDirty =
    editingRow != null &&
    (editHost.trim() !== editingRow.host ||
      editDefaultRepo.trim() !== (editingRow.default_repo || '') ||
      editUsername.trim() !== (editingRow.username || '') ||
      editPassword !== '');

  useBeforeUnload(editingId != null && isEditDirty);

  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    listContainerRegistries()
      .then(setItems)
      .catch((err) => setLoadError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const applyEditRow = useCallback(
    (row: ContainerRegistry, origin?: HTMLElement | null) => {
      // 仅点击/回车打开或切换不同行时写入 flyOrigin；J/K 与 ↑↓ 导航不触发飞入。
      // Only record flyOrigin on click/Enter for a different row; J/K and arrow nav skip fly-in.
      if (origin && editingId !== row.id) {
        flyOriginRef.current = origin;
      } else {
        flyOriginRef.current = null;
      }
      setEditingId(row.id);
      setEditHost(row.host);
      setEditDefaultRepo(row.default_repo || '');
      setEditUsername(row.username || '');
      // 密码永不出库；留空保持既有密文。
      // Stored passwords never leave the backend; an empty field keeps the secret.
      setEditPassword('');
      setEditError(null);
      setEditProbeResult(null);
    },
    [editingId],
  );

  const openCreate = () => {
    if (!isAdmin) {
      return;
    }
    setCreateHost('');
    setCreateDefaultRepo('');
    setCreateUsername('');
    setCreatePassword('');
    setCreateError(null);
    setCreateProbeResult(null);
    setCreateOpen(true);
  };

  const openEdit = useCallback(
    (row: ContainerRegistry, origin?: HTMLElement | null) => {
      if (!isAdmin) {
        return;
      }
      if (editingRow && editingRow.id !== row.id && isEditDirty) {
        setPendingTransition({type: 'switch', row, origin});
        return;
      }
      applyEditRow(row, origin);
    },
    [isAdmin, editingRow, isEditDirty, applyEditRow],
  );

  const requestCloseEdit = useCallback(() => {
    if (isEditDirty) {
      setPendingTransition({type: 'close'});
      return;
    }
    flyOriginRef.current = null;
    setEditingId(null);
    setEditError(null);
    setEditProbeResult(null);
  }, [isEditDirty]);

  const confirmDiscardEdit = () => {
    const next = pendingTransition;
    setPendingTransition(null);
    if (!next || next.type === 'close') {
      flyOriginRef.current = null;
      setEditingId(null);
      setEditError(null);
      setEditProbeResult(null);
      return;
    }
    applyEditRow(next.row, next.origin);
  };

  const editingIndex = editingId != null ? items.findIndex((row) => row.id === editingId) : -1;
  const detailNav = useMemo<DetailNav | undefined>(() => {
    if (!isAdmin || editingIndex < 0 || items.length <= 1) {
      return undefined;
    }
    return {
      index: editingIndex,
      total: items.length,
      onPrev: () => {
        if (editingIndex > 0) {
          openEdit(items[editingIndex - 1]);
        }
      },
      onNext: () => {
        if (editingIndex < items.length - 1) {
          openEdit(items[editingIndex + 1]);
        }
      },
    };
  }, [isAdmin, editingIndex, items, openEdit]);

  const onRowKeyDown = (event: KeyboardEvent<HTMLElement>, row: ContainerRegistry) => {
    if (event.target !== event.currentTarget) {
      return;
    }
    const rowEls = Array.from(
      event.currentTarget.parentElement?.querySelectorAll<HTMLElement>('[data-mrow]') ?? [],
    );
    const index = rowEls.indexOf(event.currentTarget);
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      rowEls[index + (event.key === 'ArrowDown' ? 1 : -1)]?.focus();
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      openEdit(row, event.currentTarget);
    }
  };

  const formatProbeResult = (result: RegistryProbeResult): {ok: boolean; text: string} => {
    if (result.status === 'error' || result.error) {
      return {
        ok: false,
        text: result.error ? `${t('probeFailed')}: ${result.error}` : t('probeFailed'),
      };
    }
    if (result.status === 'public') {
      return {ok: true, text: t('probePublic')};
    }
    if (result.status === 'reachable') {
      return {ok: true, text: t('probeReachable')};
    }
    return {ok: true, text: t('probeOk')};
  };

  const probeCreate = async () => {
    setCreateProbeResult(null);
    try {
      const result = await probeRegistry({
        host: createHost.trim(),
        default_repo: createDefaultRepo.trim(),
        username: createUsername.trim(),
        password: createPassword,
      });
      const feedback = formatProbeResult(result);
      setCreateProbeResult(feedback);
      if (!feedback.ok) {
        throw new Error(result.error || t('probeFailed'));
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setCreateProbeResult({ok: false, text: `${t('probeFailed')}: ${msg}`});
      throw err;
    }
  };

  const probeEdit = async () => {
    if (!editingRow) {
      return;
    }
    setEditProbeResult(null);
    try {
      const result = await probeRegistry({
        id: editingRow.id,
        host: editingRow.is_system ? editingRow.host : editHost.trim(),
        default_repo: editDefaultRepo.trim(),
        username: editUsername.trim(),
        password: editPassword,
      });
      const feedback = formatProbeResult(result);
      setEditProbeResult(feedback);
      if (!feedback.ok) {
        throw new Error(result.error || t('probeFailed'));
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setEditProbeResult({ok: false, text: `${t('probeFailed')}: ${msg}`});
      throw err;
    }
  };

  const submitCreate = async () => {
    setCreateSubmitting(true);
    setCreateError(null);
    try {
      await createContainerRegistry({
        host: createHost.trim(),
        default_repo: createDefaultRepo.trim(),
        username: createUsername.trim(),
        password: createPassword,
      });
      setCreateOpen(false);
      load();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : String(err));
    } finally {
      setCreateSubmitting(false);
    }
  };

  const submitEdit = async () => {
    if (!editingRow) {
      return;
    }
    setEditSubmitting(true);
    setEditError(null);
    try {
      await updateContainerRegistry(editingRow.id, {
        host: editingRow.is_system ? editingRow.host : editHost.trim(),
        default_repo: editDefaultRepo.trim(),
        username: editUsername.trim(),
        password: editPassword,
      });
      flyOriginRef.current = null;
      setEditingId(null);
      load();
    } catch (err) {
      setEditError(err instanceof Error ? err.message : String(err));
    } finally {
      setEditSubmitting(false);
    }
  };

  const testRow = async (id: number) => {
    if (testingIds.has(id)) {
      return;
    }
    setTestingIds((prev) => new Set(prev).add(id));
    try {
      const row = await testContainerRegistry(id);
      setItems((prev) => prev.map((item) => (item.id === id ? row : item)));
      if (row.probe_status === 'error') {
        toast.error(row.probe_error || t('probeFailed'));
      } else if (row.probe_status === 'public') {
        toast.success(t('probePublic'));
      } else {
        toast.success(t('probeOk'));
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setItems((prev) =>
        prev.map((item) =>
          item.id === id ? {...item, probe_status: 'error', probe_error: msg} : item,
        ),
      );
      toast.error(msg || t('probeFailed'));
    } finally {
      setTestingIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    }
  };

  const confirmDeleteSingle = async () => {
    if (!deleteTarget) {
      return;
    }
    const target = deleteTarget;
    setDeletingSingle(true);
    try {
      await deleteContainerRegistry(target.id);
      toggleSelect(target.id, false);
      if (editingId === target.id) {
        flyOriginRef.current = null;
        setEditingId(null);
        setEditError(null);
        setEditProbeResult(null);
      }
      setDeleteTarget(null);
      toast.success(t('batchDeleteSuccess', {count: 1}));
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setDeletingSingle(false);
    }
  };

  const deletableSelectedIds = useMemo(
    () =>
      Array.from(selectedIds).filter((id) =>
        items.some((row) => row.id === id && !row.is_system),
      ),
    [items, selectedIds],
  );

  const hasSelectedSystemRow = useMemo(
    () =>
      Array.from(selectedIds).some((id) =>
        items.some((row) => row.id === id && row.is_system),
      ),
    [items, selectedIds],
  );

  const executeBatchDelete = async () => {
    if (deletableSelectedIds.length === 0) {
      setBatchConfirmOpen(false);
      return;
    }
    setBatchDeleting(true);
    let succeeded = 0;
    let failed = 0;
    for (const id of deletableSelectedIds) {
      try {
        await deleteContainerRegistry(id);
        succeeded++;
      } catch {
        failed++;
      }
    }
    setBatchDeleting(false);
    setBatchConfirmOpen(false);
    clearSelection();
    if (editingId != null && deletableSelectedIds.includes(editingId)) {
      flyOriginRef.current = null;
      setEditingId(null);
      setEditError(null);
      setEditProbeResult(null);
    }
    if (failed === 0) {
      toast.success(t('batchDeleteSuccess', {count: succeeded}));
    } else if (succeeded > 0) {
      toast.warning(t('batchDeletePartial', {succeeded, failed}));
    } else {
      toast.error(t('batchDeletePartial', {succeeded, failed}));
    }
    load();
  };

  const statusLabels: Record<string, string> = {
    ok: t('status.ok'),
    public: t('status.public'),
    reachable: t('status.reachable'),
    error: t('status.error'),
    failed: t('status.failed'),
    unknown: t('status.unknown'),
  };

  const renderStatusBadge = (row: ContainerRegistry) => {
    const status = row.probe_status || 'unknown';
    const label = t.has(`status.${status}`) ? (statusLabels[status] ?? status) : statusLabels.unknown;
    let badge: React.ReactNode;
    if (status === 'ok' || status === 'public') {
      badge = (
        <Badge
          variant='secondary'
          className='shrink-0 border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
        >
          {label}
        </Badge>
      );
    } else if (status === 'reachable') {
      badge = (
        <Badge
          variant='outline'
          className='shrink-0 border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300'
        >
          {label}
        </Badge>
      );
    } else if (status === 'error' || status === 'failed') {
      badge = (
        <Badge variant='destructive' className='shrink-0'>
          {label}
        </Badge>
      );
    } else {
      badge = (
        <Badge variant='secondary' className='shrink-0'>
          {label}
        </Badge>
      );
    }

    return (
      <span className='inline-flex min-w-0 items-center gap-2'>
        {badge}
        {row.probe_error ? (
          <TruncatedText
            text={row.probe_error}
            className='min-w-0 flex-1 text-xs text-muted-foreground'
          />
        ) : null}
      </span>
    );
  };

  return (
    <div
      className={cn(layout.page, 'flex-1')}
      data-testid='container-registry-panel'
    >
      <div className={layout.toolbar}>
        {!isAdmin ? (
          <p className='text-xs text-muted-foreground' data-testid='registry-readonly-hint'>
            {t('readOnlyHint')}
          </p>
        ) : items.length > 0 ? (
          <span className={cn(layout.count, 'ml-0 text-xs')}>
            {items.length} {items.length === 1 ? t('itemOne') : t('itemOther')}
          </span>
        ) : null}
        {isAdmin ? (
          <div className={layout.toolbarRight}>
            <Button type='button' size='sm' onClick={openCreate} data-testid='registry-add'>
              {t('add')}
            </Button>
          </div>
        ) : null}
      </div>

      <div className={cn(layout.split, layout.splitStretch)}>
        <div
          className={layout.listCol}
          data-mcols={isAdmin ? 'registries' : 'registries-readonly'}
        >
          <TableCard
            loading={loading}
            hasData={items.length > 0}
            skeletonColumns={isAdmin ? 5 : 3}
            skeletonRows={4}
            error={loadError}
            onRetry={load}
            empty={
              <EmptyState
                icon={<Container />}
                title={t('empty')}
                description={t('subtitle')}
                action={
                  isAdmin ? (
                    <Button type='button' variant='secondary' size='sm' onClick={openCreate}>
                      {t('emptyAction')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <div className={layout.rowHead}>
              {isAdmin ? (
                <Checkbox
                  checked={
                    pageAllSelected ? true : pageSomeSelected ? 'indeterminate' : false
                  }
                  onCheckedChange={(checked) => toggleSelectPage(checked === true)}
                  aria-label={tCommon('selectAll')}
                />
              ) : null}
              <span className='truncate'>{t('colHost')}</span>
              <span className='truncate'>{t('colAuth')}</span>
              <span className='truncate'>{t('colProbe')}</span>
              {isAdmin ? (
                <span className={cn(layout.right, 'truncate')}>{t('colActions')}</span>
              ) : null}
            </div>

            <TooltipProvider delayDuration={180}>
              <div className={layout.tableScroll}>
                {items.map((row) => {
                  const isActive = editingId === row.id;
                  const isTesting = testingIds.has(row.id);
                  const menuItems: DropdownItem[] = [
                    {
                      label: tCommon('edit'),
                      icon: <Pencil className='size-4' aria-hidden />,
                      onSelect: () => openEdit(row),
                    },
                    {
                      label: t('delete'),
                      icon: <Trash2 className='size-4' aria-hidden />,
                      destructive: true,
                      separatorBefore: true,
                      disabled: Boolean(row.is_system),
                      onSelect: () => setDeleteTarget(row),
                    },
                  ];
                  return (
                    <div
                      key={row.id}
                      data-mrow={row.id}
                      data-testid={`registry-row-${row.id}`}
                      role={isAdmin ? 'button' : undefined}
                      tabIndex={isAdmin ? 0 : undefined}
                      onClick={
                        isAdmin
                          ? (event) => {
                              if (
                                (event.target as HTMLElement).closest(
                                  'button, a, input, select, [role="menu"], [role="menuitem"], [role="checkbox"]',
                                )
                              ) {
                                return;
                              }
                              openEdit(row, event.currentTarget);
                            }
                          : undefined
                      }
                      onKeyDown={isAdmin ? (event) => onRowKeyDown(event, row) : undefined}
                      className={cn(
                        layout.row,
                        !isAdmin && 'cursor-default',
                        isActive && layout.rowActive,
                      )}
                    >
                      {isAdmin ? (
                        <Checkbox
                          checked={selectedIds.has(row.id)}
                          onCheckedChange={(checked) => toggleSelect(row.id, checked === true)}
                          aria-label={row.host}
                        />
                      ) : null}

                      <div className={layout.rowName}>
                        <div className={layout.rowNameInline}>
                          {isAdmin ? (
                            <button
                              type='button'
                              className='min-w-0 truncate text-left text-primary underline-offset-2 hover:underline'
                              onClick={(event) => {
                                event.stopPropagation();
                                openEdit(
                                  row,
                                  event.currentTarget.closest<HTMLElement>('[data-mrow]'),
                                );
                              }}
                              data-testid={`registry-open-${row.id}`}
                            >
                              <TruncatedText
                                text={row.host}
                                className={cn(layout.name, 'text-xs')}
                                mono
                                detailName
                              />
                            </button>
                          ) : (
                            <TruncatedText
                              text={row.host}
                              className={cn(layout.name, 'text-xs')}
                              mono
                            />
                          )}
                          {row.is_system ? (
                            <Badge variant='secondary' className='shrink-0'>
                              {t('officialBadge')}
                            </Badge>
                          ) : null}
                        </div>
                        {row.default_repo ? (
                          <TruncatedText
                            text={row.default_repo}
                            className={layout.sub}
                            mono
                          />
                        ) : null}
                      </div>

                      <TruncatedText
                        text={row.has_auth ? row.username || '•••' : t('anonymous')}
                        className='text-xs text-foreground'
                      />

                      {renderStatusBadge(row)}

                      {isAdmin ? (
                        <div
                          className={cn(layout.rowActions, 'gap-1')}
                          onClick={(event) => event.stopPropagation()}
                          onKeyDown={(event) => event.stopPropagation()}
                        >
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                type='button'
                                variant='ghost'
                                size='sm'
                                iconOnly
                                disabled={isTesting}
                                aria-label={isTesting ? t('testing') : t('test')}
                                onClick={() => void testRow(row.id)}
                                data-testid={`registry-test-${row.id}`}
                              >
                                {isTesting ? (
                                  <Loader2 className='size-4 animate-spin' aria-hidden />
                                ) : (
                                  <PlugZap className='size-4' aria-hidden />
                                )}
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>
                              {isTesting ? t('testing') : t('test')}
                            </TooltipContent>
                          </Tooltip>
                          <DropdownMenu
                            trigger={
                              <Button
                                type='button'
                                variant='ghost'
                                size='sm'
                                iconOnly
                                aria-label={tCommon('moreActions')}
                                data-testid={`registry-more-${row.id}`}
                              >
                                <MoreHorizontal className='size-4' aria-hidden />
                              </Button>
                            }
                            items={menuItems}
                          />
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            </TooltipProvider>
          </TableCard>

          {isAdmin ? (
            <BatchBar
              selectedCount={deletableSelectedIds.length}
              onClear={clearSelection}
              result={hasSelectedSystemRow ? t('systemSkippedHint') : undefined}
              testId='registry-batch-bar'
            >
              <Button
                type='button'
                variant='secondary'
                size='sm'
                disabled={deletableSelectedIds.length === 0 || batchDeleting}
                onClick={() => setBatchConfirmOpen(true)}
                data-testid='registry-batch-delete'
              >
                {t('deleteSelected')}
              </Button>
            </BatchBar>
          ) : null}
        </div>

        <DetailHost
          open={editingId != null}
          hasContent={retained != null}
          onOpenChange={(open) => {
            if (!open) {
              requestCloseEdit();
            }
          }}
          narrow={narrow}
          title={retained?.host || t('editDialogTitle')}
          meta={
            retained
              ? retained.is_system
                ? `${t('editDialogTitle')} · ${t('officialBadge')}`
                : t('editDialogTitle')
              : undefined
          }
          nameSlot='title'
          flyOrigin={flyOriginRef}
          nav={detailNav}
          drawerWidth='w-[min(92vw,480px)]'
        >
          {retained ? (
            <div
              key={retained.id}
              className={layout.detailCol}
              data-testid='registry-edit-detail'
            >
              <div className={layout.detailScroll}>
                <section className={cn(layout.detailSection, layout.detailSectionFirst)}>
                  <div className='space-y-3'>
                    <div className='space-y-1'>
                      <Input
                        label={t('hostLabel')}
                        value={editHost}
                        disabled={Boolean(retained.is_system)}
                        placeholder={t('hostPlaceholder')}
                        onChange={(event) => setEditHost(event.target.value)}
                        data-testid='registry-edit-host-input'
                      />
                      {retained.is_system ? (
                        <p
                          className='text-xs text-muted-foreground'
                          data-testid='registry-host-immutable'
                        >
                          {t('hostImmutable')}
                        </p>
                      ) : null}
                    </div>
                    <div className='space-y-1'>
                      <Input
                        label={t('defaultRepoLabel')}
                        value={editDefaultRepo}
                        placeholder={t('defaultRepoPlaceholder')}
                        onChange={(event) => setEditDefaultRepo(event.target.value)}
                        data-testid='registry-edit-default-repo-input'
                      />
                      <p className='text-xs text-muted-foreground'>{t('defaultRepoHint')}</p>
                    </div>
                    <Input
                      label={t('usernameLabel')}
                      value={editUsername}
                      onChange={(event) => setEditUsername(event.target.value)}
                      data-testid='registry-edit-username-input'
                    />
                    <Input
                      label={t('passwordLabel')}
                      type='password'
                      value={editPassword}
                      placeholder={t('passwordKeepHint')}
                      onChange={(event) => setEditPassword(event.target.value)}
                      data-testid='registry-edit-password-input'
                    />
                  </div>
                </section>

                <section className={layout.detailSection}>
                  <div className='flex flex-wrap items-center gap-2'>
                    <ActionButton
                      label={t('probeFirst')}
                      pendingLabel={t('testing')}
                      successLabel={t('probeOk')}
                      disabled={!editHost.trim()}
                      onAction={probeEdit}
                      data-testid='registry-edit-probe'
                    />
                    {editProbeResult ? (
                      <p
                        className={`min-w-0 flex-1 text-xs ${editProbeResult.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive'}`}
                        data-testid='registry-edit-probe-result'
                      >
                        {editProbeResult.text}
                      </p>
                    ) : null}
                  </div>
                  {editError ? <p className='text-xs text-destructive'>{editError}</p> : null}
                </section>
              </div>

              <div className={layout.detailFoot}>
                <span className={layout.detailFootGrow} />
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={requestCloseEdit}
                  disabled={editSubmitting}
                >
                  {t('cancel')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  disabled={editSubmitting || !editHost.trim()}
                  onClick={submitEdit}
                  data-testid='registry-edit-submit'
                >
                  {editSubmitting ? t('submitting') : t('submitEdit')}
                </Button>
              </div>
            </div>
          ) : null}
        </DetailHost>
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent
          title={t('dialogTitle')}
          className='arc-compact w-[min(92vw,480px)]!'
          data-testid='registry-create-dialog'
        >
          <div className='space-y-3'>
            <Input
              label={t('hostLabel')}
              value={createHost}
              placeholder={t('hostPlaceholder')}
              onChange={(event) => setCreateHost(event.target.value)}
              data-testid='registry-host-input'
            />
            <div className='space-y-1'>
              <Input
                label={t('defaultRepoLabel')}
                value={createDefaultRepo}
                placeholder={t('defaultRepoPlaceholder')}
                onChange={(event) => setCreateDefaultRepo(event.target.value)}
                data-testid='registry-default-repo-input'
              />
              <p className='text-xs text-muted-foreground'>{t('defaultRepoHint')}</p>
            </div>
            <Input
              label={t('usernameLabel')}
              value={createUsername}
              onChange={(event) => setCreateUsername(event.target.value)}
              data-testid='registry-username-input'
            />
            <Input
              label={t('passwordLabel')}
              type='password'
              value={createPassword}
              onChange={(event) => setCreatePassword(event.target.value)}
              data-testid='registry-password-input'
            />
            <div className='flex items-center gap-2'>
              <ActionButton
                label={t('probeFirst')}
                pendingLabel={t('testing')}
                successLabel={t('probeOk')}
                disabled={!createHost.trim()}
                onAction={probeCreate}
                data-testid='registry-probe'
              />
              {createProbeResult ? (
                <p
                  className={`text-xs ${createProbeResult.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive'}`}
                  data-testid='registry-probe-result'
                >
                  {createProbeResult.text}
                </p>
              ) : null}
            </div>
            {createError ? <p className='text-xs text-destructive'>{createError}</p> : null}
          </div>
          <div className='mt-4 flex justify-end gap-2'>
            <Button type='button' variant='secondary' size='sm' onClick={() => setCreateOpen(false)}>
              {t('cancel')}
            </Button>
            <Button
              type='button'
              size='sm'
              disabled={createSubmitting || !createHost.trim()}
              onClick={submitCreate}
              data-testid='registry-submit'
            >
              {createSubmitting ? t('submitting') : t('submit')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && !deletingSingle) {
            setDeleteTarget(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('deleteDialogTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('deleteDialogDesc', {host: deleteTarget?.host || ''})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletingSingle}>{t('cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={deletingSingle}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
              onClick={() => void confirmDeleteSingle()}
              data-testid='registry-single-delete-confirm'
            >
              {deletingSingle ? t('submitting') : t('deleteDialogConfirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={batchConfirmOpen} onOpenChange={setBatchConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('batchDeleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('batchDeleteDesc', {count: deletableSelectedIds.length})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={batchDeleting}>{t('cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={batchDeleting}
              onClick={() => void executeBatchDelete()}
              data-testid='registry-batch-confirm'
            >
              {t('deleteConfirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <DirtyCloseDialog
        open={pendingTransition !== null}
        onDiscard={confirmDiscardEdit}
        onKeepEditing={() => setPendingTransition(null)}
      />
    </div>
  );
}
