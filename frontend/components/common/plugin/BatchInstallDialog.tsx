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
 * Batch Install Plugin Dialog
 * 批量安装插件对话框
 *
 * Allows installing multiple plugins to a selected cluster.
 * 允许将多个插件安装到选定的集群。
 */

'use client';

import { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import {
  resolvePreferredClusterId,
  rememberPreferredClusterId,
  isSoleDefaultCluster,
} from '@/lib/cluster-preference';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { ScrollArea } from '@/components/ui/scroll-area';
import { toast } from '@/lib/toast';
import { Loader2, CheckCircle, XCircle, AlertCircle } from 'lucide-react';
import services from '@/lib/services';
import type { Plugin } from '@/lib/services/plugin/types';

interface Cluster {
  id: number;
  name: string;
  version: string;
  infra_type?: string;
}

interface BatchInstallDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plugins: Plugin[];
  version: string;
}

interface InstallStatus {
  pluginName: string;
  status: 'pending' | 'installing' | 'success' | 'error';
  message?: string;
}

export function BatchInstallDialog({
  open,
  onOpenChange,
  plugins,
  version,
}: BatchInstallDialogProps) {
  const t = useTranslations();

  // State / 状态
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [selectedClusterId, setSelectedClusterId] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [installStatuses, setInstallStatuses] = useState<InstallStatus[]>([]);

  const loadClusters = useCallback(async () => {
    setLoading(true);
    try {
      // 获取集群列表（只取前 100 条，按需要可调整）
      const result = await services.cluster.getClustersSafe({
        current: 1,
        size: 100,
        name: undefined,
        status: undefined,
        deployment_mode: undefined,
      });

      if (result.success && result.data) {
        // Filter clusters with matching version / 过滤版本匹配的集群
        // K8s 集群遵循不可变镜像原则，不提供动态安装入口。
        // K8s clusters follow the immutable-image principle and are excluded.
        const matchingClusters = (result.data.clusters || []).filter(
          (c: Cluster) => c.version === version && !(c.infra_type && c.infra_type !== 'bare_metal'),
        );
        setClusters(matchingClusters);
        // 唯一匹配或已记住偏好时自动选中，免再手选。
        // Auto-select sole match or remembered preference.
        const preferred = resolvePreferredClusterId(matchingClusters);
        setSelectedClusterId(preferred != null ? String(preferred) : '');
      } else {
        setClusters([]);
        setSelectedClusterId('');
      }
    } catch (error) {
      console.error('Failed to load clusters:', error);
      setClusters([]);
    } finally {
      setLoading(false);
    }
  }, [version]);

  // Load clusters / 加载集群列表
  useEffect(() => {
    if (open) {
      void loadClusters();
      // Reset install statuses; cluster selection is filled after load.
      // 重置安装状态；集群选择在加载后自动填入。
      setInstallStatuses([]);
    }
  }, [open, loadClusters]);

  // Handle batch install / 处理批量安装
  const handleBatchInstall = async () => {
    if (!selectedClusterId || plugins.length === 0) {return;}

    setInstalling(true);
    const clusterId = parseInt(selectedClusterId, 10);

    // Initialize statuses / 初始化状态
    setInstallStatuses(
      plugins.map((p) => ({
        pluginName: p.name,
        status: 'pending',
      }))
    );

    let successCount = 0;
    let errorCount = 0;

    // Install plugins one by one / 逐个安装插件
    for (let i = 0; i < plugins.length; i++) {
      const plugin = plugins[i];

      // Update status to installing / 更新状态为安装中
      setInstallStatuses((prev) =>
        prev.map((s) =>
          s.pluginName === plugin.name ? { ...s, status: 'installing' } : s
        )
      );

      try {
        await services.plugin.installPluginAndWait(clusterId, plugin.name, version);
        successCount++;

        // Update status to success / 更新状态为成功
        setInstallStatuses((prev) =>
          prev.map((s) =>
            s.pluginName === plugin.name
              ? { ...s, status: 'success', message: t('plugin.installSuccess') }
              : s
          )
        );
      } catch (error) {
        errorCount++;
        const errorMsg =
          error instanceof Error ? error.message : t('plugin.installError');

        // Update status to error / 更新状态为错误
        setInstallStatuses((prev) =>
          prev.map((s) =>
            s.pluginName === plugin.name
              ? { ...s, status: 'error', message: errorMsg }
              : s
          )
        );
      }
    }

    setInstalling(false);

    // Show summary toast / 显示汇总提示
    if (errorCount === 0) {
      toast.success(
        t('plugin.batchInstallComplete', { count: successCount })
      );
    } else {
      toast.warning(
        t('plugin.batchInstallPartial', {
          success: successCount,
          failed: errorCount,
        })
      );
    }
  };

  // Calculate progress / 计算进度
  const completedCount = installStatuses.filter(
    (s) => s.status === 'success' || s.status === 'error'
  ).length;
  const progress =
    plugins.length > 0 ? (completedCount / plugins.length) * 100 : 0;

  // Get status icon / 获取状态图标
  const getStatusIcon = (status: InstallStatus['status']) => {
    switch (status) {
      case 'installing':
        return <Loader2 className="h-4 w-4 animate-spin text-blue-500" />;
      case 'success':
        return <CheckCircle className="h-4 w-4 text-green-500" />;
      case 'error':
        return <XCircle className="h-4 w-4 text-red-500" />;
      default:
        return <AlertCircle className="h-4 w-4 text-muted-foreground" />;
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-[500px]">
        <DialogHeader className="border-b-[0.5px] border-border/80 bg-muted/10 px-6 pt-5 pb-4">
          <DialogTitle>{t('plugin.batchInstall')}</DialogTitle>
          <DialogDescription>
            {t('plugin.batchInstallDesc', { count: plugins.length })}
          </DialogDescription>
        </DialogHeader>

        <div className="px-6 py-5">
          {/* Cluster selector / 集群选择器 */}
          <div className="space-y-2 pb-5">
            <label className="text-sm font-medium">
              {t('plugin.selectCluster')}
            </label>
            <Select
              value={selectedClusterId}
              onValueChange={(value) => {
                setSelectedClusterId(value);
                rememberPreferredClusterId(value);
              }}
              disabled={installing}
            >
              <SelectTrigger>
                <SelectValue placeholder={t('plugin.selectClusterPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {loading ? (
                  <SelectItem value="loading" disabled>
                    {t('common.loading')}
                  </SelectItem>
                ) : clusters.length === 0 ? (
                  <SelectItem value="none" disabled>
                    {t('plugin.noMatchingClusters', { version })}
                  </SelectItem>
                ) : (
                  clusters.map((cluster) => (
                    <SelectItem key={cluster.id} value={cluster.id.toString()}>
                      {cluster.name} (v{cluster.version})
                      {isSoleDefaultCluster(clusters, cluster.id)
                        ? ` · ${t('cluster.defaultBadge')}`
                        : ''}
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </div>

          {/* Plugin list / 插件列表 */}
          <div className="space-y-2.5 border-t-[0.5px] border-border/80 pt-5">
            <label className="text-sm font-medium">
              {t('plugin.pluginsToInstall')}
            </label>
            <ScrollArea className="h-[200px] pr-2">
              <div className="space-y-1.5">
                {plugins.map((plugin) => {
                  const status = installStatuses.find(
                    (s) => s.pluginName === plugin.name
                  );
                  return (
                    <div
                      key={plugin.name}
                      className="flex items-center justify-between rounded-md bg-muted/40 px-3 py-2"
                    >
                      <div className="flex items-center gap-2">
                        {status && getStatusIcon(status.status)}
                        <span className="text-sm font-medium">
                          {plugin.display_name || plugin.name}
                        </span>
                        <Badge variant="outline" className="text-xs">
                          {plugin.category}
                        </Badge>
                      </div>
                      {status?.message && status.status === 'error' && (
                        <span className="text-xs text-red-500 truncate max-w-[150px]">
                          {status.message}
                        </span>
                      )}
                    </div>
                  );
                })}
              </div>
            </ScrollArea>
          </div>

          {/* Progress bar / 进度条 */}
          {installing && (
            <div className="mt-5 space-y-2 border-t-[0.5px] border-border/80 pt-4">
              <div className="flex items-center justify-between text-sm">
                <span>{t('plugin.installing')}</span>
                <span>
                  {completedCount}/{plugins.length}
                </span>
              </div>
              <Progress value={progress} className="h-2" />
            </div>
          )}
        </div>

        <DialogFooter className="border-t-[0.5px] border-border/80 bg-muted/10 px-6 py-3.5">
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={installing}
          >
            {installing ? t('common.close') : t('common.cancel')}
          </Button>
          <Button
            onClick={handleBatchInstall}
            disabled={!selectedClusterId || installing || plugins.length === 0}
          >
            {installing ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                {t('plugin.installing')}
              </>
            ) : (
              t('plugin.installToCluster')
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
