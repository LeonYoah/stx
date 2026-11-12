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
import {DropdownMenuItem} from '@/components/ui/dropdown-menu';
import {Pagination} from '@/components/ui/pagination';
import {
  MarketplaceMorphCatalog,
  type MarketplaceCatalogItem,
  type MarketplaceViewMode,
} from '@/components/common/marketplace';
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
      const docsUrl =
        docUrls.sourceDocUrl ||
        docUrls.sinkDocUrl ||
        buildSeatunnelConnectorDocsUrl(plugin.version, locale);

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

      const menuActions = (
        <>
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              window.open(docsUrl, '_blank', 'noopener,noreferrer');
            }}
          >
            <ExternalLink className='mr-2 size-4' />
            {t('marketplaceUi.viewDocs')}
          </DropdownMenuItem>
          {showInstallButton && isDownloaded && onInstall ? (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onInstall(plugin);
              }}
            >
              <Download className='mr-2 size-4' />
              {t('plugin.install')}
            </DropdownMenuItem>
          ) : null}
          {showInstallButton &&
          !isDownloaded &&
          !isDownloading &&
          onDownload ? (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onDownload(plugin);
              }}
            >
              <Download className='mr-2 size-4' />
              {t('plugin.download')}
            </DropdownMenuItem>
          ) : null}
        </>
      );

      return {
        id: downloadKey,
        title: plugin.display_name || plugin.name,
        subtitle: plugin.description || plugin.name,
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
        fields: [
          {label: t('marketplaceUi.fieldVersion'), value: `v${plugin.version}`},
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
