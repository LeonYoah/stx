/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Composition Chart (https://beui.dev/charts/composition-chart).
 * Registry: https://beui.dev/r/composition-chart.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 *
 * 本地改动：beUI 的 lib/i18n 指向 components/charts/beui-i18n.tsx，不占用 STX next-intl 的 frontend/lib/i18n/。
 * Local change: beUI's lib/i18n points at components/charts/beui-i18n.tsx so it does not shadow STX's next-intl
 * frontend/lib/i18n/. Local STX adaptations remain under the same MIT terms and are not relicensed as Apache-2.0.
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

'use client';

import {motion} from 'motion/react';
import {type PointerEvent, useId, useRef, useState} from 'react';
import {Tooltip} from '@/components/motion/tooltip';
import {SPRING_LAYOUT} from '@/lib/ease';
import {cn} from '@/lib/utils';
import {useCompositionChart} from './context';
import {compositionArea} from './model';
import {CompositionTooltipContent} from './tooltip-content';

export function CompositionChartPlot({className}: {className?: string}) {
  const {
    columns,
    rows,
    column,
    index,
    select,
    view,
    highlight,
    reduce,
    canHover,
    t,
  } = useCompositionChart();
  const anchorRef = useRef<HTMLDivElement>(null);
  const tooltipId = useId();
  const pointerDriven = useRef(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  // Under a finger the crosshair tracks the touch directly; a glide reads as lag.
  const [touching, setTouching] = useState(false);
  const inspectPointer = (event: PointerEvent<HTMLDivElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect();
    if (!bounds.width || !bounds.height || !columns.length) {return;}
    const x = Math.min(
      1,
      Math.max(0, (event.clientX - bounds.left) / bounds.width),
    );
    select(
      columns[Math.min(columns.length - 1, Math.floor(x * columns.length))].id,
    );
  };
  if (!columns.length || !rows.length)
    {return (
      <p
        className={cn(
          'flex min-h-64 items-center justify-center text-sm text-muted-foreground',
          className,
        )}
      >
        {t('empty')}
      </p>
    );}
  return (
    <div className={cn('min-w-0 space-y-3', className)}>
      <div className='flex justify-between text-[11px] text-muted-foreground'>
        <span>
          {column?.id}
          {column?.valid ? '' : ` · ${t('noData')}`}
        </span>
        <span className='font-mono'>100%</span>
      </div>
      <div
        ref={anchorRef}
        onPointerLeave={() => setTooltipOpen(false)}
        className='relative h-64 has-focus-visible:outline-2 has-focus-visible:outline-offset-4 has-focus-visible:outline-ring sm:h-80'
        onPointerEnter={(event) => {
          if (event.pointerType !== 'touch' && !event.buttons) {
            inspectPointer(event);
            setTooltipOpen(true);
          }
        }}
        onPointerDown={(event) => {
          pointerDriven.current = true;
          if (event.pointerType === 'touch') {setTouching(true);}
          inspectPointer(event);
          setTooltipOpen(true);
        }}
        onPointerUp={() => {
          pointerDriven.current = false;
          setTouching(false);
        }}
        onPointerCancel={() => {
          pointerDriven.current = false;
          setTouching(false);
          setTooltipOpen(false);
        }}
        onPointerMove={(event) => {
          if (event.pointerType === 'touch' ? event.buttons === 1 : canHover) {
            inspectPointer(event);
          }
        }}
      >
        <svg
          aria-hidden='true'
          viewBox='0 0 100 100'
          preserveAspectRatio='none'
          className='size-full overflow-visible'
        >
          {[0, 25, 50, 75, 100].map((y) => (
            <line
              key={y}
              x1='0'
              x2='100'
              y1={y}
              y2={y}
              stroke='currentColor'
              strokeWidth='1'
              vectorEffect='non-scaling-stroke'
              className='text-border'
            />
          ))}
          {view === 'area'
            ? rows.map((row, r) => (
                <motion.path
                  key={row.id}
                  d={compositionArea(columns, r)}
                  fill={row.color}
                  initial={{opacity: 0}}
                  animate={{
                    opacity: highlight && highlight !== row.id ? 0.18 : 0.9,
                  }}
                  transition={{duration: 0.18}}
                />
              ))
            : columns.map(
                (col, i) =>
                  col.valid &&
                  col.segments.map((segment) => (
                    <motion.rect
                      key={`${col.id}/${segment.id}`}
                      x={(i / columns.length) * 100 + 10 / columns.length}
                      y='0'
                      width={80 / columns.length}
                      height='1'
                      fill={segment.color}
                      initial={false}
                      animate={{
                        y: 100 - segment.offset - segment.share,
                        scaleY: segment.share,
                        opacity:
                          highlight && highlight !== segment.id ? 0.18 : 0.9,
                      }}
                      style={{originY: '0px', originX: '0px'}}
                      transition={reduce ? {duration: 0} : SPRING_LAYOUT}
                    />
                  )),
              )}
          <motion.line
            initial={false}
            animate={{x: ((index + 0.5) / columns.length) * 100}}
            transition={reduce || touching ? {duration: 0} : SPRING_LAYOUT}
            x1='0'
            x2='0'
            y1='0'
            y2='100'
            stroke='currentColor'
            strokeWidth='1'
            vectorEffect='non-scaling-stroke'
            strokeDasharray='3 3'
            className='text-foreground/70'
          />
        </svg>
        <input
          type='range'
          aria-label={t('inspect')}
          min={0}
          max={columns.length - 1}
          step={1}
          value={index}
          aria-valuetext={column?.id}
          aria-describedby={tooltipOpen ? tooltipId : undefined}
          onFocus={() => setTooltipOpen(true)}
          onBlur={() => {
            pointerDriven.current = false;
            setTooltipOpen(false);
          }}
          onPointerDown={() => setTooltipOpen(true)}
          onKeyDown={(event) => {
            pointerDriven.current = false;
            if (event.key === 'Escape') {setTooltipOpen(false);}
          }}
          onChange={(event) => {
            // The range thumb and equal-width chart columns round differently.
            // Pointer selection has one source; native changes are for keyboard/AT.
            if (pointerDriven.current) {return;}
            select(columns[Number(event.target.value)].id);
            setTooltipOpen(true);
          }}
          className='absolute inset-0 h-full w-full cursor-crosshair opacity-0'
        />
      </div>
      <Tooltip
        id={tooltipId}
        anchorRef={anchorRef}
        followCursor
        anchorPoint={{x: (index + 0.5) / columns.length, y: 0.3}}
        side='top'
        open={tooltipOpen}
        onOpenChange={setTooltipOpen}
        className='max-w-[calc(100vw-1rem)]'
        content={<CompositionTooltipContent />}
      />
      <div className='flex justify-between gap-4 text-[11px] text-muted-foreground'>
        <span>{columns[0].id}</span>
        <span>{columns.at(-1)?.id}</span>
      </div>
    </div>
  );
}
