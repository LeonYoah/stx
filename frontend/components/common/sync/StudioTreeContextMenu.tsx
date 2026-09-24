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

import React, { useEffect, useRef, useLayoutEffect, useState } from 'react';
import { useTranslations } from 'next-intl';
import {
  FilePlus2,
  FolderPlus,
  Pencil,
  FolderInput,
  Copy,
  Trash2,
  RefreshCw,
  Sparkles,
} from 'lucide-react';
import { SyncTaskTreeNode, TreeContextMenuState } from './sync-studio-utils';

// ============================================================================
// 类型定义 / Type Definitions
// ============================================================================

export interface StudioTreeContextMenuProps {
  menuState: TreeContextMenuState;
  onClose: () => void;
  onCreateFile: (folderNode: SyncTaskTreeNode | null) => void;
  onCreateFolder: (folderNode: SyncTaskTreeNode | null) => void;
  onRename: (node: SyncTaskTreeNode) => void;
  onMove: (node: SyncTaskTreeNode) => void;
  onCopyFile: (node: SyncTaskTreeNode) => void;
  onDelete: (node: SyncTaskTreeNode) => void;
  onRefresh: () => void;
  /** 文件另存为精选模板 / Save file as curated template */
  onSaveAsCurated?: (node: SyncTaskTreeNode) => void;
}

// ============================================================================
// 组件实现：VSCode 风格右键上下文菜单
// Component Implementation: VSCode-style Right-Click Context Menu
// ============================================================================

export function StudioTreeContextMenu({
  menuState,
  onClose,
  onCreateFile,
  onCreateFolder,
  onRename,
  onMove,
  onCopyFile,
  onDelete,
  onRefresh,
  onSaveAsCurated,
}: StudioTreeContextMenuProps) {
  const t = useTranslations('workbenchStudio');
  const menuRef = useRef<HTMLDivElement>(null);
  const [adjustedPos, setAdjustedPos] = useState<{ x: number; y: number }>({
    x: menuState.x,
    y: menuState.y,
  });

  // 视口边界碰撞检测，防止菜单溢出屏幕右侧或底部
  // Viewport collision detection to prevent menu overflowing right or bottom edges
  useLayoutEffect(() => {
    if (!menuRef.current) {
      return;
    }
    const rect = menuRef.current.getBoundingClientRect();
    const padding = 12;
    let nextX = menuState.x;
    let nextY = menuState.y;

    if (nextX + rect.width > window.innerWidth - padding) {
      nextX = Math.max(padding, window.innerWidth - rect.width - padding);
    }
    if (nextY + rect.height > window.innerHeight - padding) {
      nextY = Math.max(padding, window.innerHeight - rect.height - padding);
    }
    setAdjustedPos({ x: nextX, y: nextY });
  }, [menuState.x, menuState.y]);

  // 全局点击与 Escape 快捷键监听关闭
  // Global click and Escape key listener for closing
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose();
      }
    };
    const handlePointerDown = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('mousedown', handlePointerDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('mousedown', handlePointerDown);
    };
  }, [onClose]);

  if (!menuState.open) {
    return null;
  }

  const isRoot = menuState.kind === 'root';
  const isFolder = menuState.kind === 'folder';
  const isFile = menuState.kind === 'file';
  const canEdit = menuState.node?.can_edit !== false;

  return (
    <div
      ref={menuRef}
      className='fixed z-50 min-w-[170px] select-none rounded-lg border border-border/80 bg-popover/95 p-1 text-xs text-popover-foreground shadow-xl backdrop-blur-md animate-in fade-in-0 zoom-in-95'
      style={{ left: adjustedPos.x, top: adjustedPos.y }}
      onContextMenu={(e) => e.preventDefault()}
    >
      {/* 新建操作组：仅根目录或文件夹展示 */}
      {/* Creation group: shown only for root or folder */}
      {isRoot || isFolder ? (
        <>
          <button
            type='button'
            className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground'
            onClick={() => {
              onClose();
              onCreateFolder(menuState.node || null);
            }}
          >
            <FolderPlus className='mr-2 size-3.5 text-muted-foreground transition-colors group-hover:text-foreground' />
            <span>{t('newFolder')}</span>
          </button>
          {isFolder ? (
            <button
              type='button'
              className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground'
              onClick={() => {
                onClose();
                if (menuState.node) {
                  onCreateFile(menuState.node);
                }
              }}
            >
              <FilePlus2 className='mr-2 size-3.5 text-muted-foreground transition-colors group-hover:text-foreground' />
              <span>{t('newFile')}</span>
            </button>
          ) : null}
          <div className='my-1 h-px bg-border/40' />
        </>
      ) : null}

      {/* 修改与移动组 */}
      {/* Modification & movement group */}
      {!isRoot && canEdit && menuState.node ? (
        <>
          <button
            type='button'
            className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground'
            onClick={() => {
              onClose();
              if (menuState.node) {
                onRename(menuState.node);
              }
            }}
          >
            <Pencil className='mr-2 size-3.5 text-muted-foreground transition-colors group-hover:text-foreground' />
            <span>{t('rename')}</span>
            <span className='ml-auto font-mono text-[10px] text-muted-foreground/60'>
              F2
            </span>
          </button>
          <button
            type='button'
            className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground'
            onClick={() => {
              onClose();
              if (menuState.node) {
                onMove(menuState.node);
              }
            }}
          >
            <FolderInput className='mr-2 size-3.5 text-muted-foreground transition-colors group-hover:text-foreground' />
            <span>{t('moveTo')}</span>
          </button>
        </>
      ) : null}

      {/* 文件复制 / 另存精选 */}
      {/* File duplication / save as curated */}
      {isFile && menuState.node ? (
        <>
          <button
            type='button'
            className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground'
            onClick={() => {
              onClose();
              if (menuState.node) {
                onCopyFile(menuState.node);
              }
            }}
          >
            <Copy className='mr-2 size-3.5 text-muted-foreground transition-colors group-hover:text-foreground' />
            <span>{t('copyFile')}</span>
          </button>
          {onSaveAsCurated ? (
            <button
              type='button'
              className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground'
              onClick={() => {
                onClose();
                if (menuState.node) {
                  onSaveAsCurated(menuState.node);
                }
              }}
            >
              <Sparkles className='mr-2 size-3.5 text-muted-foreground transition-colors group-hover:text-foreground' />
              <span>{t('saveAsCurated')}</span>
            </button>
          ) : null}
        </>
      ) : null}

      {/* 危险操作组：删除 */}
      {/* Destructive group: delete */}
      {!isRoot && canEdit && menuState.node ? (
        <>
          <div className='my-1 h-px bg-border/40' />
          <button
            type='button'
            className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-destructive transition-colors hover:bg-destructive/10'
            onClick={() => {
              onClose();
              if (menuState.node) {
                onDelete(menuState.node);
              }
            }}
          >
            <Trash2 className='mr-2 size-3.5 text-destructive/80 transition-colors group-hover:text-destructive' />
            <span>{t('delete')}</span>
          </button>
        </>
      ) : null}

      {/* 辅助组：刷新 */}
      {/* Utility group: refresh */}
      <div className='my-1 h-px bg-border/40' />
      <button
        type='button'
        className='group flex w-full items-center rounded-sm px-2.5 py-1.5 text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground'
        onClick={() => {
          onClose();
          onRefresh();
        }}
      >
        <RefreshCw className='mr-2 size-3.5 text-muted-foreground transition-colors group-hover:text-foreground' />
        <span>{t('refresh')}</span>
      </button>
    </div>
  );
}
