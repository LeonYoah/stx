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

import React, {useEffect, useMemo, useState} from 'react';
import Link from 'next/link';
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  useSortable,
} from '@dnd-kit/sortable';
import {CSS} from '@dnd-kit/utilities';
import {GripVertical} from 'lucide-react';
import {cn} from '@/lib/utils';

/**
 * 模块顶层导航 Tab 项定义
 * Navigation tab item definition for top-level twin modules
 */
export interface ModuleNavTabItem {
  /** 唯一标识 / Unique key */
  key: string;
  /** 显示标题 / Display label */
  label: React.ReactNode;
  /** 目标路由链接 / Target route link */
  href: string;
  /** 前置图标 / Leading icon */
  icon?: React.ReactNode;
  /** 可选的徽标数字或文字 / Optional badge count or text */
  badge?: React.ReactNode;
}

export interface ModuleNavTabsProps {
  /** 选项列表 / Tab items list */
  items: ModuleNavTabItem[];
  /** 当前激活的项 key / Currently active item key */
  activeKey: string;
  /**
   * 排序分组 ID：传入后启用拖拽排序，并按组写入 localStorage 跨页面共享
   * Reorder group id: enables drag-reorder and persists order across sibling pages
   */
  reorderGroupId?: string;
  /** 自定义外层容器类名 / Custom container class name */
  className?: string;
}

const ORDER_STORAGE_PREFIX = 'stx.module-nav.order:';

/**
 * 读取本地排序：保留已知 key 顺序，并追加新增项
 * Load persisted order: keep known keys, append any newly introduced tabs
 */
function loadOrder(groupId: string, keys: string[]): string[] {
  if (typeof window === 'undefined') {
    return keys;
  }
  try {
    const raw = window.localStorage.getItem(`${ORDER_STORAGE_PREFIX}${groupId}`);
    if (!raw) {
      return keys;
    }
    const saved = JSON.parse(raw) as unknown;
    if (!Array.isArray(saved)) {
      return keys;
    }
    const known = new Set(keys);
    const ordered = saved.filter(
      (key): key is string => typeof key === 'string' && known.has(key),
    );
    for (const key of keys) {
      if (!ordered.includes(key)) {
        ordered.push(key);
      }
    }
    return ordered;
  } catch {
    return keys;
  }
}

function saveOrder(groupId: string, keys: string[]): void {
  if (typeof window === 'undefined') {
    return;
  }
  try {
    window.localStorage.setItem(
      `${ORDER_STORAGE_PREFIX}${groupId}`,
      JSON.stringify(keys),
    );
  } catch {
    // 隐私模式或配额满时忽略持久化失败 / Ignore persist failures in private mode or quota full
  }
}

function sortItemsByKeys(
  items: ModuleNavTabItem[],
  keys: string[],
): ModuleNavTabItem[] {
  const map = new Map(items.map((item) => [item.key, item]));
  const ordered: ModuleNavTabItem[] = [];
  for (const key of keys) {
    const item = map.get(key);
    if (item) {
      ordered.push(item);
      map.delete(key);
    }
  }
  for (const item of map.values()) {
    ordered.push(item);
  }
  return ordered;
}

/**
 * 普通 Tab 链接（不可排序）
 * Plain tab link (not reorderable)
 */
function ModuleNavTabLink({
  item,
  isActive,
  withGripPadding = false,
}: {
  item: ModuleNavTabItem;
  isActive: boolean;
  withGripPadding?: boolean;
}) {
  return (
    <Link
      href={item.href}
      prefetch
      className={cn(
        'relative inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs transition-all duration-150 select-none whitespace-nowrap',
        withGripPadding && 'pl-4',
        isActive
          ? 'border border-border/50 bg-background font-semibold text-foreground shadow-2xs'
          : 'font-medium text-muted-foreground hover:bg-background/40 hover:text-foreground',
      )}
    >
      {item.icon ? (
        <span
          className={cn(
            'shrink-0 transition-colors [&_svg]:size-3.5',
            isActive
              ? 'text-primary'
              : 'text-muted-foreground group-hover/tab:text-foreground',
          )}
        >
          {item.icon}
        </span>
      ) : null}
      <span>{item.label}</span>
      {item.badge ? <span className='ml-0.5 shrink-0'>{item.badge}</span> : null}
    </Link>
  );
}

/**
 * 单个可排序 Tab：拖动手柄与路由 Link 分离，避免拖拽触发跳转
 * One sortable tab: grip handle is isolated from the route Link so drag does not navigate
 */
function SortableModuleNavTab({
  item,
  isActive,
}: {
  item: ModuleNavTabItem;
  isActive: boolean;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({id: item.key});

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Translate.toString(transform),
        transition,
      }}
      className={cn(
        'group/tab relative inline-flex items-center rounded-md transition-shadow',
        isDragging && 'z-20 opacity-90 shadow-md',
      )}
    >
      <button
        type='button'
        className={cn(
          'absolute left-0.5 top-1/2 z-10 inline-flex size-4 -translate-y-1/2 items-center justify-center rounded',
          'cursor-grab touch-none text-muted-foreground/50 opacity-0 transition-opacity',
          'hover:bg-muted hover:text-muted-foreground active:cursor-grabbing',
          'group-hover/tab:opacity-100 focus-visible:opacity-100',
          isActive && 'opacity-70',
          isDragging && 'opacity-100',
        )}
        aria-label='拖动排序 / Drag to reorder'
        title='拖动排序 / Drag to reorder'
        {...attributes}
        {...listeners}
        onClick={(event) => {
          // 阻止冒泡到 Link，纯点击手柄不应导航
          // Stop bubbling into Link so a plain grip click does not navigate
          event.preventDefault();
          event.stopPropagation();
        }}
      >
        <GripVertical className='size-3' />
      </button>

      <ModuleNavTabLink item={item} isActive={isActive} withGripPadding />
    </div>
  );
}

/**
 * 模块顶层 Tab 切换栏（主机/集群、安装包/插件等孪生模块；可选拖拽排序）
 * Top-level module tab bar for twin domains; optional drag-and-drop reorder
 */
export function ModuleNavTabs({
  items,
  activeKey,
  reorderGroupId,
  className,
}: ModuleNavTabsProps) {
  const itemKeys = useMemo(() => items.map((item) => item.key), [items]);
  const itemKeysSignature = itemKeys.join('|');
  const [orderKeys, setOrderKeys] = useState<string[]>(itemKeys);

  // 客户端读取同组持久化顺序，保证 /clusters 与 /hosts 共享先后
  // Load shared order on the client so sibling pages like /clusters and /hosts stay in sync
  useEffect(() => {
    if (!reorderGroupId) {
      setOrderKeys(itemKeys);
      return;
    }
    setOrderKeys(loadOrder(reorderGroupId, itemKeys));
  }, [reorderGroupId, itemKeysSignature, itemKeys]);

  const orderedItems = useMemo(
    () => sortItemsByKeys(items, orderKeys),
    [items, orderKeys],
  );

  const sensors = useSensors(
    useSensor(PointerSensor, {
      // 略微位移才算拖拽，避免误触导航
      // Require a small move before drag starts so clicks still navigate
      activationConstraint: {distance: 6},
    }),
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const {active, over} = event;
    if (!over || active.id === over.id || !reorderGroupId) {
      return;
    }
    setOrderKeys((prev) => {
      const oldIndex = prev.indexOf(String(active.id));
      const newIndex = prev.indexOf(String(over.id));
      if (oldIndex < 0 || newIndex < 0) {
        return prev;
      }
      const next = arrayMove(prev, oldIndex, newIndex);
      saveOrder(reorderGroupId, next);
      return next;
    });
  };

  const reorderable = Boolean(reorderGroupId);

  return (
    <nav
      aria-label='Module navigation'
      className={cn(
        'inline-flex items-center gap-1 rounded-lg border border-border/40 bg-muted/60 p-0.5 text-xs backdrop-blur-xs dark:bg-muted/40',
        className,
      )}
    >
      {reorderable ? (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
        >
          <SortableContext
            items={orderedItems.map((item) => item.key)}
            strategy={horizontalListSortingStrategy}
          >
            {orderedItems.map((item) => (
              <SortableModuleNavTab
                key={item.key}
                item={item}
                isActive={item.key === activeKey}
              />
            ))}
          </SortableContext>
        </DndContext>
      ) : (
        orderedItems.map((item) => (
          <ModuleNavTabLink
            key={item.key}
            item={item}
            isActive={item.key === activeKey}
          />
        ))
      )}
    </nav>
  );
}
