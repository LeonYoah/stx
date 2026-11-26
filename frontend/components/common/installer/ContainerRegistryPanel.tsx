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
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {ConfirmMorph} from '@/components/arc/confirm-morph/confirm-morph';
import {
  createContainerRegistry,
  deleteContainerRegistry,
  listContainerRegistries,
  testContainerRegistry,
  type ContainerRegistry,
} from '@/lib/services/installer/registry.service';

// ContainerRegistryPanel 镜像仓库管理卡：host+凭据三字段注册，注册即后台探测
// catalog 缓存，docker.io 系统行不可删改。镜像地址不再逐行登记——部署/升级从
// 仓库探测结果三层选择合成。
// ContainerRegistryPanel manages image registries: a three-field register dialog,
// catalog cache probed in the background, and an undeletable docker.io system row.
// Image references are no longer registered row by row; deploy/upgrade composes
// them from the probed tiers.
export function ContainerRegistryPanel() {
  const t = useTranslations('packages.registry');
  const [items, setItems] = useState<ContainerRegistry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [host, setHost] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [testingId, setTestingId] = useState<number | null>(null);

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

  const submit = async () => {
    setSubmitting(true);
    setSubmitError(null);
    try {
      await createContainerRegistry({host: host.trim(), username: username.trim(), password});
      setDialogOpen(false);
      setHost('');
      setUsername('');
      setPassword('');
      load();
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  const test = async (id: number) => {
    setTestingId(id);
    try {
      await testContainerRegistry(id);
    } catch {
      // 探测结果统一从刷新后的列表读取，失败原因在 probe_error 列展示。
      // Probe outcomes surface through the refreshed list's probe_error column.
    } finally {
      setTestingId(null);
      load();
    }
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

  return (
    <div className='space-y-3' data-testid='container-registry-panel'>
      <div className='flex items-center justify-between'>
        <p className='text-sm text-muted-foreground'>{t('subtitle')}</p>
        <Button type='button' size='sm' onClick={() => setDialogOpen(true)} data-testid='registry-add'>
          {t('add')}
        </Button>
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
          <Button type='button' variant='secondary' size='sm' onClick={() => setDialogOpen(true)}>
            {t('emptyAction')}
          </Button>
        </div>
      ) : (
        <table className='w-full text-sm' data-testid='registry-table'>
          <thead>
            <tr className='border-b border-border/50 text-left text-xs text-muted-foreground'>
              <th className='py-2 pr-3 font-medium'>{t('colHost')}</th>
              <th className='py-2 pr-3 font-medium'>{t('colAuth')}</th>
              <th className='py-2 pr-3 font-medium'>{t('colProbe')}</th>
              <th className='py-2 pr-3 font-medium'>{t('colRepos')}</th>
              <th className='py-2 text-right font-medium'>{t('colActions')}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((row) => (
              <tr key={row.id} className='border-b border-border/30 last:border-0'>
                <td className='py-2 pr-3 font-mono text-xs'>
                  {row.host}
                  {row.is_system ? (
                    <span className='ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px]'>{t('officialBadge')}</span>
                  ) : null}
                </td>
                <td className='py-2 pr-3 text-xs'>
                  {row.has_auth ? row.username || '•••' : t('anonymous')}
                </td>
                <td className='py-2 pr-3'>{statusBadge(row)}</td>
                <td className='py-2 pr-3 text-xs text-muted-foreground'>
                  {row.catalog_repos?.length ?? 0}
                </td>
                <td className='py-2 text-right'>
                  <span className='inline-flex items-center gap-2'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      disabled={testingId === row.id}
                      onClick={() => test(row.id)}
                      data-testid={`registry-test-${row.id}`}
                    >
                      {testingId === row.id ? t('testing') : t('test')}
                    </Button>
                    {row.is_system ? null : (
                      <ConfirmMorph
                        label={t('delete')}
                        confirmLabel={t('deleteConfirm')}
                        onConfirm={() => deleteContainerRegistry(row.id).then(load)}
                      />
                    )}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent title={t('dialogTitle')} className='arc-compact w-[min(94vw,480px)]!'>
        <div className='space-y-3'>
          <Input
            label={t('hostLabel')}
            value={host}
            placeholder='harbor.example.com 或 192.168.49.4:5000'
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
            onChange={(event) => setPassword(event.target.value)}
          />
          <p className='text-xs text-muted-foreground'>{t('hint')}</p>
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
            {submitting ? t('submitting') : t('submit')}
          </Button>
        </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
