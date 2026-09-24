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

import {fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it} from 'vitest';
import type {DiagnosticsInspectionFinding} from '@/lib/services/diagnostics';
import zh from '@/lib/i18n/locales/zh.json';
import en from '@/lib/i18n/locales/en.json';
import {
  InspectionFindingCard,
  InspectionZeroState,
} from '../InspectionEvidenceView';

function withLocale(element: React.ReactNode, locale = 'zh') {
  return render(
    <NextIntlClientProvider
      locale={locale}
      messages={locale === 'zh' ? zh : en}
    >
      {element}
    </NextIntlClientProvider>,
  );
}

const finding: DiagnosticsInspectionFinding = {
  id: 1,
  report_id: 4,
  cluster_id: 2,
  severity: 'warning',
  category: 'worker',
  check_code: 'HEAP_PRESSURE',
  check_name: 'Worker 内存持续偏高',
  summary: '最近 15 分钟内使用率超过 85%',
  evidence_summary: 'heap.use=87% · worker-03',
  recommendation: '查看线程与 GC 指标',
  related_node_id: 3,
  related_host_id: 8,
  related_error_group_id: 0,
  related_alert_id: '',
  created_at: '',
  updated_at: '',
};

describe('inspection evidence view', () => {
  it('describes a zero-finding run without claiming overall cluster health', () => {
    withLocale(<InspectionZeroState />);
    expect(screen.getByText('本次未发现需关注项')).toBeInTheDocument();
    expect(screen.getByText(/仅表示本次时间窗/)).toBeInTheDocument();
    expect(screen.queryByText(/健康正常|健康无异常/)).not.toBeInTheDocument();
  });

  it('puts observed evidence before optional investigation details', () => {
    withLocale(
      <InspectionFindingCard finding={finding} index={0} origin='worker-03' />,
    );
    expect(screen.getByText('Worker 内存持续偏高')).toBeInTheDocument();
    expect(screen.getByText('heap.use=87% · worker-03')).toBeVisible();
    const details = screen.getByText('查看检查信息').closest('details');
    expect(details).not.toHaveAttribute('open');
    fireEvent.click(screen.getByText('查看检查信息'));
    expect(details).toHaveAttribute('open');
    expect(screen.getByText('查看线程与 GC 指标')).toBeVisible();
  });

  it('keeps the empty state readable in English', () => {
    withLocale(<InspectionZeroState compact />, 'en');
    expect(screen.getByText('No findings to review in this run')).toBeVisible();
  });
});
