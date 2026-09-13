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
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import {useState, useEffect, useCallback, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
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
import {Badge} from '@/components/ui/badge';
import {toast} from 'sonner';
import {Plus, Pencil, Trash2, Search, Users, RefreshCw} from 'lucide-react';
import {WorkspaceHeader, TableLoadingBar, TableSkeletonRows} from '@/components/common/layout';
import {useGSAP} from '@gsap/react';
import {animateTableRows} from '@/lib/animations/gsap-motion';
import services from '@/lib/services';
import type {
  UserInfo,
  CreateUserRequest,
  UpdateUserRequest,
} from '@/lib/services/admin/user.service';

function isValidEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

/**
 * 用户管理组件
 */
export function UserManagement() {
  const t = useTranslations();
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const [searchUsername, setSearchUsername] = useState('');
  const pageSize = 10;

  // 对话框状态
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
  const [selectedUser, setSelectedUser] = useState<UserInfo | null>(null);

  // 表单状态
  const [formData, setFormData] = useState<CreateUserRequest & {id?: number}>({
    username: '',
    password: '',
    nickname: '',
    email: '',
    is_admin: false,
  });

  /**
   * 加载用户列表
   */
  const loadUsers = useCallback(async () => {
    setLoading(true);
    try {
      const response = await services.adminUser.listUsers({
        current: currentPage,
        size: pageSize,
        username: searchUsername || undefined,
      });
      setUsers(response.users || []);
      setTotal(response.total || 0);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '加载用户列表失败');
    } finally {
      setLoading(false);
    }
  }, [currentPage, searchUsername]);

  useEffect(() => {
    loadUsers();
  }, [loadUsers]);

  /**
   * 打开创建对话框
   */
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

  /**
   * 打开编辑对话框
   */
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

  /**
   * 打开删除确认对话框
   */
  const handleOpenDelete = (user: UserInfo) => {
    setSelectedUser(user);
    setIsDeleteDialogOpen(true);
  };

  /**
   * 创建用户
   */
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
      loadUsers();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '创建用户失败');
    }
  };

  /**
   * 更新用户
   */
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
      loadUsers();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '更新用户失败');
    }
  };

  /**
   * 删除用户
   */
  const handleDelete = async () => {
    if (!selectedUser) {
      return;
    }

    try {
      await services.adminUser.deleteUser(selectedUser.id);
      toast.success(t('admin.userManagement.deleteSuccess'));
      setIsDeleteDialogOpen(false);
      loadUsers();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除用户失败');
    }
  };

  /**
   * 切换用户状态
   */
  const handleToggleActive = async (user: UserInfo) => {
    try {
      await services.adminUser.updateUser(user.id, {
        is_active: !user.is_active,
      });
      toast.success(t('admin.userManagement.updateSuccess'));
      loadUsers();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '更新用户状态失败');
    }
  };

  const tableContainerRef = useRef<HTMLDivElement>(null);

  // 用户列表更新后执行平滑交错入场动效
  // Trigger staggered entrance animation when users data updates
  useGSAP(
    () => {
      if (users.length > 0 && !loading) {
        animateTableRows('.user-data-row');
      }
    },
    {dependencies: [users, loading], scope: tableContainerRef},
  );

  /**
   * 搜索
   */
  const handleSearch = () => {
    setCurrentPage(1);
    loadUsers();
  };

  const totalPages = Math.ceil(total / pageSize);

  return (
    <div className='space-y-6'>
      {/* 页面标题 */}
      <WorkspaceHeader
        icon={<Users />}
        title={t('admin.userManagement.title')}
        actions={
          <div className='flex gap-2'>
            <Button variant='outline' onClick={loadUsers} disabled={loading}>
              <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
              {t('common.refresh')}
            </Button>
            <Button onClick={handleOpenCreate}>
              <Plus className='h-4 w-4 mr-2' />
              {t('admin.userManagement.createUser')}
            </Button>
          </div>
        }
      />

      {/* 搜索栏 */}
      <div className='flex gap-3'>
        <div className='flex-1 max-w-sm'>
          <Input
            placeholder={t('admin.userManagement.username')}
            value={searchUsername}
            onChange={(e) => setSearchUsername(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
          />
        </div>
        <Button variant='outline' onClick={handleSearch} className='active:scale-[0.98]'>
          <Search className='h-4 w-4 mr-2' />
          {t('common.search')}
        </Button>
      </div>

      {/* 用户表格 */}
      <div ref={tableContainerRef} className='border rounded-lg relative overflow-hidden bg-card/40 shadow-xs'>
        <TableLoadingBar loading={loading} />
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t('admin.userManagement.username')}</TableHead>
              <TableHead>{t('admin.userManagement.nickname')}</TableHead>
              <TableHead>{t('admin.userManagement.email')}</TableHead>
              <TableHead>{t('admin.userManagement.isAdmin')}</TableHead>
              <TableHead>{t('admin.userManagement.isActive')}</TableHead>
              <TableHead>{t('admin.userManagement.createdAt')}</TableHead>
              <TableHead>{t('admin.userManagement.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && users.length === 0 ? (
              <TableSkeletonRows columns={8} rows={10} />
            ) : users.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={8}
                  className='text-center py-12 text-muted-foreground'
                >
                  {t('admin.userManagement.noUsers')}
                </TableCell>
              </TableRow>
            ) : (
              users.map((user) => (
                <TableRow
                  key={user.id}
                  className={`user-data-row transition-opacity duration-200 ${
                    loading ? 'opacity-60 pointer-events-none' : ''
                  }`}
                >
                  <TableCell>{user.id}</TableCell>
                  <TableCell className='font-medium'>{user.username}</TableCell>
                  <TableCell>{user.nickname || '-'}</TableCell>
                  <TableCell>{user.email || '-'}</TableCell>
                  <TableCell>
                    {user.is_admin ? (
                      <Badge variant='default'>
                        {t('admin.userManagement.isAdmin')}
                      </Badge>
                    ) : (
                      <Badge variant='secondary'>User</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={user.is_active}
                      onCheckedChange={() => handleToggleActive(user)}
                    />
                  </TableCell>
                  <TableCell>
                    {new Date(user.created_at).toLocaleDateString()}
                  </TableCell>
                  <TableCell>
                    <div className='flex gap-2'>
                      <Button
                        variant='ghost'
                        size='icon'
                        onClick={() => handleOpenEdit(user)}
                      >
                        <Pencil className='h-4 w-4' />
                      </Button>
                      <Button
                        variant='ghost'
                        size='icon'
                        onClick={() => handleOpenDelete(user)}
                      >
                        <Trash2 className='h-4 w-4 text-destructive' />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* 分页 */}
      {totalPages > 1 && (
        <div className='flex justify-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={currentPage === 1}
            onClick={() => setCurrentPage((p) => p - 1)}
          >
            {t('common.previous')}
          </Button>
          <span className='flex items-center px-4'>
            {currentPage} / {totalPages}
          </span>
          <Button
            variant='outline'
            size='sm'
            disabled={currentPage === totalPages}
            onClick={() => setCurrentPage((p) => p + 1)}
          >
            {t('common.next')}
          </Button>
        </div>
      )}

      {/* 创建用户对话框 */}
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
              variant='outline'
              onClick={() => setIsCreateDialogOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button onClick={handleCreate}>{t('common.create')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑用户对话框 */}
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
            <div className='flex items-center space-x-2'>
              <Switch
                id='edit-is_admin'
                checked={formData.is_admin}
                onCheckedChange={(checked) =>
                  setFormData({...formData, is_admin: checked})
                }
              />
              <Label htmlFor='edit-is_admin'>
                {t('admin.userManagement.isAdmin')}
              </Label>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setIsEditDialogOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button onClick={handleUpdate}>{t('common.save')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 删除确认对话框 */}
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
            <AlertDialogAction onClick={handleDelete}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
