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
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Alert} from '@/components/arc/alert/alert';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {NeutralChip, SoftChip} from '@/components/common/layout';
import {cn} from '@/lib/utils';
import {
  Package,
  Info,
  CheckCircle2,
  Layers3,
} from 'lucide-react';
import {toast} from '@/lib/toast';
import type {Plugin, MirrorSource} from '@/lib/services/plugin/types';
import {PluginDetailDialog} from '@/components/common/plugin/PluginDetailDialog';
import {getPluginDependencyStatusMeta} from '@/components/common/plugin/dependency-status';

function normalizePluginDescription(description: string): string {
  return description
    .replace(/\bconnector\b/gi, 'plugin')
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
  /**
   * 紧凑模式：去双层卡片与重复标题，仅保留一行工具栏 + 原生滚动列表
   * Compact: drop the nested cards/titles, keep one toolbar row + native scroll list
   */
  compact?: boolean;
  /** 隐藏镜像源选择（集群向导中已在配置步统一设置）/ Hide mirror picker when configured centrally */
  hideMirror?: boolean;
}

export function PluginSelectStep({
  version,
  mirror,
  onMirrorChange,
  selectedPlugins,
  onPluginsChange,
  selectedPluginProfiles = {},
  onPluginProfilesChange,
  compact = false,
  hideMirror = false,
}: PluginSelectStepProps) {
  const t = useTranslations();
  const [searchQuery, setSearchQuery] = useState('');
  const [detailPlugin, setDetailPlugin] = useState<Plugin | null>(null);
  const [detailInitialTab, setDetailInitialTab] = useState<
    'overview' | 'dependencies'
  >('overview');

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
          // 需选场景时直接定位到官方依赖/场景 Tab，一步勾选无需手动切 Tab。
          // Jump straight to the dependencies/profiles tab when scenario selection is required.
          setDetailInitialTab('dependencies');
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
          onClick={(e) => e.stopPropagation()}
          aria-label={plugin.display_name || plugin.name}
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
            <NeutralChip className={cn(dependencyMeta.className)}>
              {dependencyMeta.label}
            </NeutralChip>
            {selectedProfileKeys.length > 0 && (
              <SoftChip tone='blue' className='tabular-nums'>
                <Layers3 className='h-3 w-3' aria-hidden />
                {t('plugin.selectedProfiles')}: {selectedProfileKeys.length}
              </SoftChip>
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
          size='sm'
          data-testid={`install-plugin-details-${plugin.name}`}
          aria-label={plugin.display_name || plugin.name}
          className='flex-shrink-0'
          onClick={(e) => {
            e.stopPropagation();
            setDetailInitialTab(profileRequired ? 'dependencies' : 'overview');
            setDetailPlugin(plugin);
          }}
        >
          <Info className='size-3.5' />
        </Button>
      </div>
    );
  };

  const listBody = loading ? (
    <div className={cn(!compact && 'py-6')}>
      <Skeleton lines={6} label={t('common.loading')} />
    </div>
  ) : error ? (
    <Alert tone='danger' title={t('plugin.loadError')}>
      {error}
    </Alert>
  ) : filteredPlugins.length === 0 ? (
    <EmptyState
      icon={<Package className='size-5' aria-hidden />}
      title={t('plugin.noPluginsFound')}
      description={
        searchQuery.trim()
          ? t('plugin.filteredEmptyDesc')
          : t('plugin.noDescription')
      }
      className='py-10'
      action={
        searchQuery.trim() ? (
          <Button
            type='button'
            variant='secondary'
            size='sm'
            onClick={() => setSearchQuery('')}
          >
            {t('plugin.clearSearch')}
          </Button>
        ) : undefined
      }
    />
  ) : (
    <div
      className={cn(
        'overflow-y-auto pr-4',
        compact ? 'flex-1 min-h-0' : 'h-[350px]',
      )}
    >
      <div className='grid grid-cols-1 gap-2 md:grid-cols-2'>
        {filteredPlugins.map(renderPluginCard)}
      </div>
    </div>
  );

  const mirrorSelect = hideMirror ? null : (
    <div className={cn(compact && 'shrink-0')}>
      <Select
        label={t('installer.mirrorSource')}
        aria-label={t('installer.mirrorSource')}
        value={mirror}
        onValueChange={(value) => onMirrorChange?.(value as MirrorSource)}
        options={[
          {value: 'aliyun', label: t('installer.mirrors.aliyun')},
          {value: 'huaweicloud', label: t('installer.mirrors.huaweicloud')},
          {value: 'apache', label: t('installer.mirrors.apache')},
        ]}
      />
    </div>
  );

  const detailDialog = detailPlugin ? (
    <PluginDetailDialog
      open={!!detailPlugin}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setDetailPlugin(null);
        }
      }}
      plugin={detailPlugin}
      seatunnelVersion={version}
      initialTab={detailInitialTab}
      selectedProfileKeys={getSelectedProfileKeys(detailPlugin.name)}
      onSelectedProfileKeysChange={(keys) =>
        onPluginProfilesChange?.(detailPlugin.name, keys)
      }
    />
  ) : null;

  // 紧凑布局：一行工具栏 + 充满剩余高度的原生滚动列表
  // Compact: single toolbar row + native scroll list filling the rest
  if (compact) {
    return (
      <div className='h-full min-h-0 flex flex-col gap-3'>
        <div className='flex items-center gap-3 shrink-0'>
          <div className='flex-1 min-w-0'>
            <SearchField
              label={t('common.search')}
              placeholder={t('plugin.searchPlaceholder')}
              value={searchQuery}
              onValueChange={setSearchQuery}
            />
          </div>
          {mirrorSelect}
          <NeutralChip className='shrink-0 font-mono tabular-nums'>
            <CheckCircle2 className='h-3 w-3' aria-hidden />
            {t('cluster.wizard.pluginsSelectedCount', {
              count: selectedPlugins.length,
            })}
          </NeutralChip>
        </div>
        {listBody}
        {detailDialog}
      </div>
    );
  }

  return (
    <div className='space-y-4'>
      {/* Header / 头部 */}
      <div className='space-y-3'>
        <div className='flex items-center justify-between'>
          <h3 className='text-sm font-semibold flex items-center gap-2'>
            <Package className='h-4 w-4' aria-hidden />
            {t('installer.pluginSelection')}
          </h3>
          <NeutralChip className='font-mono tabular-nums'>
            <CheckCircle2 className='h-3 w-3' aria-hidden />
            {selectedPlugins.length} {t('plugin.selectedCount')}
          </NeutralChip>
        </div>

        {/* Search and mirror selection / 搜索与镜像源选择 */}
        <div className='flex flex-col gap-4 lg:flex-row lg:items-end'>
          <div className='relative flex-1'>
            <SearchField
              label={t('common.search')}
              placeholder={t('plugin.searchPlaceholder')}
              value={searchQuery}
              onValueChange={setSearchQuery}
            />
          </div>
          <div className='lg:w-[220px]'>{mirrorSelect}</div>
        </div>
      </div>

      {/* Plugin list / 插件列表 */}
      {listBody}

      {detailDialog}
    </div>
  );
}

export default PluginSelectStep;
