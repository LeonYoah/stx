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

import {useTranslations} from 'next-intl';
import {ExternalLink} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {cn} from '@/lib/utils';
import {parseConnectorMissingError} from './sync-studio-utils';

/**
 * 已存失败消息里的「缺连接器」归类提示：只按消息文本识别（引擎原文保留在 error_msg 里），
 * 不依赖接口下发的 error_code，因此历史作业同样生效。
 * Connector-missing hint for stored failure messages: recognized from the message text alone
 * (the engine wording is preserved in error_msg), so it works for historical jobs too without
 * relying on a response error_code.
 */
export function ConnectorMissingHint({
  message,
  preview,
  className,
}: {
  message: string;
  // preview 时缺 Http 是预览自己注入的 sink，说明文案要解释这一点（同 formatSyncUserFacingError）。
  // With preview, a missing Http is the sink preview injected itself, so the copy explains that
  // (same rule as formatSyncUserFacingError).
  preview?: boolean;
  className?: string;
}) {
  const t = useTranslations('workbenchStudio');
  const missing = parseConnectorMissingError(message);
  if (!missing) {
    return null;
  }
  const previewSink =
    Boolean(preview) &&
    missing.pluginType === 'sink' &&
    missing.identifier.toLowerCase() === 'http';
  const identifier = missing.identifier || '-';
  const keyword = (missing.identifier || 'http').toLowerCase();
  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-x-2 gap-y-1 text-xs',
        className,
      )}
    >
      <Badge
        variant='outline'
        className='border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400'
      >
        {t('connectorMissingBadge')}
      </Badge>
      <span className='text-muted-foreground'>
        {previewSink
          ? t('connectorMissingPreviewDescription')
          : t('connectorMissingDescription', {
              type: t(`connectorType.${missing.pluginType}`),
              identifier,
            })}
      </span>
      <a
        href={`/plugins?q=${encodeURIComponent(keyword)}`}
        target='_blank'
        rel='noopener noreferrer'
        className='inline-flex items-center gap-1 font-medium text-primary hover:underline'
      >
        {t('connectorMissingAction')}
        <ExternalLink className='size-3' />
      </a>
    </div>
  );
}
