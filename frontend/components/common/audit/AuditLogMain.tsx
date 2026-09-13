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
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Separator} from '@/components/ui/separator';
import {toast} from 'sonner';
import {Search, ScrollText, RefreshCw, Filter} from 'lucide-react';
import {motion} from 'motion/react';
import {easeOut} from 'motion';
import {WorkspaceHeader} from '@/components/common/layout';
import services from '@/lib/services';
import {AuditLogInfo, ListAuditLogsRequest} from '@/lib/services/audit/types';
import {AuditLogTable} from './AuditLogTable';

const PAGE_SIZE = 20;

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

  // Filter state / 过滤状态
  const [searchUsername, setSearchUsername] = useState('');
  const [filterTrigger, setFilterTrigger] = useState<string>('all');
  const [filterAction, setFilterAction] = useState<string>('all');
  const [filterResourceType, setFilterResourceType] = useState<string>('all');
  const [filterStartDate, setFilterStartDate] = useState('');
  const [filterEndDate, setFilterEndDate] = useState('');


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
        size: PAGE_SIZE,
        username: searchUsername || undefined,
        trigger: filterTrigger !== 'all' ? filterTrigger : undefined,
        action: filterAction !== 'all' ? filterAction : undefined,
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
        toast.error(result.error || t('audit.loadAuditLogsError'));
        setLogs([]);
        setTotal(0);
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('audit.loadAuditLogsError'),
      );
      setLogs([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [
    currentPage,
    searchUsername,
    filterTrigger,
    filterAction,
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
    setFilterTrigger('all');
    setFilterAction('all');
    setFilterResourceType('all');
    setFilterStartDate('');
    setFilterEndDate('');
    setCurrentPage(1);
  };

  const totalPages = Math.ceil(total / PAGE_SIZE);

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
      className='space-y-6'
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
      <motion.div variants={itemVariants}>
        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='flex items-center gap-2 text-base'>
              <Filter className='h-4 w-4' />
              {t('audit.filterConditions')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className='grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-8 gap-4 items-end'>
              <div className='min-w-0'>
                <label className='text-xs text-muted-foreground mb-1 block'>
                  {t('audit.user')}
                </label>
                <Input
                  placeholder={t('audit.searchUsernamePlaceholder')}
                  value={searchUsername}
                  onChange={(e) => setSearchUsername(e.target.value)}
                  onKeyDown={handleFilterInputKeyDown}
                />
              </div>
              <div className='min-w-0'>
                <label className='text-xs text-muted-foreground mb-1 block'>
                  {t('audit.trigger')}
                </label>
                <Select value={filterTrigger} onValueChange={setFilterTrigger}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>{t('audit.allTriggers')}</SelectItem>
                    <SelectItem value='auto'>{t('audit.triggerAuto')}</SelectItem>
                    <SelectItem value='manual'>
                      {t('audit.triggerManual')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className='min-w-0'>
                <label className='text-xs text-muted-foreground mb-1 block'>
                  {t('audit.action')}
                </label>
                <Select value={filterAction} onValueChange={setFilterAction}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>{t('audit.allActions')}</SelectItem>
                    <SelectItem value='create'>
                      {t('audit.actions.create')}
                    </SelectItem>
                    <SelectItem value='update'>
                      {t('audit.actions.update')}
                    </SelectItem>
                    <SelectItem value='delete'>
                      {t('audit.actions.delete')}
                    </SelectItem>
                    <SelectItem value='start'>
                      {t('audit.actions.start')}
                    </SelectItem>
                    <SelectItem value='stop'>
                      {t('audit.actions.stop')}
                    </SelectItem>
                    <SelectItem value='restart'>
                      {t('audit.actions.restart')}
                    </SelectItem>
                    <SelectItem value='add_node'>
                      {t('audit.actions.add_node')}
                    </SelectItem>
                    <SelectItem value='remove_node'>
                      {t('audit.actions.remove_node')}
                    </SelectItem>
                    <SelectItem value='update_node'>
                      {t('audit.actions.update_node')}
                    </SelectItem>
                    <SelectItem value='start_node'>
                      {t('audit.actions.start_node')}
                    </SelectItem>
                    <SelectItem value='stop_node'>
                      {t('audit.actions.stop_node')}
                    </SelectItem>
                    <SelectItem value='restart_node'>
                      {t('audit.actions.restart_node')}
                    </SelectItem>
                    <SelectItem value='crashed'>
                      {t('audit.actions.crashed')}
                    </SelectItem>
                    <SelectItem value='restart_failed'>
                      {t('audit.actions.restart_failed')}
                    </SelectItem>
                    <SelectItem value='install'>
                      {t('audit.actions.install')}
                    </SelectItem>
                    <SelectItem value='uninstall'>
                      {t('audit.actions.uninstall')}
                    </SelectItem>
                    <SelectItem value='enable'>
                      {t('audit.actions.enable')}
                    </SelectItem>
                    <SelectItem value='disable'>
                      {t('audit.actions.disable')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className='min-w-0'>
                <label className='text-xs text-muted-foreground mb-1 block'>
                  {t('audit.resourceType')}
                </label>
                <Select
                  value={filterResourceType}
                  onValueChange={setFilterResourceType}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('audit.allResourceTypes')}
                    </SelectItem>
                    <SelectItem value='host'>
                      {t('audit.resourceTypes.host')}
                    </SelectItem>
                    <SelectItem value='cluster'>
                      {t('audit.resourceTypes.cluster')}
                    </SelectItem>
                    <SelectItem value='cluster_node'>
                      {t('audit.resourceTypes.cluster_node')}
                    </SelectItem>
                    <SelectItem value='user'>
                      {t('audit.resourceTypes.user')}
                    </SelectItem>
                    <SelectItem value='plugin'>
                      {t('audit.resourceTypes.plugin')}
                    </SelectItem>
                    <SelectItem value='project'>
                      {t('audit.resourceTypes.project')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className='min-w-0'>
                <label className='text-xs text-muted-foreground mb-1 block'>
                  {t('audit.startDate')}
                </label>
                <Input
                  type='date'
                  value={filterStartDate}
                  onChange={(e) => setFilterStartDate(e.target.value)}
                  onKeyDown={handleFilterInputKeyDown}
                  className='w-full'
                />
              </div>
              <div className='min-w-0'>
                <label className='text-xs text-muted-foreground mb-1 block'>
                  {t('audit.endDate')}
                </label>
                <Input
                  type='date'
                  value={filterEndDate}
                  onChange={(e) => setFilterEndDate(e.target.value)}
                  onKeyDown={handleFilterInputKeyDown}
                  className='w-full'
                />
              </div>
              <div className='min-w-0 flex gap-2 flex-shrink-0 xl:col-span-2'>
                <Button onClick={handleSearch}>
                  <Search className='h-4 w-4 mr-2' />
                  {t('common.search')}
                </Button>
                <Button variant='outline' onClick={handleClearFilters}>
                  {t('common.clearFilters')}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </motion.div>

      {/* Audit Log Table / 审计日志表格 */}
      <motion.div variants={itemVariants}>
        <AuditLogTable
          logs={logs}
          loading={loading}
          currentPage={currentPage}
          totalPages={totalPages}
          total={total}
          onPageChange={handlePageChange}
        />
      </motion.div>
    </motion.div>
  );
}
