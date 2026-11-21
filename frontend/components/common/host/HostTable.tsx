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
 * 主机列表行渲染：勾选列 + 表头/行；加载/空/分页由外层 TableCard 承担。
 * 名称挂 data-detail-name，供 DetailHost 飞入（仅点击/回车写入 flyOrigin）。
 * Host list rows with a checkbox column; TableCard owns loading/empty/pagination.
 * Names carry data-detail-name for DetailHost fly-in (flyOrigin only on click/Enter).
 */

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Cloud,
  Container,
  MoreHorizontal,
  Pencil,
  Server,
  Terminal,
  Trash2,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {
  DropdownMenu,
  type DropdownItem,
} from '@/components/arc/dropdown-menu/dropdown-menu';
import {HostInfo, HostStatus, HostType} from '@/lib/services/host/types';
import {formatDateTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import layout from '@/components/common/layout/list-detail.module.css';

interface HostTableProps {
  hosts: HostInfo[];
  selectedHostId?: number | null;
  selectedIds: ReadonlySet<number>;
  pageAllSelected: boolean;
  pageSomeSelected: boolean;
  onToggleSelect: (id: number, on: boolean) => void;
  onToggleSelectPage: (on: boolean) => void;
  /** 行点击/回车打开详情；传入行元素供名称飞入。Click/Enter open; pass the row for name fly-in. */
  onViewDetail: (host: HostInfo, origin?: HTMLElement | null) => void;
  onEdit: (host: HostInfo) => void;
  onDelete: (host: HostInfo) => void;
  onInstallAgent?: (host: HostInfo) => void;
}

function getHostTypeIcon(hostType: HostType) {
  switch (hostType) {
    case HostType.BARE_METAL:
      return <Server className='size-3.5' aria-hidden />;
    case HostType.DOCKER:
      return <Container className='size-3.5' aria-hidden />;
    case HostType.KUBERNETES:
      return <Cloud className='size-3.5' aria-hidden />;
    default:
      return <Server className='size-3.5' aria-hidden />;
  }
}

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

function connectionLabel(host: HostInfo): string {
  if (host.host_type === HostType.BARE_METAL) {
    return host.ip_address || '-';
  }
  if (host.host_type === HostType.DOCKER) {
    return host.docker_api_url || '-';
  }
  if (host.host_type === HostType.KUBERNETES) {
    return host.k8s_api_url || '-';
  }
  return '-';
}

export function HostTable({
  hosts,
  selectedHostId,
  selectedIds,
  pageAllSelected,
  pageSomeSelected,
  onToggleSelect,
  onToggleSelectPage,
  onViewDetail,
  onEdit,
  onDelete,
  onInstallAgent,
}: HostTableProps) {
  const t = useTranslations();
  const [deleteTarget, setDeleteTarget] = useState<HostInfo | null>(null);

  const onRowKeyDown = (
    e: React.KeyboardEvent<HTMLElement>,
    host: HostInfo,
  ) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    const rows = Array.from(
      e.currentTarget.parentElement?.querySelectorAll<HTMLElement>(
        '[data-mrow]',
      ) ?? [],
    );
    const i = rows.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      rows[i + (e.key === 'ArrowDown' ? 1 : -1)]?.focus();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onViewDetail(host, e.currentTarget);
    }
  };

  return (
    <>
      <div className={layout.rowHead}>
        <Checkbox
          checked={
            pageAllSelected ? true : pageSomeSelected ? 'indeterminate' : false
          }
          onCheckedChange={(v) => onToggleSelectPage(v === true)}
          aria-label={t('host.selectAll')}
        />
        <span>{t('host.name')}</span>
        <span>{t('host.hostType')}</span>
        <span>{t('host.connectionInfo')}</span>
        <span>{t('host.status')}</span>
        <span>{t('host.resources')}</span>
        <span>{t('host.createdAt')}</span>
        <span>{t('host.actions')}</span>
      </div>
      <div className={layout.tableScroll}>
        {hosts.map((host) => (
          <div
            key={host.id}
            data-mrow={host.id}
            data-testid={`host-row-${host.id}`}
            role='button'
            tabIndex={0}
            onClick={(e) => {
              if (
                (e.target as HTMLElement).closest(
                  'button, a, input, select, [role="menu"], [role="menuitem"], [role="checkbox"]',
                )
              ) {
                return;
              }
              onViewDetail(host, e.currentTarget);
            }}
            onKeyDown={(e) => onRowKeyDown(e, host)}
            className={cn(
              layout.row,
              selectedHostId === host.id && layout.rowActive,
            )}
          >
            <Checkbox
              checked={selectedIds.has(host.id)}
              onCheckedChange={(v) => onToggleSelect(host.id, v === true)}
              aria-label={t('host.selectRow', {name: host.name})}
            />
            <div className={layout.rowName}>
              <div className={layout.rowNameInline}>
                {getHostTypeIcon(host.host_type)}
                <span
                  className={layout.name}
                  title={host.name}
                  data-detail-name
                >
                  {host.name}
                </span>
              </div>
              {host.description ? (
                <span className={layout.sub} title={host.description}>
                  {host.description}
                </span>
              ) : (
                <span className={layout.sub}>#{host.id}</span>
              )}
            </div>
            <Badge variant='outline'>
              {t(
                `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
              )}
            </Badge>
            <span
              className={cn(layout.sub, 'truncate')}
              title={connectionLabel(host)}
            >
              {connectionLabel(host)}
            </span>
            <div
              onClick={(e) => e.stopPropagation()}
              onKeyDown={(e) => e.stopPropagation()}
            >
              {host.host_type === HostType.BARE_METAL &&
              host.status === HostStatus.PENDING &&
              onInstallAgent ? (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type='button'
                        onClick={() => onInstallAgent(host)}
                        className='inline-flex cursor-pointer items-center gap-1.5 rounded-full border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-600 select-none transition-all hover:bg-amber-500/20 dark:text-amber-400'
                      >
                        <span className='relative flex size-1.5'>
                          <span className='absolute inline-flex size-full animate-ping rounded-full bg-amber-400 opacity-75' />
                          <span className='relative inline-flex size-1.5 rounded-full bg-amber-500' />
                        </span>
                        <span>{t(`host.statuses.${host.status}`)}</span>
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
            </div>
            <div className='space-y-0.5 text-xs'>
              <div>
                <span className='text-muted-foreground'>CPU </span>
                <span className='font-mono'>
                  {host.cpu_usage?.toFixed(1) || 0}%
                </span>
              </div>
              <div>
                <span className='text-muted-foreground'>Mem </span>
                <span className='font-mono'>
                  {host.memory_usage?.toFixed(1) || 0}%
                </span>
              </div>
              {host.host_type === HostType.BARE_METAL && host.total_memory ? (
                <div className='text-[11px] text-muted-foreground'>
                  {formatBytes(host.total_memory)}
                </div>
              ) : null}
            </div>
            <span className={cn(layout.sub, 'whitespace-nowrap')}>
              {formatDateTz(host.created_at)}
            </span>
            <div
              className={layout.rowActions}
              onClick={(e) => e.stopPropagation()}
              onKeyDown={(e) => e.stopPropagation()}
            >
              {(() => {
                const items: DropdownItem[] = [];
                if (
                  host.host_type === HostType.BARE_METAL &&
                  host.status === HostStatus.PENDING &&
                  onInstallAgent
                ) {
                  items.push({
                    label: t('host.installGuide.installAgent'),
                    icon: <Terminal className='size-4' aria-hidden />,
                    onSelect: () => onInstallAgent(host),
                  });
                }
                items.push(
                  {
                    label: t('common.edit'),
                    icon: <Pencil className='size-4' aria-hidden />,
                    onSelect: () => onEdit(host),
                  },
                  {
                    label: t('common.delete'),
                    icon: <Trash2 className='size-4' aria-hidden />,
                    destructive: true,
                    separatorBefore: true,
                    onSelect: () => setDeleteTarget(host),
                  },
                );
                return (
                  <DropdownMenu
                    trigger={
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        iconOnly
                        aria-label={t('common.moreActions')}
                      >
                        <MoreHorizontal className='size-4' aria-hidden />
                      </Button>
                    }
                    items={items}
                  />
                );
              })()}
            </div>
          </div>
        ))}
      </div>

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('host.deleteHost')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('host.deleteConfirm', {name: deleteTarget?.name || ''})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                const target = deleteTarget;
                setDeleteTarget(null);
                if (target) {
                  onDelete(target);
                }
              }}
            >
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
