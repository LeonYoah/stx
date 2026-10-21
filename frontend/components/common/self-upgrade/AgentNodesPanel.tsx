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
import {Badge} from '@/components/ui/badge';
import {
  agentNodeTone,
  summarizeAgentNodes,
  type AgentNodeTone,
  type AgentNodeUpgrade,
  type AgentVersionInfo,
} from '@/lib/services/self-upgrade';
import {cn} from '@/lib/utils';

const toneClass: Record<AgentNodeTone, string> = {
  success:
    'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/20',
  warning:
    'bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/20',
  danger: 'bg-destructive/10 text-destructive border-destructive/20',
  progress:
    'bg-blue-500/10 text-blue-700 dark:text-blue-400 border-blue-500/20',
  muted: 'text-muted-foreground',
};

function useNodeStatusLabel() {
  const t = useTranslations('selfUpgrade');
  return (status: string): string => {
    const map: Record<string, string> = {
      pending: t('nodePending'),
      upgrading: t('nodeUpgrading'),
      succeeded: t('nodeSucceeded'),
      failed: t('nodeFailed'),
      rolled_back: t('nodeRolledBack'),
      offline: t('nodeOffline'),
      skipped: t('nodeSkipped'),
      updated: t('proxyUpdated'),
    };
    return map[status] ?? status;
  };
}

// AgentNodeList 展示每个节点的 Agent/Proxy 升级状态。
// AgentNodeList shows the per-node Agent/Proxy upgrade status.
export function AgentNodeList({nodes}: {nodes: AgentNodeUpgrade[]}) {
  const t = useTranslations('selfUpgrade');
  const label = useNodeStatusLabel();
  const summary = summarizeAgentNodes(nodes);
  const proxyRestartLabel = (status: string): string => {
    const map: Record<string, string> = {
      restarted: t('proxyRestartRestarted'),
      not_running: t('proxyRestartNotRunning'),
      rolled_back: t('proxyRestartRolledBack'),
      failed: t('proxyRestartFailed'),
      skipped: t('proxyRestartSkipped'),
    };
    return map[status] ?? status;
  };
  return (
    <div className='space-y-2' data-testid='agent-node-list'>
      <div className='text-xs text-muted-foreground'>
        {t('agentNodesSummary', {
          succeeded: summary.succeeded,
          total: summary.total,
          failed: summary.failed + summary.rolledBack,
          offline: summary.offline,
        })}
      </div>
      <div className='divide-y rounded-md border text-xs'>
        {nodes.map((node) => (
          <div
            key={node.host_id}
            className='flex flex-wrap items-center gap-2 px-3 py-2'
          >
            <div className='min-w-0 flex-1'>
              <div className='font-medium truncate'>
                {node.host_name}
                {node.ip_address ? (
                  <span className='ml-1.5 font-mono text-muted-foreground'>
                    {node.ip_address}
                  </span>
                ) : null}
              </div>
              {node.message ? (
                <div className='text-muted-foreground break-words'>
                  {node.message}
                </div>
              ) : null}
              {node.proxy_message ? (
                <div className='text-muted-foreground break-words'>
                  Proxy: {node.proxy_message}
                </div>
              ) : null}
              {node.proxy_restart_message ? (
                <div className='text-muted-foreground break-words'>
                  {t('proxyRestartPrefix')}: {node.proxy_restart_message}
                </div>
              ) : null}
            </div>
            <span className='font-mono tabular-nums text-muted-foreground shrink-0'>
              {node.from_version || '—'} → {node.current_version || '—'}
            </span>
            {node.proxy_status ? (
              <Badge
                variant='outline'
                className={cn(
                  'text-[10px] shrink-0',
                  toneClass[agentNodeTone(node.proxy_status)],
                )}
              >
                Proxy {label(node.proxy_status)}
              </Badge>
            ) : null}
            {node.proxy_restart ? (
              <Badge
                variant='outline'
                data-testid='proxy-restart-badge'
                className={cn(
                  'text-[10px] shrink-0',
                  toneClass[agentNodeTone(node.proxy_restart)],
                )}
              >
                {proxyRestartLabel(node.proxy_restart)}
              </Badge>
            ) : null}
            <Badge
              variant='outline'
              className={cn(
                'text-[10px] shrink-0',
                toneClass[agentNodeTone(node.status)],
              )}
            >
              {label(node.status)}
            </Badge>
          </div>
        ))}
      </div>
    </div>
  );
}

// AgentVersionsPanel 列出各节点当前 Agent 版本并标出与控制面不一致的节点。
// AgentVersionsPanel lists each node's current Agent version and flags those that differ from the control plane.
export function AgentVersionsPanel({
  agents,
  outdated,
  controlPlaneVersion,
}: {
  agents: AgentVersionInfo[];
  outdated: number;
  controlPlaneVersion: string;
}) {
  const t = useTranslations('selfUpgrade');
  if (agents.length === 0) {
    return null;
  }
  return (
    <div className='space-y-2' data-testid='agent-versions-panel'>
      <div className='flex items-center justify-between gap-2'>
        <div className='text-xs font-medium'>{t('agentVersionsTitle')}</div>
        {outdated > 0 ? (
          <Badge
            variant='outline'
            className={cn('text-[10px]', toneClass.warning)}
          >
            {t('agentsOutdated', {
              count: outdated,
              version: controlPlaneVersion,
            })}
          </Badge>
        ) : (
          <Badge
            variant='outline'
            className={cn('text-[10px]', toneClass.success)}
          >
            {t('agentsAllCurrent')}
          </Badge>
        )}
      </div>
      <div className='grid gap-1.5 sm:grid-cols-2 lg:grid-cols-3 text-xs'>
        {agents.map((a) => (
          <div
            key={a.host_id}
            className={cn(
              'rounded border px-2.5 py-1.5 flex items-center justify-between gap-2',
              !a.up_to_date && 'border-amber-500/30 bg-amber-500/5',
            )}
          >
            <span className='truncate'>
              {a.host_name}
              {!a.online ? (
                <span className='ml-1 text-muted-foreground'>
                  ({t('nodeOffline')})
                </span>
              ) : null}
            </span>
            <span className='font-mono tabular-nums shrink-0'>
              {a.version || '—'}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
