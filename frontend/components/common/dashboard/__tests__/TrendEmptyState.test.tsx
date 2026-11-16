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

import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {TrendEmptyState} from '../TrendEmptyState';

const copy = zh.dashboard.home.clusterTab.noProm;

function renderState(reason?: string) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <TrendEmptyState reason={reason} />
    </NextIntlClientProvider>,
  );
}

describe('TrendEmptyState', () => {
  it('prompts to turn observability on when it is disabled', () => {
    renderState('observability_disabled');
    expect(screen.getByText(copy.title)).toBeTruthy();
    expect(screen.getByTestId('home-trend-empty').dataset.reason).toBe(
      'disabled',
    );
  });

  it('reports unavailable data for storage or query failures', () => {
    renderState('prometheus_unavailable');
    expect(screen.getByText(copy.unavailableTitle)).toBeTruthy();
    expect(screen.queryByText(copy.title)).toBeNull();
  });

  it('never names the underlying monitoring stack', () => {
    const {container} = renderState('observability_disabled');
    expect(container.textContent ?? '').not.toMatch(/prometheus/i);
  });
});
