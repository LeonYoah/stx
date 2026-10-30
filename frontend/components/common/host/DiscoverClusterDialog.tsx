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
 * DiscoverClusterDialog - 发现集群对话框组件
 * Displays cluster discovery progress and results.
 * 显示集群发现进度和结果。
 * Requirements: 1.2, 1.9, 9.1, 9.2
 */

'use client';

import { useState, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Loader2, Search, CheckCircle, AlertCircle, Server, FolderOpen, Tag } from 'lucide-react';
import { toast } from 'sonner';
import {
  DiscoveredCluster,
  DiscoveryResult,
  triggerDiscovery,
  confirmDiscovery,
} from '@/lib/services/discovery';

interface DiscoverClusterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hostId: number;
  hostName: string;
  onSuccess?: (clusterIds: number[]) => void;
}

type DiscoveryState = 'idle' | 'discovering' | 'discovered' | 'confirming' | 'confirmed' | 'error';

export function DiscoverClusterDialog({
  open,
  onOpenChange,
  hostId,
  hostName,
  onSuccess,
}: DiscoverClusterDialogProps) {
  const t = useTranslations();
  const [state, setState] = useState<DiscoveryState>('idle');
  const [result, setResult] = useState<DiscoveryResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedDirs, setSelectedDirs] = useState<Set<string>>(new Set());

  // Start discovery / 开始发现
  const handleDiscover = useCallback(async () => {
    try {
      setState('discovering');
      setError(null);
      const discoveryResult = await triggerDiscovery(hostId);
      setResult(discoveryResult);
      
      // Auto-select new clusters / 自动选择新发现的集群
      const newDirs = new Set<string>();
      discoveryResult.clusters?.forEach((cluster) => {
        if (cluster.is_new) {
          newDirs.add(cluster.install_dir);
        }
      });
      setSelectedDirs(newDirs);
      
      setState('discovered');
    } catch (err) {
      console.error('Discovery failed:', err);
      setError(t('discovery.discoverFailed'));
      setState('error');
    }
  }, [hostId, t]);

  // Confirm import / 确认导入
  const handleConfirm = useCallback(async () => {
    if (selectedDirs.size === 0) {
      toast.error(t('discovery.selectAtLeastOne'));
      return;
    }

    try {
      setState('confirming');
      const response = await confirmDiscovery(hostId, {
        install_dirs: Array.from(selectedDirs),
      });
      
      setState('confirmed');
      toast.success(t('discovery.importSuccess', { count: response.count }));
      
      if (onSuccess) {
        onSuccess(response.cluster_ids);
      }
      
      // Close dialog after short delay / 短暂延迟后关闭对话框
      setTimeout(() => {
        onOpenChange(false);
        resetState();
      }, 1500);
    } catch (err) {
      console.error('Confirm failed:', err);
      setError(t('discovery.confirmFailed'));
      setState('error');
    }
  }, [hostId, selectedDirs, t, onSuccess, onOpenChange]);

  // Reset state / 重置状态
  const resetState = () => {
    setState('idle');
    setResult(null);
    setError(null);
    setSelectedDirs(new Set());
  };

  // Handle dialog close / 处理对话框关闭
  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      resetState();
    }
    onOpenChange(newOpen);
  };

  // Toggle cluster selection / 切换集群选择
  const toggleCluster = (installDir: string) => {
    setSelectedDirs((prev) => {
      const next = new Set(prev);
      if (next.has(installDir)) {
        next.delete(installDir);
      } else {
        next.add(installDir);
      }
      return next;
    });
  };

  // Render cluster card / 渲染集群卡片
  const renderClusterCard = (cluster: DiscoveredCluster) => {
    const isSelected = selectedDirs.has(cluster.install_dir);
    const isNew = cluster.is_new;

    return (
      <div
        key={cluster.install_dir}
        className={`rounded-lg border p-4 transition-colors ${
          isSelected ? 'border-primary bg-primary/5' : 'border-border'
        } ${!isNew ? 'opacity-60' : ''}`}
      >
        <div className="flex items-start gap-3">
          {isNew && (
            <Checkbox
              checked={isSelected}
              onCheckedChange={() => toggleCluster(cluster.install_dir)}
              className="mt-1"
            />
          )}
          <div className="flex-1 space-y-2">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Server className="h-4 w-4 text-muted-foreground" />
                <span className="font-medium">{cluster.name || t('discovery.unknownCluster')}</span>
              </div>
              {isNew ? (
                <Badge variant="default">{t('discovery.new')}</Badge>
              ) : (
                <Badge variant="secondary">{t('discovery.existing')}</Badge>
              )}
            </div>
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <FolderOpen className="h-3 w-3" />
              <span className="font-mono">{cluster.install_dir}</span>
            </div>
            <div className="flex items-center gap-4 text-sm">
              {cluster.version && (
                <div className="flex items-center gap-1">
                  <Tag className="h-3 w-3" />
                  <span>{cluster.version}</span>
                </div>
              )}
              {cluster.deployment_mode && (
                <Badge variant="outline">{cluster.deployment_mode}</Badge>
              )}
              {cluster.nodes && (
                <span className="text-muted-foreground">
                  {t('discovery.nodesCount', { count: cluster.nodes.length })}
                </span>
              )}
            </div>
            {!isNew && cluster.existing_id > 0 && (
              <p className="text-xs text-muted-foreground">
                {t('discovery.existingClusterId', { id: cluster.existing_id })}
              </p>
            )}
          </div>
        </div>
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Search className="h-5 w-5" />
            {t('discovery.title')}
          </DialogTitle>
          <DialogDescription>
            {t('discovery.description', { host: hostName })}
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          {/* Idle state / 空闲状态 */}
          {state === 'idle' && (
            <div className="flex flex-col items-center justify-center py-8 text-center">
              <Search className="h-12 w-12 text-muted-foreground mb-4" />
              <Button onClick={handleDiscover}>
                <Search className="mr-2 h-4 w-4" />
                {t('discovery.startDiscover')}
              </Button>
            </div>
          )}

          {/* Discovering state / 发现中状态 */}
          {state === 'discovering' && (
            <div className="flex flex-col items-center justify-center py-8">
              <Loader2 className="h-12 w-12 animate-spin text-primary mb-4" />
              <p className="text-muted-foreground">{t('discovery.discovering')}</p>
            </div>
          )}

          {/* Discovered state / 已发现状态 */}
          {state === 'discovered' && result && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  {t('discovery.foundClusters', {
                    total: result.clusters?.length || 0,
                    new: result.new_count,
                    existing: result.exist_count,
                  })}
                </p>
                {result.new_count > 0 && (
                  <p className="text-sm">
                    {t('discovery.selectedCount', { count: selectedDirs.size })}
                  </p>
                )}
              </div>

              {result.clusters && result.clusters.length > 0 ? (
                <ScrollArea className="h-[300px] pr-4">
                  <div className="space-y-3">
                    {result.clusters.map(renderClusterCard)}
                  </div>
                </ScrollArea>
              ) : (
                <div className="flex flex-col items-center justify-center py-8 text-center">
                  <AlertCircle className="h-12 w-12 text-muted-foreground mb-4" />
                  <p className="text-muted-foreground">{t('discovery.noClustersFound')}</p>
                </div>
              )}
            </div>
          )}

          {/* Confirming state / 确认中状态 */}
          {state === 'confirming' && (
            <div className="flex flex-col items-center justify-center py-8">
              <Loader2 className="h-12 w-12 animate-spin text-primary mb-4" />
              <p className="text-muted-foreground">{t('discovery.importing')}</p>
            </div>
          )}

          {/* Confirmed state / 已确认状态 */}
          {state === 'confirmed' && (
            <div className="flex flex-col items-center justify-center py-8">
              <CheckCircle className="h-12 w-12 text-green-500 mb-4" />
              <p className="text-green-600">{t('discovery.importComplete')}</p>
            </div>
          )}

          {/* Error state / 错误状态 */}
          {state === 'error' && (
            <div className="flex flex-col items-center justify-center py-8">
              <AlertCircle className="h-12 w-12 text-destructive mb-4" />
              <p className="text-destructive mb-4">{error}</p>
              <Button variant="outline" onClick={() => setState('idle')}>
                {t('common.retry')}
              </Button>
            </div>
          )}
        </div>

        <DialogFooter>
          {state === 'discovered' && result && result.new_count > 0 && (
            <>
              <Button variant="outline" onClick={() => handleOpenChange(false)}>
                {t('common.cancel')}
              </Button>
              <Button onClick={handleConfirm} disabled={selectedDirs.size === 0}>
                {t('discovery.importSelected', { count: selectedDirs.size })}
              </Button>
            </>
          )}
          {(state === 'idle' || state === 'error' || (state === 'discovered' && result?.new_count === 0)) && (
            <Button variant="outline" onClick={() => handleOpenChange(false)}>
              {t('common.close')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
