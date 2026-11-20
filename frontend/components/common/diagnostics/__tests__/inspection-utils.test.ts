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

import {describe, expect, it} from 'vitest';
import type {DiagnosticsInspectionFinding} from '@/lib/services/diagnostics';
import {
  inspectionStatusTone,
  inspectionTaskStatusKey,
  isLiveInspectionStatus,
  sortFindingsBySeverity,
  taskStatusTone,
} from '../inspections/inspection-utils';

function finding(
  id: number,
  severity: string,
): DiagnosticsInspectionFinding {
  return {
    id,
    report_id: 1,
    cluster_id: 1,
    severity: severity as DiagnosticsInspectionFinding['severity'],
    category: 'errors',
    check_code: 'CHECK',
    check_name: `check-${id}`,
    summary: `summary-${id}`,
    recommendation: '',
    evidence_summary: '',
    related_node_id: 0,
    related_host_id: 0,
    related_error_group_id: 0,
    related_alert_id: '',
    created_at: '2026-11-20T10:00:00Z',
    updated_at: '2026-11-20T10:00:00Z',
  };
}

describe('inspectionStatusTone', () => {
  it('maps report statuses to soft chip tones', () => {
    expect(inspectionStatusTone('failed')).toBe('rose');
    expect(inspectionStatusTone('running')).toBe('blue');
    expect(inspectionStatusTone('completed')).toBe('emerald');
    expect(inspectionStatusTone('pending')).toBe('muted');
    expect(inspectionStatusTone('unknown')).toBe('muted');
  });
});

describe('taskStatusTone', () => {
  it('maps bundle task statuses to soft chip tones', () => {
    expect(taskStatusTone('succeeded')).toBe('emerald');
    expect(taskStatusTone('failed')).toBe('rose');
    expect(taskStatusTone('running')).toBe('blue');
    expect(taskStatusTone('pending')).toBe('muted');
    expect(taskStatusTone('cancelled')).toBe('muted');
  });
});

describe('inspectionTaskStatusKey', () => {
  it('keeps the old succeeded → completed wording', () => {
    expect(inspectionTaskStatusKey('succeeded')).toBe(
      'inspections.status.completed',
    );
  });

  it('maps other statuses to tasks.status.*', () => {
    expect(inspectionTaskStatusKey('running')).toBe('tasks.status.running');
    expect(inspectionTaskStatusKey('cancelled')).toBe(
      'tasks.status.cancelled',
    );
    expect(inspectionTaskStatusKey('weird')).toBe('');
  });
});

describe('sortFindingsBySeverity', () => {
  it('orders critical > warning > info without mutating input', () => {
    const input = [
      finding(1, 'info'),
      finding(2, 'critical'),
      finding(3, 'warning'),
      finding(4, 'critical'),
    ];
    const sorted = sortFindingsBySeverity(input);
    expect(sorted.map((f) => f.id)).toEqual([2, 4, 3, 1]);
    expect(input.map((f) => f.id)).toEqual([1, 2, 3, 4]);
  });
});

describe('isLiveInspectionStatus', () => {
  it('drives auto-refresh only for pending / running', () => {
    expect(isLiveInspectionStatus('pending')).toBe(true);
    expect(isLiveInspectionStatus('running')).toBe(true);
    expect(isLiveInspectionStatus('completed')).toBe(false);
    expect(isLiveInspectionStatus('failed')).toBe(false);
  });
});
