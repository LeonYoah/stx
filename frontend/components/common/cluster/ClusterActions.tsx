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
 * Cluster Actions Component
 * 集群操作组件
 *
 * Provides action buttons for cluster operations (start, stop, restart).
 * 提供集群操作按钮（启动、停止、重启）。
 */

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {Play, Square, RotateCcw, Loader2} from 'lucide-react';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {ClusterInfo, ClusterStatus, OperationType} from '@/lib/services/cluster/types';

interface ClusterActionsProps {
  cluster: ClusterInfo;
  onOperationComplete: () => void;
  variant?: 'default' | 'compact';
}

/**
 * Cluster Actions Component
 * 集群操作组件
 */
export function ClusterActions({
  cluster,
  onOperationComplete,
  variant = 'default',
}: ClusterActionsProps) {
  const t = useTranslations();
  const [isOperating, setIsOperating] = useState(false);
  const [confirmOperation, setConfirmOperation] = useState<OperationType | null>(null);

  const canStart = cluster.status === ClusterStatus.CREATED || cluster.status === ClusterStatus.STOPPED;
  const canStop = cluster.status === ClusterStatus.RUNNING;
  const canRestart = cluster.status === ClusterStatus.RUNNING;

  /**
   * Execute operation
   * 执行操作
   */
  const executeOperation = async (operation: OperationType) => {
    setIsOperating(true);
    setConfirmOperation(null);

    try {
      let result;
      switch (operation) {
        case OperationType.START:
          result = await services.cluster.startClusterSafe(cluster.id);
          break;
        case OperationType.STOP:
          result = await services.cluster.stopClusterSafe(cluster.id);
          break;
        case OperationType.RESTART:
          result = await services.cluster.restartClusterSafe(cluster.id);
          break;
      }

      if (result.success) {
        // Check if auto-restart is managing the startup (check both message and node_results)
        // 检查是否由自动重启托管启动（检查 message 和 node_results）
        const isAutoRestart =
          result.data?.message?.includes('auto-restart') ||
          result.data?.message?.includes('auto-start') ||
          result.data?.node_results?.some(
            (nr) => nr.message?.includes('auto-restart') || nr.message?.includes('auto-start')
          );
        const successMessage = {
          [OperationType.START]: isAutoRestart ? t('cluster.startSuccessAutoRestart') : t('cluster.startSuccess'),
          [OperationType.STOP]: t('cluster.stopSuccess'),
          [OperationType.RESTART]: isAutoRestart ? t('cluster.restartSuccessAutoRestart') : t('cluster.restartSuccess'),
        }[operation];
        toast.success(successMessage);
        onOperationComplete();
      } else {
        const errorMessage = {
          [OperationType.START]: t('cluster.startError'),
          [OperationType.STOP]: t('cluster.stopError'),
          [OperationType.RESTART]: t('cluster.restartError'),
        }[operation];
        toast.error(result.error || errorMessage);
      }
    } finally {
      setIsOperating(false);
    }
  };

  /**
   * Handle operation click
   * 处理操作点击
   */
  const handleOperationClick = (operation: OperationType) => {
    // All start / stop / restart require user confirmation
    // 所有启动、停止、重启均需用户二次确认
    setConfirmOperation(operation);
  };

  /**
   * Get confirmation message
   * 获取确认消息
   */
  const getConfirmMessage = (operation: OperationType) => {
    switch (operation) {
      case OperationType.START:
        return t('cluster.startConfirmMessage', {name: cluster.name});
      case OperationType.STOP:
        return t('cluster.stopConfirmMessage', {name: cluster.name});
      case OperationType.RESTART:
        return t('cluster.restartConfirmMessage', {name: cluster.name});
      default:
        return '';
    }
  };

  /**
   * Get confirmation title
   * 获取确认标题
   */
  const getConfirmTitle = (operation: OperationType) => {
    switch (operation) {
      case OperationType.START:
        return t('cluster.startConfirmTitle');
      case OperationType.STOP:
        return t('cluster.stopConfirmTitle');
      case OperationType.RESTART:
        return t('cluster.restartConfirmTitle');
      default:
        return '';
    }
  };

  if (variant === 'compact') {
    return (
      <>
        <div className='flex gap-1'>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => handleOperationClick(OperationType.START)}
            disabled={!canStart || isOperating}
            title={t('cluster.start')}
          >
            {isOperating ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <Play className='h-4 w-4' />
            )}
          </Button>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => handleOperationClick(OperationType.STOP)}
            disabled={!canStop || isOperating}
            title={t('cluster.stop')}
          >
            <Square className='h-4 w-4' />
          </Button>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => handleOperationClick(OperationType.RESTART)}
            disabled={!canRestart || isOperating}
            title={t('cluster.restart')}
          >
            <RotateCcw className='h-4 w-4' />
          </Button>
        </div>

        {/* Confirmation Dialog / 确认对话框 */}
        <AlertDialog
          open={!!confirmOperation}
          onOpenChange={() => setConfirmOperation(null)}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {confirmOperation && getConfirmTitle(confirmOperation)}
              </AlertDialogTitle>
              <AlertDialogDescription>
                {confirmOperation && getConfirmMessage(confirmOperation)}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
              <AlertDialogAction
                onClick={() => confirmOperation && executeOperation(confirmOperation)}
              >
                {t('common.confirm')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </>
    );
  }

  return (
    <>
      <div className='flex gap-2'>
        <Button
          variant='outline'
          onClick={() => handleOperationClick(OperationType.START)}
          disabled={!canStart || isOperating}
        >
          {isOperating ? (
            <Loader2 className='h-4 w-4 mr-2 animate-spin' />
          ) : (
            <Play className='h-4 w-4 mr-2' />
          )}
          {t('cluster.start')}
        </Button>
        <Button
          variant='outline'
          onClick={() => handleOperationClick(OperationType.STOP)}
          disabled={!canStop || isOperating}
        >
          <Square className='h-4 w-4 mr-2' />
          {t('cluster.stop')}
        </Button>
        <Button
          variant='outline'
          onClick={() => handleOperationClick(OperationType.RESTART)}
          disabled={!canRestart || isOperating}
        >
          <RotateCcw className='h-4 w-4 mr-2' />
          {t('cluster.restart')}
        </Button>
      </div>

      {/* Confirmation Dialog / 确认对话框 */}
      <AlertDialog
        open={!!confirmOperation}
        onOpenChange={() => setConfirmOperation(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmOperation && getConfirmTitle(confirmOperation)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmOperation && getConfirmMessage(confirmOperation)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => confirmOperation && executeOperation(confirmOperation)}
            >
              {t('common.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
