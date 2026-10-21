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

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  ArrowUpCircle,
  FlaskConical,
  ShieldCheck,
} from 'lucide-react';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {Button} from '@/components/ui/button';
import {
  countUpgradeNodes,
  restartingProcesses,
  type AgentVersionInfo,
  type ExecutionMode,
  type RestartPlanInfo,
} from '@/lib/services/self-upgrade';

export interface ExecuteActionsProps {
  targetVersion?: string;
  canUpgrade: boolean;
  busy: boolean;
  restartPlan?: RestartPlanInfo | null;
  agents?: AgentVersionInfo[];
  onExecute: (mode: ExecutionMode) => void | Promise<void>;
}

// ExecuteActions：主按钮「开始升级」先弹确认框再真实应用；次按钮「演练」直接发送 dry_run。
// ExecuteActions: the primary "Start upgrade" confirms first and then really applies; the secondary
// "Dry run" sends dry_run directly.
export function ExecuteActions({
  targetVersion,
  canUpgrade,
  busy,
  restartPlan,
  agents,
  onExecute,
}: ExecuteActionsProps) {
  const t = useTranslations('selfUpgrade');
  const [confirmOpen, setConfirmOpen] = useState(false);
  const counts = countUpgradeNodes(agents, targetVersion);
  const processes = restartingProcesses(restartPlan);
  const version = targetVersion
    ? targetVersion.replace(/^[vV](?=\d)/, '')
    : '—';
  const strategy = restartPlan?.control_plane_strategy || 'standalone';

  const processLabel = (p: string): string => {
    switch (p) {
      case 'controlPlane':
        return t('restartControlPlane', {strategy});
      case 'frontend':
        return t('restartFrontend');
      case 'agents':
        return t('restartAgents');
      default:
        return t('restartProxies');
    }
  };

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Button
        size='sm'
        variant='outline'
        className='active:scale-[0.98]'
        disabled={!canUpgrade || busy}
        onClick={() => void onExecute('dry_run')}
        data-testid='self-upgrade-dry-run'
      >
        <FlaskConical className='h-4 w-4 mr-1.5' />
        {t('dryRunButton')}
      </Button>
      <Button
        size='sm'
        className='active:scale-[0.98]'
        disabled={!canUpgrade || busy}
        onClick={() => setConfirmOpen(true)}
        data-testid='self-upgrade-start'
      >
        <ArrowUpCircle className='h-4 w-4 mr-1.5' />
        {t('startUpgrade')}
      </Button>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent data-testid='self-upgrade-confirm'>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('confirmTitle', {version})}</AlertDialogTitle>
            <AlertDialogDescription>{t('confirmDesc')}</AlertDialogDescription>
          </AlertDialogHeader>

          <dl className='grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm'>
            <dt className='text-muted-foreground'>{t('confirmTarget')}</dt>
            <dd
              className='font-mono font-semibold'
              data-testid='confirm-target'
            >
              v{version}
            </dd>
            <dt className='text-muted-foreground'>{t('confirmComponents')}</dt>
            <dd data-testid='confirm-components'>
              {[
                t('componentControlPlane'),
                t('componentAgent'),
                t('componentProxy'),
                ...(restartPlan?.frontend_detected
                  ? [t('componentFrontend')]
                  : []),
              ].join(' · ')}
            </dd>
            <dt className='text-muted-foreground'>{t('confirmNodes')}</dt>
            <dd data-testid='confirm-nodes'>
              {t('confirmNodesValue', {
                total: counts.total,
                toUpgrade: counts.toUpgrade,
                online: counts.online,
                offline: counts.offline,
              })}
              {counts.offlineToUpgrade > 0 ? (
                <div className='text-xs text-amber-700 dark:text-amber-300'>
                  {t('confirmOfflineNote', {count: counts.offlineToUpgrade})}
                </div>
              ) : null}
            </dd>
            <dt className='text-muted-foreground'>{t('confirmRestarts')}</dt>
            <dd>
              <ul
                className='list-disc pl-4 space-y-0.5'
                data-testid='confirm-restarts'
              >
                {processes.map((p) => (
                  <li key={p}>{processLabel(p)}</li>
                ))}
              </ul>
            </dd>
          </dl>

          <div className='rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs flex gap-2'>
            <AlertTriangle className='h-4 w-4 text-amber-600 shrink-0' />
            <span>{t('confirmRestartWarning')}</span>
          </div>
          <div
            className='rounded-md border border-emerald-500/30 bg-emerald-500/5 px-3 py-2 text-xs flex gap-2'
            data-testid='confirm-seatunnel-safe'
          >
            <ShieldCheck className='h-4 w-4 text-emerald-600 shrink-0' />
            <span>{t('confirmSeaTunnelSafe')}</span>
          </div>

          <AlertDialogFooter>
            <AlertDialogCancel>{t('confirmCancel')}</AlertDialogCancel>
            <AlertDialogAction
              data-testid='self-upgrade-confirm-apply'
              onClick={() => void onExecute('apply')}
            >
              {t('confirmApply')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
