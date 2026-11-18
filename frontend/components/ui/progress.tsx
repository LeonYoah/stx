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

import * as React from 'react';
import * as ProgressPrimitive from '@radix-ui/react-progress';

import {cn} from '@/lib/utils';

function Progress({
  className,
  indicatorClassName,
  value,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root> & {
  /** 指示条的可选样式覆盖（如按使用率换 warning/danger 色）。/ Optional indicator override (e.g. warning/danger by usage). */
  indicatorClassName?: string;
}) {
  return (
    <ProgressPrimitive.Root
      data-slot='progress'
      className={cn(
        'bg-primary/20 relative h-2 w-full overflow-hidden rounded-full',
        className,
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot='progress-indicator'
        className={cn('bg-primary h-full w-full flex-1 transition-all', indicatorClassName)}
        style={{transform: `translateX(-${100 - (value || 0)}%)`}}
      />
    </ProgressPrimitive.Root>
  );
}

export {Progress};
