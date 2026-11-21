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

/**
 * 用户管理：TableCard + DetailHost 常驻详情编辑（#178 联动/飞入 + DirtyCloseDialog）+
 * usePagedSelection/BatchBar；新建用户走居中 Arc 弹窗，破坏性确认走 AlertDialog；
 * 不能操作自己（批量跳过）。
 * User management: TableCard + persistent DetailHost editing (#178 linked motion/fly-in +
 * DirtyCloseDialog) + usePagedSelection/BatchBar; creation uses a centered Arc dialog and
 * destructive confirms stay on AlertDialog; self is never mutated (batch skip).
 */

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  MoreHorizontal,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
  Users,
  UserX,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Switch} from '@/components/arc/switch/switch';
import {Badge} from '@/components/ui/badge';
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
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {toast} from '@/lib/toast';
import {
  BatchBar,
  DetailHost,
  DirtyCloseDialog,
  TableCard,
  WorkspaceHeader,
  useBeforeUnload,
  useDetailNarrow,
  usePagedSelection,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {useServerListQuery} from '@/lib/hooks/use-server-list-query';
import {useAuth} from '@/hooks/use-auth';
import services from '@/lib/services';
import type {
  CreateUserRequest,
  UpdateUserRequest,
  UserInfo,
} from '@/lib/services/admin/user.service';
import {formatDateTimeTz, formatDateTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {AccountsNavTabs} from './AccountsNavTabs';

function isValidEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

const DEFAULT_PAGE_SIZE = 10;

type BatchKind = 'delete' | 'deactivate';
type PendingDetailTarget =
  | {kind: 'close'}
  | {kind: 'user'; user: UserInfo; origin?: HTMLElement | null};

export function UserManagement() {
  const t = useTranslations();
  const narrow = useDetailNarrow();
  // 当前登录用户：不能停用自己或取消自己的管理员权限（后端同样拒绝），避免唯一管理员把自己锁在门外。
  // Current user: cannot deactivate themselves or revoke their own admin permission (the backend rejects it too),
  // so a sole admin cannot lock themselves out.
  const {user: currentUser} = useAuth();
  const isSelf = useCallback(
    (user: UserInfo | null) =>
      Boolean(user && currentUser && user.id === currentUser.id),
    [currentUser],
  );

  const {page, pageSize, filters, setPage, setPageSize, patchFilters, resetFilters} =
    useServerListQuery({
      initialFilters: {username: ''},
      defaultPageSize: DEFAULT_PAGE_SIZE,
    });

  const [usernameDraft, setUsernameDraft] = useState(filters.username);
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // 新建弹窗独立表单态，避免与右侧常驻详情编辑态互相覆盖。
  // Separate state for the create dialog so creating a user never clobbers an open detail rail.
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreateUserRequest>({
    username: '',
    password: '',
    nickname: '',
    email: '',
    is_admin: false,
  });

  // 常驻详情栏状态（含脏表单拦截与名称飞入来源行）。
  // Persistent detail rail state (with dirty-form interception and name fly-in origin).
  const [activeUserId, setActiveUserId] = useState<number | null>(null);
  const [activeUser, setActiveUser] = useState<UserInfo | null>(null);
  const [editForm, setEditForm] = useState<{
    password: string;
    nickname: string;
    email: string;
    is_admin: boolean;
  }>({
    password: '',
    nickname: '',
    email: '',
    is_admin: false,
  });
  const [savingEdit, setSavingEdit] = useState(false);
  const [pendingTarget, setPendingTarget] =
    useState<PendingDetailTarget | null>(null);
  const flyOriginRef = useRef<HTMLElement | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<UserInfo | null>(null);
  const [deactivateTarget, setDeactivateTarget] = useState<UserInfo | null>(
    null,
  );
  const [batchBusy, setBatchBusy] = useState(false);
  const [batchConfirm, setBatchConfirm] = useState<BatchKind | null>(null);
  const [batchResult, setBatchResult] = useState<{
    ok: number;
    skipped: number;
    failed: number;
  } | null>(null);

  const editDirty = useMemo(() => {
    if (!activeUser) {
      return false;
    }
    return (
      editForm.nickname !== (activeUser.nickname || '') ||
      (editForm.email || '') !== (activeUser.email || '') ||
      Boolean(editForm.password) ||
      editForm.is_admin !== activeUser.is_admin
    );
  }, [activeUser, editForm]);

  useBeforeUnload(activeUserId !== null && editDirty);

  const drawerOpen = activeUserId !== null;
  const drawerUser = useDrawerItem(activeUser);

  const pageIds = useMemo(() => users.map((u) => u.id), [users]);
  const {
    ids: selectedIds,
    setIds: setSelectedIds,
    toggle: toggleSelect,
    togglePage: toggleSelectPage,
    clear: clearSelection,
    pageAllSelected,
    pageSomeSelected,
  } = usePagedSelection(pageIds);

  const listRequestIdRef = useRef(0);
  const editDirtyRef = useRef(editDirty);
  editDirtyRef.current = editDirty;

  const loadUsers = useCallback(async () => {
    const requestId = listRequestIdRef.current + 1;
    listRequestIdRef.current = requestId;
    setLoading(true);
    try {
      const response = await services.adminUser.listUsers({
        current: page,
        size: pageSize,
        username: filters.username || undefined,
      });
      if (listRequestIdRef.current !== requestId) {
        return;
      }
      const nextUsers = response.users || [];
      setUsers(nextUsers);
      setTotal(response.total || 0);
      setError(null);
      // 详情未脏时用最新列表记录同步头部与状态（例如表格里切换了启用状态）。
      // Sync open detail metadata/status from fresh list rows when not dirty.
      setActiveUser((prev) => {
        if (!prev) {
          return prev;
        }
        const fresh = nextUsers.find((u) => u.id === prev.id);
        if (!fresh) {
          return prev;
        }
        if (!editDirtyRef.current) {
          setEditForm({
            password: '',
            nickname: fresh.nickname || '',
            email: fresh.email || '',
            is_admin: fresh.is_admin,
          });
        }
        return fresh;
      });
    } catch (err) {
      if (listRequestIdRef.current !== requestId) {
        return;
      }
      setError(err instanceof Error ? err.message : t('common.error'));
    } finally {
      if (listRequestIdRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [page, pageSize, filters.username, t]);

  useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  const commitUsername = useCallback(() => {
    const next = usernameDraft.trim();
    if (next !== filters.username) {
      patchFilters({username: next});
    }
  }, [usernameDraft, filters.username, patchFilters]);

  const applyUserSelection = useCallback(
    (user: UserInfo, origin?: HTMLElement | null) => {
      flyOriginRef.current = origin ?? null;
      setActiveUserId(user.id);
      setActiveUser(user);
      setEditForm({
        password: '',
        nickname: user.nickname || '',
        email: user.email || '',
        is_admin: user.is_admin,
      });
    },
    [],
  );

  const closeDetailImmediate = useCallback(() => {
    flyOriginRef.current = null;
    setActiveUserId(null);
    setActiveUser(null);
    setEditForm({
      password: '',
      nickname: '',
      email: '',
      is_admin: false,
    });
  }, []);

  const requestSelectUser = useCallback(
    (user: UserInfo, origin?: HTMLElement | null) => {
      if (activeUserId === user.id) {
        return;
      }
      if (activeUserId !== null && editDirty) {
        setPendingTarget({kind: 'user', user, origin});
        return;
      }
      applyUserSelection(user, origin);
    },
    [activeUserId, editDirty, applyUserSelection],
  );

  const requestCloseDetail = useCallback(() => {
    if (activeUserId !== null && editDirty) {
      setPendingTarget({kind: 'close'});
      return;
    }
    closeDetailImmediate();
  }, [activeUserId, editDirty, closeDetailImmediate]);

  const discardAndCommit = useCallback(() => {
    const target = pendingTarget;
    setPendingTarget(null);
    if (!target || target.kind === 'close') {
      closeDetailImmediate();
      return;
    }
    applyUserSelection(target.user, target.origin);
  }, [pendingTarget, closeDetailImmediate, applyUserSelection]);

  const handleOpenCreate = () => {
    setCreateForm({
      username: '',
      password: '',
      nickname: '',
      email: '',
      is_admin: false,
    });
    setIsCreateDialogOpen(true);
  };

  const handleOpenDelete = (user: UserInfo) => {
    if (isSelf(user)) {
      toast.error(t('admin.userManagement.errors.cannotDeleteSelf'));
      return;
    }
    setDeleteTarget(user);
  };

  const handleCreate = async () => {
    if (!createForm.username || createForm.username.length < 3) {
      toast.error(t('admin.userManagement.errors.usernameTooShort'));
      return;
    }
    if (!createForm.password || createForm.password.length < 6) {
      toast.error(t('admin.userManagement.errors.passwordTooShort'));
      return;
    }
    if (createForm.email && !isValidEmail(createForm.email)) {
      toast.error(t('admin.userManagement.errors.invalidEmail'));
      return;
    }

    try {
      await services.adminUser.createUser({
        username: createForm.username,
        password: createForm.password,
        nickname: createForm.nickname,
        email: createForm.email?.trim(),
        is_admin: createForm.is_admin,
      });
      toast.success(t('admin.userManagement.createSuccess'));
      setIsCreateDialogOpen(false);
      void loadUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.error'));
    }
  };

  const handleUpdate = async () => {
    if (!activeUser || savingEdit) {
      return;
    }

    const updateData: UpdateUserRequest = {};
    if (editForm.nickname !== (activeUser.nickname || '')) {
      updateData.nickname = editForm.nickname;
    }
    if ((editForm.email || '') !== (activeUser.email || '')) {
      if (editForm.email && !isValidEmail(editForm.email)) {
        toast.error(t('admin.userManagement.errors.invalidEmail'));
        return;
      }
      updateData.email = editForm.email?.trim() || '';
    }
    if (editForm.password) {
      if (editForm.password.length < 6) {
        toast.error(t('admin.userManagement.errors.passwordTooShort'));
        return;
      }
      updateData.password = editForm.password;
    }
    if (editForm.is_admin !== activeUser.is_admin) {
      updateData.is_admin = editForm.is_admin;
    }

    setSavingEdit(true);
    try {
      await services.adminUser.updateUser(activeUser.id, updateData);
      toast.success(t('admin.userManagement.updateSuccess'));
      const updated: UserInfo = {
        ...activeUser,
        nickname: editForm.nickname,
        email: editForm.email?.trim() || '',
        is_admin: editForm.is_admin,
      };
      setActiveUser(updated);
      setEditForm({
        password: '',
        nickname: updated.nickname || '',
        email: updated.email || '',
        is_admin: updated.is_admin,
      });
      void loadUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.error'));
    } finally {
      setSavingEdit(false);
    }
  };

  const handleDelete = async () => {
    const target = deleteTarget;
    if (!target || isSelf(target)) {
      return;
    }
    try {
      await services.adminUser.deleteUser(target.id);
      toast.success(t('admin.userManagement.deleteSuccess'));
      setDeleteTarget(null);
      if (activeUserId === target.id) {
        closeDetailImmediate();
      }
      {
        const next = new Set(selectedIds);
        next.delete(target.id);
        setSelectedIds(next);
      }
      void loadUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.error'));
    }
  };

  const updateActive = async (user: UserInfo, active: boolean) => {
    try {
      await services.adminUser.updateUser(user.id, {is_active: active});
      toast.success(t('admin.userManagement.updateSuccess'));
      void loadUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.error'));
    }
  };

  const handleToggleActive = (user: UserInfo) => {
    if (isSelf(user)) {
      return;
    }
    if (user.is_active) {
      setDeactivateTarget(user);
      return;
    }
    void updateActive(user, true);
  };

  const handleConfirmDeactivate = async () => {
    const target = deactivateTarget;
    setDeactivateTarget(null);
    if (target) {
      await updateActive(target, false);
    }
  };

  const runBatch = useCallback(
    async (kind: BatchKind) => {
      const ids = Array.from(selectedIds);
      if (ids.length === 0 || batchBusy) {
        return;
      }
      setBatchBusy(true);
      setBatchResult(null);
      const outcome = {ok: 0, skipped: 0, failed: 0};
      const failedIds: number[] = [];
      let skippedSelf = false;
      const byId = new Map(users.map((u) => [u.id, u]));
      for (const id of ids) {
        if (currentUser && id === currentUser.id) {
          outcome.skipped += 1;
          skippedSelf = true;
          continue;
        }
        if (kind === 'deactivate') {
          const known = byId.get(id);
          if (known && !known.is_active) {
            outcome.skipped += 1;
            continue;
          }
          try {
            await services.adminUser.updateUser(id, {is_active: false});
            outcome.ok += 1;
          } catch {
            outcome.failed += 1;
            failedIds.push(id);
          }
        } else {
          try {
            await services.adminUser.deleteUser(id);
            outcome.ok += 1;
            if (activeUserId === id) {
              closeDetailImmediate();
            }
          } catch {
            outcome.failed += 1;
            failedIds.push(id);
          }
        }
      }
      setBatchResult(outcome);
      setBatchBusy(false);
      setBatchConfirm(null);
      // 保留失败项勾选以便直接重试，清除已成功或跳过项。Keep failed items selected for retry; clear succeeded/skipped ones.
      setSelectedIds(new Set(failedIds));
      void loadUsers();
      if (skippedSelf) {
        toast.message(t('admin.userManagement.batchSkipSelf'));
      }
      const summary = t('admin.userManagement.batchResult', {
        ok: outcome.ok,
        skipped: outcome.skipped,
        failed: outcome.failed,
      });
      if (outcome.failed > 0 && outcome.ok === 0) {
        toast.error(summary);
      } else if (outcome.failed > 0 || outcome.skipped > 0) {
        toast.warning(summary);
      } else {
        toast.success(summary);
      }
    },
    [
      selectedIds,
      batchBusy,
      users,
      currentUser,
      activeUserId,
      closeDetailImmediate,
      setSelectedIds,
      loadUsers,
      t,
    ],
  );

  const filtersActive = Boolean(filters.username);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const activeIndex = activeUser
    ? users.findIndex((u) => u.id === activeUser.id)
    : -1;
  const detailNav =
    activeUser && activeIndex >= 0
      ? {
          index: activeIndex,
          total: users.length,
          // J/K / 头部导航不写 flyOrigin。J/K and header nav never write flyOrigin.
          onPrev: () =>
            activeIndex > 0 && requestSelectUser(users[activeIndex - 1]),
          onNext: () =>
            activeIndex < users.length - 1 &&
            requestSelectUser(users[activeIndex + 1]),
        }
      : undefined;

  const onRowKeyDown = (
    e: React.KeyboardEvent<HTMLElement>,
    user: UserInfo,
  ) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    const rows = Array.from(
      e.currentTarget.parentElement?.querySelectorAll<HTMLElement>(
        '[data-mrow]',
      ) ?? [],
    );
    const i = rows.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      rows[i + (e.key === 'ArrowDown' ? 1 : -1)]?.focus();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      requestSelectUser(user, e.currentTarget);
    }
  };

  return (
    <div className={cn(layout.page, layout.pageViewport)}>
      <WorkspaceHeader
        icon={<Users />}
        title={t('admin.userManagement.title')}
        tabs={<AccountsNavTabs activeKey='users' />}
        actions={
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => void loadUsers()}
              disabled={loading}
            >
              <RefreshCw
                className={cn('size-3.5', loading && 'animate-spin')}
                aria-hidden
              />
              {t('common.refresh')}
            </Button>
            <Button type='button' size='sm' onClick={handleOpenCreate}>
              <Plus className='size-3.5' aria-hidden />
              {t('admin.userManagement.createUser')}
            </Button>
          </div>
        }
      />

      <div className={layout.toolbar}>
        <div className={layout.toolbarSearch}>
          <SearchField
            label={t('admin.userManagement.username')}
            value={usernameDraft}
            onValueChange={(v) => {
              setUsernameDraft(v);
              if (v === '' && filters.username !== '') {
                patchFilters({username: ''});
              }
            }}
            placeholder={t('admin.userManagement.username')}
            clearLabel={t('common.searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commitUsername();
              }
            }}
          />
        </div>
        {filtersActive ? (
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={() => {
              setUsernameDraft('');
              resetFilters();
            }}
          >
            {t('common.clearFilters')}
          </Button>
        ) : null}
      </div>

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='users'>
          <TableCard
            loading={loading}
            hasData={users.length > 0}
            skeletonColumns={8}
            skeletonRows={8}
            error={error}
            onRetry={() => void loadUsers()}
            pagination={{
              currentPage: page,
              totalPages,
              pageSize,
              totalItems: total,
              onPageChange: setPage,
              onPageSizeChange: setPageSize,
              pageSizeOptions: [10, 20, 50, 100],
            }}
            empty={
              <EmptyState
                title={t('admin.userManagement.noUsers')}
                description={
                  filtersActive
                    ? t('common.noMatchingData')
                    : t('admin.userManagement.title')
                }
                action={
                  filtersActive ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={() => {
                        setUsernameDraft('');
                        resetFilters();
                      }}
                    >
                      {t('common.clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <div className={layout.rowHead}>
              <Checkbox
                checked={
                  pageAllSelected
                    ? true
                    : pageSomeSelected
                      ? 'indeterminate'
                      : false
                }
                onCheckedChange={(v) => toggleSelectPage(v === true)}
                aria-label={t('admin.userManagement.selectAll')}
              />
              <span>{t('admin.userManagement.username')}</span>
              <span>{t('admin.userManagement.nickname')}</span>
              <span>{t('admin.userManagement.email')}</span>
              <span>{t('admin.userManagement.isAdmin')}</span>
              <span>{t('admin.userManagement.isActive')}</span>
              <span>{t('admin.userManagement.createdAt')}</span>
              <span>{t('admin.userManagement.actions')}</span>
            </div>
            <div className={layout.tableScroll}>
              {users.map((user) => (
                <div
                  key={user.id}
                  data-mrow={user.id}
                  data-testid={`user-row-${user.id}`}
                  role='button'
                  tabIndex={0}
                  onClick={(e) => {
                    if (
                      (e.target as HTMLElement).closest(
                        'button, a, input, select, [role="menu"], [role="menuitem"], [role="checkbox"], [role="switch"]',
                      )
                    ) {
                      return;
                    }
                    requestSelectUser(user, e.currentTarget);
                  }}
                  onKeyDown={(e) => onRowKeyDown(e, user)}
                  className={cn(
                    layout.row,
                    activeUserId === user.id && layout.rowActive,
                  )}
                >
                  <Checkbox
                    checked={selectedIds.has(user.id)}
                    onCheckedChange={(v) => toggleSelect(user.id, v === true)}
                    aria-label={t('admin.userManagement.selectRow', {
                      username: user.username,
                    })}
                  />
                  <div className={layout.rowName}>
                    <span
                      className={layout.name}
                      title={user.username}
                      data-detail-name
                    >
                      {user.username}
                    </span>
                    <span className={layout.sub}>#{user.id}</span>
                  </div>
                  <span className={layout.sub}>{user.nickname || '-'}</span>
                  <span
                    className={cn(layout.sub, 'truncate')}
                    title={user.email || ''}
                  >
                    {user.email || '-'}
                  </span>
                  {user.is_admin ? (
                    <Badge variant='default'>
                      {t('admin.userManagement.isAdmin')}
                    </Badge>
                  ) : (
                    <Badge variant='secondary'>User</Badge>
                  )}
                  <div
                    onClick={(e) => e.stopPropagation()}
                    onKeyDown={(e) => e.stopPropagation()}
                  >
                    {isSelf(user) ? (
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className='inline-flex'>
                              <Switch
                                checked={user.is_active}
                                disabled
                                aria-label={`${t('admin.userManagement.isActive')} ${user.username}`}
                              />
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>
                            <p>{t('admin.userManagement.cannotDeactivateSelf')}</p>
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    ) : (
                      <Switch
                        checked={user.is_active}
                        aria-label={`${t('admin.userManagement.isActive')} ${user.username}`}
                        onCheckedChange={() => handleToggleActive(user)}
                      />
                    )}
                  </div>
                  <span className={cn(layout.sub, 'whitespace-nowrap')}>
                    {formatDateTz(user.created_at)}
                  </span>
                  <div
                    className={layout.rowActions}
                    onClick={(e) => e.stopPropagation()}
                    onKeyDown={(e) => e.stopPropagation()}
                  >
                    <DropdownMenu
                      trigger={
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          aria-label={t('common.moreActions')}
                        >
                          <MoreHorizontal className='size-4' aria-hidden />
                        </Button>
                      }
                      items={[
                        {
                          label: t('common.edit'),
                          icon: <Pencil className='size-4' aria-hidden />,
                          onSelect: () => requestSelectUser(user),
                        },
                        {
                          label: t('common.delete'),
                          icon: <Trash2 className='size-4' aria-hidden />,
                          destructive: true,
                          separatorBefore: true,
                          disabled: isSelf(user),
                          onSelect: () => handleOpenDelete(user),
                        },
                      ]}
                    />
                  </div>
                </div>
              ))}
            </div>
          </TableCard>

          <BatchBar
            selectedCount={selectedIds.size}
            busy={batchBusy}
            result={
              batchResult
                ? t('admin.userManagement.batchResult', {
                    ok: batchResult.ok,
                    skipped: batchResult.skipped,
                    failed: batchResult.failed,
                  })
                : null
            }
            onClear={() => {
              clearSelection();
              setBatchResult(null);
            }}
          >
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={batchBusy || selectedIds.size === 0}
              onClick={() => setBatchConfirm('deactivate')}
            >
              <UserX className='size-3.5' aria-hidden />
              {t('admin.userManagement.batchDeactivate')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={batchBusy || selectedIds.size === 0}
              onClick={() => setBatchConfirm('delete')}
            >
              <Trash2 className='size-3.5' aria-hidden />
              {t('admin.userManagement.batchDelete')}
            </Button>
          </BatchBar>
        </div>

        <DetailHost
          open={drawerOpen}
          hasContent={drawerUser != null}
          onOpenChange={(o) => {
            if (!o) {
              requestCloseDetail();
            }
          }}
          narrow={narrow}
          title={drawerUser?.username || t('admin.userManagement.editUser')}
          meta={
            drawerUser
              ? `#${drawerUser.id}${drawerUser.nickname ? ` · ${drawerUser.nickname}` : ''}`
              : undefined
          }
          nav={detailNav}
          flyOrigin={flyOriginRef}
          nameSlot='title'
          drawerWidth='w-[min(92vw,520px)]'
        >
          {drawerUser ? (
            <div className={layout.detailCol} data-testid='user-detail'>
              <div className={layout.detailScroll}>
                <section
                  className={cn(layout.detailSection, layout.detailSectionFirst)}
                >
                  <div className='flex flex-wrap items-center gap-2'>
                    <Badge
                      variant={drawerUser.is_active ? 'default' : 'secondary'}
                    >
                      {drawerUser.is_active
                        ? t('common.enabled')
                        : t('common.disabled')}
                    </Badge>
                    <Badge
                      variant={drawerUser.is_admin ? 'default' : 'outline'}
                    >
                      {drawerUser.is_admin
                        ? t('admin.userManagement.isAdmin')
                        : 'User'}
                    </Badge>
                  </div>
                  <dl className={layout.detailKvGrid}>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>ID</dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                        #{drawerUser.id}
                      </dd>
                    </div>
                    <div className={layout.detailKvItem}>
                      <dt className={layout.detailKvKey}>
                        {t('admin.userManagement.createdAt')}
                      </dt>
                      <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                        {formatDateTimeTz(drawerUser.created_at)}
                      </dd>
                    </div>
                  </dl>
                </section>

                <section className={layout.detailSection}>
                  <h3 className={layout.sectionTitle}>
                    {t('admin.userManagement.editUser')}
                  </h3>
                  <div className='space-y-4'>
                    <Input
                      id='edit-username'
                      label={t('admin.userManagement.username')}
                      value={drawerUser.username}
                      disabled
                    />
                    <Input
                      id='edit-password'
                      type='password'
                      label={t('admin.userManagement.password')}
                      placeholder={t('admin.userManagement.passwordPlaceholder')}
                      value={editForm.password}
                      onChange={(e) =>
                        setEditForm({...editForm, password: e.target.value})
                      }
                    />
                    <Input
                      id='edit-nickname'
                      label={t('admin.userManagement.nickname')}
                      value={editForm.nickname}
                      onChange={(e) =>
                        setEditForm({...editForm, nickname: e.target.value})
                      }
                    />
                    <Input
                      id='edit-email'
                      type='email'
                      label={t('admin.userManagement.email')}
                      value={editForm.email}
                      onChange={(e) =>
                        setEditForm({...editForm, email: e.target.value})
                      }
                    />
                    <div className='space-y-1'>
                      <Switch
                        id='edit-is_admin'
                        label={t('admin.userManagement.isAdmin')}
                        checked={editForm.is_admin}
                        disabled={
                          isSelf(drawerUser) && Boolean(editForm.is_admin)
                        }
                        onCheckedChange={(checked) =>
                          setEditForm({...editForm, is_admin: checked})
                        }
                      />
                      {isSelf(drawerUser) && editForm.is_admin ? (
                        <p className='text-xs text-muted-foreground'>
                          {t('admin.userManagement.cannotRevokeOwnAdmin')}
                        </p>
                      ) : null}
                    </div>
                  </div>
                </section>
              </div>
              <div className={layout.detailFoot}>
                {!isSelf(drawerUser) ? (
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    onClick={() => handleOpenDelete(drawerUser)}
                  >
                    <Trash2
                      className='size-3.5 text-destructive'
                      aria-hidden
                    />
                    {t('common.delete')}
                  </Button>
                ) : null}
                <span className={layout.detailFootGrow} />
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={requestCloseDetail}
                  disabled={savingEdit}
                >
                  {t('common.cancel')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  loading={savingEdit}
                  disabled={!editDirty || savingEdit}
                  onClick={() => void handleUpdate()}
                >
                  {savingEdit ? t('common.saving') : t('common.save')}
                </Button>
              </div>
            </div>
          ) : null}
        </DetailHost>
      </div>

      <DirtyCloseDialog
        open={pendingTarget !== null}
        onDiscard={discardAndCommit}
        onKeepEditing={() => setPendingTarget(null)}
      />

      <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
        <DialogContent
          title={t('admin.userManagement.createUser')}
          className='w-[min(calc(100vw-2rem),480px)]!'
          bodyClassName='max-h-[min(85dvh,640px)] overflow-y-auto'
        >
          <div className='space-y-4'>
            <Input
              id='username'
              label={t('admin.userManagement.username')}
              value={createForm.username}
              onChange={(e) =>
                setCreateForm({...createForm, username: e.target.value})
              }
            />
            <Input
              id='password'
              type='password'
              label={t('admin.userManagement.password')}
              value={createForm.password}
              onChange={(e) =>
                setCreateForm({...createForm, password: e.target.value})
              }
            />
            <Input
              id='nickname'
              label={t('admin.userManagement.nickname')}
              value={createForm.nickname}
              onChange={(e) =>
                setCreateForm({...createForm, nickname: e.target.value})
              }
            />
            <Input
              id='email'
              type='email'
              label={t('admin.userManagement.email')}
              value={createForm.email || ''}
              onChange={(e) =>
                setCreateForm({...createForm, email: e.target.value})
              }
            />
            <Switch
              id='is_admin'
              label={t('admin.userManagement.isAdmin')}
              checked={createForm.is_admin}
              onCheckedChange={(checked) =>
                setCreateForm({...createForm, is_admin: checked})
              }
            />
          </div>
          <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => setIsCreateDialogOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button type='button' size='sm' onClick={() => void handleCreate()}>
              {t('common.create')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deactivateTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeactivateTarget(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('admin.userManagement.deactivateUser')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('admin.userManagement.deactivateConfirm', {
                username: deactivateTarget?.username || '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void handleConfirmDeactivate()}>
              {t('admin.userManagement.deactivate')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('admin.userManagement.deleteUser')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('admin.userManagement.deleteConfirm', {
                username: deleteTarget?.username || '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void handleDelete()}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={batchConfirm !== null}
        onOpenChange={(open) => {
          if (!open) {
            setBatchConfirm(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {batchConfirm === 'deactivate'
                ? t('admin.userManagement.batchDeactivateConfirmTitle')
                : t('admin.userManagement.batchDeleteConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {batchConfirm === 'deactivate'
                ? t('admin.userManagement.batchDeactivateConfirmDesc', {
                    count: selectedIds.size,
                  })
                : t('admin.userManagement.batchDeleteConfirmDesc', {
                    count: selectedIds.size,
                  })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={batchBusy || !batchConfirm}
              onClick={() => {
                if (batchConfirm) {
                  void runBatch(batchConfirm);
                }
              }}
            >
              {batchConfirm === 'deactivate'
                ? t('admin.userManagement.batchDeactivate')
                : t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
