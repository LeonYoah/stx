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
 * Command Table Component
 * 命令表格组件
 *
 * Displays a table of command logs with actions for viewing details.
 * 显示命令日志表格，支持查看详情操作。
 */

import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Progress} from '@/components/ui/progress';
import {Pagination} from '@/components/ui/pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {Eye} from 'lucide-react';
import {CommandLogInfo, CommandStatus} from '@/lib/services/audit/types';

interface CommandTableProps {
  commands: CommandLogInfo[];
  loading: boolean;
  currentPage: number;
  totalPages: number;
  total: number;
  pageSize?: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  onViewDetail: (command: CommandLogInfo) => void;
}

/**
 * Get status badge variant
 * 获取状态徽章变体
 */
function getStatusBadgeVariant(
  status: CommandStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case CommandStatus.SUCCESS:
      return 'default';
    case CommandStatus.RUNNING:
      return 'secondary';
    case CommandStatus.PENDING:
      return 'outline';
    case CommandStatus.FAILED:
      return 'destructive';
    case CommandStatus.CANCELLED:
      return 'outline';
    default:
      return 'secondary';
  }
}


/**
 * Format duration between two dates
 * 格式化两个日期之间的时长
 */
function formatDuration(
  startedAt: string | null,
  finishedAt: string | null,
): string {
  if (!startedAt) {return '-';}
  const start = new Date(startedAt);
  const end = finishedAt ? new Date(finishedAt) : new Date();
  const durationMs = end.getTime() - start.getTime();
  const seconds = Math.floor(durationMs / 1000);
  if (seconds < 60) {return `${seconds}s`;}
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) {return `${minutes}m ${remainingSeconds}s`;}
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `${hours}h ${remainingMinutes}m`;
}

/**
 * Command Table Component
 * 命令表格组件
 */
export function CommandTable({
  commands,
  loading,
  currentPage,
  totalPages,
  total,
  pageSize = 10,
  onPageChange,
  onPageSizeChange,
  onViewDetail,
}: CommandTableProps) {
  const t = useTranslations();

  return (
    <div className='space-y-4'>
      <div className='border rounded-lg relative overflow-hidden bg-card/40 shadow-xs flex flex-col min-h-[480px] sm:min-h-[calc(100vh-270px)]'>
        <div className='overflow-x-auto flex-1'>
          <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-[50px]'>ID</TableHead>
              <TableHead>{t('audit.commandId')}</TableHead>
              <TableHead>{t('audit.commandType')}</TableHead>
              <TableHead>{t('audit.agentId')}</TableHead>
              <TableHead>{t('audit.status')}</TableHead>
              <TableHead>{t('audit.progress')}</TableHead>
              <TableHead>{t('audit.duration')}</TableHead>
              <TableHead>{t('audit.createdAt')}</TableHead>
              <TableHead className='w-[80px]'>{t('common.view')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={9} className='text-center py-8'>
                  {t('common.loading')}
                </TableCell>
              </TableRow>
            ) : commands.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={9}
                  className='text-center py-8 text-muted-foreground'
                >
                  {t('audit.noCommands')}
                </TableCell>
              </TableRow>
            ) : (
              commands.map((command) => (
                <TableRow key={command.id}>
                  <TableCell>{command.id}</TableCell>
                  <TableCell>
                    <span className='font-mono text-sm truncate max-w-[150px] block'>
                      {command.command_id}
                    </span>
                  </TableCell>
                  <TableCell>
                    <Badge variant='outline'>{command.command_type}</Badge>
                  </TableCell>
                  <TableCell>
                    <span className='font-mono text-sm truncate max-w-[120px] block'>
                      {command.agent_id || '-'}
                    </span>
                  </TableCell>
                  <TableCell>
                    <Badge variant={getStatusBadgeVariant(command.status)}>
                      {t(`audit.statuses.${command.status}`)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2 min-w-[100px]'>
                      <Progress value={command.progress} className='h-2' />
                      <span className='text-xs text-muted-foreground'>
                        {command.progress}%
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    {formatDuration(command.started_at, command.finished_at)}
                  </TableCell>
                  <TableCell>
                    {new Date(command.created_at).toLocaleString()}
                  </TableCell>
                  <TableCell>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant='ghost'
                            size='icon'
                            onClick={() => onViewDetail(command)}
                          >
                            <Eye className='h-4 w-4' />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{t('common.view')}</TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        </div>

        {/* 底部分页栏 / Table Footer Pagination */}
        <div className='border-t bg-muted/10 px-4 py-2.5 mt-auto'>
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            pageSize={pageSize}
            totalItems={total}
            onPageChange={onPageChange}
            onPageSizeChange={onPageSizeChange}
            showPageSizeSelector={Boolean(onPageSizeChange)}
            pageSizeOptions={[10, 20, 50, 100]}
          />
        </div>
      </div>
    </div>
  );
}
