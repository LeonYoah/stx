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

/**
 * Precheck Step Component
 * 预检查步骤组件
 *
 * Displays precheck items and their status.
 * 显示预检查项及其状态。
 */

'use client';

import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {Badge} from '@/components/ui/badge';
import {ScrollArea} from '@/components/ui/scroll-area';
import {cn} from '@/lib/utils';
import {localizeBackendText} from '@/lib/i18n/localize-text';
import {
  CheckCircle2,
  XCircle,
  AlertTriangle,
  RefreshCw,
  Cpu,
  HardDrive,
  MemoryStick,
  Network,
  Coffee,
  Loader2,
  PlayCircle,
} from 'lucide-react';
import type {
  PrecheckResult,
  PrecheckItem,
  CheckStatus,
} from '@/lib/services/installer/types';

interface PrecheckStepProps {
  /** Precheck result / 预检查结果 */
  result: PrecheckResult | null;
  /** Loading state / 加载状态 */
  loading: boolean;
  /** Error message / 错误信息 */
  error: string | null;
  /** Callback to run precheck / 运行预检查的回调 */
  onRunPrecheck: () => void;
}

// Icon mapping for precheck items / 预检查项的图标映射
const itemIcons: Record<string, React.ComponentType<{className?: string}>> = {
  memory: MemoryStick,
  cpu: Cpu,
  disk: HardDrive,
  ports: Network,
  java: Coffee,
};

// Status configuration / 状态配置
const statusConfig: Record<
  CheckStatus,
  {
    icon: React.ComponentType<{className?: string}>;
    color: string;
    bgColor: string;
    label: string;
  }
> = {
  passed: {
    icon: CheckCircle2,
    color: 'text-green-600 dark:text-green-400',
    bgColor: 'bg-green-100 dark:bg-green-900/30',
    label: 'Passed',
  },
  failed: {
    icon: XCircle,
    color: 'text-red-600 dark:text-red-400',
    bgColor: 'bg-red-100 dark:bg-red-900/30',
    label: 'Failed',
  },
  warning: {
    icon: AlertTriangle,
    color: 'text-yellow-600 dark:text-yellow-400',
    bgColor: 'bg-yellow-100 dark:bg-yellow-900/30',
    label: 'Warning',
  },
};

export function PrecheckStep({
  result,
  loading,
  error,
  onRunPrecheck,
}: PrecheckStepProps) {
  const t = useTranslations();

  // Get overall status badge / 获取整体状态徽章
  const getOverallStatusBadge = () => {
    if (!result) {
      return null;
    }
    const config = statusConfig[result.overall_status];
    const Icon = config.icon;
    return (
      <Badge
        variant='outline'
        className={cn('gap-1', config.color, config.bgColor)}
      >
        <Icon className='h-3 w-3' />
        {result.overall_status === 'passed' && t('installer.precheckPassed')}
        {result.overall_status === 'failed' && t('installer.precheckFailed')}
        {result.overall_status === 'warning' && t('installer.precheckWarning')}
      </Badge>
    );
  };

  // Render precheck item / 渲染预检查项
  const renderPrecheckItem = (item: PrecheckItem) => {
    const config = statusConfig[item.status];
    const StatusIcon = config.icon;
    const ItemIcon = itemIcons[item.name.toLowerCase()] || CheckCircle2;

    return (
      <div
        key={item.name}
        className={cn(
          'flex items-start gap-4 p-4 rounded-lg border transition-colors',
          item.status === 'failed' &&
            'border-red-200 dark:border-red-900/50 bg-red-50/50 dark:bg-red-900/10',
          item.status === 'warning' &&
            'border-yellow-200 dark:border-yellow-900/50 bg-yellow-50/50 dark:bg-yellow-900/10',
          item.status === 'passed' &&
            'border-green-200 dark:border-green-900/50 bg-green-50/50 dark:bg-green-900/10',
        )}
      >
        {/* Item icon / 项目图标 */}
        <div className={cn('p-2 rounded-lg', config.bgColor)}>
          <ItemIcon className={cn('h-5 w-5', config.color)} />
        </div>

        {/* Item content / 项目内容 */}
        <div className='flex-1 min-w-0'>
          <div className='flex items-center justify-between gap-2'>
            <h4 className='font-medium'>
              {t(`installer.precheckItems.${item.name.toLowerCase()}`) ||
                item.name}
            </h4>
            <StatusIcon className={cn('h-5 w-5 flex-shrink-0', config.color)} />
          </div>
          <p className='text-sm text-muted-foreground mt-1'>
            {localizeBackendText(item.message)}
          </p>

          {/* Details / 详细信息 */}
          {item.details && Object.keys(item.details).length > 0 && (
            <div className='mt-2 text-xs text-muted-foreground space-y-1'>
              {Object.entries(item.details).map(([key, value]) => (
                <div key={key} className='flex items-center gap-2'>
                  <span className='font-medium'>{key}:</span>
                  <span>{String(value)}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className='space-y-4'>
      {/* Header card / 头部卡片 */}
      <Card>
        <CardHeader className='pb-3'>
          <div className='flex items-center justify-between'>
            <div>
              <CardTitle className='text-lg'>
                {t('installer.precheck')}
              </CardTitle>
              <CardDescription>{t('installer.precheckDesc')}</CardDescription>
            </div>
            {getOverallStatusBadge()}
          </div>
        </CardHeader>
        <CardContent>
          <div className='flex items-center gap-4'>
            <Button
              onClick={onRunPrecheck}
              disabled={loading}
              data-testid='install-precheck-run'
              className='gap-2'
            >
              {loading ? (
                <>
                  <Loader2 className='h-4 w-4 animate-spin' />
                  {t('common.loading')}
                </>
              ) : result ? (
                <>
                  <RefreshCw className='h-4 w-4' />
                  {t('installer.rerunPrecheck')}
                </>
              ) : (
                <>
                  <PlayCircle className='h-4 w-4' />
                  {t('installer.runPrecheck')}
                </>
              )}
            </Button>

            {result && (
              <p className='text-sm text-muted-foreground'>
                {localizeBackendText(result.summary)}
              </p>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Error display / 错误显示 */}
      {error && (
        <Card className='border-destructive'>
          <CardContent className='pt-6'>
            <div className='flex items-start gap-2 text-destructive'>
              <XCircle className='h-5 w-5 flex-shrink-0 mt-0.5' />
              <p>{error}</p>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Precheck items / 预检查项 */}
      {result && result.items.length > 0 && (
        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='text-base'>
              {t('installer.precheckItems.title') || 'Check Results'}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea className='h-[300px] pr-4'>
              <div className='space-y-3'>
                {result.items.map(renderPrecheckItem)}
              </div>
            </ScrollArea>
          </CardContent>
        </Card>
      )}

      {/* Empty state / 空状态 */}
      {!result && !loading && !error && (
        <Card>
          <CardContent className='py-12'>
            <div className='text-center text-muted-foreground'>
              <PlayCircle className='h-12 w-12 mx-auto mb-4 opacity-50' />
              <p className='text-lg font-medium mb-2'>
                {t('installer.runPrecheckPrompt') || 'Run Precheck'}
              </p>
              <p className='text-sm'>
                {t('installer.runPrecheckPromptDesc') ||
                  'Click the button above to check if this host meets the installation requirements.'}
              </p>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export default PrecheckStep;
