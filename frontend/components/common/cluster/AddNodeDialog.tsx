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
import {toast} from 'sonner';
import {Loader2, RefreshCw, Server} from 'lucide-react';

import services from '@/lib/services';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Checkbox} from '@/components/ui/checkbox';
import {Input} from '@/components/ui/input';
import {JavaHomePicker} from '@/components/common/java/JavaHomePicker';
import {Label} from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {ToggleGroup, ToggleGroupItem} from '@/components/ui/toggle-group';
import {AgentStatus, HostInfo} from '@/lib/services/host/types';
import {
  ClusterConfig,
  DeploymentMode,
  NodeInfo,
  NodeRole,
  ScaleMode,
  ScaleNodePorts,
  ScaleOperationInfo,
  ScaleOutRequest,
  ScalePlan,
  getScaleInProgressDetail,
} from '@/lib/services/cluster';
import {ScalePlanView} from './scale/ScalePlanView';
import {ScaleOperationProgress} from './scale/ScaleOperationProgress';

interface AddNodeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  clusterId: number;
  deploymentMode: DeploymentMode;
  /** 兼容旧调用方，扩容计划由后端计算 / Kept for existing callers; the backend computes the plan */
  clusterConfig?: ClusterConfig;
  clusterInstallDir?: string;
  existingNodes?: NodeInfo[];
  /** 扩容成功后回调（刷新集群数据）/ Called after a successful scale-out (refresh cluster data) */
  onSuccess: () => void;
}

/** 向导步骤 / Wizard steps */
type WizardStep = 'host' | 'plan' | 'execute';

const WIZARD_STEPS: WizardStep[] = ['host', 'plan', 'execute'];

const EMPTY_PORTS: ScaleNodePorts = {
  hazelcast_port: 0,
  api_port: 0,
  worker_port: 0,
};

/**
 * 解析端口输入，非法或空值返回 0（交由后端取默认值）。
 * Parse a port input; invalid or empty values become 0 (the backend picks the default).
 */
function parsePort(value: string): number {
  const port = Number.parseInt(value, 10);
  return Number.isFinite(port) && port > 0 ? port : 0;
}

/**
 * 添加节点（扩容）向导：选择主机 → 自动识别安装/纳管并预检 → 执行进度。
 * 计划由后端计算（成员地址、回环修正、插件补齐、预检），前端只负责展示、收集确认与提交。
 * Add-node (scale-out) wizard: choose a host → auto-detect install/register and precheck → execution progress.
 * The backend computes the plan (members, loopback fix, plugin replay, prechecks); the UI only shows it,
 * collects acknowledgements and submits.
 */
export function AddNodeDialog({
  open,
  onOpenChange,
  clusterId,
  deploymentMode,
  onSuccess,
}: AddNodeDialogProps) {
  const t = useTranslations();
  const ts = useTranslations('cluster.scale');
  const isHybrid = deploymentMode === DeploymentMode.HYBRID;

  const [step, setStep] = useState<WizardStep>('host');
  const [hosts, setHosts] = useState<HostInfo[]>([]);
  const [loadingHosts, setLoadingHosts] = useState(false);
  const [hostId, setHostId] = useState('');
  const [role, setRole] = useState<NodeRole | ''>('');

  // 计划与配置 / Plan and configuration
  const [plan, setPlan] = useState<ScalePlan | null>(null);
  const [planLoading, setPlanLoading] = useState(false);
  const [planError, setPlanError] = useState('');
  const [mode, setMode] = useState<ScaleMode | ''>('');
  const [modeTouched, setModeTouched] = useState(false);
  const [installDir, setInstallDir] = useState('');
  const [javaHome, setJavaHome] = useState('');
  const [ports, setPorts] = useState<ScaleNodePorts>(EMPTY_PORTS);
  const [dirty, setDirty] = useState(false);
  const [restartAck, setRestartAck] = useState(false);
  // 入群后滚动重启现有节点，默认取计划推荐值。/ Rolling-restart existing nodes after the join; defaults to the plan's recommendation.
  const [rollingRestart, setRollingRestart] = useState(false);

  // 执行 / Execution
  const [submitting, setSubmitting] = useState(false);
  const [operationId, setOperationId] = useState<number | null>(null);
  const [initialOperation, setInitialOperation] =
    useState<ScaleOperationInfo | null>(null);

  const resetState = () => {
    setStep('host');
    setHostId('');
    setRole('');
    setPlan(null);
    setPlanLoading(false);
    setPlanError('');
    setMode('');
    setModeTouched(false);
    setInstallDir('');
    setJavaHome('');
    setPorts(EMPTY_PORTS);
    setDirty(false);
    setRestartAck(false);
    setRollingRestart(false);
    setSubmitting(false);
    setOperationId(null);
    setInitialOperation(null);
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    resetState();
    let cancelled = false;
    const loadHosts = async () => {
      setLoadingHosts(true);
      const result = await services.host.getHostsSafe({
        current: 1,
        size: 100,
        agent_status: AgentStatus.INSTALLED,
      });
      if (cancelled) {
        return;
      }
      if (result.success && result.data) {
        setHosts(result.data.hosts || []);
      } else {
        setHosts([]);
        toast.error(result.error || t('host.loadError'));
      }
      setLoadingHosts(false);
    };
    void loadHosts();
    return () => {
      cancelled = true;
    };
    // 仅在打开时初始化 / Initialize only when the dialog opens
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  /**
   * 组装扩容请求；full=false 时只带主机与角色，让后端自动识别模式并给出默认目录与端口。
   * Build the scale-out request; with full=false only host and role are sent so the backend detects the mode
   * and fills default directory and ports.
   */
  const buildRequest = (full: boolean): ScaleOutRequest => {
    const request: ScaleOutRequest = {host_id: Number(hostId)};
    if (!isHybrid && role) {
      request.role = role;
    }
    if (full) {
      if (modeTouched && mode) {
        request.mode = mode;
      }
      request.install_dir = installDir.trim() || undefined;
      request.ports = ports;
      request.java_home = javaHome.trim() || undefined;
    }
    return request;
  };

  const runPlan = async (full: boolean) => {
    setPlanLoading(true);
    setPlanError('');
    try {
      const next = await services.clusterScale.planScaleOut(
        clusterId,
        buildRequest(full),
      );
      setPlan(next);
      setMode(next.mode ?? next.detected_mode ?? 'install');
      setInstallDir(next.target.install_dir);
      setJavaHome(next.target.java_home || '');
      setPorts({
        hazelcast_port: next.target.hazelcast_port,
        api_port: next.target.api_port,
        worker_port: next.target.worker_port,
      });
      setDirty(false);
      setRestartAck(false);
      setRollingRestart(Boolean(next.rolling_restart?.recommended));
      setStep('plan');
    } catch (error) {
      setPlanError(error instanceof Error ? error.message : String(error));
    } finally {
      setPlanLoading(false);
    }
  };

  const markDirty = () => setDirty(true);

  const handleSubmit = async () => {
    if (!plan) {
      return;
    }
    setSubmitting(true);
    try {
      const result = await services.clusterScale.submitScaleOut(clusterId, {
        ...buildRequest(true),
        // 显式提交计划中的模式，保证与用户看到的计划一致。
        // Submit the plan's mode explicitly so it matches what the user saw.
        mode: plan.mode,
        plan_fingerprint: plan.fingerprint,
        loopback_restart_ack: restartAck,
        rolling_restart:
          rollingRestart && Boolean(plan.rolling_restart?.available),
      });
      setOperationId(result.operation_id);
      setInitialOperation(result.operation);
      setStep('execute');
    } catch (error) {
      const conflict = getScaleInProgressDetail(error);
      if (conflict) {
        toast.error(
          ts('inProgressConflict', {
            id: conflict.operation_id,
            kind: ts(`kind.${conflict.kind}`),
            user: conflict.created_by_name || `#${conflict.created_by}`,
          }),
        );
      } else {
        toast.error(error instanceof Error ? error.message : String(error));
        // 计划已变化时自动重新预检。/ Re-run the precheck automatically when the plan changed.
        const code = (error as {status?: number}).status;
        if (code === 409) {
          void runPlan(true);
        }
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleFinished = (operation: ScaleOperationInfo) => {
    if (operation.status === 'succeeded') {
      onSuccess();
    }
  };

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      resetState();
    }
    onOpenChange(next);
  };

  const canGoNext =
    !!hostId && (isHybrid || !!role) && !planLoading && hosts.length > 0;
  const canSubmit =
    !!plan &&
    plan.can_submit &&
    !dirty &&
    !planLoading &&
    !submitting &&
    (!plan.requires_restart_ack || restartAck);

  const detectionText = () => {
    if (!plan) {
      return '';
    }
    if (plan.detected_mode === 'register') {
      return ts('detectedRunning', {
        dir: plan.target.install_dir,
        role: plan.target.role,
      });
    }
    return ts('detectedNone');
  };

  const renderStepIndicator = () => (
    <ol className='flex flex-wrap items-center gap-2 text-xs'>
      {WIZARD_STEPS.map((item, index) => {
        const active = item === step;
        const done = WIZARD_STEPS.indexOf(step) > index;
        return (
          <li key={item} className='flex items-center gap-2'>
            {index > 0 && <span className='h-px w-6 bg-border' />}
            <span
              className={`flex h-5 w-5 items-center justify-center rounded-full border text-[11px] ${
                active
                  ? 'border-primary bg-primary text-primary-foreground'
                  : done
                    ? 'border-primary text-primary'
                    : 'text-muted-foreground'
              }`}
            >
              {index + 1}
            </span>
            <span className={active ? 'font-medium' : 'text-muted-foreground'}>
              {ts(`wizardSteps.${item}`)}
            </span>
          </li>
        );
      })}
    </ol>
  );

  const renderHostStep = () => (
    <div className='space-y-4'>
      <div className='space-y-2'>
        <Label>
          {t('cluster.selectHost')} <span className='text-destructive'>*</span>
        </Label>
        {loadingHosts ? (
          <div className='flex h-9 items-center gap-2 text-sm text-muted-foreground'>
            <Loader2 className='h-4 w-4 animate-spin' />
            {t('common.loading')}
          </div>
        ) : hosts.length === 0 ? (
          <div className='rounded-md border p-4 text-sm text-muted-foreground'>
            {t('cluster.noAvailableHosts')}
          </div>
        ) : (
          <Select value={hostId} onValueChange={setHostId}>
            <SelectTrigger>
              <SelectValue placeholder={t('cluster.selectHostPlaceholder')} />
            </SelectTrigger>
            <SelectContent>
              {hosts.map((host) => (
                <SelectItem key={host.id} value={host.id.toString()}>
                  <div className='flex items-center gap-2'>
                    <Server className='h-4 w-4' />
                    <span>{host.name}</span>
                    {host.ip_address && (
                      <code className='font-mono text-xs text-muted-foreground'>
                        {host.ip_address}
                      </code>
                    )}
                  </div>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        <p className='text-xs text-muted-foreground'>
          {t('cluster.onlyAgentInstalledHosts')}
        </p>
      </div>

      <div className='space-y-2'>
        <Label>{ts('roleLabel')}</Label>
        {isHybrid ? (
          <p className='rounded-md bg-muted/50 px-3 py-2 text-sm'>
            {ts('roleHybridFixed')}
          </p>
        ) : (
          <>
            <Select
              value={role}
              onValueChange={(value) => setRole(value as NodeRole)}
            >
              <SelectTrigger>
                <SelectValue placeholder={ts('roleSeparatedHint')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NodeRole.MASTER}>
                  {t('cluster.roles.master')}
                </SelectItem>
                <SelectItem value={NodeRole.WORKER}>
                  {t('cluster.roles.worker')}
                </SelectItem>
              </SelectContent>
            </Select>
            <p className='text-xs text-muted-foreground'>
              {ts('roleSeparatedHint')}
            </p>
          </>
        )}
      </div>

      {planLoading && (
        <p className='flex items-center gap-2 text-sm text-muted-foreground'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {ts('planLoading')}
        </p>
      )}
      {planError && (
        <p className='break-words text-sm text-red-600'>
          {ts('planFailed')}: {planError}
        </p>
      )}
    </div>
  );

  /**
   * 滚动重启选项：现有节点不会热加载 member-list，重启后才能在集群分裂后主动找回新节点。
   * Rolling-restart option: existing nodes do not hot-reload the member list; only after a restart can they find the
   * new node again after a split.
   */
  const renderRollingRestart = () => {
    const restart = plan?.rolling_restart;
    if (!restart || restart.nodes.length === 0) {
      return null;
    }
    const names = restart.nodes.map((node) => node.host_name).join(', ');
    return (
      <section className='mt-4 space-y-1.5 rounded-lg border p-3'>
        <label className='flex items-start gap-2 text-sm font-medium'>
          <Checkbox
            className='mt-0.5'
            checked={rollingRestart && restart.available}
            disabled={!restart.available}
            onCheckedChange={(checked) => setRollingRestart(checked === true)}
          />
          <span>
            {ts('rollingRestartLabel', {count: restart.nodes.length, names})}
          </span>
        </label>
        <p className='pl-6 text-xs text-muted-foreground'>
          {ts('rollingRestartDesc')}
        </p>
        {!restart.available && restart.blocked_reason && (
          <p className='pl-6 text-xs text-amber-700 dark:text-amber-400'>
            {restart.blocked_reason}
          </p>
        )}
        {restart.available && restart.note && (
          <p className='pl-6 text-xs text-amber-700 dark:text-amber-400'>
            {restart.note}
          </p>
        )}
      </section>
    );
  };

  const renderPlanStep = () =>
    plan && (
      <div className='space-y-4'>
        <section className='space-y-3 rounded-lg border p-3'>
          <p className='text-sm'>{detectionText()}</p>
          <div className='flex flex-wrap items-center gap-3'>
            <Label className='text-xs text-muted-foreground'>
              {ts('modeLabel')}
            </Label>
            <ToggleGroup
              type='single'
              variant='outline'
              size='sm'
              value={mode}
              onValueChange={(value) => {
                if (value) {
                  setMode(value as ScaleMode);
                  setModeTouched(true);
                  markDirty();
                }
              }}
            >
              <ToggleGroupItem value='install'>
                {ts('modeInstall')}
              </ToggleGroupItem>
              <ToggleGroupItem value='register'>
                {ts('modeRegister')}
              </ToggleGroupItem>
            </ToggleGroup>
            {modeTouched && mode !== plan.detected_mode && (
              <span className='text-xs text-amber-600'>{ts('modeManual')}</span>
            )}
          </div>

          <div className='grid gap-3 md:grid-cols-[2fr_1fr_1fr_1fr]'>
            <div className='space-y-1'>
              <Label className='text-xs'>{ts('installDir')}</Label>
              <Input
                className='font-mono'
                value={installDir}
                onChange={(e) => {
                  setInstallDir(e.target.value);
                  markDirty();
                }}
              />
            </div>
            <div className='space-y-1'>
              <Label className='text-xs'>{ts('hazelcastPort')}</Label>
              <Input
                type='number'
                value={ports.hazelcast_port || ''}
                onChange={(e) => {
                  setPorts((prev) => ({
                    ...prev,
                    hazelcast_port: parsePort(e.target.value),
                  }));
                  markDirty();
                }}
              />
            </div>
            {plan.target.role !== NodeRole.WORKER && (
              <div className='space-y-1'>
                <Label className='text-xs'>{ts('apiPort')}</Label>
                <Input
                  type='number'
                  value={ports.api_port || ''}
                  onChange={(e) => {
                    setPorts((prev) => ({
                      ...prev,
                      api_port: parsePort(e.target.value),
                    }));
                    markDirty();
                  }}
                />
              </div>
            )}
            {plan.target.role === NodeRole.MASTER_WORKER && (
              <div className='space-y-1'>
                <Label className='text-xs'>{ts('workerPort')}</Label>
                <Input
                  type='number'
                  value={ports.worker_port || ''}
                  onChange={(e) => {
                    setPorts((prev) => ({
                      ...prev,
                      worker_port: parsePort(e.target.value),
                    }));
                    markDirty();
                  }}
                />
              </div>
            )}
          </div>
          <JavaHomePicker
            hostId={Number(hostId) || null}
            value={javaHome}
            preferredJavaHome={
              plan.detected_mode === 'register'
                ? plan.target.java_home || undefined
                : undefined
            }
            onChange={({javaHome: next}) => {
              setJavaHome(next);
              markDirty();
            }}
          />
          <div className='space-y-0.5 text-xs text-muted-foreground'>
            <p>{ts('installDirHint')}</p>
            <p>{ts('portsHint')}</p>
          </div>
        </section>

        {planError && (
          <p className='break-words text-sm text-red-600'>
            {ts('planFailed')}: {planError}
          </p>
        )}

        {/* 配置变更或重新预检期间半透明保留旧结果，避免高度塌陷 / Keep the old result dimmed while stale or reloading */}
        <div
          className={
            dirty || planLoading ? 'pointer-events-none opacity-60' : ''
          }
        >
          <ScalePlanView
            plan={plan}
            restartAck={restartAck}
            onRestartAckChange={setRestartAck}
            extraSteps={
              rollingRestart && plan.rolling_restart?.available
                ? ['rolling_restart']
                : []
            }
          />
          {renderRollingRestart()}
        </div>
      </div>
    );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-4xl' aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{t('cluster.addNode')}</DialogTitle>
        </DialogHeader>

        {renderStepIndicator()}

        <div className='max-h-[64vh] overflow-y-auto pr-1'>
          {step === 'host' && renderHostStep()}
          {step === 'plan' && renderPlanStep()}
          {step === 'execute' && operationId !== null && (
            <ScaleOperationProgress
              clusterId={clusterId}
              operationId={operationId}
              initial={initialOperation}
              onFinished={handleFinished}
            />
          )}
        </div>

        <DialogFooter className='gap-2 sm:gap-0'>
          {step === 'host' && (
            <>
              <Button variant='outline' onClick={() => handleOpenChange(false)}>
                {t('common.cancel')}
              </Button>
              <Button onClick={() => void runPlan(false)} disabled={!canGoNext}>
                {planLoading && (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                )}
                {ts('next')}
              </Button>
            </>
          )}
          {step === 'plan' && (
            <>
              <Button
                variant='outline'
                onClick={() => setStep('host')}
                disabled={planLoading || submitting}
              >
                {ts('back')}
              </Button>
              <Button
                variant='secondary'
                onClick={() => void runPlan(true)}
                disabled={planLoading || submitting}
              >
                {planLoading ? (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                ) : (
                  <RefreshCw className='mr-2 h-4 w-4' />
                )}
                {ts('recheck')}
              </Button>
              <Button
                onClick={handleSubmit}
                disabled={!canSubmit}
                title={
                  plan && !plan.can_submit ? ts('cannotSubmit') : undefined
                }
              >
                {submitting && (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                )}
                {ts('submitScaleOut')}
              </Button>
            </>
          )}
          {step === 'execute' && (
            <Button variant='outline' onClick={() => handleOpenChange(false)}>
              {ts('close')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
