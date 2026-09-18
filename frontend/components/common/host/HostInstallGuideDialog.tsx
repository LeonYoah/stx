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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import {Terminal} from 'lucide-react';
import {HostInfo} from '@/lib/services/host/types';
import {HostInstallGuideContent} from './HostInstallGuideContent';

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

  if (!host) {return null;}

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-[680px] max-h-[90vh] overflow-y-auto p-5 sm:p-6'>
        <DialogHeader className='pb-3 border-b'>
          <div className='flex items-center gap-2 text-primary'>
            <Terminal className='h-5 w-5' />
            <DialogTitle className='text-base font-semibold'>
              {t('host.installGuide.title')}
            </DialogTitle>
          </div>
          <DialogDescription className='text-xs text-muted-foreground'>
            {t('host.installGuide.subtitle')}
          </DialogDescription>
        </DialogHeader>

        <div className='pt-2'>
          <HostInstallGuideContent
            host={host}
            onConnected={onConnected}
            onViewDetail={(targetHost) => {
              onOpenChange(false);
              onViewDetail?.(targetHost);
            }}
            onClose={() => onOpenChange(false)}
          />
        </div>
      </DialogContent>
    </Dialog>
  );
}
