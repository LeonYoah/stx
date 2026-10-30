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

import type {ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {Loader2} from 'lucide-react';

import {cn} from '@/lib/utils';

/** Per-node JDK runtime fetch state for AC7 / 节点 JDK 运行时拉取状态（AC7） */
export type NodeJavaRuntimeView =
  | {status: 'loading'}
  | {
      status: 'ok';
      version?: number;
      version_string?: string;
      java_home?: string;
    }
  | {status: 'error'};

interface NodeJavaHomeCellProps {
  /** Configured path; empty means auto detection / 配置路径，空表示自动探测 */
  configuredJavaHome?: string;
  runtime?: NodeJavaRuntimeView;
  className?: string;
  'data-testid'?: string;
}

/**
 * Show configured JDK path plus fetched version (AC7). Path always renders; version failure never blocks.
 * 显示配置的 JDK 路径与拉取到的版本（AC7）。路径始终展示；版本失败不阻塞页面。
 */
export function NodeJavaHomeCell({
  configuredJavaHome,
  runtime,
  className,
  'data-testid': testId,
}: NodeJavaHomeCellProps) {
  const t = useTranslations();
  const pathLabel = configuredJavaHome?.trim()
    ? configuredJavaHome.trim()
    : t('javaHome.badgeAuto');

  let versionLine: ReactNode = null;
  if (!runtime || runtime.status === 'loading') {
    versionLine = (
      <span className='inline-flex items-center gap-1 text-muted-foreground'>
        <Loader2 className='h-3 w-3 animate-spin' />
        {t('javaHome.versionLoading')}
      </span>
    );
  } else if (runtime.status === 'ok') {
    const ver =
      (runtime.version_string && runtime.version_string.trim()) ||
      (runtime.version && runtime.version > 0 ? String(runtime.version) : '');
    if (ver) {
      versionLine = (
        <span className='text-muted-foreground'>
          {t('javaHome.effectiveVersionValue', {version: ver})}
        </span>
      );
    } else {
      versionLine = (
        <span className='text-muted-foreground'>
          {t('javaHome.versionUnavailable')}
        </span>
      );
    }
  } else {
    versionLine = (
      <span className='text-muted-foreground'>
        {t('javaHome.versionUnavailable')}
      </span>
    );
  }

  return (
    <div
      className={cn('space-y-0.5 font-mono text-xs max-w-[180px]', className)}
      title={`${pathLabel}${runtime?.status === 'ok' && runtime.version ? ` · Java ${runtime.version_string || runtime.version}` : ''}`}
      data-testid={testId}
    >
      <div className='truncate'>{pathLabel}</div>
      <div className='truncate text-[10px] leading-tight'>{versionLine}</div>
    </div>
  );
}

export default NodeJavaHomeCell;
