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
 * Audit Log Main Component
 * 审计日志主组件
 *
 * This component provides the main interface for audit log management,
 * including listing, searching, and filtering operations.
 * 本组件提供审计日志管理的主界面，包括列表、搜索和过滤操作。
 */

import {useState, useEffect, useCallback, type KeyboardEvent} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Separator} from '@/components/ui/separator';
import {toast} from 'sonner';
import {Search, ScrollText, RefreshCw} from 'lucide-react';
import {motion} from 'motion/react';
import {easeOut} from 'motion';
import {WorkspaceHeader} from '@/components/common/layout';
import services from '@/lib/services';
import {AuditLogInfo, ListAuditLogsRequest} from '@/lib/services/audit/types';
import {AuditLogTable} from './AuditLogTable';
import {AuditTraceSheet} from './AuditTraceSheet';

/** 审计筛选只暴露用户能理解的操作类别，不把每一种内部 action 铺进下拉框。
 * Audit filters expose user-facing action categories, not every internal action.
 */
const AUDIT_ACTION_GROUPS = ['create', 'update', 'delete', 'lifecycle', 'install', 'diagnose', 'task'] as const;

/** 对象只保留控制层资源。命令、任务模块不做成筛选项。
 * Object filters keep control-plane resources. Commands and task modules are not filters.
 */
const AUDIT_RESOURCE_TYPES = ['host', 'cluster', 'cluster_node', 'user', 'plugin'] as const;

const AUDIT_SOURCES = ['web', 'cli', 'system'] as const;

const DEFAULT_PAGE_SIZE = 20;

/**
 * Audit Log Main Component
 * 审计日志主组件
 */
export function AuditLogMain() {
  const t = useTranslations();

  // Data state / 数据状态
  const [logs, setLogs] = useState<AuditLogInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);

  // Filter state / 过滤状态
  const [searchUsername, setSearchUsername] = useState('');
  const [filterSource, setFilterSource] = useState<string>('all');
  const [filterActionGroup, setFilterActionGroup] = useState<string>('all');
  const [filterResourceType, setFilterResourceType] = useState<string>('all');
  const [filterStartDate, setFilterStartDate] = useState('');
  const [filterEndDate, setFilterEndDate] = useState('');
  const [traceLog, setTraceLog] = useState<AuditLogInfo | null>(null);


  /**
   * Load audit logs list
   * 加载审计日志列表
   */
  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      // 日期转 RFC3339：开始日 00:00:00，结束日 23:59:59.999（含当天整日）
      let startTime: string | undefined;
      let endTime: string | undefined;
      if (filterStartDate) {
        startTime = new Date(filterStartDate + 'T00:00:00').toISOString();
      }
      if (filterEndDate) {
        endTime = new Date(filterEndDate + 'T23:59:59.999').toISOString();
      }
      const params: ListAuditLogsRequest = {
        current: currentPage,
        size: pageSize,
        username: searchUsername || undefined,
        client_type: filterSource !== 'all' ? filterSource : undefined,
        action_group: filterActionGroup !== 'all' ? filterActionGroup : undefined,
        resource_type:
          filterResourceType !== 'all' ? filterResourceType : undefined,
        start_time: startTime,
        end_time: endTime,
      };

      const result = await services.audit.getAuditLogsSafe(params);

      if (result.success && result.data) {
        setLogs(result.data.logs || []);
        setTotal(result.data.total || 0);
      } else {
        toast.error(result.error || t('audit.loadError'));
        setLogs([]);
        setTotal(0);
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('audit.loadError'),
      );
      setLogs([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [
    currentPage,
    pageSize,
    searchUsername,
    filterSource,
    filterActionGroup,
    filterResourceType,
    filterStartDate,
    filterEndDate,
    t,
  ]);

  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  /**
   * Handle search
   * 处理搜索
   */
  const handleSearch = () => {
    setCurrentPage(1);
    loadLogs();
  };

  /**
   * Handle refresh
   * 处理刷新
   */
  const handleRefresh = () => {
    loadLogs();
  };

  /**
   * Submit filter form via Enter key
   * 通过 Enter 键提交过滤条件
   */
  const handleFilterInputKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') {
      return;
    }
    e.preventDefault();
    handleSearch();
  };

  /**
   * Handle page change
   * 处理页面变化
   */
  const handlePageChange = (page: number) => {
    setCurrentPage(page);
  };

  /**
   * Clear all filters
   * 清除所有过滤条件
   */
  const handleClearFilters = () => {
    setSearchUsername('');
    setFilterSource('all');
    setFilterActionGroup('all');
    setFilterResourceType('all');
    setFilterStartDate('');
    setFilterEndDate('');
    setCurrentPage(1);
  };

  const totalPages = Math.ceil(total / pageSize);

  const containerVariants = {
    hidden: {opacity: 0},
    visible: {
      opacity: 1,
      transition: {
        duration: 0.5,
        staggerChildren: 0.1,
        ease: easeOut,
      },
    },
  };

  const itemVariants = {
    hidden: {opacity: 0, y: 20},
    visible: {
      opacity: 1,
      y: 0,
      transition: {duration: 0.6, ease: easeOut},
    },
  };

  return (
    <motion.div
      className='space-y-6 flex-1 flex flex-col'
      initial='hidden'
      animate='visible'
      variants={containerVariants}
    >
      {/* Header / 标题 */}
      <motion.div variants={itemVariants}>
        <WorkspaceHeader
          icon={<ScrollText />}
          title={t('audit.auditLogsTitle')}
          subtitle={t('audit.auditLogsDescription')}
          actions={
            <Button variant='outline' onClick={handleRefresh}>
              <RefreshCw className='h-4 w-4 mr-2' />
              {t('common.refresh')}
            </Button>
          }
        />
      </motion.div>

      <Separator />

      {/* Filters / 筛选条件 */}
      <motion.div
        className='flex flex-wrap gap-2.5 items-center'
        variants={itemVariants}
      >
        <div className='min-w-[150px] max-w-[200px]'>
          <Input
            placeholder={t('audit.searchUsernamePlaceholder')}
            value={searchUsername}
            onChange={(e) => setSearchUsername(e.target.value)}
            onKeyDown={handleFilterInputKeyDown}
            className='h-9 text-xs'
          />
        </div>

        <Select value={filterSource} onValueChange={setFilterSource}>
          <SelectTrigger className='w-[120px] h-9 text-xs'>
            <SelectValue placeholder={t('audit.source')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('audit.allSources')}</SelectItem>
            {AUDIT_SOURCES.map((source) => (
              <SelectItem key={source} value={source}>
                {source === 'system' ? t('audit.actorSystem') : t(`audit.clientTypes.${source}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={filterActionGroup} onValueChange={setFilterActionGroup}>
          <SelectTrigger className='w-[120px] h-9 text-xs'>
            <SelectValue placeholder={t('audit.action')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('audit.allActions')}</SelectItem>
            {AUDIT_ACTION_GROUPS.map((group) => (
              <SelectItem key={group} value={group}>
                {t(`audit.actionGroups.${group}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={filterResourceType} onValueChange={setFilterResourceType}>
          <SelectTrigger className='w-[140px] h-9 text-xs'>
            <SelectValue placeholder={t('audit.resourceType')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('audit.allResourceTypes')}</SelectItem>
            {AUDIT_RESOURCE_TYPES.map((resourceType) => (
              <SelectItem key={resourceType} value={resourceType}>
                {t(`audit.resourceTypes.${resourceType}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
          <Input
            type='date'
            value={filterStartDate}
            onChange={(e) => setFilterStartDate(e.target.value)}
            onKeyDown={handleFilterInputKeyDown}
            className='w-[135px] h-9 text-xs'
          />
          <span>-</span>
          <Input
            type='date'
            value={filterEndDate}
            onChange={(e) => setFilterEndDate(e.target.value)}
            onKeyDown={handleFilterInputKeyDown}
            className='w-[135px] h-9 text-xs'
          />
        </div>

        <Button onClick={handleSearch} size='sm' className='h-9 active:scale-[0.98]'>
          <Search className='h-3.5 w-3.5 mr-1.5' />
          {t('common.search')}
        </Button>

        {(searchUsername || filterSource !== 'all' || filterActionGroup !== 'all' || filterResourceType !== 'all' || filterStartDate || filterEndDate) && (
          <Button variant='ghost' size='sm' onClick={handleClearFilters} className='h-9 text-muted-foreground hover:text-foreground'>
            {t('common.clearFilters')}
          </Button>
        )}
      </motion.div>

      {/* Audit Log Table / 审计日志表格 */}
      <motion.div variants={itemVariants} className='flex-1 flex flex-col'>
        <AuditLogTable
          logs={logs}
          loading={loading}
          currentPage={currentPage}
          totalPages={totalPages}
          total={total}
          pageSize={pageSize}
          onPageChange={handlePageChange}
          onPageSizeChange={(size) => {
            setPageSize(size);
            setCurrentPage(1);
          }}
          onOpenTrace={setTraceLog}
        />
      </motion.div>
      <AuditTraceSheet log={traceLog} onOpenChange={(open) => {
        if (!open) {
          setTraceLog(null);
        }
      }} />
    </motion.div>
  );
}
