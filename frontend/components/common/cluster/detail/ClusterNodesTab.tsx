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
 * 集群详情「节点」Tab：单行 Arc 工具栏 + TableCard + 0.5px 细线表行 +
 * 底部浮动 BatchBar（避让 Dock，替代原卡片头内联批量条）+ ProcessEventList。
 * Cluster detail Nodes tab: single-row Arc toolbar + TableCard + 0.5px row dividers +
 * floating BatchBar above Dock + ProcessEventList.
 */
import {useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  FileText,
  Pencil,
  Play,
  Plus,
  RotateCcw,
  Server,
  Square,
  Trash2,
} from 'lucide-react';
import {Alert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  BatchBar,
  NeutralChip,
  TableCard,
  TruncatedText,
} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  NodeJavaHomeCell,
  type NodeJavaRuntimeView,
} from '@/components/common/java/NodeJavaHomeCell';
import {
  NodeRole,
  NodeStatus,
  type NodeInfo,
  type ScaleOperationInfo,
} from '@/lib/services/cluster';
import {NodeStatusChip} from '../ClusterStatusChip';
import {ProcessEventList} from '../ProcessEventList';

export interface ClusterNodesTabProps {
  clusterId: number;
  nodes: NodeInfo[];
  refreshing: boolean;
  isOperating: boolean;
  nodeOperating: number | null;
  selectedNodeIds: Set<number>;
  nodeJavaRuntimes: Record<number, NodeJavaRuntimeView>;
  runningScaleOperation: ScaleOperationInfo | null;
  onToggleNodeSelection: (nodeId: number) => void;
  onToggleAllNodes: (nodes: NodeInfo[], checked: boolean) => void;
  onClearSelection: () => void;
  onAddNode: () => void;
  onOpenScaleProgress: () => void;
  onBatchOp: (op: 'start' | 'stop' | 'restart') => void;
  onNodeOp: (op: 'start' | 'stop' | 'restart', node: NodeInfo) => void;
  onViewLogs: (node: NodeInfo) => void;
  onEditNode: (node: NodeInfo) => void;
  onRemoveNode: (node: NodeInfo) => void;
}

function getRoleKey(role: string): string {
  return role === 'master/worker' ? 'masterWorker' : role;
}

export function ClusterNodesTab({
  clusterId,
  nodes,
  refreshing,
  isOperating,
  nodeOperating,
  selectedNodeIds,
  nodeJavaRuntimes,
  runningScaleOperation,
  onToggleNodeSelection,
  onToggleAllNodes,
  onClearSelection,
  onAddNode,
  onOpenScaleProgress,
  onBatchOp,
  onNodeOp,
  onViewLogs,
  onEditNode,
  onRemoveNode,
}: ClusterNodesTabProps) {
  const t = useTranslations();
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [roleFilter, setRoleFilter] = useState<string>('all');
  const [searchTerm, setSearchTerm] = useState('');

  const filteredNodes = useMemo(() => {
    const q = searchTerm.trim().toLowerCase();
    return nodes.filter((node) => {
      if (statusFilter !== 'all' && node.status !== statusFilter) {
        return false;
      }
      if (roleFilter === 'master') {
        if (
          node.role !== NodeRole.MASTER &&
          node.role !== NodeRole.MASTER_WORKER
        ) {
          return false;
        }
      } else if (roleFilter === 'worker') {
        if (
          node.role !== NodeRole.WORKER &&
          node.role !== NodeRole.MASTER_WORKER
        ) {
          return false;
        }
      }
      if (!q) {
        return true;
      }
      return (
        (node.host_name || '').toLowerCase().includes(q) ||
        (node.host_ip || '').toLowerCase().includes(q) ||
        (node.install_dir || '').toLowerCase().includes(q) ||
        String(node.id).includes(q)
      );
    });
  }, [nodes, roleFilter, searchTerm, statusFilter]);

  const hasFilters =
    statusFilter !== 'all' || roleFilter !== 'all' || Boolean(searchTerm.trim());

  const allSelected =
    filteredNodes.length > 0 &&
    filteredNodes.every((n) => selectedNodeIds.has(n.id));

  return (
    <div className='space-y-6' data-testid='cluster-nodes-panel'>
      {/* 节点单行工具栏 / Single-row nodes toolbar */}
      <div className={layout.toolbar}>
        <SegmentedControl
          label={t('cluster.list.statusFilterNodes')}
          value={statusFilter}
          onValueChange={setStatusFilter}
          options={[
            {value: 'all', label: t('cluster.allStatuses')},
            {value: NodeStatus.RUNNING, label: t('cluster.nodeStatuses.running')},
            {value: NodeStatus.OFFLINE, label: t('cluster.nodeStatuses.offline')},
            {value: NodeStatus.STOPPED, label: t('cluster.nodeStatuses.stopped')},
            {value: NodeStatus.ERROR, label: t('cluster.nodeStatuses.error')},
          ]}
        />

        <SegmentedControl
          label={t('cluster.list.roleFilterNodes')}
          value={roleFilter}
          onValueChange={setRoleFilter}
          options={[
            {value: 'all', label: t('cluster.list.allRoles')},
            {value: 'master', label: t('cluster.roles.master')},
            {value: 'worker', label: t('cluster.roles.worker')},
          ]}
        />

        <div className={layout.toolbarSearch}>
          <SearchField
            label={t('cluster.searchNodes')}
            value={searchTerm}
            onValueChange={setSearchTerm}
            placeholder={t('cluster.searchNodes')}
            clearLabel={t('cluster.list.searchClear')}
          />
        </div>

        <div className={layout.toolbarRight}>
          {hasFilters ? (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => {
                setStatusFilter('all');
                setRoleFilter('all');
                setSearchTerm('');
              }}
            >
              {t('common.clearFilters')}
            </Button>
          ) : null}
          <Button type='button' size='sm' onClick={onAddNode}>
            <Plus className='size-3.5' aria-hidden />
            {t('cluster.addNode')}
          </Button>
        </div>
      </div>

      {/* 扩缩容进行中提示条 / Running scale operation banner */}
      {runningScaleOperation ? (
        <Alert
          tone={runningScaleOperation.status === 'running' ? 'info' : 'danger'}
          title={
            runningScaleOperation.status === 'running'
              ? t('cluster.scale.inProgressBanner', {
                  kind: t(`cluster.scale.kind.${runningScaleOperation.kind}`),
                  step: t(
                    `cluster.scale.steps.${runningScaleOperation.current_step}`,
                  ),
                })
              : t('cluster.scale.operationFailed', {
                  error: runningScaleOperation.error || '-',
                })
          }
        >
          <div className='mt-2'>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              onClick={onOpenScaleProgress}
            >
              {t('cluster.scale.viewProgress')}
            </Button>
          </div>
        </Alert>
      ) : null}

      {/* 节点主表格卡片 / Main nodes TableCard */}
      <TableCard
        loading={refreshing}
        hasData={filteredNodes.length > 0}
        skeletonColumns={10}
        skeletonRows={5}
        empty={
          <EmptyState
            icon={<Server className='size-6' aria-hidden />}
            title={hasFilters ? t('common.noMatchingData') : t('cluster.noNodes')}
            description={
              hasFilters
                ? t('cluster.list.emptyFilteredDesc')
                : t('cluster.selectNodesToOperate')
            }
            action={
              hasFilters ? (
                <Button
                  type='button'
                  size='sm'
                  variant='secondary'
                  onClick={() => {
                    setStatusFilter('all');
                    setRoleFilter('all');
                    setSearchTerm('');
                  }}
                >
                  {t('common.clearFilters')}
                </Button>
              ) : (
                <Button type='button' size='sm' onClick={onAddNode}>
                  <Plus className='size-3.5' aria-hidden />
                  {t('cluster.addNode')}
                </Button>
              )
            }
          />
        }
      >
        <div className='overflow-x-auto'>
          <Table className='[&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
            <TableHeader>
              <TableRow>
                <TableHead className='w-10 pl-4'>
                  <Checkbox
                    checked={allSelected}
                    onCheckedChange={(v) =>
                      onToggleAllNodes(filteredNodes, v === true)
                    }
                    aria-label={t('cluster.nodeList')}
                  />
                </TableHead>
                <TableHead className='w-16'>ID</TableHead>
                <TableHead>{t('cluster.hostName')}</TableHead>
                <TableHead>{t('cluster.hostIP')}</TableHead>
                <TableHead>{t('cluster.nodeRole')}</TableHead>
                <TableHead>{t('cluster.installDir')}</TableHead>
                <TableHead>{t('javaHome.effectiveTitle')}</TableHead>
                <TableHead>{t('cluster.nodeStatus')}</TableHead>
                <TableHead>{t('cluster.processPID')}</TableHead>
                <TableHead className='pr-4 text-right'>
                  {t('cluster.actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredNodes.map((node) => {
                const roleKey = `cluster.roles.${getRoleKey(node.role)}`;
                const roleLabel = t.has(roleKey) ? t(roleKey) : node.role;
                const busy = nodeOperating === node.id;
                return (
                  <TableRow key={node.id} className='hover:bg-muted/25'>
                    <TableCell className='pl-4'>
                      <Checkbox
                        checked={selectedNodeIds.has(node.id)}
                        onCheckedChange={() => onToggleNodeSelection(node.id)}
                        aria-label={node.host_name || String(node.id)}
                      />
                    </TableCell>
                    <TableCell className='font-mono text-xs whitespace-nowrap text-muted-foreground tabular-nums'>
                      #{node.id}
                    </TableCell>
                    <TableCell className='font-medium whitespace-nowrap'>
                      {node.host_name || '-'}
                    </TableCell>
                    <TableCell className='font-mono text-xs whitespace-nowrap tabular-nums'>
                      {node.host_ip || '-'}
                    </TableCell>
                    <TableCell>
                      <NeutralChip>{roleLabel}</NeutralChip>
                    </TableCell>
                    <TableCell className='max-w-[220px] font-mono text-xs'>
                      <TruncatedText
                        text={node.install_dir}
                        mode='path'
                        mono
                        copyable
                      />
                    </TableCell>
                    <TableCell>
                      <NodeJavaHomeCell
                        configuredJavaHome={node.java_home}
                        runtime={nodeJavaRuntimes[node.id]}
                        data-testid={`node-java-home-${node.id}`}
                      />
                    </TableCell>
                    <TableCell>
                      <NodeStatusChip status={node.status} />
                    </TableCell>
                    <TableCell className='font-mono text-xs whitespace-nowrap tabular-nums'>
                      {node.process_pid || '-'}
                    </TableCell>
                    <TableCell className='pr-4 text-right'>
                      <div className='flex items-center justify-end gap-0.5'>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          disabled={
                            busy ||
                            node.status === NodeStatus.RUNNING ||
                            node.status === NodeStatus.OFFLINE
                          }
                          loading={busy}
                          aria-label={t('cluster.start')}
                          title={t('cluster.start')}
                          onClick={() => onNodeOp('start', node)}
                        >
                          <Play
                            className='size-3.5 text-emerald-600 dark:text-emerald-400'
                            aria-hidden
                          />
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          disabled={busy || node.status !== NodeStatus.RUNNING}
                          aria-label={t('cluster.stop')}
                          title={t('cluster.stop')}
                          onClick={() => onNodeOp('stop', node)}
                        >
                          <Square
                            className='size-3.5 text-amber-600 dark:text-amber-400'
                            aria-hidden
                          />
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          disabled={busy || node.status !== NodeStatus.RUNNING}
                          aria-label={t('cluster.restart')}
                          title={t('cluster.restart')}
                          onClick={() => onNodeOp('restart', node)}
                        >
                          <RotateCcw
                            className='size-3.5 text-blue-600 dark:text-blue-400'
                            aria-hidden
                          />
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          aria-label={t('cluster.viewLogs')}
                          title={t('cluster.viewLogs')}
                          onClick={() => onViewLogs(node)}
                        >
                          <FileText className='size-3.5' aria-hidden />
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          aria-label={t('cluster.editNode')}
                          title={t('cluster.editNode')}
                          onClick={() => onEditNode(node)}
                        >
                          <Pencil className='size-3.5' aria-hidden />
                        </Button>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          iconOnly
                          aria-label={t('cluster.removeNode')}
                          title={t('cluster.removeNode')}
                          onClick={() => onRemoveNode(node)}
                        >
                          <Trash2
                            className='size-3.5 text-destructive'
                            aria-hidden
                          />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      </TableCard>

      {/* 底部浮动批量操作条，避让 Dock（ui-conventions §5） */}
      {/* Floating bottom batch bar above Dock (ui-conventions §5) */}
      {selectedNodeIds.size > 0 ? (
        <div
          className='fixed bottom-24 left-1/2 z-30 -translate-x-1/2'
          data-testid='cluster-nodes-batch-bar'
        >
          <BatchBar
            selectedCount={selectedNodeIds.size}
            busy={isOperating}
            onClear={onClearSelection}
          >
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={isOperating}
              onClick={() => onBatchOp('start')}
            >
              <Play className='size-3.5' aria-hidden />
              {t('cluster.start')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={isOperating}
              onClick={() => onBatchOp('stop')}
            >
              <Square className='size-3.5' aria-hidden />
              {t('cluster.stop')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              disabled={isOperating}
              onClick={() => onBatchOp('restart')}
            >
              <RotateCcw className='size-3.5' aria-hidden />
              {t('cluster.restart')}
            </Button>
          </BatchBar>
        </div>
      ) : null}

      {/* 进程事件列表 / Process lifecycle events */}
      <ProcessEventList clusterId={clusterId} />
    </div>
  );
}
