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

import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  ChevronRight,
  MinusCircle,
  XCircle,
} from 'lucide-react';

import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/ui/checkbox';
import type {
  ScaleCheck,
  ScalePlan,
  ScaleStepName,
} from '@/lib/services/cluster';

/**
 * 伸缩计划展示：阻断 / 提醒 / 通过分级、成员变化、插件、作业风险与确认勾选。
 * 遵循渐进式披露：阻断与提醒默认展开，通过项折叠。
 * Scaling plan view: blocking / warning / passed tiers, member changes, plugins, job risk and
 * acknowledgements. Follows progressive disclosure: blocking and warning items are expanded,
 * passed items are collapsed.
 */
export interface ScalePlanViewProps {
  plan: ScalePlan;
  /** 回环重启确认（扩容）/ Loopback restart acknowledgement (scale-out) */
  restartAck?: boolean;
  onRestartAckChange?: (value: boolean) => void;
  /** 作业风险确认（缩容）/ Job risk acknowledgement (scale-in) */
  jobsAck?: boolean;
  onJobsAckChange?: (value: boolean) => void;
  /** 计划外、由用户选择追加的步骤（如滚动重启）/ Extra user-chosen steps appended to the preview (e.g. rolling restart) */
  extraSteps?: ScaleStepName[];
}

/**
 * 预检项状态图标 / Precheck status icon
 */
function CheckIcon({status}: {status: ScaleCheck['status']}) {
  switch (status) {
    case 'passed':
      return (
        <CheckCircle2 className='mt-0.5 h-4 w-4 shrink-0 text-green-600' />
      );
    case 'failed':
      return <XCircle className='mt-0.5 h-4 w-4 shrink-0 text-red-600' />;
    case 'warning':
      return (
        <AlertTriangle className='mt-0.5 h-4 w-4 shrink-0 text-amber-500' />
      );
    default:
      return (
        <MinusCircle className='mt-0.5 h-4 w-4 shrink-0 text-muted-foreground' />
      );
  }
}

/**
 * 地址胶囊：原子信息不折断，整体随容器换行。
 * Address chips: atomic values never break internally and wrap as a whole.
 */
function AddressChips({addresses}: {addresses: string[]}) {
  if (addresses.length === 0) {
    return <span className='text-xs text-muted-foreground'>—</span>;
  }
  return (
    <div className='flex flex-wrap gap-1.5'>
      {addresses.map((address) => (
        <code
          key={address}
          className='whitespace-nowrap rounded bg-muted px-1.5 py-0.5 font-mono text-xs'
        >
          {address}
        </code>
      ))}
    </div>
  );
}

export function ScalePlanView({
  plan,
  restartAck = false,
  onRestartAckChange,
  jobsAck = false,
  onJobsAckChange,
  extraSteps,
}: ScalePlanViewProps) {
  const t = useTranslations('cluster.scale');

  const checkLabel = (name: string) =>
    t.has(`checks.${name}`) ? t(`checks.${name}`) : name;

  const blocking = plan.checks.filter(
    (check) => check.status === 'failed' && check.blocking,
  );
  const warnings = plan.checks.filter(
    (check) =>
      check.status === 'warning' ||
      (check.status === 'failed' && !check.blocking),
  );
  const passed = plan.checks.filter(
    (check) => check.status === 'passed' || check.status === 'skipped',
  );
  const membersChanged =
    plan.members_before.join(',') !== plan.members_after.join(',');
  const localCheckpoint = plan.checkpoint_storage?.local ?? false;
  const previewSteps = [...plan.steps, ...(extraSteps ?? [])];

  const renderCheck = (check: ScaleCheck, index: number) => (
    <li
      key={`${check.name}-${index}`}
      className='flex items-start gap-2 text-sm'
    >
      <CheckIcon status={check.status} />
      <div className='min-w-0'>
        <span className='font-medium'>{checkLabel(check.name)}</span>
        {check.target && (
          <code className='ml-1.5 whitespace-nowrap font-mono text-xs text-muted-foreground'>
            {check.target}
          </code>
        )}
        <p className='break-words text-xs text-muted-foreground'>
          {check.message}
        </p>
      </div>
    </li>
  );

  return (
    <div className='space-y-4'>
      {blocking.length > 0 && (
        <section className='rounded-lg border border-red-200 bg-red-50/60 p-3 dark:border-red-900/60 dark:bg-red-950/30'>
          <h4 className='mb-2 text-sm font-semibold text-red-700 dark:text-red-400'>
            {t('blockingTitle', {count: blocking.length})}
          </h4>
          <ul className='space-y-2'>{blocking.map(renderCheck)}</ul>
        </section>
      )}

      {plan.requires_restart_ack && plan.loopback_nodes.length > 0 && (
        <section className='space-y-2 rounded-lg border border-amber-300 bg-amber-50/60 p-3 dark:border-amber-800/60 dark:bg-amber-950/30'>
          <h4 className='text-sm font-semibold text-amber-800 dark:text-amber-300'>
            {t('loopbackTitle', {count: plan.loopback_nodes.length})}
          </h4>
          <p className='text-xs text-muted-foreground'>{t('loopbackDesc')}</p>
          <ul className='flex flex-wrap gap-2'>
            {plan.loopback_nodes.map((node) => (
              <li key={node.node_id}>
                <Badge variant='outline' className='gap-1.5 font-normal'>
                  <span>{node.host_name}</span>
                  <code className='font-mono text-xs'>{node.address}</code>
                </Badge>
              </li>
            ))}
          </ul>
          {onRestartAckChange && (
            <label className='flex items-center gap-2 text-sm font-medium'>
              <Checkbox
                checked={restartAck}
                onCheckedChange={(checked) =>
                  onRestartAckChange(checked === true)
                }
              />
              {t('loopbackAck', {count: plan.loopback_nodes.length})}
            </label>
          )}
        </section>
      )}

      {plan.requires_jobs_ack && (
        <section className='space-y-2 rounded-lg border border-amber-300 bg-amber-50/60 p-3 dark:border-amber-800/60 dark:bg-amber-950/30'>
          <h4 className='text-sm font-semibold text-amber-800 dark:text-amber-300'>
            {t('jobsRiskTitle')}
          </h4>
          {plan.running_jobs_known ? (
            <ul className='flex flex-wrap gap-1.5'>
              {plan.running_jobs.map((job) => (
                <li key={job.job_id}>
                  <Badge variant='secondary' className='font-normal'>
                    {job.job_name || job.job_id}
                  </Badge>
                </li>
              ))}
            </ul>
          ) : (
            <p className='text-sm'>{t('runningJobsUnknown')}</p>
          )}
          {localCheckpoint && (
            <p className='rounded-md bg-red-100 px-2 py-1.5 text-xs font-medium text-red-700 dark:bg-red-950/50 dark:text-red-400'>
              {t('checkpointLocalRisk', {
                location: plan.checkpoint_storage?.location || '-',
              })}
            </p>
          )}
          {onJobsAckChange && (
            <label className='flex items-center gap-2 text-sm font-medium'>
              <Checkbox
                checked={jobsAck}
                onCheckedChange={(checked) => onJobsAckChange(checked === true)}
              />
              {t('jobsRiskAck')}
            </label>
          )}
        </section>
      )}

      {warnings.length > 0 && (
        <section className='rounded-lg border p-3'>
          <h4 className='mb-2 text-sm font-semibold text-amber-700 dark:text-amber-400'>
            {t('warningTitle', {count: warnings.length})}
          </h4>
          <ul className='space-y-2'>{warnings.map(renderCheck)}</ul>
        </section>
      )}

      {passed.length > 0 && (
        <details className='group rounded-lg border p-3'>
          <summary className='flex cursor-pointer list-none items-center gap-1 text-sm font-medium text-muted-foreground'>
            <ChevronRight className='h-4 w-4 transition-transform group-open:rotate-90' />
            {t('passedTitle', {count: passed.length})}
          </summary>
          <ul className='mt-2 space-y-2'>{passed.map(renderCheck)}</ul>
        </details>
      )}

      <section className='grid gap-3 rounded-lg border p-3 md:grid-cols-2'>
        <div className='space-y-1.5'>
          <h4 className='text-xs font-medium text-muted-foreground'>
            {t('target')}
          </h4>
          <div className='flex flex-wrap items-center gap-2 text-sm'>
            <span className='font-medium'>{plan.target.host_name}</span>
            {plan.target.address && (
              <code className='whitespace-nowrap font-mono text-xs'>
                {plan.target.address}
              </code>
            )}
          </div>
          {plan.target.install_dir && (
            <code
              className='block truncate font-mono text-xs text-muted-foreground'
              title={plan.target.install_dir}
            >
              {plan.target.install_dir}
            </code>
          )}
        </div>
        <div className='space-y-1.5'>
          <h4 className='text-xs font-medium text-muted-foreground'>
            {t('membersChange')}
          </h4>
          {membersChanged ? (
            <div className='space-y-1'>
              <div className='flex items-start gap-2'>
                <span className='w-14 shrink-0 text-xs text-muted-foreground'>
                  {t('membersBefore')}
                </span>
                <AddressChips addresses={plan.members_before} />
              </div>
              <div className='flex items-start gap-2'>
                <span className='w-14 shrink-0 text-xs text-muted-foreground'>
                  {t('membersAfter')}
                </span>
                <AddressChips addresses={plan.members_after} />
              </div>
            </div>
          ) : (
            <AddressChips addresses={plan.members_after} />
          )}
        </div>
      </section>

      {plan.kind === 'scale_out' && (
        <section className='space-y-1.5 rounded-lg border p-3'>
          <h4 className='text-xs font-medium text-muted-foreground'>
            {t('pluginsToReplay')}
          </h4>
          {plan.plugins_to_replay.length === 0 ? (
            <p className='text-xs text-muted-foreground'>{t('noPlugins')}</p>
          ) : (
            <ul className='flex flex-wrap gap-1.5'>
              {plan.plugins_to_replay.map((plugin) => (
                <li key={`${plugin.name}@${plugin.version}`}>
                  <Badge
                    variant='outline'
                    className='gap-1 font-normal'
                    title={
                      plugin.profile_unknown
                        ? t('profileUnknown')
                        : plugin.profile_inferred
                          ? t('profileInferred')
                          : undefined
                    }
                  >
                    <span>
                      {plugin.name}@{plugin.version}
                    </span>
                    {plugin.profile_keys.length > 0 && (
                      <span className='text-muted-foreground'>
                        ({plugin.profile_keys.join(', ')})
                      </span>
                    )}
                    {plugin.profile_unknown && (
                      <AlertTriangle className='h-3 w-3 text-amber-500' />
                    )}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}

      {plan.java_proxy_follow && (
        <p className='text-xs text-muted-foreground'>
          {t('javaProxyFollow', {
            from: plan.java_proxy_follow.from_host_name,
            to: plan.java_proxy_follow.to_host_name,
          })}
        </p>
      )}

      {previewSteps.length > 0 && (
        <section className='space-y-1.5'>
          <h4 className='text-xs font-medium text-muted-foreground'>
            {t('stepsPreview')}
          </h4>
          <ol className='flex flex-wrap items-center gap-1 text-xs'>
            {previewSteps.map((step, index) => (
              <li key={step} className='flex items-center gap-1'>
                {index > 0 && (
                  <ArrowRight className='h-3 w-3 text-muted-foreground' />
                )}
                <span className='whitespace-nowrap rounded bg-muted px-1.5 py-0.5'>
                  {t(`steps.${step}`)}
                </span>
              </li>
            ))}
          </ol>
        </section>
      )}
    </div>
  );
}
