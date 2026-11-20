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
 * 审计日志主页：TableCard + useServerListQuery；追踪仍走 AuditTraceSheet。
 * Audit log page: TableCard + useServerListQuery; traces stay on AuditTraceSheet.
 */

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {RefreshCw, ScrollText} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import {
  ArcDateRangePicker,
  TableCard,
  WorkspaceHeader,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {useServerListQuery} from '@/lib/hooks/use-server-list-query';
import {dateRangeToYmdPair, ymdPairToDateRange} from '@/lib/date-range';
import {wallTimeToRFC3339} from '@/lib/timezone';
import services from '@/lib/services';
import {AuditLogInfo, ListAuditLogsRequest} from '@/lib/services/audit/types';
import type {DateRange} from '@/components/arc/date-range-picker/date-range-picker';
import {cn} from '@/lib/utils';
import {AuditLogTable} from './AuditLogTable';
import {AuditTraceSheet} from './AuditTraceSheet';

/** 审计筛选只暴露用户能理解的操作类别，不把每一种内部 action 铺进下拉框。
 * Audit filters expose user-facing action categories, not every internal action.
 */
const AUDIT_ACTION_GROUPS = [
  'create',
  'update',
  'delete',
  'lifecycle',
  'install',
  'diagnose',
  'task',
] as const;

/** 对象只保留控制层资源。命令、任务模块不做成筛选项。
 * Object filters keep control-plane resources. Commands and task modules are not filters.
 */
const AUDIT_RESOURCE_TYPES = [
  'host',
  'cluster',
  'cluster_node',
  'user',
  'plugin',
  'cli_token',
] as const;

const AUDIT_SOURCES = ['web', 'cli', 'system'] as const;

const DEFAULT_PAGE_SIZE = 20;
const ALL_VALUE = 'all';

type AuditFilters = {
  username: string;
  source: string;
  actionGroup: string;
  resourceType: string;
  startDate: string;
  endDate: string;
};

const INITIAL_FILTERS: AuditFilters = {
  username: '',
  source: ALL_VALUE,
  actionGroup: ALL_VALUE,
  resourceType: ALL_VALUE,
  startDate: '',
  endDate: '',
};

export function AuditLogMain() {
  const t = useTranslations();
  const {
    page,
    pageSize,
    filters,
    setPage,
    setPageSize,
    patchFilters,
    resetFilters,
  } = useServerListQuery({
    initialFilters: INITIAL_FILTERS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const [usernameDraft, setUsernameDraft] = useState(filters.username);
  const [logs, setLogs] = useState<AuditLogInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [traceLog, setTraceLog] = useState<AuditLogInfo | null>(null);

  const dateRange = useMemo(
    () => ymdPairToDateRange(filters.startDate, filters.endDate),
    [filters.startDate, filters.endDate],
  );

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      let startTime: string | undefined;
      let endTime: string | undefined;
      if (filters.startDate) {
        startTime = wallTimeToRFC3339(filters.startDate + 'T00:00:00');
      }
      if (filters.endDate) {
        endTime = wallTimeToRFC3339(filters.endDate + 'T23:59:59.999');
      }
      const params: ListAuditLogsRequest = {
        current: page,
        size: pageSize,
        username: filters.username || undefined,
        client_type: filters.source !== ALL_VALUE ? filters.source : undefined,
        action_group:
          filters.actionGroup !== ALL_VALUE ? filters.actionGroup : undefined,
        resource_type:
          filters.resourceType !== ALL_VALUE ? filters.resourceType : undefined,
        start_time: startTime,
        end_time: endTime,
      };

      const result = await services.audit.getAuditLogsSafe(params);
      if (result.success && result.data) {
        setLogs(result.data.logs || []);
        setTotal(result.data.total || 0);
        setError(null);
      } else {
        setError(result.error || t('audit.loadError'));
        setLogs([]);
        setTotal(0);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('audit.loadError'));
      setLogs([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, filters, t]);

  useEffect(() => {
    void loadLogs();
  }, [loadLogs]);

  const commitUsername = useCallback(() => {
    const next = usernameDraft.trim();
    if (next !== filters.username) {
      patchFilters({username: next});
    }
  }, [usernameDraft, filters.username, patchFilters]);

  const handleDateRangeChange = (range: DateRange) => {
    const next = dateRangeToYmdPair(range);
    patchFilters({startDate: next.startDate, endDate: next.endDate});
  };

  const handleClearFilters = () => {
    setUsernameDraft('');
    resetFilters();
  };

  const filtersActive =
    Boolean(filters.username) ||
    filters.source !== ALL_VALUE ||
    filters.actionGroup !== ALL_VALUE ||
    filters.resourceType !== ALL_VALUE ||
    Boolean(filters.startDate) ||
    Boolean(filters.endDate);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className={cn(layout.page, layout.pageViewport)}>
      <WorkspaceHeader
        icon={<ScrollText />}
        title={t('audit.auditLogsTitle')}
        subtitle={t('audit.auditLogsDescription')}
        actions={
          <Button
            type='button'
            variant='secondary'
            size='sm'
            onClick={() => void loadLogs()}
            disabled={loading}
          >
            <RefreshCw
              className={cn('size-3.5', loading && 'animate-spin')}
              aria-hidden
            />
            {t('common.refresh')}
          </Button>
        }
      />

      <div className={layout.toolbar}>
        <div className={layout.toolbarSearch}>
          <SearchField
            label={t('audit.searchUsernamePlaceholder')}
            value={usernameDraft}
            onValueChange={(v) => {
              setUsernameDraft(v);
              if (v === '' && filters.username !== '') {
                patchFilters({username: ''});
              }
            }}
            placeholder={t('audit.searchUsernamePlaceholder')}
            clearLabel={t('common.searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commitUsername();
              }
            }}
          />
        </div>
        <div className={cn(layout.filterCtrl, layout.filterWsm)}>
          <Select
            label={t('audit.source')}
            value={filters.source}
            onValueChange={(v) => patchFilters({source: v})}
            displayValue={
              filters.source === ALL_VALUE ? t('audit.source') : undefined
            }
            options={[
              {value: ALL_VALUE, label: t('audit.allSources')},
              ...AUDIT_SOURCES.map((source) => ({
                value: source,
                label:
                  source === 'system'
                    ? t('audit.actorSystem')
                    : t(`audit.clientTypes.${source}`),
              })),
            ]}
          />
        </div>
        <div className={cn(layout.filterCtrl, layout.filterWsm)}>
          <Select
            label={t('audit.action')}
            value={filters.actionGroup}
            onValueChange={(v) => patchFilters({actionGroup: v})}
            displayValue={
              filters.actionGroup === ALL_VALUE ? t('audit.action') : undefined
            }
            options={[
              {value: ALL_VALUE, label: t('audit.allActions')},
              ...AUDIT_ACTION_GROUPS.map((group) => ({
                value: group,
                label: t(`audit.actionGroups.${group}`),
              })),
            ]}
          />
        </div>
        <div className={cn(layout.filterCtrl, layout.filterWmd)}>
          <Select
            label={t('audit.resourceType')}
            value={filters.resourceType}
            onValueChange={(v) => patchFilters({resourceType: v})}
            displayValue={
              filters.resourceType === ALL_VALUE
                ? t('audit.resourceType')
                : undefined
            }
            options={[
              {value: ALL_VALUE, label: t('audit.allResourceTypes')},
              ...AUDIT_RESOURCE_TYPES.map((resourceType) => ({
                value: resourceType,
                label: t(`audit.resourceTypes.${resourceType}`),
              })),
            ]}
          />
        </div>
        <ArcDateRangePicker
          value={dateRange}
          onChange={handleDateRangeChange}
          label={t('common.dateRange.label')}
          placeholder={t('common.dateRange.placeholder')}
        />
        {filtersActive ? (
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={handleClearFilters}
          >
            {t('common.clearFilters')}
          </Button>
        ) : null}
      </div>

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='audit'>
          <TableCard
            loading={loading}
            hasData={logs.length > 0}
            skeletonColumns={9}
            skeletonRows={8}
            error={error}
            onRetry={() => void loadLogs()}
            pagination={{
              currentPage: page,
              totalPages,
              pageSize,
              totalItems: total,
              onPageChange: setPage,
              onPageSizeChange: setPageSize,
              pageSizeOptions: [10, 20, 50, 100],
            }}
            empty={
              <EmptyState
                title={t('audit.noAuditLogs')}
                description={
                  filtersActive
                    ? t('common.noMatchingData')
                    : t('audit.auditLogsDescription')
                }
                action={
                  filtersActive ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={handleClearFilters}
                    >
                      {t('common.clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <AuditLogTable logs={logs} onOpenTrace={setTraceLog} />
          </TableCard>
        </div>
      </div>

      <AuditTraceSheet
        log={traceLog}
        onOpenChange={(open) => {
          if (!open) {
            setTraceLog(null);
          }
        }}
      />
    </div>
  );
}
