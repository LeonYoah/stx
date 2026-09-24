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
import {GitFork, Loader2, Save, Sparkles, Trash2} from 'lucide-react';
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

export function CuratedTemplatesPanel({
  onInsert,
  onSaveCurrent,
  onForkAndEdit,
  onDelete,
  refreshToken = 0,
}: {
  onInsert: (item: SyncCuratedTemplateView) => void;
  onSaveCurrent: () => void;
  /** 编辑内置：fork 后插入编辑器 / Edit builtin: fork then insert */
  onForkAndEdit: (item: SyncCuratedTemplateView) => void;
  /** 删除我的/副本精选 / Delete user or override curated */
  onDelete: (item: SyncCuratedTemplateView) => void;
  /** 另存成功后递增以刷新列表 / Increment after save-as to refresh list */
  refreshToken?: number;
}) {
  const t = useTranslations('workbenchStudio');
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<SyncCuratedTemplateView[]>([]);
  const [section, setSection] = useState<string>('all');
  const [query, setQuery] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busyKey, setBusyKey] = useState<string | null>(null);

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

  const itemKey = (item: SyncCuratedTemplateView) =>
    `${item.origin}-${item.builtin_id || item.id}-${item.name}`;

  return (
    <div className='space-y-2.5'>
      <div className='flex items-center justify-between gap-2'>
        <div className='flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-foreground'>
          <Sparkles className='size-3.5 text-primary' />
          <span>{t('curatedTemplates')}</span>
        </div>
        <Button
          type='button'
          size='sm'
          variant='outline'
          className='h-7 px-2 text-[11px]'
          onClick={onSaveCurrent}
        >
          <Save className='size-3.5 mr-1' />
          {t('saveAsCurated')}
        </Button>
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
        <div className='space-y-3 max-h-64 overflow-y-auto pr-1'>
          {grouped.map((group) => (
            <div key={group.section} className='space-y-1.5'>
              <div className='text-[10px] uppercase tracking-wide text-muted-foreground'>
                {group.section}
              </div>
              {group.items.map((item) => {
                const key = itemKey(item);
                const canForkEdit = item.origin === 'builtin' && !!item.builtin_id;
                const canDelete =
                  (item.origin === 'user' || item.origin === 'override') &&
                  !!item.id;
                return (
                  <div
                    key={key}
                    className='rounded-md border border-border/50 bg-background/60 hover:border-primary/40 hover:bg-muted/30 transition-colors'
                  >
                    <button
                      type='button'
                      className='w-full px-2.5 pt-2 pb-1.5 text-left'
                      onClick={() => onInsert(item)}
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
                        {item.has_override ? (
                          <Badge
                            variant='outline'
                            className='text-[9px] px-1 py-0 shrink-0'
                          >
                            {t('curatedHasOverride')}
                          </Badge>
                        ) : null}
                      </div>
                      {item.description ? (
                        <p className='mt-0.5 text-[10px] text-muted-foreground line-clamp-2'>
                          {item.description}
                        </p>
                      ) : null}
                    </button>
                    {(canForkEdit || canDelete) && (
                      <div className='flex items-center gap-1 px-2 pb-2'>
                        {canForkEdit ? (
                          <Button
                            type='button'
                            size='sm'
                            variant='ghost'
                            className='h-6 px-1.5 text-[10px]'
                            disabled={busyKey === key}
                            onClick={(e) => {
                              e.stopPropagation();
                              setBusyKey(key);
                              Promise.resolve(onForkAndEdit(item)).finally(() =>
                                setBusyKey(null),
                              );
                            }}
                          >
                            {busyKey === key ? (
                              <Loader2 className='size-3 mr-1 animate-spin' />
                            ) : (
                              <GitFork className='size-3 mr-1' />
                            )}
                            {item.has_override
                              ? t('curatedEditOverride')
                              : t('curatedForkEdit')}
                          </Button>
                        ) : null}
                        {canDelete ? (
                          <Button
                            type='button'
                            size='sm'
                            variant='ghost'
                            className='h-6 px-1.5 text-[10px] text-destructive hover:text-destructive'
                            disabled={busyKey === key}
                            onClick={(e) => {
                              e.stopPropagation();
                              if (!window.confirm(t('curatedDeleteConfirm'))) {
                                return;
                              }
                              setBusyKey(key);
                              Promise.resolve(onDelete(item)).finally(() =>
                                setBusyKey(null),
                              );
                            }}
                          >
                            <Trash2 className='size-3 mr-1' />
                            {t('delete')}
                          </Button>
                        ) : null}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      )}
      <p className='text-[11px] leading-5 text-muted-foreground'>
        {t('curatedTemplateHint')}
      </p>
    </div>
  );
}
