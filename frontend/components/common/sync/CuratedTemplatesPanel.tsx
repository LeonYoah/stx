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

import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Eye, Loader2, Plus, Sparkles} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {cn} from '@/lib/utils';
import type {
  SyncCuratedSection,
  SyncCuratedTemplateView,
} from '@/lib/services/sync';
import services from '@/lib/services';

const SECTION_ORDER: SyncCuratedSection[] = [
  'env',
  'source',
  'transform',
  'sink',
  'combo',
];

function itemKey(item: SyncCuratedTemplateView) {
  return `${item.origin}-${item.builtin_id || item.id}-${item.name}`;
}

/**
 * 精选模板插入面板：列表在上可选预览，确认后再插入。
 * Curated insert panel: list on top with preview, insert after confirm.
 */
export function CuratedTemplatesPanel({
  onInsert,
  clusterId,
  refreshToken = 0,
}: {
  onInsert: (item: SyncCuratedTemplateView) => void;
  /** 有集群时用 render 预览（含低版本 plugin IO 改写） / Use render for preview when cluster is set */
  clusterId?: string;
  refreshToken?: number;
}) {
  const t = useTranslations('workbenchStudio');
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<SyncCuratedTemplateView[]>([]);
  const [section, setSection] = useState<string>('all');
  const [query, setQuery] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [preview, setPreview] = useState('');
  const [previewLoading, setPreviewLoading] = useState(false);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await services.sync.listCuratedTemplates({
        section: section === 'all' ? undefined : section,
        q: query.trim() || undefined,
      });
      setItems(data.items || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('loadCuratedFailed'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [section, refreshToken]);

  const grouped = useMemo(() => {
    const map = new Map<string, SyncCuratedTemplateView[]>();
    for (const item of items) {
      const key = item.section || 'combo';
      const list = map.get(key) || [];
      list.push(item);
      map.set(key, list);
    }
    return SECTION_ORDER.filter((key) => map.has(key)).map((key) => ({
      section: key,
      items: map.get(key) || [],
    }));
  }, [items]);

  const selected = useMemo(
    () => items.find((item) => itemKey(item) === selectedKey) || null,
    [items, selectedKey],
  );

  // 选中后立刻展示正文；有集群再走 render 做版本改写预览
  // Show content immediately on select; render with cluster for legacy rewrite preview
  useEffect(() => {
    if (!selected) {
      setPreview('');
      setPreviewLoading(false);
      return;
    }
    const fallback = (selected.content || '').trim();
    setPreview(fallback);
    const clusterNum = clusterId ? Number(clusterId) : 0;
    if (!Number.isFinite(clusterNum) || clusterNum <= 0) {
      setPreviewLoading(false);
      return;
    }
    let cancelled = false;
    setPreviewLoading(true);
    void services.sync
      .renderCuratedTemplate({
        builtin_id: selected.builtin_id,
        id: selected.id,
        cluster_id: clusterNum,
      })
      .then((data) => {
        if (!cancelled) {
          setPreview((data.content || fallback).trim());
        }
      })
      .catch(() => {
        if (!cancelled) {
          setPreview(fallback);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setPreviewLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [selected, clusterId]);

  // 列表刷新后若当前选中项消失则清空
  // Clear selection when it disappears after list refresh
  useEffect(() => {
    if (selectedKey && !items.some((item) => itemKey(item) === selectedKey)) {
      setSelectedKey(null);
    }
  }, [items, selectedKey]);

  return (
    <div className='space-y-2.5'>
      <div className='flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-foreground'>
        <Sparkles className='size-3.5 text-primary' />
        <span>{t('curatedTemplates')}</span>
      </div>

      <div className='flex gap-2'>
        <Select value={section} onValueChange={setSection}>
          <SelectTrigger className='h-8 text-xs'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('curatedSectionAll')}</SelectItem>
            <SelectItem value='env'>env</SelectItem>
            <SelectItem value='source'>source</SelectItem>
            <SelectItem value='transform'>transform</SelectItem>
            <SelectItem value='sink'>sink</SelectItem>
            <SelectItem value='combo'>combo</SelectItem>
          </SelectContent>
        </Select>
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              void load();
            }
          }}
          placeholder={t('searchCurated')}
          className='h-8 text-xs'
        />
      </div>

      {loading ? (
        <div className='flex items-center gap-2 text-xs text-muted-foreground py-3'>
          <Loader2 className='size-3.5 animate-spin' />
          {t('loadingCurated')}
        </div>
      ) : error ? (
        <p className='text-[11px] text-destructive'>{error}</p>
      ) : grouped.length === 0 ? (
        <p className='text-[11px] text-muted-foreground'>{t('noCuratedTemplates')}</p>
      ) : (
        <div className='space-y-1.5 max-h-44 overflow-y-auto pr-1'>
          {grouped.map((group) => (
            <div key={group.section} className='space-y-1'>
              <div className='text-[10px] uppercase tracking-wide text-muted-foreground sticky top-0 bg-muted/10 py-0.5'>
                {group.section}
              </div>
              {group.items.map((item) => {
                const key = itemKey(item);
                const active = key === selectedKey;
                return (
                  <button
                    key={key}
                    type='button'
                    className={cn(
                      'w-full rounded-md border px-2.5 py-1.5 text-left transition-colors',
                      active
                        ? 'border-primary/50 bg-primary/5'
                        : 'border-border/50 bg-background/60 hover:border-primary/40 hover:bg-muted/30',
                    )}
                    onClick={() => setSelectedKey(key)}
                  >
                    <div className='flex items-center gap-1.5 min-w-0'>
                      <span className='text-xs font-medium truncate'>
                        {item.name}
                      </span>
                      <Badge
                        variant='secondary'
                        className='text-[9px] px-1 py-0 shrink-0'
                      >
                        {item.origin}
                      </Badge>
                    </div>
                    {item.description ? (
                      <p className='mt-0.5 text-[10px] text-muted-foreground line-clamp-1'>
                        {item.description}
                      </p>
                    ) : null}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      )}

      {/* 预览区：选中后展示 HOCON */}
      {/* Preview pane: show HOCON after selection */}
      <div className='rounded-md border border-border/50 bg-background/70 overflow-hidden'>
        <div className='flex items-center justify-between gap-2 border-b border-border/40 px-2.5 py-1.5'>
          <div className='flex items-center gap-1.5 text-[10px] uppercase tracking-wide text-muted-foreground'>
            <Eye className='size-3' />
            <span>{t('curatedPreview')}</span>
            {previewLoading ? (
              <Loader2 className='size-3 animate-spin' />
            ) : null}
          </div>
          <Button
            type='button'
            size='sm'
            className='h-6 px-2 text-[11px]'
            disabled={!selected}
            onClick={() => {
              if (selected) {
                onInsert(selected);
              }
            }}
          >
            <Plus className='size-3 mr-1' />
            {t('curatedInsert')}
          </Button>
        </div>
        {selected ? (
          <pre className='max-h-40 overflow-auto px-2.5 py-2 text-[10px] leading-4 font-mono text-foreground/90 whitespace-pre-wrap break-all'>
            {preview || t('curatedPreviewEmpty')}
          </pre>
        ) : (
          <p className='px-2.5 py-3 text-[11px] text-muted-foreground'>
            {t('curatedPreviewHint')}
          </p>
        )}
      </div>

      <p className='text-[11px] leading-5 text-muted-foreground'>
        {t('curatedTemplateHint')}
      </p>
    </div>
  );
}
