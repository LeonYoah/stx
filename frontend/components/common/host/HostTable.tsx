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
 * Host Table Component
 * 主机表格组件
 *
 * Displays a table of hosts with actions for viewing, editing, and deleting.
 * 显示主机表格，支持查看、编辑和删除操作。
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
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {Eye, Pencil, Trash2, Server, Container, Cloud, Search, Terminal} from 'lucide-react';
import {useGSAP} from '@gsap/react';
import {TableLoadingBar, TableSkeletonRows} from '@/components/common/layout';
import {animateTableRows} from '@/lib/animations/gsap-motion';
import {HostInfo, HostType, HostStatus} from '@/lib/services/host/types';

interface HostTableProps {
  hosts: HostInfo[];
  loading: boolean;
  currentPage: number;
  totalPages: number;
  total: number;
  pageSize?: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  onViewDetail: (host: HostInfo) => void;
  onEdit: (host: HostInfo) => void;
  onDelete: (host: HostInfo) => void;
  onDiscoverCluster?: (host: HostInfo) => void;
  onInstallAgent?: (host: HostInfo) => void;
}

/**
 * Get host type icon
 * 获取主机类型图标
 */
function getHostTypeIcon(hostType: HostType) {
  switch (hostType) {
    case HostType.BARE_METAL:
      return <Server className='h-4 w-4' />;
    case HostType.DOCKER:
      return <Container className='h-4 w-4' />;
    case HostType.KUBERNETES:
      return <Cloud className='h-4 w-4' />;
    default:
      return <Server className='h-4 w-4' />;
  }
}


/**
 * Get status badge variant
 * 获取状态徽章变体
 */
function getStatusBadgeVariant(
  status: HostStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case HostStatus.CONNECTED:
      return 'default';
    case HostStatus.PENDING:
      return 'secondary';
    case HostStatus.OFFLINE:
      return 'outline';
    case HostStatus.ERROR:
      return 'destructive';
    default:
      return 'secondary';
  }
}

/**
 * Format bytes to human readable string
 * 格式化字节为人类可读字符串
 */
function formatBytes(bytes: number | undefined): string {
  if (!bytes) {
    return '-';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let unitIndex = 0;
  let value = bytes;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  return `${value.toFixed(1)} ${units[unitIndex]}`;
}

/**
 * Host Table Component
 * 主机表格组件
 */
export function HostTable({
  hosts,
  loading,
  currentPage,
  totalPages,
  total,
  pageSize = 10,
  onPageChange,
  onPageSizeChange,
  onViewDetail,
  onEdit,
  onDelete,
  onDiscoverCluster,
  onInstallAgent,
}: HostTableProps) {
  const t = useTranslations();
  const tableContainerRef = useRef<HTMLDivElement>(null);

  // 主机数据更新后执行平滑交错入场动效
  // Trigger staggered entrance animation when hosts data updates
  useGSAP(
    () => {
      if (hosts.length > 0 && !loading) {
        animateTableRows('.host-data-row');
      }
    },
    {dependencies: [hosts, loading], scope: tableContainerRef},
  );

  return (
    <div ref={tableContainerRef} className='space-y-4'>
      <div className='border rounded-lg relative overflow-hidden bg-card/40 shadow-xs'>
        <TableLoadingBar loading={loading} />
        <div className='overflow-x-auto'>
          <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-[50px]'>ID</TableHead>
              <TableHead>{t('host.name')}</TableHead>
              <TableHead>{t('host.hostType')}</TableHead>
              <TableHead>{t('host.connectionInfo')}</TableHead>
              <TableHead>{t('host.status')}</TableHead>
              <TableHead>{t('host.resources')}</TableHead>
              <TableHead>{t('host.createdAt')}</TableHead>
              <TableHead className='min-w-[140px]'>{t('host.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && hosts.length === 0 ? (
              <TableSkeletonRows columns={8} rows={10} />
            ) : hosts.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={8}
                  className='text-center py-12 text-muted-foreground'
                >
                  {t('host.noHosts')}
                </TableCell>
              </TableRow>
            ) : (
              hosts.map((host) => (
                <TableRow
                  key={host.id}
                  className={`host-data-row transition-opacity duration-200 ${
                    loading ? 'opacity-60 pointer-events-none' : ''
                  }`}
                >
                  <TableCell>{host.id}</TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      {getHostTypeIcon(host.host_type)}
                      <span className='font-medium'>{host.name}</span>
                    </div>
                    {host.description && (
                      <p className='text-xs text-muted-foreground mt-1 truncate max-w-[200px]'>
                        {host.description}
                      </p>
                    )}
                  </TableCell>
                  <TableCell>
                    <Badge variant='outline'>
                      {t(`host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {host.host_type === HostType.BARE_METAL && (
                      <div className='text-sm'>{host.ip_address || '-'}</div>
                    )}
                    {host.host_type === HostType.DOCKER && (
                      <div className='text-sm truncate max-w-[150px]'>
                        {host.docker_api_url || '-'}
                      </div>
                    )}
                    {host.host_type === HostType.KUBERNETES && (
                      <div className='text-sm'>
                        <div className='truncate max-w-[150px]'>
                          {host.k8s_api_url || '-'}
                        </div>
                        {host.k8s_namespace && (
                          <div className='text-xs text-muted-foreground'>
                            ns: {host.k8s_namespace}
                          </div>
                        )}
                      </div>
                    )}
                  </TableCell>
                  <TableCell>
                    {host.host_type === HostType.BARE_METAL &&
                    host.status === HostStatus.PENDING &&
                    onInstallAgent ? (
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type='button'
                              onClick={() => onInstallAgent(host)}
                              className='inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium border border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400 hover:bg-amber-500/20 transition-all cursor-pointer select-none'
                            >
                              <span className='relative flex h-1.5 w-1.5'>
                                <span className='animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75' />
                                <span className='relative inline-flex rounded-full h-1.5 w-1.5 bg-amber-500' />
                              </span>
                              <span>{t(`host.statuses.${host.status}`)}</span>
                              <span className='text-[10px] underline underline-offset-2 opacity-85'>
                                {t('host.installGuide.installAgent')}
                              </span>
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>
                            {t('host.installGuide.installAgentTooltip')}
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    ) : (
                      <Badge variant={getStatusBadgeVariant(host.status)}>
                        {t(`host.statuses.${host.status}`)}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className='text-sm space-y-1'>
                      <div className='flex items-center gap-2'>
                        <span className='text-muted-foreground'>CPU:</span>
                        <span>{host.cpu_usage?.toFixed(1) || 0}%</span>
                      </div>
                      <div className='flex items-center gap-2'>
                        <span className='text-muted-foreground'>Mem:</span>
                        <span>{host.memory_usage?.toFixed(1) || 0}%</span>
                      </div>
                      {host.host_type === HostType.BARE_METAL && host.total_memory && (
                        <div className='text-xs text-muted-foreground'>
                          {formatBytes(host.total_memory)}
                        </div>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    {new Date(host.created_at).toLocaleDateString()}
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-1'>
                      {/* Install Agent Button - prominent action for pending bare-metal hosts */}
                      {/* 部署 Agent 按钮 - 物理机待连接时置前展示清晰引导 */}
                      {host.host_type === HostType.BARE_METAL &&
                        host.status === HostStatus.PENDING &&
                        onInstallAgent && (
                          <TooltipProvider>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  variant='outline'
                                  size='sm'
                                  className='h-7 px-2 text-xs border-amber-500/40 text-amber-600 dark:text-amber-400 bg-amber-500/10 hover:bg-amber-500/20 gap-1 font-medium'
                                  onClick={() => onInstallAgent(host)}
                                >
                                  <Terminal className='h-3.5 w-3.5' />
                                  <span>{t('host.installGuide.installAgent')}</span>
                                </Button>
                              </TooltipTrigger>
                              <TooltipContent>
                                {t('host.installGuide.installAgentTooltip')}
                              </TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        )}

                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant='ghost'
                              size='icon'
                              onClick={() => onViewDetail(host)}
                            >
                              <Eye className='h-4 w-4' />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t('common.view')}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>

                      {/* Discover Cluster Button - only for bare_metal hosts with installed agent */}
                      {/* 发现集群按钮 - 仅对已安装 Agent 的物理机主机显示 */}
                      {host.host_type === HostType.BARE_METAL && onDiscoverCluster && (
                        <TooltipProvider>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant='ghost'
                                size='icon'
                                onClick={() => onDiscoverCluster(host)}
                                disabled={host.status !== HostStatus.CONNECTED}
                              >
                                <Search className='h-4 w-4' />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>
                              {host.status === HostStatus.PENDING
                                ? t('discovery.agentNotInstalled')
                                : host.status === HostStatus.OFFLINE
                                  ? t('discovery.agentOffline')
                                  : t('discovery.discoverCluster')}
                            </TooltipContent>
                          </Tooltip>
                        </TooltipProvider>
                      )}

                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant='ghost'
                              size='icon'
                              onClick={() => onEdit(host)}
                            >
                              <Pencil className='h-4 w-4' />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t('common.edit')}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>

                      <AlertDialog>
                        <TooltipProvider>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <AlertDialogTrigger asChild>
                                <Button variant='ghost' size='icon'>
                                  <Trash2 className='h-4 w-4 text-destructive' />
                                </Button>
                              </AlertDialogTrigger>
                            </TooltipTrigger>
                            <TooltipContent>{t('common.delete')}</TooltipContent>
                          </Tooltip>
                        </TooltipProvider>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>
                              {t('host.deleteHost')}
                            </AlertDialogTitle>
                            <AlertDialogDescription>
                              {t('host.deleteConfirm', {name: host.name})}
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>
                              {t('common.cancel')}
                            </AlertDialogCancel>
                            <AlertDialogAction onClick={() => onDelete(host)}>
                              {t('common.delete')}
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        </div>

        {/* 底部分页栏 / Table Footer Pagination */}
        <div className='border-t bg-muted/10 px-4 py-2.5'>
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
