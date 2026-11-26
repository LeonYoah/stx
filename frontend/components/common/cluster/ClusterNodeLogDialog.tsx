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

import {useCallback} from 'react';
import {useTranslations} from 'next-intl';
import {FileText} from 'lucide-react';
import {NeutralChip} from '@/components/common/layout';
import services from '@/lib/services';
import type {NodeInfo} from '@/lib/services/cluster/types';
import {LogViewerDialog, type LogFetchParams} from './LogViewerDialog';

interface ClusterNodeLogDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  node: NodeInfo | null;
  clusterId: number;
}

/**
 * 物理机节点日志弹窗：呈现层完全复用 LogViewerDialog，只注入 Agent 文件日志获取器。
 * Bare-metal node log dialog: the presentation layer is entirely the shared
 * LogViewerDialog; only the Agent file-log fetcher is injected here.
 */
export function ClusterNodeLogDialog({
  open,
  onOpenChange,
  node,
  clusterId,
}: ClusterNodeLogDialogProps) {
  const t = useTranslations();

  const fetcher = useCallback(
    async (params: LogFetchParams) => {
      if (!node) {
        return {error: t('cluster.getLogsError')};
      }
      const result = await services.cluster.getNodeLogsSafe(clusterId, node.id, {
        lines: params.lines,
        mode: params.mode,
        filter: params.filter,
        date: params.date,
      });
      if (result.success && result.data) {
        return {logs: result.data.logs || ''};
      }
      return {error: result.error || t('cluster.getLogsError')};
    },
    [clusterId, node, t],
  );

  const titleNode = (
    <span className='flex flex-wrap items-center gap-2'>
      <FileText className='size-4 shrink-0 text-primary' aria-hidden />
      <span>{t('cluster.viewLogs')}</span>
      {node ? (
        <NeutralChip className='font-mono tabular-nums'>
          {node.host_name || node.host_ip} (#{node.id})
        </NeutralChip>
      ) : null}
      {node?.role ? <NeutralChip>{node.role.toUpperCase()}</NeutralChip> : null}
    </span>
  );

  return (
    <LogViewerDialog
      open={open}
      onOpenChange={onOpenChange}
      title={titleNode}
      description={
        node?.install_dir ? (
          <span
            className='block truncate font-mono text-xs'
            title={`${node.install_dir}/logs`}
          >
            {node.install_dir}/logs
          </span>
        ) : undefined
      }
      fetcher={fetcher}
    />
  );
}
