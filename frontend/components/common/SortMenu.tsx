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

import {ArrowUpDown} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';

/** 单个排序选项：sort + order 一个组合占一个菜单项。/ One sort option: a sort+order pair per menu item. */
export interface SortMenuOption<K extends string = string> {
  sort: K;
  order: 'asc' | 'desc';
  label: string;
  /** 选项不可用时的原因（悬浮提示）；有值即禁用。/ Why the option is unavailable — shown as a hover hint; disables the item. */
  disabledReason?: string;
}

export interface SortMenuProps<K extends string = string> {
  /** 触发按钮前缀（如「排序」）；当前选中的选项跟在 label 后面。/ Trigger prefix (e.g. "Sort"); the active option's label trails it. */
  label: string;
  options: SortMenuOption<K>[];
  value: {sort: K; order: 'asc' | 'desc'};
  onSelect: (sort: K, order: 'asc' | 'desc') => void;
  testId?: string;
}

/**
 * 通用排序下拉：单列头表达不下多个排序维度时用（GitHub issues 式）。
 * 选项是扁平的 sort+order 组合；禁用项保留在菜单里并附原因，比藏起来更能解释约束。
 * Generic sort dropdown for cases where one column header cannot express several sort
 * dimensions (GitHub issues style). Options are flat sort+order pairs; disabled options
 * stay visible with their reason — hiding them would hide the constraint.
 */
export function SortMenu<K extends string>({
  label,
  options,
  value,
  onSelect,
  testId,
}: SortMenuProps<K>) {
  const active = options.find(
    (option) => option.sort === value.sort && option.order === value.order,
  );
  const radioValue = active ? `${active.sort}:${active.order}` : '';
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8 gap-1.5 text-xs'
          data-testid={testId}
        >
          <ArrowUpDown className='size-3.5' />
          <span className='text-muted-foreground'>{label}</span>
          {active ? <span>{active.label}</span> : null}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='start' className='min-w-44'>
        <DropdownMenuRadioGroup
          value={radioValue}
          onValueChange={(next) => {
            // 选项值固定 sort:order 编码；排序键约定下不含冒号。
            // Option values are encoded as sort:order; sort keys never contain a colon.
            const [sort, order] = next.split(':');
            onSelect(sort as K, order === 'asc' ? 'asc' : 'desc');
          }}
        >
          {options.map((option) => {
            const optionValue = `${option.sort}:${option.order}`;
            const item = (
              <DropdownMenuRadioItem
                key={optionValue}
                value={optionValue}
                disabled={Boolean(option.disabledReason)}
                className='text-xs'
              >
                {option.label}
              </DropdownMenuRadioItem>
            );
            if (!option.disabledReason) {
              return item;
            }
            // Radix 禁用项 pointer-events-none，需要包一层 span 才能挂悬浮提示。
            // A disabled Radix item swallows pointer events, so the hint needs a wrapping span.
            return (
              <Tooltip key={optionValue}>
                <TooltipTrigger asChild>
                  <span className='block'>{item}</span>
                </TooltipTrigger>
                <TooltipContent side='right' className='max-w-[240px] text-xs'>
                  {option.disabledReason}
                </TooltipContent>
              </Tooltip>
            );
          })}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
