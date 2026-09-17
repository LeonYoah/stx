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
 * Audit Log Table Component
 * 审计日志表格组件
 *
 * Displays a table of audit logs with filtering and pagination.
 * 显示审计日志表格，支持过滤和分页。
 */

import {useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Pagination} from '@/components/ui/pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {User, Globe} from 'lucide-react';
import {useGSAP} from '@gsap/react';
import {TableLoadingBar, TableSkeletonRows} from '@/components/common/layout';
import {animateTableRows} from '@/lib/animations/gsap-motion';
import {AuditLogInfo} from '@/lib/services/audit/types';

interface AuditLogTableProps {
  logs: AuditLogInfo[];
  loading: boolean;
  currentPage: number;
  totalPages: number;
  total: number;
  pageSize?: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
}

/**
 * Format resource display for table; cluster_node "53/48" -> "集群编号 53 下的节点 48"
 * 格式化资源名称展示：cluster_node 的 53/48 显示为「集群编号 53 下的节点 48」
 */
function formatResourceDisplay(
  resourceType: string,
  resourceId: string,
  resourceName: string,
  t: (key: string, values?: Record<string, string | number>) => string,
): string {
  if (resourceType === 'cluster_node' && resourceId && /^\d+\/\d+$/.test(resourceId)) {
    const [, nodeId] = resourceId.split('/');
    if (resourceName) {
      return t('audit.clusterNodeWithName', {clusterName: resourceName, nodeId});
    }
    const [clusterId] = resourceId.split('/');
    return t('audit.clusterNodeResourceName', {clusterId, nodeId});
  }
  if (resourceName) {
    return resourceName;
  }
  return resourceId || '-';
}

/** 取操作的中文/本地化文案，无则回退为原始 action */
function getActionLabel(action: string, t: (key: string) => string): string {
  try {
    const key = `audit.actions.${action}` as 'audit.actions.create';
    const out = t(key);
    if (out && out !== key) {
      return out;
    }
  } catch {
    // key 不存在时 next-intl 可能抛错，忽略
  }
  return action;
}

/** 取资源类型的中文/本地化文案，无则回退为原始 resource_type */
function getResourceTypeLabel(resourceType: string, t: (key: string) => string): string {
  try {
    const key = `audit.resourceTypes.${resourceType}` as 'audit.resourceTypes.host';
    const out = t(key);
    if (out && out !== key) {
      return out;
    }
  } catch {
    // key 不存在时忽略
  }
  return resourceType;
}

/** 根据 trigger 字段或 details.trigger 显示触发方式：自动 / 手动（兼容旧数据） */
function getTriggerDisplay(
  log: Pick<AuditLogInfo, 'trigger' | 'details'>,
  t: (key: string) => string,
): { label: string; variant: 'secondary' | 'outline' } {
  const trigger = log.trigger ?? (log.details?.trigger as string | undefined);
  if (trigger === 'auto') {
    return { label: t('audit.triggerAuto'), variant: 'secondary' };
  }
  if (trigger === 'manual') {
    return { label: t('audit.triggerManual'), variant: 'outline' };
  }
  return { label: '-', variant: 'outline' };
}

/**
 * Get action badge variant
 * 获取操作徽章变体
 */
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
export function AuditLogTable({
  logs,
  loading,
  currentPage,
  totalPages,
  total,
  pageSize = 20,
  onPageChange,
  onPageSizeChange,
}: AuditLogTableProps) {
  const t = useTranslations();
  const tableContainerRef = useRef<HTMLDivElement>(null);

  // 审计日志数据更新后执行平滑交错入场动效
  // Trigger staggered entrance animation when audit logs data updates
  useGSAP(
    () => {
      if (logs.length > 0 && !loading) {
        animateTableRows('.audit-data-row');
      }
    },
    {dependencies: [logs, loading], scope: tableContainerRef},
  );

  return (
    <div ref={tableContainerRef} className='space-y-4 flex-1 flex flex-col'>
      <div className='border rounded-lg relative overflow-hidden bg-card/40 shadow-xs flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-270px)]'>
        <TableLoadingBar loading={loading} />
        <div className='overflow-x-auto flex-1'>
          <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-[50px]'>ID</TableHead>
              <TableHead>{t('audit.user')}</TableHead>
              <TableHead>{t('audit.trigger')}</TableHead>
              <TableHead>{t('audit.action')}</TableHead>
              <TableHead>{t('audit.resourceType')}</TableHead>
              <TableHead>{t('audit.resourceName')}</TableHead>
              <TableHead>{t('audit.ipAddress')}</TableHead>
              <TableHead>{t('audit.createdAt')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && logs.length === 0 ? (
              <TableSkeletonRows columns={8} rows={15} />
            ) : logs.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={8}
                  className='text-center py-12 text-muted-foreground'
                >
                  {t('audit.noAuditLogs')}
                </TableCell>
              </TableRow>
            ) : (
              logs.map((log) => (
                <TableRow
                  key={log.id}
                  className={`audit-data-row transition-opacity duration-200 ${
                    loading ? 'opacity-60 pointer-events-none' : ''
                  }`}
                >
                  <TableCell>{log.id}</TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      <User className='h-4 w-4 text-muted-foreground' />
                      <span>{log.username || '-'}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    {(() => {
                      const { label, variant } = getTriggerDisplay(log, t);
                      return <Badge variant={variant}>{label}</Badge>;
                    })()}
                  </TableCell>
                  <TableCell>
                    <Badge variant={getActionBadgeVariant(log.action)}>
                      {getActionLabel(log.action, t)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge variant='outline'>
                      {getResourceTypeLabel(log.resource_type, t)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className='truncate max-w-[150px] block cursor-default'>
                            {formatResourceDisplay(
                              log.resource_type,
                              log.resource_id || '',
                              log.resource_name || '',
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
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      <Globe className='h-4 w-4 text-muted-foreground' />
                      <span className='font-mono text-sm'>
                        {log.ip_address || '-'}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    {new Date(log.created_at).toLocaleString()}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        </div>

        {/* 底部分页栏 / Table Footer Pagination */}
        <div className='border-t bg-muted/10 px-4 py-2.5 mt-auto'>
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            pageSize={pageSize}
            totalItems={total}
            onPageChange={onPageChange}
            onPageSizeChange={onPageSizeChange}
            showPageSizeSelector={Boolean(onPageSizeChange)}
            pageSizeOptions={[10, 20, 50, 100]}
          />
        </div>
      </div>
    </div>
  );
}
