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

import {
  useState,
  useRef,
  useEffect,
  type MouseEvent,
  type DragEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import {useTranslations} from 'next-intl';
import {
  Braces,
  Code2,
  Database,
  FileCode2,
  FilePlus2,
  FileText,
  FolderPlus,
  Lock,
  SquareTerminal,
} from 'lucide-react';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {
  FileTree,
  FileTreeFile,
  FileTreeFolder,
} from '@/components/motion/file-tree';
import {cn} from '@/lib/utils';
import type {SyncTaskTreeNode} from '@/lib/services/sync';
import {findTreeNode} from './sync-studio-utils';

/** 行内新建占位节点 id，避免与真实任务 id 冲突。 / Synthetic id for the inline-create row. */
const CREATE_ROW_VALUE = '__creating__';

/**
 * 窄目录栏里文件名截断时，悬停展示全称。
 * Show the full truncated explorer name on hover.
 */
function TreeNodeName({name}: {name: string}) {
  const labelRef = useRef<HTMLSpanElement>(null);
  const [truncated, setTruncated] = useState(false);

  useEffect(() => {
    const node = labelRef.current;
    if (!node || typeof ResizeObserver === 'undefined') {
      return;
    }
    const update = () => {
      setTruncated(node.scrollWidth > node.clientWidth + 1);
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(node);
    return () => observer.disconnect();
  }, [name]);

  return (
    <Tooltip delayDuration={400}>
      <TooltipTrigger asChild>
        <span
          ref={labelRef}
          className='block w-full min-w-0 truncate font-mono text-xs'
        >
          {name}
        </span>
      </TooltipTrigger>
      {truncated ? (
        <TooltipContent side='right' className='max-w-xs break-all font-mono text-xs'>
          {name}
        </TooltipContent>
      ) : null}
    </Tooltip>
  );
}

/**
 * 根据文件名扩展名返回对应的 VSCode 风格图标
 * Return VSCode-style icon based on file extension
 */
function getFileIcon(name: string) {
  const lower = name.toLowerCase();
  if (lower.endsWith('.conf') || lower.endsWith('.hocon')) {
    return <FileCode2 className='size-4 shrink-0 text-sky-500' />;
  }
  if (lower.endsWith('.json')) {
    return <Braces className='size-4 shrink-0 text-amber-500' />;
  }
  if (lower.endsWith('.sql')) {
    return <Database className='size-4 shrink-0 text-violet-500' />;
  }
  if (lower.endsWith('.yaml') || lower.endsWith('.yml')) {
    return <FileText className='size-4 shrink-0 text-emerald-500' />;
  }
  if (lower.endsWith('.xml')) {
    return <Code2 className='size-4 shrink-0 text-orange-500' />;
  }
  if (lower.endsWith('.sh') || lower.endsWith('.bash')) {
    return <SquareTerminal className='size-4 shrink-0 text-green-500' />;
  }
  if (lower.endsWith('.py')) {
    return <FileCode2 className='size-4 shrink-0 text-yellow-500' />;
  }
  if (lower.endsWith('.properties') || lower.endsWith('.env')) {
    return <FileText className='size-4 shrink-0 text-slate-400' />;
  }
  return <FileCode2 className='size-4 shrink-0 text-sky-500/80' />;
}

export interface TreeViewProps {
  nodes: SyncTaskTreeNode[];
  selectedNodeId: number | null;
  selectedFolderId: number | null;
  expandedFolderIds: number[];
  onSelect: (node: SyncTaskTreeNode) => void;
  onContextMenu: (
    event: MouseEvent,
    kind: 'folder' | 'file',
    node: SyncTaskTreeNode,
  ) => void;
  dirtyNodeIds?: number[];

  // 展开状态由 beUI FileTree 回报，避免与 onSelect 重复折叠。
  // Expansion is owned by beUI FileTree so onSelect does not double-toggle.
  onExpandedChange?: (expandedIds: number[]) => void;

  // 行内重命名状态与回调
  // Inline rename state and callbacks
  renamingNodeId?: number | null;
  onRenameStart?: (node: SyncTaskTreeNode) => void;
  onRenameCommit?: (node: SyncTaskTreeNode, newName: string) => void | Promise<void>;
  onRenameCancel?: () => void;

  // 行内新建输入状态与回调
  // Inline creation state and callbacks
  creatingNode?: {parentId: number | null; nodeType: 'file' | 'folder'} | null;
  onCreateCommit?: (name: string) => void | Promise<void>;
  onCreateCancel?: () => void;

  // 拖拽移动状态与回调
  // Drag-and-drop state and callbacks
  draggingNodeId?: number | null;
  dragOverFolderId?: number | null;
  onDragStart?: (node: SyncTaskTreeNode) => void;
  onDragOver?: (targetFolder: SyncTaskTreeNode) => void;
  onDragLeave?: (targetFolder: SyncTaskTreeNode) => void;
  onDrop?: (targetFolder: SyncTaskTreeNode) => void;
  onDragEnd?: () => void;
}

/**
 * 工作台资源树：用 beUI File Tree 做展开、选中高亮和键盘导航，
 * 右键、行内重命名/新建、拖拽移动仍走原有回调。
 * Workbench explorer: beUI File Tree handles expand, selection, and keyboard
 * navigation; context menu, inline rename/create, and drag-move stay custom.
 */
export function TreeView({
  nodes,
  selectedNodeId,
  selectedFolderId,
  expandedFolderIds,
  onSelect,
  onContextMenu,
  dirtyNodeIds = [],
  onExpandedChange,
  renamingNodeId = null,
  onRenameStart,
  onRenameCommit,
  onRenameCancel,
  creatingNode = null,
  onCreateCommit,
  onCreateCancel,
  draggingNodeId = null,
  dragOverFolderId = null,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
  onDragEnd,
}: TreeViewProps) {
  const t = useTranslations('workbenchStudio');
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);
  const createInputRef = useRef<HTMLInputElement>(null);

  // 扁平树后必须在整棵树上查找重命名目标，不能只看当前层。
  // After flattening, look up the rename target in the whole tree.
  useEffect(() => {
    if (renamingNodeId === null) return;
    const target = findTreeNode(nodes, renamingNodeId);
    if (!target) return;
    setRenameValue(target.name);
    setTimeout(() => {
      if (!renameInputRef.current) return;
      renameInputRef.current.focus();
      const dotIdx = target.name.lastIndexOf('.');
      if (dotIdx > 0 && target.node_type === 'file') {
        renameInputRef.current.setSelectionRange(0, dotIdx);
      } else {
        renameInputRef.current.select();
      }
    }, 30);
  }, [renamingNodeId, nodes]);

  // 当触发行内新建时聚焦输入框
  // Focus input when triggering inline creation
  useEffect(() => {
    if (creatingNode) {
      setTimeout(() => {
        createInputRef.current?.focus();
      }, 30);
    }
  }, [creatingNode]);

  const handleRenameKeyDown = (
    e: KeyboardEvent<HTMLInputElement>,
    node: SyncTaskTreeNode,
  ) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      const trimmed = renameValue.trim();
      if (trimmed && trimmed !== node.name) {
        onRenameCommit?.(node, trimmed);
      } else {
        onRenameCancel?.();
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      onRenameCancel?.();
    }
  };

  const handleCreateKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      const value = e.currentTarget.value.trim();
      if (value) {
        onCreateCommit?.(value);
      } else {
        onCreateCancel?.();
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      onCreateCancel?.();
    }
  };

  const handleValueChange = (value: string) => {
    if (value === CREATE_ROW_VALUE) return;
    const id = Number(value);
    if (!Number.isFinite(id)) return;
    const node = findTreeNode(nodes, id);
    if (node) onSelect(node);
  };

  const handleExpandedChange = (ids: string[]) => {
    onExpandedChange?.(
      ids
        .map((id) => Number(id))
        .filter((id) => Number.isFinite(id) && id > 0),
    );
  };

  const selectedValue =
    selectedNodeId != null
      ? String(selectedNodeId)
      : selectedFolderId != null
        ? String(selectedFolderId)
        : null;

  const renderCreateRow = (): ReactNode => {
    if (!creatingNode) return null;
    return (
      <FileTreeFile
        value={CREATE_ROW_VALUE}
        name=''
        selectable={false}
        draggable={false}
        className='border border-primary/40 bg-background/95'
        icon={
          creatingNode.nodeType === 'folder' ? (
            <FolderPlus className='size-4 text-amber-500/90' />
          ) : (
            <FilePlus2 className='size-4 text-sky-500/90' />
          )
        }
        label={
          <input
            ref={createInputRef}
            type='text'
            className='h-7 min-w-0 w-full rounded bg-transparent px-1 font-mono text-xs text-foreground outline-none ring-1 ring-primary/50'
            placeholder={
              creatingNode.nodeType === 'folder' ? t('newFolder') : t('newFile')
            }
            onClick={(e) => e.stopPropagation()}
            onKeyDown={handleCreateKeyDown}
            onBlur={(e) => {
              const value = e.currentTarget.value.trim();
              if (value) onCreateCommit?.(value);
              else onCreateCancel?.();
            }}
          />
        }
      />
    );
  };

  const renderTrailing = (node: SyncTaskTreeNode): ReactNode => {
    const isDirty = dirtyNodeIds.includes(node.id);

    return (
      <>
        {node.node_type === 'file' && isDirty ? (
          <span
            className='size-1.5 shrink-0 rounded-full bg-amber-500'
            title={t('unsavedChanges')}
          />
        ) : null}

        {node.node_type === 'file' && node.can_edit === false ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Lock className='size-3.5 text-amber-500 shrink-0' />
            </TooltipTrigger>
            <TooltipContent side='right'>{t('readOnlyLocked')}</TooltipContent>
          </Tooltip>
        ) : null}

        {node.node_type === 'file' ? (
          <span
            className={cn(
              'inline-flex items-center rounded border px-1 py-0 font-mono text-[10px] font-normal leading-4',
              node.current_version > 0
                ? 'border-border/60 bg-muted/40 text-muted-foreground'
                : 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
            )}
          >
            {node.current_version > 0 ? `v${node.current_version}` : t('draft')}
          </span>
        ) : null}
      </>
    );
  };

  const nodeRowClassName = (node: SyncTaskTreeNode) =>
    cn(
      draggingNodeId === node.id && 'opacity-35',
      node.node_type === 'folder' &&
        dragOverFolderId === node.id &&
        'bg-primary/15 ring-1 ring-primary/40',
    );

  const nodeDragHandlers = (node: SyncTaskTreeNode) => {
    const isRenaming = renamingNodeId === node.id;
    return {
      draggable: !isRenaming,
      onDragStart: (e: DragEvent<HTMLElement>) => {
        if (isRenaming) {
          e.preventDefault();
          return;
        }
        e.dataTransfer.setData('text/plain', String(node.id));
        e.dataTransfer.effectAllowed = 'move';
        onDragStart?.(node);
      },
      onDragOver: (e: DragEvent<HTMLElement>) => {
        if (node.node_type === 'folder') {
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
          onDragOver?.(node);
        }
      },
      onDragLeave: () => {
        if (node.node_type === 'folder') onDragLeave?.(node);
      },
      onDrop: (e: DragEvent<HTMLElement>) => {
        e.preventDefault();
        if (node.node_type === 'folder') onDrop?.(node);
      },
      onDragEnd: () => {
        onDragEnd?.();
      },
    };
  };

  const renderRenameLabel = (node: SyncTaskTreeNode) => (
    <input
      ref={renameInputRef}
      type='text'
      value={renameValue}
      onChange={(e) => setRenameValue(e.target.value)}
      onKeyDown={(e) => handleRenameKeyDown(e, node)}
      onBlur={() => {
        const trimmed = renameValue.trim();
        if (trimmed && trimmed !== node.name) {
          onRenameCommit?.(node, trimmed);
        } else {
          onRenameCancel?.();
        }
      }}
      onClick={(e) => e.stopPropagation()}
      className='h-7 min-w-0 w-full rounded bg-background px-1 font-mono text-xs text-foreground outline-none ring-1 ring-primary'
    />
  );

  const renderNodes = (treeNodes: SyncTaskTreeNode[]): ReactNode =>
    treeNodes.map((node) => {
      const isRenaming = renamingNodeId === node.id;
      const shared = {
        value: String(node.id),
        name: node.name,
        className: nodeRowClassName(node),
        trailing: renderTrailing(node),
        label: isRenaming ? (
          renderRenameLabel(node)
        ) : (
          <TreeNodeName name={node.name} />
        ),
        onContextMenu: (event: MouseEvent<HTMLElement>) =>
          onContextMenu(event, node.node_type, node),
        onDoubleClick: (event: MouseEvent<HTMLElement>) => {
          event.stopPropagation();
          if (node.can_edit !== false) onRenameStart?.(node);
        },
        ...nodeDragHandlers(node),
      };

      if (node.node_type === 'folder') {
        return (
          <FileTreeFolder key={node.id} {...shared}>
            {creatingNode?.parentId === node.id ? renderCreateRow() : null}
            {node.children?.length ? renderNodes(node.children) : null}
          </FileTreeFolder>
        );
      }

      return (
        <FileTreeFile
          key={node.id}
          {...shared}
          icon={getFileIcon(node.name)}
        />
      );
    });

  return (
    <FileTree
      value={selectedValue}
      onValueChange={handleValueChange}
      expandedIds={expandedFolderIds.map(String)}
      onExpandedChange={handleExpandedChange}
      ariaLabel={t('resources')}
      labels={{label: t('resources')}}
      indent={14}
      className='select-none'
    >
      {creatingNode?.parentId === null ? renderCreateRow() : null}
      {renderNodes(nodes)}
    </FileTree>
  );
}
