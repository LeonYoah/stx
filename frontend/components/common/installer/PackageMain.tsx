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
 * 安装包管理主组件
 */

'use client';

import {useState, useMemo} from 'react';
import {toast} from 'sonner';
import {usePackages} from '@/hooks/use-installer';
import {PackageTable} from './PackageTable';
import {UploadPackageDialog} from './UploadPackageDialog';
import {DownloadPackageDialog} from './DownloadPackageDialog';
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
import {Upload, RefreshCw, Package, Puzzle, Cloud, HardDrive} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {
  WorkspaceHeader,
  StatPillsBar,
  ModuleNavTabs,
  type StatPillItem,
} from '@/components/common/layout';
import type {MirrorSource} from '@/lib/services/installer/types';

export function PackageMain() {
  const t = useTranslations();
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

  // 状态胶囊配置：消除冗余说明卡片，垂直空间利用率最大化
  // Status pills configuration: eliminate redundant description cards to maximize vertical space
  const pillItems: StatPillItem[] = useMemo(
    () => [
      {
        key: 'online',
        label: t('installer.onlineVersions'),
        count: packages?.versions.length || 0,
        icon: <Cloud className='h-3.5 w-3.5' />,
      },
      {
        key: 'local',
        label: t('installer.localPackages'),
        count: packages?.local_packages.length || 0,
        icon: <HardDrive className='h-3.5 w-3.5' />,
        variant:
          (packages?.local_packages.length || 0) > 0 ? 'success' : 'default',
      },
    ],
    [packages?.versions.length, packages?.local_packages.length, t],
  );

  return (
    <div className='w-full space-y-3.5'>
      {/* 头部标题区域（全局紧凑型规范） / Unified Compact Workspace Header */}
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
                className='h-7.5 px-2 text-xs bg-primary/10 text-primary border-primary/30 font-medium'
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
              className='h-8 text-xs'
            >
              <RefreshCw
                className={`h-3.5 w-3.5 mr-1.5 ${loading ? 'animate-spin' : ''}`}
              />
              {t('common.refresh')}
            </Button>
            <Button
              size='sm'
              onClick={() => setUploadDialogOpen(true)}
              className='h-8 text-xs'
            >
              <Upload className='h-3.5 w-3.5 mr-1.5' />
              {t('installer.uploadPackage')}
            </Button>
          </div>
        }
      />

      {/* 错误显示 / Error banner */}
      {error && (
        <div className='rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive'>
          {error}
        </div>
      )}

      {/* 一体化紧凑卡片容器 / Unified Compact Table Container */}
      <Card className='border-border/70 shadow-xs overflow-hidden flex flex-col min-h-[480px] sm:min-h-[calc(100vh-270px)]'>
        {/* 工具栏：状态胶囊切换器 + 快速操作 / Toolbar: Segmented pills + Quick actions */}
        <div className='p-3 sm:p-3.5 border-b bg-muted/20 flex flex-wrap items-center justify-between gap-2.5'>
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
              className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground'
            >
              <RefreshCw
                className={`h-3 w-3 mr-1 ${refreshingVersions ? 'animate-spin' : ''}`}
              />
              {t('installer.refreshVersions')}
            </Button>
          )}
        </div>

        {/* 表格主体内容：直达数据行，杜绝层层嵌套 / Table Body: Direct access to data rows */}
        <div className='p-3 sm:p-4 flex-1 flex flex-col'>
          {activeTab === 'online' ? (
            <PackageTable
              type='online'
              versions={packages?.versions || []}
              localPackages={packages?.local_packages || []}
              recommendedVersion={packages?.recommended_version}
              loading={loading}
              onDownloadRequest={setDownloadVersion}
              downloads={downloads}
            />
          ) : (
            <PackageTable
              type='local'
              localPackages={packages?.local_packages || []}
              loading={loading}
              onDelete={handleDelete}
              onSourceUpload={handleSourceUpload}
              onSourceFetch={(version) => fetchSource(version, 'apache')}
              onSourceDownload={downloadSource}
            />
          )}
        </div>
      </Card>

      {/* 上传对话框 / Upload Dialog */}
      <UploadPackageDialog
        open={uploadDialogOpen}
        onOpenChange={setUploadDialogOpen}
        onUpload={handleUpload}
        existingLocalVersions={(packages?.local_packages || []).map(
          (pkg) => pkg.version,
        )}
      />

      {/* 在线下载对话框 / Online download dialog */}
      <DownloadPackageDialog
        open={downloadVersion !== null}
        version={downloadVersion}
        onOpenChange={(open) => !open && setDownloadVersion(null)}
        onDownload={handleDownload}
      />

      {/* 删除本地安装包确认：沿用 AlertDialog，不走浏览器 confirm */}
      {/* Confirm local package deletion with AlertDialog, not the browser confirm */}
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
    </div>
  );
}
