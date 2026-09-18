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
 * Config Management Main Component
 * 配置管理主组件
 */

'use client';

import { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';
import { RefreshCw, Settings, FileText, Server, Check, AlertTriangle } from 'lucide-react';
import {WorkspaceHeader} from '@/components/common/layout';
import { motion } from 'motion/react';
import services from '@/lib/services';
import type { ConfigInfo, ConfigVersionInfo } from '@/lib/services/config';
import { ConfigType, ConfigTypeNames } from '@/lib/services/config';
import { ConfigEditor } from './ConfigEditor';
import { ConfigVersionHistory } from './ConfigVersionHistory';

interface ConfigMainProps {
  clusterId: number;
  clusterName?: string;
}

/**
 * Config Management Main Component
 * 配置管理主组件
 */
export function ConfigMain({ clusterId, clusterName }: ConfigMainProps) {
  const t = useTranslations();

  // Data state / 数据状态
  const [configs, setConfigs] = useState<ConfigInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Selection state / 选择状态
  const [selectedConfigType, setSelectedConfigType] = useState<ConfigType>(ConfigType.SEATUNNEL);
  const [selectedConfig, setSelectedConfig] = useState<ConfigInfo | null>(null);
  const [activeTab, setActiveTab] = useState<'template' | 'nodes'>('template');

  // Editor state / 编辑器状态
  const [isEditing, setIsEditing] = useState(false);
  const [editContent, setEditContent] = useState('');

  // Version history state / 版本历史状态
  const [versions, setVersions] = useState<ConfigVersionInfo[]>([]);
  const [showVersions, setShowVersions] = useState(false);

  /**
   * Load cluster configs
   * 加载集群配置
   */
  const loadConfigs = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await services.config.getClusterConfigs(clusterId);
      setConfigs(result || []);
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : t('config.loadError');
      setError(errorMsg);
      toast.error(errorMsg);
    } finally {
      setLoading(false);
    }
  }, [clusterId, t]);

  useEffect(() => {
    loadConfigs();
  }, [loadConfigs]);

  /**
   * Get template config for selected type
   * 获取选中类型的模板配置
   */
  const getTemplateConfig = useCallback(() => {
    return configs.find(c => c.config_type === selectedConfigType && c.is_template);
  }, [configs, selectedConfigType]);

  /**
   * Get node configs for selected type
   * 获取选中类型的节点配置
   */
  const getNodeConfigs = useCallback(() => {
    return configs.filter(c => c.config_type === selectedConfigType && !c.is_template);
  }, [configs, selectedConfigType]);

  /**
   * Handle config type change
   * 处理配置类型变更
   */
  const handleConfigTypeChange = (type: ConfigType) => {
    setSelectedConfigType(type);
    setSelectedConfig(null);
    setIsEditing(false);
    setShowVersions(false);
  };

  /**
   * Handle select config
   * 处理选择配置
   */
  const handleSelectConfig = (config: ConfigInfo) => {
    setSelectedConfig(config);
    setEditContent(config.content);
    setIsEditing(false);
    setShowVersions(false);
  };

  /**
   * Handle save config
   * 处理保存配置
   */
  const handleSaveConfig = async (comment?: string) => {
    if (!selectedConfig) {return;}

    try {
      await services.config.updateConfig(selectedConfig.id, {
        content: editContent,
        comment: comment || '',
      });
      toast.success(t('config.saveSuccess'));
      setIsEditing(false);
      loadConfigs();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : t('config.saveFailed');
      toast.error(errorMsg);
    }
  };

  /**
   * Handle promote config to cluster
   * 处理推广配置到集群
   */
  const handlePromote = async (config: ConfigInfo) => {
    try {
      await services.config.promoteConfig(config.id, {
        comment: t('config.promoteComment'),
      });
      toast.success(t('config.promoteSuccess'));
      loadConfigs();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : t('config.promoteFailed');
      toast.error(errorMsg);
    }
  };

  /**
   * Handle sync from template
   * 处理从模板同步
   */
  const handleSyncFromTemplate = async (config: ConfigInfo) => {
    try {
      await services.config.syncFromTemplate(config.id, {
        comment: t('config.syncComment'),
      });
      toast.success(t('config.syncSuccess'));
      loadConfigs();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : t('config.syncFailed');
      toast.error(errorMsg);
    }
  };

  /**
   * Handle view version history
   * 处理查看版本历史
   */
  const handleViewVersions = async (config: ConfigInfo) => {
    try {
      const result = await services.config.getConfigVersions(config.id);
      setVersions(result || []);
      setSelectedConfig(config);
      setShowVersions(true);
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : t('config.loadVersionsFailed');
      toast.error(errorMsg);
    }
  };

  /**
   * Handle rollback to version
   * 处理回滚到指定版本
   */
  const handleRollback = async (version: number) => {
    if (!selectedConfig) {return;}

    try {
      await services.config.rollbackConfig(selectedConfig.id, {
        version,
        comment: t('config.rollbackComment', { version }),
      });
      toast.success(t('config.rollbackSuccess'));
      setShowVersions(false);
      loadConfigs();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : t('config.rollbackFailed');
      toast.error(errorMsg);
    }
  };

  const containerVariants = {
    hidden: { opacity: 0 },
    visible: {
      opacity: 1,
      transition: { duration: 0.5, staggerChildren: 0.1 },
    },
  };

  const itemVariants = {
    hidden: { opacity: 0, y: 20 },
    visible: { opacity: 1, y: 0, transition: { duration: 0.6 } },
  };

  const templateConfig = getTemplateConfig();
  const nodeConfigs = getNodeConfigs();

  return (
    <motion.div
      className="space-y-6"
      initial="hidden"
      animate="visible"
      variants={containerVariants}
    >
      {/* Header / 标题 */}
      <motion.div variants={itemVariants}>
        <WorkspaceHeader
          icon={<Settings />}
          title={t('config.title')}
          subtitle={
            clusterName ? t('config.clusterConfigDesc', { name: clusterName }) : t('config.desc')
          }
          actions={
            <Button variant="outline" onClick={loadConfigs} disabled={loading}>
              <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
              {t('common.refresh')}
            </Button>
          }
        />
      </motion.div>

      <Separator />

      {/* Error display / 错误显示 */}
      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6">
            <p className="text-destructive">{error}</p>
          </CardContent>
        </Card>
      )}

      {/* Config type selector / 配置类型选择器 */}
      <motion.div variants={itemVariants}>
        <Select
          value={selectedConfigType}
          onValueChange={(v) => handleConfigTypeChange(v as ConfigType)}
        >
          <SelectTrigger className="w-[250px]">
            <SelectValue placeholder={t('config.selectType')} />
          </SelectTrigger>
          <SelectContent>
            {Object.values(ConfigType).map((type) => (
              <SelectItem key={type} value={type}>
                <div className="flex items-center gap-2">
                  <FileText className="h-4 w-4" />
                  {ConfigTypeNames[type]}
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </motion.div>

      {/* Config tabs / 配置标签页 */}
      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'template' | 'nodes')}>
        <TabsList>
          <TabsTrigger value="template" className="flex items-center gap-2">
            <FileText className="h-4 w-4" />
            {t('config.clusterTemplate')}
          </TabsTrigger>
          <TabsTrigger value="nodes" className="flex items-center gap-2">
            <Server className="h-4 w-4" />
            {t('config.nodeConfigs')}
            {nodeConfigs.length > 0 && (
              <Badge variant="secondary" className="ml-1">
                {nodeConfigs.length}
              </Badge>
            )}
          </TabsTrigger>
        </TabsList>

        {/* Template tab / 模板标签页 */}
        <TabsContent value="template" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                {t('config.clusterTemplate')}
                <Badge variant="outline">{ConfigTypeNames[selectedConfigType]}</Badge>
              </CardTitle>
              <CardDescription>
                {t('config.templateDesc')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {loading ? (
                <div className="text-center py-8 text-muted-foreground">
                  <RefreshCw className="h-8 w-8 mx-auto animate-spin mb-4" />
                  <p>{t('common.loading')}</p>
                </div>
              ) : templateConfig ? (
                <ConfigEditor
                  config={templateConfig}
                  content={selectedConfig?.id === templateConfig.id ? editContent : templateConfig.content}
                  isEditing={selectedConfig?.id === templateConfig.id && isEditing}
                  onEdit={() => {
                    handleSelectConfig(templateConfig);
                    setIsEditing(true);
                  }}
                  onSave={handleSaveConfig}
                  onCancel={() => {
                    setIsEditing(false);
                    setEditContent(templateConfig.content);
                  }}
                  onChange={setEditContent}
                  onViewVersions={() => handleViewVersions(templateConfig)}
                />
              ) : (
                <div className="text-center py-8 text-muted-foreground">
                  <FileText className="h-12 w-12 mx-auto mb-4 opacity-50" />
                  <p>{t('config.noTemplate')}</p>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Nodes tab / 节点标签页 */}
        <TabsContent value="nodes" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                {t('config.nodeConfigs')}
                <Badge variant="secondary">{nodeConfigs.length}</Badge>
              </CardTitle>
              <CardDescription>
                {t('config.nodeConfigsDesc')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {loading ? (
                <div className="text-center py-8 text-muted-foreground">
                  <RefreshCw className="h-8 w-8 mx-auto animate-spin mb-4" />
                  <p>{t('common.loading')}</p>
                </div>
              ) : nodeConfigs.length === 0 ? (
                <div className="text-center py-8 text-muted-foreground">
                  <Server className="h-12 w-12 mx-auto mb-4 opacity-50" />
                  <p>{t('config.noNodeConfigs')}</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {nodeConfigs.map((config) => (
                    <Card key={config.id} className="border">
                      <CardHeader className="pb-2">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <Server className="h-4 w-4" />
                            <span className="font-medium">
                              {config.host_name || `Node ${config.host_id}`}
                            </span>
                            {config.host_ip && (
                              <span className="text-muted-foreground text-sm">
                                ({config.host_ip})
                              </span>
                            )}
                          </div>
                          <div className="flex items-center gap-2">
                            {config.match_template ? (
                              <Badge variant="outline" className="text-green-600">
                                <Check className="h-3 w-3 mr-1" />
                                {t('config.matchTemplate')}
                              </Badge>
                            ) : (
                              <Badge variant="outline" className="text-yellow-600">
                                <AlertTriangle className="h-3 w-3 mr-1" />
                                {t('config.customized')}
                              </Badge>
                            )}
                            <Badge variant="secondary">v{config.version}</Badge>
                          </div>
                        </div>
                      </CardHeader>
                      <CardContent>
                        <ConfigEditor
                          config={config}
                          content={selectedConfig?.id === config.id ? editContent : config.content}
                          isEditing={selectedConfig?.id === config.id && isEditing}
                          onEdit={() => {
                            handleSelectConfig(config);
                            setIsEditing(true);
                          }}
                          onSave={handleSaveConfig}
                          onCancel={() => {
                            setIsEditing(false);
                            setEditContent(config.content);
                          }}
                          onChange={setEditContent}
                          onViewVersions={() => handleViewVersions(config)}
                          onPromote={() => handlePromote(config)}
                          onSyncFromTemplate={() => handleSyncFromTemplate(config)}
                          showPromote={!config.match_template}
                          showSync={!config.match_template}
                        />
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Version history dialog / 版本历史对话框 */}
      {showVersions && selectedConfig && (
        <ConfigVersionHistory
          config={selectedConfig}
          versions={versions}
          onClose={() => setShowVersions(false)}
          onRollback={handleRollback}
        />
      )}
    </motion.div>
  );
}
