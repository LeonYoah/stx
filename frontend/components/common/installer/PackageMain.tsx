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

/**
 * Package Management Main Component
 * 安装包管理主组件 — Users 页 UI 语言：bento + 表/卡形变
 */

'use client';

import {useCallback, useMemo, useState} from 'react';
import {motion, useReducedMotion} from 'motion/react';
import {toast} from 'sonner';
import {usePackages} from '@/hooks/use-installer';
import {PackageTable} from './PackageTable';
import {UploadPackageDialog} from './UploadPackageDialog';
import {DownloadPackageDialog} from './DownloadPackageDialog';
import {OfflineBundleDialog} from './OfflineBundleDialog';
import {Button} from '@/components/ui/button';
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
import {Card} from '@/components/ui/card';
import {Badge} from '@/components/ui/badge';
import {
  RefreshCw,
  Package,
  Puzzle,
  Cloud,
  HardDrive,
  PackageOpen,
  Download,
  Trash2,
} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {
  WorkspaceHeader,
  StatPillsBar,
  ModuleNavTabs,
  type StatPillItem,
} from '@/components/common/layout';
import {
  MarketplaceListToolbar,
  runWithConcurrency,
  useMarketplaceViewMode,
  type MarketplaceSelectionState,
} from '@/components/common/marketplace';
import {buildMarketplacePageVariants} from '@/lib/animations/marketplace-motion';
import type {MirrorSource} from '@/lib/services/installer/types';

export function PackageMain() {
  const t = useTranslations();
  const reduceMotion = useReducedMotion() ?? false;
  const {container: containerVariants, item: itemVariants} = useMemo(
    () => buildMarketplacePageVariants(reduceMotion),
    [reduceMotion],
  );
  const [onlineViewMode, setOnlineViewMode] = useMarketplaceViewMode(
    'stx.marketplace.packages.online.view',
    'table',
  );
  const [localViewMode, setLocalViewMode] = useMarketplaceViewMode(
    'stx.marketplace.packages.local.view',
    'table',
  );
  const {
    packages,
    loading,
    error,
    refresh,
    uploadPackage,
    deletePackage,
    startDownload,
    uploadSource,
    fetchSource,
    downloadSource,
    downloads,
    refreshVersions,
    refreshingVersions,
  } = usePackages();
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);
  const [downloadVersion, setDownloadVersion] = useState<string | null>(null);
  // 删除确认用独立 open，避免 Action preventDefault 后受控状态关不掉
  // Keep a dedicated open flag so preventDefault on AlertDialogAction cannot leave the dialog stuck
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleteVersion, setDeleteVersion] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [activeTab, setActiveTab] = useState<'online' | 'local'>('online');
  const [offlineBundleOpen, setOfflineBundleOpen] = useState(false);
  const [selectedOnlineIds, setSelectedOnlineIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [selectedLocalIds, setSelectedLocalIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [batchFailedIds, setBatchFailedIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);

  const handleUpload = async (
    file: File,
    version: string,
    sourceFile?: File,
    onProgress?: (percent: number) => void,
  ) => {
    await uploadPackage(file, version, sourceFile, onProgress);
    setUploadDialogOpen(false);
  };

  const handleSourceUpload = async (version: string, sourceFile: File) => {
    await uploadSource(version, sourceFile);
  };

  // 打开页面内确认框，不用浏览器原生 confirm；延后到下拉菜单卸下后再开，避免双层 Modal 抢焦点
  // Open in-app confirm (not window.confirm); defer until the dropdown unmounts so nested modals do not fight focus
  const handleDelete = (version: string) => {
    setDeleteVersion(version);
    window.setTimeout(() => setDeleteConfirmOpen(true), 0);
  };

  const handleConfirmDelete = async () => {
    if (!deleteVersion) {
      return;
    }
    const version = deleteVersion;
    setDeleting(true);
    try {
      await deletePackage(version);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('installer.deletePackageFailed'),
      );
    } finally {
      setDeleting(false);
    }
  };

  const handleDownload = async (
    version: string,
    mirror: MirrorSource,
    withSource: boolean,
    idempotencyKey: string,
  ) => {
    await startDownload(version, mirror, withSource, idempotencyKey);
  };

  const onlineCount = packages?.versions.length || 0;
  const localCount = packages?.local_packages.length || 0;
  const onlineIds = useMemo(
    () => (packages?.versions || []).map((v) => `online-${v}`),
    [packages?.versions],
  );
  const localIds = useMemo(
    () =>
      (packages?.local_packages || []).map((pkg) => `local-${pkg.version}`),
    [packages?.local_packages],
  );
  const localVersionSet = useMemo(
    () => new Set((packages?.local_packages || []).map((pkg) => pkg.version)),
    [packages?.local_packages],
  );

  const onlineSelection: MarketplaceSelectionState = useMemo(
    () => ({
      selectedIds: selectedOnlineIds,
      onToggle: (id, selected) => {
        setSelectedOnlineIds((prev) => {
          const next = new Set(prev);
          if (selected) {
            next.add(id);
          } else {
            next.delete(id);
          }
          return next;
        });
      },
      onToggleAll: (selected) => {
        setSelectedOnlineIds((prev) => {
          if (!selected) {
            const next = new Set(prev);
            for (const id of onlineIds) {
              next.delete(id);
            }
            return next;
          }
          return new Set([...prev, ...onlineIds]);
        });
      },
      onClear: () => {
        setSelectedOnlineIds(new Set());
        setBatchFailedIds(new Set());
      },
    }),
    [onlineIds, selectedOnlineIds],
  );

  const localSelection: MarketplaceSelectionState = useMemo(
    () => ({
      selectedIds: selectedLocalIds,
      onToggle: (id, selected) => {
        setSelectedLocalIds((prev) => {
          const next = new Set(prev);
          if (selected) {
            next.add(id);
          } else {
            next.delete(id);
          }
          return next;
        });
      },
      onToggleAll: (selected) => {
        setSelectedLocalIds(selected ? new Set(localIds) : new Set());
      },
      onClear: () => {
        setSelectedLocalIds(new Set());
        setBatchFailedIds(new Set());
      },
    }),
    [localIds, selectedLocalIds],
  );

  const handleBatchDownloadOnline = useCallback(async () => {
    const versions = Array.from(selectedOnlineIds)
      .map((id) => id.replace(/^online-/, ''))
      .filter((version) => !localVersionSet.has(version));
    const skipped = selectedOnlineIds.size - versions.length;
    if (versions.length === 0) {
      toast.info(t('marketplaceUi.batchAllSkipped'));
      return;
    }
    if (skipped > 0) {
      toast.info(
        t('marketplaceUi.batchSkipHint', {
          process: versions.length,
          skip: skipped,
        }),
      );
    }
    const failed: string[] = [];
    const succeeded: string[] = [];
    // 多项时用默认镜像并发入队，避免反复弹下载对话框。
    // Queue selected versions with the default mirror; skip the per-item download dialog.
    await runWithConcurrency(versions, async (version) => {
      const id = `online-${version}`;
      try {
        await startDownload(
          version,
          'aliyun',
          false,
          `batch-${version}-${Date.now()}`,
        );
        succeeded.push(id);
      } catch {
        failed.push(id);
      }
    });
    setBatchFailedIds(new Set(failed));
    if (failed.length === 0) {
      setSelectedOnlineIds(new Set());
      toast.success(
        t('marketplaceUi.batchSummary', {
          success: succeeded.length,
          failed: 0,
        }),
      );
    } else {
      setSelectedOnlineIds(new Set(failed));
      toast.error(
        t('marketplaceUi.batchSummary', {
          success: succeeded.length,
          failed: failed.length,
        }),
      );
    }
  }, [localVersionSet, selectedOnlineIds, startDownload, t]);

  const handleBatchDeleteLocal = useCallback(async () => {
    const versions = Array.from(selectedLocalIds).map((id) =>
      id.replace(/^local-/, ''),
    );
    const failed: string[] = [];
    const succeeded: string[] = [];
    await runWithConcurrency(versions, async (version) => {
      const id = `local-${version}`;
      try {
        await deletePackage(version);
        succeeded.push(id);
      } catch {
        failed.push(id);
      }
    });
    setBatchDeleteOpen(false);
    setBatchFailedIds(new Set(failed));
    if (failed.length === 0) {
      setSelectedLocalIds(new Set());
      toast.success(
        t('marketplaceUi.batchSummary', {
          success: succeeded.length,
          failed: 0,
        }),
      );
    } else {
      setSelectedLocalIds(new Set(failed));
      toast.error(
        t('marketplaceUi.batchSummary', {
          success: succeeded.length,
          failed: failed.length,
        }),
      );
    }
  }, [deletePackage, selectedLocalIds, t]);

  // 状态胶囊配置 / Status pills configuration
  const pillItems: StatPillItem[] = useMemo(
    () => [
      {
        key: 'online',
        label: t('installer.onlineVersions'),
        count: onlineCount,
        icon: <Cloud className='h-3.5 w-3.5' />,
      },
      {
        key: 'local',
        label: t('installer.localPackages'),
        count: localCount,
        icon: <HardDrive className='h-3.5 w-3.5' />,
        variant: localCount > 0 ? 'success' : 'default',
      },
    ],
    [localCount, onlineCount, t],
  );

  return (
    <motion.div
      data-testid='package-management-root'
      data-reduce-motion={reduceMotion ? 'true' : 'false'}
      className='w-full space-y-3.5'
      initial='hidden'
      animate='visible'
      variants={containerVariants}
    >
      <motion.div variants={itemVariants}>
        <WorkspaceHeader
          icon={<Package />}
          title={t('installer.packageManagement')}
          subtitle={t('installer.packageManagementDesc')}
          tabs={
            <ModuleNavTabs
              reorderGroupId='packages-plugins'
              items={[
                {
                  key: 'packages',
                  label: t('installer.packageManagement'),
                  href: '/packages',
                  icon: <Package className='size-3.5' />,
                },
                {
                  key: 'plugins',
                  label: t('dock.pluginMarketplace'),
                  href: '/plugins',
                  icon: <Puzzle className='size-3.5' />,
                },
              ]}
              activeKey='packages'
            />
          }
          actions={
            <div className='flex items-center gap-2'>
              {packages?.recommended_version && (
                <Badge
                  variant='outline'
                  data-testid='package-recommended-version-pill'
                  className='h-7.5 rounded-full border-primary/30 bg-primary/10 px-2 text-xs font-medium text-primary'
                >
                  {t('installer.recommendedVersion')}: v
                  {packages.recommended_version}
                </Badge>
              )}
              <Button
                variant='outline'
                size='sm'
                onClick={refresh}
                disabled={loading}
                className='h-8 rounded-md border-border/60 text-xs'
              >
                <RefreshCw
                  className={`mr-1.5 h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`}
                />
                {t('common.refresh')}
              </Button>
              <Button
                size='sm'
                onClick={() => setOfflineBundleOpen(true)}
                className='h-8 rounded-md text-xs'
              >
                <PackageOpen className='mr-1.5 h-3.5 w-3.5' />
                {t('installer.offlineAssetsTitle')}
              </Button>
            </div>
          }
        />
      </motion.div>

      {error && (
        <div
          role='alert'
          className='rounded-xl border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive'
        >
          {error}
        </div>
      )}

      <motion.div variants={itemVariants} className='flex flex-1 flex-col'>
        <Card className='flex min-h-[480px] flex-col overflow-hidden rounded-xl border-border/60 shadow-xs sm:min-h-[calc(100vh-270px)]'>
          <div className='flex flex-wrap items-center justify-between gap-2.5 border-b bg-muted/20 p-3 sm:p-3.5'>
            <StatPillsBar
              items={pillItems}
              activeKey={activeTab}
              onChange={(key) => setActiveTab(key as 'online' | 'local')}
            />

            {activeTab === 'online' && (
              <Button
                variant='ghost'
                size='sm'
                onClick={refreshVersions}
                disabled={refreshingVersions}
                className='h-7 rounded-md px-2 text-xs text-muted-foreground hover:text-foreground'
              >
                <RefreshCw
                  className={`mr-1 h-3 w-3 ${refreshingVersions ? 'animate-spin' : ''}`}
                />
                {t('installer.refreshVersions')}
              </Button>
            )}
          </div>

          <div className='flex flex-1 flex-col gap-4 p-3 sm:p-4'>
            {activeTab === 'online' ? (
              <>
                <MarketplaceListToolbar
                  title={t('marketplaceUi.packageListOnline')}
                  count={onlineCount}
                  viewMode={onlineViewMode}
                  onViewModeChange={setOnlineViewMode}
                />
                <PackageTable
                  type='online'
                  versions={packages?.versions || []}
                  localPackages={packages?.local_packages || []}
                  recommendedVersion={packages?.recommended_version}
                  loading={loading}
                  error={error}
                  viewMode={onlineViewMode}
                  onDownloadRequest={setDownloadVersion}
                  downloads={downloads}
                  selection={onlineSelection}
                  bulkActions={
                    <>
                      <Button
                        type='button'
                        size='sm'
                        className='h-8 rounded-md text-xs'
                        data-testid='package-batch-download'
                        onClick={() => void handleBatchDownloadOnline()}
                      >
                        <Download className='mr-1.5 size-3.5' />
                        {t('marketplaceUi.batchDownload')}
                      </Button>
                      {batchFailedIds.size > 0 ? (
                        <Button
                          type='button'
                          size='sm'
                          variant='secondary'
                          className='h-8 rounded-md text-xs'
                          onClick={() => {
                            setSelectedOnlineIds(new Set(batchFailedIds));
                            void handleBatchDownloadOnline();
                          }}
                        >
                          {t('marketplaceUi.batchRetryFailed')}
                        </Button>
                      ) : null}
                    </>
                  }
                />
              </>
            ) : (
              <>
                <MarketplaceListToolbar
                  title={t('marketplaceUi.packageListLocal')}
                  count={localCount}
                  viewMode={localViewMode}
                  onViewModeChange={setLocalViewMode}
                />
                <PackageTable
                  type='local'
                  localPackages={packages?.local_packages || []}
                  loading={loading}
                  error={error}
                  viewMode={localViewMode}
                  onDelete={handleDelete}
                  onSourceUpload={handleSourceUpload}
                  onSourceFetch={(version) => fetchSource(version, 'apache')}
                  onSourceDownload={downloadSource}
                  selection={localSelection}
                  bulkActions={
                    <>
                      <Button
                        type='button'
                        size='sm'
                        variant='destructive'
                        className='h-8 rounded-md text-xs'
                        data-testid='package-batch-delete'
                        onClick={() => setBatchDeleteOpen(true)}
                      >
                        <Trash2 className='mr-1.5 size-3.5' />
                        {t('marketplaceUi.batchDelete')}
                      </Button>
                      {batchFailedIds.size > 0 ? (
                        <Button
                          type='button'
                          size='sm'
                          variant='secondary'
                          className='h-8 rounded-md text-xs'
                          onClick={() =>
                            setSelectedLocalIds(new Set(batchFailedIds))
                          }
                        >
                          {t('marketplaceUi.batchRetryFailed')}
                        </Button>
                      ) : null}
                    </>
                  }
                />
              </>
            )}
          </div>
        </Card>
      </motion.div>

      <OfflineBundleDialog
        open={offlineBundleOpen}
        onOpenChange={setOfflineBundleOpen}
        localPackages={packages?.local_packages || []}
        onOpenSinglePackageUpload={() => setUploadDialogOpen(true)}
        onUploadSinglePackage={async (file, version) => {
          await uploadPackage(file, version);
        }}
        onImported={() => {
          void refresh();
        }}
      />

      <UploadPackageDialog
        open={uploadDialogOpen}
        onOpenChange={setUploadDialogOpen}
        onUpload={handleUpload}
        existingLocalVersions={(packages?.local_packages || []).map(
          (pkg) => pkg.version,
        )}
      />

      <DownloadPackageDialog
        open={downloadVersion !== null}
        version={downloadVersion}
        onOpenChange={(open) => !open && setDownloadVersion(null)}
        onDownload={handleDownload}
      />

      <AlertDialog open={batchDeleteOpen} onOpenChange={setBatchDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('marketplaceUi.batchConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('marketplaceUi.batchConfirmDelete', {
                count: selectedLocalIds.size,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
              onClick={() => void handleBatchDeleteLocal()}
            >
              {t('common.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={deleteConfirmOpen}
        onOpenChange={(open) => {
          setDeleteConfirmOpen(open);
          if (!open) {
            setDeleting(false);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('installer.deletePackageTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('installer.confirmDeletePackage', {version: deleteVersion || ''})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
              onClick={() => void handleConfirmDelete()}
            >
              {deleting ? t('common.loading') : t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </motion.div>
  );
}
