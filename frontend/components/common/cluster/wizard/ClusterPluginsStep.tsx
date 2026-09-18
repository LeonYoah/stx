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
 * Cluster Plugins Selection Step Component
 * 集群插件挑选步骤组件
 *
 * Streamlined plugin selector integrating PluginSelectStep without redundant notices.
 * 紧凑的插件选择步骤，集成 PluginSelectStep，杜绝多处重复唠叨的免责说明。
 */

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {PluginSelectStep} from '@/components/common/installer/PluginSelectStep';
import {ClusterDeployConfig} from './types';

interface ClusterPluginsStepProps {
  /** Cluster deploy config / 集群部署配置 */
  config: ClusterDeployConfig;
  /** Update config callback / 更新配置回调 */
  updateConfig: (updates: Partial<ClusterDeployConfig>) => void;
}

export function ClusterPluginsStep({
  config,
  updateConfig,
}: ClusterPluginsStepProps) {
  const t = useTranslations();

  return (
    <div className='h-full flex flex-col overflow-hidden'>
      {/* Top Header / 顶部摘要栏 */}
      <div className='flex items-center justify-between pb-3 border-b mb-3'>
        <div className='flex items-center gap-2'>
          <span className='text-xs font-semibold uppercase tracking-wider text-muted-foreground'>
            {t('cluster.wizard.pluginsLibrary')}
          </span>
          <Badge
            variant='outline'
            className='text-[10px] h-4 text-muted-foreground'
          >
            {t('cluster.wizard.optionalConfig')}
          </Badge>
        </div>

        <Badge
          variant={config.selectedPlugins.length > 0 ? 'default' : 'outline'}
          className='text-xs h-5 px-2 font-mono'
        >
          {t('cluster.wizard.pluginsSelectedCount', {
            count: config.selectedPlugins.length,
          })}
        </Badge>
      </div>

      {/* Plugin Selector Container / 插件选择器容器 */}
      <div className='flex-1 min-h-0 overflow-hidden'>
        <PluginSelectStep
          version={config.version}
          mirror={config.mirror}
          onMirrorChange={(mirror) => updateConfig({mirror})}
          selectedPlugins={config.selectedPlugins}
          selectedPluginProfiles={config.selectedPluginProfiles}
          onPluginsChange={(plugins) => {
            const selectedPluginSet = new Set(plugins);
            const nextProfiles = Object.fromEntries(
              Object.entries(config.selectedPluginProfiles).filter(
                ([pluginName]) => selectedPluginSet.has(pluginName),
              ),
            );
            updateConfig({
              selectedPlugins: plugins,
              selectedPluginProfiles: nextProfiles,
            });
          }}
          onPluginProfilesChange={(pluginName, profileKeys) =>
            updateConfig({
              selectedPluginProfiles: {
                ...config.selectedPluginProfiles,
                [pluginName]: profileKeys,
              },
            })
          }
        />
      </div>
    </div>
  );
}

export default ClusterPluginsStep;
