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
 * Package catalog (online / local) with table ↔ card morph.
 * Online rows never download via whole-row click — only explicit buttons.
 * 安装包目录（在线 / 本地），支持表卡形变。
 * 在线版本整行不触发下载，只通过显式按钮发起。
 */

'use client';

import {useMemo} from 'react';
import {
  Download,
  FileCode2,
  Package,
  Star,
  Trash2,
  Upload,
} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {DropdownMenuItem} from '@/components/ui/dropdown-menu';
import {FileUpload} from '@/components/ui/file-upload';
import {
  MarketplaceMorphCatalog,
  type MarketplaceCatalogItem,
  type MarketplaceViewMode,
} from '@/components/common/marketplace';
import {formatDateTimeTz} from '@/lib/timezone';
import type {DownloadTask, PackageInfo} from '@/lib/services/installer/types';

interface PackageTableProps {
  type: 'online' | 'local';
  versions?: string[];
  localPackages?: PackageInfo[];
  recommendedVersion?: string;
  loading?: boolean;
  error?: string | null;
  viewMode: MarketplaceViewMode;
  onDelete?: (version: string) => void;
  onDownloadRequest?: (version: string) => void;
  downloads?: DownloadTask[];
  onSourceUpload?: (version: string, file: File) => void;
  onSourceFetch?: (version: string) => void;
  onSourceDownload?: (version: string) => void;
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) {
    return '0 B';
  }
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
}

function formatDate(dateStr?: string): string {
  if (!dateStr) {
    return '-';
  }
  return formatDateTimeTz(dateStr);
}

function formatSpeed(bytesPerSecond: number): string {
  if (bytesPerSecond < 1024) {
    return `${bytesPerSecond} B/s`;
  }
  if (bytesPerSecond < 1024 * 1024) {
    return `${(bytesPerSecond / 1024).toFixed(1)} KB/s`;
  }
  return `${(bytesPerSecond / 1024 / 1024).toFixed(1)} MB/s`;
}

/**
 * Online / local package list rendered through MorphCatalog.
 * 在线 / 本地安装包列表，经 MorphCatalog 渲染。
 */
export function PackageTable({
  type,
  versions = [],
  localPackages = [],
  recommendedVersion,
  loading,
  error = null,
  viewMode,
  onDelete,
  onDownloadRequest,
  downloads = [],
  onSourceUpload,
  onSourceFetch,
  onSourceDownload,
}: PackageTableProps) {
  const t = useTranslations();

  const items: MarketplaceCatalogItem[] = useMemo(() => {
    if (type === 'online') {
      return versions.map((version) => {
        const localPackage = localPackages.find((pkg) => pkg.version === version);
        const task = downloads.find((d) => d.version === version);
        const isDownloaded = Boolean(localPackage);
        const isRecommended = version === recommendedVersion;
        const isBusy =
          Boolean(task) &&
          (task?.status === 'downloading' || task?.status === 'pending');
        const isFailed = task?.status === 'failed';

        let status: MarketplaceCatalogItem['status'] = {
          label: t('marketplaceUi.statusAvailable'),
          tone: 'muted',
        };
        let progress: MarketplaceCatalogItem['progress'] = null;

        if (isDownloaded) {
          status = {label: t('marketplaceUi.statusDownloaded'), tone: 'ok'};
        } else if (isBusy && task) {
          status = {
            label: t('marketplaceUi.statusDownloading'),
            tone: 'info',
          };
          progress = {
            label: t('marketplaceUi.progressDownload'),
            value: task.progress || 0,
            display: `${task.progress || 0}% · ${formatSpeed(task.speed || 0)}`,
          };
        } else if (isFailed) {
          status = {label: t('marketplaceUi.statusFailed'), tone: 'error'};
        } else if (task?.status === 'completed') {
          status = {label: t('marketplaceUi.statusDownloaded'), tone: 'ok'};
        }

        let sourceLabel = t('installer.sourceAvailable');
        if (
          localPackage?.has_source ||
          task?.source_status === 'completed'
        ) {
          sourceLabel = t('installer.sourceDownloaded');
        } else if (
          task?.source_requested &&
          (task.source_status === 'pending' ||
            task.source_status === 'downloading')
        ) {
          sourceLabel = `${t('installer.sourceDownloading')} ${task.source_progress || 0}%`;
        } else if (task?.source_status === 'failed') {
          sourceLabel = t('installer.sourceFailed');
        } else if (localPackage) {
          sourceLabel = t('installer.sourceMissing');
        }

        const badges: MarketplaceCatalogItem['badges'] = [
          {label: `v${version}`, highlight: isRecommended},
        ];
        if (isRecommended) {
          badges.push({
            label: t('marketplaceUi.statusRecommended'),
            highlight: true,
          });
        }

        // Visible download / retry only — whole row must not trigger download.
        // 仅可见下载/重试按钮；整行不得触发下载。
        const showDownloadButton = !isDownloaded || isFailed;
        const primaryActions = showDownloadButton ? (
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8 rounded-md text-xs'
            data-testid={`package-download-btn-${version}`}
            disabled={isBusy}
            onClick={(e) => {
              e.stopPropagation();
              if (isBusy) {
                return;
              }
              onDownloadRequest?.(version);
            }}
          >
            <Download className='mr-1.5 size-3.5' />
            {isFailed
              ? t('installer.retry')
              : isBusy
                ? t('marketplaceUi.statusDownloading')
                : t('installer.downloadToServer')}
          </Button>
        ) : undefined;

        return {
          id: `online-${version}`,
          title: `v${version}`,
          subtitle: isRecommended
            ? t('installer.recommendedVersion')
            : t('installer.available'),
          icon: (
            <span className='inline-flex size-9 items-center justify-center rounded-full bg-primary/10 text-primary'>
              {isRecommended ? (
                <Star className='size-4' />
              ) : (
                <Package className='size-4' />
              )}
            </span>
          ),
          badges,
          status,
          fields: [
            {
              label: t('marketplaceUi.fieldSource'),
              value: sourceLabel,
            },
            {
              label: t('installer.status'),
              value: status.label,
            },
          ],
          progress,
          meta: isDownloaded ? t('installer.downloaded') : undefined,
          primaryActions,
          // No empty «…» menu for downloaded rows. / 已下载行不传空菜单。
          menuActions: undefined,
          // Whole row must not start downloads or other side effects.
          // 整行不得发起下载或其他副作用。
          onOpen: undefined,
        };
      });
    }

    return localPackages.map((pkg) => {
      let sourceLabel = t('installer.sourceMissing');
      let sourceToneOk = false;
      if (pkg.has_source) {
        sourceLabel = `${t('installer.sourceImported')} · ${formatFileSize(pkg.source_file_size || 0)}`;
        sourceToneOk = true;
      } else if (pkg.source_status === 'failed') {
        sourceLabel = t('installer.sourceFailed');
      }

      const menuActions = (
        <>
          {pkg.has_source ? (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onSourceDownload?.(pkg.version);
              }}
            >
              <Download className='mr-2 size-4' />
              {t('installer.downloadSource')}
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onSourceFetch?.(pkg.version);
              }}
            >
              <FileCode2 className='mr-2 size-4' />
              {t('installer.fetchSource')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem asChild>
            <FileUpload
              variant='menu-item'
              accept='.tar.gz'
              resetOnSelect
              data-testid={`package-source-upload-${pkg.version}`}
              onFileChange={(selected) => {
                if (selected) {
                  onSourceUpload?.(pkg.version, selected);
                }
              }}
            >
              <Upload className='size-4' />
              {pkg.has_source
                ? t('installer.replaceSource')
                : t('installer.uploadSource')}
            </FileUpload>
          </DropdownMenuItem>
          <DropdownMenuItem
            className='text-destructive'
            onSelect={() => {
              window.setTimeout(() => onDelete?.(pkg.version), 0);
            }}
          >
            <Trash2 className='mr-2 size-4' />
            {t('common.delete')}
          </DropdownMenuItem>
        </>
      );

      return {
        id: `local-${pkg.version}`,
        title: `v${pkg.version}`,
        subtitle: pkg.file_name,
        icon: (
          <span className='inline-flex size-9 items-center justify-center rounded-full bg-muted text-muted-foreground'>
            <Package className='size-4' />
          </span>
        ),
        badges: [
          {
            label: sourceToneOk
              ? t('installer.sourceImported')
              : sourceLabel,
            highlight: sourceToneOk,
          },
        ],
        status: {
          label: sourceToneOk
            ? t('installer.sourceImported')
            : pkg.source_status === 'failed'
              ? t('installer.sourceFailed')
              : t('installer.sourceMissing'),
          tone: sourceToneOk
            ? 'ok'
            : pkg.source_status === 'failed'
              ? 'error'
              : 'muted',
        },
        fields: [
          {
            label: t('marketplaceUi.fieldSize'),
            value: formatFileSize(pkg.file_size),
          },
          {
            label: t('marketplaceUi.fieldSource'),
            value: sourceLabel,
          },
          {
            label: t('marketplaceUi.fieldUpdated'),
            value: formatDate(pkg.uploaded_at),
          },
        ],
        meta: formatDate(pkg.uploaded_at),
        menuActions,
      };
    });
  }, [
    downloads,
    localPackages,
    onDelete,
    onDownloadRequest,
    onSourceDownload,
    onSourceFetch,
    onSourceUpload,
    recommendedVersion,
    t,
    type,
    versions,
  ]);

  const empty =
    type === 'online'
      ? t('installer.noVersionsAvailable')
      : t('installer.noLocalPackages');

  return (
    <MarketplaceMorphCatalog
      items={items}
      viewMode={viewMode}
      loading={Boolean(loading)}
      error={error}
      empty={empty}
      data-testid={
        type === 'online' ? 'package-online-catalog' : 'package-local-catalog'
      }
    />
  );
}
