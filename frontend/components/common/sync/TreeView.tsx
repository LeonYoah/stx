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
} from 'react';
import {useTranslations} from 'next-intl';
import {
  Braces,
  ChevronRight,
  Code2,
  Database,
  FileCode2,
  FilePlus2,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  Lock,
  Pencil,
  SquareTerminal,
  Trash2,
} from 'lucide-react';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {cn} from '@/lib/utils';
import type {SyncTaskTreeNode} from '@/lib/services/sync';

/**
 * 根据文件名扩展名返回对应的 VSCode 风格图标
 * Return VSCode-style icon based on file extension
 */
function getFileIcon(name: string) {
  const lower = name.toLowerCase();
  if (lower.endsWith('.conf') || lower.endsWith('.hocon')) {
    return <FileCode2 className='size-3.5 shrink-0 text-sky-500/90' />;
  }
  if (lower.endsWith('.json')) {
    return <Braces className='size-3.5 shrink-0 text-amber-500/90' />;
  }
  if (lower.endsWith('.sql')) {
    return <Database className='size-3.5 shrink-0 text-violet-500/90' />;
  }
  if (lower.endsWith('.yaml') || lower.endsWith('.yml')) {
    return <FileText className='size-3.5 shrink-0 text-emerald-500/90' />;
  }
  if (lower.endsWith('.xml')) {
    return <Code2 className='size-3.5 shrink-0 text-orange-500/90' />;
  }
  if (lower.endsWith('.sh') || lower.endsWith('.bash')) {
    return <SquareTerminal className='size-3.5 shrink-0 text-green-500/90' />;
  }
  if (lower.endsWith('.py')) {
    return <FileCode2 className='size-3.5 shrink-0 text-yellow-500/90' />;
  }
  if (lower.endsWith('.properties') || lower.endsWith('.env')) {
    return <FileText className='size-3.5 shrink-0 text-slate-400' />;
  }
  return <FileCode2 className='size-3.5 shrink-0 text-sky-500/80' />;
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
  depth?: number;

  // 行内重命名状态与回调
  // Inline rename state and callbacks
  renamingNodeId?: number | null;
  onRenameStart?: (node: SyncTaskTreeNode) => void;
  onRenameCommit?: (node: SyncTaskTreeNode, newName: string) => void | Promise<void>;
  onRenameCancel?: () => void;

  // 悬停快捷操作回调
  // Hover quick action callbacks
  onCreateFile?: (parentNode: SyncTaskTreeNode) => void;
  onCreateFolder?: (parentNode: SyncTaskTreeNode) => void;
  onDelete?: (node: SyncTaskTreeNode) => void;

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
 * 递归文件目录树组件（VSCode 风格丝滑交互）
 * Recursive File Directory Tree Component (VSCode-like smooth interaction)
 */
export function TreeView({
  nodes,
  selectedNodeId,
  selectedFolderId,
  expandedFolderIds,
  onSelect,
  onContextMenu,
  dirtyNodeIds = [],
  depth = 0,
  renamingNodeId = null,
  onRenameStart,
  onRenameCommit,
  onRenameCancel,
  onCreateFile,
  onCreateFolder,
  onDelete,
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

  // 当进入重命名模式时，初始化输入框并聚焦选中文件名
  // When entering rename mode, initialize input and select text
  useEffect(() => {
    if (renamingNodeId !== null) {
      const target = nodes.find((n) => n.id === renamingNodeId);
      if (target) {
        setRenameValue(target.name);
        setTimeout(() => {
          if (renameInputRef.current) {
            renameInputRef.current.focus();
            const dotIdx = target.name.lastIndexOf('.');
            if (dotIdx > 0 && target.node_type === 'file') {
              renameInputRef.current.setSelectionRange(0, dotIdx);
            } else {
              renameInputRef.current.select();
            }
          }
        }, 30);
      }
    }
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

  return (
    <div className='space-y-0.5 py-0.5 select-none'>
      {/* 根级行内创建输入行 */}
      {/* Root-level inline creation row */}
      {depth === 0 && creatingNode?.parentId === null ? (
        <div
          className='flex items-center gap-1.5 rounded-sm border border-primary/40 bg-background/95 px-2 py-1 shadow-2xs'
          style={{paddingLeft: '6px'}}
        >
          <span className='flex size-3.5 shrink-0 items-center justify-center'>
            {creatingNode.nodeType === 'folder' ? (
              <FolderPlus className='size-3.5 text-amber-500/90' />
            ) : (
              <FilePlus2 className='size-3.5 text-sky-500/90' />
            )}
          </span>
          <input
            ref={createInputRef}
            type='text'
            className='h-5 min-w-0 flex-1 rounded bg-transparent px-1 font-mono text-xs text-foreground outline-none ring-1 ring-primary/50'
            placeholder={
              creatingNode.nodeType === 'folder'
                ? t('newFolder')
                : t('newFile')
            }
            onKeyDown={handleCreateKeyDown}
            onBlur={(e) => {
              const value = e.currentTarget.value.trim();
              if (value) onCreateCommit?.(value);
              else onCreateCancel?.();
            }}
          />
        </div>
      ) : null}

      {nodes.map((node) => {
        const selected =
          node.id === selectedNodeId || node.id === selectedFolderId;
        const isExpanded = expandedFolderIds.includes(node.id);
        const hasChildren = Boolean(node.children && node.children.length > 0);
        const isDirty = dirtyNodeIds.includes(node.id);
        const isRenaming = renamingNodeId === node.id;
        const isDragging = draggingNodeId === node.id;
        const isDragOver =
          node.node_type === 'folder' && dragOverFolderId === node.id;

        return (
          <div key={node.id} className='relative'>
            {/* 纵深缩进参考线（层级大于0时显示） */}
            {/* Hierarchical indent guide line (displayed when depth > 0) */}
            {depth > 0 ? (
              <div
                className='pointer-events-none absolute bottom-0 top-0 border-l border-border/30 transition-colors group-hover:border-border/60'
                style={{left: `${depth * 14 - 3}px`}}
              />
            ) : null}

            <div
              draggable={!isRenaming}
              onDragStart={(e) => {
                if (isRenaming) {
                  e.preventDefault();
                  return;
                }
                e.dataTransfer.setData('text/plain', String(node.id));
                e.dataTransfer.effectAllowed = 'move';
                onDragStart?.(node);
              }}
              onDragOver={(e) => {
                if (node.node_type === 'folder') {
                  e.preventDefault();
                  e.dataTransfer.dropEffect = 'move';
                  onDragOver?.(node);
                }
              }}
              onDragLeave={() => {
                if (node.node_type === 'folder') {
                  onDragLeave?.(node);
                }
              }}
              onDrop={(e) => {
                e.preventDefault();
                if (node.node_type === 'folder') {
                  onDrop?.(node);
                }
              }}
              onDragEnd={() => {
                onDragEnd?.();
              }}
              className={cn(
                'group flex w-full items-center justify-between rounded-sm border border-transparent px-2 py-1 text-left text-xs transition-colors duration-150 cursor-pointer',
                selected
                  ? 'border-primary/30 bg-primary/10 font-medium text-primary shadow-2xs'
                  : 'text-foreground/80 hover:bg-muted/60 hover:text-foreground',
                isDragging && 'opacity-35',
                isDragOver &&
                  'border-primary/70 bg-primary/15 ring-1 ring-primary/40 shadow-xs',
              )}
              style={{paddingLeft: `${depth * 14 + 6}px`}}
              onClick={() => onSelect(node)}
              onDoubleClick={(e) => {
                e.stopPropagation();
                if (node.can_edit !== false) {
                  onRenameStart?.(node);
                }
              }}
              onContextMenu={(event) =>
                onContextMenu(event, node.node_type, node)
              }
            >
              <span className='flex min-w-0 flex-1 items-center gap-1.5'>
                {node.node_type === 'folder' ? (
                  <>
                    <span
                      className='flex size-3.5 shrink-0 items-center justify-center'
                      onClick={(e) => {
                        e.stopPropagation();
                        onSelect(node);
                      }}
                    >
                      {hasChildren ? (
                        <ChevronRight
                          className={cn(
                            'size-3 shrink-0 text-muted-foreground/70 transition-transform duration-200',
                            isExpanded && 'rotate-90 text-foreground',
                          )}
                        />
                      ) : null}
                    </span>
                    {isExpanded ? (
                      <FolderOpen className='size-3.5 shrink-0 text-amber-500/90' />
                    ) : (
                      <Folder className='size-3.5 shrink-0 text-amber-500/80' />
                    )}
                  </>
                ) : (
                  <>
                    <span className='inline-block size-3.5 shrink-0' />
                    {getFileIcon(node.name)}
                  </>
                )}

                {/* 节点名称或行内重命名输入框 */}
                {/* Node name or inline rename input */}
                {isRenaming ? (
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
                    className='h-5 min-w-0 flex-1 rounded bg-background px-1 font-mono text-xs text-foreground outline-none ring-1 ring-primary'
                  />
                ) : (
                  <span className='truncate'>{node.name}</span>
                )}
              </span>

              {/* 右侧指示器与 VSCode 风格悬停快捷操作按钮组 */}
              {/* Right indicators and VSCode-style hover quick actions */}
              <div className='ml-1.5 flex shrink-0 items-center gap-1'>
                {/* 悬停快捷按钮（仅非重命名模式下展示） */}
                {/* Hover action buttons (only shown when not renaming) */}
                {!isRenaming ? (
                  <div className='hidden group-hover:flex items-center gap-0.5 pr-0.5 animate-in fade-in-50 duration-100'>
                    {node.node_type === 'folder' ? (
                      <>
                        <button
                          type='button'
                          title={t('newFile')}
                          aria-label={t('newFile')}
                          className='rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground'
                          onClick={(e) => {
                            e.stopPropagation();
                            onCreateFile?.(node);
                          }}
                        >
                          <FilePlus2 className='size-3' />
                        </button>
                        <button
                          type='button'
                          title={t('newFolder')}
                          aria-label={t('newFolder')}
                          className='rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground'
                          onClick={(e) => {
                            e.stopPropagation();
                            onCreateFolder?.(node);
                          }}
                        >
                          <FolderPlus className='size-3' />
                        </button>
                      </>
                    ) : null}
                    {node.can_edit !== false ? (
                      <button
                        type='button'
                        title={t('rename')}
                        aria-label={t('rename')}
                        className='rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground'
                        onClick={(e) => {
                          e.stopPropagation();
                          onRenameStart?.(node);
                        }}
                      >
                        <Pencil className='size-3' />
                      </button>
                    ) : null}
                    <button
                      type='button'
                      title={t('delete')}
                      aria-label={t('delete')}
                      className='rounded p-0.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive'
                      onClick={(e) => {
                        e.stopPropagation();
                        onDelete?.(node);
                      }}
                    >
                      <Trash2 className='size-3' />
                    </button>
                  </div>
                ) : null}

                {/* 未保存修改指示圆点 */}
                {/* Unsaved changes indicator dot */}
                {node.node_type === 'file' && isDirty ? (
                  <span
                    className='size-1.5 shrink-0 rounded-full bg-amber-500'
                    title={t('unsavedChanges')}
                  />
                ) : null}

                {/* 权限锁定标识 */}
                {/* Permission lock indicator */}
                {node.node_type === 'file' && node.can_edit === false ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Lock className='size-3 text-amber-500 shrink-0' />
                    </TooltipTrigger>
                    <TooltipContent side='right'>{t('readOnlyLocked')}</TooltipContent>
                  </Tooltip>
                ) : null}

                {/* 微型版本指示胶囊 */}
                {/* Micro version badge pill */}
                {node.node_type === 'file' ? (
                  <span
                    className={cn(
                      'inline-flex items-center rounded border px-1 py-0 font-mono text-[9px] font-normal leading-4',
                      node.current_version > 0
                        ? 'border-border/60 bg-muted/40 text-muted-foreground'
                        : 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
                    )}
                  >
                    {node.current_version > 0
                      ? `v${node.current_version}`
                      : t('draft')}
                  </span>
                ) : null}
              </div>
            </div>

            {/* 子节点与行内创建 */}
            {/* Child nodes and inline creation */}
            {node.node_type === 'folder' && isExpanded ? (
              <div className='overflow-hidden transition-all duration-150'>
                {/* 文件夹下行内创建输入行 */}
                {/* Subfolder inline creation input row */}
                {creatingNode?.parentId === node.id ? (
                  <div
                    className='flex items-center gap-1.5 rounded-sm border border-primary/40 bg-background/95 px-2 py-1 shadow-2xs my-0.5'
                    style={{paddingLeft: `${(depth + 1) * 14 + 6}px`}}
                  >
                    <span className='flex size-3.5 shrink-0 items-center justify-center'>
                      {creatingNode.nodeType === 'folder' ? (
                        <FolderPlus className='size-3.5 text-amber-500/90' />
                      ) : (
                        <FilePlus2 className='size-3.5 text-sky-500/90' />
                      )}
                    </span>
                    <input
                      ref={createInputRef}
                      type='text'
                      className='h-5 min-w-0 flex-1 rounded bg-transparent px-1 font-mono text-xs text-foreground outline-none ring-1 ring-primary/50'
                      placeholder={
                        creatingNode.nodeType === 'folder'
                          ? t('newFolder')
                          : t('newFile')
                      }
                      onKeyDown={handleCreateKeyDown}
                      onBlur={(e) => {
                        const value = e.currentTarget.value.trim();
                        if (value) onCreateCommit?.(value);
                        else onCreateCancel?.();
                      }}
                    />
                  </div>
                ) : null}

                {hasChildren ? (
                  <TreeView
                    nodes={node.children || []}
                    selectedNodeId={selectedNodeId}
                    selectedFolderId={selectedFolderId}
                    expandedFolderIds={expandedFolderIds}
                    onSelect={onSelect}
                    onContextMenu={onContextMenu}
                    dirtyNodeIds={dirtyNodeIds}
                    depth={depth + 1}
                    renamingNodeId={renamingNodeId}
                    onRenameStart={onRenameStart}
                    onRenameCommit={onRenameCommit}
                    onRenameCancel={onRenameCancel}
                    onCreateFile={onCreateFile}
                    onCreateFolder={onCreateFolder}
                    onDelete={onDelete}
                    creatingNode={creatingNode}
                    onCreateCommit={onCreateCommit}
                    onCreateCancel={onCreateCancel}
                    draggingNodeId={draggingNodeId}
                    dragOverFolderId={dragOverFolderId}
                    onDragStart={onDragStart}
                    onDragOver={onDragOver}
                    onDragLeave={onDragLeave}
                    onDrop={onDrop}
                    onDragEnd={onDragEnd}
                  />
                ) : null}
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
