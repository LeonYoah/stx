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
 * 集群卡片视图项：单层扁平画布 + 0.5px 细线底栏 + useViewMorph 共享元素形变锚点，
 * 配合 ClusterMain 卡片视图与 DetailHost 速览栏联动。
 * Cluster grid card item: flat single-layer surface with 0.5px hairline footer and
 * useViewMorph shared layout anchors for ClusterMain grid view and DetailHost integration.
 */
import type {RefObject} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {motion} from 'motion/react';
import {
  Activity,
  ArrowUpRight,
  MoreHorizontal,
  Pencil,
  Play,
  RotateCcw,
  Server,
  Square,
  Trash2,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {
  DropdownMenu,
  type DropdownItem,
} from '@/components/arc/dropdown-menu/dropdown-menu';
import {NeutralChip, SoftChip} from '@/components/common/layout';
import {useViewMorph} from '@/lib/animations/view-morph';
import {
  OperationType,
  type ClusterInfo,
} from '@/lib/services/cluster/types';
import {cn} from '@/lib/utils';
import {ClusterStatusChip} from './ClusterStatusChip';
import {getClusterActionFlags} from './useClusterLifecycle';

export interface ClusterCardProps {
  cluster: ClusterInfo;
  active?: boolean;
  isDefault?: boolean;
  operating?: boolean;
  flyOrigin?: RefObject<HTMLElement | null>;
  onSelect: (cluster: ClusterInfo) => void;
  onEdit: (cluster: ClusterInfo) => void;
  onLifecycle: (cluster: ClusterInfo, operation: OperationType) => void;
  onDelete: (cluster: ClusterInfo) => void;
}

export function ClusterCard({
  cluster,
  active = false,
  isDefault = false,
  operating = false,
  flyOrigin,
  onSelect,
  onEdit,
  onLifecycle,
  onDelete,
}: ClusterCardProps) {
  const t = useTranslations();
  const router = useRouter();
  const morph = useViewMorph('clusters');

  const {canStart, canStop, canRestart, canDelete} =
    getClusterActionFlags(cluster);
  const modeKey = `cluster.modes.${cluster.deployment_mode}`;
  const modeLabel = t.has(modeKey) ? t(modeKey) : cluster.deployment_mode;
  const totalNodes = cluster.node_count ?? 0;
  const onlineNodes = cluster.online_nodes ?? 0;
  const hasOfflineNodes = totalNodes > 0 && onlineNodes < totalNodes;

  const menuItems: DropdownItem[] = [
    {
      label: t('cluster.list.openConsole'),
      icon: <ArrowUpRight className='size-3.5' aria-hidden />,
      onSelect: () => router.push(`/clusters/${cluster.id}`),
    },
    {
      label: t('common.edit'),
      icon: <Pencil className='size-3.5' aria-hidden />,
      onSelect: () => onEdit(cluster),
    },
    {
      label: t('stUpgrade.entry'),
      icon: <Activity className='size-3.5' aria-hidden />,
      onSelect: () => router.push(`/clusters/${cluster.id}/upgrade/prepare`),
    },
    {
      label: t('cluster.start'),
      icon: <Play className='size-3.5' aria-hidden />,
      disabled: !canStart || operating,
      separatorBefore: true,
      onSelect: () => onLifecycle(cluster, OperationType.START),
    },
    {
      label: t('cluster.stop'),
      icon: <Square className='size-3.5' aria-hidden />,
      disabled: !canStop || operating,
      onSelect: () => onLifecycle(cluster, OperationType.STOP),
    },
    {
      label: t('cluster.restart'),
      icon: <RotateCcw className='size-3.5' aria-hidden />,
      disabled: !canRestart || operating,
      onSelect: () => onLifecycle(cluster, OperationType.RESTART),
    },
    {
      label: t('common.delete'),
      icon: <Trash2 className='size-3.5' aria-hidden />,
      disabled: !canDelete || operating,
      destructive: true,
      separatorBefore: true,
      onSelect: () => onDelete(cluster),
    },
  ];

  const handleOpen = (el: HTMLElement | null) => {
    if (flyOrigin) {
      flyOrigin.current = el;
    }
    onSelect(cluster);
  };

  return (
    <div
      role='button'
      tabIndex={0}
      aria-pressed={active}
      data-testid={`cluster-card-${cluster.id}`}
      onClick={(e) => handleOpen(e.currentTarget)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleOpen(e.currentTarget);
        }
      }}
      className={cn(
        'group flex min-w-0 cursor-pointer flex-col rounded-(--radius-panel) border bg-card text-left transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)',
        active
          ? 'border-(--accent) bg-(--accent-subtle)/40 shadow-[inset_0_0_0_1px_var(--accent)]'
          : 'border-(--border) hover:bg-muted/30',
      )}
    >
      {/* 卡片主体 / Card body */}
      <div className='flex flex-1 flex-col gap-3 px-4 pt-4 pb-3.5'>
        <div className='flex items-start justify-between gap-2'>
          <div className='min-w-0 flex-1 space-y-1'>
            <div className='flex min-w-0 items-center gap-1.5'>
              <motion.span
                layoutId={morph.id(String(cluster.id), 'title')}
                transition={morph.transition}
                data-detail-name
                className='truncate text-sm font-semibold text-foreground'
                title={cluster.name}
              >
                {cluster.name}
              </motion.span>
              {isDefault ? (
                <NeutralChip>{t('cluster.defaultBadge')}</NeutralChip>
              ) : null}
              {cluster.config_error ? (
                <span data-testid={`cluster-config-error-${cluster.id}`}>
                  <SoftChip tone='amber'>
                    {t('cluster.list.configReadFailed')}
                  </SoftChip>
                </span>
              ) : null}
            </div>
            <p
              className='truncate text-xs text-muted-foreground'
              title={cluster.description || cluster.cluster_code || ''}
            >
              {cluster.description || cluster.cluster_code || `#${cluster.id}`}
            </p>
          </div>
          <div
            className='shrink-0'
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => e.stopPropagation()}
          >
            <DropdownMenu
              items={menuItems}
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
            />
          </div>
        </div>

        {/* 核心元信息紧凑横条 / Compact metadata row */}
        <div className='flex flex-wrap items-center gap-2 pt-0.5'>
          <motion.span
            layoutId={morph.id(String(cluster.id), 'status')}
            transition={morph.transition}
            className='inline-flex'
          >
            <ClusterStatusChip cluster={cluster} />
          </motion.span>
          <motion.span
            layoutId={morph.id(String(cluster.id), 'mode')}
            transition={morph.transition}
            className='inline-flex'
          >
            <NeutralChip>{modeLabel}</NeutralChip>
          </motion.span>
          {cluster.version ? (
            <span className='font-mono text-xs text-muted-foreground tabular-nums'>
              v{cluster.version}
            </span>
          ) : null}
        </div>
      </div>

      {/* 0.5px 细线底栏 / 0.5px hairline footer */}
      <div
        className='flex items-center justify-between gap-2 border-t-[0.5px] border-border/80 px-4 py-2.5'
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => e.stopPropagation()}
      >
        <div className='flex items-center gap-1.5 text-xs tabular-nums'>
          <Server className='size-3.5 text-muted-foreground' aria-hidden />
          <span
            className={cn(
              'font-medium',
              hasOfflineNodes
                ? 'text-rose-600 dark:text-rose-400'
                : 'text-foreground',
            )}
          >
            {onlineNodes}
          </span>
          <span className='text-muted-foreground'>
            / {totalNodes} {t('cluster.nodes')}
          </span>
        </div>

        <Button
          type='button'
          variant='ghost'
          size='sm'
          onClick={() => router.push(`/clusters/${cluster.id}`)}
        >
          {t('cluster.list.openConsole')}
          <ArrowUpRight className='size-3.5' aria-hidden />
        </Button>
      </div>
    </div>
  );
}
