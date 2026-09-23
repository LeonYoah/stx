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
 * Cluster Card Component
 * 集群卡片组件
 *
 * Displays a card for a single cluster with status, actions, and quick operations.
 * 显示单个集群的卡片，包含状态、操作和快捷操作。
 */

import {useState} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Card, CardContent, CardFooter, CardHeader, CardTitle} from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
import {
  MoreVertical,
  Play,
  Square,
  RotateCcw,
  Pencil,
  Trash2,
  Eye,
  Server,
  Loader2,
} from 'lucide-react';
import {Checkbox} from '@/components/ui/checkbox';
import {toast} from 'sonner';
import services from '@/lib/services';
import {ClusterInfo, ClusterStatus} from '@/lib/services/cluster/types';

interface ClusterCardProps {
  cluster: ClusterInfo;
  onEdit: (cluster: ClusterInfo) => void;
  onDelete: (cluster: ClusterInfo, options?: { forceDelete?: boolean }) => void;
  onRefresh: () => void;
  /** 唯一集群时展示默认标记 / Show default badge when this is the sole cluster */
  isDefault?: boolean;
}

/**
 * Get status badge variant
 * 获取状态徽章变体
 */
function getStatusBadgeVariant(
  status: ClusterStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case ClusterStatus.RUNNING:
      return 'default';
    case ClusterStatus.CREATED:
    case ClusterStatus.STOPPED:
      return 'secondary';
    case ClusterStatus.DEPLOYING:
      return 'outline';
    case ClusterStatus.ERROR:
      return 'destructive';
    default:
      return 'secondary';
  }
}

/**
 * Get status color class; when running but 0 nodes online, show as unhealthy (red).
 * 获取状态颜色类；运行中但 0 节点在线时按异常显示（红点）。
 */
function getStatusColorClass(cluster: ClusterInfo): string {
  if (cluster.status === ClusterStatus.RUNNING && (cluster.online_nodes === 0 || cluster.health_status === 'unhealthy')) {
    return 'bg-red-500';
  }
  switch (cluster.status) {
    case ClusterStatus.RUNNING:
      return 'bg-green-500';
    case ClusterStatus.DEPLOYING:
      return 'bg-yellow-500 animate-pulse';
    case ClusterStatus.STOPPED:
      return 'bg-gray-400';
    case ClusterStatus.ERROR:
      return 'bg-red-500';
    default:
      return 'bg-gray-300';
  }
}

/**
 * Whether to show cluster as "unhealthy" on the card (running but 0 online).
 * 是否在卡片上显示为「异常」（运行中但 0 在线）。
 */
function isRunningButUnhealthy(cluster: ClusterInfo): boolean {
  return cluster.status === ClusterStatus.RUNNING && (cluster.online_nodes === 0 || cluster.health_status === 'unhealthy');
}

/**
 * Cluster Card Component
 * 集群卡片组件
 */
type ConfirmOp = 'start' | 'stop' | 'restart';

export function ClusterCard({
  cluster,
  onEdit,
  onDelete,
  onRefresh,
  isDefault = false,
}: ClusterCardProps) {
  const t = useTranslations();
  const router = useRouter();
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
  const [forceDelete, setForceDelete] = useState(false);
  const [confirmOp, setConfirmOp] = useState<ConfirmOp | null>(null);
  const [isOperating, setIsOperating] = useState(false);

  /**
   * Handle start cluster
   * 处理启动集群
   */
  const handleStart = async () => {
    setIsOperating(true);
    try {
      const result = await services.cluster.startClusterSafe(cluster.id);
      if (result.success) {
        // Check if auto-restart is managing the startup (check both message and node_results)
        // 检查是否由自动重启托管启动（检查 message 和 node_results）
        const isAutoRestart =
          result.data?.message?.includes('auto-restart') ||
          result.data?.message?.includes('auto-start') ||
          result.data?.node_results?.some(
            (nr) => nr.message?.includes('auto-restart') || nr.message?.includes('auto-start')
          );
        toast.success(isAutoRestart ? t('cluster.startSuccessAutoRestart') : t('cluster.startSuccess'));
        onRefresh();
      } else {
        toast.error(result.error || t('cluster.startError'));
      }
    } finally {
      setIsOperating(false);
    }
  };

  /**
   * Handle stop cluster
   * 处理停止集群
   */
  const handleStop = async () => {
    setIsOperating(true);
    try {
      const result = await services.cluster.stopClusterSafe(cluster.id);
      if (result.success) {
        toast.success(t('cluster.stopSuccess'));
        onRefresh();
      } else {
        toast.error(result.error || t('cluster.stopError'));
      }
    } finally {
      setIsOperating(false);
    }
  };

  /**
   * Handle restart cluster
   * 处理重启集群
   */
  const handleRestart = async () => {
    setIsOperating(true);
    try {
      const result = await services.cluster.restartClusterSafe(cluster.id);
      if (result.success) {
        // Check if auto-restart is managing the startup (check both message and node_results)
        // 检查是否由自动重启托管启动（检查 message 和 node_results）
        const isAutoRestart =
          result.data?.message?.includes('auto-restart') ||
          result.data?.message?.includes('auto-start') ||
          result.data?.node_results?.some(
            (nr) => nr.message?.includes('auto-restart') || nr.message?.includes('auto-start')
          );
        toast.success(isAutoRestart ? t('cluster.restartSuccessAutoRestart') : t('cluster.restartSuccess'));
        onRefresh();
      } else {
        toast.error(result.error || t('cluster.restartError'));
      }
    } finally {
      setIsOperating(false);
    }
  };

  /**
   * Handle view detail
   * 处理查看详情
   */
  const handleViewDetail = () => {
    router.push(`/clusters/${cluster.id}`);
  };

  /**
   * Handle confirm delete
   * 处理确认删除
   */
  const handleConfirmDelete = () => {
    setIsDeleteDialogOpen(false);
    onDelete(cluster, { forceDelete });
    setForceDelete(false);
  };

  const canStart = cluster.status === ClusterStatus.CREATED || cluster.status === ClusterStatus.STOPPED;
  const canStop = cluster.status === ClusterStatus.RUNNING;
  const canRestart = cluster.status === ClusterStatus.RUNNING;
  const canDelete = cluster.status !== ClusterStatus.RUNNING && cluster.status !== ClusterStatus.DEPLOYING;

  return (
    <>
      <Card className='grid-card-animate hover:shadow-md transition-shadow min-h-[320px] flex flex-col'>
        <CardHeader className='pb-3 pt-5 px-5'>
          <div className='flex items-start justify-between'>
            <div className='flex items-center gap-2'>
              <div className={`w-2 h-2 rounded-full ${getStatusColorClass(cluster)}`} />
              <CardTitle className='text-lg'>{cluster.name}</CardTitle>
              {isDefault ? (
                <Badge variant='secondary' className='text-[10px] px-1.5 py-0'>
                  {t('cluster.defaultBadge')}
                </Badge>
              ) : null}
            </div>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant='ghost' size='icon' className='h-8 w-8'>
                  <MoreVertical className='h-4 w-4' />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end'>
                <DropdownMenuItem onClick={handleViewDetail}>
                  <Eye className='h-4 w-4 mr-2' />
                  {t('common.view')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => onEdit(cluster)}>
                  <Pencil className='h-4 w-4 mr-2' />
                  {t('common.edit')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={() => setConfirmOp('start')}
                  disabled={!canStart || isOperating}
                >
                  <Play className='h-4 w-4 mr-2' />
                  {t('cluster.start')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => setConfirmOp('stop')}
                  disabled={!canStop || isOperating}
                >
                  <Square className='h-4 w-4 mr-2' />
                  {t('cluster.stop')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => setConfirmOp('restart')}
                  disabled={!canRestart || isOperating}
                >
                  <RotateCcw className='h-4 w-4 mr-2' />
                  {t('cluster.restart')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={() => setIsDeleteDialogOpen(true)}
                  disabled={!canDelete}
                  className='text-destructive'
                >
                  <Trash2 className='h-4 w-4 mr-2' />
                  {t('common.delete')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          {cluster.description && (
            <p className='text-sm text-muted-foreground mt-1 line-clamp-2'>
              {cluster.description}
            </p>
          )}
        </CardHeader>

        <CardContent className='pb-3 px-5 flex-1'>
          <div className='space-y-4'>
            <div className='flex items-center justify-between'>
              <span className='text-sm text-muted-foreground'>{t('cluster.status')}</span>
              <Badge
                variant={isRunningButUnhealthy(cluster) ? 'destructive' : getStatusBadgeVariant(cluster.status)}
              >
                {isRunningButUnhealthy(cluster)
                  ? t('cluster.healthStatuses.unhealthy')
                  : t(`cluster.statuses.${cluster.status}`)}
              </Badge>
            </div>

            <div className='flex items-center justify-between'>
              <span className='text-sm text-muted-foreground'>{t('cluster.deploymentMode')}</span>
              <Badge variant='outline'>
                {t(`cluster.modes.${cluster.deployment_mode}`)}
              </Badge>
            </div>

            <div className='flex items-center justify-between'>
              <span className='text-sm text-muted-foreground'>{t('cluster.nodes')}</span>
              <div className='flex items-center gap-1'>
                <Server className='h-4 w-4 text-muted-foreground' />
                <span className='text-sm font-medium'>{cluster.node_count}</span>
              </div>
            </div>

            {cluster.version && (
              <div className='flex items-center justify-between'>
                <span className='text-sm text-muted-foreground'>{t('cluster.version')}</span>
                <span className='text-sm'>{cluster.version}</span>
              </div>
            )}
          </div>
        </CardContent>

        <CardFooter className='pt-3 pb-5 px-5'>
          <div className='flex w-full gap-2'>
            {canStart && (
              <Button
                variant='outline'
                size='sm'
                className='flex-1'
                onClick={() => setConfirmOp('start')}
                disabled={isOperating}
              >
                {isOperating ? (
                  <Loader2 className='h-4 w-4 mr-1 animate-spin' />
                ) : (
                  <Play className='h-4 w-4 mr-1' />
                )}
                {t('cluster.start')}
              </Button>
            )}
            {canStop && (
              <Button
                variant='outline'
                size='sm'
                className='flex-1'
                onClick={() => setConfirmOp('stop')}
                disabled={isOperating}
              >
                {isOperating ? (
                  <Loader2 className='h-4 w-4 mr-1 animate-spin' />
                ) : (
                  <Square className='h-4 w-4 mr-1' />
                )}
                {t('cluster.stop')}
              </Button>
            )}
            <Button
              variant='default'
              size='sm'
              className='flex-1'
              onClick={handleViewDetail}
            >
              <Eye className='h-4 w-4 mr-1' />
              {t('common.view')}
            </Button>
          </div>
        </CardFooter>
      </Card>

      {/* Start/Stop/Restart confirmation / 启动/停止/重启二次确认 */}
      <AlertDialog open={!!confirmOp} onOpenChange={() => setConfirmOp(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmOp === 'start' && t('cluster.startConfirmTitle')}
              {confirmOp === 'stop' && t('cluster.stopConfirmTitle')}
              {confirmOp === 'restart' && t('cluster.restartConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmOp === 'start' && t('cluster.startConfirmMessage', {name: cluster.name})}
              {confirmOp === 'stop' && t('cluster.stopConfirmMessage', {name: cluster.name})}
              {confirmOp === 'restart' && t('cluster.restartConfirmMessage', {name: cluster.name})}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={async () => {
                if (confirmOp === 'start') {
                  await handleStart();
                } else if (confirmOp === 'stop') {
                  await handleStop();
                } else if (confirmOp === 'restart') {
                  await handleRestart();
                }
                setConfirmOp(null);
              }}
            >
              {confirmOp === 'start' && t('cluster.start')}
              {confirmOp === 'stop' && t('cluster.stop')}
              {confirmOp === 'restart' && t('cluster.restart')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Confirmation Dialog / 删除确认对话框 */}
      <AlertDialog
        open={isDeleteDialogOpen}
        onOpenChange={(open) => {
          setIsDeleteDialogOpen(open);
          if (!open) {setForceDelete(false);}
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('cluster.deleteCluster')}</AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div className='space-y-3'>
                <p>{t('cluster.deleteConfirm', {name: cluster.name})}</p>
                <p className='text-sm text-muted-foreground'>
                  {t('cluster.deleteConfirmWarning')}
                </p>
                <label className='flex items-center gap-2 cursor-pointer'>
                  <Checkbox
                    checked={forceDelete}
                    onCheckedChange={(v) => setForceDelete(v === true)}
                  />
                  <span className='text-sm'>{t('cluster.forceDeleteOption')}</span>
                </label>
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmDelete}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
