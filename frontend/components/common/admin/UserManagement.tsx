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
 * 用户管理：TableCard + usePagedSelection/BatchBar + Arc 新建/编辑弹窗；
 * 破坏性确认仍走 AlertDialog；不能操作自己（批量跳过）。
 * User management: TableCard + usePagedSelection/BatchBar + Arc create/edit;
 * destructive confirms stay on AlertDialog; self is never mutated (batch skip).
 */

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Pencil, Plus, RefreshCw, Trash2, Users, UserX} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
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
  TableCard,
  WorkspaceHeader,
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
import {formatDateTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {AccountsNavTabs} from './AccountsNavTabs';

function isValidEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

const DEFAULT_PAGE_SIZE = 10;

type BatchKind = 'delete' | 'deactivate';

export function UserManagement() {
  const t = useTranslations();
  // 当前登录用户：不能停用自己或取消自己的管理员权限（后端同样拒绝），避免唯一管理员把自己锁在门外。
  // Current user: cannot deactivate themselves or revoke their own admin permission (the backend rejects it too),
  // so a sole admin cannot lock themselves out.
  const {user: currentUser} = useAuth();
  const isSelf = (user: UserInfo | null) =>
    Boolean(user && currentUser && user.id === currentUser.id);

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

  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
  const [selectedUser, setSelectedUser] = useState<UserInfo | null>(null);
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

  const [formData, setFormData] = useState<CreateUserRequest & {id?: number}>({
    username: '',
    password: '',
    nickname: '',
    email: '',
    is_admin: false,
  });

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
      setUsers(response.users || []);
      setTotal(response.total || 0);
      setError(null);
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

  const handleOpenCreate = () => {
    setFormData({
      username: '',
      password: '',
      nickname: '',
      email: '',
      is_admin: false,
    });
    setIsCreateDialogOpen(true);
  };

  const handleOpenEdit = (user: UserInfo) => {
    setSelectedUser(user);
    setFormData({
      id: user.id,
      username: user.username,
      password: '',
      nickname: user.nickname || '',
      email: user.email || '',
      is_admin: user.is_admin,
    });
    setIsEditDialogOpen(true);
  };

  const handleOpenDelete = (user: UserInfo) => {
    if (isSelf(user)) {
      toast.error(t('admin.userManagement.errors.cannotDeleteSelf'));
      return;
    }
    setSelectedUser(user);
    setIsDeleteDialogOpen(true);
  };

  const handleCreate = async () => {
    if (!formData.username || formData.username.length < 3) {
      toast.error(t('admin.userManagement.errors.usernameTooShort'));
      return;
    }
    if (!formData.password || formData.password.length < 6) {
      toast.error(t('admin.userManagement.errors.passwordTooShort'));
      return;
    }
    if (formData.email && !isValidEmail(formData.email)) {
      toast.error(t('admin.userManagement.errors.invalidEmail'));
      return;
    }

    try {
      await services.adminUser.createUser({
        username: formData.username,
        password: formData.password,
        nickname: formData.nickname,
        email: formData.email?.trim(),
        is_admin: formData.is_admin,
      });
      toast.success(t('admin.userManagement.createSuccess'));
      setIsCreateDialogOpen(false);
      void loadUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.error'));
    }
  };

  const handleUpdate = async () => {
    if (!selectedUser) {
      return;
    }

    const updateData: UpdateUserRequest = {};
    if (formData.nickname !== selectedUser.nickname) {
      updateData.nickname = formData.nickname;
    }
    if ((formData.email || '') !== (selectedUser.email || '')) {
      if (formData.email && !isValidEmail(formData.email)) {
        toast.error(t('admin.userManagement.errors.invalidEmail'));
        return;
      }
      updateData.email = formData.email?.trim() || '';
    }
    if (formData.password) {
      if (formData.password.length < 6) {
        toast.error(t('admin.userManagement.errors.passwordTooShort'));
        return;
      }
      updateData.password = formData.password;
    }
    if (formData.is_admin !== selectedUser.is_admin) {
      updateData.is_admin = formData.is_admin;
    }

    try {
      await services.adminUser.updateUser(selectedUser.id, updateData);
      toast.success(t('admin.userManagement.updateSuccess'));
      setIsEditDialogOpen(false);
      void loadUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.error'));
    }
  };

  const handleDelete = async () => {
    if (!selectedUser || isSelf(selectedUser)) {
      return;
    }
    try {
      await services.adminUser.deleteUser(selectedUser.id);
      toast.success(t('admin.userManagement.deleteSuccess'));
      setIsDeleteDialogOpen(false);
      {
        const next = new Set(selectedIds);
        next.delete(selectedUser.id);
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
    [selectedIds, batchBusy, users, currentUser, setSelectedIds, loadUsers, t],
  );

  const filtersActive = Boolean(filters.username);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

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
                  className={cn(layout.row)}
                  data-testid={`user-row-${user.id}`}
                  style={{cursor: 'default'}}
                >
                  <Checkbox
                    checked={selectedIds.has(user.id)}
                    onCheckedChange={(v) => toggleSelect(user.id, v === true)}
                    aria-label={t('admin.userManagement.selectRow', {
                      username: user.username,
                    })}
                  />
                  <div className={layout.rowName}>
                    <span className={layout.name} title={user.username}>
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
                  <div>
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
                  <div className='flex items-center gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      iconOnly
                      aria-label={t('common.edit')}
                      onClick={() => handleOpenEdit(user)}
                    >
                      <Pencil className='size-3.5' aria-hidden />
                    </Button>
                    {isSelf(user) ? (
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className='inline-flex'>
                              <Button
                                type='button'
                                variant='ghost'
                                size='sm'
                                iconOnly
                                disabled
                                aria-label={t('common.delete')}
                              >
                                <Trash2
                                  className='size-3.5 text-destructive'
                                  aria-hidden
                                />
                              </Button>
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>
                            <p>
                              {t('admin.userManagement.errors.cannotDeleteSelf')}
                            </p>
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    ) : (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        iconOnly
                        aria-label={t('common.delete')}
                        onClick={() => handleOpenDelete(user)}
                      >
                        <Trash2
                          className='size-3.5 text-destructive'
                          aria-hidden
                        />
                      </Button>
                    )}
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
      </div>

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
              value={formData.username}
              onChange={(e) =>
                setFormData({...formData, username: e.target.value})
              }
            />
            <Input
              id='password'
              type='password'
              label={t('admin.userManagement.password')}
              value={formData.password}
              onChange={(e) =>
                setFormData({...formData, password: e.target.value})
              }
            />
            <Input
              id='nickname'
              label={t('admin.userManagement.nickname')}
              value={formData.nickname}
              onChange={(e) =>
                setFormData({...formData, nickname: e.target.value})
              }
            />
            <Input
              id='email'
              type='email'
              label={t('admin.userManagement.email')}
              value={formData.email || ''}
              onChange={(e) =>
                setFormData({...formData, email: e.target.value})
              }
            />
            <Switch
              id='is_admin'
              label={t('admin.userManagement.isAdmin')}
              checked={formData.is_admin}
              onCheckedChange={(checked) =>
                setFormData({...formData, is_admin: checked})
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

      <Dialog open={isEditDialogOpen} onOpenChange={setIsEditDialogOpen}>
        <DialogContent
          title={t('admin.userManagement.editUser')}
          className='w-[min(calc(100vw-2rem),480px)]!'
          bodyClassName='max-h-[min(85dvh,640px)] overflow-y-auto'
        >
          <div className='space-y-4'>
            <Input
              id='edit-username'
              label={t('admin.userManagement.username')}
              value={formData.username}
              disabled
            />
            <Input
              id='edit-password'
              type='password'
              label={t('admin.userManagement.password')}
              placeholder={t('admin.userManagement.passwordPlaceholder')}
              value={formData.password}
              onChange={(e) =>
                setFormData({...formData, password: e.target.value})
              }
            />
            <Input
              id='edit-nickname'
              label={t('admin.userManagement.nickname')}
              value={formData.nickname}
              onChange={(e) =>
                setFormData({...formData, nickname: e.target.value})
              }
            />
            <Input
              id='edit-email'
              type='email'
              label={t('admin.userManagement.email')}
              value={formData.email || ''}
              onChange={(e) =>
                setFormData({...formData, email: e.target.value})
              }
            />
            <div className='space-y-1'>
              <Switch
                id='edit-is_admin'
                label={t('admin.userManagement.isAdmin')}
                checked={formData.is_admin}
                disabled={isSelf(selectedUser) && Boolean(formData.is_admin)}
                onCheckedChange={(checked) =>
                  setFormData({...formData, is_admin: checked})
                }
              />
              {isSelf(selectedUser) && formData.is_admin ? (
                <p className='text-xs text-muted-foreground'>
                  {t('admin.userManagement.cannotRevokeOwnAdmin')}
                </p>
              ) : null}
            </div>
          </div>
          <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => setIsEditDialogOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button type='button' size='sm' onClick={() => void handleUpdate()}>
              {t('common.save')}
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
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('admin.userManagement.deleteUser')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('admin.userManagement.deleteConfirm', {
                username: selectedUser?.username || '',
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
