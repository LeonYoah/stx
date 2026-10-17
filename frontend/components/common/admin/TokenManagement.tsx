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

import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Copy, KeyRound, Plus, RefreshCw, Search, ShieldOff} from 'lucide-react';
import {toast} from 'sonner';
import {useGSAP} from '@gsap/react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Pagination} from '@/components/ui/pagination';
import {Badge} from '@/components/ui/badge';
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
import {ToggleGroup, ToggleGroupItem} from '@/components/ui/toggle-group';
import {
  StatPillsBar,
  TableLoadingBar,
  TableSkeletonRows,
  WorkspaceHeader,
} from '@/components/common/layout';
import {animateTableRows} from '@/lib/animations/gsap-motion';
import services from '@/lib/services';
import type {UserInfo} from '@/lib/services/admin/user.service';
import type {
  CLITokenInfo,
  CreateCLITokenResult,
} from '@/lib/services/admin/token.service';
import {AccountsNavTabs} from './AccountsNavTabs';

type ExpiresPreset = '7d' | '30d' | '90d' | 'never' | 'custom';

/**
 * 管理员令牌管理页：签发永久或定期 CLI 令牌，原文只展示一次。
 * Admin token page: issue permanent or expiring CLI tokens; plaintext is shown once.
 */
export function TokenManagement() {
  const t = useTranslations();
  const [tokens, setTokens] = useState<CLITokenInfo[]>([]);
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [activeCount, setActiveCount] = useState(0);
  const [expiredCount, setExpiredCount] = useState(0);
  const [revokedCount, setRevokedCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [searchName, setSearchName] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isRevokeOpen, setIsRevokeOpen] = useState(false);
  const [selectedToken, setSelectedToken] = useState<CLITokenInfo | null>(null);
  const [issuedToken, setIssuedToken] = useState<CreateCLITokenResult | null>(
    null,
  );
  const [copied, setCopied] = useState(false);
  const [formName, setFormName] = useState('');
  const [formUserId, setFormUserId] = useState('');
  const [expiresPreset, setExpiresPreset] = useState<ExpiresPreset>('never');
  const [customDays, setCustomDays] = useState('60');
  const tableContainerRef = useRef<HTMLDivElement>(null);

  const loadTokens = useCallback(async () => {
    setLoading(true);
    try {
      const response = await services.adminToken.listTokens({
        current: currentPage,
        size: pageSize,
        name: searchName || undefined,
        status: statusFilter === 'all' ? undefined : statusFilter,
      });
      setTokens(response.tokens || []);
      setTotal(response.total || 0);
      setActiveCount(response.active || 0);
      setExpiredCount(response.expired || 0);
      setRevokedCount(response.revoked || 0);
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('admin.tokenManagement.loadError'),
      );
    } finally {
      setLoading(false);
    }
  }, [currentPage, pageSize, searchName, statusFilter, t]);

  useEffect(() => {
    void loadTokens();
  }, [loadTokens]);

  useGSAP(
    () => {
      if (tokens.length > 0 && !loading) {
        animateTableRows('.token-data-row');
      }
    },
    {dependencies: [tokens, loading], scope: tableContainerRef},
  );

  const openCreate = async () => {
    setFormName('');
    setExpiresPreset('never');
    setCustomDays('60');
    setIssuedToken(null);
    setCopied(false);
    try {
      const response = await services.adminUser.listUsers({
        current: 1,
        size: 100,
        is_active: true,
      });
      const list = response.users || [];
      setUsers(list);
      setFormUserId(list[0]?.id ? String(list[0].id) : '');
    } catch {
      setUsers([]);
      setFormUserId('');
    }
    setIsCreateOpen(true);
  };

  const resolveExpiresIn = (): string | null => {
    if (expiresPreset !== 'custom') {
      return expiresPreset;
    }
    const days = Number(customDays);
    if (!Number.isInteger(days) || days < 1 || days > 3650) {
      toast.error(t('admin.tokenManagement.errors.invalidCustomDays'));
      return null;
    }
    return `${days}d`;
  };

  const handleCreate = async () => {
    if (!formName.trim()) {
      toast.error(t('admin.tokenManagement.errors.nameRequired'));
      return;
    }
    const userId = Number(formUserId);
    if (!userId) {
      toast.error(t('admin.tokenManagement.errors.userRequired'));
      return;
    }
    const expiresIn = resolveExpiresIn();
    if (!expiresIn) {
      return;
    }
    try {
      const issued = await services.adminToken.createToken({
        user_id: userId,
        name: formName.trim(),
        expires_in: expiresIn,
      });
      setIssuedToken(issued);
      toast.success(t('admin.tokenManagement.createSuccess'));
      void loadTokens();
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('admin.tokenManagement.createError'),
      );
    }
  };

  const handleCopy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast.success(t('admin.tokenManagement.copied'));
    } catch {
      toast.error(t('admin.tokenManagement.copyFailed'));
    }
  };

  const handleRevoke = async () => {
    if (!selectedToken) {
      return;
    }
    try {
      await services.adminToken.revokeToken(selectedToken.id);
      toast.success(t('admin.tokenManagement.revokeSuccess'));
      setIsRevokeOpen(false);
      void loadTokens();
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('admin.tokenManagement.revokeError'),
      );
    }
  };

  const statusLabel = (status: string) => {
    if (status === 'active') {
      return t('admin.tokenManagement.statusActive');
    }
    if (status === 'expired') {
      return t('admin.tokenManagement.statusExpired');
    }
    if (status === 'revoked') {
      return t('admin.tokenManagement.statusRevoked');
    }
    return status || '-';
  };

  const expiryLabel = (token: CLITokenInfo) => {
    if (token.permanent || !token.expires_at) {
      return t('admin.tokenManagement.permanent');
    }
    return new Date(token.expires_at).toLocaleString();
  };

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const dialogClass =
    'border border-border/80 dark:border-border/60 shadow-2xl sm:max-w-lg';

  return (
    <div className='space-y-6'>
      <WorkspaceHeader
        icon={<KeyRound />}
        title={t('admin.tokenManagement.title')}
        subtitle={t('admin.tokenManagement.subtitle')}
        tabs={<AccountsNavTabs activeKey='tokens' />}
        actions={
          <div className='flex gap-2'>
            <Button variant='outline' onClick={() => void loadTokens()} disabled={loading}>
              <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
              {t('common.refresh')}
            </Button>
            <Button onClick={() => void openCreate()}>
              <Plus className='h-4 w-4 mr-2' />
              {t('admin.tokenManagement.createToken')}
            </Button>
          </div>
        }
      />

      <StatPillsBar
        activeKey={statusFilter}
        onChange={(key) => {
          setStatusFilter(key);
          setCurrentPage(1);
        }}
        items={[
          {
            key: 'all',
            label: t('admin.tokenManagement.filterAll'),
            count: activeCount + expiredCount + revokedCount,
          },
          {
            key: 'active',
            label: t('admin.tokenManagement.statusActive'),
            count: activeCount,
            variant: 'success',
          },
          {
            key: 'expired',
            label: t('admin.tokenManagement.statusExpired'),
            count: expiredCount,
            variant: 'warning',
          },
          {
            key: 'revoked',
            label: t('admin.tokenManagement.statusRevoked'),
            count: revokedCount,
            variant: 'danger',
          },
        ]}
      />

      <div className='flex gap-3'>
        <div className='flex-1 max-w-sm'>
          <Input
            placeholder={t('admin.tokenManagement.searchName')}
            value={searchName}
            onChange={(event) => setSearchName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                setCurrentPage(1);
                void loadTokens();
              }
            }}
          />
        </div>
        <Button
          variant='outline'
          onClick={() => {
            setCurrentPage(1);
            void loadTokens();
          }}
          className='active:scale-[0.98]'
        >
          <Search className='h-4 w-4 mr-2' />
          {t('common.search')}
        </Button>
      </div>

      <div
        ref={tableContainerRef}
        className='border rounded-lg relative overflow-hidden bg-card/40 shadow-xs flex flex-col min-h-[480px] sm:min-h-[calc(100vh-270px)]'
      >
        <TableLoadingBar loading={loading} />
        <div className='overflow-x-auto flex-1'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>{t('admin.tokenManagement.name')}</TableHead>
                <TableHead>{t('admin.tokenManagement.owner')}</TableHead>
                <TableHead>{t('admin.tokenManagement.prefix')}</TableHead>
                <TableHead>{t('admin.tokenManagement.expires')}</TableHead>
                <TableHead>{t('admin.tokenManagement.lastUsedAt')}</TableHead>
                <TableHead>{t('admin.tokenManagement.status')}</TableHead>
                <TableHead>{t('admin.userManagement.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading && tokens.length === 0 ? (
                <TableSkeletonRows columns={8} rows={10} />
              ) : tokens.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={8}
                    className='text-center py-12 text-muted-foreground'
                  >
                    {t('admin.tokenManagement.noTokens')}
                  </TableCell>
                </TableRow>
              ) : (
                tokens.map((token) => (
                  <TableRow
                    key={token.id}
                    className={`token-data-row transition-opacity duration-200 ${
                      loading ? 'opacity-60 pointer-events-none' : ''
                    }`}
                  >
                    <TableCell>{token.id}</TableCell>
                    <TableCell className='font-medium'>
                      {token.name || '-'}
                    </TableCell>
                    <TableCell>{token.username || '-'}</TableCell>
                    <TableCell>
                      <span className='font-mono whitespace-nowrap'>
                        {token.token_prefix}******
                      </span>
                    </TableCell>
                    <TableCell className='whitespace-nowrap'>
                      {expiryLabel(token)}
                    </TableCell>
                    <TableCell className='whitespace-nowrap'>
                      {token.last_used_at
                        ? new Date(token.last_used_at).toLocaleString()
                        : '-'}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          token.status === 'active'
                            ? 'default'
                            : token.status === 'expired'
                              ? 'secondary'
                              : 'destructive'
                        }
                      >
                        {statusLabel(token.status)}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {token.status === 'revoked' ? (
                        <span className='text-muted-foreground'>-</span>
                      ) : (
                        <Button
                          variant='ghost'
                          size='icon'
                          onClick={() => {
                            setSelectedToken(token);
                            setIsRevokeOpen(true);
                          }}
                        >
                          <ShieldOff className='h-4 w-4 text-destructive' />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        <div className='border-t bg-muted/10 px-4 py-2.5 mt-auto'>
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            pageSize={pageSize}
            totalItems={total}
            onPageChange={setCurrentPage}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setCurrentPage(1);
            }}
            pageSizeOptions={[10, 20, 50, 100]}
          />
        </div>
      </div>

      <Dialog
        open={isCreateOpen}
        onOpenChange={(open) => {
          setIsCreateOpen(open);
          if (!open) {
            setIssuedToken(null);
            setCopied(false);
          }
        }}
      >
        <DialogContent className={dialogClass}>
          <DialogHeader>
            <DialogTitle>
              {issuedToken
                ? t('admin.tokenManagement.plaintextTitle')
                : t('admin.tokenManagement.createToken')}
            </DialogTitle>
          </DialogHeader>
          {issuedToken?.token ? (
            <div className='space-y-4 py-2'>
              <p className='text-sm text-muted-foreground'>
                {t('admin.tokenManagement.plaintextOnce')}
              </p>
              <div className='flex items-center gap-2'>
                <Input
                  readOnly
                  value={issuedToken.token}
                  className='font-mono'
                />
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => void handleCopy(issuedToken.token || '')}
                >
                  {copied ? (
                    <Check className='h-4 w-4' />
                  ) : (
                    <Copy className='h-4 w-4' />
                  )}
                </Button>
              </div>
              <p className='text-xs text-muted-foreground font-mono whitespace-nowrap overflow-x-auto'>
                stx login --server &lt;url&gt; --token-stdin
              </p>
            </div>
          ) : (
            <div className='space-y-4 py-2'>
              <div className='space-y-2'>
                <Label htmlFor='token-name'>
                  {t('admin.tokenManagement.name')}
                </Label>
                <Input
                  id='token-name'
                  value={formName}
                  onChange={(event) => setFormName(event.target.value)}
                  placeholder={t('admin.tokenManagement.namePlaceholder')}
                />
              </div>
              <div className='space-y-2'>
                <Label>{t('admin.tokenManagement.owner')}</Label>
                <Select value={formUserId} onValueChange={setFormUserId}>
                  <SelectTrigger>
                    <SelectValue
                      placeholder={t('admin.tokenManagement.ownerPlaceholder')}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {users.map((user) => (
                      <SelectItem key={user.id} value={String(user.id)}>
                        {user.username}
                        {user.is_admin
                          ? ` · ${t('admin.userManagement.isAdmin')}`
                          : ''}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className='space-y-2'>
                <Label>{t('admin.tokenManagement.expires')}</Label>
                <ToggleGroup
                  type='single'
                  variant='outline'
                  size='sm'
                  value={expiresPreset}
                  onValueChange={(value) => {
                    if (value) {
                      setExpiresPreset(value as ExpiresPreset);
                    }
                  }}
                  className='flex w-full flex-wrap gap-1'
                >
                  <ToggleGroupItem value='7d'>
                    {t('admin.tokenManagement.expires7d')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='30d'>
                    {t('admin.tokenManagement.expires30d')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='90d'>
                    {t('admin.tokenManagement.expires90d')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='never'>
                    {t('admin.tokenManagement.expiresNever')}
                  </ToggleGroupItem>
                  <ToggleGroupItem value='custom'>
                    {t('admin.tokenManagement.expiresCustom')}
                  </ToggleGroupItem>
                </ToggleGroup>
                {expiresPreset === 'custom' ? (
                  <div className='space-y-1'>
                    <Input
                      type='number'
                      min={1}
                      max={3650}
                      value={customDays}
                      onChange={(event) => setCustomDays(event.target.value)}
                    />
                    <p className='text-xs text-muted-foreground'>
                      {t('admin.tokenManagement.customDaysHint')}
                    </p>
                  </div>
                ) : null}
              </div>
            </div>
          )}
          <DialogFooter>
            {issuedToken?.token ? (
              <Button onClick={() => setIsCreateOpen(false)}>
                {t('common.close')}
              </Button>
            ) : (
              <>
                <Button variant='outline' onClick={() => setIsCreateOpen(false)}>
                  {t('common.cancel')}
                </Button>
                <Button onClick={() => void handleCreate()}>
                  {t('common.create')}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={isRevokeOpen} onOpenChange={setIsRevokeOpen}>
        <AlertDialogContent className='border border-border/80 dark:border-border/60 shadow-2xl'>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('admin.tokenManagement.revokeToken')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('admin.tokenManagement.revokeConfirm', {
                name: selectedToken?.name || selectedToken?.token_prefix || '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void handleRevoke()}>
              {t('admin.tokenManagement.revokeToken')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
