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

import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {DetailHost, useDetailNarrow} from '@/components/common/layout';
import {ActionButton} from '@/components/arc/action-button/action-button';
import {Input} from '@/components/arc/input/input';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {ConfirmMorph} from '@/components/arc/confirm-morph/confirm-morph';
import {SortableDataTable, type DataColumn} from '@/components/arc/sortable-data-table/sortable-data-table';
import {
  createContainerRegistry,
  deleteContainerRegistry,
  listContainerRegistries,
  probeRegistry,
  testContainerRegistry,
  updateContainerRegistry,
  type ContainerRegistry,
} from '@/lib/services/installer/registry.service';

type Row = ContainerRegistry & Record<string, unknown>;

// ContainerRegistryPanel 镜像仓库管理卡：标准 SortableDataTable（多选批量删除），
// 注册/编辑弹窗内置不落库的「测试连接」，docker.io 系统行不可删改。
// ContainerRegistryPanel manages image registries with the standard
// SortableDataTable (multi-select bulk delete); the register/edit dialog embeds a
// non-persisting test-connection action, and the docker.io system row is immutable.
export function ContainerRegistryPanel() {
  const t = useTranslations('packages.registry');
  const [items, setItems] = useState<ContainerRegistry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [host, setHost] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [probeResult, setProbeResult] = useState<{ok: boolean; text: string} | null>(null);
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  // 抽屉滑出动画期间保留最后编辑的行，内容不先消失。
  // Keep the edited row mounted while the drawer slides out.
  const editingRow = editingId != null ? items.find((row) => row.id === editingId) ?? null : null;
  const retained = useDrawerItem(editingRow);
  const narrow = useDetailNarrow();

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

  const openCreate = () => {
    setEditingId(null);
    setHost('');
    setUsername('');
    setPassword('');
    setSubmitError(null);
    setProbeResult(null);
    setDialogOpen(true);
  };

  const openEdit = (row: ContainerRegistry) => {
    setEditingId(row.id);
    setHost(row.host);
    setUsername(row.username || '');
    // 密码不回填——密文不出后端，留空表示保持原密码。
    // The password is never backfilled (ciphertext never leaves the backend);
    // an empty field keeps the stored one.
    setPassword('');
    setSubmitError(null);
    setProbeResult(null);
    setDialogOpen(true);
  };

  const submit = async () => {
    setSubmitting(true);
    setSubmitError(null);
    try {
      const payload = {host: host.trim(), username: username.trim(), password};
      if (editingId != null) {
        await updateContainerRegistry(editingId, payload);
      } else {
        await createContainerRegistry(payload);
      }
      setDialogOpen(false);
      setEditingId(null);
      load();
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  // 弹窗内测试连接：不落库，直接用表单值探测，结果行内反馈。
  // The in-dialog test probes the form values without persisting anything and
  // reports inline.
  const probe = async () => {
    setProbeResult(null);
    try {
      const result = await probeRegistry({host: host.trim(), username: username.trim(), password});
      if (result.error) {
        setProbeResult({ok: false, text: `${t('probeFailed')}: ${result.error}`});
      } else if (result.status === 'reachable') {
        setProbeResult({ok: true, text: t('probeReachable')});
      } else {
        const count = result.repos?.length ?? 0;
        setProbeResult({ok: true, text: count > 0 ? `${t('probeOk')} · ${count}` : t('probeOk')});
      }
    } catch (err) {
      setProbeResult({ok: false, text: `${t('probeFailed')}: ${err instanceof Error ? err.message : String(err)}`});
    }
  };

  const testRow = async (id: number) => {
    try {
      const row = await testContainerRegistry(id);
      // 原地替换该行，不触发整表骨架重载——全量刷新会被误读成页面刷新。
      // Replace the row in place instead of reloading the whole table; a full
      // skeleton flash reads as a page refresh.
      setItems((prev) => prev.map((item) => (item.id === id ? row : item)));
    } catch (err) {
      setItems((prev) =>
        prev.map((item) =>
          item.id === id
            ? {...item, probe_status: 'failed', probe_error: err instanceof Error ? err.message : String(err)}
            : item,
        ),
      );
      throw err;
    }
  };

  const deleteSelected = async () => {
    const ids = selectedKeys.map(Number).filter((id) => items.some((row) => row.id === id && !row.is_system));
    for (const id of ids) {
      // 串行删除避免半批失败难排查；仓库数量级小，无需并发。
      // Deletes run serially so a partial failure stays diagnosable; registry
      // counts are small enough that concurrency is pointless.
      await deleteContainerRegistry(id).catch(() => undefined);
    }
    setSelectedKeys([]);
    load();
  };

  const statusBadge = (row: ContainerRegistry) => {
    const status = row.probe_status || 'unknown';
    const tone =
      status === 'ok' || status === 'public'
        ? 'text-emerald-600 dark:text-emerald-400'
        : status === 'reachable'
          ? 'text-amber-600 dark:text-amber-400'
          : 'text-destructive';
    return (
      <span className={`text-xs ${tone}`} title={row.probe_error || undefined}>
        {t(`status.${status}` as never) || status}
      </span>
    );
  };

  const columns: DataColumn<Row>[] = [
    {
      key: 'host',
      label: t('colHost'),
      sortable: true,
      render: (_value, row) => (
        <span className='font-mono text-xs'>
          <button
            type='button'
            className='text-primary underline-offset-2 hover:underline'
            onClick={(event) => {
              event.stopPropagation();
              openEdit(row);
            }}
            data-testid={`registry-open-${row.id}`}
          >
            {row.host}
          </button>
          {row.is_system ? (
            <span className='ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px]'>{t('officialBadge')}</span>
          ) : null}
        </span>
      ),
    },
    {
      key: 'has_auth',
      label: t('colAuth'),
      render: (_value, row) => <span className='text-xs'>{row.has_auth ? row.username || '•••' : t('anonymous')}</span>,
    },
    {
      key: 'probe_status',
      label: t('colProbe'),
      sortable: true,
      render: (_value, row) => statusBadge(row),
    },
    {
      key: 'repo_count',
      label: t('colRepos'),
      numeric: true,
      render: (_value, row) => <span className='text-xs text-muted-foreground'>{row.catalog_repos?.length ?? 0}</span>,
    },
    {
      key: 'actions',
      label: t('colActions'),
      width: 220,
      render: (_value, row) => (
        <span className='inline-flex items-center gap-2'>
          <span onClick={(event) => event.stopPropagation()}>
            <ActionButton
              label={t('test')}
              pendingLabel={t('testing')}
              successLabel={t('probeOk')}
              onAction={() => testRow(row.id)}
              data-testid={`registry-test-${row.id}`}
            />
          </span>
          {row.is_system ? null : (
            <span onClick={(event) => event.stopPropagation()}>
              <ConfirmMorph
                label={t('delete')}
                confirmLabel={t('deleteConfirm')}
                onConfirm={() => deleteContainerRegistry(row.id).then(load)}
              />
            </span>
          )}
        </span>
      ),
    },
  ];

  const rows = items.map((item) => ({...item, repo_count: item.catalog_repos?.length ?? 0})) as Row[];
  const hasSelection = selectedKeys.length > 0;

  return (
    <div className='space-y-3' data-testid='container-registry-panel'>
      <div className='flex items-center justify-end'>
        <span className='inline-flex items-center gap-2'>
          {hasSelection ? (
            <ConfirmMorph
              label={`${t('deleteSelected')} (${selectedKeys.length})`}
              confirmLabel={t('deleteConfirm')}
              onConfirm={deleteSelected}
            />
          ) : null}
          <Button type='button' size='sm' onClick={openCreate} data-testid='registry-add'>
            {t('add')}
          </Button>
        </span>
      </div>
      {loading ? (
        <Skeleton lines={3} className='h-9' label={t('title')} />
      ) : loadError ? (
        <div className='flex items-center gap-2'>
          <p className='min-w-0 flex-1 truncate text-xs text-destructive'>{loadError}</p>
          <Button type='button' variant='secondary' size='sm' onClick={load}>
            {t('retry')}
          </Button>
        </div>
      ) : items.length === 0 ? (
        <div className='space-y-2 py-6 text-center'>
          <p className='text-sm text-muted-foreground'>{t('empty')}</p>
          <Button type='button' variant='secondary' size='sm' onClick={openCreate}>
            {t('emptyAction')}
          </Button>
        </div>
      ) : (
        <SortableDataTable
          rows={rows}
          columns={columns}
          rowKey={(row) => String(row.id)}
          caption={t('title')}
          emptyMessage={t('empty')}
          selectable
          selectedKeys={selectedKeys}
          onSelectionChange={setSelectedKeys}
          onRowActivate={(row) => openEdit(row)}
        />
      )}
      <DetailHost
        open={dialogOpen}
        hasContent={retained != null || editingId == null}
        onOpenChange={(open) => {
          if (!open) {setDialogOpen(false);}
        }}
        narrow={narrow}
        title={editingId != null ? t('editDialogTitle') : t('dialogTitle')}
        meta={editingRow?.host}
        drawerWidth='w-[min(92vw,480px)]'
      >
        <div className='space-y-3 px-4 py-3'>
            <Input
              label={t('hostLabel')}
              value={host}
              placeholder='harbor.example.com 或 192.168.49.4:5000'
              readOnly={editingId != null}
              disabled={editingId != null}
              onChange={(event) => setHost(event.target.value)}
              data-testid='registry-host-input'
            />
            <Input
              label={t('usernameLabel')}
              value={username}
              onChange={(event) => setUsername(event.target.value)}
            />
            <Input
              label={t('passwordLabel')}
              type='password'
              value={password}
              placeholder={editingId != null ? t('passwordKeepHint') : undefined}
              onChange={(event) => setPassword(event.target.value)}
            />
            <div className='flex items-center gap-2'>
              <ActionButton
                label={t('probeFirst')}
                pendingLabel={t('testing')}
                successLabel={t('probeOk')}
                disabled={!host.trim()}
                onAction={probe}
                data-testid='registry-probe'
              />
              {probeResult ? (
                <p
                  className={`text-xs ${probeResult.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive'}`}
                  data-testid='registry-probe-result'
                >
                  {probeResult.text}
                </p>
              ) : null}
            </div>
            {submitError ? <p className='text-xs text-destructive'>{submitError}</p> : null}
          </div>
          <div className='mt-4 flex justify-end gap-2'>
            <Button type='button' variant='secondary' size='sm' onClick={() => setDialogOpen(false)}>
              {t('cancel')}
            </Button>
            <Button
              type='button'
              size='sm'
              disabled={submitting || !host.trim()}
              onClick={submit}
              data-testid='registry-submit'
            >
              {submitting ? t('submitting') : editingId != null ? t('submitEdit') : t('submit')}
            </Button>
          </div>
      </DetailHost>
    </div>
  );
}
