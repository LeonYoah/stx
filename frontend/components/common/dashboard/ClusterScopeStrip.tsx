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

import type {CSSProperties, ReactNode} from 'react';
import {Skeleton} from '@/components/ui/skeleton';
import {cn} from '@/lib/utils';
import {DASHBOARD_CELL_DIVIDER} from './KpiStrip';
import {useRovingKeys} from './use-roving-keys';

export interface ClusterScopeItem {
  /** 'all' 或集群 id 字符串。/ 'all' or a cluster id as a string. */
  key: string;
  /** 名称行（集群名 · 版本 / 全部集群）。/ Name line (cluster name · version / all clusters). */
  title: ReactNode;
  /** 说明行（节点 x/y · 运行 n · 堆 n% · 告警 n）。/ Description line (nodes, jobs, heap, alerts). */
  description: ReactNode;
  /** 右侧健康徽标（「全部集群」格没有）。/ Right-side health badge (absent on the "all" cell). */
  meta?: ReactNode;
}

/**
 * 集群范围选择条：与 KpiStrip / ThroughputTabs 同套的「底板 + 1px 分隔线」格子，
 * 首格「全部集群」，后面一集群一格；选中格顶部 2px --accent 色条 + 淡底。
 * roving tabindex + ←/→/Home/End（移动即选中，与 ThroughputTabs 一致）。
 * Cluster scope strip: the same "board + 1px divider" cells as KpiStrip /
 * ThroughputTabs — an "all clusters" cell first, then one cell per cluster; the
 * selected cell gets a 2px --accent bar on top and a muted background. Roving
 * tabindex + arrow/Home/End keys (moving selects, same as ThroughputTabs).
 */
export function ClusterScopeStrip({
  items,
  value,
  onValueChange,
  label,
}: {
  items: ClusterScopeItem[];
  value: string;
  onValueChange: (key: string) => void;
  /** radiogroup 的 aria-label。/ aria-label of the radiogroup. */
  label: string;
}) {
  const {refAt, onKeyDown} = useRovingKeys<HTMLButtonElement>(
    items.length,
    (index) => {
      const item = items[index];
      if (item) {
        onValueChange(item.key);
      }
    },
  );

  return (
    <div className='overflow-hidden rounded-xl border border-border/70 bg-card'>
      <div
        role='radiogroup'
        aria-label={label}
        className='-mr-px -mb-px grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
        data-testid='home-cluster-scope'
      >
        {items.map((item, index) => {
          const selected = item.key === value;
          return (
            <button
              key={item.key}
              ref={refAt(index)}
              type='button'
              role='radio'
              aria-checked={selected}
              tabIndex={selected ? 0 : -1}
              data-testid={`home-cluster-scope-${item.key}`}
              onClick={() => onValueChange(item.key)}
              onKeyDown={(event) => onKeyDown(event, index)}
              className={cn(
                DASHBOARD_CELL_DIVIDER,
                'relative flex flex-col gap-1.5 px-4 py-3 text-left transition-colors',
                'focus-visible:bg-muted/30',
                'aria-checked:bg-muted/50',
              )}
              // 焦点框向内收 2px，避免被底板 overflow-hidden 裁掉；与 ThroughputTabs 同一约定。
              // Pull the keyboard focus outline 2px inward so the board's
              // overflow-hidden does not clip it, same contract as ThroughputTabs.
              style={
                {'--focus-outline-offset': '-2px'} as CSSProperties
              }
            >
              {selected ? (
                <span
                  aria-hidden
                  className='absolute inset-x-0 top-0 h-0.5 bg-(--accent)'
                />
              ) : null}
              <span className='flex items-center justify-between gap-2'>
                <span className='min-w-0 truncate text-xs font-medium'>
                  {item.title}
                </span>
                {item.meta}
              </span>
              <span className='text-xs text-muted-foreground'>
                {item.description}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

/**
 * 选择条骨架：同一底板与网格，每格三条骨架，高度与真实格子一致，不塌陷。
 * Strip skeleton: same board and grid with three bars per cell, matching the
 * real cells' height so nothing collapses.
 */
export function ClusterScopeStripSkeleton({count}: {count: number}) {
  return (
    <div
      className='overflow-hidden rounded-xl border border-border/70 bg-card'
      data-testid='home-cluster-scope-loading'
    >
      <div className='-mr-px -mb-px grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'>
        {Array.from({length: count}).map((_, index) => (
          <div
            key={index}
            className={cn(DASHBOARD_CELL_DIVIDER, 'px-4 py-3')}
          >
            <Skeleton className='h-3 w-24' />
            <Skeleton className='mt-2.5 h-3 w-36' />
            <Skeleton className='mt-1.5 h-3 w-28' />
          </div>
        ))}
      </div>
    </div>
  );
}
