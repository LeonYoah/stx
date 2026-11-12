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
 * Plugin Grid / Catalog — table ↔ card via shared MorphCatalog.
 * 插件目录：通过共享 MorphCatalog 在表格与卡片间切换。
 */

'use client';

import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Download, ExternalLink} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {DropdownMenuItem} from '@/components/ui/dropdown-menu';
import {Pagination} from '@/components/ui/pagination';
import {
  MarketplaceMorphCatalog,
  type MarketplaceCatalogItem,
  type MarketplaceSelectionState,
  type MarketplaceViewMode,
} from '@/components/common/marketplace';
import type {ReactNode} from 'react';
import {useLocale} from '@/lib/i18n';
import {buildSeatunnelConnectorDocsUrl} from '@/lib/seatunnel-version';
import type {Plugin} from '@/lib/services/plugin';
import {getPluginDependencyStatusMeta} from './dependency-status';
import {
  generateConnectorDocUrls,
  getConnectorSemanticIcon,
} from './connector-quick-helper';

interface PluginGridProps {
  plugins: Plugin[];
  loading: boolean;
  error?: string | null;
  viewMode: MarketplaceViewMode;
  onViewDetail: (plugin: Plugin) => void;
  showInstallButton?: boolean;
  onInstall?: (plugin: Plugin) => void;
  onDownload?: (plugin: Plugin) => void;
  downloadingPlugins?: Set<string>;
  downloadedPlugins?: Set<string>;
  downloadProgressMap?: Map<string, number>;
  failedDownloadMap?: Map<string, string>;
  installedClusterCountMap?: Map<string, number>;
  /** Table multi-select (hidden action bar in card view). / 表格多选（卡片视图保留选中但不显示操作条）。 */
  selection?: MarketplaceSelectionState;
  bulkActions?: ReactNode;
  bulkHint?: ReactNode;
}

const DEFAULT_PAGE_SIZE = 12;

/**
 * Available-plugins catalog with pagination and table/card morph.
 * 「在线可用」目录：分页 + 表/卡形变。
 */
export function PluginGrid({
  plugins,
  loading,
  error = null,
  viewMode,
  onViewDetail,
  showInstallButton = false,
  onInstall,
  onDownload,
  downloadingPlugins = new Set(),
  downloadedPlugins = new Set(),
  downloadProgressMap = new Map(),
  failedDownloadMap = new Map(),
  installedClusterCountMap = new Map(),
  selection,
  bulkActions,
  bulkHint,
}: PluginGridProps) {
  const t = useTranslations();
  const {locale} = useLocale();
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);

  const totalItems = plugins.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize) || 1);

  const currentPlugins = useMemo(() => {
    const startIndex = (currentPage - 1) * pageSize;
    return plugins.slice(startIndex, startIndex + pageSize);
  }, [plugins, currentPage, pageSize]);

  useEffect(() => {
    if (currentPage > 1 && currentPage > totalPages) {
      setCurrentPage(1);
    }
  }, [plugins, totalPages, currentPage]);

  const items: MarketplaceCatalogItem[] = useMemo(() => {
    return currentPlugins.map((plugin) => {
      const downloadKey = `${plugin.name}:${plugin.version}`;
      const isDownloaded = downloadedPlugins.has(downloadKey);
      const isDownloadFailed = failedDownloadMap.has(downloadKey);
      const failedMessage = isDownloadFailed
        ? (failedDownloadMap.get(downloadKey) ?? null)
        : null;
      const isDownloading =
        !isDownloaded &&
        !isDownloadFailed &&
        downloadingPlugins.has(downloadKey);
      const progress = isDownloaded
        ? 100
        : (downloadProgressMap.get(downloadKey) ?? 0);
      const installedClusterCount =
        installedClusterCountMap.get(downloadKey) ?? 0;
      const semanticIcon = getConnectorSemanticIcon(
        plugin.name,
        plugin.category,
      );
      const IconComponent = semanticIcon.icon;
      const dependencyMeta = getPluginDependencyStatusMeta(plugin, t);
      const docUrls = generateConnectorDocUrls(
        plugin.name,
        plugin.version,
        locale,
        plugin.category,
      );
      // Prefer a real connector page; only fall back to the connectors index when none exist.
      // 优先真实连接器文档页；两边都不存在时才回退到连接器索引。
      const docsUrl =
        docUrls.sourceDocUrl ||
        docUrls.sinkDocUrl ||
        undefined;
      const docsFallbackUrl = buildSeatunnelConnectorDocsUrl(
        plugin.version,
        locale,
      );
      const docsHref = docsUrl ?? docsFallbackUrl;
      const hasExactDocs = Boolean(docsUrl);

      let status: MarketplaceCatalogItem['status'];
      let progressBlock: MarketplaceCatalogItem['progress'] = null;
      if (isDownloadFailed) {
        status = {
          label: t('marketplaceUi.statusFailed'),
          tone: 'error',
        };
      } else if (isDownloading) {
        status = {
          label: t('marketplaceUi.statusDownloading'),
          tone: 'info',
        };
        progressBlock = {
          label: t('marketplaceUi.progressDownload'),
          value: Math.max(0, Math.min(100, progress)),
          display: `${Math.max(0, Math.min(100, progress))}%`,
        };
      } else if (isDownloaded) {
        status = {
          label: t('marketplaceUi.statusReady'),
          tone: 'ok',
        };
      } else {
        status = {
          label: t('marketplaceUi.statusAvailable'),
          tone: 'muted',
        };
      }

      const badges: MarketplaceCatalogItem['badges'] = [
        {
          label: t(`plugin.category.${plugin.category}`),
        },
      ];
      if (semanticIcon.tagLabel) {
        badges.push({label: semanticIcon.tagLabel});
      }
      if (installedClusterCount > 0) {
        badges.push({
          label: `${t('plugin.installed')} · ${installedClusterCount}`,
          highlight: true,
        });
      }

      // 文档等次要入口进「…」菜单；无精确文档时标明指向索引页。
      // Docs stay in the «…» menu; label index fallback when no exact page exists.
      const menuActions = (
        <DropdownMenuItem
          onClick={(e) => {
            e.stopPropagation();
            window.open(docsHref, '_blank', 'noopener,noreferrer');
          }}
        >
          <ExternalLink className='mr-2 size-4' />
          {hasExactDocs
            ? t('marketplaceUi.viewDocs')
            : t('marketplaceUi.viewDocsIndex')}
        </DropdownMenuItem>
      );

      let primaryActions: MarketplaceCatalogItem['primaryActions'];
      if (showInstallButton && isDownloaded && onInstall) {
        primaryActions = (
          <Button
            type='button'
            size='sm'
            className='h-8 rounded-md text-xs'
            data-testid={`plugin-install-${downloadKey}`}
            onClick={(e) => {
              e.stopPropagation();
              onInstall(plugin);
            }}
          >
            <Download className='mr-1.5 size-3.5' />
            {t('plugin.install')}
          </Button>
        );
      } else if (
        showInstallButton &&
        !isDownloaded &&
        !isDownloading &&
        onDownload
      ) {
        primaryActions = (
          <Button
            type='button'
            size='sm'
            className='h-8 rounded-md text-xs'
            data-testid={`plugin-download-${downloadKey}`}
            onClick={(e) => {
              e.stopPropagation();
              onDownload(plugin);
            }}
          >
            <Download className='mr-1.5 size-3.5' />
            {isDownloadFailed ? t('common.retry') : t('plugin.download')}
          </Button>
        );
      }

      return {
        id: downloadKey,
        title: plugin.display_name || plugin.name,
        // 正面展示版本；完整简介挪到背面。
        // Version on the front; full blurb moves to the card back.
        subtitle: `v${plugin.version}`,
        description: plugin.description || undefined,
        icon: (
          <span
            className={`${semanticIcon.bgColor} ${semanticIcon.textColor} inline-flex size-9 items-center justify-center rounded-full`}
          >
            {semanticIcon.iconUrl ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={semanticIcon.iconUrl}
                alt=''
                className='size-5 object-contain'
                loading='lazy'
              />
            ) : (
              <IconComponent className='size-4' />
            )}
          </span>
        ),
        badges,
        status,
        // 背面：依赖、集群、失败详情等次要信息。
        // Card back: deps, clusters, failure detail — secondary only.
        fields: [
          {
            label: t('plugin.officialDependencyStatus'),
            value: dependencyMeta.label,
          },
          ...(installedClusterCount > 0
            ? [
                {
                  label: t('marketplaceUi.fieldClusters'),
                  value: String(installedClusterCount),
                },
              ]
            : []),
          ...(failedMessage
            ? [
                {
                  label: t('marketplaceUi.statusFailed'),
                  value: failedMessage,
                },
              ]
            : []),
        ],
        progress: progressBlock,
        meta: `v${plugin.version}`,
        primaryActions,
        menuActions,
        onOpen: () => onViewDetail(plugin),
      };
    });
  }, [
    currentPlugins,
    downloadProgressMap,
    downloadedPlugins,
    downloadingPlugins,
    failedDownloadMap,
    installedClusterCountMap,
    locale,
    onDownload,
    onInstall,
    onViewDetail,
    showInstallButton,
    t,
  ]);

  return (
    <div className='space-y-4' data-testid='plugin-available-grid'>
      <MarketplaceMorphCatalog
        items={items}
        viewMode={viewMode}
        loading={loading}
        error={error}
        empty={t('plugin.noPluginsFound')}
        selection={selection}
        bulkActions={bulkActions}
        bulkHint={bulkHint}
        data-testid='plugin-available-catalog'
      />
      {totalItems > 0 && (
        <Pagination
          currentPage={currentPage}
          totalPages={totalPages}
          pageSize={pageSize}
          totalItems={totalItems}
          onPageChange={setCurrentPage}
          onPageSizeChange={(size) => {
            setPageSize(size);
            setCurrentPage(1);
          }}
          pageSizeOptions={[12, 24, 48, 96]}
          showPageSizeSelector={true}
          showTotalItems={true}
        />
      )}
    </div>
  );
}
