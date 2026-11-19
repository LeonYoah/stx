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
 * Per-engine mini visual used as the card's image hero.
 * 按引擎类型渲染的卡片「图片」区：指标=阈值 sparkline，可用性/同步=24h 健康轨道，事件=触发时间轴。
 * 轴标签与空事件文案由 labels 传入，组件内不写死语言。Axis and empty labels come in via props for i18n.
 */
import {useTranslations} from 'next-intl';
import type {ProtoRule} from './rules-mock';

export interface RuleVisualLabels {
  /** 左轴标签。Left axis label, e.g. "24 小时" / "24h". */
  hours24: string;
  /** 右轴标签。Right axis label, e.g. "现在" / "now". */
  now: string;
  /** 事件规则无事件时的占位文案。Shown when an event rule has no events in range. */
  noEvents24h: string;
}

const W = 280;
const H = 88;
const PAD = 10;

function Sparkline({rule}: {rule: ProtoRule}) {
  const series = rule.visual.series ?? [];
  if (series.length < 2) {
    return null;
  }
  const max = Math.max(100, ...series);
  const inner = {w: W - PAD * 2, h: H - PAD * 2 - 14};
  const x = (i: number) => PAD + (i / (series.length - 1)) * inner.w;
  const y = (v: number) => PAD + inner.h - (v / max) * inner.h;
  const points = series.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`);
  const areaPoints = [`${x(0).toFixed(1)},${y(0).toFixed(1)}`]
    .concat(points.slice(1))
    .concat([`${x(series.length - 1).toFixed(1)},${(PAD + inner.h).toFixed(1)}`, `${x(0).toFixed(1)},${(PAD + inner.h).toFixed(1)}`]);
  const thresholdY = rule.threshold ? y(rule.threshold.value) : null;
  const firing = rule.firingCount > 0;

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className='h-full w-full' role='img' aria-hidden='true'>
      <defs>
        <linearGradient id={`fill-${rule.key}`} x1='0' y1='0' x2='0' y2='1'>
          <stop offset='0%' stopColor={firing ? 'var(--danger, #e5484d)' : 'var(--accent, #0091ff)'} stopOpacity='0.22' />
          <stop offset='100%' stopColor={firing ? 'var(--danger, #e5484d)' : 'var(--accent, #0091ff)'} stopOpacity='0' />
        </linearGradient>
      </defs>
      {thresholdY != null ? (
        <line
          x1={PAD}
          x2={W - PAD}
          y1={thresholdY}
          y2={thresholdY}
          stroke='var(--danger, #e5484d)'
          strokeOpacity='0.55'
          strokeDasharray='4 3'
          strokeWidth='1'
        />
      ) : null}
      <polygon points={areaPoints.join(' ')} fill={`url(#fill-${rule.key})`} />
      <polyline
        points={points.join(' ')}
        fill='none'
        stroke={firing ? 'var(--danger, #e5484d)' : 'var(--accent, #0091ff)'}
        strokeWidth='1.6'
        strokeLinejoin='round'
        strokeLinecap='round'
      />
      {rule.threshold ? (
        <text
          x={W - PAD}
          y={(thresholdY ?? PAD) - 3}
          textAnchor='end'
          fontSize='8.5'
          fill='var(--text-secondary, #6b7280)'
        >
          {rule.threshold.operator} {rule.threshold.value}
          {rule.threshold.unit}
        </text>
      ) : null}
    </svg>
  );
}

function UptimeTrack({rule, labels, showAxis}: {rule: ProtoRule; labels: RuleVisualLabels; showAxis: boolean}) {
  const segments = rule.visual.segments ?? [];
  const inner = {w: W - PAD * 2};
  const gap = 2;
  const bw = (inner.w - gap * (segments.length - 1)) / Math.max(1, segments.length);
  const barY = PAD + 16;
  const barH = H - PAD * 2 - 22;
  // 正常段用中性灰，只有异常段上色，避免满屏绿色抢眼。Healthy segments stay neutral; only anomalies get color.
  const colors: Record<string, string> = {
    ok: 'var(--border-strong, #c9c9d1)',
    bad: 'var(--danger, #e5484d)',
    degraded: 'var(--warning, #f5a524)',
  };
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className='h-full w-full' role='img' aria-hidden='true'>
      {segments.map((seg, i) => (
        <rect
          key={i}
          x={PAD + i * (bw + gap)}
          y={seg === 'ok' ? barY + barH * 0.28 : barY}
          width={bw}
          height={seg === 'ok' ? barH * 0.72 : barH}
          rx='1.5'
          fill={colors[seg]}
          fillOpacity={seg === 'ok' ? 0.5 : 0.9}
        />
      ))}
      {showAxis ? (
        <>
          <text x={PAD} y={H - PAD} fontSize='8.5' fill='var(--text-secondary, #6b7280)'>
            {labels.hours24}
          </text>
          <text x={W - PAD} y={H - PAD} textAnchor='end' fontSize='8.5' fill='var(--text-secondary, #6b7280)'>
            {labels.now}
          </text>
        </>
      ) : null}
    </svg>
  );
}

function EventTrack({rule, labels, compact, showAxis}: {rule: ProtoRule; labels: RuleVisualLabels; compact?: boolean; showAxis: boolean}) {
  const events = rule.visual.events ?? [];
  const inner = {w: W - PAD * 2};
  const axisY = PAD + (H - PAD * 2) / 2;
  if (events.length === 0) {
    // 小缩略图放不下说明文字，只画一条淡轴。Tiny tiles only get a faint axis, text would wrap.
    if (compact) {
      return (
        <svg viewBox={`0 0 ${W} ${H}`} className='h-full w-full' role='img' aria-hidden='true'>
          <line x1={PAD} x2={W - PAD} y1={axisY} y2={axisY} stroke='var(--border, #e5e7eb)' strokeWidth='1.5' strokeDasharray='3 4' />
        </svg>
      );
    }
    return (
      <div className='flex h-full items-center justify-center text-xs text-muted-foreground'>
        {labels.noEvents24h}
      </div>
    );
  }
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className='h-full w-full' role='img' aria-hidden='true'>
      <line
        x1={PAD}
        x2={W - PAD}
        y1={axisY}
        y2={axisY}
        stroke='var(--border, #e5e7eb)'
        strokeWidth='1.5'
      />
      {events.map((hour, i) => {
        const cx = PAD + (hour / 23) * inner.w;
        return (
          <g key={i}>
            <circle cx={cx} cy={axisY} r='4.5' fill='var(--danger, #e5484d)' fillOpacity='0.9' />
            <circle cx={cx} cy={axisY} r='8' fill='var(--danger, #e5484d)' fillOpacity='0.15' />
          </g>
        );
      })}
      {showAxis ? (
        <>
          <text x={PAD} y={H - PAD} fontSize='8.5' fill='var(--text-secondary, #6b7280)'>
            {labels.hours24}
          </text>
          <text x={W - PAD} y={H - PAD} textAnchor='end' fontSize='8.5' fill='var(--text-secondary, #6b7280)'>
            {labels.now}
          </text>
        </>
      ) : null}
    </svg>
  );
}

export function RuleVisual({rule, labels, compact, showAxis = true}: {rule: ProtoRule; labels: RuleVisualLabels; compact?: boolean; showAxis?: boolean}) {
  if (rule.engine === 'tsdb') {
    return <Sparkline rule={rule} />;
  }
  if (rule.engine === 'event') {
    return <EventTrack rule={rule} labels={labels} compact={compact} showAxis={showAxis} />;
  }
  return <UptimeTrack rule={rule} labels={labels} showAxis={showAxis} />;
}

/** RuleVisual 轴文案的本地化标签。Localized labels for RuleVisual axis text. */
export function useRuleVisualLabels(): RuleVisualLabels {
  const t = useTranslations('monitoringProto');
  return {hours24: t('hours24'), now: t('now'), noEvents24h: t('noEvents24h')};
}
