/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI File Tree (https://beui.dev/components/motion/file-tree).
 * Registry: https://beui.dev/r/file-tree.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 *
 * 不引入 beUI 的 lib/i18n.tsx，以免覆盖 STX 的 next-intl `lib/i18n/`。
 * Do not install beUI's lib/i18n.tsx; STX already owns frontend/lib/i18n/.
 * Local STX adaptations (row slots, treeitem rows) remain under the same MIT
 * terms and are not relicensed as Apache-2.0.
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

'use client';

/*
 * STX 扩展：label / trailing / selectable / 拖拽 / 右键，行用 div[role=treeitem]
 * 以便行内嵌套快捷按钮。展开与选中仍走官方 FileTree API。
 * STX slots: label, trailing, selectable, drag, context menu. Rows are
 * div[role=treeitem] so hover actions can stay real buttons.
 */

import {AnimatePresence, motion, useReducedMotion} from 'motion/react';
import {ChevronRight, File, Folder, FolderOpen} from 'lucide-react';
import {
  Children,
  Fragment,
  useCallback,
  isValidElement,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
} from 'react';
import {SharedLayoutBg} from '@/components/motion/shared-layout-bg';
import {EASE_OUT, SPRING_LAYOUT, SPRING_SWAP} from '@/lib/ease';
import {cn} from '@/lib/utils';

/**
 * 行级交互扩展，供工作台右键、拖拽、行内编辑复用。
 * Row-level interaction slots for workbench context menu, drag, and inline edit.
 */
export type FileTreeRowEvents = {
  selectable?: boolean;
  label?: ReactNode;
  trailing?: ReactNode;
  draggable?: boolean;
  onContextMenu?: (event: MouseEvent<HTMLElement>) => void;
  onDoubleClick?: (event: MouseEvent<HTMLElement>) => void;
  onDragStart?: (event: DragEvent<HTMLElement>) => void;
  onDragOver?: (event: DragEvent<HTMLElement>) => void;
  onDragLeave?: (event: DragEvent<HTMLElement>) => void;
  onDrop?: (event: DragEvent<HTMLElement>) => void;
  onDragEnd?: (event: DragEvent<HTMLElement>) => void;
};

type FileTreeItem = {
  value: string;
  name: string;
  type: 'file' | 'folder';
  children?: FileTreeItem[];
  icon?: ReactNode;
  disabled?: boolean;
  className?: string;
} & FileTreeRowEvents;

export interface FileTreeFolderProps extends FileTreeRowEvents {
  value: string;
  name: string;
  icon?: ReactNode;
  disabled?: boolean;
  children?: ReactNode;
  className?: string;
}

export interface FileTreeFileProps extends FileTreeRowEvents {
  value: string;
  name: string;
  icon?: ReactNode;
  disabled?: boolean;
  className?: string;
}

export type FileTreeClassNames = {
  tree?: string;
  item?: string;
  icon?: string;
  label?: string;
};

/** English defaults. Override per instance with `labels`. */
export const fileTreeMessages = {
  label: 'Files',
};

export type FileTreeMessages = typeof fileTreeMessages;

export interface FileTreeProps {
  children: ReactNode;
  value?: string | null;
  defaultValue?: string | null;
  onValueChange?: (value: string) => void;
  expandedIds?: string[];
  defaultExpandedIds?: string[];
  onExpandedChange?: (expandedIds: string[]) => void;
  ariaLabel?: string;
  /** Override any built-in string. */
  labels?: Partial<FileTreeMessages>;
  indent?: number;
  className?: string;
  classNames?: FileTreeClassNames;
}

type FlatFileTreeItem = {
  item: FileTreeItem;
  depth: number;
  parentId: string | null;
  position: number;
  setSize: number;
};

function pickRowEvents(props: FileTreeRowEvents): FileTreeRowEvents {
  return {
    selectable: props.selectable,
    label: props.label,
    trailing: props.trailing,
    draggable: props.draggable,
    onContextMenu: props.onContextMenu,
    onDoubleClick: props.onDoubleClick,
    onDragStart: props.onDragStart,
    onDragOver: props.onDragOver,
    onDragLeave: props.onDragLeave,
    onDrop: props.onDrop,
    onDragEnd: props.onDragEnd,
  };
}

// These declarative parts are read by FileTree and turned into one flattened,
// keyboard-navigable collection. They intentionally render nothing alone.
export function FileTreeFolder(_props: FileTreeFolderProps) {
  return null;
}

export function FileTreeFile(_props: FileTreeFileProps) {
  return null;
}

const ROW_ENTER = {duration: 0.22, ease: EASE_OUT} as const;
const BRANCH_DRAW = {duration: 0.3, ease: EASE_OUT} as const;

function flattenItems(
  items: FileTreeItem[],
  expanded: ReadonlySet<string>,
  depth = 0,
  parentId: string | null = null,
): FlatFileTreeItem[] {
  return items.flatMap((item, index) => {
    const row = {
      item,
      depth,
      parentId,
      position: index + 1,
      setSize: items.length,
    };

    if (
      item.type !== 'folder' ||
      !expanded.has(item.value) ||
      !item.children?.length
    ) {
      return [row];
    }

    return [
      row,
      ...flattenItems(item.children, expanded, depth + 1, item.value),
    ];
  });
}

function itemsFromChildren(children: ReactNode): FileTreeItem[] {
  const items: FileTreeItem[] = [];

  Children.forEach(children, (child) => {
    if (!isValidElement(child)) return;

    if (child.type === Fragment) {
      const props = child.props as {children?: ReactNode};
      items.push(...itemsFromChildren(props.children));
      return;
    }

    if (child.type === FileTreeFolder) {
      const props = child.props as FileTreeFolderProps;
      items.push({
        value: props.value,
        name: props.name,
        type: 'folder',
        icon: props.icon,
        disabled: props.disabled,
        className: props.className,
        children: itemsFromChildren(props.children),
        ...pickRowEvents(props),
      });
      return;
    }

    if (child.type === FileTreeFile) {
      const props = child.props as FileTreeFileProps;
      items.push({
        value: props.value,
        name: props.name,
        type: 'file',
        icon: props.icon,
        disabled: props.disabled,
        className: props.className,
        ...pickRowEvents(props),
      });
    }
  });

  return items;
}

function DefaultIcon({
  item,
  open,
  reduce,
}: {
  item: FileTreeItem;
  open: boolean;
  reduce: boolean;
}) {
  if (item.type === 'file') return <File className='size-4' />;
  if (reduce) {
    return open ? (
      <FolderOpen className='size-4' />
    ) : (
      <Folder className='size-4' />
    );
  }

  return (
    <AnimatePresence initial={false} mode='popLayout'>
      <motion.span
        key={open ? 'open' : 'closed'}
        initial={{opacity: 0, scale: 0.75, rotate: open ? -8 : 8}}
        animate={{opacity: 1, scale: 1, rotate: 0}}
        exit={{opacity: 0, scale: 0.75, rotate: open ? 8 : -8}}
        transition={SPRING_SWAP}
        className='absolute inset-0 grid place-items-center'
      >
        {open ? (
          <FolderOpen className='size-4' />
        ) : (
          <Folder className='size-4' />
        )}
      </motion.span>
    </AnimatePresence>
  );
}

export function FileTree({
  children,
  value,
  defaultValue = null,
  onValueChange,
  expandedIds,
  defaultExpandedIds = [],
  onExpandedChange,
  ariaLabel,
  labels,
  indent = 18,
  className,
  classNames,
}: FileTreeProps) {
  const reduce = useReducedMotion() ?? false;
  const treeLabel = ariaLabel ?? labels?.label ?? fileTreeMessages.label;
  const [internalValue, setInternalValue] = useState(defaultValue);
  const [internalExpandedIds, setInternalExpandedIds] =
    useState(defaultExpandedIds);
  const [focusedId, setFocusedId] = useState<string | null>(
    value ?? defaultValue,
  );
  const rowRefs = useRef(new Map<string, HTMLElement>());
  const selectedId = value === undefined ? internalValue : value;
  const currentExpandedIds = expandedIds ?? internalExpandedIds;
  const expanded = useMemo(
    () => new Set(currentExpandedIds),
    [currentExpandedIds],
  );
  const items = useMemo(() => itemsFromChildren(children), [children]);
  const rows = useMemo(() => flattenItems(items, expanded), [expanded, items]);

  // Keep a real row tabbable in the first commit and immediately after a
  // collapse removes the previously focused descendant.
  const focusedRow =
    focusedId !== null && rows.some(({item}) => item.value === focusedId)
      ? focusedId
      : (rows[0]?.item.value ?? null);
  if (focusedId !== focusedRow) setFocusedId(focusedRow);

  const focusRow = useCallback((id: string) => {
    setFocusedId(id);
    const row = rowRefs.current.get(id);
    if (row) row.focus();
    else requestAnimationFrame(() => rowRefs.current.get(id)?.focus());
  }, []);

  const selectItem = useCallback(
    (item: FileTreeItem) => {
      if (item.disabled || item.selectable === false) return;
      if (value === undefined) setInternalValue(item.value);
      onValueChange?.(item.value);
    },
    [onValueChange, value],
  );

  const setExpanded = useCallback(
    (next: string[]) => {
      if (expandedIds === undefined) setInternalExpandedIds(next);
      onExpandedChange?.(next);
    },
    [expandedIds, onExpandedChange],
  );

  const toggleFolder = useCallback(
    (id: string) => {
      const next = new Set(currentExpandedIds);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      setExpanded(Array.from(next));
    },
    [currentExpandedIds, setExpanded],
  );

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLElement>, row: FlatFileTreeItem) => {
      // 行内重命名/新建输入的按键不能再走树导航，否则 Enter 会选中并折叠，方向键会把焦点挪走。
      // Ignore keys from nested inputs/buttons so inline edit does not also select, toggle, or move focus.
      const source = event.target;
      if (
        source instanceof HTMLElement &&
        source !== event.currentTarget &&
        source.closest(
          'input, textarea, select, button, [contenteditable="true"]',
        )
      ) {
        return;
      }

      const index = rows.findIndex(({item}) => item.value === row.item.value);
      const previous = rows[index - 1];
      const next = rows[index + 1];
      const isFolder = row.item.type === 'folder';
      const isOpen = expanded.has(row.item.value);

      if (event.key === 'ArrowDown' && next) {
        event.preventDefault();
        focusRow(next.item.value);
      } else if (event.key === 'ArrowUp' && previous) {
        event.preventDefault();
        focusRow(previous.item.value);
      } else if (event.key === 'Home' && rows[0]) {
        event.preventDefault();
        focusRow(rows[0].item.value);
      } else if (event.key === 'End' && rows.at(-1)) {
        event.preventDefault();
        focusRow(rows.at(-1)?.item.value ?? row.item.value);
      } else if (event.key === 'ArrowRight' && isFolder) {
        event.preventDefault();
        if (!isOpen && !row.item.disabled) toggleFolder(row.item.value);
        else if (next?.parentId === row.item.value) focusRow(next.item.value);
      } else if (event.key === 'ArrowLeft') {
        event.preventDefault();
        if (isFolder && isOpen && !row.item.disabled) {
          toggleFolder(row.item.value);
        } else if (row.parentId) {
          focusRow(row.parentId);
        }
      } else if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        if (row.item.disabled) return;
        selectItem(row.item);
        if (isFolder) toggleFolder(row.item.value);
      }
    },
    [expanded, focusRow, rows, selectItem, toggleFolder],
  );

  return (
    <SharedLayoutBg
      role='tree'
      data-testid='studio-file-tree'
      aria-label={treeLabel}
      aria-multiselectable='false'
      inset={0}
      pillClassName='rounded-xl bg-muted'
      pillContainerClassName='inset-y-auto top-0 h-9'
      className={cn('min-w-0', className, classNames?.tree)}
    >
      {rows.map((row) => {
        const isFolder = row.item.type === 'folder';
        const isOpen = isFolder && expanded.has(row.item.value);
        const isSelected = selectedId === row.item.value;

        return (
          <motion.div
            layout={reduce ? false : 'position'}
            key={row.item.value}
            initial={reduce ? false : {opacity: 0, y: -6}}
            animate={{
              opacity: row.item.disabled ? 0.42 : 1,
              y: 0,
              transition: reduce
                ? {duration: 0}
                : {
                    ...ROW_ENTER,
                    delay: Math.min(row.position * 0.025, 0.1),
                  },
            }}
            transition={reduce ? {duration: 0} : SPRING_LAYOUT}
          >
            <div
              ref={(node) => {
                if (node) rowRefs.current.set(row.item.value, node);
                else rowRefs.current.delete(row.item.value);
              }}
              role='treeitem'
              aria-level={row.depth + 1}
              aria-posinset={row.position}
              aria-setsize={row.setSize}
              aria-selected={isSelected}
              aria-expanded={isFolder ? isOpen : undefined}
              aria-disabled={row.item.disabled || undefined}
              tabIndex={focusedRow === row.item.value ? 0 : -1}
              draggable={row.item.draggable}
              onFocus={() => setFocusedId(row.item.value)}
              onKeyDown={(event) => handleKeyDown(event, row)}
              onClick={() => {
                if (row.item.disabled) return;
                selectItem(row.item);
                if (isFolder) toggleFolder(row.item.value);
              }}
              onContextMenu={row.item.onContextMenu}
              onDoubleClick={row.item.onDoubleClick}
              onDragStart={row.item.onDragStart}
              onDragOver={row.item.onDragOver}
              onDragLeave={row.item.onDragLeave}
              onDrop={row.item.onDrop}
              onDragEnd={row.item.onDragEnd}
              className={cn(
                'group relative flex h-9 w-full items-center gap-2 overflow-hidden rounded-lg pr-2 text-left text-sm text-muted-foreground outline-none',
                'cursor-pointer transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
                isSelected && 'bg-muted font-medium text-foreground',
                classNames?.item,
                row.item.className,
              )}
              style={{paddingLeft: 8 + row.depth * indent}}
            >
              {row.depth > 0 ? (
                <motion.span
                  aria-hidden='true'
                  initial={reduce ? false : {opacity: 0, scaleY: 0}}
                  animate={{opacity: 1, scaleY: 1}}
                  transition={reduce ? {duration: 0} : BRANCH_DRAW}
                  className='absolute top-0 bottom-0 w-px origin-top bg-border/70'
                  style={{left: 16 + (row.depth - 1) * indent}}
                />
              ) : null}

              <motion.span
                aria-hidden='true'
                animate={{rotate: isOpen ? 90 : 0}}
                transition={reduce ? {duration: 0} : SPRING_SWAP}
                className={cn(
                  'relative z-10 grid size-4 shrink-0 place-items-center',
                  !isFolder && 'opacity-0',
                )}
              >
                <ChevronRight className='size-3.5' />
              </motion.span>

              <span
                aria-hidden='true'
                className={cn(
                  'relative z-10 grid size-4 shrink-0 place-items-center text-muted-foreground transition-colors group-hover:text-foreground',
                  isFolder && isOpen && 'text-foreground',
                  classNames?.icon,
                )}
              >
                {row.item.icon ?? (
                  <DefaultIcon item={row.item} open={isOpen} reduce={reduce} />
                )}
              </span>

              <span
                className={cn(
                  'relative z-10 min-w-0 flex-1 overflow-hidden',
                  !row.item.label && 'truncate',
                  classNames?.label,
                )}
              >
                {row.item.label ?? row.item.name}
              </span>

              {row.item.trailing ? (
                <span className='relative z-10 ml-auto flex shrink-0 items-center gap-1'>
                  {row.item.trailing}
                </span>
              ) : null}
            </div>
          </motion.div>
        );
      })}
    </SharedLayoutBg>
  );
}
