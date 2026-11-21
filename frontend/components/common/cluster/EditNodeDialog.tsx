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

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {AlertCircle, CheckCircle2, Loader2, XCircle} from 'lucide-react';

import services from '@/lib/services';
import {Button} from '@/components/arc/button/button';
import {Switch} from '@/components/arc/switch/switch';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {HelpHint} from '@/components/common/HelpHint';
import {JavaHomePicker} from '@/components/common/java/JavaHomePicker';
import {
  ClusterConfig,
  DefaultPorts,
  DeploymentMode,
  NodeInfo,
  NodeRole,
  PrecheckCheckItem,
  PrecheckResult,
  buildNodeJVMOverride,
  getClusterPortDefaultsForRole,
  getClusterJVMConfig,
  getClusterJVMValueForRole,
  getNodeJVMOverrideValue,
} from '@/lib/services/cluster/types';
import {buildResolvedClusterConfigFromConfigs} from './nodeConfigDefaults';

interface EditNodeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  node: NodeInfo | null;
  deploymentMode: DeploymentMode;
  clusterConfig?: ClusterConfig;
  clusterInstallDir?: string;
  onSuccess: () => void;
}

function getRoleTranslationKey(
  role: string,
): 'master' | 'worker' | 'masterWorker' | 'undefined' {
  if (role === NodeRole.MASTER) {
    return 'master';
  }
  if (role === NodeRole.WORKER) {
    return 'worker';
  }
  if (role === NodeRole.MASTER_WORKER) {
    return 'masterWorker';
  }
  return 'undefined';
}

function getPrimaryPortLabelKey(
  role: NodeRole,
): 'hazelcastMasterPort' | 'hazelcastWorkerPort' {
  return role === NodeRole.WORKER
    ? 'hazelcastWorkerPort'
    : 'hazelcastMasterPort';
}

function getPrimaryPortRequiredMessageKey(
  role: NodeRole,
): 'cluster.hazelcastPortRequired' | 'cluster.workerPortRequired' {
  return role === NodeRole.WORKER
    ? 'cluster.workerPortRequired'
    : 'cluster.hazelcastPortRequired';
}

function resolveNodeInstallDir(
  node: NodeInfo,
  clusterInstallDir?: string,
): string {
  if (node.install_dir?.trim()) {
    return node.install_dir.trim();
  }
  if (clusterInstallDir?.trim()) {
    return clusterInstallDir.trim();
  }
  return '/opt/seatunnel';
}

export function EditNodeDialog({
  open,
  onOpenChange,
  node,
  deploymentMode,
  clusterConfig,
  clusterInstallDir,
  onSuccess,
}: EditNodeDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);
  const [precheckLoading, setPrecheckLoading] = useState(false);
  const [precheckResult, setPrecheckResult] = useState<PrecheckResult | null>(
    null,
  );
  const [resolvedClusterConfig, setResolvedClusterConfig] = useState<
    ClusterConfig | undefined
  >(clusterConfig);

  const [installDir, setInstallDir] = useState('');
  const [javaHome, setJavaHome] = useState('');
  const [hazelcastPort, setHazelcastPort] = useState(0);
  const [apiPort, setApiPort] = useState(0);
  const [jvmOverrideEnabled, setJvmOverrideEnabled] = useState(false);
  const [jvmHeapSize, setJvmHeapSize] = useState(0);

  useEffect(() => {
    if (!node || !open) {
      return;
    }
    let cancelled = false;

    const initialize = async () => {
      let effectiveClusterConfig = clusterConfig;
      if (!getClusterJVMConfig(clusterConfig)) {
        const configsResult = await services.config.getClusterConfigsSafe(
          node.cluster_id,
        );
        if (configsResult.success && configsResult.data) {
          effectiveClusterConfig = buildResolvedClusterConfigFromConfigs(
            clusterConfig,
            deploymentMode,
            configsResult.data,
          );
        }
      }

      if (cancelled) {
        return;
      }

      const clusterPortDefaults = getClusterPortDefaultsForRole(
        node.role,
        effectiveClusterConfig,
      );
      setResolvedClusterConfig(effectiveClusterConfig);
      setInstallDir(resolveNodeInstallDir(node, clusterInstallDir));
      setJavaHome(node.java_home?.trim() ?? '');
      setHazelcastPort(
        node.hazelcast_port ||
          clusterPortDefaults.hazelcastPort ||
          (node.role === NodeRole.WORKER
            ? DefaultPorts.WORKER_HAZELCAST
            : DefaultPorts.MASTER_HAZELCAST),
      );
      setApiPort(
        node.role === NodeRole.WORKER
          ? 0
          : node.api_port ||
              clusterPortDefaults.apiPort ||
              DefaultPorts.MASTER_API,
      );

      const currentOverride = getNodeJVMOverrideValue(
        node.role,
        node.overrides,
      );
      const inheritedValue = getClusterJVMValueForRole(
        node.role,
        effectiveClusterConfig,
      );
      setJvmOverrideEnabled(currentOverride !== undefined);
      setJvmHeapSize(currentOverride ?? inheritedValue ?? 0);
      setPrecheckResult(null);
    };

    void initialize();

    return () => {
      cancelled = true;
    };
  }, [node, open, clusterConfig, clusterInstallDir, deploymentMode]);

  const handlePrecheck = async () => {
    if (!node) {
      return;
    }
    if (!hazelcastPort || hazelcastPort <= 0) {
      toast.error(t(getPrimaryPortRequiredMessageKey(node.role)));
      return;
    }

    setPrecheckLoading(true);
    setPrecheckResult(null);
    try {
      const result = await services.cluster.precheckNodeSafe(node.cluster_id, {
        host_id: node.host_id,
        role: node.role,
        install_dir:
          installDir.trim() || resolveNodeInstallDir(node, clusterInstallDir),
        hazelcast_port: hazelcastPort,
        api_port:
          node.role === NodeRole.WORKER ? undefined : apiPort || undefined,
      });

      if (result.success && result.data) {
        setPrecheckResult(result.data);
        if (result.data.success) {
          toast.success(t('cluster.precheckPassed'));
        } else {
          toast.warning(t('cluster.precheckFailed'));
        }
      } else {
        toast.error(result.error || t('cluster.precheckError'));
      }
    } finally {
      setPrecheckLoading(false);
    }
  };

  const persistNodeChanges = async () => {
    if (!node) {
      return;
    }
    if (!installDir.trim()) {
      toast.error(t('cluster.installDirRequired'));
      return;
    }
    if (!hazelcastPort || hazelcastPort <= 0) {
      toast.error(t(getPrimaryPortRequiredMessageKey(node.role)));
      return;
    }
    if (jvmOverrideEnabled && (!jvmHeapSize || jvmHeapSize <= 0)) {
      toast.error(t('cluster.jvmHeapSizeRequired'));
      return;
    }

    setLoading(true);
    try {
      const result = await services.cluster.updateNodeSafe(
        node.cluster_id,
        node.id,
        {
          install_dir: installDir.trim(),
          // 仅变化时提交：服务端会对新路径做 Agent 验活，未改时不下发可避免
          // 主机离线时仅改端口被 JDK 校验挡住。
          // Only sent when changed: the server verifies the new path via the Agent,
          // so omitting unchanged values keeps port-only edits unblocked on offline hosts.
          java_home:
            javaHome.trim() !== (node.java_home?.trim() ?? '')
              ? javaHome.trim()
              : undefined,
          hazelcast_port: hazelcastPort,
          api_port:
            node.role === NodeRole.WORKER ? undefined : apiPort || undefined,
          overrides: jvmOverrideEnabled
            ? buildNodeJVMOverride(node.role, jvmHeapSize)
            : {},
        },
      );

      if (!result.success) {
        toast.error(result.error || t('cluster.updateNodeError'));
        return;
      }

      onSuccess();
      onOpenChange(false);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async () => {
    await persistNodeChanges();
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'passed':
        return <CheckCircle2 className='h-4 w-4 text-green-500' />;
      case 'failed':
        return <XCircle className='h-4 w-4 text-red-500' />;
      default:
        return <AlertCircle className='h-4 w-4 text-yellow-500' />;
    }
  };

  if (!node) {
    return null;
  }

  const clusterDefaultHeap = getClusterJVMValueForRole(
    node.role,
    resolvedClusterConfig,
  );

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='arc-compact sm:max-w-[680px]'>
          <DialogHeader>
            <DialogTitle>{t('cluster.editNode')}</DialogTitle>
            <DialogDescription>
              {t('cluster.editNodeDescription', {
                host: node.host_name,
                ip: node.host_ip,
              })}
            </DialogDescription>
          </DialogHeader>

          <div className='max-h-[70vh] space-y-5 overflow-y-auto py-1 pr-1'>
            <dl className='grid grid-cols-2 gap-x-6 gap-y-3 border-b-[0.5px] border-border/80 pb-4 text-xs sm:grid-cols-4'>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>{t('cluster.hostName')}</dt>
                <dd className='truncate text-sm font-medium text-foreground'>
                  {node.host_name}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>{t('cluster.hostIP')}</dt>
                <dd className='truncate font-mono text-sm font-medium tabular-nums text-foreground'>
                  {node.host_ip}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>{t('cluster.nodeRole')}</dt>
                <dd className='truncate text-sm font-medium text-foreground'>
                  {t(`cluster.roles.${getRoleTranslationKey(node.role)}`)}
                </dd>
              </div>
              <div className='min-w-0 space-y-0.5'>
                <dt className='text-muted-foreground'>
                  {t('cluster.nodeStatus')}
                </dt>
                <dd className='truncate text-sm font-medium text-foreground'>
                  {t(`cluster.nodeStatuses.${node.status}`)}
                </dd>
              </div>
            </dl>

            <div className='space-y-2'>
              <Label>
                {t('cluster.installDir')}{' '}
                <span className='text-destructive'>*</span>
              </Label>
              <Input
                value={installDir}
                onChange={(e) => setInstallDir(e.target.value)}
                placeholder={t('cluster.installDirPlaceholder')}
                className='font-mono'
              />
            </div>

            {/* 节点 JDK 可编辑：升级为需要更高 JDK 的版本（≥3.1.0）时的换 JDK 出口。/ Editable node JDK: the remediation path when upgrading to a version needing a newer JDK (≥3.1.0). */}
            <JavaHomePicker
              hostId={node.host_id}
              value={javaHome}
              onChange={({javaHome: next}) => setJavaHome(next)}
            />

            <div className='space-y-3 border-t-[0.5px] border-border/80 pt-4'>
              <div className='flex items-center gap-2'>
                <Label>{t('cluster.portConfig')}</Label>
                <HelpHint
                  label={t('cluster.portConfig')}
                  content={t('cluster.portConfigDescription')}
                  data-testid='edit-node-port-config-hint'
                />
              </div>
              <div className='grid gap-4 md:grid-cols-3'>
                <div className='space-y-1'>
                  <Label className='text-xs'>
                    {t(`cluster.${getPrimaryPortLabelKey(node.role)}`)}{' '}
                    <span className='text-destructive'>*</span>
                  </Label>
                  <Input
                    type='number'
                    value={hazelcastPort}
                    onChange={(e) =>
                      setHazelcastPort(parseInt(e.target.value, 10) || 0)
                    }
                  />
                </div>

                {node.role !== NodeRole.WORKER && (
                  <div className='space-y-1'>
                    <Label className='text-xs'>
                      {t('cluster.apiPort')}{' '}
                      <span className='text-muted-foreground'>
                        ({t('common.optional')})
                      </span>
                    </Label>
                    <Input
                      type='number'
                      value={apiPort || ''}
                      onChange={(e) =>
                        setApiPort(parseInt(e.target.value, 10) || 0)
                      }
                    />
                  </div>
                )}
              </div>
            </div>

            <div className='space-y-3 border-t-[0.5px] border-border/80 pt-4'>
              <div className='flex items-center justify-between gap-4'>
                <div className='space-y-1'>
                  <Label>{t('cluster.jvmOverrideTitle')}</Label>
                  {!jvmOverrideEnabled && (
                    <p className='text-xs text-muted-foreground'>
                      {clusterDefaultHeap
                        ? t('cluster.jvmInheritDefault', {
                            value: clusterDefaultHeap,
                          })
                        : t('cluster.jvmNoClusterDefault')}
                    </p>
                  )}
                </div>
                <Switch
                  checked={jvmOverrideEnabled}
                  onCheckedChange={(checked) => {
                    setJvmOverrideEnabled(checked);
                    if (checked && jvmHeapSize <= 0) {
                      setJvmHeapSize(clusterDefaultHeap ?? 0);
                    }
                  }}
                />
              </div>

              {jvmOverrideEnabled && (
                <div className='max-w-xs space-y-2'>
                  <div className='flex items-center gap-2'>
                    <Label className='text-xs'>
                      {t('cluster.jvmHeapSize')}{' '}
                      <span className='text-destructive'>*</span>
                    </Label>
                    <HelpHint
                      label={t('cluster.jvmHeapSize')}
                      content={t('cluster.jvmHeapSizeHint')}
                      data-testid='edit-node-jvm-heap-hint'
                    />
                  </div>
                  <Input
                    type='number'
                    min={1}
                    value={jvmHeapSize || ''}
                    onChange={(e) =>
                      setJvmHeapSize(parseInt(e.target.value, 10) || 0)
                    }
                  />
                </div>
              )}
            </div>

            {precheckResult && (
              <div className='space-y-2 border-t-[0.5px] border-border/80 pt-4'>
                <div className='flex items-center gap-2'>
                  {precheckResult.success ? (
                    <CheckCircle2 className='h-4 w-4 text-emerald-500' />
                  ) : (
                    <XCircle className='h-4 w-4 text-rose-500' />
                  )}
                  <span className='text-sm font-medium'>
                    {precheckResult.success
                      ? t('cluster.precheckPassed')
                      : t('cluster.precheckFailed')}
                  </span>
                </div>
                <div className='space-y-1'>
                  {precheckResult.checks.map(
                    (check: PrecheckCheckItem, index: number) => (
                      <div
                        key={index}
                        className='flex items-start gap-2 text-xs'
                      >
                        {getStatusIcon(check.status)}
                        <div>
                          <span className='font-medium'>
                            {t(`cluster.precheckItems.${check.name}`, {
                              defaultValue: check.name,
                            })}
                            :
                          </span>{' '}
                          <span className='text-muted-foreground'>
                            {check.message}
                          </span>
                        </div>
                      </div>
                    ),
                  )}
                </div>
              </div>
            )}
          </div>

          <DialogFooter className='gap-2'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={() => onOpenChange(false)}
              disabled={loading || precheckLoading}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              onClick={handlePrecheck}
              disabled={loading}
              loading={precheckLoading}
            >
              {t('cluster.precheck')}
            </Button>
            <Button
              type='button'
              size='sm'
              onClick={handleSubmit}
              disabled={precheckLoading}
              loading={loading}
            >
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
