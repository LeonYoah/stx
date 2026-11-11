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

/**
 * Hover-expandable installed-cluster stack for the local plugins table.
 * 本地插件表用的可悬停展开已安装集群堆叠。
 */

'use client';

import {useMemo, useState} from 'react';
import {motion, useReducedMotion} from 'motion/react';
import {useTranslations} from 'next-intl';
import {Avatar, AvatarFallback} from '@/components/ui/avatar';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import {
  CLUSTER_STACK_COLLAPSED_OVERLAP,
  CLUSTER_STACK_EXPANDED_GAP,
  getClusterStackInitials,
  PLUGIN_MOTION_INSTANT,
  resolveClusterStackOffset,
} from './plugin-motion';

export interface InstalledClusterStackItem {
  id: number;
  name: string;
}

interface InstalledClusterStackProps {
  clusters: InstalledClusterStackItem[];
  /** Max visible chips before +N overflow. / 溢出前最多展示的芯片数。 */
  maxVisible?: number;
  className?: string;
}

/**
 * Compact overlapping cluster chips; hover enlarges one and gently pushes neighbors.
 * Keeps a fixed row height so the local-plugins table layout stays stable.
 * 紧凑重叠的集群芯片；悬停放大一项并轻推相邻项。固定行高以免撑乱本地插件表布局。
 */
export function InstalledClusterStack({
  clusters,
  maxVisible = 4,
  className,
}: InstalledClusterStackProps) {
  const t = useTranslations();
  const reduceMotion = useReducedMotion() ?? false;
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);

  const visible = useMemo(
    () => clusters.slice(0, Math.max(0, maxVisible)),
    [clusters, maxVisible],
  );
  const overflow = Math.max(0, clusters.length - visible.length);
  const overflowNames = useMemo(
    () => clusters.slice(visible.length).map((item) => item.name),
    [clusters, visible.length],
  );

  if (clusters.length === 0) {
    return (
      <span
        data-testid='plugin-cluster-stack-empty'
        className='text-muted-foreground text-sm whitespace-nowrap'
      >
        {t('plugin.notInstalled')}
      </span>
    );
  }

  const transition = reduceMotion
    ? PLUGIN_MOTION_INSTANT
    : {type: 'spring' as const, stiffness: 420, damping: 28, mass: 0.6};

  const overflowMargin =
    hoveredIndex !== null
      ? CLUSTER_STACK_EXPANDED_GAP
      : CLUSTER_STACK_COLLAPSED_OVERLAP;

  return (
    <TooltipProvider delayDuration={120}>
      <div
        data-testid='plugin-cluster-stack'
        data-reduce-motion={reduceMotion ? 'true' : 'false'}
        className={cn(
          // 固定高度，放大只走 transform，不把表格行撑开。
          // Fixed height; enlargement uses transform only so table rows stay stable.
          'relative inline-flex h-8 max-w-[220px] items-center overflow-visible',
          className,
        )}
        onMouseLeave={() => setHoveredIndex(null)}
        role='group'
        aria-label={t('plugin.installedClusters')}
      >
        {visible.map((cluster, index) => {
          const offset = resolveClusterStackOffset(
            index,
            hoveredIndex,
            reduceMotion,
          );
          const initials = getClusterStackInitials(cluster.name);

          return (
            <Tooltip key={cluster.id}>
              <TooltipTrigger asChild>
                <motion.button
                  type='button'
                  data-testid={`plugin-cluster-stack-item-${cluster.id}`}
                  className='relative inline-flex shrink-0 rounded-full border-0 bg-transparent p-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
                  style={{zIndex: offset.zIndex}}
                  initial={false}
                  animate={{
                    marginLeft: offset.marginLeft,
                    scale: offset.scale,
                  }}
                  transition={transition}
                  onMouseEnter={() => setHoveredIndex(index)}
                  onFocus={() => setHoveredIndex(index)}
                  onBlur={() => setHoveredIndex(null)}
                  aria-label={cluster.name}
                >
                  <Avatar className='size-7 border-2 border-background shadow-sm ring-1 ring-border'>
                    {/* 仅用集群名缩写，不使用用户头像资源。
                        Cluster-name initials only — never invent user photos. */}
                    <AvatarFallback className='bg-primary/10 text-[10px] font-semibold text-primary'>
                      {initials}
                    </AvatarFallback>
                  </Avatar>
                </motion.button>
              </TooltipTrigger>
              <TooltipContent side='top' className='text-xs'>
                {cluster.name}
              </TooltipContent>
            </Tooltip>
          );
        })}

        {overflow > 0 && (
          <Tooltip>
            <TooltipTrigger asChild>
              <motion.span
                data-testid='plugin-cluster-stack-overflow'
                className='relative inline-flex size-7 shrink-0 items-center justify-center rounded-full border-2 border-background bg-muted text-[10px] font-semibold text-muted-foreground shadow-sm ring-1 ring-border'
                style={{zIndex: visible.length + 1}}
                initial={false}
                animate={{marginLeft: overflowMargin}}
                transition={transition}
                aria-label={t('plugin.installedClusters')}
              >
                +{overflow}
              </motion.span>
            </TooltipTrigger>
            <TooltipContent side='top' className='max-w-xs text-xs'>
              {overflowNames.join(', ')}
            </TooltipContent>
          </Tooltip>
        )}

        {/* 安装量可读：总数胶囊，避免仅靠堆叠猜测数量。
            Readable install count pill so quantity is not guessed from the stack alone. */}
        <span
          data-testid='plugin-cluster-stack-count'
          className='ml-2 shrink-0 whitespace-nowrap text-xs tabular-nums text-muted-foreground'
        >
          {clusters.length}
        </span>
      </div>
    </TooltipProvider>
  );
}
