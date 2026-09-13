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
 * Command Log Main Component
 * 命令记录主组件
 *
 * This component provides the main interface for command log management,
 * including listing, searching, and filtering operations.
 * 本组件提供命令记录管理的主界面，包括列表、搜索和过滤操作。
 */

import {useState, useEffect, useCallback} from 'react';
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
import {Search, Terminal, RefreshCw} from 'lucide-react';
import {motion} from 'motion/react';
import {easeOut} from 'motion';
import {WorkspaceHeader} from '@/components/common/layout';
import services from '@/lib/services';
import {
  CommandLogInfo,
  CommandStatus,
  ListCommandLogsRequest,
} from '@/lib/services/audit/types';
import {CommandTable} from './CommandTable';
import {CommandDetail} from './CommandDetail';

const PAGE_SIZE = 10;

/**
 * Command Log Main Component
 * 命令记录主组件
 */
export function CommandMain() {
  const t = useTranslations();

  // Data state / 数据状态
  const [commands, setCommands] = useState<CommandLogInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [currentPage, setCurrentPage] = useState(1);

  // Filter state / 过滤状态
  const [searchCommandId, setSearchCommandId] = useState('');
  const [filterStatus, setFilterStatus] = useState<string>('all');
  const [filterCommandType, setFilterCommandType] = useState<string>('all');


  // Dialog state / 对话框状态
  const [isDetailOpen, setIsDetailOpen] = useState(false);
  const [selectedCommand, setSelectedCommand] = useState<CommandLogInfo | null>(
    null,
  );

  /**
   * Load command logs list
   * 加载命令记录列表
   */
  const loadCommands = useCallback(async () => {
    setLoading(true);
    try {
      const params: ListCommandLogsRequest = {
        current: currentPage,
        size: PAGE_SIZE,
        command_id: searchCommandId || undefined,
        status:
          filterStatus !== 'all' ? (filterStatus as CommandStatus) : undefined,
        command_type:
          filterCommandType !== 'all' ? filterCommandType : undefined,
      };

      const result = await services.audit.getCommandLogsSafe(params);

      if (result.success && result.data) {
        setCommands(result.data.commands || []);
        setTotal(result.data.total || 0);
      } else {
        toast.error(result.error || t('audit.loadCommandsError'));
        setCommands([]);
        setTotal(0);
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('audit.loadCommandsError'),
      );
      setCommands([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [currentPage, searchCommandId, filterStatus, filterCommandType, t]);

  useEffect(() => {
    loadCommands();
  }, [loadCommands]);

  /**
   * Handle search
   * 处理搜索
   */
  const handleSearch = () => {
    setCurrentPage(1);
    loadCommands();
  };

  /**
   * Handle refresh
   * 处理刷新
   */
  const handleRefresh = () => {
    loadCommands();
  };

  /**
   * Handle page change
   * 处理页面变化
   */
  const handlePageChange = (page: number) => {
    setCurrentPage(page);
  };

  /**
   * Handle view command detail
   * 处理查看命令详情
   */
  const handleViewDetail = (command: CommandLogInfo) => {
    setSelectedCommand(command);
    setIsDetailOpen(true);
  };

  /**
   * Clear all filters
   * 清除所有过滤条件
   */
  const handleClearFilters = () => {
    setSearchCommandId('');
    setFilterStatus('all');
    setFilterCommandType('all');
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
          icon={<Terminal />}
          title={t('audit.commandsTitle')}
          subtitle={t('audit.commandsDescription')}
          actions={
            <Button variant='outline' onClick={handleRefresh}>
              <RefreshCw className='h-4 w-4 mr-2' />
              {t('common.refresh')}
            </Button>
          }
        />
      </motion.div>

      <Separator />

      {/* Filters / 过滤器 */}
      <motion.div
        className='flex flex-wrap gap-4 items-end'
        variants={itemVariants}
      >
        <div className='flex-1 min-w-[200px] max-w-sm'>
          <Input
            placeholder={t('audit.searchCommandPlaceholder')}
            value={searchCommandId}
            onChange={(e) => setSearchCommandId(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
          />
        </div>

        <Select value={filterStatus} onValueChange={setFilterStatus}>
          <SelectTrigger className='w-[150px]'>
            <SelectValue placeholder={t('audit.status')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('audit.allStatuses')}</SelectItem>
            <SelectItem value={CommandStatus.PENDING}>
              {t('audit.statuses.pending')}
            </SelectItem>
            <SelectItem value={CommandStatus.RUNNING}>
              {t('audit.statuses.running')}
            </SelectItem>
            <SelectItem value={CommandStatus.SUCCESS}>
              {t('audit.statuses.success')}
            </SelectItem>
            <SelectItem value={CommandStatus.FAILED}>
              {t('audit.statuses.failed')}
            </SelectItem>
            <SelectItem value={CommandStatus.CANCELLED}>
              {t('audit.statuses.cancelled')}
            </SelectItem>
          </SelectContent>
        </Select>

        <Select value={filterCommandType} onValueChange={setFilterCommandType}>
          <SelectTrigger className='w-[150px]'>
            <SelectValue placeholder={t('audit.commandType')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('audit.allTypes')}</SelectItem>
            <SelectItem value='PRECHECK'>{t('audit.types.precheck')}</SelectItem>
            <SelectItem value='INSTALL'>{t('audit.types.install')}</SelectItem>
            <SelectItem value='UNINSTALL'>
              {t('audit.types.uninstall')}
            </SelectItem>
            <SelectItem value='START'>{t('audit.types.start')}</SelectItem>
            <SelectItem value='STOP'>{t('audit.types.stop')}</SelectItem>
            <SelectItem value='RESTART'>{t('audit.types.restart')}</SelectItem>
          </SelectContent>
        </Select>

        <Button variant='outline' onClick={handleSearch}>
          <Search className='h-4 w-4 mr-2' />
          {t('common.search')}
        </Button>

        <Button variant='ghost' onClick={handleClearFilters}>
          {t('common.clearFilters')}
        </Button>
      </motion.div>

      {/* Command Table / 命令表格 */}
      <motion.div variants={itemVariants}>
        <CommandTable
          commands={commands}
          loading={loading}
          currentPage={currentPage}
          totalPages={totalPages}
          total={total}
          onPageChange={handlePageChange}
          onViewDetail={handleViewDetail}
        />
      </motion.div>

      {/* Command Detail / 命令详情 */}
      {selectedCommand && (
        <CommandDetail
          open={isDetailOpen}
          onOpenChange={setIsDetailOpen}
          command={selectedCommand}
        />
      )}
    </motion.div>
  );
}
