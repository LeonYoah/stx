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
 * 工作台文件标签栏：基于 Radix Tabs 提供 WAI-ARIA 键盘导航，
 * 配合 Motion layoutId 指示条、未保存圆点、中键关闭与右键上下文菜单。
 * Workbench open-file tab strip built on Radix Tabs for keyboard navigation,
 * Motion layoutId active accent bar, dirty dot, middle-click close, and context menu.
 */
import {useEffect, useId, useRef, useState} from 'react';
import * as TabsPrimitive from '@radix-ui/react-tabs';
import {LayoutGroup, motion, useReducedMotion} from 'motion/react';
import {FileCode2, MoreHorizontal, X} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {motionTokens} from '@/components/arc/lib/motion-tokens';
import {Button} from '@/components/arc/button/button';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {cn} from '@/lib/utils';

export interface StudioFileTabItem {
  id: number;
  name: string;
  dirty?: boolean;
}

export interface StudioFileTabsProps {
  tabs: StudioFileTabItem[];
  activeTabId: number | null;
  onSelectTab: (tabId: number) => void;
  onCloseTab: (tabId: number) => void;
  onCloseOtherTabs: (keepTabId: number) => void;
  onCloseAllTabs: () => void;
}

export function StudioFileTabs({
  tabs,
  activeTabId,
  onSelectTab,
  onCloseTab,
  onCloseOtherTabs,
  onCloseAllTabs,
}: StudioFileTabsProps) {
  const t = useTranslations('workbenchStudio');
  const reduced = useReducedMotion();
  const groupId = useId();
  const stripRef = useRef<HTMLDivElement | null>(null);
  const itemRefs = useRef<Record<number, HTMLDivElement | null>>({});
  const menuRef = useRef<HTMLDivElement | null>(null);
  const [contextMenu, setContextMenu] = useState<{
    tabId: number;
    x: number;
    y: number;
  } | null>(null);

  useEffect(() => {
    if (!activeTabId) {
      return;
    }
    const strip = stripRef.current;
    const activeEl = itemRefs.current[activeTabId];
    if (!strip || !activeEl) {
      return;
    }
    const stripRect = strip.getBoundingClientRect();
    const tabRect = activeEl.getBoundingClientRect();
    const isOutOfView =
      tabRect.left < stripRect.left || tabRect.right > stripRect.right;
    if (!isOutOfView) {
      return;
    }
    const targetLeft =
      activeEl.offsetLeft - strip.clientWidth / 2 + activeEl.clientWidth / 2;
    strip.scrollTo?.({
      left: Math.max(0, targetLeft),
      behavior: reduced ? 'auto' : 'smooth',
    });
  }, [tabs, activeTabId, reduced]);

  useEffect(() => {
    if (!contextMenu) {
      return;
    }
    const close = () => setContextMenu(null);
    const onPointerDown = (event: MouseEvent) => {
      if (
        menuRef.current &&
        event.target instanceof Node &&
        !menuRef.current.contains(event.target)
      ) {
        close();
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        close();
      }
    };
    window.addEventListener('mousedown', onPointerDown);
    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('scroll', close, true);
    return () => {
      window.removeEventListener('mousedown', onPointerDown);
      window.removeEventListener('keydown', onKeyDown);
      window.removeEventListener('scroll', close, true);
    };
  }, [contextMenu]);

  return (
    <TabsPrimitive.Root
      value={activeTabId ? String(activeTabId) : ''}
      onValueChange={(nextValue) => {
        const nextId = Number(nextValue);
        if (Number.isFinite(nextId) && nextId > 0) {
          onSelectTab(nextId);
        }
      }}
      className='flex h-10 shrink-0 items-center justify-between overflow-hidden border-b border-border/40 bg-muted/20 px-1.5 select-none'
    >
      <LayoutGroup id={`studio-file-tabs-${groupId}`}>
        <div
          ref={stripRef}
          className='flex h-full flex-1 items-center overflow-x-auto py-1 scrollbar-none'
          onWheel={(event) => {
            if (Math.abs(event.deltaY) > Math.abs(event.deltaX) && stripRef.current) {
              stripRef.current.scrollLeft += event.deltaY;
            }
          }}
        >
          {tabs.length > 0 ? (
            <TabsPrimitive.List
              aria-label={t('openFiles')}
              className='flex h-full items-center gap-1'
            >
              {tabs.map((tab) => {
                const isTabActive = activeTabId === tab.id;
                const isTabDirty = Boolean(tab.dirty);
                return (
                  <div
                    key={tab.id}
                    ref={(node) => {
                      itemRefs.current[tab.id] = node;
                    }}
                    className={cn(
                      'group relative flex h-full shrink-0 items-center rounded-[calc(var(--radius-control,0.875rem)-4px)] transition-colors',
                      isTabActive
                        ? 'text-foreground'
                        : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
                    )}
                    onAuxClick={(event) => {
                      if (event.button === 1) {
                        event.preventDefault();
                        onCloseTab(tab.id);
                      }
                    }}
                    onContextMenu={(event) => {
                      event.preventDefault();
                      setContextMenu({
                        tabId: tab.id,
                        x: event.clientX,
                        y: event.clientY,
                      });
                    }}
                  >
                    {isTabActive ? (
                      <motion.span
                        layoutId='studio-file-tab-pill'
                        transition={
                          reduced
                            ? {duration: 0}
                            : motionTokens.spring.morph
                        }
                        className='pointer-events-none absolute inset-0 rounded-[calc(var(--radius-control,0.875rem)-4px)] border border-border/70 bg-background shadow-2xs'
                      />
                    ) : null}

                    {/* 标签触发器与关闭按钮拆成兄弟节点，保证无障碍语义与不嵌套 button。 */}
                    {/* Keep Trigger and close button as siblings so WAI-ARIA tabs stay valid without nested buttons. */}
                    <TabsPrimitive.Trigger
                      value={String(tab.id)}
                      className={cn(
                        'relative z-10 flex h-full cursor-pointer items-center gap-1.5 rounded-l-[calc(var(--radius-control,0.875rem)-4px)] pl-2.5 pr-1 text-xs outline-none focus-visible:ring-1 focus-visible:ring-ring',
                        isTabActive && 'font-medium',
                      )}
                    >
                      <FileCode2
                        className={cn(
                          'size-3.5 shrink-0',
                          isTabActive
                            ? 'text-primary'
                            : 'text-muted-foreground/60',
                        )}
                      />
                      <span className='max-w-[150px] truncate' title={tab.name}>
                        {tab.name}
                      </span>
                    </TabsPrimitive.Trigger>

                    <div className='relative z-10 flex items-center pr-1.5'>
                      <div className='relative flex size-4 shrink-0 items-center justify-center'>
                        {isTabDirty ? (
                          <span
                            aria-label={t('unsavedDraft')}
                            title={t('unsavedChanges')}
                            className='pointer-events-none size-1.5 rounded-full bg-amber-500 transition-all group-hover:scale-0 group-hover:opacity-0'
                          />
                        ) : null}
                        <button
                          type='button'
                          aria-label={`${t('close')} ${tab.name}`}
                          className={cn(
                            'absolute inset-0 flex items-center justify-center rounded-full text-muted-foreground transition-all hover:bg-muted-foreground/20 hover:text-foreground',
                            isTabDirty
                              ? 'scale-0 opacity-0 group-hover:scale-100 group-hover:opacity-100 focus-visible:scale-100 focus-visible:opacity-100'
                              : isTabActive
                                ? 'opacity-60 hover:opacity-100'
                                : 'opacity-0 group-hover:opacity-60 hover:!opacity-100 focus-visible:opacity-100',
                          )}
                          onClick={() => onCloseTab(tab.id)}
                        >
                          <X className='size-2.5' />
                        </button>
                      </div>
                    </div>
                  </div>
                );
              })}
            </TabsPrimitive.List>
          ) : (
            <div className='flex items-center px-2.5 text-xs text-muted-foreground/60'>
              {t('noOpenFiles')}
            </div>
          )}
        </div>
      </LayoutGroup>

      {/* 标签栏快捷菜单（关闭其他 / 关闭全部） / Tab bar overflow menu */}
      {tabs.length > 0 ? (
        <div className='flex items-center pl-1'>
          <DropdownMenu
            trigger={
              <Button
                type='button'
                size='sm'
                variant='ghost'
                iconOnly
                aria-label={t('tabActions')}
                className='size-7! min-h-7! text-muted-foreground hover:text-foreground'
              >
                <MoreHorizontal className='size-3.5' />
              </Button>
            }
            items={[
              ...(activeTabId
                ? [
                    {
                      label: t('closeOtherTabs'),
                      disabled: tabs.length <= 1,
                      onSelect: () => onCloseOtherTabs(activeTabId),
                    },
                  ]
                : []),
              {
                label: t('closeAllTabs'),
                onSelect: onCloseAllTabs,
              },
            ]}
          />
        </div>
      ) : null}

      {/* 右键上下文菜单（沿用 Arc 弹层 Token） / Right-click context menu using Arc popover tokens */}
      {contextMenu ? (
        <div
          ref={menuRef}
          role='menu'
          style={{
            left: `${contextMenu.x}px`,
            top: `${contextMenu.y}px`,
            borderRadius: 'var(--radius-popover, 0.75rem)',
            background: 'var(--surface-raised)',
            borderColor: 'var(--border)',
            boxShadow: 'var(--shadow-floating)',
          }}
          className='fixed z-50 min-w-[148px] border p-1 text-xs text-foreground'
        >
          <button
            type='button'
            role='menuitem'
            className='flex w-full items-center rounded-[calc(var(--radius-popover,0.75rem)-4px)] px-2.5 py-1.5 text-left transition-colors hover:bg-muted/80'
            onClick={() => {
              const targetId = contextMenu.tabId;
              setContextMenu(null);
              onCloseTab(targetId);
            }}
          >
            {t('close')}
          </button>
          <button
            type='button'
            role='menuitem'
            disabled={tabs.length <= 1}
            className='flex w-full items-center rounded-[calc(var(--radius-popover,0.75rem)-4px)] px-2.5 py-1.5 text-left transition-colors hover:bg-muted/80 disabled:pointer-events-none disabled:opacity-40'
            onClick={() => {
              const targetId = contextMenu.tabId;
              setContextMenu(null);
              onCloseOtherTabs(targetId);
            }}
          >
            {t('closeOtherTabs')}
          </button>
          <button
            type='button'
            role='menuitem'
            className='flex w-full items-center rounded-[calc(var(--radius-popover,0.75rem)-4px)] px-2.5 py-1.5 text-left transition-colors hover:bg-muted/80'
            onClick={() => {
              setContextMenu(null);
              onCloseAllTabs();
            }}
          >
            {t('closeAllTabs')}
          </button>
        </div>
      ) : null}
    </TabsPrimitive.Root>
  );
}
