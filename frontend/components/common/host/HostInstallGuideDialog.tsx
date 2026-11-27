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
 * Host Install Guide Dialog Component
 * 主机 Agent 部署与接入引导弹窗
 *
 * Standalone modal dialog that wraps HostInstallGuideContent for easy reuse
 * across table actions and host details.
 * 独立的弹窗组件，供主机表格、详情等入口随时调起 Agent 安装命令与心跳侦听引导。
 */

import {useTranslations} from 'next-intl';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {HostInfo, HostType} from '@/lib/services/host/types';
import {HostInstallGuideContent} from './HostInstallGuideContent';
import {K8sConnectGuideContent} from './K8sConnectGuideContent';

interface HostInstallGuideDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  host: HostInfo | null;
  onConnected?: (updatedHost: HostInfo) => void;
  onViewDetail?: (host: HostInfo) => void;
}

export function HostInstallGuideDialog({
  open,
  onOpenChange,
  host,
  onConnected,
  onViewDetail,
}: HostInstallGuideDialogProps) {
  const t = useTranslations();

  if (!host) {
    return null;
  }

  const isK8s = host.host_type === HostType.KUBERNETES;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        title={isK8s ? t('host.k8sGuide.title') : t('host.installGuide.title')}
        description={
          isK8s ? t('host.k8sGuide.subtitle') : t('host.installGuide.subtitle')
        }
        className='arc-compact w-[min(calc(100vw-2rem),680px)]!'
        bodyClassName='max-h-[min(85dvh,720px)] overflow-y-auto'
      >
        {isK8s ? (
          <K8sConnectGuideContent
            host={host}
            onConnected={onConnected}
            onViewDetail={(targetHost) => {
              onOpenChange(false);
              onViewDetail?.(targetHost);
            }}
            onClose={() => onOpenChange(false)}
          />
        ) : (
          <HostInstallGuideContent
            host={host}
            onConnected={onConnected}
            onViewDetail={(targetHost) => {
              onOpenChange(false);
              onViewDetail?.(targetHost);
            }}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
