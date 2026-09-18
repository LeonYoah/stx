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

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Checkbox} from '@/components/ui/checkbox';
import {Input} from '@/components/ui/input';
import {Tabs, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {toast} from 'sonner';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Ban,
  BookOpen,
  Check,
  Copy,
  Download,
  ExternalLink,
  FolderOpen,
  Layers3,
  Package,
  RefreshCw,
  RotateCcw,
  Search,
  Sparkles,
} from 'lucide-react';
import type {
  OfficialDependenciesResponse,
  Plugin,
  PluginDependency,
  PluginDependencyDisable,
} from '@/lib/services/plugin';
import {useLocale} from '@/lib/i18n';
import {PluginService} from '@/lib/services/plugin';
import {openSeaTunnelAskAi} from '@/lib/services/kapa-ai';
import {PluginDependencyConfigSection} from './DependencyConfigDialog';
import {getPluginDependencyStatusMeta} from './dependency-status';
import {
  generateConnectorDocUrls,
  getConnectorCapability,
  getConnectorSemanticIcon,
} from './connector-quick-helper';

interface PluginDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plugin: Plugin;
  seatunnelVersion?: string;
  selectedProfileKeys?: string[];
  onSelectedProfileKeysChange?: (keys: string[]) => void;
  onDownload?: (plugin: Plugin, profileKeys: string[]) => void;
}

/**
 * Check if the SeaTunnel version supports isolated dependency directory
 * 检查 SeaTunnel 版本是否支持隔离依赖目录 (plugins/<mapping>/lib)
 */
function supportsIsolatedDependency(version?: string): boolean {
  if (!version) {
    return false;
  }
  const parse = (input: string) =>
    input
      .split('.')
      .map((part) => Number.parseInt(part.split('-')[0] || '0', 10) || 0);
  const current = parse(version);
  const baseline = [2, 3, 12];
  for (let index = 0; index < baseline.length; index += 1) {
    const left = current[index] ?? 0;
    const right = baseline[index];
    if (left !== right) {
      return left > right;
    }
  }
  return true;
}

/**
 * Remove duplicate strings
 * 数组去重辅助方法
 */
function dedupeStrings(values: string[]): string[] {
  return Array.from(new Set(values.filter(Boolean)));
}

/**
 * Build test identifier for dependency actions
 * 构建依赖操作的测试标识符
 */
function buildDependencyActionId(prefix: string, artifactId: string): string {
  return `${prefix}-${artifactId.replace(/[^a-z0-9]+/gi, '-').replace(/^-+|-+$/g, '')}`.toLowerCase();
}

/**
 * Plugin Detail Dialog Component
 * 插件详情弹窗组件 - 展示连接器元数据、官方文档直达、附属依赖与自定义驱动配置
 */
export function PluginDetailDialog({
  open,
  onOpenChange,
  plugin,
  seatunnelVersion,
  selectedProfileKeys = [],
  onSelectedProfileKeysChange,
  onDownload,
}: PluginDetailDialogProps) {
  const t = useTranslations();
  const {locale} = useLocale();

  // State management / 状态管理
  const [officialDeps, setOfficialDeps] =
    useState<OfficialDependenciesResponse | null>(null);
  const [loadingOfficialDeps, setLoadingOfficialDeps] = useState(false);
  const [operatingDependencyKey, setOperatingDependencyKey] = useState<
    string | null
  >(null);
  const [activeTab, setActiveTab] = useState<
    'all' | 'overview' | 'dependencies' | 'custom'
  >('all');
  const [profileFilter, setProfileFilter] = useState('');
  const [copiedCoord, setCopiedCoord] = useState(false);
  const [copiedXml, setCopiedXml] = useState(false);

  // Version and metadata resolution / 版本与元数据推导
  const effectiveVersion = seatunnelVersion || plugin.version;
  const semanticIcon = getConnectorSemanticIcon(plugin.name, plugin.category);
  const IconComponent = semanticIcon.icon;
  const capability = getConnectorCapability(plugin.name, plugin.category);
  const dependencyStatusMeta = getPluginDependencyStatusMeta(plugin, t);
  const useIsolatedDeps = supportsIsolatedDependency(effectiveVersion);

  // Load official dependencies / 加载官方依赖画像
  const loadOfficialDeps = useCallback(async () => {
    if (!plugin?.name) {
      return;
    }
    setLoadingOfficialDeps(true);
    try {
      const data = await PluginService.getOfficialDependencies(
        plugin.name,
        seatunnelVersion || plugin.version,
      );
      setOfficialDeps(data);
    } catch (error) {
      console.error('Failed to load official dependencies:', error);
      setOfficialDeps(null);
    } finally {
      setLoadingOfficialDeps(false);
    }
  }, [plugin?.name, plugin?.version, seatunnelVersion]);

  useEffect(() => {
    if (open && plugin?.name) {
      void loadOfficialDeps();
    }
  }, [loadOfficialDeps, open, plugin?.name]);

  // Profiles resolution / 画像列表与过滤
  const availableProfiles = useMemo(
    () => officialDeps?.profiles || [],
    [officialDeps?.profiles],
  );
  const hasMultiProfiles = availableProfiles.length > 1;

  const filteredProfiles = useMemo(() => {
    const q = profileFilter.trim().toLowerCase();
    if (!q) {
      return availableProfiles;
    }
    return availableProfiles.filter(
      (p) =>
        (p.profile_name || '').toLowerCase().includes(q) ||
        (p.profile_key || '').toLowerCase().includes(q),
    );
  }, [availableProfiles, profileFilter]);

  // Effective dependencies computation / 计算当前生效的官方依赖列表
  const effectiveDependencies = useMemo(() => {
    if (!officialDeps) {
      return [];
    }
    if (!hasMultiProfiles) {
      return officialDeps.effective_dependencies || [];
    }
    if (selectedProfileKeys.length === 0) {
      return [];
    }
    const selected = new Set(selectedProfileKeys);
    const merged = new Map<string, PluginDependency>();
    availableProfiles.forEach((profile) => {
      if (!selected.has(profile.profile_key)) {
        return;
      }
      (profile.items || []).forEach((item) => {
        if (item.disabled) {
          return;
        }
        const key = `${item.group_id}:${item.artifact_id}:${item.version}:${item.target_dir}`;
        if (!merged.has(key)) {
          merged.set(key, {
            group_id: item.group_id,
            artifact_id: item.artifact_id,
            version: item.version,
            target_dir: item.target_dir,
            source_type: 'official',
          });
        }
      });
    });
    return Array.from(merged.values());
  }, [availableProfiles, hasMultiProfiles, officialDeps, selectedProfileKeys]);

  const disabledDependencies = useMemo(
    () => officialDeps?.disabled_dependencies || [],
    [officialDeps?.disabled_dependencies],
  );

  const dependencyTargetDirs = useMemo(
    () =>
      dedupeStrings(
        effectiveDependencies
          .filter((item) => item.target_dir !== 'connectors')
          .map((item) => item.target_dir),
      ),
    [effectiveDependencies],
  );

  // Profile selection handlers / 场景画像勾选处理
  const handleToggleProfile = (profileKey: string, checked: boolean) => {
    const next = new Set(selectedProfileKeys);
    if (checked) {
      next.add(profileKey);
    } else {
      next.delete(profileKey);
    }
    onSelectedProfileKeysChange?.(Array.from(next).sort());
  };

  const handleSelectAllProfiles = () => {
    onSelectedProfileKeysChange?.(
      availableProfiles.map((p) => p.profile_key).sort(),
    );
  };

  const handleClearAllProfiles = () => {
    onSelectedProfileKeysChange?.([]);
  };

  // Official dependency enable/disable handlers / 依赖禁用与启用处理
  const handleDisableOfficialDependency = async (dep: PluginDependency) => {
    const actionKey = `${dep.group_id}:${dep.artifact_id}:${dep.version}:${dep.target_dir}:disable`;
    setOperatingDependencyKey(actionKey);
    try {
      await PluginService.disableOfficialDependency(plugin.name, {
        seatunnel_version: effectiveVersion,
        group_id: dep.group_id,
        artifact_id: dep.artifact_id,
        version: dep.version,
        target_dir: dep.target_dir,
      });
      toast.success(t('plugin.disableOfficialDependencySuccess'));
      await loadOfficialDeps();
    } catch (error) {
      const errorMsg =
        error instanceof Error
          ? error.message
          : t('plugin.disableOfficialDependencyFailed');
      toast.error(errorMsg);
    } finally {
      setOperatingDependencyKey(null);
    }
  };

  const handleEnableOfficialDependency = async (
    item: PluginDependencyDisable,
  ) => {
    const actionKey = `${item.id}:enable`;
    setOperatingDependencyKey(actionKey);
    try {
      await PluginService.enableOfficialDependency(plugin.name, item.id);
      toast.success(t('plugin.enableOfficialDependencySuccess'));
      await loadOfficialDeps();
    } catch (error) {
      const errorMsg =
        error instanceof Error
          ? error.message
          : t('plugin.enableOfficialDependencyFailed');
      toast.error(errorMsg);
    } finally {
      setOperatingDependencyKey(null);
    }
  };

  // Copy to clipboard helper / 复制到剪贴板辅助方法
  const handleCopyText = async (text: string, kind: 'coord' | 'xml') => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(t('plugin.copied'));
      if (kind === 'coord') {
        setCopiedCoord(true);
        setTimeout(() => setCopiedCoord(false), 2000);
      } else {
        setCopiedXml(true);
        setTimeout(() => setCopiedXml(false), 2000);
      }
    } catch {
      toast.error('Copy failed');
    }
  };

  /**
   * Handle opening official Apache SeaTunnel Ask AI modal with connector-targeted query
   * 处理唤起 Apache SeaTunnel 官网 Ask AI 智能问答对话框，并携带针对当前连接器的提问（支持中英双语国际化模板）
   */
  const handleOpenAskAi = async () => {
    const connectorName = plugin.display_name || plugin.name;
    const initialQuery = t('plugin.askAiQuestionTemplate', {
      connector: connectorName,
    });
    const success = await openSeaTunnelAskAi(initialQuery);
    if (!success) {
      toast.error(t('plugin.askAiUnavailable'), {
        description: t('plugin.askAiUnavailableDesc'),
      });
    }
  };

  const docUrls = useMemo(
    () =>
      generateConnectorDocUrls(
        plugin.name,
        effectiveVersion,
        locale,
        plugin.category,
      ),
    [effectiveVersion, locale, plugin.category, plugin.name],
  );

  const mavenCoordText = `${plugin.group_id}:${plugin.artifact_id}:${plugin.version}`;
  const mavenXmlText = `<dependency>\n  <groupId>${plugin.group_id}</groupId>\n  <artifactId>${plugin.artifact_id}</artifactId>\n  <version>${plugin.version}</version>\n</dependency>`;

  const downloadDisabled = hasMultiProfiles && selectedProfileKeys.length === 0;

  // Render helpers / 局部渲染逻辑
  const shouldRenderOverview = activeTab === 'all' || activeTab === 'overview';
  const shouldRenderDependencies =
    activeTab === 'all' || activeTab === 'dependencies';
  const shouldRenderCustom = activeTab === 'all' || activeTab === 'custom';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid={`plugin-detail-dialog-${plugin.name}`}
        className='w-[94vw] max-w-[94vw] sm:max-w-[760px] lg:max-w-[820px] h-[72vh] sm:h-[75vh] max-h-[75vh] overflow-hidden p-0 gap-0 flex flex-col'
      >
        {/* Header with high-density visual hierarchy / 高辨识度强化头部 */}
        <DialogHeader className='px-6 pt-5 pb-4 border-b shrink-0 bg-muted/10'>
          <div className='flex items-start justify-between gap-4'>
            <div className='flex items-center gap-3.5 min-w-0'>
              <div
                className={`p-2.5 rounded-lg shrink-0 flex items-center justify-center w-11 h-11 ${semanticIcon.bgColor} ${semanticIcon.textColor}`}
              >
                {semanticIcon.iconUrl ? (
                  <img
                    src={semanticIcon.iconUrl}
                    alt={plugin.display_name || plugin.name}
                    className='h-6 w-6 object-contain'
                    onError={(e) => {
                      (e.target as HTMLElement).style.display = 'none';
                      const fallback = (e.target as HTMLElement).nextElementSibling;
                      if (fallback) (fallback as HTMLElement).style.display = 'block';
                    }}
                  />
                ) : null}
                <IconComponent
                  className={`h-6 w-6 ${semanticIcon.iconUrl ? 'hidden' : ''}`}
                />
              </div>
              <div className='min-w-0'>
                <div className='flex items-center gap-2 flex-wrap'>
                  <DialogTitle className='text-lg sm:text-xl font-bold truncate'>
                    {plugin.display_name || plugin.name}
                  </DialogTitle>
                  <Badge
                    variant='outline'
                    className={`${semanticIcon.bgColor} ${semanticIcon.textColor}`}
                  >
                    {t(`plugin.category.${plugin.category}`)}
                  </Badge>
                  {semanticIcon.tagLabel && (
                    <Badge variant='secondary' className='text-xs'>
                      {semanticIcon.tagLabel}
                    </Badge>
                  )}
                  <Badge variant='secondary' className='font-mono text-xs'>
                    v{effectiveVersion}
                  </Badge>
                  {dependencyStatusMeta && (
                    <Badge
                      variant='outline'
                      className={`text-xs ${dependencyStatusMeta.className}`}
                    >
                      {dependencyStatusMeta.label}
                    </Badge>
                  )}
                </div>
                <DialogDescription className='text-xs text-muted-foreground mt-1 font-mono truncate'>
                  {plugin.name}
                </DialogDescription>
              </div>
            </div>

            {/* Quick header action for official docs / 顶部官方文档快捷直达入口 */}
            <div className='flex items-center gap-2 shrink-0 pr-8'>
              {(docUrls.sourceDocUrl || docUrls.sinkDocUrl) && (
                <Button
                  variant='outline'
                  size='sm'
                  className='h-7 text-xs hidden sm:inline-flex'
                  asChild
                >
                  <a
                    href={docUrls.sourceDocUrl || docUrls.sinkDocUrl}
                    target='_blank'
                    rel='noopener noreferrer'
                  >
                    <ExternalLink className='h-3.5 w-3.5 mr-1 text-primary' />
                    {t('plugin.officialDoc')}
                  </a>
                </Button>
              )}
            </div>
          </div>
        </DialogHeader>

        {/* Tab switcher bar / 顶层 Tab 分组控制栏 */}
        <div className='px-6 py-2.5 border-b bg-muted/20 shrink-0 flex items-center justify-between gap-2'>
          <Tabs
            value={activeTab}
            onValueChange={(val) =>
              setActiveTab(
                val as 'all' | 'overview' | 'dependencies' | 'custom',
              )
            }
          >
            <TabsList className='h-8'>
              <TabsTrigger
                value='all'
                data-testid='plugin-detail-tab-all'
                className='text-xs px-3'
              >
                {t('plugin.tabAll')}
              </TabsTrigger>
              <TabsTrigger
                value='overview'
                data-testid='plugin-detail-tab-overview'
                className='text-xs px-3'
              >
                {t('plugin.tabOverview')}
              </TabsTrigger>
              <TabsTrigger
                value='dependencies'
                data-testid='plugin-detail-tab-dependencies'
                className='text-xs px-3'
              >
                {t('plugin.tabDependencies')}
                {effectiveDependencies.length > 0 && (
                  <span className='ml-1 rounded-full bg-primary/10 px-1.5 py-0.2 text-[10px] font-semibold text-primary'>
                    {effectiveDependencies.length}
                  </span>
                )}
              </TabsTrigger>
              <TabsTrigger
                value='custom'
                data-testid='plugin-detail-tab-custom'
                className='text-xs px-3'
              >
                {t('plugin.tabCustom')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>

        {/* Main scrollable body with fixed flex layout / 固定高度防抖动的独立滚动主体 */}
        <div className='flex-1 min-h-0 overflow-hidden'>
          <ScrollArea className='h-full w-full'>
            <div className='p-6 space-y-6'>
            {/* Section 1: Overview & Official Documentation Guide / 第一部分：概览与官方文档直达 */}
            {shouldRenderOverview && (
              <div className='space-y-5 rounded-lg border bg-card p-5 shadow-xs'>
                {/* Plugin description / 插件描述 */}
                <div className='space-y-1.5'>
                  <h4 className='text-sm font-semibold flex items-center gap-2'>
                    <Package className='h-4 w-4 text-primary' />
                    {plugin.display_name || plugin.name}
                  </h4>
                  <p className='text-sm text-muted-foreground leading-relaxed'>
                    {plugin.description || t('plugin.noDescription')}
                  </p>
                </div>

                {/* Maven coordinates with 1-click copy / Maven 坐标与一键复制 */}
                <div className='rounded-md border bg-muted/30 p-3.5 space-y-2'>
                  <div className='flex items-center justify-between gap-2'>
                    <span className='text-xs font-medium flex items-center gap-1.5 text-muted-foreground'>
                      <Package className='h-3.5 w-3.5' />
                      {t('plugin.mavenCoordinates')}
                    </span>
                    <div className='flex items-center gap-1.5'>
                      <Button
                        variant='ghost'
                        size='sm'
                        className='h-6 text-xs px-2'
                        onClick={() => handleCopyText(mavenCoordText, 'coord')}
                      >
                        {copiedCoord ? (
                          <Check className='h-3 w-3 mr-1 text-green-600' />
                        ) : (
                          <Copy className='h-3 w-3 mr-1' />
                        )}
                        {t('plugin.copyCoordinates')}
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        className='h-6 text-xs px-2'
                        onClick={() => handleCopyText(mavenXmlText, 'xml')}
                      >
                        {copiedXml ? (
                          <Check className='h-3 w-3 mr-1 text-green-600' />
                        ) : (
                          <Copy className='h-3 w-3 mr-1' />
                        )}
                        {t('plugin.copyXml')}
                      </Button>
                    </div>
                  </div>
                  <div className='grid grid-cols-1 sm:grid-cols-3 gap-2 font-mono text-xs'>
                    <div className='bg-background/80 rounded px-2.5 py-1.5 border'>
                      <span className='text-muted-foreground'>groupId: </span>
                      <span className='font-medium'>{plugin.group_id}</span>
                    </div>
                    <div className='bg-background/80 rounded px-2.5 py-1.5 border'>
                      <span className='text-muted-foreground'>artifactId: </span>
                      <span className='font-medium'>{plugin.artifact_id}</span>
                    </div>
                    <div className='bg-background/80 rounded px-2.5 py-1.5 border'>
                      <span className='text-muted-foreground'>version: </span>
                      <span className='font-medium'>{plugin.version}</span>
                    </div>
                  </div>
                </div>

                {/* Official documentation & configuration samples / 官方配置文档与样例直达 */}
                <div className='rounded-lg border bg-muted/20 p-4 space-y-3'>
                  <div className='space-y-1'>
                    <h4 className='text-sm font-semibold flex items-center gap-2'>
                      <BookOpen className='h-4 w-4 text-primary' />
                      {t('plugin.officialDocGuide')}
                    </h4>
                    <p className='text-xs text-muted-foreground leading-relaxed'>
                      {t('plugin.officialDocGuideDesc')}
                    </p>
                  </div>

                  <div className='flex items-center gap-2.5 flex-wrap pt-1'>
                    {docUrls.sourceDocUrl && (
                      <Button
                        variant='outline'
                        size='sm'
                        className='h-8 text-xs font-medium'
                        asChild
                      >
                        <a
                          href={docUrls.sourceDocUrl}
                          target='_blank'
                          rel='noopener noreferrer'
                        >
                          <ExternalLink className='h-3.5 w-3.5 mr-1.5 text-primary' />
                          {t('plugin.sourceDoc')}
                        </a>
                      </Button>
                    )}
                    {docUrls.sinkDocUrl && (
                      <Button
                        variant='outline'
                        size='sm'
                        className='h-8 text-xs font-medium'
                        asChild
                      >
                        <a
                          href={docUrls.sinkDocUrl}
                          target='_blank'
                          rel='noopener noreferrer'
                        >
                          <ExternalLink className='h-3.5 w-3.5 mr-1.5 text-primary' />
                          {t('plugin.sinkDoc')}
                        </a>
                      </Button>
                    )}
                    <Button
                      variant='outline'
                      size='sm'
                      className='h-8 text-xs font-medium border-sky-300 text-sky-700 hover:bg-sky-50 dark:border-sky-800 dark:text-sky-300 dark:hover:bg-sky-950/50'
                      onClick={handleOpenAskAi}
                      data-testid='plugin-ask-ai-btn'
                    >
                      <Sparkles className='h-3.5 w-3.5 mr-1.5 text-sky-500 animate-pulse' />
                      {t('plugin.askAi')}
                    </Button>
                  </div>
                </div>

                {/* Installation paths explanation / 安装路径与依赖隔离说明 */}
                <div className='pt-2 border-t space-y-2'>
                  <h5 className='text-xs font-medium text-muted-foreground flex items-center gap-1.5'>
                    <FolderOpen className='h-3.5 w-3.5' />
                    {t('plugin.installPath')}
                  </h5>
                  <div className='text-xs space-y-1.5'>
                    <div className='flex items-center gap-2'>
                      <Badge variant='outline' className='text-[10px]'>
                        {t('plugin.connectorPath')}
                      </Badge>
                      <code className='text-xs bg-muted px-2 py-0.5 rounded font-mono'>
                        connectors/
                      </code>
                    </div>
                    <p className='text-xs text-muted-foreground mt-1'>
                      {useIsolatedDeps
                        ? t('plugin.installPathIsolatedDesc')
                        : t('plugin.installPathLegacyDesc')}
                    </p>
                  </div>
                </div>
              </div>
            )}

            {/* Section 2: Official Dependencies & Profiles / 第二部分：官方依赖画像与消除重复的统一表格 */}
            {shouldRenderDependencies && (
              <div
                className='space-y-5 rounded-lg border bg-card p-5 shadow-xs'
                data-testid='plugin-official-dependencies'
              >
                <div>
                  <h4 className='font-medium text-sm flex items-center gap-2'>
                    <Layers3 className='h-4 w-4 text-primary' />
                    {t('plugin.officialDependencies')}
                  </h4>
                  <p className='text-xs text-muted-foreground mt-1'>
                    {t('plugin.officialDependenciesDesc')}
                  </p>
                </div>

                {loadingOfficialDeps ? (
                  <div className='text-sm text-muted-foreground py-6 text-center'>
                    <RefreshCw className='h-4 w-4 animate-spin inline mr-2' />
                    {t('common.loading')}
                  </div>
                ) : !officialDeps ? (
                  <div className='text-sm text-muted-foreground py-4 text-center bg-muted/30 rounded-md'>
                    {t('plugin.officialDependenciesUnavailable')}
                  </div>
                ) : (
                  <div className='space-y-5'>
                    {/* Scenario profiles selection / 场景画像勾选器 */}
                    {hasMultiProfiles && (
                      <div className='space-y-2.5 rounded-lg border bg-muted/20 p-4'>
                        <div className='flex items-center justify-between gap-2 flex-wrap'>
                          <div className='text-sm font-medium'>
                            {t('plugin.profileSelection')}
                          </div>
                          <div className='flex items-center gap-2'>
                            <Button
                              variant='ghost'
                              size='sm'
                              className='h-6 text-xs px-2'
                              onClick={handleSelectAllProfiles}
                            >
                              {t('plugin.selectAllProfiles')}
                            </Button>
                            <Button
                              variant='ghost'
                              size='sm'
                              className='h-6 text-xs px-2 text-muted-foreground'
                              onClick={handleClearAllProfiles}
                            >
                              {t('plugin.clearAllProfiles')}
                            </Button>
                          </div>
                        </div>

                        {/* Search filter for profiles / 场景快速搜索过滤 */}
                        {availableProfiles.length > 5 && (
                          <div className='relative'>
                            <Search className='absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground' />
                            <Input
                              placeholder={t('plugin.profileFilterPlaceholder')}
                              value={profileFilter}
                              onChange={(e) => setProfileFilter(e.target.value)}
                              className='h-8 pl-8 text-xs bg-background'
                            />
                          </div>
                        )}

                        <div className='grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-[220px] overflow-y-auto pr-1'>
                          {filteredProfiles.length === 0 ? (
                            <div className='col-span-2 text-xs text-muted-foreground text-center py-3'>
                              {t('plugin.noMatchingProfiles')}
                            </div>
                          ) : (
                            filteredProfiles.map((profile) => {
                              const itemCount = (profile.items || []).length;
                              const isChecked = selectedProfileKeys.includes(
                                profile.profile_key,
                              );
                              return (
                                <label
                                  key={profile.id}
                                  data-testid={`plugin-profile-${profile.profile_key}`}
                                  className={`flex items-start gap-2.5 rounded-md border px-3 py-2 cursor-pointer transition-colors ${
                                    isChecked
                                      ? 'bg-primary/5 border-primary/40'
                                      : 'bg-background hover:bg-muted/40'
                                  }`}
                                >
                                  <Checkbox
                                    checked={isChecked}
                                    onCheckedChange={(checked) =>
                                      handleToggleProfile(
                                        profile.profile_key,
                                        checked === true,
                                      )
                                    }
                                    className='mt-0.5'
                                  />
                                  <div className='flex min-w-0 flex-1 flex-col'>
                                    <span className='text-xs font-medium'>
                                      {profile.profile_name ||
                                        profile.profile_key}
                                    </span>
                                    <span className='text-[11px] text-muted-foreground mt-0.5'>
                                      {itemCount > 0
                                        ? t('plugin.profileDependencyCount', {
                                            count: itemCount,
                                          })
                                        : t('plugin.noExtraDependencies')}
                                    </span>
                                  </div>
                                </label>
                              );
                            })
                          )}
                        </div>
                      </div>
                    )}

                    {/* Consolidated Active Official Dependencies Table / 消除重复的单张统一依赖表格 */}
                    {effectiveDependencies.length > 0 ? (
                      <div
                        className='space-y-3'
                        data-testid='plugin-active-official-dependencies'
                      >
                        <div className='flex items-center justify-between gap-2 flex-wrap'>
                          <div>
                            <h5 className='text-sm font-medium'>
                              {t('plugin.activeOfficialDependencies')} (
                              {effectiveDependencies.length})
                            </h5>
                            <p className='text-xs text-muted-foreground mt-0.5'>
                              {t('plugin.activeOfficialDependenciesDesc')}
                            </p>
                          </div>
                          {dependencyTargetDirs.length > 0 && (
                            <div className='flex flex-wrap justify-end gap-1.5'>
                              {dependencyTargetDirs.map((targetDir) => (
                                <Badge
                                  key={targetDir}
                                  variant='outline'
                                  className='max-w-[260px] whitespace-normal break-all text-[10px]'
                                >
                                  {targetDir}
                                </Badge>
                              ))}
                            </div>
                          )}
                        </div>

                        <div className='overflow-x-auto rounded-lg border'>
                          <Table className='min-w-[760px]'>
                            <TableHeader>
                              <TableRow>
                                <TableHead className='w-[110px]'>
                                  {t('plugin.dependencyType')}
                                </TableHead>
                                <TableHead>{t('plugin.groupId')}</TableHead>
                                <TableHead>{t('plugin.artifactId')}</TableHead>
                                <TableHead className='w-[100px]'>
                                  {t('plugin.version')}
                                </TableHead>
                                <TableHead>{t('plugin.targetDir')}</TableHead>
                                <TableHead className='w-[80px] text-right'>
                                  {t('common.actions')}
                                </TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {effectiveDependencies.map((dep) => {
                                const isConnector =
                                  dep.target_dir === 'connectors';
                                const actionKey = `${dep.group_id}:${dep.artifact_id}:${dep.version}:${dep.target_dir}:disable`;
                                const busy =
                                  operatingDependencyKey === actionKey;
                                return (
                                  <TableRow
                                    key={`${dep.group_id}:${dep.artifact_id}:${dep.version}:${dep.target_dir}`}
                                  >
                                    <TableCell>
                                      <Badge
                                        variant={
                                          isConnector ? 'secondary' : 'outline'
                                        }
                                        className='text-[10px] whitespace-nowrap'
                                      >
                                        {isConnector
                                          ? t('plugin.typeConnector')
                                          : t('plugin.typeDriver')}
                                      </Badge>
                                    </TableCell>
                                    <TableCell className='font-mono text-xs whitespace-nowrap'>
                                      {dep.group_id}
                                    </TableCell>
                                    <TableCell className='font-mono text-xs whitespace-nowrap font-medium'>
                                      {dep.artifact_id}
                                    </TableCell>
                                    <TableCell className='font-mono text-xs whitespace-nowrap'>
                                      {dep.version}
                                    </TableCell>
                                    <TableCell className='text-xs'>
                                      <code className='bg-muted px-1.5 py-0.5 rounded text-[11px] font-mono break-all'>
                                        {dep.target_dir}
                                      </code>
                                    </TableCell>
                                    <TableCell className='text-right'>
                                      <Button
                                        variant='ghost'
                                        size='sm'
                                        aria-label={`${t('plugin.disable')} ${dep.artifact_id}`}
                                        data-testid={buildDependencyActionId(
                                          'plugin-disable-official',
                                          dep.artifact_id,
                                        )}
                                        className='text-amber-600 hover:text-amber-700 h-7 w-7 p-0'
                                        disabled={busy}
                                        onClick={() =>
                                          handleDisableOfficialDependency(dep)
                                        }
                                      >
                                        {busy ? (
                                          <RefreshCw className='h-3.5 w-3.5 animate-spin' />
                                        ) : (
                                          <Ban className='h-3.5 w-3.5' />
                                        )}
                                      </Button>
                                    </TableCell>
                                  </TableRow>
                                );
                              })}
                            </TableBody>
                          </Table>
                        </div>
                      </div>
                    ) : officialDeps.dependency_status === 'not_required' ? (
                      <div className='text-sm text-muted-foreground py-4 text-center bg-muted/30 rounded-md'>
                        {t('plugin.officialDependenciesNotRequired')}
                      </div>
                    ) : hasMultiProfiles && selectedProfileKeys.length === 0 ? (
                      <div className='text-sm text-muted-foreground py-4 text-center bg-muted/30 rounded-md'>
                        {t('plugin.selectProfilesToPreview')}
                      </div>
                    ) : (
                      <div className='text-sm text-muted-foreground py-4 text-center bg-muted/30 rounded-md'>
                        {t('plugin.officialDependenciesUnknown')}
                      </div>
                    )}

                    {/* Disabled Official Dependencies / 已禁用的官方依赖列表 */}
                    {disabledDependencies.length > 0 && (
                      <div
                        className='space-y-3'
                        data-testid='plugin-disabled-official-dependencies'
                      >
                        <div>
                          <h5 className='text-sm font-medium text-muted-foreground'>
                            {t('plugin.disabledOfficialDependencies')} (
                            {disabledDependencies.length})
                          </h5>
                          <p className='mt-0.5 text-xs text-muted-foreground'>
                            {t('plugin.disabledOfficialDependenciesDesc')}
                          </p>
                        </div>
                        <div className='overflow-x-auto rounded-lg border border-dashed'>
                          <Table className='min-w-[760px]'>
                            <TableHeader>
                              <TableRow>
                                <TableHead>{t('plugin.groupId')}</TableHead>
                                <TableHead>{t('plugin.artifactId')}</TableHead>
                                <TableHead className='w-[100px]'>
                                  {t('plugin.version')}
                                </TableHead>
                                <TableHead>{t('plugin.targetDir')}</TableHead>
                                <TableHead className='w-[80px] text-right'>
                                  {t('common.actions')}
                                </TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {disabledDependencies.map((item) => {
                                const actionKey = `${item.id}:enable`;
                                const busy =
                                  operatingDependencyKey === actionKey;
                                return (
                                  <TableRow key={item.id}>
                                    <TableCell className='font-mono text-xs text-muted-foreground whitespace-nowrap'>
                                      {item.group_id}
                                    </TableCell>
                                    <TableCell className='font-mono text-xs text-muted-foreground whitespace-nowrap line-through'>
                                      {item.artifact_id}
                                    </TableCell>
                                    <TableCell className='font-mono text-xs text-muted-foreground whitespace-nowrap'>
                                      {item.version}
                                    </TableCell>
                                    <TableCell className='text-xs'>
                                      <code className='bg-muted px-1.5 py-0.5 rounded text-[11px] font-mono break-all opacity-70'>
                                        {item.target_dir}
                                      </code>
                                    </TableCell>
                                    <TableCell className='text-right'>
                                      <Button
                                        variant='ghost'
                                        size='sm'
                                        aria-label={`${t('plugin.enable')} ${item.artifact_id}`}
                                        data-testid={buildDependencyActionId(
                                          'plugin-enable-official',
                                          item.artifact_id,
                                        )}
                                        className='text-emerald-600 hover:text-emerald-700 h-7 w-7 p-0'
                                        disabled={busy}
                                        onClick={() =>
                                          handleEnableOfficialDependency(item)
                                        }
                                      >
                                        {busy ? (
                                          <RefreshCw className='h-3.5 w-3.5 animate-spin' />
                                        ) : (
                                          <RotateCcw className='h-3.5 w-3.5' />
                                        )}
                                      </Button>
                                    </TableCell>
                                  </TableRow>
                                );
                              })}
                            </TableBody>
                          </Table>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}

            {/* Section 3: Custom Dependencies / 第三部分：自定义补丁与驱动扩展 */}
            {shouldRenderCustom && (
              <div className='rounded-lg border bg-card p-5 shadow-xs'>
                <PluginDependencyConfigSection
                  pluginName={plugin.name}
                  seatunnelVersion={effectiveVersion}
                  compact
                  onChanged={loadOfficialDeps}
                />
              </div>
            )}
          </div>
        </ScrollArea>
        </div>

        {/* Footer actions / 底部操作栏 */}
        {onDownload && (
          <div className='px-6 py-3.5 border-t bg-muted/10 shrink-0 flex items-center justify-between gap-3'>
            <div className='text-xs text-muted-foreground'>
              {downloadDisabled && t('plugin.selectScenarioFirst')}
            </div>
            <Button
              onClick={() => onDownload(plugin, selectedProfileKeys)}
              disabled={downloadDisabled}
              className='gap-2'
            >
              <Download className='h-4 w-4' />
              {t('plugin.download')}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
