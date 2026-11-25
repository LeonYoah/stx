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
 * 令牌管理：TableCard + SegmentedControl（计数来自接口）+ 跨页多选 BatchBar +
 * Arc 新建弹窗；吊销确认仍走 AlertDialog。
 * Token management: TableCard + SegmentedControl (API counts) + cross-page
 * BatchBar + Arc create dialog; revoke confirms stay on AlertDialog.
 */

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {KeyRound, Plus, RefreshCw, ShieldOff} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Select} from '@/components/arc/select/select';
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
  BatchBar,
  TableCard,
  TruncatedText,
  WorkspaceHeader,
  usePagedSelection,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {useServerListQuery} from '@/lib/hooks/use-server-list-query';
import services from '@/lib/services';
import type {UserInfo} from '@/lib/services/admin/user.service';
import type {
  CLITokenInfo,
  CreateCLITokenResult,
} from '@/lib/services/admin/token.service';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {AccountsNavTabs} from './AccountsNavTabs';
import {CopyButton} from '@/components/arc/copy-button/copy-button';

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
  const [batchRevokeOpen, setBatchRevokeOpen] = useState(false);
  const [batchBusy, setBatchBusy] = useState(false);
  const [batchResult, setBatchResult] = useState<{
    ok: number;
    skipped: number;
    failed: number;
  } | null>(null);
  const [selectedToken, setSelectedToken] = useState<CLITokenInfo | null>(null);
  const [issuedToken, setIssuedToken] = useState<CreateCLITokenResult | null>(
    null,
  );
  const [formName, setFormName] = useState('');
  const [formUserId, setFormUserId] = useState('');
  const [expiresPreset, setExpiresPreset] = useState<ExpiresPreset>('never');
  const [customDays, setCustomDays] = useState('60');

  const pageIds = useMemo(() => tokens.map((tok) => tok.id), [tokens]);
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

  const loadTokens = useCallback(async () => {
    const requestId = listRequestIdRef.current + 1;
    listRequestIdRef.current = requestId;
    setLoading(true);
    try {
      const response = await services.adminToken.listTokens({
        current: page,
        size: pageSize,
        name: filters.name || undefined,
        status: filters.status === ALL_VALUE ? undefined : filters.status,
      });
      if (listRequestIdRef.current !== requestId) {
        return;
      }
      setTokens(response.tokens || []);
      setTotal(response.total || 0);
      setActiveCount(response.active || 0);
      setExpiredCount(response.expired || 0);
      setRevokedCount(response.revoked || 0);
      setError(null);
    } catch (err) {
      if (listRequestIdRef.current !== requestId) {
        return;
      }
      setError(
        err instanceof Error
          ? err.message
          : t('admin.tokenManagement.loadError'),
      );
    } finally {
      if (listRequestIdRef.current === requestId) {
        setLoading(false);
      }
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

  const handleRevoke = async () => {
    if (!selectedToken) {
      return;
    }
    try {
      await services.adminToken.revokeToken(selectedToken.id);
      toast.success(t('admin.tokenManagement.revokeSuccess'));
      setIsRevokeOpen(false);
      {
        const next = new Set(selectedIds);
        next.delete(selectedToken.id);
        setSelectedIds(next);
      }
      void loadTokens();
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t('admin.tokenManagement.revokeError'),
      );
    }
  };

  const runBatchRevoke = useCallback(async () => {
    const ids = Array.from(selectedIds);
    if (ids.length === 0 || batchBusy) {
      return;
    }
    setBatchBusy(true);
    setBatchResult(null);
    const outcome = {ok: 0, skipped: 0, failed: 0};
    const failedIds: number[] = [];
    const byId = new Map(tokens.map((tok) => [tok.id, tok]));
    for (const id of ids) {
      const known = byId.get(id);
      if (known?.status === 'revoked') {
        outcome.skipped += 1;
        continue;
      }
      try {
        await services.adminToken.revokeToken(id);
        outcome.ok += 1;
      } catch {
        outcome.failed += 1;
        failedIds.push(id);
      }
    }
    setBatchResult(outcome);
    setBatchBusy(false);
    setBatchRevokeOpen(false);
    // 保留失败项勾选以便直接重试，清除已成功或跳过项。Keep failed items selected for retry; clear succeeded/skipped ones.
    setSelectedIds(new Set(failedIds));
    void loadTokens();
    const summary = t('admin.tokenManagement.batchResult', {
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
  }, [selectedIds, batchBusy, tokens, setSelectedIds, loadTokens, t]);

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
    return formatDateTimeMinuteTz(token.expires_at);
  };

  const segCount = (n: number) => (
    <span className={layout.count}>{n}</span>
  );

  const filtersActive =
    Boolean(filters.name) || filters.status !== ALL_VALUE;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

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
            skeletonColumns={8}
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
                aria-label={t('admin.tokenManagement.selectAll')}
              />
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
                  <Checkbox
                    checked={selectedIds.has(token.id)}
                    onCheckedChange={(v) => toggleSelect(token.id, v === true)}
                    aria-label={t('admin.tokenManagement.selectRow', {
                      name: token.name || token.token_prefix || String(token.id),
                    })}
                  />
                  <div className={layout.rowName}>
                    <TruncatedText text={token.name} className={layout.name} />
                    <span className={layout.sub}>#{token.id}</span>
                  </div>
                  <TruncatedText text={token.username} className={layout.sub} />
                  <span className='font-mono text-xs whitespace-nowrap'>
                    {token.token_prefix}******
                  </span>
                  <span className={cn(layout.sub, 'whitespace-nowrap')}>
                    {expiryLabel(token)}
                  </span>
                  <span className={cn(layout.sub, 'whitespace-nowrap')}>
                    {token.last_used_at
                      ? formatDateTimeMinuteTz(token.last_used_at)
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

          <BatchBar
            selectedCount={selectedIds.size}
            busy={batchBusy}
            result={
              batchResult
                ? t('admin.tokenManagement.batchResult', {
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
              onClick={() => setBatchRevokeOpen(true)}
            >
              <ShieldOff className='size-3.5' aria-hidden />
              {t('admin.tokenManagement.batchRevoke')}
            </Button>
          </BatchBar>
        </div>
      </div>

      <Dialog
        open={isCreateOpen}
        onOpenChange={(open) => {
          setIsCreateOpen(open);
          if (!open) {
            setIssuedToken(null);
                  }
        }}
      >
        <DialogContent
          title={
            issuedToken
              ? t('admin.tokenManagement.plaintextTitle')
              : t('admin.tokenManagement.createToken')
          }
          className='w-[min(calc(100vw-2rem),480px)]!'
          bodyClassName='max-h-[min(85dvh,640px)] overflow-y-auto'
        >
          {issuedToken?.token ? (
            <div className='space-y-4'>
              <p className='text-sm text-muted-foreground'>
                {t('admin.tokenManagement.plaintextOnce')}
              </p>
              <div className='flex items-end gap-2'>
                <div className='min-w-0 flex-1'>
                  <Input
                    label={t('admin.tokenManagement.plaintextTitle')}
                    readOnly
                    value={issuedToken.token}
                    className='font-mono'
                  />
                </div>
                <CopyButton
                  value={issuedToken.token || ''}
                  label={t('common.copy')}
                  copiedLabel={t('common.copied')}
                  failedLabel={t('common.copyFailed')}
                  iconOnly
                />
              </div>
              <p className='overflow-x-auto text-xs font-mono text-muted-foreground whitespace-nowrap'>
                stx login --server &lt;url&gt; --token-stdin
              </p>
            </div>
          ) : (
            <div className='space-y-4'>
              <Input
                id='token-name'
                label={t('admin.tokenManagement.name')}
                value={formName}
                onChange={(event) => setFormName(event.target.value)}
                placeholder={t('admin.tokenManagement.namePlaceholder')}
              />
              <Select
                label={t('admin.tokenManagement.owner')}
                placeholder={t('admin.tokenManagement.ownerPlaceholder')}
                value={formUserId}
                onValueChange={setFormUserId}
                options={users.map((user) => ({
                  value: String(user.id),
                  label: user.is_admin
                    ? `${user.username} · ${t('admin.userManagement.isAdmin')}`
                    : user.username,
                }))}
              />
              <div className='space-y-2'>
                <div className='text-xs font-medium text-foreground'>
                  {t('admin.tokenManagement.expires')}
                </div>
                <SegmentedControl
                  label={t('admin.tokenManagement.expires')}
                  value={expiresPreset}
                  onValueChange={(value) =>
                    setExpiresPreset(value as ExpiresPreset)
                  }
                  options={[
                    {
                      value: '7d',
                      label: t('admin.tokenManagement.expires7d'),
                    },
                    {
                      value: '30d',
                      label: t('admin.tokenManagement.expires30d'),
                    },
                    {
                      value: '90d',
                      label: t('admin.tokenManagement.expires90d'),
                    },
                    {
                      value: 'never',
                      label: t('admin.tokenManagement.expiresNever'),
                    },
                    {
                      value: 'custom',
                      label: t('admin.tokenManagement.expiresCustom'),
                    },
                  ]}
                />
                {expiresPreset === 'custom' ? (
                  <Input
                    type='number'
                    label={t('admin.tokenManagement.expiresCustom')}
                    description={t('admin.tokenManagement.customDaysHint')}
                    min={1}
                    max={3650}
                    value={customDays}
                    onChange={(event) => setCustomDays(event.target.value)}
                  />
                ) : null}
              </div>
            </div>
          )}
          <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
            {issuedToken?.token ? (
              <Button
                type='button'
                size='sm'
                onClick={() => setIsCreateOpen(false)}
              >
                {t('common.close')}
              </Button>
            ) : (
              <>
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={() => setIsCreateOpen(false)}
                >
                  {t('common.cancel')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  onClick={() => void handleCreate()}
                >
                  {t('common.create')}
                </Button>
              </>
            )}
          </div>
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

      <AlertDialog open={batchRevokeOpen} onOpenChange={setBatchRevokeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('admin.tokenManagement.batchRevokeConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('admin.tokenManagement.batchRevokeConfirmDesc', {
                count: selectedIds.size,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={batchBusy}
              onClick={() => void runBatchRevoke()}
            >
              {t('admin.tokenManagement.batchRevoke')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
