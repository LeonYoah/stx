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

import Link from 'next/link';
import {useId, useState} from 'react';
import {useTranslations} from 'next-intl';
import {ArrowUpRight, Copy, FileSearch, Search} from 'lucide-react';
import {toast} from '@/lib/toast';
import type {DiagnosticsInspectionFinding} from '@/lib/services/diagnostics';
import {Button} from '@/components/ui/button';
import {cn} from '@/lib/utils';
import {localizeDiagnosticsText} from './text-utils';
import {writeClipboard} from '@/components/arc/lib/use-copy-feedback';

interface InspectionFindingCardProps {
  finding: DiagnosticsInspectionFinding;
  index: number;
  origin: string;
  compact?: boolean;
}

const severityStyles = {
  critical: {
    border: 'border-l-rose-600 dark:border-l-rose-400',
    text: 'text-rose-600 dark:text-rose-400',
  },
  warning: {
    border: 'border-l-amber-500 dark:border-l-amber-400',
    text: 'text-amber-600 dark:text-amber-400',
  },
  info: {
    border: 'border-l-sky-600 dark:border-l-sky-400',
    text: 'text-sky-600 dark:text-sky-400',
  },
} as const;

// 一条发现先呈现观测值，检查代码和排查方向由读者自行展开。
// Show observed evidence first; leave check metadata and investigation hints on demand.
export function InspectionFindingCard({
  finding,
  index,
  origin,
  compact = false,
}: InspectionFindingCardProps) {
  const t = useTranslations('diagnosticsCenter.inspections');
  const title = localizeDiagnosticsText(finding.check_name || finding.summary);
  const observation = localizeDiagnosticsText(finding.summary);
  const evidence = localizeDiagnosticsText(finding.evidence_summary);
  const recommendation = localizeDiagnosticsText(finding.recommendation);
  const hasDetails = Boolean(
    finding.category ||
    finding.check_code ||
    recommendation ||
    finding.related_error_group_id,
  );
  const severity = severityStyles[finding.severity];
  const [detailsOpen, setDetailsOpen] = useState(false);
  const detailsId = useId();

  const copyRecommendation = async () => {
    try {
      await writeClipboard(recommendation);
      toast.success(t('evidence.copied'));
    } catch {
      toast.error(t('evidence.copyFailed'));
    }
  };

  return (
    <article
      className={`border border-border/70 border-l-[3px] bg-card ${severity.border} ${compact ? 'px-4 py-4' : 'px-5 py-6 sm:px-7 sm:py-7'}`}
    >
      <div className='flex items-start gap-4 sm:gap-6'>
        <span
          aria-hidden='true'
          className='shrink-0 pt-0.5 font-mono text-xs tabular-nums text-muted-foreground'
        >
          {String(index + 1).padStart(2, '0')}
        </span>
        <div className='min-w-0 flex-1'>
          <div className='mb-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs'>
            <span className={`font-semibold ${severity.text}`}>
              {t(`severity.${finding.severity}`)}
            </span>
            {origin !== '-' && (
              <span className='min-w-0 break-words text-muted-foreground'>
                {origin}
              </span>
            )}
          </div>
          <h3
            className={`${compact ? 'text-base' : 'text-lg sm:text-xl'} font-semibold leading-snug tracking-tight text-foreground [text-wrap:balance]`}
          >
            {title}
          </h3>
          {observation && observation !== title && (
            <p className='mt-2 max-w-3xl text-sm leading-relaxed text-muted-foreground'>
              {observation}
            </p>
          )}
          <div className='mt-5 border-t border-border/60 pt-4'>
            <div className='mb-1.5 flex items-center gap-2 text-xs font-medium text-muted-foreground'>
              <FileSearch aria-hidden='true' className='h-3.5 w-3.5' />
              {t('evidence.observed')}
            </div>
            <p className='whitespace-pre-wrap break-words font-mono text-[13px] leading-relaxed text-foreground'>
              {evidence || t('evidence.missingObservation')}
            </p>
          </div>
          {hasDetails && (
            <div className='mt-5 border-t border-border/60 pt-3'>
              <button
                type='button'
                aria-expanded={detailsOpen}
                aria-controls={detailsId}
                onClick={() => setDetailsOpen(open => !open)}
                className='w-fit text-sm font-medium text-muted-foreground transition-colors duration-150 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary motion-reduce:transition-none'
              >
                <span className='inline-flex items-center gap-2'>
                  <Search aria-hidden='true' className='h-3.5 w-3.5' />
                  {t('evidence.details')}
                  <span
                    aria-hidden='true'
                    className={cn(
                      'inline-block transition-transform duration-150 motion-reduce:transition-none',
                      detailsOpen && 'rotate-90',
                    )}
                  >
                    →
                  </span>
                </span>
              </button>
              {/* 折叠内容常驻 DOM 才能做高度过渡；inert 在收起时把它移出无障碍树与焦点序列。
                  Content stays mounted so its height can transition; inert removes it from the a11y tree and tab order while collapsed. */}
              <div
                id={detailsId}
                inert={!detailsOpen}
                className={cn(
                  'grid transition-[grid-template-rows] motion-reduce:transition-none',
                  detailsOpen
                    ? 'grid-rows-[1fr] duration-200 ease-[cubic-bezier(0.16,1,0.3,1)]'
                    : 'grid-rows-[0fr] duration-150 ease-[cubic-bezier(0.55,0,1,0.45)]',
                )}
              >
                <div className='min-h-0 overflow-hidden'>
                  <div className='space-y-4 pt-4 text-sm'>
                    {(finding.category || finding.check_code) && (
                      <p className='break-words text-xs text-muted-foreground'>
                        {[finding.category, finding.check_code]
                          .filter(Boolean)
                          .join(' · ')}
                      </p>
                    )}
                    {recommendation && (
                      <div className='border-l-2 border-border pl-3'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <span className='text-xs font-medium text-foreground'>
                            {t('evidence.investigate')}
                          </span>
                          <Button
                            type='button'
                            variant='ghost'
                            size='sm'
                            className='h-7 gap-1 px-2 text-xs'
                            onClick={() => void copyRecommendation()}
                          >
                            <Copy aria-hidden='true' className='h-3.5 w-3.5' />
                            {t('evidence.copy')}
                          </Button>
                        </div>
                        <p className='mt-1 max-w-3xl whitespace-pre-wrap leading-relaxed text-muted-foreground'>
                          {recommendation}
                        </p>
                      </div>
                    )}
                    {finding.related_error_group_id > 0 && (
                      <Link
                        href={`/diagnostics?tab=errors&cluster_id=${finding.cluster_id}&group_id=${finding.related_error_group_id}&source=inspection-finding`}
                        className='inline-flex items-center gap-1.5 text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary'
                      >
                        {t('actions.viewErrorGroup')}
                        <ArrowUpRight
                          aria-hidden='true'
                          className='h-3.5 w-3.5'
                        />
                      </Link>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </article>
  );
}

// 零发现只描述本次检查结果，不宣称整个集群健康。
// An empty inspection describes this run only, not the health of the entire cluster.
export function InspectionZeroState({compact = false}: {compact?: boolean}) {
  const t = useTranslations('diagnosticsCenter.inspections');
  return (
    <div
      className={`border border-border/70 bg-card ${compact ? 'px-6 py-9' : 'px-8 py-14 sm:px-12 sm:py-20'}`}
    >
      <div className='flex flex-col gap-5 sm:flex-row sm:items-center sm:gap-10'>
        <div
          aria-hidden='true'
          className='font-serif text-7xl leading-none text-foreground sm:text-8xl'
        >
          0
        </div>
        <div className='max-w-md'>
          <h2 className='text-lg font-semibold tracking-tight text-foreground sm:text-xl'>
            {t('evidence.zeroTitle')}
          </h2>
          <p className='mt-2 text-sm leading-relaxed text-muted-foreground'>
            {t('evidence.zeroScope')}
          </p>
        </div>
      </div>
    </div>
  );
}
