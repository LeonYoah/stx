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
 * MonitorConfigPanel - 监控配置面板组件
 * Displays and manages monitoring configuration for a cluster.
 * 显示和管理集群的监控配置。
 * Requirements: 5.1, 5.3, 5.8
 */

'use client';

import { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Loader2, Save, RotateCcw, Settings, Activity, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import {
  MonitorConfig,
  UpdateMonitorConfigRequest,
  getMonitorConfig,
  updateMonitorConfig,
} from '@/lib/services/monitor';
import {formatDateTimeTz} from '@/lib/timezone';

interface MonitorConfigPanelProps {
  clusterId: number;
  clusterName?: string;
}

export function MonitorConfigPanel({ clusterId, clusterName }: MonitorConfigPanelProps) {
  const t = useTranslations();
  const [config, setConfig] = useState<MonitorConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);

  // Form state / 表单状态
  const [formData, setFormData] = useState<UpdateMonitorConfigRequest>({});

  // Load config / 加载配置
  const loadConfig = useCallback(async () => {
    try {
      setLoading(true);
      const data = await getMonitorConfig(clusterId);
      setConfig(data);
      setFormData({
        auto_restart: data.auto_restart,
        monitor_interval: data.monitor_interval,
        restart_delay: data.restart_delay,
        max_restarts: data.max_restarts,
        time_window: data.time_window,
        cooldown_period: data.cooldown_period,
      });
      setHasChanges(false);
    } catch (error) {
      console.error('Failed to load monitor config:', error);
      toast.error(t('monitor.loadError'));
    } finally {
      setLoading(false);
    }
  }, [clusterId, t]);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  // Handle form changes / 处理表单变更
  const handleChange = (field: keyof UpdateMonitorConfigRequest, value: boolean | number) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setHasChanges(true);
  };

  // Save config / 保存配置
  const handleSave = async () => {
    try {
      setSaving(true);
      const updated = await updateMonitorConfig(clusterId, {
        auto_restart: formData.auto_restart,
        monitor_interval: formData.monitor_interval,
        restart_delay: formData.restart_delay,
        max_restarts: formData.max_restarts,
        time_window: formData.time_window,
        cooldown_period: formData.cooldown_period,
      });
      setConfig(updated);
      setHasChanges(false);
      toast.success(t('monitor.saveSuccess'));
    } catch (error) {
      console.error('Failed to save monitor config:', error);
      toast.error(t('monitor.saveError'));
    } finally {
      setSaving(false);
    }
  };

  // Reset form / 重置表单
  const handleReset = () => {
    if (config) {
      setFormData({
        auto_restart: config.auto_restart,
        monitor_interval: config.monitor_interval,
        restart_delay: config.restart_delay,
        max_restarts: config.max_restarts,
        time_window: config.time_window,
        cooldown_period: config.cooldown_period,
      });
      setHasChanges(false);
    }
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="flex items-center justify-center py-8">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Settings className="h-5 w-5" />
            <CardTitle>{t('monitor.title')}</CardTitle>
          </div>
          <div className="flex items-center gap-2">
            {config?.last_sync_at && (
              <Badge variant="outline" className="text-xs">
                {t('monitor.lastSync')}: {formatDateTimeTz(config.last_sync_at)}
              </Badge>
            )}
            <Badge variant="secondary">v{config?.config_version}</Badge>
          </div>
        </div>
        <CardDescription>
          {clusterName
            ? t('monitor.descriptionWithCluster', { name: clusterName })
            : t('monitor.description')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Main switches / 主开关 */}
        <div className="grid gap-4 md:grid-cols-2">
          <div className="flex items-center justify-between rounded-lg border p-4">
            <Label className="flex items-center gap-2">
              <Activity className="h-4 w-4" />
              {t('monitor.autoMonitor')}
            </Label>
            {/* 「始终开启」说明进 Badge Tooltip / "Always on" explanation lives in the badge tooltip */}
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="default" className="shrink-0 cursor-help" data-testid="monitor-auto-always-on">
                  {t('monitor.alwaysOn')}
                </Badge>
              </TooltipTrigger>
              <TooltipContent side="top" className="max-w-xs text-xs leading-relaxed">
                {t('monitor.autoMonitorDesc')}
              </TooltipContent>
            </Tooltip>
          </div>
          <div className="flex items-center justify-between rounded-lg border p-4">
            <div className="space-y-0.5">
              <Label className="flex items-center gap-2">
                <RefreshCw className="h-4 w-4" />
                {t('monitor.autoRestart')}
              </Label>
              <p className="text-sm text-muted-foreground">{t('monitor.autoRestartDesc')}</p>
            </div>
            <Switch
              checked={formData.auto_restart}
              onCheckedChange={(checked) => handleChange('auto_restart', checked)}
            />
          </div>
        </div>

        {/* Config parameters / 配置参数 */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="monitor_interval">{t('monitor.monitorInterval')}</Label>
            <div className="flex items-center gap-2">
              <Input
                id="monitor_interval"
                type="number"
                min={1}
                max={60}
                value={formData.monitor_interval}
                onChange={(e) => handleChange('monitor_interval', parseInt(e.target.value) || 5)}
                className="w-24"
              />
              <span className="text-sm text-muted-foreground">{t('monitor.seconds')}</span>
            </div>
            <p className="text-xs text-muted-foreground">{t('monitor.monitorIntervalHint')}</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="restart_delay">{t('monitor.restartDelay')}</Label>
            <div className="flex items-center gap-2">
              <Input
                id="restart_delay"
                type="number"
                min={1}
                max={300}
                value={formData.restart_delay}
                onChange={(e) => handleChange('restart_delay', parseInt(e.target.value) || 10)}
                className="w-24"
              />
              <span className="text-sm text-muted-foreground">{t('monitor.seconds')}</span>
            </div>
            <p className="text-xs text-muted-foreground">{t('monitor.restartDelayHint')}</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="max_restarts">{t('monitor.maxRestarts')}</Label>
            <div className="flex items-center gap-2">
              <Input
                id="max_restarts"
                type="number"
                min={1}
                max={10}
                value={formData.max_restarts}
                onChange={(e) => handleChange('max_restarts', parseInt(e.target.value) || 3)}
                className="w-24"
              />
              <span className="text-sm text-muted-foreground">{t('monitor.times')}</span>
            </div>
            <p className="text-xs text-muted-foreground">{t('monitor.maxRestartsHint')}</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="time_window">{t('monitor.timeWindow')}</Label>
            <div className="flex items-center gap-2">
              <Input
                id="time_window"
                type="number"
                min={60}
                max={3600}
                value={formData.time_window}
                onChange={(e) => handleChange('time_window', parseInt(e.target.value) || 300)}
                className="w-24"
              />
              <span className="text-sm text-muted-foreground">{t('monitor.seconds')}</span>
            </div>
            <p className="text-xs text-muted-foreground">{t('monitor.timeWindowHint')}</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="cooldown_period">{t('monitor.cooldownPeriod')}</Label>
            <div className="flex items-center gap-2">
              <Input
                id="cooldown_period"
                type="number"
                min={60}
                max={86400}
                value={formData.cooldown_period}
                onChange={(e) => handleChange('cooldown_period', parseInt(e.target.value) || 1800)}
                className="w-24"
              />
              <span className="text-sm text-muted-foreground">{t('monitor.seconds')}</span>
            </div>
            <p className="text-xs text-muted-foreground">{t('monitor.cooldownPeriodHint')}</p>
          </div>
        </div>

        {/* Action buttons / 操作按钮 */}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={handleReset} disabled={!hasChanges || saving}>
            <RotateCcw className="mr-2 h-4 w-4" />
            {t('common.reset')}
          </Button>
          <Button onClick={handleSave} disabled={!hasChanges || saving}>
            {saving ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-2 h-4 w-4" />
            )}
            {t('common.save')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
