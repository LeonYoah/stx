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
 * Plugin Selection Step Component
 * 插件选择步骤组件
 *
 * Allows users to select plugins to install with SeaTunnel.
 * 允许用户选择要与 SeaTunnel 一起安装的插件。
 */

'use client';

import {useState, useMemo, useCallback} from 'react';
import {useTranslations} from 'next-intl';
import {useAvailablePlugins} from '@/hooks/use-plugin';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Input} from '@/components/ui/input';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/ui/checkbox';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Label} from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {cn} from '@/lib/utils';
import {
  Search,
  Package,
  Info,
  Loader2,
  CheckCircle2,
  Layers3,
} from 'lucide-react';
import {toast} from 'sonner';
import type {Plugin, MirrorSource} from '@/lib/services/plugin/types';
import {PluginDetailDialog} from '@/components/common/plugin/PluginDetailDialog';
import {getPluginDependencyStatusMeta} from '@/components/common/plugin/dependency-status';

function normalizePluginDescription(description: string): string {
  return description
    .replace(/connector/gi, 'plugin')
    .replace(/连接器/g, '插件');
}

interface PluginSelectStepProps {
  /** SeaTunnel version / SeaTunnel 版本 */
  version: string;
  /** Mirror source / 镜像源 */
  mirror: MirrorSource;
  /** Callback when mirror source changes / 镜像源变化时的回调 */
  onMirrorChange?: (mirror: MirrorSource) => void;
  /** Selected plugin names / 已选择的插件名称 */
  selectedPlugins: string[];
  /** Callback when plugins selection changes / 插件选择变化时的回调 */
  onPluginsChange: (plugins: string[]) => void;
  /** Selected profile keys by plugin / 按插件记录画像选择 */
  selectedPluginProfiles?: Record<string, string[]>;
  /** Callback when plugin profile selection changes / 插件画像选择变化回调 */
  onPluginProfilesChange?: (pluginName: string, profileKeys: string[]) => void;
}

export function PluginSelectStep({
  version,
  mirror,
  onMirrorChange,
  selectedPlugins,
  onPluginsChange,
  selectedPluginProfiles = {},
  onPluginProfilesChange,
}: PluginSelectStepProps) {
  const t = useTranslations();
  const [searchQuery, setSearchQuery] = useState('');
  const [detailPlugin, setDetailPlugin] = useState<Plugin | null>(null);

  const getSelectedProfileKeys = useCallback(
    (pluginName: string) => selectedPluginProfiles[pluginName] || [],
    [selectedPluginProfiles],
  );

  const requiresProfileSelection = useCallback(
    (plugin: Plugin) =>
      plugin.artifact_id === 'connector-jdbc' || plugin.name === 'jdbc',
    [],
  );

  // Fetch available plugins / 获取可用插件
  const {plugins, loading, error} = useAvailablePlugins(version, mirror);

  // Filter and search plugins / 过滤和搜索插件
  const filteredPlugins = useMemo(() => {
    let result = plugins;
    if (searchQuery.trim()) {
      const lowerQuery = searchQuery.toLowerCase();
      result = result.filter(
        (p) =>
          p.name.toLowerCase().includes(lowerQuery) ||
          p.display_name.toLowerCase().includes(lowerQuery) ||
          p.description?.toLowerCase().includes(lowerQuery),
      );
    }

    return result;
  }, [plugins, searchQuery]);

  // Render plugin card / 渲染插件卡片
  const renderPluginCard = (plugin: Plugin) => {
    const isSelected = selectedPlugins.includes(plugin.name);
    const selectedProfileKeys = getSelectedProfileKeys(plugin.name);
    const profileRequired = requiresProfileSelection(plugin);
    const profileMissing = profileRequired && selectedProfileKeys.length === 0;
    const dependencyMeta = getPluginDependencyStatusMeta(plugin, t);

    const handleToggle = () => {
      const nextSelected = !isSelected;
      if (nextSelected) {
        onPluginsChange([...selectedPlugins, plugin.name]);
        if (profileMissing) {
          setDetailPlugin(plugin);
          toast.info(t('plugin.selectScenarioFirst'));
        }
        return;
      }
      onPluginsChange(selectedPlugins.filter((p) => p !== plugin.name));
    };

    return (
      <div
        key={plugin.name}
        data-testid={`install-plugin-card-${plugin.name}`}
        className={cn(
          'relative flex items-start gap-3 p-3 rounded-lg border transition-colors cursor-pointer',
          isSelected
            ? 'border-primary bg-primary/5'
            : 'border-muted hover:border-muted-foreground/50',
        )}
        onClick={handleToggle}
      >
        {/* Checkbox / 复选框 */}
        <Checkbox
          checked={isSelected}
          onCheckedChange={() => handleToggle()}
          className='mt-1'
        />

        {/* Plugin info / 插件信息 */}
        <div className='flex-1 min-w-0'>
          <div className='mb-1 flex items-center gap-2'>
            <span className='font-medium text-sm truncate'>
              {plugin.display_name || plugin.name}
            </span>
          </div>

          <p className='text-xs text-muted-foreground line-clamp-2'>
            {plugin.description
              ? normalizePluginDescription(plugin.description)
              : t('plugin.noDescription')}
          </p>

          <div className='mt-2 flex flex-wrap items-center gap-2'>
            <Badge
              variant='outline'
              className={cn('text-xs', dependencyMeta.className)}
            >
              {dependencyMeta.label}
            </Badge>
            {selectedProfileKeys.length > 0 && (
              <Badge variant='secondary' className='text-xs'>
                <Layers3 className='mr-1 h-3 w-3' />
                {t('plugin.selectedProfiles')}: {selectedProfileKeys.length}
              </Badge>
            )}
          </div>
          {profileMissing && isSelected ? (
            <p className='mt-2 text-xs text-amber-600 dark:text-amber-400'>
              {t('plugin.selectProfilesToPreview')}
            </p>
          ) : (
            <p className='mt-2 text-xs text-muted-foreground line-clamp-2'>
              {dependencyMeta.description}
            </p>
          )}
        </div>

        {/* Info button / 信息按钮 */}
        <Button
          variant='ghost'
          size='icon'
          data-testid={`install-plugin-details-${plugin.name}`}
          className='h-6 w-6 flex-shrink-0'
          onClick={(e) => {
            e.stopPropagation();
            setDetailPlugin(plugin);
          }}
        >
          <Info className='h-4 w-4' />
        </Button>
      </div>
    );
  };

  return (
    <div className='space-y-4'>
      {/* Header / 头部 */}
      <Card>
        <CardHeader className='pb-3'>
          <div className='flex items-center justify-between'>
            <div>
              <CardTitle className='text-lg flex items-center gap-2'>
                <Package className='h-5 w-5' />
                {t('installer.pluginSelection') || 'Plugin Selection'}
              </CardTitle>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <Badge variant='outline' className='gap-1'>
                <CheckCircle2 className='h-3 w-3' />
                {selectedPlugins.length}{' '}
                {t('plugin.selectedCount') || 'selected'}
              </Badge>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Search and mirror selection / 搜索与镜像源选择 */}
          <div className='flex flex-col gap-4 lg:flex-row lg:items-end'>
            <div className='relative flex-1'>
              <Search className='absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground' />
              <Input
                placeholder={t('plugin.searchPlaceholder')}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className='pl-9'
              />
            </div>
            <div className='space-y-2 lg:w-[220px]'>
              <Label>{t('installer.mirrorSource')}</Label>
              <Select
                value={mirror}
                onValueChange={(value: MirrorSource) => onMirrorChange?.(value)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='aliyun'>
                    {t('installer.mirrors.aliyun')}
                  </SelectItem>
                  <SelectItem value='huaweicloud'>
                    {t('installer.mirrors.huaweicloud')}
                  </SelectItem>
                  <SelectItem value='apache'>
                    {t('installer.mirrors.apache')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Plugin list / 插件列表 */}
      <Card>
        <CardContent className='pt-6'>
          {loading ? (
            <div className='flex items-center justify-center py-12'>
              <Loader2 className='h-8 w-8 animate-spin text-muted-foreground' />
            </div>
          ) : error ? (
            <div className='text-center py-12 text-destructive'>
              <p>{error}</p>
            </div>
          ) : filteredPlugins.length === 0 ? (
            <div className='text-center py-12 text-muted-foreground'>
              <Package className='h-12 w-12 mx-auto mb-4 opacity-50' />
              <p>{t('plugin.noPluginsFound')}</p>
            </div>
          ) : (
            <ScrollArea className='h-[350px] pr-4'>
              <div className='grid grid-cols-1 gap-2 md:grid-cols-2'>
                {filteredPlugins.map(renderPluginCard)}
              </div>
            </ScrollArea>
          )}
        </CardContent>
      </Card>

      {detailPlugin && (
        <PluginDetailDialog
          open={!!detailPlugin}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) {
              setDetailPlugin(null);
            }
          }}
          plugin={detailPlugin}
          seatunnelVersion={version}
          selectedProfileKeys={getSelectedProfileKeys(detailPlugin.name)}
          onSelectedProfileKeysChange={(keys) =>
            onPluginProfilesChange?.(detailPlugin.name, keys)
          }
        />
      )}
    </div>
  );
}

export default PluginSelectStep;
