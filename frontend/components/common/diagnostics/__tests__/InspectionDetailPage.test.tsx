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
import {beforeEach, describe, expect, it, vi} from 'vitest';
import type {DiagnosticsInspectionReport} from '@/lib/services/diagnostics';
import zh from '@/lib/i18n/locales/zh.json';
import InspectionDetailPage from '../InspectionDetailPage';

const getDetail = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({
  default: {diagnostics: {getInspectionReportDetailSafe: getDetail}},
}));

const baseReport: DiagnosticsInspectionReport = {
  id: 14,
  cluster_id: 1,
  cluster_name: 'stx-prod',
  status: 'completed',
  trigger_source: 'manual',
  lookback_minutes: 30,
  error_threshold: 1,
  requested_by_user_id: 1,
  requested_by: 'operator',
  summary: '集群健康正常',
  error_message: '',
  finding_total: 0,
  critical_count: 0,
  warning_count: 0,
  info_count: 0,
  finished_at: '2026-09-24T09:00:00Z',
  created_at: '2026-09-24T08:30:00Z',
  updated_at: '2026-09-24T09:00:00Z',
};

function renderDetail() {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <InspectionDetailPage inspectionId={14} />
    </NextIntlClientProvider>,
  );
}

describe('inspection detail states', () => {
  beforeEach(() => {
    getDetail.mockReset();
    // 测试环境没有 matchMedia，模拟普通动画偏好以渲染真实页头。
    // JSDOM lacks matchMedia; provide the standard preference for the real header.
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi
        .fn()
        .mockImplementation(() => ({
          matches: false,
          addEventListener: vi.fn(),
          removeEventListener: vi.fn(),
        })),
    });
  });

  it('shows a scoped zero state without promoting an old health summary', async () => {
    getDetail.mockResolvedValue({
      success: true,
      data: {report: baseReport, findings: [], related_diagnostic_task: null},
    });
    renderDetail();
    expect(await screen.findByText('本次未发现需关注项')).toBeVisible();
    expect(screen.queryByText('集群健康正常')).not.toBeInTheDocument();
    expect(screen.queryByText('诊断包')).not.toBeInTheDocument();
  });

  it('does not show a zero result for a failed run', async () => {
    getDetail.mockResolvedValue({
      success: true,
      data: {report: {...baseReport, status: 'failed'}, findings: []},
    });
    renderDetail();
    expect(await screen.findByText('检查尚未完成')).toBeVisible();
    expect(screen.queryByText('本次未发现需关注项')).not.toBeInTheDocument();
  });

  it('does not call missing finding details a zero result', async () => {
    getDetail.mockResolvedValue({
      success: true,
      data: {report: {...baseReport, finding_total: 2}, findings: []},
    });
    renderDetail();
    expect(
      await screen.findByText('发现项暂未载入，请稍后刷新。'),
    ).toBeVisible();
    expect(screen.queryByText('本次未发现需关注项')).not.toBeInTheDocument();
  });
});
