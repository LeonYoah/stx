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

/**
 * Cluster Configs Component
 * 集群配置组件
 */

import {useState, useEffect, useCallback, useMemo, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Badge} from '@/components/ui/badge';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select';
import {AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle} from '@/components/ui/alert-dialog';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {Textarea} from '@/components/ui/textarea';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {toast} from 'sonner';
import {Settings, RefreshCw, FileText, Server, Check, AlertTriangle, Edit, History, Upload, Download, Loader2, FolderSync, Eye, GitCompareArrows, WandSparkles, CircleHelp, Search, MoreHorizontal} from 'lucide-react';
import {ConfigService} from '@/lib/services/config';
import type {ConfigInfo, ConfigVersionInfo} from '@/lib/services/config';
import {ConfigType, ConfigTypeNames, getConfigTypesForMode} from '@/lib/services/config';
import services from '@/lib/services';
import type {NodeInfo} from '@/lib/services/cluster/types';
import {EmptyState, StatPillsBar, TableLoadingBar, TableSkeletonRows} from '@/components/common/layout';
import {Table, TableBody, TableCell, TableHead, TableHeader, TableRow} from '@/components/ui/table';
import {DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger} from '@/components/ui/dropdown-menu';
import {Skeleton} from '@/components/ui/skeleton';
import {cn} from '@/lib/utils';

interface ClusterConfigsProps {
  clusterId: number;
  deploymentMode: string;
  onConfigChanged?: () => void;
}

interface DiffRow {
  leftLineNumber: number | null;
  leftText: string;
  rightLineNumber: number | null;
  rightText: string;
  changed: boolean;
}

function buildDiffRows(left: string, right: string): DiffRow[] {
  const leftLines = left.split('\n');
  const rightLines = right.split('\n');
  const maxLen = Math.max(leftLines.length, rightLines.length);

  return Array.from({length: maxLen}, (_, index) => {
    const leftText = leftLines[index] ?? '';
    const rightText = rightLines[index] ?? '';
    return {
      leftLineNumber: index < leftLines.length ? index + 1 : null,
      leftText,
      rightLineNumber: index < rightLines.length ? index + 1 : null,
      rightText,
      changed: leftText !== rightText,
    };
  });
}

function isYamlConfigType(configType?: ConfigType | null): boolean {
  switch (configType) {
    case ConfigType.SEATUNNEL:
    case ConfigType.HAZELCAST:
    case ConfigType.HAZELCAST_CLIENT:
    case ConfigType.HAZELCAST_MASTER:
    case ConfigType.HAZELCAST_WORKER:
      return true;
    default:
      return false;
  }
}

function extractSeatunnelHTTPPort(content: string): number | null {
  const lines = content.split(/\r?\n/);
  let seatunnelIndent: number | null = null;
  let engineIndent: number | null = null;
  let httpIndent: number | null = null;

  for (const rawLine of lines) {
    const trimmed = rawLine.trim();
    if (!trimmed || trimmed.startsWith('#')) {
      continue;
    }
    const indent = rawLine.length - rawLine.trimStart().length;
    if (!trimmed.includes(':')) {
      continue;
    }
    const key = trimmed.slice(0, trimmed.indexOf(':')).trim();
    const value = trimmed.slice(trimmed.indexOf(':') + 1).trim();

    if (seatunnelIndent !== null && indent <= seatunnelIndent) {
      seatunnelIndent = null;
      engineIndent = null;
      httpIndent = null;
    }
    if (engineIndent !== null && indent <= engineIndent) {
      engineIndent = null;
      httpIndent = null;
    }
    if (httpIndent !== null && indent <= httpIndent) {
      httpIndent = null;
    }

    if (key === 'seatunnel') {
      seatunnelIndent = indent;
      engineIndent = null;
      httpIndent = null;
      continue;
    }
    if (seatunnelIndent !== null && key === 'engine' && indent > seatunnelIndent) {
      engineIndent = indent;
      httpIndent = null;
      continue;
    }
    if (engineIndent !== null && key === 'http' && indent > engineIndent) {
      httpIndent = indent;
      continue;
    }
    if (httpIndent !== null && key === 'port' && indent > httpIndent) {
      const parsed = Number.parseInt(value.replace(/^['"]|['"]$/g, ''), 10);
      return Number.isFinite(parsed) ? parsed : null;
    }
  }

  return null;
}

function shouldWarnSeatunnelPortSync(config: ConfigInfo | null, nextContent: string, syncAfterSave: boolean): boolean {
  if (!config || config.config_type !== ConfigType.SEATUNNEL) {
    return false;
  }
  if (config.is_template && !syncAfterSave) {
    return false;
  }
  const currentPort = extractSeatunnelHTTPPort(config.content);
  const nextPort = extractSeatunnelHTTPPort(nextContent);
  return nextPort !== null && currentPort !== nextPort;
}

export function ClusterConfigs({clusterId, deploymentMode, onConfigChanged}: ClusterConfigsProps) {
  const t = useTranslations();
  const [configs, setConfigs] = useState<ConfigInfo[]>([]);
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [loading, setLoading] = useState(true);
  
  // Get available config types based on deployment mode
  const availableConfigTypes = getConfigTypesForMode(deploymentMode);
  const [selectedConfigType, setSelectedConfigType] = useState<ConfigType>(availableConfigTypes[0]);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editingConfig, setEditingConfig] = useState<ConfigInfo | null>(null);
  const [editContent, setEditContent] = useState('');
  const [editComment, setEditComment] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveMode, setSaveMode] = useState<'save' | 'save-and-sync'>('save');
  const [portChangeConfirmOpen, setPortChangeConfirmOpen] = useState(false);
  const [pendingSaveSync, setPendingSaveSync] = useState(false);
  const [repairing, setRepairing] = useState(false);
  const [versionDialogOpen, setVersionDialogOpen] = useState(false);
  const [versionConfig, setVersionConfig] = useState<ConfigInfo | null>(null);
  const [versions, setVersions] = useState<ConfigVersionInfo[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [selectedVersionId, setSelectedVersionId] = useState<number | null>(null);
  const [versionViewMode, setVersionViewMode] = useState<'preview' | 'compare'>('preview');
  
  // Init config dialog state
  const [initDialogOpen, setInitDialogOpen] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string>('');
  const [initLoading, setInitLoading] = useState(false);
  const [syncAllLoading, setSyncAllLoading] = useState(false);
  const [logModeSwitching, setLogModeSwitching] = useState(false);
  // 是否已完成首次加载，避免刷新时整块替换
  // Whether the first load finished, so refresh does not replace the whole panel
  const [hasLoaded, setHasLoaded] = useState(false);
  // 节点配置筛选与搜索 / Node config filter and search
  const [nodeFilter, setNodeFilter] = useState<'all' | 'matched' | 'mismatched'>('all');
  const [nodeSearch, setNodeSearch] = useState('');
  const editTextareaRef = useRef<HTMLTextAreaElement | null>(null);

  const selectedVersion = useMemo(
    () => versions.find((version) => version.id === selectedVersionId) || null,
    [versions, selectedVersionId]
  );
  const diffRows = useMemo(
    () => buildDiffRows(versionConfig?.content || '', selectedVersion?.content || ''),
    [versionConfig?.content, selectedVersion?.content]
  );

  // 用 ref 持有回调，避免父组件内联函数导致 loadData 身份抖动、配置 Tab 死循环请求
  // Hold callback in a ref so parent inline lambdas cannot churn loadData and loop-fetch the configs tab
  const onConfigChangedRef = useRef(onConfigChanged);
  onConfigChangedRef.current = onConfigChanged;

  /** 配置真正变更后，通知存储可视化反向刷新 / After real config mutations, refresh storage views */
  const notifyConfigChanged = useCallback(() => {
    onConfigChangedRef.current?.();
  }, []);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [configsResult, nodesResult] = await Promise.all([
        ConfigService.getClusterConfigs(clusterId),
        services.cluster.getNodesSafe(clusterId),
      ]);
      setConfigs(configsResult || []);
      if (nodesResult.success && nodesResult.data) {
        setNodes(nodesResult.data);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.loadError'));
    } finally {
      setHasLoaded(true);
      setLoading(false);
    }
  }, [clusterId, t]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const templateConfig = configs.find(c => c.config_type === selectedConfigType && c.is_template);
  const nodeConfigs = configs.filter(c => c.config_type === selectedConfigType && !c.is_template);
  const isLog4j2 = selectedConfigType === ConfigType.LOG4J2;
  const isPerJobLogMode = Boolean(
    templateConfig?.content &&
      /rootLogger\.appenderRef\.file\.ref\s*=\s*routingAppender/.test(templateConfig.content)
  );

  const handleSwitchLogMode = async (targetMode: 'per_job' | 'mixed') => {
    setLogModeSwitching(true);
    try {
      const res = await services.cluster.switchJobLogModeSafe(clusterId, targetMode);
      if (!res.success) {
        toast.error(res.error || t('config.switchLogModeError'));
        return;
      }
      toast.success(
        targetMode === 'per_job'
          ? t('config.switchToPerJobSuccess')
          : t('config.switchToMixedSuccess')
      );
      await loadData();
      onConfigChanged?.();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.switchLogModeError'));
    } finally {
      setLogModeSwitching(false);
    }
  };

  const mismatchedNodeCount = nodeConfigs.filter((config) => !config.match_template).length;
  const matchedNodeCount = nodeConfigs.length - mismatchedNodeCount;
  const keyword = nodeSearch.trim().toLowerCase();
  // 按对齐状态与主机关键字过滤节点配置 / Filter node configs by alignment and host keyword
  const filteredNodeConfigs = nodeConfigs.filter((config) => {
    const matchesFilter =
      nodeFilter === 'all' ||
      (nodeFilter === 'matched' && config.match_template) ||
      (nodeFilter === 'mismatched' && !config.match_template);
    if (!matchesFilter) {
      return false;
    }
    if (!keyword) {
      return true;
    }
    const haystack = `${config.host_name || ''} ${config.host_ip || ''}`.toLowerCase();
    return haystack.includes(keyword);
  });

  const handleEdit = (config: ConfigInfo) => {
    setEditingConfig(config);
    setEditContent(config.content);
    setEditComment('');
    setEditDialogOpen(true);
  };

  const beginSave = async (syncAfterSave = false) => {
    if (!editingConfig) {return;}
    setSaveMode(syncAfterSave ? 'save-and-sync' : 'save');
    setSaving(true);
    try {
      const result = await ConfigService.updateConfig(editingConfig.id, {content: editContent, comment: editComment || undefined});
      if (syncAfterSave && editingConfig.is_template) {
        const syncResult = await ConfigService.syncTemplateToAllNodes(clusterId, editingConfig.config_type);
        if (syncResult.synced_count > 0) {
          if (syncResult.push_errors && syncResult.push_errors.length > 0) {
            const errorNodes = syncResult.push_errors.map((e) => e.host_ip || `Host ${e.host_id}`).join(', ');
            toast.warning(t('config.saveAndSyncSuccessWithPushErrors', {count: syncResult.synced_count, nodes: errorNodes}));
          } else {
            toast.success(t('config.saveAndSyncSuccess', {count: syncResult.synced_count}));
          }
        } else {
          toast.success(t('config.saveAndSyncSuccessNoChanges'));
        }
      } else if (result.push_error) {
        toast.warning(t('config.saveSuccessWithPushError', {error: result.push_error}));
      } else {
        toast.success(t('config.saveSuccess'));
      }
      setEditDialogOpen(false);
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.saveFailed'));
    } finally {
      setSaving(false);
      setSaveMode('save');
      setPendingSaveSync(false);
    }
  };

  const handleSave = async (syncAfterSave = false) => {
    if (shouldWarnSeatunnelPortSync(editingConfig, editContent, syncAfterSave)) {
      setPendingSaveSync(syncAfterSave);
      setPortChangeConfirmOpen(true);
      return;
    }
    await beginSave(syncAfterSave);
  };

  const handleSmartRepair = async () => {
    if (!editingConfig || !isYamlConfigType(editingConfig.config_type)) {
      return;
    }
    setRepairing(true);
    try {
      const normalized = await ConfigService.normalizeConfig({
        config_type: editingConfig.config_type,
        content: editContent,
      });
      setEditContent(normalized);
      toast.success(t('config.smartRepairSuccess'));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.smartRepairFailed'));
    } finally {
      setRepairing(false);
    }
  };

  const handleViewVersions = async (config: ConfigInfo) => {
    setVersionConfig(config);
    setVersionDialogOpen(true);
    setLoadingVersions(true);
    setVersionViewMode('preview');
    try {
      const result = await ConfigService.getConfigVersions(config.id);
      const versionList = result || [];
      setVersions(versionList);
      setSelectedVersionId(versionList[0]?.id ?? null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.loadVersionsFailed'));
    } finally {
      setLoadingVersions(false);
    }
  };

  const handleRollback = async (version: number) => {
    if (!versionConfig) {return;}
    try {
      const result = await ConfigService.rollbackConfig(versionConfig.id, {version, comment: `Rollback to v${version}`});
      if (result.push_error) {
        toast.warning(t('config.rollbackSuccessWithPushError', {error: result.push_error}));
      } else {
        toast.success(t('config.rollbackSuccess'));
      }
      setVersionDialogOpen(false);
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.rollbackFailed'));
    }
  };

  const handlePreviewVersion = (versionId: number) => {
    setSelectedVersionId(versionId);
    setVersionViewMode('preview');
  };

  const handleCompareVersion = (versionId: number) => {
    setSelectedVersionId(versionId);
    setVersionViewMode('compare');
  };

  const handlePromote = async (config: ConfigInfo) => {
    try {
      await ConfigService.promoteConfig(config.id, {comment: t('config.promoteComment')});
      toast.success(t('config.promoteSuccess'));
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.promoteFailed'));
    }
  };

  const handleSyncFromTemplate = async (config: ConfigInfo) => {
    try {
      const result = await ConfigService.syncFromTemplate(config.id, {comment: t('config.syncComment')});
      if (result.push_error) {
        toast.warning(t('config.syncSuccessWithPushError', {error: result.push_error}));
      } else {
        toast.success(t('config.syncSuccess'));
      }
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.syncFailed'));
    }
  };

  const handleOpenInitDialog = () => {
    if (nodes.length > 0) {
      setSelectedNodeId(String(nodes[0].id));
    }
    setInitDialogOpen(true);
  };

  const handleInitConfigs = async () => {
    if (!selectedNodeId) {
      toast.error(t('config.selectNodeFirst'));
      return;
    }
    const node = nodes.find(n => n.id === Number(selectedNodeId));
    if (!node) {
      toast.error(t('config.nodeNotFound'));
      return;
    }
    setInitLoading(true);
    try {
      await ConfigService.initClusterConfigs(clusterId, node.host_id, node.install_dir);
      toast.success(t('config.initSuccess'));
      setInitDialogOpen(false);
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.initFailed'));
    } finally {
      setInitLoading(false);
    }
  };

  const handleSyncToAllNodes = async () => {
    if (!templateConfig) {return;}
    setSyncAllLoading(true);
    try {
      const result = await ConfigService.syncTemplateToAllNodes(clusterId, selectedConfigType);
      if (result.synced_count > 0) {
        if (result.push_errors && result.push_errors.length > 0) {
          const errorNodes = result.push_errors.map(e => e.host_ip || `Host ${e.host_id}`).join(', ');
          toast.warning(t('config.syncAllSuccessWithPushErrors', {count: result.synced_count, nodes: errorNodes}));
        } else {
          toast.success(t('config.syncAllSuccess', {count: result.synced_count}));
        }
      } else {
        toast.info(t('config.syncAllNoChanges'));
      }
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('config.syncAllFailed'));
    } finally {
      setSyncAllLoading(false);
    }
  };

  return (
    <Card data-testid="cluster-configs-root" className="border rounded-xl relative overflow-hidden bg-card/40 shadow-xs flex flex-col flex-1 min-h-[480px]">
      <TableLoadingBar loading={loading && hasLoaded} />
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle className="flex items-center gap-2">
          <Settings className="h-5 w-5" />
          {t('config.title')}
        </CardTitle>
        <div className="flex items-center gap-2">
          <Select value={selectedConfigType} onValueChange={(v) => {
            setSelectedConfigType(v as ConfigType);
            setNodeFilter('all');
            setNodeSearch('');
          }}>
            <SelectTrigger className="w-[200px]" data-testid="cluster-configs-type-select"><SelectValue /></SelectTrigger>
            <SelectContent>
              {availableConfigTypes.map((type) => (
                <SelectItem key={type} value={type}>{ConfigTypeNames[type]}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button variant="outline" size="sm" onClick={handleOpenInitDialog} disabled={loading || nodes.length === 0} data-testid="cluster-configs-init-button" title={t('config.tipInit')}>
            <FolderSync className="h-4 w-4 mr-1" />
            {t('config.initFromNode')}
          </Button>
          <Button variant="outline" size="icon" onClick={loadData} disabled={loading}>
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {loading && !hasLoaded ? (
          <div className="space-y-3 py-2">
            <Skeleton className="h-8 w-64" />
            <Table>
              <TableBody>
                <TableSkeletonRows columns={5} rows={4} />
              </TableBody>
            </Table>
          </div>
        ) : configs.length === 0 ? (
          <EmptyState icon={FileText} title={t('config.noConfigs')} description={t('config.initHint')}>
            {nodes.length > 0 && (
              <Button variant="outline" onClick={handleOpenInitDialog}>
                <FolderSync className="h-4 w-4 mr-2" />
                {t('config.initFromNode')}
              </Button>
            )}
          </EmptyState>
        ) : (
          <div className={loading ? 'opacity-60 pointer-events-none transition-opacity duration-200' : ''}>
          <Tabs defaultValue="template" className="w-full">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="template" className="flex items-center gap-2" data-testid="cluster-configs-tab-template">
                <FileText className="h-4 w-4" />{t('config.clusterTemplate')}
              </TabsTrigger>
              <TabsTrigger value="nodes" className="flex items-center gap-2" data-testid="cluster-configs-tab-nodes">
                <Server className="h-4 w-4" />{t('config.nodeConfigs')}
                {nodeConfigs.length > 0 && <Badge variant="secondary" className="ml-1">{nodeConfigs.length}</Badge>}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="template" className="mt-4">
              {templateConfig && mismatchedNodeCount > 0 && (
                <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/20 dark:text-amber-200" data-testid="cluster-configs-pending-sync">
                  <span className="font-medium">{t('config.pendingSyncTitle', {count: mismatchedNodeCount})}</span>
                  <Button variant="outline" size="sm" onClick={handleSyncToAllNodes} disabled={syncAllLoading} data-testid="cluster-configs-template-sync-all-banner">
                    {syncAllLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Download className="mr-2 h-4 w-4" />}
                    {t('config.syncToAllNodes')}
                  </Button>
                </div>
              )}
              {templateConfig ? (
                <div className="space-y-3">
                  {isLog4j2 && (
                    <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 rounded-lg border border-primary/20 bg-primary/5 p-3.5 text-xs">
                      <div className="space-y-1">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-foreground">{t('config.jobLogModeTitle')}:</span>
                          <Badge
                            variant="outline"
                            className={cn(
                              'px-2 py-0.5 font-medium',
                              isPerJobLogMode
                                ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400'
                                : 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400'
                            )}
                          >
                            {isPerJobLogMode
                              ? t('config.jobLogModePerJobBadge')
                              : t('config.jobLogModeMixedBadge')}
                          </Badge>
                        </div>
                        <p className="text-muted-foreground">
                          {isPerJobLogMode
                            ? t('config.jobLogModePerJobDesc')
                            : t('config.jobLogModeMixedDesc')}
                        </p>
                      </div>
                      <Button
                        variant={isPerJobLogMode ? 'outline' : 'default'}
                        size="sm"
                        className="shrink-0"
                        disabled={logModeSwitching}
                        onClick={() => handleSwitchLogMode(isPerJobLogMode ? 'mixed' : 'per_job')}
                      >
                        {logModeSwitching ? (
                          <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                        ) : isPerJobLogMode ? (
                          <Settings className="mr-1.5 h-3.5 w-3.5" />
                        ) : (
                          <WandSparkles className="mr-1.5 h-3.5 w-3.5" />
                        )}
                        {isPerJobLogMode
                          ? t('config.switchToMixedModeBtn')
                          : t('config.switchToPerJobModeBtn')}
                      </Button>
                    </div>
                  )}
                  <p className="text-xs text-muted-foreground">{t('config.tabTemplateHint')}</p>
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Badge variant="outline">{ConfigTypeNames[selectedConfigType]}</Badge>
                      <Badge variant="secondary">v{templateConfig.version}</Badge>
                      <span className="text-sm text-muted-foreground">{new Date(templateConfig.updated_at).toLocaleString()}</span>
                    </div>
                    <div className="flex gap-1">
                      <Button variant="outline" size="sm" title={t('config.tipEdit')} onClick={() => handleEdit(templateConfig)} data-testid="cluster-configs-template-edit">
                        <Edit className="h-4 w-4 mr-1" />{t('common.edit')}
                      </Button>
                      <Button variant="outline" size="sm" title={t('config.tipVersions')} onClick={() => handleViewVersions(templateConfig)} data-testid="cluster-configs-template-versions">
                        <History className="h-4 w-4 mr-1" />{t('config.versions')}
                      </Button>
                      <Button variant="outline" size="sm" title={t('config.tipSyncAll')} onClick={handleSyncToAllNodes} disabled={syncAllLoading || nodeConfigs.length === 0} data-testid="cluster-configs-template-sync-all">
                        {syncAllLoading ? <Loader2 className="h-4 w-4 mr-1 animate-spin" /> : <Download className="h-4 w-4 mr-1" />}
                        {t('config.syncToAllNodes')}
                      </Button>
                    </div>
                  </div>
                  <pre className="min-h-[420px] max-h-[70vh] overflow-auto rounded-md border bg-muted/40 p-4 text-xs font-mono whitespace-pre-wrap break-all" data-testid="cluster-configs-template-content">{templateConfig.content}</pre>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground py-4 text-center">{t('config.noTemplate')}</p>
              )}
            </TabsContent>
            <TabsContent value="nodes" className="mt-4 space-y-3">
              <p className="text-xs text-muted-foreground">{t('config.tabNodeHint')}</p>
              <StatPillsBar
                activeKey={nodeFilter}
                onChange={(key) => setNodeFilter(key as 'all' | 'matched' | 'mismatched')}
                items={[
                  {key: 'all', label: t('config.filterAll'), count: nodeConfigs.length},
                  {key: 'matched', label: t('config.filterMatched'), count: matchedNodeCount, variant: 'success'},
                  {key: 'mismatched', label: t('config.filterMismatched'), count: mismatchedNodeCount, variant: mismatchedNodeCount > 0 ? 'warning' : 'default'},
                ]}
              />
              <div className="relative max-w-sm">
                <Search className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={nodeSearch}
                  onChange={(event) => setNodeSearch(event.target.value)}
                  className="pl-9"
                  placeholder={t('config.searchNodes')}
                  data-testid="cluster-configs-node-search"
                />
              </div>
              {nodeConfigs.length === 0 ? (
                <p className="text-sm text-muted-foreground py-4 text-center">{t('config.noNodeConfigs')}</p>
              ) : (
                <div className="overflow-hidden rounded-lg border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('config.selectNode')}</TableHead>
                        <TableHead>{t('config.version')}</TableHead>
                        <TableHead>{t('config.updatedAt')}</TableHead>
                        <TableHead className="text-right">{t('common.actions')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredNodeConfigs.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={4} className="py-8 text-center text-xs text-muted-foreground">
                            {t('config.noMatchingNodes')}
                          </TableCell>
                        </TableRow>
                      ) : filteredNodeConfigs.map((config) => (
                        <TableRow key={config.id}>
                          <TableCell>
                            <div className="flex min-w-0 items-center gap-2">
                              <span className="truncate font-medium">{config.host_name || `Node ${config.host_id}`}</span>
                              {config.host_ip && <span className="truncate font-mono text-xs text-muted-foreground">{config.host_ip}</span>}
                              {config.match_template ? (
                                <Badge variant="outline" className="text-green-600"><Check className="h-3 w-3 mr-1" />{t('config.matchTemplate')}</Badge>
                              ) : (
                                <Badge variant="outline" className="text-yellow-600"><AlertTriangle className="h-3 w-3 mr-1" />{t('config.customized')}</Badge>
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="font-mono text-xs">v{config.version}</TableCell>
                          <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{new Date(config.updated_at).toLocaleString()}</TableCell>
                          <TableCell className="text-right">
                            <div className="flex items-center justify-end gap-1">
                              {!config.match_template && (
                                <>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    className="h-8"
                                    title={t('config.tipSyncNode')}
                                    onClick={() => handleSyncFromTemplate(config)}
                                    data-testid={`cluster-configs-node-sync-${config.id}`}
                                  >
                                    <Download className="mr-1 size-3.5" />
                                    {t('config.syncFromTemplate')}
                                  </Button>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    className="h-8"
                                    title={t('config.tipPromote')}
                                    onClick={() => handlePromote(config)}
                                    data-testid={`cluster-configs-node-promote-${config.id}`}
                                  >
                                    <Upload className="mr-1 size-3.5" />
                                    {t('config.promoteToCluster')}
                                  </Button>
                                </>
                              )}
                              <DropdownMenu>
                                <DropdownMenuTrigger asChild>
                                  <Button variant="ghost" size="icon" className="size-8" aria-label={t('common.actions')}>
                                    <MoreHorizontal className="size-4" />
                                  </Button>
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end">
                                  <DropdownMenuItem title={t('config.tipEdit')} onClick={() => handleEdit(config)} data-testid={`cluster-configs-node-edit-${config.id}`}>
                                    <Edit className="mr-2 size-4" />
                                    {t('common.edit')}
                                  </DropdownMenuItem>
                                  <DropdownMenuItem title={t('config.tipVersions')} onClick={() => handleViewVersions(config)} data-testid={`cluster-configs-node-versions-${config.id}`}>
                                    <History className="mr-2 size-4" />
                                    {t('config.versions')}
                                  </DropdownMenuItem>
                                </DropdownMenuContent>
                              </DropdownMenu>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </TabsContent>
          </Tabs>
          </div>
        )}
      </CardContent>

      {/* Init Config Dialog */}
      <Dialog open={initDialogOpen} onOpenChange={setInitDialogOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t('config.initFromNode')}</DialogTitle>
            <DialogDescription>{t('config.initFromNodeDesc')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <Label>{t('config.selectNode')}</Label>
              <ScrollArea className="mt-2 max-h-64 rounded-md border">
                <div className="space-y-2 p-2">
                  {nodes.map((node) => {
                    const selected = selectedNodeId === String(node.id);
                    return (
                      <button
                        key={node.id}
                        type="button"
                        onClick={() => setSelectedNodeId(String(node.id))}
                        data-testid={`cluster-configs-init-node-${node.id}`}
                        className={`w-full rounded-md border p-3 text-left transition-colors ${
                          selected ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted/50'
                        }`}
                      >
                        <div className="flex items-center justify-between gap-3">
                          <div className="min-w-0">
                            <div className="truncate font-medium">
                              {node.host_name || `Node ${node.id}`} ({node.host_ip})
                            </div>
                            <div className="mt-1 truncate text-sm text-muted-foreground">
                              {node.install_dir}
                            </div>
                          </div>
                          <Badge variant={selected ? 'default' : 'outline'}>
                            {node.role}
                          </Badge>
                        </div>
                      </button>
                    );
                  })}
                </div>
              </ScrollArea>
            </div>
            {selectedNodeId && (
              <div className="text-sm text-muted-foreground bg-muted p-3 rounded-md">
                <p>{t('config.initWillPull')}</p>
                <ul className="list-disc list-inside mt-2 space-y-1">
                  {availableConfigTypes.map((type) => (
                    <li key={type}>{ConfigTypeNames[type]}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setInitDialogOpen(false)}>{t('common.cancel')}</Button>
            <Button onClick={handleInitConfigs} disabled={initLoading || !selectedNodeId} data-testid="cluster-configs-init-confirm">
              {initLoading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              {t('config.initConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
        <DialogContent
          className="!max-w-5xl max-h-[90vh]"
          data-testid="cluster-configs-edit-dialog"
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            window.requestAnimationFrame(() => {
              editTextareaRef.current?.focus();
            });
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('config.editConfig')} - {editingConfig?.is_template ? t('config.clusterTemplate') : editingConfig?.host_name}</DialogTitle>
            <DialogDescription>{ConfigTypeNames[editingConfig?.config_type as ConfigType]}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <div className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <Label>{t('config.content')}</Label>
                  {isYamlConfigType(editingConfig?.config_type) && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button type="button" className="text-muted-foreground transition-colors hover:text-foreground">
                          <CircleHelp className="h-4 w-4" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent className="max-w-xs">
                        {t('config.smartRepairHint')}
                      </TooltipContent>
                    </Tooltip>
                  )}
                </div>
                {isYamlConfigType(editingConfig?.config_type) && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={handleSmartRepair}
                    disabled={repairing || saving}
                    data-testid="cluster-configs-smart-repair"
                  >
                    {repairing ? (
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    ) : (
                      <WandSparkles className="mr-2 h-4 w-4" />
                    )}
                    {t('config.smartRepair')}
                  </Button>
                )}
              </div>
              <Textarea
                ref={editTextareaRef}
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                className="font-mono text-sm h-[500px]"
                data-testid="cluster-configs-edit-content"
              />
              {isYamlConfigType(editingConfig?.config_type) && (
                <p className="mt-2 text-xs text-muted-foreground">
                  {t('config.yamlValidationHint')}
                </p>
              )}
            </div>
            <div>
              <Label>{t('config.comment')}</Label>
              <Input value={editComment} onChange={(e) => setEditComment(e.target.value)} placeholder={t('config.commentPlaceholder')} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditDialogOpen(false)}>{t('common.cancel')}</Button>
            {editingConfig?.is_template && (
              <Button variant="outline" onClick={() => handleSave(false)} disabled={saving} data-testid="cluster-configs-save-only">
                {saving && saveMode === 'save' && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('config.saveOnly')}
              </Button>
            )}
            <Button onClick={() => handleSave(editingConfig?.is_template)} disabled={saving} data-testid="cluster-configs-save-primary">
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {editingConfig?.is_template ? t('config.saveAndSync') : t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={portChangeConfirmOpen}
        onOpenChange={(open) => {
          setPortChangeConfirmOpen(open);
          if (!open) {
            setPendingSaveSync(false);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('config.portChangeConfirmTitle')}</AlertDialogTitle>
            <AlertDialogDescription className="space-y-2">
              <p>{t('config.portChangeConfirmDesc')}</p>
              <ul className="list-disc pl-5">
                <li>{t('config.portChangeConfirmSyncConfig')}</li>
                <li>{t('config.portChangeConfirmSyncMetadata')}</li>
                <li>{t('config.portChangeConfirmImpact')}</li>
              </ul>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void beginSave(pendingSaveSync)}>
              {t('common.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Version History Dialog */}
      <Dialog open={versionDialogOpen} onOpenChange={setVersionDialogOpen}>
        <DialogContent className="w-[96vw] sm:max-w-7xl max-h-[90vh]" data-testid="cluster-configs-versions-dialog">
          <DialogHeader>
            <DialogTitle>{t('config.versionHistory')}</DialogTitle>
            <DialogDescription>{versionConfig?.is_template ? t('config.clusterTemplate') : versionConfig?.host_name}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
            {loadingVersions ? (
              <div className="flex items-center justify-center py-8"><Loader2 className="h-6 w-6 animate-spin" /></div>
            ) : versions.length === 0 ? (
              <p className="text-center py-8 text-muted-foreground">{t('config.noVersions')}</p>
            ) : (
              <>
                <ScrollArea className="h-[65vh] rounded-md border">
                  <div className="space-y-3 p-3">
                    {versions.map((version) => {
                      const selected = version.id === selectedVersionId;
                      const isCurrent = version.version === versionConfig?.version;
                      return (
                        <div key={version.id} className={`rounded-lg border p-3 ${selected ? 'border-primary bg-primary/5' : ''}`}>
                          <div className="flex items-center gap-2">
                            <Badge variant={selected ? 'default' : 'secondary'}>v{version.version}</Badge>
                            {isCurrent && <Badge variant="outline">{t('config.current')}</Badge>}
                          </div>
                          <p className="mt-2 text-sm text-muted-foreground">
                            {new Date(version.created_at).toLocaleString()}
                          </p>
                          <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">
                            {version.comment || t('config.noComment')}
                          </p>
                          <div className="mt-3 flex flex-wrap gap-2">
                            <Button variant="outline" size="sm" onClick={() => handlePreviewVersion(version.id)} data-testid={`cluster-configs-version-preview-${version.id}`}>
                              <Eye className="h-4 w-4 mr-1" />
                              {t('config.preview')}
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => handleCompareVersion(version.id)} disabled={isCurrent} data-testid={`cluster-configs-version-compare-${version.id}`}>
                              <GitCompareArrows className="h-4 w-4 mr-1" />
                              {t('config.compareWithCurrent')}
                            </Button>
                            {!isCurrent && (
                              <Button variant="outline" size="sm" onClick={() => handleRollback(version.version)} data-testid={`cluster-configs-version-rollback-${version.id}`}>
                                {t('config.rollback')}
                              </Button>
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </ScrollArea>
                <div className="min-w-0 rounded-md border">
                  {selectedVersion ? (
                    <div className="flex h-[65vh] flex-col">
                      <div className="border-b px-4 py-3">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge variant="secondary">v{selectedVersion.version}</Badge>
                          {selectedVersion.version === versionConfig?.version && (
                            <Badge variant="outline">{t('config.current')}</Badge>
                          )}
                          <span className="text-sm text-muted-foreground">
                            {new Date(selectedVersion.created_at).toLocaleString()}
                          </span>
                        </div>
                        <p className="mt-2 text-sm text-muted-foreground">
                          {selectedVersion.comment || t('config.noComment')}
                        </p>
                      </div>
                      <Tabs value={versionViewMode} onValueChange={(value) => setVersionViewMode(value as 'preview' | 'compare')} className="flex min-h-0 flex-1 flex-col">
                        <div className="border-b px-4 py-2">
                          <TabsList>
                            <TabsTrigger value="preview">{t('config.preview')}</TabsTrigger>
                            <TabsTrigger value="compare" disabled={selectedVersion.version === versionConfig?.version}>
                              {t('config.compareWithCurrent')}
                            </TabsTrigger>
                          </TabsList>
                        </div>
                        <TabsContent value="preview" className="mt-0 min-h-0 flex-1">
                          <ScrollArea className="h-full">
                            <pre className="min-h-full whitespace-pre-wrap break-all p-4 text-xs font-mono" data-testid="cluster-configs-version-preview-content">
                              {selectedVersion.content}
                            </pre>
                          </ScrollArea>
                        </TabsContent>
                        <TabsContent value="compare" className="mt-0 min-h-0 flex-1">
                          {selectedVersion.version === versionConfig?.version ? (
                            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                              {t('config.currentVersionCompareHint')}
                            </div>
                          ) : (
                            <div className="flex h-full min-h-0 flex-col">
                              <div className="grid grid-cols-2 border-b text-xs text-muted-foreground">
                                <div className="border-r px-4 py-2">
                                  {t('config.currentVersionLabel', {version: versionConfig?.version ?? '-'})}
                                </div>
                                <div className="px-4 py-2">
                                  {t('config.versionLabel', {version: selectedVersion.version})}
                                </div>
                              </div>
                              <ScrollArea className="h-full" data-testid="cluster-configs-version-compare-content">
                                <div className="grid min-w-[920px] grid-cols-2 text-xs font-mono">
                                  <div className="border-r">
                                    {diffRows.map((row, index) => (
                                      <div
                                        key={`left-${index}`}
                                        className={`grid grid-cols-[56px,minmax(0,1fr)] px-2 py-1 ${row.changed ? 'bg-amber-50 dark:bg-amber-950/20' : ''}`}
                                      >
                                        <span className="pr-3 text-right text-muted-foreground">{row.leftLineNumber ?? ''}</span>
                                        <span className="whitespace-pre-wrap break-all">{row.leftText}</span>
                                      </div>
                                    ))}
                                  </div>
                                  <div>
                                    {diffRows.map((row, index) => (
                                      <div
                                        key={`right-${index}`}
                                        className={`grid grid-cols-[56px,minmax(0,1fr)] px-2 py-1 ${row.changed ? 'bg-amber-50 dark:bg-amber-950/20' : ''}`}
                                      >
                                        <span className="pr-3 text-right text-muted-foreground">{row.rightLineNumber ?? ''}</span>
                                        <span className="whitespace-pre-wrap break-all">{row.rightText}</span>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              </ScrollArea>
                            </div>
                          )}
                        </TabsContent>
                      </Tabs>
                    </div>
                  ) : (
                    <div className="flex h-[65vh] items-center justify-center text-sm text-muted-foreground">
                      {t('config.selectVersionToPreview')}
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setVersionDialogOpen(false)}>{t('common.close')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
