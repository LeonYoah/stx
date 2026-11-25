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

import {useLayoutEffect, useMemo, useRef, useState, type MouseEvent} from 'react';
import {useTranslations} from 'next-intl';
import {Check, Copy} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Badge} from '@/components/ui/badge';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {Table, type TableColumn} from '@/components/motion/table';
import {cn} from '@/lib/utils';
import {formatCellValue, getPreviewRowKindBadgeClass} from './sync-studio-utils';
import {writeClipboard} from '@/components/arc/lib/use-copy-feedback';

type PreviewRow = Record<string, unknown>;

// 行高与控制台其他表一致（StudioDataTable 的 h-8）；虚拟滚动要求固定行高。
// Row height matches the other console tables (StudioDataTable h-8); virtualization needs a fixed row height.
const PREVIEW_ROW_HEIGHT = 32;
// 量不到高度（如测试环境没有布局）时的兜底视口高度。
// Fallback viewport height when nothing can be measured (e.g. tests without layout).
const PREVIEW_FALLBACK_HEIGHT = 320;
// 动态列给一个默认宽度：宽表横向滚动，而不是把每列挤到看不清。
// Dynamic columns get a default width so wide tables scroll sideways instead of squeezing every column.
const PREVIEW_COLUMN_WIDTH = '160px';

/**
 * 量出容器高度：beUI Table 的虚拟滚动需要像素高度，而控制台高度由拖动分隔条决定。
 * Measure the container height: beUI Table virtualization needs a pixel height while the console height
 * follows the drag handle.
 */
function useElementHeight<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [height, setHeight] = useState(0);
  // 布局阶段量高，首帧就用真实高度，避免先按兜底高度画一帧再跳。
  // Measure during layout so the first paint already uses the real height instead of jumping from the fallback.
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) {
      return;
    }
    const update = () => setHeight(Math.floor(element.clientHeight));
    update();
    if (typeof ResizeObserver === 'undefined') {
      return;
    }
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  return {ref, height};
}

/**
 * 排序值：数字按数值比，NULL 与空串排在最前，其余按显示文本比。
 * Sort value: numbers compare numerically, NULL and empty strings sort first, everything else by display text.
 */
function previewSortValue(value: unknown): string | number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (value === null || value === undefined) {
    return '';
  }
  return formatCellValue(value);
}

function PreviewCell({column, value}: {column: string; value: unknown}) {
  const t = useTranslations('workbenchStudio');
  const [copied, setCopied] = useState(false);

  // NULL 与空串在数据上不是一回事，必须看得出区别。
  // NULL and an empty string are different data and must look different.
  if (value === null || value === undefined) {
    return <span className='font-mono text-xs italic text-muted-foreground/70'>NULL</span>;
  }
  const text = formatCellValue(value);
  if (text === '') {
    return <span className='text-xs text-muted-foreground/70'>{t('previewEmptyString')}</span>;
  }
  if (column === 'RowKind') {
    return (
      <Badge
        variant='outline'
        className={cn('max-w-full truncate rounded-sm', getPreviewRowKindBadgeClass(text))}
      >
        {text}
      </Badge>
    );
  }

  const handleCopy = async (event: MouseEvent) => {
    // 别让点击冒泡成行选中或列排序。 / Keep the click from bubbling into row or column handlers.
    event.stopPropagation();
    try {
      await writeClipboard(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t('copyFailed'));
    }
  };

  return (
    <div className='group/cell flex min-w-0 items-center gap-1'>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className='min-w-0 flex-1 truncate'>{text}</span>
        </TooltipTrigger>
        <TooltipContent className='max-w-[480px] whitespace-pre-wrap break-all'>
          {text}
        </TooltipContent>
      </Tooltip>
      <button
        type='button'
        aria-label={t('copyCellValue', {column})}
        className='inline-flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover/cell:opacity-100'
        onClick={handleCopy}
      >
        {copied ? <Check className='size-3 text-emerald-500' /> : <Copy className='size-3' />}
      </button>
    </div>
  );
}

/**
 * 预览数据表：beUI Table（虚拟滚动、列宽拖动、列重排、排序），一次渲染全部样本行，不再分页。
 * 工作台其他列表仍用 StudioDataTable；这里是数据浏览，列动态、可能很宽、最多 1 万行。
 * Preview data table: beUI Table (virtualized, resizable, reorderable, sortable) rendering every sample row
 * without pagination. Other workbench lists stay on StudioDataTable; this one browses data with dynamic,
 * possibly wide columns and up to 10k rows.
 */
export function PreviewDataTable({
  columns,
  rows,
  loading,
  emptyMessage,
}: {
  columns: string[];
  rows: PreviewRow[];
  loading?: boolean;
  emptyMessage: string;
}) {
  const t = useTranslations('workbenchStudio');
  const {ref, height} = useElementHeight<HTMLDivElement>();

  const tableColumns = useMemo<TableColumn<PreviewRow>[]>(
    () =>
      columns.map((column) => ({
        key: column,
        header: column,
        sortable: true,
        width: PREVIEW_COLUMN_WIDTH,
        cell: (row) => <PreviewCell column={column} value={row[column]} />,
        sortValue: (row) => previewSortValue(row[column]),
      })),
    [columns],
  );

  // beUI 内置文案经 labels 走 next-intl；{column} 由 beUI 自己替换，所以取原文（t.raw）而不让 next-intl 格式化。
  // beUI built-in strings go through next-intl via labels; beUI fills {column} itself, so take the raw text (t.raw)
  // instead of letting next-intl format it.
  const labels = useMemo(
    () => ({
      empty: emptyMessage,
      resizeColumn: String(t.raw('previewResizeColumn')),
      reorderColumn: String(t.raw('previewReorderColumn')),
    }),
    [emptyMessage, t],
  );

  return (
    <div ref={ref} className='studio-preview-table min-h-0 flex-1'>
      <Table<PreviewRow>
          data={rows}
          columns={tableColumns}
          rowHeight={PREVIEW_ROW_HEIGHT}
          height={height > 0 ? height : PREVIEW_FALLBACK_HEIGHT}
          resizable
          reorderable
          minColumnWidth={72}
          loading={Boolean(loading) && rows.length === 0}
          skeletonRows={6}
          labels={labels}
          className='border-0 text-xs'
        />
    </div>
  );
}
