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
 * 用户管理：TableCard + useServerListQuery；删除/停用仍走 AlertDialog；不能操作自己。
 * User management: TableCard + useServerListQuery; delete/deactivate stay on
 * AlertDialog; self-actions remain blocked.
 */

import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Pencil, Plus, RefreshCw, Trash2, Users} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {Badge} from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
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
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {toast} from '@/lib/toast';
import {TableCard, WorkspaceHeader} from '@/components/common/layout';
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

  const [formData, setFormData] = useState<CreateUserRequest & {id?: number}>({
    username: '',
    password: '',
    nickname: '',
    email: '',
    is_admin: false,
  });

  const loadUsers = useCallback(async () => {
    setLoading(true);
    try {
      const response = await services.adminUser.listUsers({
        current: page,
        size: pageSize,
        username: filters.username || undefined,
      });
      setUsers(response.users || []);
      setTotal(response.total || 0);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.error'));
      setUsers([]);
      setTotal(0);
    } finally {
      setLoading(false);
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
    if (!selectedUser) {
      return;
    }
    try {
      await services.adminUser.deleteUser(selectedUser.id);
      toast.success(t('admin.userManagement.deleteSuccess'));
      setIsDeleteDialogOpen(false);
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
            skeletonColumns={7}
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
            <div className={layout.rowHead} aria-hidden>
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
                  <div className={layout.rowName}>
                    <span className={layout.name} title={user.username}>
                      {user.username}
                    </span>
                    <span className={layout.sub}>#{user.id}</span>
                  </div>
                  <span className={layout.sub}>{user.nickname || '-'}</span>
                  <span className={cn(layout.sub, 'truncate')} title={user.email || ''}>
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
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      iconOnly
                      aria-label={t('common.delete')}
                      onClick={() => handleOpenDelete(user)}
                    >
                      <Trash2 className='size-3.5 text-destructive' aria-hidden />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </TableCard>
        </div>
      </div>

      <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('admin.userManagement.createUser')}</DialogTitle>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label htmlFor='username'>
                {t('admin.userManagement.username')}
              </Label>
              <Input
                id='username'
                value={formData.username}
                onChange={(e) =>
                  setFormData({...formData, username: e.target.value})
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='password'>
                {t('admin.userManagement.password')}
              </Label>
              <Input
                id='password'
                type='password'
                value={formData.password}
                onChange={(e) =>
                  setFormData({...formData, password: e.target.value})
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='nickname'>
                {t('admin.userManagement.nickname')}
              </Label>
              <Input
                id='nickname'
                value={formData.nickname}
                onChange={(e) =>
                  setFormData({...formData, nickname: e.target.value})
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='email'>{t('admin.userManagement.email')}</Label>
              <Input
                id='email'
                type='email'
                value={formData.email || ''}
                onChange={(e) =>
                  setFormData({...formData, email: e.target.value})
                }
              />
            </div>
            <div className='flex items-center space-x-2'>
              <Switch
                id='is_admin'
                checked={formData.is_admin}
                onCheckedChange={(checked) =>
                  setFormData({...formData, is_admin: checked})
                }
              />
              <Label htmlFor='is_admin'>
                {t('admin.userManagement.isAdmin')}
              </Label>
            </div>
          </div>
          <DialogFooter>
            <Button
              type='button'
              variant='secondary'
              onClick={() => setIsCreateDialogOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button type='button' onClick={() => void handleCreate()}>
              {t('common.create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={isEditDialogOpen} onOpenChange={setIsEditDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('admin.userManagement.editUser')}</DialogTitle>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label htmlFor='edit-username'>
                {t('admin.userManagement.username')}
              </Label>
              <Input id='edit-username' value={formData.username} disabled />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='edit-password'>
                {t('admin.userManagement.password')}
              </Label>
              <Input
                id='edit-password'
                type='password'
                placeholder={t('admin.userManagement.passwordPlaceholder')}
                value={formData.password}
                onChange={(e) =>
                  setFormData({...formData, password: e.target.value})
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='edit-nickname'>
                {t('admin.userManagement.nickname')}
              </Label>
              <Input
                id='edit-nickname'
                value={formData.nickname}
                onChange={(e) =>
                  setFormData({...formData, nickname: e.target.value})
                }
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='edit-email'>
                {t('admin.userManagement.email')}
              </Label>
              <Input
                id='edit-email'
                type='email'
                value={formData.email || ''}
                onChange={(e) =>
                  setFormData({...formData, email: e.target.value})
                }
              />
            </div>
            <div className='space-y-1'>
              <div className='flex items-center space-x-2'>
                <Switch
                  id='edit-is_admin'
                  checked={formData.is_admin}
                  disabled={isSelf(selectedUser) && Boolean(formData.is_admin)}
                  onCheckedChange={(checked) =>
                    setFormData({...formData, is_admin: checked})
                  }
                />
                <Label htmlFor='edit-is_admin'>
                  {t('admin.userManagement.isAdmin')}
                </Label>
              </div>
              {isSelf(selectedUser) && formData.is_admin ? (
                <p className='text-xs text-muted-foreground'>
                  {t('admin.userManagement.cannotRevokeOwnAdmin')}
                </p>
              ) : null}
            </div>
          </div>
          <DialogFooter>
            <Button
              type='button'
              variant='secondary'
              onClick={() => setIsEditDialogOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button type='button' onClick={() => void handleUpdate()}>
              {t('common.save')}
            </Button>
          </DialogFooter>
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
    </div>
  );
}
