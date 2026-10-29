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

import {ArrowDownToLine, Cpu, Database} from 'lucide-react';
import {cn} from '@/lib/utils';

/**
 * 算子类型的统一配色（沿用原 DAG 预览）：Source 绿、Sink 靛、Transform 琥珀。
 * Shared operator type palette (kept from the former DAG preview): source emerald, sink indigo, transform amber.
 */
export function operatorBadgeTone(type: string): string {
  switch (type) {
    case 'source':
      return 'border-emerald-500/30 bg-emerald-500/15 text-emerald-700 dark:text-emerald-300';
    case 'sink':
      return 'border-indigo-500/30 bg-indigo-500/15 text-indigo-700 dark:text-indigo-300';
    default:
      return 'border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-300';
  }
}

/** 小地图与左侧色条用的类型色。/ Type color for the minimap and the left accent bar. */
export function operatorAccent(type: string): string {
  switch (type) {
    case 'source':
      return '#10b981';
    case 'sink':
      return '#6366f1';
    default:
      return '#f59e0b';
  }
}

export function OperatorIcon({type, className}: {type: string; className?: string}) {
  switch (type) {
    case 'source':
      return <Database className={cn('size-3.5 text-emerald-600 dark:text-emerald-400', className)} />;
    case 'sink':
      return <ArrowDownToLine className={cn('size-3.5 text-indigo-600 dark:text-indigo-400', className)} />;
    default:
      return <Cpu className={cn('size-3.5 text-amber-600 dark:text-amber-400', className)} />;
  }
}
