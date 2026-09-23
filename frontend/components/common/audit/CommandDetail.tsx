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
 * Command Detail Component
 * 命令详情组件
 *
 * Displays detailed information about a command execution,
 * including output logs and error information.
 * 显示命令执行的详细信息，包括输出日志和错误信息。
 */

import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Progress} from '@/components/ui/progress';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Separator} from '@/components/ui/separator';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {
  Terminal,
  Clock,
  Server,
  AlertCircle,
  CheckCircle,
  XCircle,
  Loader2,
} from 'lucide-react';
import {CommandLogInfo, CommandStatus} from '@/lib/services/audit/types';

interface CommandDetailProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  command: CommandLogInfo;
}

/**
 * Get status icon
 * 获取状态图标
 */
function getStatusIcon(status: CommandStatus) {
  switch (status) {
    case CommandStatus.SUCCESS:
      return <CheckCircle className='h-5 w-5 text-green-500' />;
    case CommandStatus.RUNNING:
      return <Loader2 className='h-5 w-5 text-blue-500 animate-spin' />;
    case CommandStatus.PENDING:
      return <Clock className='h-5 w-5 text-yellow-500' />;
    case CommandStatus.FAILED:
      return <XCircle className='h-5 w-5 text-red-500' />;
    case CommandStatus.CANCELLED:
      return <AlertCircle className='h-5 w-5 text-gray-500' />;
    default:
      return <Clock className='h-5 w-5 text-gray-500' />;
  }
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
 * Format date time
 * 格式化日期时间
 */
function formatDateTime(dateStr: string | null): string {
  if (!dateStr) {return '-';}
  return new Date(dateStr).toLocaleString();
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
  if (seconds < 60) {return `${seconds} 秒`;}
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) {return `${minutes} 分 ${remainingSeconds} 秒`;}
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `${hours} 小时 ${remainingMinutes} 分`;
}

/**
 * Command Detail Component
 * 命令详情组件
 */
export function CommandDetail({
  open,
  onOpenChange,
  command,
}: CommandDetailProps) {
  const t = useTranslations();

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-[600px] sm:max-w-[600px]'>
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'>
            <Terminal className='h-5 w-5' />
            {t('audit.commandDetail')}
          </SheetTitle>
          <SheetDescription>
            {t('audit.commandDetailDescription')}
          </SheetDescription>
        </SheetHeader>

        <div className='mt-6 space-y-6'>
          {/* Status Overview / 状态概览 */}
          <div className='flex items-center justify-between p-4 bg-muted rounded-lg'>
            <div className='flex items-center gap-3'>
              {getStatusIcon(command.status)}
              <div>
                <Badge variant={getStatusBadgeVariant(command.status)}>
                  {t(`audit.statuses.${command.status}`)}
                </Badge>
                <p className='text-sm text-muted-foreground mt-1'>
                  {command.command_type}
                </p>
              </div>
            </div>
            <div className='text-right'>
              <div className='flex items-center gap-2'>
                <Progress value={command.progress} className='w-24 h-2' />
                <span className='text-sm font-medium'>{command.progress}%</span>
              </div>
              <p className='text-xs text-muted-foreground mt-1'>
                {formatDuration(command.started_at, command.finished_at)}
              </p>
            </div>
          </div>

          <Separator />

          {/* Basic Info / 基本信息 */}
          <div className='grid grid-cols-2 gap-4'>
            <div>
              <p className='text-sm text-muted-foreground'>
                {t('audit.commandId')}
              </p>
              <p className='font-mono text-sm break-all'>{command.command_id}</p>
            </div>
            <div>
              <p className='text-sm text-muted-foreground'>
                {t('audit.agentId')}
              </p>
              <p className='font-mono text-sm break-all'>
                {command.agent_id || '-'}
              </p>
            </div>
            <div>
              <p className='text-sm text-muted-foreground'>
                {t('audit.hostId')}
              </p>
              <p className='text-sm'>{command.host_id || '-'}</p>
            </div>
            <div>
              <p className='text-sm text-muted-foreground'>
                {t('audit.createdBy')}
              </p>
              <p className='text-sm'>{command.created_by || '-'}</p>
            </div>
          </div>

          <Separator />

          {/* Time Info / 时间信息 */}
          <div className='grid grid-cols-3 gap-4'>
            <div>
              <p className='text-sm text-muted-foreground flex items-center gap-1'>
                <Clock className='h-3 w-3' />
                {t('audit.createdAt')}
              </p>
              <p className='text-sm'>{formatDateTime(command.created_at)}</p>
            </div>
            <div>
              <p className='text-sm text-muted-foreground flex items-center gap-1'>
                <Server className='h-3 w-3' />
                {t('audit.startedAt')}
              </p>
              <p className='text-sm'>{formatDateTime(command.started_at)}</p>
            </div>
            <div>
              <p className='text-sm text-muted-foreground flex items-center gap-1'>
                <CheckCircle className='h-3 w-3' />
                {t('audit.finishedAt')}
              </p>
              <p className='text-sm'>{formatDateTime(command.finished_at)}</p>
            </div>
          </div>

          <Separator />

          {command.display_command ? (
            <div>
              <p className='text-sm text-muted-foreground'>
                {t('audit.actualCommand')}
              </p>
              <code
                className='mt-2 block overflow-x-auto whitespace-nowrap rounded-md border bg-muted/30 p-3 font-mono text-xs'
                title={command.display_command}
              >
                {command.display_command}
              </code>
            </div>
          ) : null}

          {command.display_command ? <Separator /> : null}

          {/* Output and Error Tabs / 输出和错误标签页 */}
          <Tabs defaultValue='output' className='w-full'>
            <TabsList className='grid w-full grid-cols-3'>
              <TabsTrigger value='output'>{t('audit.output')}</TabsTrigger>
              <TabsTrigger value='error'>{t('audit.error')}</TabsTrigger>
              <TabsTrigger value='parameters'>
                {t('audit.parameters')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='output' className='mt-4'>
              <ScrollArea className='h-[200px] w-full rounded-md border p-4'>
                {command.output ? (
                  <pre className='text-xs font-mono whitespace-pre-wrap'>
                    {command.output}
                  </pre>
                ) : (
                  <p className='text-sm text-muted-foreground'>
                    {t('audit.noOutput')}
                  </p>
                )}
              </ScrollArea>
            </TabsContent>
            <TabsContent value='error' className='mt-4'>
              <ScrollArea className='h-[200px] w-full rounded-md border p-4'>
                {command.error ? (
                  <pre className='text-xs font-mono whitespace-pre-wrap text-red-500'>
                    {command.error}
                  </pre>
                ) : (
                  <p className='text-sm text-muted-foreground'>
                    {t('audit.noError')}
                  </p>
                )}
              </ScrollArea>
            </TabsContent>
            <TabsContent value='parameters' className='mt-4'>
              <ScrollArea className='h-[200px] w-full rounded-md border p-4'>
                {command.parameters ? (
                  <pre className='text-xs font-mono whitespace-pre-wrap'>
                    {JSON.stringify(command.parameters, null, 2)}
                  </pre>
                ) : (
                  <p className='text-sm text-muted-foreground'>
                    {t('audit.noParameters')}
                  </p>
                )}
              </ScrollArea>
            </TabsContent>
          </Tabs>
        </div>
      </SheetContent>
    </Sheet>
  );
}
