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
import {toast} from 'sonner';
import {
  ArrowLeft,
  FileSearch,
  Package,
  PlugZap,
  RefreshCw,
  ServerCog,
  ShieldAlert,
} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {WorkspaceHeader} from '@/components/common/layout';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Separator} from '@/components/ui/separator';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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
    if (draft.request?.target_install_dir) {
      setTargetInstallDir(draft.request.target_install_dir);
      setTargetInstallDirTouched(true);
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

  const buildRequest = useCallback(
    (): CreatePlanRequest => ({
      cluster_id: clusterId,
      target_version: targetVersion,
      target_install_dir: targetInstallDir.trim() || undefined,
      package_checksum: packageChecksum || undefined,
      connector_names: connectorNamesText
        .split(',')
        .map((name) => name.trim())
        .filter(Boolean),
    }),
    [
      clusterId,
      connectorNamesText,
      packageChecksum,
      targetInstallDir,
      targetVersion,
    ],
  );

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

  const canContinueToConfig = useMemo(
    () =>
      Boolean(precheck) &&
      !isPrecheckStale &&
      !blockingIssues.some((issue) => !canContinueThroughConfig(issue)),
    [blockingIssues, canContinueThroughConfig, isPrecheckStale, precheck],
  );

  return (
    <div className='space-y-6' data-testid='upgrade-prepare-page'>
      <WorkspaceHeader
        icon={<FileSearch />}
        title={t('prepareTitle')}
        subtitle={
          <>
            <span>{t('prepareDescription')}</span>
            {cluster ? (
              <span className='mt-0.5 block'>
                {cluster.name} · {t('currentVersion')}: {cluster.version || '-'}
              </span>
            ) : null}
          </>
        }
        actions={
          <>
            <Button
              variant='outline'
              onClick={() => router.push(`/clusters/${clusterId}`)}
            >
              <ArrowLeft className='mr-2 h-4 w-4' />
              {t('backToCluster')}
            </Button>
            <Button variant='outline' onClick={handleClearDraft}>
              {t('clearDraft')}
            </Button>
            <Button
              onClick={handleContinue}
              data-testid='upgrade-prepare-continue'
              disabled={!canContinueToConfig}
            >
              {t('continueToConfig')}
            </Button>
          </>
        }
      />

      {isPrecheckStale ? (
        <div className='rounded-lg border border-amber-500/30 bg-amber-500/5 p-4'>
          <div className='text-sm font-medium text-amber-700 dark:text-amber-300'>
            {t('draftChangedNoticeTitle')}
          </div>
          <div className='mt-1 text-sm text-muted-foreground'>
            {t('draftChangedNoticeDescription')}
          </div>
        </div>
      ) : null}

      <div className='grid gap-6 xl:grid-cols-[420px_minmax(0,1fr)]'>
        <Card>
          <CardHeader>
            <CardTitle>{t('upgradeInput')}</CardTitle>
            <CardDescription>{t('upgradeInputDescription')}</CardDescription>
          </CardHeader>
          <CardContent className='space-y-4'>
            <div className='space-y-2'>
              <Label htmlFor='target-version'>{t('targetVersion')}</Label>
              <Select value={targetVersion} onValueChange={setTargetVersion}>
                <SelectTrigger
                  id='target-version'
                  data-testid='upgrade-prepare-target-version'
                >
                  <SelectValue placeholder={t('targetVersionPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {(availableVersions?.versions || []).map((version) => (
                    <SelectItem key={version} value={version}>
                      {version}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='package-checksum'>{t('packageChecksum')}</Label>
              <Input
                id='package-checksum'
                value={packageChecksum}
                onChange={(event) => setPackageChecksum(event.target.value)}
                placeholder={t('packageChecksumPlaceholder')}
              />
            </div>

            <div className='space-y-2'>
              <Label htmlFor='target-install-dir'>
                {t('targetInstallDir')}
              </Label>
              <Input
                id='target-install-dir'
                data-testid='upgrade-prepare-target-install-dir'
                value={targetInstallDir}
                onChange={(event) => {
                  setTargetInstallDir(event.target.value);
                  setTargetInstallDirTouched(true);
                }}
                placeholder={t('targetInstallDirPlaceholder')}
              />
              <p className='text-xs text-muted-foreground'>
                {t('targetInstallDirDescription')}
              </p>
            </div>

            <div className='space-y-2'>
              <Label htmlFor='connector-names'>{t('connectorNames')}</Label>
              <Input
                id='connector-names'
                value={connectorNamesText}
                onChange={(event) => setConnectorNamesText(event.target.value)}
                placeholder={t('connectorNamesPlaceholder')}
              />
            </div>

            <Separator />

            <div className='grid gap-3 sm:grid-cols-2'>
              <Button
                variant='outline'
                onClick={() => void loadInitialData()}
                disabled={loading}
              >
                <RefreshCw className='mr-2 h-4 w-4' />
                {commonT('refresh')}
              </Button>
              <Button
                onClick={handleRunPrecheck}
                data-testid='upgrade-prepare-run-precheck'
                disabled={runningPrecheck || loading}
              >
                <ShieldAlert className='mr-2 h-4 w-4' />
                {runningPrecheck
                  ? t('checking')
                  : precheck
                    ? t('rerunPrecheck')
                    : t('runPrecheck')}
              </Button>
            </div>
          </CardContent>
        </Card>

        <div className='space-y-6'>
          <Card data-testid='upgrade-prepare-summary'>
            <CardHeader>
              <CardTitle>{t('precheckSummary')}</CardTitle>
              <CardDescription>
                {t('precheckSummaryDescription')}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              {!precheck ? (
                <div className='rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                  {loading ? t('loadingPrepare') : t('runPrecheckFirst')}
                </div>
              ) : (
                <>
                  <div className='flex flex-wrap items-center gap-2'>
                    <Badge
                      data-testid='upgrade-prepare-ready-badge'
                      variant={precheck.ready ? 'default' : 'destructive'}
                    >
                      {precheck.ready ? t('readyToPlan') : t('blocked')}
                    </Badge>
                    <span className='text-sm text-muted-foreground'>
                      {t('issueCount')}: {precheck.issues.length}
                    </span>
                    <span className='text-sm text-muted-foreground'>
                      {t('nodeTargetCount')}: {precheck.node_targets.length}
                    </span>
                  </div>

                  <div className='grid gap-4 xl:grid-cols-2'>
                    <SummaryCard
                      icon={<Package className='h-4 w-4 text-primary' />}
                      title={t('packageInfo')}
                      description={precheck.package_manifest?.file_name || '-'}
                      lines={[
                        `${t('targetVersion')}: ${precheck.target_version}`,
                        `${t('checksum')}: ${precheck.package_manifest?.checksum || '-'}`,
                        `${t('packageArch')}: ${precheck.package_manifest?.arch || '-'}`,
                      ]}
                    />
                    <SummaryCard
                      icon={<PlugZap className='h-4 w-4 text-primary' />}
                      title={t('connectorInfo')}
                      description={`${precheck.connector_manifest?.connectors.length || 0} / ${precheck.connector_manifest?.libraries.length || 0} / ${precheck.connector_manifest?.plugin_dependencies.length || 0}`}
                      lines={[
                        `${t('connectorCount')}: ${precheck.connector_manifest?.connectors.length || 0}`,
                        `${t('libraryCount')}: ${precheck.connector_manifest?.libraries.length || 0}`,
                        `${t('pluginDependencyCount')}: ${precheck.connector_manifest?.plugin_dependencies.length || 0}`,
                        `${t('configConflicts')}: ${precheck.config_merge_plan?.conflict_count || 0}`,
                      ]}
                    />
                  </div>

                  <div className='rounded-lg border border-dashed bg-muted/30 p-4 text-sm text-muted-foreground'>
                    <p className='font-medium text-foreground'>
                      {t('pluginUpgradeBehaviorTitle')}
                    </p>
                    <p className='mt-2'>
                      {t('pluginUpgradeBehaviorDescription')}
                    </p>
                    <p className='mt-3'>
                      <span className='font-medium text-foreground'>
                        {t('upgradeSmokeCheckTitle')}
                      </span>
                      {' · '}
                      {t('upgradeSmokeCheckDescription')}
                    </p>
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          {precheck ? (
            <Card data-testid='upgrade-prepare-issues'>
              <CardHeader>
                <CardTitle>{t('issuesTitle')}</CardTitle>
                <CardDescription>{t('issuesDescription')}</CardDescription>
              </CardHeader>
              <CardContent className='space-y-4'>
                {precheck.issues.length === 0 ? (
                  <div className='rounded-lg border border-dashed p-6 text-sm text-muted-foreground'>
                    {t('noIssues')}
                  </div>
                ) : (
                  <div className='space-y-4'>
                    {Object.entries(issuesByCategory).map(
                      ([category, issues]) => (
                        <div
                          key={category}
                          className='space-y-3 rounded-lg border p-4'
                        >
                          <div className='flex items-center justify-between gap-3'>
                            <div className='font-medium'>
                              {getIssueCategoryLabel(
                                category as BlockingIssue['category'],
                              )}
                            </div>
                            <Badge variant='destructive'>{issues.length}</Badge>
                          </div>
                          <ul className='space-y-2 text-sm text-muted-foreground'>
                            {issues.map((issue) => (
                              <li
                                key={`${issue.category}-${issue.code}`}
                                className='rounded-md bg-muted/40 p-3'
                              >
                                <div className='font-medium text-foreground'>
                                  {issue.code}
                                </div>
                                <div>{issue.message}</div>
                              </li>
                            ))}
                          </ul>
                        </div>
                      ),
                    )}
                  </div>
                )}

                {!precheck.ready ? (
                  <div className='flex flex-wrap gap-2'>
                    {issuesByCategory.package ? (
                      <Button
                        variant='outline'
                        onClick={() => router.push('/packages')}
                        data-testid='upgrade-prepare-go-packages'
                      >
                        <Package className='mr-2 h-4 w-4' />
                        {t('goPackages')}
                      </Button>
                    ) : null}
                    {issuesByCategory.connector ? (
                      <Button
                        variant='outline'
                        onClick={() => router.push('/plugins')}
                        data-testid='upgrade-prepare-go-plugins'
                      >
                        <PlugZap className='mr-2 h-4 w-4' />
                        {t('goPlugins')}
                      </Button>
                    ) : null}
                    {issuesByCategory.node ? (
                      <Button
                        variant='outline'
                        onClick={() => router.push(`/clusters/${clusterId}`)}
                        data-testid='upgrade-prepare-resolve-node-issues'
                      >
                        <ServerCog className='mr-2 h-4 w-4' />
                        {t('resolveNodeIssues')}
                      </Button>
                    ) : null}
                    {issuesByCategory.config && hasConfigMissingIssue ? (
                      <Button
                        variant='outline'
                        onClick={() => void handleInitClusterConfigs()}
                        data-testid='upgrade-prepare-init-configs'
                        disabled={
                          initializingConfigs ||
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
              </CardContent>
            </Card>
          ) : null}

          {precheck?.node_targets?.length ? (
            <Card data-testid='upgrade-prepare-node-targets'>
              <CardHeader>
                <CardTitle>{t('nodeTargets')}</CardTitle>
                <CardDescription>
                  {isPrecheckStale
                    ? t('nodeTargetsPreviewDescription')
                    : t('nodeTargetsDescription')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('host')}</TableHead>
                      <TableHead>{t('role')}</TableHead>
                      <TableHead>{t('sourceVersion')}</TableHead>
                      <TableHead>{t('targetVersion')}</TableHead>
                      <TableHead>{t('targetDir')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {precheck.node_targets.map((target) => (
                      <TableRow key={`${target.host_id}-${target.role}`}>
                        <TableCell>{target.host_name}</TableCell>
                        <TableCell>{target.role}</TableCell>
                        <TableCell>{target.source_version}</TableCell>
                        <TableCell
                          className={
                            isPrecheckStale
                              ? 'text-amber-700 dark:text-amber-300'
                              : ''
                          }
                        >
                          {isPrecheckStale
                            ? currentDraftRequest.target_version
                            : target.target_version}
                        </TableCell>
                        <TableCell
                          className={`font-mono text-xs ${isPrecheckStale ? 'text-amber-700 dark:text-amber-300' : ''}`}
                        >
                          {isPrecheckStale
                            ? draftTargetInstallDirPreview
                            : target.target_install_dir}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          ) : null}
        </div>
      </div>
    </div>
  );
}

interface SummaryCardProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  lines: string[];
}

function SummaryCard({icon, title, description, lines}: SummaryCardProps) {
  return (
    <div className='rounded-lg border p-4'>
      <div className='flex items-center gap-2 text-sm font-medium'>
        {icon}
        {title}
      </div>
      <div className='mt-2 text-sm text-foreground'>{description}</div>
      <div className='mt-3 space-y-1 text-xs text-muted-foreground'>
        {lines.map((line) => (
          <div key={line}>{line}</div>
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
  };
}
