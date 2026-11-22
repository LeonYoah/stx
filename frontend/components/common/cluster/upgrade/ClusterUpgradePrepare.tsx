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
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  FileSearch,
  Package,
  PlugZap,
  RefreshCw,
  ServerCog,
  ShieldAlert,
} from 'lucide-react';
import {Alert as ArcAlert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Badge} from '@/components/arc/badge/badge';
import {ButtonGroup} from '@/components/arc/button-group/button-group';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {TableLoadingBar, WorkspaceHeader} from '@/components/common/layout';
import {Skeleton} from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import services from '@/lib/services';
import {resolveSeatunnelVersion} from '@/lib/seatunnel-version';
import {
  clearStUpgradeSession,
  loadStUpgradeSession,
  patchStUpgradeSession,
} from '@/lib/st-upgrade-session';
import type {AvailableVersions} from '@/lib/services/installer/types';
import type {ClusterInfo} from '@/lib/services/cluster/types';
import type {
  BlockingIssue,
  CreatePlanRequest,
  PrecheckResult,
} from '@/lib/services/st-upgrade';
import {UpgradeSteps} from './UpgradeSteps';
import {HelpHint} from '@/components/common/HelpHint';
import {JavaHomePicker} from '@/components/common/java/JavaHomePicker';
import {DisabledReasonTooltip} from '@/components/common/DisabledReasonTooltip';
import {getIssueCategoryLabel} from './utils';

interface ClusterUpgradePrepareProps {
  clusterId: number;
}

export function ClusterUpgradePrepare({clusterId}: ClusterUpgradePrepareProps) {
  const t = useTranslations('stUpgrade');
  const commonT = useTranslations('common');
  const router = useRouter();

  const [cluster, setCluster] = useState<ClusterInfo | null>(null);
  const [availableVersions, setAvailableVersions] =
    useState<AvailableVersions | null>(null);
  const [targetVersion, setTargetVersion] = useState('');
  const [targetInstallDir, setTargetInstallDir] = useState('');
  const [targetInstallDirTouched, setTargetInstallDirTouched] = useState(false);
  const [packageChecksum, setPackageChecksum] = useState('');
  const [connectorNamesText, setConnectorNamesText] = useState('');
  const [confirmedRequest, setConfirmedRequest] =
    useState<CreatePlanRequest | null>(null);
  const [precheck, setPrecheck] = useState<PrecheckResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [runningPrecheck, setRunningPrecheck] = useState(false);
  const [initializingConfigs, setInitializingConfigs] = useState(false);
  // 高级选项默认折叠，渐进式披露 / Advanced options collapsed by default
  const [advancedExpanded, setAdvancedExpanded] = useState(false);
  const [nodeJavaHomes, setNodeJavaHomes] = useState<Record<string, string>>(
    {},
  );

  const restoreDraft = useCallback(() => {
    const draft = loadStUpgradeSession(clusterId);
    if (!draft) {
      return;
    }
    if (draft.request?.target_version) {
      setTargetVersion(draft.request.target_version);
    }
    if (draft.request?.package_checksum) {
      setPackageChecksum(draft.request.package_checksum);
    }
    if (draft.request?.connector_names?.length) {
      setConnectorNamesText(draft.request.connector_names.join(','));
    }
    // 草稿已有高级字段时自动展开 / Auto-expand when draft has advanced fields
    if (
      draft.request?.package_checksum ||
      draft.request?.connector_names?.length
    ) {
      setAdvancedExpanded(true);
    }
    if (draft.request?.target_install_dir) {
      setTargetInstallDir(draft.request.target_install_dir);
      setTargetInstallDirTouched(true);
    }
    if (draft.request?.node_java_homes) {
      setNodeJavaHomes({...draft.request.node_java_homes});
    }
    if (draft.request) {
      setConfirmedRequest(draft.request);
    }
    if (draft.precheck) {
      setPrecheck(draft.precheck);
    }
  }, [clusterId]);

  const suggestedTargetInstallDir = useMemo(
    () =>
      buildSuggestedTargetInstallDir(
        cluster?.install_dir,
        cluster?.version,
        targetVersion,
      ),
    [cluster?.install_dir, cluster?.version, targetVersion],
  );

  const loadInitialData = useCallback(async () => {
    setLoading(true);
    try {
      const [clusterResult, versions] = await Promise.all([
        services.cluster.getClusterSafe(clusterId),
        services.installer.listPackages(),
      ]);

      if (!clusterResult.success || !clusterResult.data) {
        throw new Error(clusterResult.error || t('loadClusterFailed'));
      }

      const clusterInfo = clusterResult.data;
      setCluster(clusterInfo);
      setAvailableVersions(versions);
      setTargetVersion(
        (current) =>
          current || resolveSeatunnelVersion(versions) || clusterInfo.version,
      );
      restoreDraft();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('loadPrepareFailed'),
      );
    } finally {
      setLoading(false);
    }
  }, [clusterId, restoreDraft, t]);

  useEffect(() => {
    void loadInitialData();
  }, [loadInitialData]);

  useEffect(() => {
    if (targetInstallDirTouched) {
      return;
    }
    setTargetInstallDir(suggestedTargetInstallDir);
  }, [suggestedTargetInstallDir, targetInstallDirTouched]);

  const buildRequest = useCallback((): CreatePlanRequest => {
    const nodeHomes = compactNodeJavaHomes(nodeJavaHomes);
    return {
      cluster_id: clusterId,
      target_version: targetVersion,
      target_install_dir: targetInstallDir.trim() || undefined,
      package_checksum: packageChecksum || undefined,
      connector_names: connectorNamesText
        .split(',')
        .map((name) => name.trim())
        .filter(Boolean),
      node_java_homes: nodeHomes,
    };
  }, [
    clusterId,
    connectorNamesText,
    nodeJavaHomes,
    packageChecksum,
    targetInstallDir,
    targetVersion,
  ]);

  const currentDraftRequest = useMemo(() => buildRequest(), [buildRequest]);
  const draftTargetInstallDirPreview = useMemo(
    () => targetInstallDir.trim() || suggestedTargetInstallDir,
    [suggestedTargetInstallDir, targetInstallDir],
  );

  const handleRunPrecheck = async () => {
    if (!targetVersion) {
      toast.error(t('targetVersionRequired'));
      return;
    }
    setRunningPrecheck(true);
    try {
      const request = buildRequest();
      const result = await services.stUpgrade.runPrecheckSafe(request);
      if (!result.success || !result.data) {
        toast.error(result.error || t('precheckFailed'));
        return;
      }

      setPrecheck(result.data);
      setConfirmedRequest(request);
      patchStUpgradeSession(clusterId, {
        clusterId,
        request,
        precheck: result.data,
        plan: undefined,
        task: undefined,
      });
      if (result.data.ready) {
        toast.success(t('precheckReady'));
      } else {
        toast.warning(t('precheckUpdated'));
      }
    } finally {
      setRunningPrecheck(false);
    }
  };

  const handleInitClusterConfigs = async () => {
    const initTarget = precheck?.node_targets?.[0];
    if (!initTarget?.source_install_dir) {
      toast.error(t('initClusterConfigsUnavailable'));
      return;
    }

    setInitializingConfigs(true);
    try {
      const result = await services.config.initClusterConfigsSafe(
        clusterId,
        initTarget.host_id,
        initTarget.source_install_dir,
      );
      if (!result.success) {
        toast.error(result.error || t('initClusterConfigsFailed'));
        return;
      }

      toast.success(t('initClusterConfigsSuccess'));
      await handleRunPrecheck();
    } finally {
      setInitializingConfigs(false);
    }
  };

  const handleContinue = () => {
    if (!precheck) {
      toast.error(t('runPrecheckFirst'));
      return;
    }
    if (!hasExplicitTargetJdks(precheck, confirmedRequest)) {
      toast.error(t('targetJdkRequired'));
      return;
    }
    if (isPrecheckStale) {
      toast.error(t('continueRequiresLatestPrecheck'));
      return;
    }
    router.push(`/clusters/${clusterId}/upgrade/config`);
  };

  const handleClearDraft = () => {
    clearStUpgradeSession(clusterId);
    setPrecheck(null);
    setPackageChecksum('');
    setConnectorNamesText('');
    const nextVersion =
      resolveSeatunnelVersion(availableVersions || undefined) ||
      cluster?.version ||
      '';
    setTargetVersion(nextVersion);
    setTargetInstallDir(
      buildSuggestedTargetInstallDir(
        cluster?.install_dir,
        cluster?.version,
        nextVersion,
      ),
    );
    setTargetInstallDirTouched(false);
    setNodeJavaHomes({});
    setConfirmedRequest(null);
    toast.success(t('draftCleared'));
  };

  const issuesByCategory = useMemo(() => {
    const groups: Record<string, BlockingIssue[]> = {};
    (precheck?.issues || []).forEach((issue) => {
      if (!groups[issue.category]) {
        groups[issue.category] = [];
      }
      groups[issue.category].push(issue);
    });
    return groups;
  }, [precheck]);

  const blockingIssues = useMemo(
    () => (precheck?.issues || []).filter((issue) => issue.blocking),
    [precheck],
  );

  const hasJdkNodeIssues = useMemo(
    () =>
      (issuesByCategory.node || []).some((issue) =>
        JDK_NODE_ISSUE_CODES.has(issue.code),
      ),
    [issuesByCategory.node],
  );
  const hasNonJdkNodeIssues = useMemo(
    () =>
      (issuesByCategory.node || []).some(
        (issue) => !JDK_NODE_ISSUE_CODES.has(issue.code),
      ),
    [issuesByCategory.node],
  );

  const hasConfigMissingIssue = useMemo(
    () =>
      blockingIssues.some(
        (issue) =>
          issue.category === 'config' && issue.code === 'config_missing',
      ),
    [blockingIssues],
  );

  const canContinueThroughConfig = useCallback(
    (issue: BlockingIssue) =>
      issue.category === 'config' &&
      issue.code !== 'config_missing' &&
      issue.code !== 'config_merge_plan_missing',
    [],
  );

  const isPrecheckStale = useMemo(() => {
    if (!precheck || !confirmedRequest) {
      return false;
    }

    return !isSameUpgradeRequest(currentDraftRequest, confirmedRequest);
  }, [confirmedRequest, currentDraftRequest, precheck]);

  const missingDraftTargetJdks =
    Boolean(precheck) && !hasExplicitTargetJdks(precheck, currentDraftRequest);
  const hasConfirmedTargetJdks = hasExplicitTargetJdks(
    precheck,
    confirmedRequest,
  );

  const canContinueToConfig = useMemo(
    () =>
      Boolean(precheck) &&
      !isPrecheckStale &&
      hasConfirmedTargetJdks &&
      !blockingIssues.some((issue) => !canContinueThroughConfig(issue)),
    [
      blockingIssues,
      canContinueThroughConfig,
      hasConfirmedTargetJdks,
      isPrecheckStale,
      precheck,
    ],
  );

  // 「进入配置合并页」不可用时的原因 / Why continuing to the merge page is blocked
  // 能继续时为 null；aria-disabled + Tooltip 共用同一个按钮
  // null when continuing is allowed; the same button takes aria-disabled + tooltip otherwise
  const continueBlockedReason = canContinueToConfig
    ? null
    : !precheck
      ? t('runPrecheckFirst')
      : missingDraftTargetJdks
        ? t('targetJdkRequired')
        : isPrecheckStale
          ? t('draftChangedNoticeTitle')
          : t('continueBlockedHint');

  // 零塌陷加载：首屏骨架屏占位 / Zero-collapse loading skeleton
  if (loading && !cluster) {
    return (
      <div
        className='upgrade-page arc-compact'
        data-testid='upgrade-prepare-page'
      >
        <WorkspaceHeader
          breadcrumbs={[
            {
              label: t('clusterDetailBreadcrumb'),
              href: `/clusters/${clusterId}`,
            },
            {
              label: t('prepareTitle'),
            },
          ]}
          icon={<FileSearch />}
          title={t('prepareTitle')}
          subtitle={<span>{t('prepareDescription')}</span>}
        />
        <UpgradeSteps current='prepare' clusterId={clusterId} />
        <div className='grid gap-4 xl:grid-cols-[400px_minmax(0,1fr)]'>
          <div className='rounded-(--radius-panel) border border-(--border) bg-card p-5 space-y-4'>
            <Skeleton className='h-5 w-40' />
            <div className='space-y-3 border-t-[0.5px] border-border/80 pt-4'>
              <Skeleton className='h-4 w-28' />
              <Skeleton className='h-9 w-full' />
              <Skeleton className='h-4 w-28' />
              <Skeleton className='h-9 w-full' />
            </div>
            <div className='flex gap-2 border-t-[0.5px] border-border/80 pt-4'>
              <Skeleton className='h-8 w-24' />
              <Skeleton className='h-8 w-32' />
            </div>
          </div>
          <div className='rounded-(--radius-panel) border border-(--border) bg-card p-5 space-y-4'>
            <Skeleton className='h-5 w-40' />
            <div className='border-t-[0.5px] border-border/80 pt-4'>
              <Skeleton className='h-36 w-full' />
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      className='upgrade-page arc-compact'
      data-testid='upgrade-prepare-page'
    >
      <WorkspaceHeader
        breadcrumbs={[
          {
            label: t('clusterDetailBreadcrumb'),
            href: `/clusters/${clusterId}`,
          },
          {
            label: t('prepareTitle'),
          },
        ]}
        icon={<FileSearch />}
        title={t('prepareTitle')}
        subtitle={
          <span className='inline-flex flex-wrap items-center gap-1.5'>
            <span className='tabular-nums'>
              {cluster
                ? `${cluster.name} · ${t('currentVersion')}: ${cluster.version || '-'}`
                : t('prepareStage')}
            </span>
            <HelpHint
              label={t('prepareTitle')}
              content={t('prepareDescription')}
            />
          </span>
        }
        actions={
          <div className='flex flex-wrap items-center gap-2'>
            {/* 返回和清空是一组次要动作；继续单独用主按钮，禁用原因留在 tooltip。 */}
            {/* Back and clear share one group. Continue stays a primary button so the blocked reason can sit on a tooltip. */}
            <ButtonGroup
              label={t('upgradeStagesAria')}
              size='sm'
              items={[
                {
                  id: 'back',
                  label: t('backToCluster'),
                  icon: <ArrowLeft />,
                  onSelect: () => router.push(`/clusters/${clusterId}`),
                },
                {
                  id: 'clear',
                  label: t('clearDraft'),
                  onSelect: handleClearDraft,
                },
              ]}
            />
            <DisabledReasonTooltip reason={continueBlockedReason}>
              <Button
                size='sm'
                data-testid='upgrade-prepare-continue'
                aria-disabled={continueBlockedReason ? 'true' : undefined}
                onClick={(event) => {
                  if (continueBlockedReason) {
                    event.preventDefault();
                    return;
                  }
                  handleContinue();
                }}
              >
                {t('continueToConfig')}
              </Button>
            </DisabledReasonTooltip>
          </div>
        }
      />

      <UpgradeSteps current='prepare' clusterId={clusterId} />

      {isPrecheckStale ? (
        <ArcAlert tone='warning' title={t('draftChangedNoticeTitle')}>
          {t('draftChangedNoticeDescription')}
        </ArcAlert>
      ) : null}

      <div className='grid items-start gap-4 xl:grid-cols-[400px_minmax(0,1fr)]'>
        {/* 左侧参数输入单层扁平画布 / Left parameter input flat single-layer surface */}
        <section className='overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'>
          <div className='px-5 py-4'>
            <h2 className='flex items-center gap-1.5 text-sm font-semibold text-foreground'>
              {t('upgradeInput')}
              <HelpHint
                label={t('upgradeInput')}
                content={t('upgradeInputDescription')}
              />
            </h2>
          </div>

          <div className='space-y-4 border-t-[0.5px] border-border/80 px-5 py-4'>
            <Select
              id='target-version'
              data-testid='upgrade-prepare-target-version'
              label={t('targetVersion')}
              value={targetVersion}
              onValueChange={setTargetVersion}
              placeholder={t('targetVersionPlaceholder')}
              options={(availableVersions?.versions || []).map((version) => ({
                value: version,
                label: version,
              }))}
            />

            <Input
              id='target-install-dir'
              label={t('targetInstallDir')}
              description={t('targetInstallDirDescription')}
              data-testid='upgrade-prepare-target-install-dir'
              className='font-mono text-xs'
              value={targetInstallDir}
              onChange={(event) => {
                setTargetInstallDir(event.target.value);
                setTargetInstallDirTouched(true);
              }}
              placeholder={t('targetInstallDirPlaceholder')}
            />

            {/* 高级选项折叠：用 0.5px 细线自然分隔，不套子框 / Advanced options collapsible separated by 0.5px hairline */}
            <div className='border-t-[0.5px] border-border/80 pt-3'>
              <button
                type='button'
                className='flex w-full items-center justify-between gap-2 py-1 text-left text-xs font-medium text-muted-foreground transition-colors hover:text-foreground'
                onClick={() => setAdvancedExpanded((prev) => !prev)}
                aria-expanded={advancedExpanded}
                data-testid='upgrade-prepare-advanced-toggle'
              >
                <span className='inline-flex items-center gap-1.5'>
                  {advancedExpanded ? (
                    <ChevronDown className='size-3.5' aria-hidden />
                  ) : (
                    <ChevronRight className='size-3.5' aria-hidden />
                  )}
                  {t('advancedOptions')}
                </span>
              </button>
              {advancedExpanded ? (
                <div className='mt-3 space-y-3'>
                  <Input
                    id='package-checksum'
                    label={t('packageChecksum')}
                    className='font-mono text-xs'
                    value={packageChecksum}
                    onChange={(event) =>
                      setPackageChecksum(event.target.value)
                    }
                    placeholder={t('packageChecksumPlaceholder')}
                  />
                  <Input
                    id='connector-names'
                    label={t('connectorNames')}
                    className='font-mono text-xs'
                    value={connectorNamesText}
                    onChange={(event) =>
                      setConnectorNamesText(event.target.value)
                    }
                    placeholder={t('connectorNamesPlaceholder')}
                  />
                </div>
              ) : null}
            </div>

            <div className='flex flex-wrap items-center gap-2 border-t-[0.5px] border-border/80 pt-4'>
              <Button
                variant='secondary'
                size='sm'
                onClick={() => void loadInitialData()}
                disabled={loading}
              >
                <RefreshCw className='size-3.5' aria-hidden />
                {commonT('refresh')}
              </Button>
              <Button
                size='sm'
                onClick={handleRunPrecheck}
                data-testid='upgrade-prepare-run-precheck'
                loading={runningPrecheck}
                disabled={loading}
              >
                <ShieldAlert className='size-3.5' aria-hidden />
                {runningPrecheck
                  ? t('runningPrecheck')
                  : precheck
                    ? t('rerunPrecheck')
                    : t('runPrecheck')}
              </Button>
            </div>
          </div>
        </section>

        {/* 右侧结果区压平为单层扁平画布，以 0.5px 细线分隔摘要 / 阻断项 / 节点目标 */}
        {/* Right results flattened into one single-layer card divided by 0.5px hairlines */}
        <section className='relative overflow-hidden rounded-(--radius-panel) border border-(--border) bg-card'>
          <TableLoadingBar loading={runningPrecheck} />

          {/* 1. 预检摘要 / Precheck summary */}
          <div className='px-6 py-5' data-testid='upgrade-prepare-summary'>
            <div className='flex flex-wrap items-center justify-between gap-2 pb-4'>
              <h2 className='text-sm font-semibold text-foreground'>
                {t('precheckSummary')}
              </h2>
              {precheck ? (
                <div className='flex flex-wrap items-center gap-3 text-xs text-muted-foreground tabular-nums'>
                  <Badge
                    data-testid='upgrade-prepare-ready-badge'
                    tone={precheck.ready ? 'success' : 'danger'}
                    outline
                    dot
                    size='sm'
                  >
                    {precheck.ready ? t('readyToPlan') : t('blocked')}
                  </Badge>
                  <span>
                    {t('issueCount')}: {precheck.issues.length}
                  </span>
                  <span>
                    {t('nodeTargetCount')}: {precheck.node_targets.length}
                  </span>
                </div>
              ) : null}
            </div>

            {!precheck ? (
              <div className='border-t-[0.5px] border-border/80 pt-4'>
                <EmptyState
                  icon={<FileSearch />}
                  title={loading ? t('loadingPrepare') : t('runPrecheckFirst')}
                  description={t('prepareDescription')}
                />
              </div>
            ) : (
              <div className='space-y-5 border-t-[0.5px] border-border/80 pt-5'>
                <div className='grid gap-6 xl:grid-cols-2 xl:gap-0 xl:divide-x-[0.5px] xl:divide-border/80'>
                  <SummarySection
                    icon={<Package className='size-4 text-primary' aria-hidden />}
                    title={t('packageInfo')}
                    description={precheck.package_manifest?.file_name || '-'}
                    lines={[
                      `${t('targetVersion')}: ${precheck.target_version}`,
                      `${t('checksum')}: ${precheck.package_manifest?.checksum || '-'}`,
                      `${t('packageArch')}: ${precheck.package_manifest?.arch || '-'}`,
                    ]}
                    className='xl:pr-6'
                  />
                  <SummarySection
                    icon={<PlugZap className='size-4 text-primary' aria-hidden />}
                    title={t('connectorInfo')}
                    description={`${precheck.connector_manifest?.connectors.length || 0} / ${precheck.connector_manifest?.libraries.length || 0} / ${precheck.connector_manifest?.plugin_dependencies.length || 0}`}
                    lines={[
                      `${t('connectorCount')}: ${precheck.connector_manifest?.connectors.length || 0}`,
                      `${t('libraryCount')}: ${precheck.connector_manifest?.libraries.length || 0}`,
                      `${t('pluginDependencyCount')}: ${precheck.connector_manifest?.plugin_dependencies.length || 0}`,
                      `${t('configConflicts')}: ${precheck.config_merge_plan?.conflict_count || 0}`,
                    ]}
                    className='xl:pl-6'
                  />
                </div>

                <div className='space-y-2 border-t-[0.5px] border-border/80 pt-4 text-xs'>
                  <div className='flex flex-wrap items-center gap-x-5 gap-y-1.5'>
                    <div className='inline-flex items-center gap-1.5 font-medium text-foreground'>
                      {t('pluginUpgradeBehaviorTitle')}
                      <HelpHint
                        label={t('pluginUpgradeBehaviorTitle')}
                        content={t('pluginUpgradeBehaviorTip')}
                        data-testid='upgrade-prepare-plugin-behavior-hint'
                      />
                    </div>
                    <div className='inline-flex items-center gap-1.5 font-medium text-foreground'>
                      {t('upgradeSmokeCheckTitle')}
                      <HelpHint
                        label={t('upgradeSmokeCheckTitle')}
                        content={t('upgradeSmokeCheckDescription')}
                        data-testid='upgrade-prepare-smoke-check-hint'
                      />
                    </div>
                  </div>
                  {issuesByCategory.connector ? (
                    <p
                      className='flex items-center gap-1.5 text-amber-700 dark:text-amber-300'
                      data-testid='upgrade-prepare-plugin-missing-warning'
                    >
                      <AlertTriangle className='size-3.5 shrink-0' aria-hidden />
                      {t('pluginPackageMissingWarning')}
                    </p>
                  ) : null}
                </div>
              </div>
            )}
          </div>

          {/* 2. 预检问题 / Precheck issues */}
          {precheck ? (
            <div
              className='border-t-[0.5px] border-border/80 px-6 py-5'
              data-testid='upgrade-prepare-issues'
            >
              <h2 className='pb-3 text-sm font-semibold text-foreground'>
                {t('issuesTitle')}
              </h2>

              {precheck.issues.length === 0 ? (
                <div className='flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-300'>
                  <CheckCircle2 className='size-4 shrink-0' aria-hidden />
                  <span>{t('noIssues')}</span>
                </div>
              ) : (
                <div className='divide-y-[0.5px] divide-border/80'>
                  {Object.entries(issuesByCategory).map(
                    ([category, issues]) => (
                      <div key={category} className='py-3 first:pt-0 last:pb-0'>
                        <div className='flex items-center justify-between gap-3 pb-2'>
                          <span className='text-xs font-semibold text-foreground'>
                            {getIssueCategoryLabel(
                              category as BlockingIssue['category'],
                            )}
                          </span>
                          <Badge tone='danger' outline size='sm'>
                            {issues.length}
                          </Badge>
                        </div>
                        <ul className='divide-y-[0.5px] divide-border/70 text-xs text-muted-foreground'>
                          {issues.map((issue, idx) => (
                            <li
                              key={`${issue.category}-${issue.code}-${idx}`}
                              className='py-2 first:pt-0 last:pb-0'
                            >
                              <div className='font-mono font-medium text-foreground'>
                                {issue.code}
                              </div>
                              <div className='mt-0.5 leading-relaxed'>
                                {issue.message}
                              </div>
                            </li>
                          ))}
                        </ul>
                      </div>
                    ),
                  )}
                </div>
              )}

              {hasJdkNodeIssues ? (
                <p
                  className='mt-3 text-xs text-muted-foreground'
                  data-testid='upgrade-prepare-jdk-issue-hint'
                >
                  {t('jdkIssueHint')}
                </p>
              ) : null}

              {!precheck.ready ? (
                <div className='mt-4 flex flex-wrap gap-2'>
                  {issuesByCategory.package ? (
                    <Button
                      variant='secondary'
                      size='sm'
                      onClick={() => router.push('/packages')}
                      data-testid='upgrade-prepare-go-packages'
                    >
                      <Package className='size-3.5' aria-hidden />
                      {t('goPackages')}
                    </Button>
                  ) : null}
                  {issuesByCategory.connector ? (
                    <Button
                      variant='secondary'
                      size='sm'
                      onClick={() => router.push('/plugins')}
                      data-testid='upgrade-prepare-go-plugins'
                    >
                      <PlugZap className='size-3.5' aria-hidden />
                      {t('goPlugins')}
                    </Button>
                  ) : null}
                  {hasNonJdkNodeIssues ? (
                    <Button
                      variant='secondary'
                      size='sm'
                      onClick={() => router.push(`/clusters/${clusterId}`)}
                      data-testid='upgrade-prepare-resolve-node-issues'
                    >
                      <ServerCog className='size-3.5' aria-hidden />
                      {t('resolveNodeIssues')}
                    </Button>
                  ) : null}
                  {issuesByCategory.config && hasConfigMissingIssue ? (
                    <Button
                      variant='secondary'
                      size='sm'
                      onClick={() => void handleInitClusterConfigs()}
                      data-testid='upgrade-prepare-init-configs'
                      loading={initializingConfigs}
                      disabled={
                        runningPrecheck ||
                        !precheck?.node_targets?.[0]?.source_install_dir
                      }
                    >
                      {initializingConfigs
                        ? t('initializingClusterConfigs')
                        : t('initClusterConfigs')}
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </div>
          ) : null}

          {/* 3. 节点目标矩阵 / Node target matrix */}
          {precheck?.node_targets?.length ? (
            <div
              className='border-t-[0.5px] border-border/80 px-6 py-5'
              data-testid='upgrade-prepare-node-targets'
            >
              <h2 className='flex items-center gap-1.5 pb-3 text-sm font-semibold text-foreground'>
                {t('nodeTargets')}
                <HelpHint
                  label={t('nodeTargets')}
                  content={
                    isPrecheckStale
                      ? t('nodeTargetsPreviewDescription')
                      : t('nodeTargetsDescription')
                  }
                />
              </h2>
              <div className='overflow-x-auto'>
                <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
                  <TableHeader>
                    <TableRow className='hover:bg-transparent'>
                      <TableHead>{t('host')}</TableHead>
                      <TableHead>{t('role')}</TableHead>
                      <TableHead>{t('sourceVersion')}</TableHead>
                      <TableHead>{t('targetVersion')}</TableHead>
                      <TableHead>{t('targetDir')}</TableHead>
                      <TableHead>
                        <span className='inline-flex items-center gap-1'>
                          {t('targetJdk')}
                          <HelpHint
                            label={t('targetJdk')}
                            content={t('targetJdkHint')}
                          />
                        </span>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {precheck.node_targets.map((target) => {
                      const displayTargetDir = isPrecheckStale
                        ? draftTargetInstallDirPreview
                        : target.target_install_dir;
                      return (
                        <TableRow key={`${target.host_id}-${target.role}`}>
                          <TableCell className='font-medium whitespace-nowrap'>
                            {target.host_name}
                          </TableCell>
                          <TableCell className='whitespace-nowrap'>
                            {target.role}
                          </TableCell>
                          <TableCell className='font-mono text-xs tabular-nums whitespace-nowrap'>
                            {target.source_version}
                          </TableCell>
                          <TableCell
                            className={`font-mono text-xs tabular-nums whitespace-nowrap ${
                              isPrecheckStale
                                ? 'text-amber-700 dark:text-amber-300'
                                : ''
                            }`}
                          >
                            {isPrecheckStale
                              ? currentDraftRequest.target_version
                              : target.target_version}
                          </TableCell>
                          <TableCell
                            className={`max-w-[220px] truncate font-mono text-xs ${
                              isPrecheckStale
                                ? 'text-amber-700 dark:text-amber-300'
                                : ''
                            }`}
                            title={displayTargetDir}
                          >
                            {displayTargetDir}
                          </TableCell>
                          <TableCell className='min-w-[18rem] align-top'>
                            <div
                              data-testid={`upgrade-prepare-node-jdk-${target.cluster_node_id}`}
                            >
                              <JavaHomePicker
                                id={`upgrade-target-jdk-${target.cluster_node_id}`}
                                hostId={target.host_id}
                                hideLabel
                                autoSelectFirst={false}
                                value={
                                  nodeJavaHomes[
                                    String(target.cluster_node_id)
                                  ] ?? ''
                                }
                                onChange={({javaHome}) =>
                                  setNodeJavaHomes((prev) => ({
                                    ...prev,
                                    [String(target.cluster_node_id)]: javaHome,
                                  }))
                                }
                              />
                            </div>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            </div>
          ) : null}
        </section>
      </div>
    </div>
  );
}

interface SummarySectionProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  lines: string[];
  className?: string;
}

function SummarySection({
  icon,
  title,
  description,
  lines,
  className,
}: SummarySectionProps) {
  return (
    <div className={className}>
      <div className='flex items-center gap-2 text-xs font-semibold text-foreground'>
        {icon}
        {title}
      </div>
      <div
        className='mt-1.5 truncate font-mono text-sm font-medium text-foreground tabular-nums'
        title={description}
      >
        {description}
      </div>
      <div className='mt-2.5 space-y-1 text-xs text-muted-foreground tabular-nums'>
        {lines.map((line) => (
          <div key={line} className='truncate' title={line}>
            {line}
          </div>
        ))}
      </div>
    </div>
  );
}

function buildSuggestedTargetInstallDir(
  currentInstallDir?: string,
  sourceVersion?: string,
  targetVersion?: string,
): string {
  const nextVersion = (targetVersion || '').trim();
  if (!nextVersion) {
    return '';
  }

  const currentDir = (currentInstallDir || '').trim();
  const currentVersion = (sourceVersion || '').trim();
  if (currentDir && currentVersion && currentDir.includes(currentVersion)) {
    return currentDir.replace(currentVersion, nextVersion);
  }

  return `/opt/seatunnel-${nextVersion}`;
}

function isSameUpgradeRequest(
  left: CreatePlanRequest,
  right: CreatePlanRequest,
): boolean {
  return (
    JSON.stringify(normalizeUpgradeRequest(left)) ===
    JSON.stringify(normalizeUpgradeRequest(right))
  );
}

function normalizeUpgradeRequest(request: CreatePlanRequest) {
  return {
    cluster_id: request.cluster_id,
    target_version: request.target_version?.trim() || '',
    target_install_dir: request.target_install_dir?.trim() || '',
    package_checksum: request.package_checksum?.trim() || '',
    connector_names: [...(request.connector_names || [])]
      .map((name) => name.trim())
      .filter(Boolean)
      .sort(),
    node_java_homes: compactNodeJavaHomes(request.node_java_homes || {}),
  };
}

const JDK_NODE_ISSUE_CODES = new Set([
  'jdk_below_required',
  'java_unusable',
  'jdk_unusual_version',
  'jdk_path_invalid',
  'java_check_failed',
]);

function compactNodeJavaHomes(
  homes: Record<string, string>,
): Record<string, string> | undefined {
  const next: Record<string, string> = {};
  for (const key of Object.keys(homes).sort()) {
    const value = (homes[key] ?? '').trim();
    if (value) {
      next[key] = value;
    }
  }
  return Object.keys(next).length > 0 ? next : undefined;
}

function hasExplicitTargetJdks(
  precheck: PrecheckResult | null,
  request: CreatePlanRequest | null,
): boolean {
  if (!precheck?.node_targets?.length || !request) {
    return false;
  }
  const homes = compactNodeJavaHomes(request.node_java_homes || {});
  if (!homes) {
    return false;
  }
  return precheck.node_targets.every((target) =>
    Boolean(homes[String(target.cluster_node_id)]),
  );
}
