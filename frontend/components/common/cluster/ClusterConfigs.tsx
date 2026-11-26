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
 * 集群配置工作台：采用左侧配置文件清单 + 右侧单层扁平画布（配置预览 + 同页节点同步状态），
 * 去掉右上角文件下拉框、内部二级 Tab 嵌套与 StatPillsBar，减少跳转步骤。
 * Cluster configs workbench: left file rail + right flat single-layer surface (content preview +
 * inline node sync status) without nested tabs, hidden dropdowns, or StatPillsBar.
 */

import {useState, useEffect, useCallback, useMemo, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  Check,
  Download,
  Edit,
  FileText,
  FolderSync,
  GitCompareArrows,
  History,
  MoreHorizontal,
  RefreshCw,
  Upload,
  WandSparkles,
} from 'lucide-react';
import {Alert as ArcAlert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {DropdownMenu as ArcDropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  ConfirmDialog,
  DirtyCloseDialog,
  NeutralChip,
  SoftChip,
  TableLoadingBar,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {Skeleton} from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Textarea} from '@/components/ui/textarea';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {toast} from '@/lib/toast';
import {
  ConfigService,
  ConfigType,
  ConfigTypeNames,
  getConfigTypesForMode,
  summarizeSyncResult,
} from '@/lib/services/config';
import type {ConfigInfo, ConfigVersionInfo} from '@/lib/services/config';
import services from '@/lib/services';
import type {NodeInfo} from '@/lib/services/cluster/types';
import {formatDateTimeMinuteTz, formatDateTimeTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';

interface ClusterConfigsProps {
  clusterId: number;
  deploymentMode: string;
  onConfigChanged?: () => void;
  /** K8s 集群配置唯一真相是 ConfigMap：无节点/模板/下发概念，编辑保存走 updateK8sConfigMapFile。
      K8s clusters treat the in-cluster ConfigMap as the single source of truth — no
      node/template/push semantics; saving edits goes through updateK8sConfigMapFile. */
  isKubernetes?: boolean;
  k8sConfigMapName?: string;
  k8sNamespace?: string;
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
    if (
      seatunnelIndent !== null &&
      key === 'engine' &&
      indent > seatunnelIndent
    ) {
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

function shouldWarnSeatunnelPortSync(
  config: ConfigInfo | null,
  nextContent: string,
  syncAfterSave: boolean,
): boolean {
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

// extractHazelcastClusterName 从 Hazelcast 配置文件内容中提取 cluster-name。
// extractHazelcastClusterName extracts cluster-name from Hazelcast configuration text.
function extractHazelcastClusterName(content: string): string | null {
  const lines = content.split(/\r?\n/);
  let topSectionIndent: number | null = null;
  let inHazelcastSection = false;

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

    if (topSectionIndent !== null && indent <= topSectionIndent) {
      topSectionIndent = null;
      inHazelcastSection = false;
    }

    if (key === 'hazelcast' || key === 'hazelcast-client') {
      topSectionIndent = indent;
      inHazelcastSection = true;
      continue;
    }

    if (
      inHazelcastSection &&
      key === 'cluster-name' &&
      indent > (topSectionIndent ?? 0)
    ) {
      const parsed = value.replace(/^['"]|['"]$/g, '').trim();
      return parsed || null;
    }
  }

  return null;
}

function isHazelcastConfigType(configType?: ConfigType | null): boolean {
  switch (configType) {
    case ConfigType.HAZELCAST:
    case ConfigType.HAZELCAST_CLIENT:
    case ConfigType.HAZELCAST_MASTER:
    case ConfigType.HAZELCAST_WORKER:
      return true;
    default:
      return false;
  }
}

function shouldWarnHazelcastClusterNameChange(
  config: ConfigInfo | null,
  nextContent: string,
): boolean {
  if (!config || !isHazelcastConfigType(config.config_type)) {
    return false;
  }
  const currentName = extractHazelcastClusterName(config.content);
  const nextName = extractHazelcastClusterName(nextContent);
  return nextName !== null && currentName !== null && currentName !== nextName;
}

export function ClusterConfigs({
  clusterId,
  deploymentMode,
  onConfigChanged,
  isKubernetes = false,
  k8sConfigMapName,
  k8sNamespace,
}: ClusterConfigsProps) {
  const t = useTranslations();
  const [configs, setConfigs] = useState<ConfigInfo[]>([]);
  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingLogMode, setPendingLogMode] = useState<
    'per_job' | 'mixed' | null
  >(null);
  const [hasLoaded, setHasLoaded] = useState(false);

  const availableConfigTypes = useMemo(() => {
    if (!isKubernetes) {
      return getConfigTypesForMode(deploymentMode);
    }
    // K8s 文件清单以 ConfigMap 实际内容为准：先排已知类型，再追加自定义文件名，
    // 避免硬编码漏掉 ConfigMap 里真实挂载的文件。
    // For K8s the file rail follows the actual ConfigMap contents: known types first,
    // then any custom file names, so nothing mounted in the ConfigMap is hidden by
    // a hardcoded list.
    const known = getConfigTypesForMode(deploymentMode).filter((type) =>
      configs.some((c) => c.config_type === type),
    );
    const extras = configs
      .map((c) => c.config_type)
      .filter((type) => !known.includes(type));
    return [...new Set([...known, ...extras])];
  }, [isKubernetes, deploymentMode, configs]);
  const [selectedConfigType, setSelectedConfigType] = useState<ConfigType>(
    availableConfigTypes[0],
  );

  // 编辑弹窗状态与脏态拦截 / Edit dialog state & dirty close guard
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [dirtyCloseOpen, setDirtyCloseOpen] = useState(false);
  const [editingConfig, setEditingConfig] = useState<ConfigInfo | null>(null);
  const [editContent, setEditContent] = useState('');
  const [editComment, setEditComment] = useState('');
  const [k8sRestartAfterSave, setK8sRestartAfterSave] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saveMode, setSaveMode] = useState<'save' | 'save-and-sync'>('save');
  const [portChangeConfirmOpen, setPortChangeConfirmOpen] = useState(false);
  const [clusterNameConfirmOpen, setClusterNameConfirmOpen] = useState(false);
  const [pendingSaveSync, setPendingSaveSync] = useState(false);
  const [repairing, setRepairing] = useState(false);

  // 版本历史弹窗状态 / Version history dialog state
  const [versionDialogOpen, setVersionDialogOpen] = useState(false);
  const [versionConfig, setVersionConfig] = useState<ConfigInfo | null>(null);
  const [versions, setVersions] = useState<ConfigVersionInfo[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [selectedVersionId, setSelectedVersionId] = useState<number | null>(
    null,
  );
  const [versionViewMode, setVersionViewMode] = useState<'preview' | 'compare'>(
    'preview',
  );

  // 初始化弹窗状态 / Init from node dialog state
  const [initDialogOpen, setInitDialogOpen] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string>('');
  const [initLoading, setInitLoading] = useState(false);
  const [syncAllLoading, setSyncAllLoading] = useState(false);
  const [logModeSwitching, setLogModeSwitching] = useState(false);

  // 节点筛选状态 / Inline node filter state
  const [nodeFilter, setNodeFilter] = useState<'all' | 'matched' | 'mismatched'>(
    'all',
  );
  const [nodeSearch, setNodeSearch] = useState('');
  const editTextareaRef = useRef<HTMLTextAreaElement | null>(null);
  const reqSeqRef = useRef(0);

  const selectedVersion = useMemo(
    () => versions.find((version) => version.id === selectedVersionId) || null,
    [versions, selectedVersionId],
  );
  const diffRows = useMemo(
    () =>
      buildDiffRows(
        versionConfig?.content || '',
        selectedVersion?.content || '',
      ),
    [versionConfig?.content, selectedVersion?.content],
  );

  const onConfigChangedRef = useRef(onConfigChanged);
  onConfigChangedRef.current = onConfigChanged;

  const notifyConfigChanged = useCallback(() => {
    onConfigChangedRef.current?.();
  }, []);

  const loadData = useCallback(async () => {
    const seq = ++reqSeqRef.current;
    setLoading(true);
    try {
      const [configsResult, nodesResult] = await Promise.all([
        ConfigService.getClusterConfigs(clusterId),
        services.cluster.getNodesSafe(clusterId),
      ]);
      if (seq !== reqSeqRef.current) {
        return;
      }
      setConfigs(configsResult || []);
      if (nodesResult.success && nodesResult.data) {
        setNodes(nodesResult.data);
      }
    } catch (err) {
      if (seq === reqSeqRef.current) {
        toast.error(err instanceof Error ? err.message : t('config.loadError'));
      }
    } finally {
      if (seq === reqSeqRef.current) {
        setHasLoaded(true);
        setLoading(false);
      }
    }
  }, [clusterId, t]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  // 统计每个配置文件的待同步/异常节点数，在左侧文件清单直接透出
  // Per-file mismatch/pending counts surfaced directly in the left file rail
  const fileAlertCountByType = useMemo(() => {
    const map: Partial<Record<ConfigType, number>> = {};
    for (const cfg of configs) {
      if (cfg.is_template) {
        continue;
      }
      if (!cfg.match_template || cfg.sync_status === 'pending_push') {
        map[cfg.config_type] = (map[cfg.config_type] ?? 0) + 1;
      }
    }
    return map;
  }, [configs]);

  const templateConfig = useMemo(
    () =>
      configs.find(
        (c) => c.config_type === selectedConfigType && c.is_template,
      ),
    [configs, selectedConfigType],
  );
  const nodeConfigs = useMemo(
    () =>
      configs.filter(
        (c) => c.config_type === selectedConfigType && !c.is_template,
      ),
    [configs, selectedConfigType],
  );

  const isLog4j2 = selectedConfigType === ConfigType.LOG4J2;
  const isPerJobLogMode = Boolean(
    templateConfig?.content &&
      /rootLogger\.appenderRef\.file\.ref\s*=\s*routingAppender/.test(
        templateConfig.content,
      ),
  );

  const mismatchedNodeCount = useMemo(
    () => nodeConfigs.filter((config) => !config.match_template).length,
    [nodeConfigs],
  );
  const filteredNodeConfigs = useMemo(() => {
    const keyword = nodeSearch.trim().toLowerCase();
    return nodeConfigs.filter((config) => {
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
      const haystack =
        `${config.host_name || ''} ${config.host_ip || ''}`.toLowerCase();
      return haystack.includes(keyword);
    });
  }, [nodeConfigs, nodeFilter, nodeSearch]);

  const editDirty = Boolean(
    editingConfig &&
      (editContent !== editingConfig.content || editComment.trim().length > 0),
  );

  const handleSelectFile = (type: ConfigType) => {
    setSelectedConfigType(type);
    setNodeFilter('all');
    setNodeSearch('');
  };

  const handleSwitchLogMode = async (targetMode: 'per_job' | 'mixed') => {
    setLogModeSwitching(true);
    try {
      const res = await services.cluster.switchJobLogModeSafe(
        clusterId,
        targetMode,
      );
      if (!res.success) {
        toast.error(res.error || t('config.switchLogModeError'));
        return;
      }
      toast.success(
        targetMode === 'per_job'
          ? t('config.switchToPerJobSuccess')
          : t('config.switchToMixedSuccess'),
      );
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('config.switchLogModeError'),
      );
    } finally {
      setLogModeSwitching(false);
    }
  };

  const handleEdit = (config: ConfigInfo) => {
    setEditingConfig(config);
    setEditContent(config.content);
    setEditComment('');
    setEditDialogOpen(true);
  };

  const requestCloseEditDialog = (nextOpen: boolean) => {
    if (nextOpen) {
      setEditDialogOpen(true);
      return;
    }
    if (saving) {
      return;
    }
    if (editDirty) {
      setDirtyCloseOpen(true);
      return;
    }
    setEditDialogOpen(false);
  };

  const beginSave = async (syncAfterSave = false) => {
    if (!editingConfig) {
      return;
    }
    setSaveMode(syncAfterSave ? 'save-and-sync' : 'save');
    setSaving(true);
    try {
      // K8s：写回 ConfigMap 并按勾选滚动重启生效；无版本/模板/节点推送语义。
      // K8s: write back into the ConfigMap and optionally roll-restart per the
      // checkbox; there is no version/template/node-push semantics.
      if (isKubernetes) {
        const fileName = editingConfig.file_path || editingConfig.config_type;
        await services.cluster.updateK8sConfigMapFile(clusterId, fileName, editContent, k8sRestartAfterSave);
        toast.success(t('config.saveSuccess'));
        setEditDialogOpen(false);
        await loadData();
        notifyConfigChanged();
        return;
      }
      const result = await ConfigService.updateConfig(editingConfig.id, {
        content: editContent,
        comment: editComment || undefined,
      });
      if (syncAfterSave && editingConfig.is_template) {
        const syncResult = await ConfigService.syncTemplateToAllNodes(
          clusterId,
          editingConfig.config_type,
        );
        const summary = summarizeSyncResult(syncResult);
        if (summary.kind === 'partial') {
          toast.warning(
            t('config.saveAndSyncSuccessWithPushErrors', {
              count: summary.count,
              nodes: summary.nodes,
            }),
          );
        } else if (summary.kind === 'success') {
          toast.success(t('config.saveAndSyncSuccess', {count: summary.count}));
        } else {
          toast.success(t('config.saveAndSyncSuccessNoChanges'));
        }
      } else if (result.push_error) {
        toast.warning(
          t('config.saveSuccessWithPushError', {error: result.push_error}),
        );
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
    if (shouldWarnHazelcastClusterNameChange(editingConfig, editContent)) {
      setPendingSaveSync(syncAfterSave);
      setClusterNameConfirmOpen(true);
      return;
    }
    if (
      shouldWarnSeatunnelPortSync(editingConfig, editContent, syncAfterSave)
    ) {
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
      toast.error(
        err instanceof Error ? err.message : t('config.smartRepairFailed'),
      );
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
      toast.error(
        err instanceof Error ? err.message : t('config.loadVersionsFailed'),
      );
    } finally {
      setLoadingVersions(false);
    }
  };

  const handleRollback = async (version: number) => {
    if (!versionConfig) {
      return;
    }
    try {
      const result = await ConfigService.rollbackConfig(versionConfig.id, {
        version,
        comment: `Rollback to v${version}`,
      });
      if (result.push_error) {
        toast.warning(
          t('config.rollbackSuccessWithPushError', {error: result.push_error}),
        );
      } else {
        toast.success(t('config.rollbackSuccess'));
      }
      setVersionDialogOpen(false);
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('config.rollbackFailed'),
      );
    }
  };

  const handlePromote = async (config: ConfigInfo) => {
    try {
      const promoteResult = await ConfigService.promoteConfig(config.id, {
        comment: t('config.promoteComment'),
      });
      const summary = summarizeSyncResult(promoteResult);
      if (summary.kind === 'partial') {
        toast.warning(
          t('config.promoteSuccessWithPushErrors', {
            count: summary.count,
            nodes: summary.nodes,
          }),
        );
      } else {
        toast.success(t('config.promoteSuccess'));
      }
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('config.promoteFailed'),
      );
    }
  };

  const handleSyncFromTemplate = async (config: ConfigInfo) => {
    try {
      const result = await ConfigService.syncFromTemplate(config.id, {
        comment: t('config.syncComment'),
      });
      if (result.push_error) {
        toast.warning(
          t('config.syncSuccessWithPushError', {error: result.push_error}),
        );
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
    const node = nodes.find((n) => n.id === Number(selectedNodeId));
    if (!node) {
      toast.error(t('config.nodeNotFound'));
      return;
    }
    setInitLoading(true);
    try {
      await ConfigService.initClusterConfigs(
        clusterId,
        node.host_id,
        node.install_dir,
      );
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
    if (!templateConfig) {
      return;
    }
    setSyncAllLoading(true);
    try {
      const result = await ConfigService.syncTemplateToAllNodes(
        clusterId,
        selectedConfigType,
      );
      const summary = summarizeSyncResult(result);
      if (summary.kind === 'partial') {
        toast.warning(
          t('config.syncAllSuccessWithPushErrors', {
            count: summary.count,
            nodes: summary.nodes,
          }),
        );
      } else if (summary.kind === 'success') {
        toast.success(t('config.syncAllSuccess', {count: summary.count}));
      } else {
        toast.info(t('config.syncAllNoChanges'));
      }
      await loadData();
      notifyConfigChanged();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('config.syncAllFailed'),
      );
    } finally {
      setSyncAllLoading(false);
    }
  };

  return (
    <section
      data-testid='cluster-configs-root'
      className='relative overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'
    >
      <TableLoadingBar loading={loading && hasLoaded} />

      {loading && !hasLoaded ? (
        <div className='grid min-h-[480px] lg:grid-cols-[232px_minmax(0,1fr)]'>
          <div className='space-y-2 border-b-[0.5px] border-border/80 p-4 lg:border-r-[0.5px] lg:border-b-0'>
            <Skeleton className='h-4 w-24' />
            <Skeleton className='h-8 w-full' />
            <Skeleton className='h-8 w-full' />
            <Skeleton className='h-8 w-full' />
            <Skeleton className='h-8 w-full' />
          </div>
          <div className='space-y-4 p-5'>
            <Skeleton className='h-7 w-64' />
            <Skeleton className='h-72 w-full' />
            <Skeleton className='h-24 w-full' />
          </div>
        </div>
      ) : configs.length === 0 ? (
        <div className='px-6 py-12'>
          <EmptyState
            icon={<FileText />}
            title={t('config.noConfigs')}
            description={t('config.initHint')}
            action={
              nodes.length > 0 ? (
                <Button
                  type='button'
                  size='sm'
                  onClick={handleOpenInitDialog}
                  data-testid='cluster-configs-init-button'
                >
                  <FolderSync className='size-3.5' aria-hidden />
                  {t('config.initFromNode')}
                </Button>
              ) : undefined
            }
          />
        </div>
      ) : (
        <div className='grid min-h-[520px] lg:grid-cols-[232px_minmax(0,1fr)]'>
          {/* 左侧配置文件清单：平铺全部文件并透出未对齐节点数 / Left file rail with per-file drift indicator */}
          <aside className='flex flex-col border-b-[0.5px] border-border/80 lg:border-r-[0.5px] lg:border-b-0'>
            <div className='flex items-center justify-between gap-2 border-b-[0.5px] border-border/80 px-4 py-3'>
              <span className='text-xs font-semibold text-foreground'>
                {t('config.title')}
              </span>
              <div className='flex items-center gap-1'>
                {!isKubernetes ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      iconOnly
                      onClick={handleOpenInitDialog}
                      disabled={loading || nodes.length === 0}
                      data-testid='cluster-configs-init-button'
                      aria-label={t('config.initFromNode')}
                    >
                      <FolderSync className='size-3.5' aria-hidden />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side='bottom' className='max-w-xs text-xs'>
                    {t('config.initFromNode')} — {t('config.tipInit')}
                  </TooltipContent>
                </Tooltip>
                ) : null}
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  iconOnly
                  onClick={() => void loadData()}
                  disabled={loading}
                  aria-label={t('common.refresh')}
                  title={t('common.refresh')}
                >
                  <RefreshCw
                    className={`size-3.5 ${loading ? 'animate-spin' : ''}`}
                    aria-hidden
                  />
                </Button>
              </div>
            </div>

            <nav
              className='flex gap-1 overflow-x-auto p-2 lg:flex-col lg:overflow-x-visible'
              aria-label={t('config.title')}
              data-testid='cluster-configs-file-list'
            >
              {availableConfigTypes.map((type) => {
                const active = type === selectedConfigType;
                const alertCount = fileAlertCountByType[type] ?? 0;
                return (
                  <button
                    key={type}
                    type='button'
                    // 固定按钮无障碍名称，避免待同步角标数字拼入 getByRole 匹配串 / Keep button accessible name stable when alert count badge renders
                    aria-label={ConfigTypeNames[type] || type}
                    onClick={() => handleSelectFile(type)}
                    data-testid={`cluster-configs-file-${type}`}
                    aria-current={active ? 'page' : undefined}
                    className={cn(
                      'flex shrink-0 items-center justify-between gap-2 rounded-md px-2.5 py-2 text-left font-mono text-xs transition-colors lg:w-full',
                      active
                        ? 'bg-muted font-semibold text-foreground'
                        : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
                    )}
                  >
                    <span className='truncate'>{ConfigTypeNames[type] || type}</span>
                    {alertCount > 0 ? (
                      <SoftChip
                        tone='amber'
                        className='shrink-0 px-1.5 text-[10px]'
                      >
                        {alertCount}
                      </SoftChip>
                    ) : null}
                  </button>
                );
              })}
            </nav>
          </aside>

          {/* 右侧单层工作区：上段配置内容，下段节点同步状态（0.5px 细线分隔，零二级 Tab） */}
          {/* Right flat single-layer pane: config content above, node sync table below via 0.5px hairline */}
          <div className='min-w-0 divide-y-[0.5px] divide-border/80'>
            {/* 顶栏：文件名、版本、更新时间与主次动作 / Top bar */}
            <div className='flex flex-wrap items-center justify-between gap-3 px-5 py-3.5'>
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <h3 className='truncate font-mono text-sm font-semibold text-foreground'>
                  {ConfigTypeNames[selectedConfigType] || selectedConfigType}
                </h3>
                {isKubernetes && k8sConfigMapName ? (
                  <NeutralChip className='font-mono'>
                    {`ConfigMap: ${k8sConfigMapName}`}
                  </NeutralChip>
                ) : null}
                {templateConfig && !isKubernetes ? (
                  <>
                    <NeutralChip className='font-mono tabular-nums'>
                      v{templateConfig.version}
                    </NeutralChip>
                    <span className='font-mono text-xs text-muted-foreground tabular-nums'>
                      {formatDateTimeMinuteTz(templateConfig.updated_at)}
                    </span>
                  </>
                ) : null}
              </div>

              {templateConfig ? (
                <div className='flex flex-wrap items-center gap-1.5'>
                  {!isKubernetes ? (
                  <Button
                    type='button'
                    variant='secondary'
                    size='sm'
                    title={t('config.tipVersions')}
                    onClick={() => void handleViewVersions(templateConfig)}
                    data-testid='cluster-configs-template-versions'
                  >
                    <History className='size-3.5' aria-hidden />
                    {t('config.versions')}
                  </Button>
                  ) : null}
                  {!isKubernetes ? (
                  <Button
                    type='button'
                    variant={mismatchedNodeCount > 0 ? 'primary' : 'secondary'}
                    size='sm'
                    title={t('config.tipSyncAll')}
                    onClick={() => void handleSyncToAllNodes()}
                    loading={syncAllLoading}
                    disabled={nodeConfigs.length === 0}
                    data-testid='cluster-configs-template-sync-all'
                  >
                    <Download className='size-3.5' aria-hidden />
                    {t('config.syncToAllNodes')}
                  </Button>
                  ) : null}
                  <Button
                    type='button'
                    variant={mismatchedNodeCount > 0 ? 'secondary' : 'primary'}
                    size='sm'
                    title={t('config.tipEdit')}
                    onClick={() => handleEdit(templateConfig)}
                    data-testid='cluster-configs-template-edit'
                  >
                    <Edit className='size-3.5' aria-hidden />
                    {t('common.edit')}
                  </Button>
                </div>
              ) : null}
            </div>

            {/* 配置内容区 / Config content section */}
            <div className='space-y-3.5 px-5 py-4'>
              {templateConfig && !isKubernetes && mismatchedNodeCount > 0 ? (
                <div data-testid='cluster-configs-pending-sync'>
                  <ArcAlert
                    tone='warning'
                    title={t('config.pendingSyncTitle', {
                      count: mismatchedNodeCount,
                    })}
                  >
                    <div className='mt-2'>
                      <Button
                        type='button'
                        variant='secondary'
                        size='sm'
                        onClick={() => void handleSyncToAllNodes()}
                        loading={syncAllLoading}
                        data-testid='cluster-configs-template-sync-all-banner'
                      >
                        <Download className='size-3.5' aria-hidden />
                        {t('config.syncToAllNodes')}
                      </Button>
                    </div>
                  </ArcAlert>
                </div>
              ) : null}

              {templateConfig && isLog4j2 ? (
                <div className='flex flex-wrap items-center justify-between gap-3 border-b-[0.5px] border-border/80 pb-3.5 text-xs'>
                  <div className='space-y-0.5'>
                    <div className='flex items-center gap-2'>
                      <span className='font-semibold text-foreground'>
                        {t('config.jobLogModeTitle')}
                      </span>
                      <span data-testid='cluster-configs-log-mode-badge'>
                        <SoftChip
                          tone={isPerJobLogMode ? 'emerald' : 'amber'}
                        >
                          {isPerJobLogMode
                            ? t('config.jobLogModePerJobBadge')
                            : t('config.jobLogModeMixedBadge')}
                        </SoftChip>
                      </span>
                    </div>
                    <p className='text-muted-foreground'>
                      {isPerJobLogMode
                        ? t('config.jobLogModeRestartHint')
                        : t('config.jobLogModeMixedDesc')}
                    </p>
                  </div>
                  <div className='w-full shrink-0 sm:w-[260px]'>
                    <Select
                      aria-label={t('config.jobLogModeTitle')}
                      data-testid='cluster-configs-log-mode-select'
                      value={isPerJobLogMode ? 'per_job' : 'mixed'}
                      disabled={logModeSwitching}
                      onValueChange={(next) => {
                        const current = isPerJobLogMode ? 'per_job' : 'mixed';
                        if (
                          !logModeSwitching &&
                          (next === 'per_job' || next === 'mixed') &&
                          next !== current
                        ) {
                          setPendingLogMode(next);
                        }
                      }}
                      options={[
                        {
                          value: 'per_job',
                          label: t('config.jobLogModePerJobBadge'),
                        },
                        {
                          value: 'mixed',
                          label: t('config.jobLogModeMixedBadge'),
                        },
                      ]}
                    />
                  </div>
                </div>
              ) : null}

              {templateConfig ? (
                <pre
                  className='max-h-[460px] min-h-[240px] overflow-auto rounded-md bg-muted/35 p-4 font-mono text-xs leading-5 whitespace-pre'
                  data-testid='cluster-configs-template-content'
                >
                  {templateConfig.content}
                </pre>
              ) : (
                <p className='py-6 text-center text-xs text-muted-foreground'>
                  {t('config.noTemplate')}
                </p>
              )}
            </div>

            {!isKubernetes ? (<>
            {/* 同页节点同步状态区：一眼查看全部节点一致性，无需切 Tab */}
            {/* Inline node sync status table right below content, no secondary tab switch */}
            <div className='space-y-3 px-5 py-4'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <div className='flex items-center gap-2'>
                  <h4 className='text-xs font-semibold text-foreground'>
                    {t('config.nodeConfigs')}
                  </h4>
                  <span className='text-xs text-muted-foreground tabular-nums'>
                    {nodeConfigs.length}
                  </span>
                </div>

                {nodeConfigs.length > 1 || nodeSearch ? (
                  <div className='flex flex-wrap items-center gap-2'>
                    <SegmentedControl
                      label={t('config.nodeConfigs')}
                      value={nodeFilter}
                      onValueChange={(v) =>
                        setNodeFilter(v as 'all' | 'matched' | 'mismatched')
                      }
                      options={[
                        {value: 'all', label: t('config.filterAll')},
                        {value: 'matched', label: t('config.filterMatched')},
                        {
                          value: 'mismatched',
                          label: t('config.filterMismatched'),
                        },
                      ]}
                    />
                    <div className={layout.toolbarSearch}>
                      <SearchField
                        label={t('config.searchNodes')}
                        value={nodeSearch}
                        onValueChange={setNodeSearch}
                        placeholder={t('config.searchNodes')}
                        clearLabel={t('common.clearFilters')}
                        data-testid='cluster-configs-node-search'
                      />
                    </div>
                  </div>
                ) : null}
              </div>

              {nodeConfigs.length === 0 ? (
                <p className='py-3 text-xs text-muted-foreground'>
                  {t('config.noNodeConfigs')}
                </p>
              ) : (
                <div className='overflow-x-auto'>
                  <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
                    <TableHeader>
                      <TableRow className='hover:bg-transparent'>
                        <TableHead>{t('config.selectNode')}</TableHead>
                        <TableHead>{t('config.version')}</TableHead>
                        <TableHead>{t('config.updatedAt')}</TableHead>
                        <TableHead className='text-right'>
                          {t('common.actions')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredNodeConfigs.length === 0 ? (
                        <TableRow>
                          <TableCell
                            colSpan={4}
                            className='py-6 text-center text-xs text-muted-foreground'
                          >
                            {t('config.noMatchingNodes')}
                          </TableCell>
                        </TableRow>
                      ) : (
                        filteredNodeConfigs.map((config) => (
                          <TableRow key={config.id}>
                            <TableCell>
                              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                                <span className='truncate font-medium'>
                                  {config.host_name || `Node ${config.host_id}`}
                                </span>
                                {config.host_ip ? (
                                  <span className='font-mono text-xs text-muted-foreground tabular-nums'>
                                    {config.host_ip}
                                  </span>
                                ) : null}
                                {config.sync_status === 'pending_push' ? (
                                  <span
                                    title={t('config.pendingPushTip', {
                                      error: config.sync_error || '-',
                                    })}
                                    data-testid={`cluster-configs-node-pending-${config.id}`}
                                  >
                                    <SoftChip tone='amber'>
                                      <AlertTriangle
                                        className='size-3'
                                        aria-hidden
                                      />
                                      {t('config.pendingPush')}
                                    </SoftChip>
                                  </span>
                                ) : null}
                                {config.match_template ? (
                                  <SoftChip tone='emerald'>
                                    <Check className='size-3' aria-hidden />
                                    {t('config.matchTemplate')}
                                  </SoftChip>
                                ) : (
                                  <SoftChip tone='amber'>
                                    <AlertTriangle className='size-3' aria-hidden />
                                    {t('config.customized')}
                                  </SoftChip>
                                )}
                              </div>
                            </TableCell>
                            <TableCell className='font-mono text-xs tabular-nums whitespace-nowrap'>
                              v{config.version}
                            </TableCell>
                            <TableCell className='font-mono text-xs text-muted-foreground tabular-nums whitespace-nowrap'>
                              {formatDateTimeMinuteTz(config.updated_at)}
                            </TableCell>
                            <TableCell className='text-right'>
                              <div className='flex items-center justify-end gap-1.5'>
                                {!config.match_template ? (
                                  <>
                                    <Button
                                      type='button'
                                      variant='secondary'
                                      size='sm'
                                      title={t('config.tipSyncNode')}
                                      onClick={() =>
                                        void handleSyncFromTemplate(config)
                                      }
                                      data-testid={`cluster-configs-node-sync-${config.id}`}
                                    >
                                      <Download className='size-3.5' aria-hidden />
                                      {t('config.syncFromTemplate')}
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='secondary'
                                      size='sm'
                                      title={t('config.tipPromote')}
                                      onClick={() => void handlePromote(config)}
                                      data-testid={`cluster-configs-node-promote-${config.id}`}
                                    >
                                      <Upload className='size-3.5' aria-hidden />
                                      {t('config.promoteToCluster')}
                                    </Button>
                                  </>
                                ) : null}
                                <ArcDropdownMenu
                                  trigger={
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='sm'
                                      iconOnly
                                      aria-label={t('common.actions')}
                                    >
                                      <MoreHorizontal
                                        className='size-4'
                                        aria-hidden
                                      />
                                    </Button>
                                  }
                                  items={[
                                    {
                                      label: t('common.edit'),
                                      icon: <Edit className='size-3.5' aria-hidden />,
                                      onSelect: () => handleEdit(config),
                                    },
                                    {
                                      label: t('config.versions'),
                                      icon: (
                                        <History className='size-3.5' aria-hidden />
                                      ),
                                      onSelect: () =>
                                        void handleViewVersions(config),
                                    },
                                  ]}
                                />
                              </div>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              )}
            </div>
            </>) : null}
          </div>
        </div>
      )}

      {/* 从节点初始化弹窗 / Init from node dialog */}
      <Dialog open={initDialogOpen} onOpenChange={setInitDialogOpen}>
        <DialogContent
          className='arc-compact w-[min(92vw,580px)]!'
          title={t('config.initFromNode')}
          description={t('config.initFromNodeDesc')}
        >
          <div className='space-y-4'>
            <div className='space-y-2'>
              <span className='block text-xs font-medium text-foreground'>
                {t('config.selectNode')}
              </span>
              <div className='max-h-60 divide-y-[0.5px] divide-border/80 overflow-y-auto rounded-md border border-border/80'>
                {nodes.map((node) => {
                  const selected = selectedNodeId === String(node.id);
                  return (
                    <button
                      key={node.id}
                      type='button'
                      onClick={() => setSelectedNodeId(String(node.id))}
                      data-testid={`cluster-configs-init-node-${node.id}`}
                      className={cn(
                        'flex w-full items-center justify-between gap-3 px-3.5 py-2.5 text-left transition-colors',
                        selected ? 'bg-muted' : 'hover:bg-muted/40',
                      )}
                    >
                      <div className='min-w-0'>
                        <div className='truncate text-sm font-medium text-foreground'>
                          {node.host_name || `Node ${node.id}`} ({node.host_ip})
                        </div>
                        <div className='mt-0.5 truncate font-mono text-xs text-muted-foreground'>
                          {node.install_dir}
                        </div>
                      </div>
                      <NeutralChip>{node.role}</NeutralChip>
                    </button>
                  );
                })}
              </div>
            </div>
            {selectedNodeId ? (
              <div className='border-t-[0.5px] border-border/80 pt-3 text-xs text-muted-foreground'>
                <p>{t('config.initWillPull')}</p>
                <div className='mt-2 flex flex-wrap gap-1.5'>
                  {availableConfigTypes.map((type) => (
                    <NeutralChip key={type} className='font-mono'>
                      {ConfigTypeNames[type]}
                    </NeutralChip>
                  ))}
                </div>
              </div>
            ) : null}
            <div className='flex items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
              <Button
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => setInitDialogOpen(false)}
              >
                {t('common.cancel')}
              </Button>
              <Button
                type='button'
                size='sm'
                onClick={() => void handleInitConfigs()}
                loading={initLoading}
                disabled={!selectedNodeId}
                data-testid='cluster-configs-init-confirm'
              >
                {t('config.initConfirm')}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* 配置编辑弹窗（含脏态关闭拦截与智能修复）/ Config edit dialog with dirty guard & smart repair */}
      <Dialog open={editDialogOpen} onOpenChange={requestCloseEditDialog}>
        <DialogContent
          className='arc-compact w-[min(94vw,1040px)]!'
          title={`${t('config.editConfig')} · ${
            editingConfig?.is_template
              ? t('config.clusterTemplate')
              : editingConfig?.host_name || ''
          }`}
          description={
            <span className='font-mono text-xs'>
              {ConfigTypeNames[editingConfig?.config_type as ConfigType]}
            </span>
          }
          data-testid='cluster-configs-edit-dialog'
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            window.requestAnimationFrame(() => {
              editTextareaRef.current?.focus();
            });
          }}
        >
          <div className='space-y-4'>
            <div className='space-y-2'>
              <div className='flex items-center justify-between gap-3'>
                <label
                  htmlFor='cluster-config-edit-content'
                  className='text-xs font-medium text-foreground'
                >
                  {t('config.content')}
                </label>
                {isYamlConfigType(editingConfig?.config_type) ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        type='button'
                        variant='secondary'
                        size='sm'
                        onClick={() => void handleSmartRepair()}
                        loading={repairing}
                        disabled={saving}
                        data-testid='cluster-configs-smart-repair'
                      >
                        <WandSparkles className='size-3.5' aria-hidden />
                        {t('config.smartRepair')}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent
                      side='top'
                      className='max-w-xs text-xs leading-relaxed'
                    >
                      {t('config.smartRepairHint')}
                    </TooltipContent>
                  </Tooltip>
                ) : null}
              </div>
              <Textarea
                id='cluster-config-edit-content'
                ref={editTextareaRef}
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                className='h-[460px] font-mono text-xs leading-5'
                data-testid='cluster-configs-edit-content'
              />
            </div>
            {!isKubernetes ? (
              <Input
                label={t('config.comment')}
                value={editComment}
                onChange={(e) => setEditComment(e.target.value)}
                placeholder={t('config.commentPlaceholder')}
              />
            ) : null}
            <div className='flex items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
              {isKubernetes ? (
                <label className='mr-auto flex items-center gap-2 text-xs text-foreground'>
                  <Checkbox
                    checked={k8sRestartAfterSave}
                    onCheckedChange={(checked) => setK8sRestartAfterSave(checked === true)}
                    data-testid='cluster-configs-k8s-restart-checkbox'
                  />
                  {t('cluster.k8s.saveAndRollout')}
                </label>
              ) : null}
              <Button
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => requestCloseEditDialog(false)}
                disabled={saving}
              >
                {t('common.cancel')}
              </Button>
              {!isKubernetes && editingConfig?.is_template ? (
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={() => void handleSave(false)}
                  loading={saving && saveMode === 'save'}
                  disabled={saving}
                  data-testid='cluster-configs-save-only'
                >
                  {t('config.saveOnly')}
                </Button>
              ) : null}
              <Button
                type='button'
                size='sm'
                onClick={() =>
                  void handleSave(!isKubernetes && Boolean(editingConfig?.is_template))
                }
                loading={
                  saving &&
                  (saveMode === 'save-and-sync' || !editingConfig?.is_template)
                }
                disabled={saving}
                data-testid='cluster-configs-save-primary'
              >
                {!isKubernetes && editingConfig?.is_template
                  ? t('config.saveAndSync')
                  : t('common.save')}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <DirtyCloseDialog
        open={dirtyCloseOpen}
        onDiscard={() => {
          setDirtyCloseOpen(false);
          setEditDialogOpen(false);
        }}
        onKeepEditing={() => setDirtyCloseOpen(false)}
      />

      {/* 集群标识修改级联风险警示对话框 / Cluster Name Change Cascading Risk Warning Dialog */}
      <ConfirmDialog
        open={clusterNameConfirmOpen}
        onOpenChange={(open) => {
          setClusterNameConfirmOpen(open);
          if (!open) {
            setPendingSaveSync(false);
          }
        }}
        title={
          <span className='flex items-center gap-2 text-amber-600 dark:text-amber-500'>
            <AlertTriangle className='size-4 shrink-0' aria-hidden />
            {t('cluster.clusterNameChangeWarningTitle')}
          </span>
        }
        cancelLabel={t('common.cancel')}
        confirmLabel={t('common.confirm')}
        confirmVariant='danger'
        onConfirm={async () => {
          setClusterNameConfirmOpen(false);
          if (
            shouldWarnSeatunnelPortSync(
              editingConfig,
              editContent,
              pendingSaveSync,
            )
          ) {
            setPortChangeConfirmOpen(true);
            return;
          }
          await beginSave(pendingSaveSync);
        }}
      >
        <p className='text-sm leading-relaxed whitespace-pre-line text-foreground/90'>
          {t('cluster.clusterNameChangeWarningContent')}
        </p>
      </ConfirmDialog>

      <ConfirmDialog
        open={portChangeConfirmOpen}
        onOpenChange={(open) => {
          setPortChangeConfirmOpen(open);
          if (!open) {
            setPendingSaveSync(false);
          }
        }}
        title={t('config.portChangeConfirmTitle')}
        description={t('config.portChangeConfirmDesc')}
        cancelLabel={t('common.cancel')}
        confirmLabel={t('common.confirm')}
        onConfirm={async () => {
          setPortChangeConfirmOpen(false);
          await beginSave(pendingSaveSync);
        }}
      >
        <ul className='list-disc space-y-1 pl-5 text-sm text-muted-foreground'>
          <li>{t('config.portChangeConfirmSyncConfig')}</li>
          <li>{t('config.portChangeConfirmSyncMetadata')}</li>
          <li>{t('config.portChangeConfirmImpact')}</li>
        </ul>
      </ConfirmDialog>

      {/* 版本历史弹窗：压平为单层左右分栏，左侧精简列表点击即选，右侧 SegmentedControl 切预览/对比 */}
      {/* Version history dialog: flat single-layer split, clickable version rail on left, SegmentedControl preview/compare on right */}
      <Dialog open={versionDialogOpen} onOpenChange={setVersionDialogOpen}>
        <DialogContent
          className='arc-compact w-[min(95vw,1180px)]!'
          bodyClassName='p-0! flex flex-col overflow-hidden'
          title={t('config.versionHistory')}
          description={`${
            versionConfig?.is_template
              ? t('config.clusterTemplate')
              : versionConfig?.host_name || ''
          }${
            versionConfig
              ? ` · ${ConfigTypeNames[versionConfig.config_type]}`
              : ''
          }`}
          data-testid='cluster-configs-versions-dialog'
        >
          {loadingVersions ? (
            <div className='grid gap-4 p-5 lg:grid-cols-[260px_minmax(0,1fr)]'>
              <div className='space-y-2'>
                <Skeleton className='h-14 w-full' />
                <Skeleton className='h-14 w-full' />
                <Skeleton className='h-14 w-full' />
              </div>
              <Skeleton className='h-[55vh] w-full' />
            </div>
          ) : versions.length === 0 ? (
            <div className='py-10'>
              <EmptyState
                icon={<History />}
                title={t('config.noVersions')}
                description={t('config.versionHistory')}
              />
            </div>
          ) : (
            <div className='grid h-[64vh] min-h-0 overflow-hidden lg:grid-cols-[260px_minmax(0,1fr)]'>
              {/* 左侧精简版本清单 / Compact version rail */}
              <div className='min-h-0 divide-y-[0.5px] divide-border/70 overflow-y-auto border-b-[0.5px] border-border/80 lg:border-r-[0.5px] lg:border-b-0'>
                {versions.map((version) => {
                  const selected = version.id === selectedVersionId;
                  const isCurrent = version.version === versionConfig?.version;
                  return (
                    <div
                      key={version.id}
                      className={cn(
                        'flex items-start justify-between gap-2 px-3 py-3 transition-colors',
                        selected ? 'bg-muted/70' : 'hover:bg-muted/35',
                      )}
                    >
                      <button
                        type='button'
                        className='min-w-0 flex-1 text-left'
                        onClick={() => {
                          setSelectedVersionId(version.id);
                          setVersionViewMode('preview');
                        }}
                        data-testid={`cluster-configs-version-preview-${version.id}`}
                      >
                        <div className='flex items-center gap-1.5'>
                          <span className='font-mono text-xs font-semibold tabular-nums text-foreground'>
                            v{version.version}
                          </span>
                          {isCurrent ? (
                            <SoftChip tone='emerald'>
                              {t('config.current')}
                            </SoftChip>
                          ) : null}
                        </div>
                        <div className='mt-1 font-mono text-[11px] text-muted-foreground tabular-nums'>
                          {formatDateTimeMinuteTz(version.created_at)}
                        </div>
                        <p className='mt-1 line-clamp-2 text-xs text-muted-foreground'>
                          {version.comment || t('config.noComment')}
                        </p>
                      </button>

                      {!isCurrent ? (
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          title={t('config.compareWithCurrent')}
                          aria-label={t('config.compareWithCurrent')}
                          onClick={() => {
                            setSelectedVersionId(version.id);
                            setVersionViewMode('compare');
                          }}
                          data-testid={`cluster-configs-version-compare-${version.id}`}
                        >
                          <GitCompareArrows className='size-3.5' aria-hidden />
                        </Button>
                      ) : null}
                    </div>
                  );
                })}
              </div>

              {/* 右侧预览与对比区 / Right preview & diff pane */}
              <div className='flex min-h-0 min-w-0 flex-col'>
                {selectedVersion ? (
                  <>
                    <div className='flex flex-wrap items-center justify-between gap-2 border-b-[0.5px] border-border/80 px-4 py-3'>
                      <div className='flex min-w-0 flex-wrap items-center gap-2 text-xs'>
                        <NeutralChip className='font-mono tabular-nums'>
                          v{selectedVersion.version}
                        </NeutralChip>
                        <span className='font-mono text-muted-foreground tabular-nums'>
                          {formatDateTimeTz(selectedVersion.created_at)}
                        </span>
                      </div>

                      <div className='flex items-center gap-2'>
                        <SegmentedControl
                          label={t('config.versionHistory')}
                          value={versionViewMode}
                          onValueChange={(value) => {
                            if (
                              value === 'compare' &&
                              selectedVersion.version === versionConfig?.version
                            ) {
                              return;
                            }
                            setVersionViewMode(value as 'preview' | 'compare');
                          }}
                          options={[
                            {value: 'preview', label: t('config.preview')},
                            {
                              value: 'compare',
                              label: t('config.compareWithCurrent'),
                            },
                          ]}
                        />
                        {selectedVersion.version !== versionConfig?.version ? (
                          <Button
                            type='button'
                            variant='secondary'
                            size='sm'
                            onClick={() =>
                              void handleRollback(selectedVersion.version)
                            }
                            data-testid={`cluster-configs-version-rollback-${selectedVersion.id}`}
                          >
                            {t('config.rollback')}
                          </Button>
                        ) : null}
                      </div>
                    </div>

                    {versionViewMode === 'preview' ? (
                      <div className='min-h-0 flex-1 overflow-auto'>
                        <pre
                          className='min-h-full p-4 font-mono text-xs leading-5 whitespace-pre'
                          data-testid='cluster-configs-version-preview-content'
                        >
                          {selectedVersion.content}
                        </pre>
                      </div>
                    ) : selectedVersion.version === versionConfig?.version ? (
                      <div className='flex flex-1 items-center justify-center text-xs text-muted-foreground'>
                        {t('config.currentVersionCompareHint')}
                      </div>
                    ) : (
                      <div className='flex min-h-0 flex-1 flex-col'>
                        <div className='grid grid-cols-2 border-b-[0.5px] border-border/80 bg-muted/20 text-xs text-muted-foreground'>
                          <div className='border-r-[0.5px] border-border/70 px-4 py-2 font-mono tabular-nums'>
                            {t('config.currentVersionLabel', {
                              version: versionConfig?.version ?? '-',
                            })}
                          </div>
                          <div className='px-4 py-2 font-mono tabular-nums'>
                            {t('config.versionLabel', {
                              version: selectedVersion.version,
                            })}
                          </div>
                        </div>
                        <div
                          className='min-h-0 flex-1 overflow-auto'
                          data-testid='cluster-configs-version-compare-content'
                        >
                          <div className='grid min-w-[820px] grid-cols-2 font-mono text-xs'>
                            <div className='border-r-[0.5px] border-border/70'>
                              {diffRows.map((row, index) => (
                                <div
                                  key={`left-${index}`}
                                  className={cn(
                                    'grid grid-cols-[48px_minmax(0,1fr)] px-2 py-0.5',
                                    row.changed &&
                                      'bg-amber-500/10 dark:bg-amber-500/15',
                                  )}
                                >
                                  <span className='pr-3 text-right text-muted-foreground tabular-nums'>
                                    {row.leftLineNumber ?? ''}
                                  </span>
                                  <span className='whitespace-pre-wrap break-words'>
                                    {row.leftText}
                                  </span>
                                </div>
                              ))}
                            </div>
                            <div>
                              {diffRows.map((row, index) => (
                                <div
                                  key={`right-${index}`}
                                  className={cn(
                                    'grid grid-cols-[48px_minmax(0,1fr)] px-2 py-0.5',
                                    row.changed &&
                                      'bg-amber-500/10 dark:bg-amber-500/15',
                                  )}
                                >
                                  <span className='pr-3 text-right text-muted-foreground tabular-nums'>
                                    {row.rightLineNumber ?? ''}
                                  </span>
                                  <span className='whitespace-pre-wrap break-words'>
                                    {row.rightText}
                                  </span>
                                </div>
                              ))}
                            </div>
                          </div>
                        </div>
                      </div>
                    )}
                  </>
                ) : (
                  <div className='flex flex-1 items-center justify-center text-xs text-muted-foreground'>
                    {t('config.selectVersionToPreview')}
                  </div>
                )}
              </div>
            </div>
          )}

          <div className='flex items-center justify-end border-t-[0.5px] border-border/80 px-5 py-3'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => setVersionDialogOpen(false)}
            >
              {t('common.close')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={pendingLogMode !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingLogMode(null);
          }
        }}
        title={t('config.switchLogModeConfirmTitle')}
        description={
          pendingLogMode
            ? t('config.switchLogModeConfirmDesc', {
                mode:
                  pendingLogMode === 'per_job'
                    ? t('config.jobLogModePerJobBadge')
                    : t('config.jobLogModeMixedBadge'),
              })
            : ''
        }
        cancelLabel={t('common.cancel')}
        confirmLabel={t('config.switchLogModeConfirmAction')}
        loading={logModeSwitching}
        onConfirm={async () => {
          if (!pendingLogMode) {
            return;
          }
          await handleSwitchLogMode(pendingLogMode);
          setPendingLogMode(null);
        }}
      />
    </section>
  );
}
