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
 * 令牌管理：TableCard + SegmentedControl（计数来自接口全量 active/expired/revoked）。
 * Token management: TableCard + SegmentedControl (counts from API totals).
 */

import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Copy, KeyRound, Plus, RefreshCw, ShieldOff} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {ToggleGroup, ToggleGroupItem} from '@/components/ui/toggle-group';
import {TableCard, WorkspaceHeader} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {useServerListQuery} from '@/lib/hooks/use-server-list-query';
import services from '@/lib/services';
import type {UserInfo} from '@/lib/services/admin/user.service';
import type {
  CLITokenInfo,
  CreateCLITokenResult,
} from '@/lib/services/admin/token.service';
import {cn} from '@/lib/utils';
import {AccountsNavTabs} from './AccountsNavTabs';

type ExpiresPreset = '7d' | '30d' | '90d' | 'never' | 'custom';

const DEFAULT_PAGE_SIZE = 10;
const ALL_VALUE = 'all';

/**
 * 管理员令牌管理页：签发永久或定期 CLI 令牌，原文只展示一次。
 * Admin token page: issue permanent or expiring CLI tokens; plaintext is shown once.
 */
export function TokenManagement() {
  const t = useTranslations();
  const {page, pageSize, filters, setPage, setPageSize, patchFilters, resetFilters} =
    useServerListQuery({
      initialFilters: {name: '', status: ALL_VALUE},
      defaultPageSize: DEFAULT_PAGE_SIZE,
    });

  const [nameDraft, setNameDraft] = useState(filters.name);
  const [tokens, setTokens] = useState<CLITokenInfo[]>([]);
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [activeCount, setActiveCount] = useState(0);
  const [expiredCount, setExpiredCount] = useState(0);
  const [revokedCount, setRevokedCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
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

  const loadTokens = useCallback(async () => {
    setLoading(true);
    try {
      const response = await services.adminToken.listTokens({
        current: page,
        size: pageSize,
        name: filters.name || undefined,
        status: filters.status === ALL_VALUE ? undefined : filters.status,
      });
      setTokens(response.tokens || []);
      setTotal(response.total || 0);
      setActiveCount(response.active || 0);
      setExpiredCount(response.expired || 0);
      setRevokedCount(response.revoked || 0);
      setError(null);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t('admin.tokenManagement.loadError'),
      );
      setTokens([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, filters.name, filters.status, t]);

  useEffect(() => {
    void loadTokens();
  }, [loadTokens]);

  const commitName = useCallback(() => {
    const next = nameDraft.trim();
    if (next !== filters.name) {
      patchFilters({name: next});
    }
  }, [nameDraft, filters.name, patchFilters]);

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
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
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
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
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

  const segCount = (n: number) => (
    <span className={layout.count}>{n}</span>
  );

  const filtersActive =
    Boolean(filters.name) || filters.status !== ALL_VALUE;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const dialogClass =
    'border border-border/80 dark:border-border/60 shadow-2xl sm:max-w-lg';

  return (
    <div className={cn(layout.page, layout.pageViewport)}>
      <WorkspaceHeader
        icon={<KeyRound />}
        title={t('admin.tokenManagement.title')}
        subtitle={t('admin.tokenManagement.subtitle')}
        tabs={<AccountsNavTabs activeKey='tokens' />}
        actions={
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => void loadTokens()}
              disabled={loading}
            >
              <RefreshCw
                className={cn('size-3.5', loading && 'animate-spin')}
                aria-hidden
              />
              {t('common.refresh')}
            </Button>
            <Button type='button' size='sm' onClick={() => void openCreate()}>
              <Plus className='size-3.5' aria-hidden />
              {t('admin.tokenManagement.createToken')}
            </Button>
          </div>
        }
      />

      <div className={layout.toolbar}>
        <SegmentedControl
          label={t('admin.tokenManagement.status')}
          value={filters.status}
          onValueChange={(v) => patchFilters({status: v})}
          options={[
            {
              value: ALL_VALUE,
              label: t('admin.tokenManagement.filterAll'),
              accessory: segCount(activeCount + expiredCount + revokedCount),
            },
            {
              value: 'active',
              label: t('admin.tokenManagement.statusActive'),
              accessory: segCount(activeCount),
            },
            {
              value: 'expired',
              label: t('admin.tokenManagement.statusExpired'),
              accessory: segCount(expiredCount),
            },
            {
              value: 'revoked',
              label: t('admin.tokenManagement.statusRevoked'),
              accessory: segCount(revokedCount),
            },
          ]}
        />
        <div className={layout.toolbarSearch}>
          <SearchField
            label={t('admin.tokenManagement.searchName')}
            value={nameDraft}
            onValueChange={(v) => {
              setNameDraft(v);
              if (v === '' && filters.name !== '') {
                patchFilters({name: ''});
              }
            }}
            placeholder={t('admin.tokenManagement.searchName')}
            clearLabel={t('common.searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commitName();
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
              setNameDraft('');
              resetFilters();
            }}
          >
            {t('common.clearFilters')}
          </Button>
        ) : null}
      </div>

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='tokens'>
          <TableCard
            loading={loading}
            hasData={tokens.length > 0}
            skeletonColumns={7}
            skeletonRows={8}
            error={error}
            onRetry={() => void loadTokens()}
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
                title={t('admin.tokenManagement.noTokens')}
                description={
                  filtersActive
                    ? t('common.noMatchingData')
                    : t('admin.tokenManagement.subtitle')
                }
                action={
                  filtersActive ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={() => {
                        setNameDraft('');
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
              <span>{t('admin.tokenManagement.name')}</span>
              <span>{t('admin.tokenManagement.owner')}</span>
              <span>{t('admin.tokenManagement.prefix')}</span>
              <span>{t('admin.tokenManagement.expires')}</span>
              <span>{t('admin.tokenManagement.lastUsedAt')}</span>
              <span>{t('admin.tokenManagement.status')}</span>
              <span>{t('admin.userManagement.actions')}</span>
            </div>
            <div className={layout.tableScroll}>
              {tokens.map((token) => (
                <div
                  key={token.id}
                  className={cn(layout.row)}
                  style={{cursor: 'default'}}
                  data-testid={`token-row-${token.id}`}
                >
                  <div className={layout.rowName}>
                    <span className={layout.name} title={token.name || '-'}>
                      {token.name || '-'}
                    </span>
                    <span className={layout.sub}>#{token.id}</span>
                  </div>
                  <span className={layout.sub}>{token.username || '-'}</span>
                  <span className='font-mono text-xs whitespace-nowrap'>
                    {token.token_prefix}******
                  </span>
                  <span className={cn(layout.sub, 'whitespace-nowrap')}>
                    {expiryLabel(token)}
                  </span>
                  <span className={cn(layout.sub, 'whitespace-nowrap')}>
                    {token.last_used_at
                      ? new Date(token.last_used_at).toLocaleString()
                      : '-'}
                  </span>
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
                  <div>
                    {token.status === 'revoked' ? (
                      <span className='text-muted-foreground'>-</span>
                    ) : (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        iconOnly
                        aria-label={t('admin.tokenManagement.revokeToken')}
                        onClick={() => {
                          setSelectedToken(token);
                          setIsRevokeOpen(true);
                        }}
                      >
                        <ShieldOff
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
                  variant='secondary'
                  onClick={() => void handleCopy(issuedToken.token || '')}
                  aria-label={t('common.copy')}
                >
                  {copied ? (
                    <Check className='size-4' aria-hidden />
                  ) : (
                    <Copy className='size-4' aria-hidden />
                  )}
                </Button>
              </div>
              <p className='overflow-x-auto text-xs font-mono text-muted-foreground whitespace-nowrap'>
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
              <Button type='button' onClick={() => setIsCreateOpen(false)}>
                {t('common.close')}
              </Button>
            ) : (
              <>
                <Button
                  type='button'
                  variant='secondary'
                  onClick={() => setIsCreateOpen(false)}
                >
                  {t('common.cancel')}
                </Button>
                <Button type='button' onClick={() => void handleCreate()}>
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
