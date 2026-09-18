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
 * Plugin Card Component
 * 插件卡片组件
 */

'use client';

import {useTranslations} from 'next-intl';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {ExternalLink, Download, CheckCircle} from 'lucide-react';
import {useLocale} from '@/lib/i18n';
import {buildSeatunnelConnectorDocsUrl} from '@/lib/seatunnel-version';
import type {Plugin} from '@/lib/services/plugin';
import {getPluginDependencyStatusMeta} from './dependency-status';
import {
  getConnectorSemanticIcon,
  generateConnectorDocUrls,
} from './connector-quick-helper';

interface PluginCardProps {
  plugin: Plugin;
  onClick: () => void;
  showInstallButton?: boolean;
  isInstalled?: boolean;
  isDownloaded?: boolean;
  isDownloading?: boolean;
  downloadProgress?: number;
  onInstall?: () => void;
  onDownload?: () => void;
}

/**
 * Plugin Card Component
 * 插件卡片组件 - 展示单个插件的基本信息
 */
export function PluginCard({
  plugin,
  onClick,
  showInstallButton = false,
  isInstalled = false,
  isDownloaded = false,
  isDownloading = false,
  downloadProgress = 0,
  onInstall,
  onDownload,
}: PluginCardProps) {
  const t = useTranslations();
  const {locale} = useLocale();
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
  const dependencyMeta = getPluginDependencyStatusMeta(plugin, t);
  const semanticIcon = getConnectorSemanticIcon(plugin.name, plugin.category);
  const IconComponent = semanticIcon.icon;

  /**
   * Handle install button click
   * 处理安装按钮点击
   */
  const handleInstallClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onInstall?.();
  };

  /**
   * Handle download button click
   * 处理下载按钮点击
   */
  const handleDownloadClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDownload?.();
  };

  return (
    <Card
      data-testid={`plugin-card-${plugin.name}`}
      className='cursor-pointer transition-all duration-200 hover:shadow-md hover:border-primary/50'
      onClick={onClick}
    >
      <CardHeader className='pb-3'>
        <div className='flex items-start justify-between'>
          <div className='flex items-center gap-3'>
            <div
              className={`p-2 rounded-lg ${semanticIcon.bgColor} ${semanticIcon.textColor} flex items-center justify-center w-9 h-9 shrink-0`}
            >
              {semanticIcon.iconUrl ? (
                <img
                  src={semanticIcon.iconUrl}
                  alt={plugin.display_name || plugin.name}
                  className='h-5 w-5 object-contain'
                  loading='lazy'
                  onError={(e) => {
                    (e.target as HTMLElement).style.display = 'none';
                    const fallback = (e.target as HTMLElement).nextElementSibling;
                    if (fallback) (fallback as HTMLElement).style.display = 'block';
                  }}
                />
              ) : null}
              <IconComponent
                className={`h-5 w-5 ${semanticIcon.iconUrl ? 'hidden' : ''}`}
              />
            </div>
            <div>
              <CardTitle className='text-base font-semibold line-clamp-1'>
                {plugin.display_name || plugin.name}
              </CardTitle>
              <CardDescription className='text-xs mt-0.5'>
                {plugin.name}
              </CardDescription>
            </div>
          </div>
          <a
            href={docsUrl}
            target='_blank'
            rel='noopener noreferrer'
            onClick={(e) => e.stopPropagation()}
            className='text-muted-foreground hover:text-primary'
          >
            <ExternalLink className='h-4 w-4' />
          </a>
        </div>
      </CardHeader>
      <CardContent className='pt-0'>
        <p className='text-sm text-muted-foreground line-clamp-2 mb-3 min-h-[2.5rem]'>
          {plugin.description || t('plugin.noDescription')}
        </p>
        <div className='flex items-center justify-between'>
          <div className='flex items-center gap-1.5 flex-wrap'>
            <Badge
              variant='outline'
              className={`${semanticIcon.bgColor} ${semanticIcon.textColor}`}
            >
              {t(`plugin.category.${plugin.category}`)}
            </Badge>
            {semanticIcon.tagLabel && (
              <Badge variant='secondary' className='text-[10px] px-1.5 py-0'>
                {semanticIcon.tagLabel}
              </Badge>
            )}
          </div>
          <span className='text-xs text-muted-foreground'>
            v{plugin.version}
          </span>
        </div>
        <div className='mt-3 rounded-md border border-dashed px-3 py-2 min-h-[66px]'>
          <div className='flex items-center justify-between gap-2'>
            <span className='text-xs font-medium text-muted-foreground'>
              {t('plugin.officialDependencyStatus')}
            </span>
            <Badge variant='outline' className={dependencyMeta.className}>
              {dependencyMeta.label}
            </Badge>
          </div>
          <p className='mt-1 text-xs text-muted-foreground line-clamp-2'>
            {dependencyMeta.description}
          </p>
        </div>

        {/* Install/Download/Status button / 安装/下载/状态按钮 */}
        {showInstallButton && (
          <div className='mt-3 pt-3 border-t space-y-2'>
            {isInstalled ? (
              <Button variant='outline' size='sm' className='w-full' disabled>
                <CheckCircle className='h-4 w-4 mr-2 text-green-600' />
                {t('plugin.installed')}
              </Button>
            ) : isDownloading ? (
              <div className='space-y-2'>
                <div className='flex items-center justify-between text-xs text-muted-foreground'>
                  <span>{t('plugin.downloading')}</span>
                  <span>{downloadProgress}%</span>
                </div>
                <div className='w-full bg-gray-200 rounded-full h-1.5 dark:bg-gray-700'>
                  <div
                    className='bg-blue-600 h-1.5 rounded-full transition-all duration-300'
                    style={{width: `${downloadProgress}%`}}
                  />
                </div>
              </div>
            ) : isDownloaded ? (
              <Button
                variant='outline'
                size='sm'
                className='w-full'
                onClick={handleInstallClick}
              >
                <Download className='h-4 w-4 mr-2' />
                {t('plugin.install')}
              </Button>
            ) : (
              <Button
                variant='outline'
                size='sm'
                className='w-full'
                onClick={handleDownloadClick}
              >
                <Download className='h-4 w-4 mr-2' />
                {t('plugin.download')}
              </Button>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
