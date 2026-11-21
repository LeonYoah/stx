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
 * 审计日志行渲染：只负责表头 + 行；加载/空/分页由外层 TableCard 承担。
 * Audit log row renderer: header + rows only; TableCard owns loading/empty/pagination.
 */

import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {Globe, KeyRound, User} from 'lucide-react';
import {cn} from '@/lib/utils';
import {AuditLogInfo} from '@/lib/services/audit/types';
import {lookupAuditLabel, type AuditTranslator} from './audit-i18n';
import {formatDateTimeTz} from '@/lib/timezone';
import layout from '@/components/common/layout/list-detail.module.css';

interface AuditLogTableProps {
  logs: AuditLogInfo[];
  onOpenTrace?: (log: AuditLogInfo) => void;
}

/**
 * Format resource display for table; cluster_node "53/48" -> "集群编号 53 下的节点 48"
 * 格式化资源名称展示：cluster_node 的 53/48 显示为「集群编号 53 下的节点 48」
 */
function formatResourceDisplay(
  resourceType: string,
  resourceId: string,
  resourceName: string,
  details: AuditLogInfo['details'],
  t: (key: string, values?: Record<string, string | number>) => string,
): string {
  if (
    resourceType === 'cluster_node' &&
    resourceId &&
    /^\d+\/\d+$/.test(resourceId)
  ) {
    const [, nodeId] = resourceId.split('/');
    if (resourceName) {
      return t('audit.clusterNodeWithName', {
        clusterName: resourceName,
        nodeId,
      });
    }
    const [clusterId] = resourceId.split('/');
    return t('audit.clusterNodeResourceName', {clusterId, nodeId});
  }
  // 命令行只展示具体命令名，类型列已经说明这是命令。
  // The name column shows the concrete command; the type column already says it is a command.
  if (resourceType === 'agent_command') {
    const commandType =
      typeof details?.command_type === 'string'
        ? details.command_type.trim()
        : '';
    return resourceName || commandType || resourceId || '-';
  }
  if (resourceName) {
    return resourceName;
  }
  return resourceId || '-';
}

function isSystemActor(
  log: Pick<
    AuditLogInfo,
    | 'user_id'
    | 'username'
    | 'action'
    | 'resource_type'
    | 'trigger'
    | 'user_agent'
    | 'details'
  >,
): boolean {
  if (log.user_id) {
    return false;
  }
  const username = log.username?.trim().toLowerCase() ?? '';
  if (username === 'agent') {
    return true;
  }
  if (username) {
    return false;
  }
  const action = log.action || '';
  const resourceType = log.resource_type || '';
  const trigger = log.trigger ?? (log.details?.trigger as string | undefined);
  return (
    resourceType === 'agent' ||
    resourceType === 'agent_command' ||
    action.startsWith('agent') ||
    trigger === 'auto' ||
    (log.user_agent || '').includes('stx-agent')
  );
}

function getActionLabel(
  action: string,
  t: ReturnType<typeof useTranslations>,
): string {
  return lookupAuditLabel('actions', action, action, t);
}

function getResourceTypeLabel(
  resourceType: string,
  t: ReturnType<typeof useTranslations>,
): string {
  return lookupAuditLabel('resourceTypes', resourceType, resourceType, t);
}

function getActorLabel(
  log: Pick<
    AuditLogInfo,
    | 'user_id'
    | 'username'
    | 'action'
    | 'resource_type'
    | 'trigger'
    | 'user_agent'
    | 'details'
  >,
  t: (key: string) => string,
): string {
  const username = log.username?.trim();
  if (log.user_id && username) {
    return username;
  }
  if (isSystemActor(log)) {
    return t('audit.actorSystem');
  }
  return username || '-';
}

function getTokenLabel(
  log: Pick<AuditLogInfo, 'token_id' | 'token_prefix' | 'token_name' | 'client_type'>,
  t: (key: string) => string,
): string {
  const name = log.token_name?.trim();
  const prefix = log.token_prefix?.trim();
  if (name && prefix) {
    return `${name} (${prefix}****)`;
  }
  if (name) {
    return name;
  }
  if (prefix) {
    return `${prefix}****`;
  }
  if ((log.client_type || '').toLowerCase() === 'web') {
    return t('audit.tokenSession');
  }
  return '-';
}

function getSourceLabel(
  log: Pick<
    AuditLogInfo,
    | 'user_id'
    | 'username'
    | 'action'
    | 'resource_type'
    | 'trigger'
    | 'user_agent'
    | 'details'
    | 'client_type'
  >,
  t: AuditTranslator,
): string {
  const clientType = log.client_type?.trim().toLowerCase() ?? '';
  const writerSources = new Set([
    'reconciler',
    'scheduler',
    'dolphin_worker',
    'agent_callback',
  ]);
  if (writerSources.has(clientType)) {
    const writerLabel = lookupAuditLabel(
      'clientTypes',
      clientType,
      clientType,
      t,
    );
    const initiator =
      typeof log.details?.initiator_client_type === 'string'
        ? log.details.initiator_client_type.trim().toLowerCase()
        : '';
    if (initiator && (initiator === 'cli' || initiator === 'web' || initiator === 'api')) {
      const initiatorLabel = lookupAuditLabel(
        'clientTypes',
        initiator,
        initiator,
        t,
      );
      return `${initiatorLabel} → ${writerLabel}`;
    }
    return writerLabel;
  }
  if (clientType === 'cli' || clientType === 'web') {
    return lookupAuditLabel('clientTypes', clientType, clientType, t);
  }
  if (clientType === 'system' || clientType === 'api' || isSystemActor(log)) {
    return t('audit.actorSystem');
  }
  if (log.user_id) {
    return lookupAuditLabel('clientTypes', 'web', 'Web', t);
  }
  return '-';
}

function getActionBadgeVariant(
  action: string,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (action.toLowerCase()) {
    case 'create':
      return 'default';
    case 'update':
      return 'secondary';
    case 'delete':
      return 'destructive';
    case 'start':
    case 'stop':
    case 'restart':
      return 'outline';
    default:
      return 'secondary';
  }
}

/**
 * Audit Log Table Component
 * 审计日志表格组件
 */
export function AuditLogTable({logs, onOpenTrace}: AuditLogTableProps) {
  const t = useTranslations();

  return (
    <>
      <div className={layout.rowHead} aria-hidden>
        <span>{t('audit.user')}</span>
        <span>{t('audit.token')}</span>
        <span>{t('audit.source')}</span>
        <span>{t('audit.action')}</span>
        <span>{t('audit.resourceType')}</span>
        <span>{t('audit.resourceName')}</span>
        <span>{t('audit.commandColumn')}</span>
        <span>{t('audit.ipAddress')}</span>
        <span>{t('audit.createdAt')}</span>
      </div>
      <div className={layout.tableScroll}>
        {logs.map((log) => (
          <div key={log.id} className={cn(layout.row)} data-testid={`audit-row-${log.id}`}>
            <div className={layout.rowName}>
              <div className={layout.rowNameInline}>
                <User className='size-3.5 shrink-0 text-muted-foreground' aria-hidden />
                <span className={layout.name} title={getActorLabel(log, t)}>
                  {getActorLabel(log, t)}
                </span>
              </div>
            </div>
            <div className={cn(layout.sub, 'flex min-w-0 items-center gap-1.5 font-mono text-xs')}>
              <KeyRound className='size-3.5 shrink-0 text-muted-foreground' aria-hidden />
              <span className='truncate whitespace-nowrap' title={getTokenLabel(log, t)}>
                {getTokenLabel(log, t)}
              </span>
            </div>
            <span className={layout.sub}>{getSourceLabel(log, t)}</span>
            <Badge variant={getActionBadgeVariant(log.action)}>
              {getActionLabel(log.action, t)}
            </Badge>
            <Badge variant='outline'>
              {getResourceTypeLabel(log.resource_type, t)}
            </Badge>
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className={cn(layout.sub, 'truncate')}>
                    {formatResourceDisplay(
                      log.resource_type,
                      log.resource_id || '',
                      log.resource_name || '',
                      log.details,
                      t,
                    )}
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  <p>ID: {log.resource_id || '-'}</p>
                  <p>Name: {log.resource_name || '-'}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <div>
              {log.command_count && log.request_id && onOpenTrace ? (
                <button
                  type='button'
                  className='text-xs text-primary hover:underline'
                  onClick={() => onOpenTrace(log)}
                >
                  {t('audit.serverCommands', {count: log.command_count})}
                </button>
              ) : Boolean(log.details?.engine_url || log.details?.engine_base_url) &&
                onOpenTrace ? (
                <button
                  type='button'
                  className='inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline'
                  onClick={() => onOpenTrace(log)}
                >
                  <span className='inline-block size-1.5 rounded-full bg-emerald-500' />
                  {t('audit.clusterApiCall')}
                </button>
              ) : (
                <span className='text-muted-foreground'>-</span>
              )}
            </div>
            <div className='flex min-w-0 items-center gap-1.5'>
              <Globe className='size-3.5 shrink-0 text-muted-foreground' aria-hidden />
              <span className='truncate font-mono text-xs'>{log.ip_address || '-'}</span>
            </div>
            <span className={cn(layout.sub, 'whitespace-nowrap')}>
              {formatDateTimeTz(log.created_at)}
            </span>
          </div>
        ))}
      </div>
    </>
  );
}
