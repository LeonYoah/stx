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

import {useMemo, useState} from 'react';
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
import {RefreshCw, Package, Puzzle, Cloud, HardDrive, PackageOpen} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {
  WorkspaceHeader,
  StatPillsBar,
  ModuleNavTabs,
  type StatPillItem,
} from '@/components/common/layout';
import {
  MarketplaceListToolbar,
  MarketplaceOverviewBento,
  useMarketplaceViewMode,
  type MarketplaceOverviewModel,
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
  const withSourceCount = (packages?.local_packages || []).filter(
    (pkg) => pkg.has_source,
  ).length;
  const withoutSourceCount = Math.max(0, localCount - withSourceCount);

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

  /**
   * Overview from real package data only — hide zeros / missing recommended.
   * 总览只用真实安装包数据；0 / 无推荐版本则隐藏。
   */
  const overviewModel: MarketplaceOverviewModel = useMemo(() => {
    const slices = [
      {
        id: 'with-source',
        label: t('marketplaceUi.packageDistWithSource'),
        count: withSourceCount,
        toneClass: 'bg-emerald-500/15',
      },
      {
        id: 'no-source',
        label: t('marketplaceUi.packageDistNoSource'),
        count: withoutSourceCount,
        toneClass: 'bg-muted',
      },
    ].filter((s) => s.count > 0);

    const stats = [
      ...(localCount > 0
        ? [
            {
              id: 'local',
              label: t('marketplaceUi.packageOverviewLocal'),
              value: localCount,
            },
          ]
        : []),
      ...(packages?.recommended_version
        ? [
            {
              id: 'recommended',
              label: t('marketplaceUi.packageOverviewRecommended'),
              value: `v${packages.recommended_version}`,
            },
          ]
        : []),
      ...(withSourceCount > 0
        ? [
            {
              id: 'with-source',
              label: t('marketplaceUi.packageOverviewWithSource'),
              value: withSourceCount,
            },
          ]
        : []),
    ];

    return {
      hero: {
        label: t('marketplaceUi.packageOverviewVersions'),
        value: onlineCount > 0 ? onlineCount : '',
        caption:
          onlineCount > 0
            ? t('marketplaceUi.packageOverviewVersionsCaption')
            : undefined,
      },
      distribution:
        slices.length > 0
          ? {
              title: t('marketplaceUi.packageDistTitle'),
              slices,
            }
          : undefined,
      stats,
    };
  }, [
    localCount,
    onlineCount,
    packages?.recommended_version,
    t,
    withSourceCount,
    withoutSourceCount,
  ]);

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

      <MarketplaceOverviewBento model={overviewModel} />

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
