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
import {motion, useReducedMotion} from 'motion/react';
import {useTranslations} from 'next-intl';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {HELP_HINT_DELAY_MS} from '@/components/common/HelpHint';
import {cn} from '@/lib/utils';
import {
  CLUSTER_STACK_COLLAPSED_OVERLAP,
  CLUSTER_STACK_EXPANDED_GAP,
  CLUSTER_STACK_HOVER_SCALE,
  PLUGIN_MOTION_INSTANT,
} from '@/components/common/plugin/plugin-motion';
import type {MarketplaceBubbleMember} from './types';

/**
 * Overlapping circular bubbles with hover enlarge / neighbor push.
 * 重叠圆形气泡；悬停放大并推让相邻项。
 */
export function MarketplaceBubbleStack({
  members,
  maxVisible = 5,
  className,
}: {
  members: MarketplaceBubbleMember[];
  maxVisible?: number;
  className?: string;
}) {
  const t = useTranslations('marketplaceUi');
  const reduceMotion = useReducedMotion() ?? false;
  const [hovered, setHovered] = useState<number | null>(null);

  if (members.length === 0) {
    return null;
  }

  const visible = members.slice(0, maxVisible);
  const overflow = Math.max(0, members.length - visible.length);
  const overflowNames = members.slice(visible.length).map((m) => m.label);
  const transition = reduceMotion
    ? PLUGIN_MOTION_INSTANT
    : {type: 'spring' as const, stiffness: 420, damping: 28, mass: 0.55};

  return (
    <div
      className={cn('relative inline-flex h-9 items-center', className)}
      onMouseLeave={() => setHovered(null)}
      role='group'
      aria-label={t('bubbleStackLabel')}
    >
      {visible.map((member, index) => {
        const expanded = hovered !== null;
        const isHovered = hovered === index;
        const marginLeft =
          index === 0
            ? 0
            : expanded
              ? CLUSTER_STACK_EXPANDED_GAP
              : CLUSTER_STACK_COLLAPSED_OVERLAP;
        const scale =
          !reduceMotion && isHovered ? CLUSTER_STACK_HOVER_SCALE : 1;

        return (
          <Tooltip key={member.id} delayDuration={HELP_HINT_DELAY_MS}>
            <TooltipTrigger asChild>
              <motion.button
                type='button'
                className={cn(
                  'relative inline-flex size-8 shrink-0 items-center justify-center rounded-full border-2 border-background text-[10px] font-semibold shadow-sm ring-1 ring-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  member.toneClass || 'bg-primary/15 text-primary',
                )}
                style={{zIndex: isHovered ? 40 : index + 1}}
                initial={false}
                animate={{marginLeft, scale}}
                transition={transition}
                onMouseEnter={() => setHovered(index)}
                onFocus={() => setHovered(index)}
                onBlur={() => setHovered(null)}
                aria-label={member.label}
              >
                <span className='sr-only'>{member.label}</span>
                <span aria-hidden='true'>{member.mark}</span>
              </motion.button>
            </TooltipTrigger>
            <TooltipContent side='top' className='text-xs'>
              {member.label}
            </TooltipContent>
          </Tooltip>
        );
      })}

      {overflow > 0 && (
        <Tooltip delayDuration={HELP_HINT_DELAY_MS}>
          <TooltipTrigger asChild>
            <motion.button
              type='button'
              className='relative inline-flex size-8 shrink-0 items-center justify-center rounded-full border-2 border-background bg-muted text-[10px] font-semibold text-muted-foreground shadow-sm ring-1 ring-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
              style={{zIndex: visible.length + 1}}
              initial={false}
              animate={{
                marginLeft:
                  hovered !== null
                    ? CLUSTER_STACK_EXPANDED_GAP
                    : CLUSTER_STACK_COLLAPSED_OVERLAP,
              }}
              transition={transition}
              aria-label={t('overflowAria', {
                names: overflowNames.join(t('listSeparator')),
              })}
            >
              +{overflow}
            </motion.button>
          </TooltipTrigger>
          <TooltipContent side='top' className='max-w-xs text-xs'>
            {overflowNames.join(t('listSeparator'))}
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
