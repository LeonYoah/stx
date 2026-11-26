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
 * Deliveries view: the global notification-delivery log (channel / status /
 * event filters + server-side pagination), rendered as the second view inside
 * the notify tab. Read-only table — per-alert and per-channel slices still
 * live in the alert rail and the channel drawer.
 * 投递记录视图：通知页二级视图下的全局投递日志（渠道/状态/事件筛选 +
 * 服务端分页）。只读表格——按告警与按渠道的切片仍在告警详情栏与渠道抽屉里。
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/arc/button/button';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Select} from '@/components/arc/select/select';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationDelivery,
  NotificationDeliveryEventType,
  NotificationDeliveryFilterParams,
  NotificationDeliveryStatus,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {DeliveryChip} from '../status-chips';
import {NeutralChip, TruncatedText} from '@/components/common/layout';
import {formatAgo, minutesAgo} from '../monitoring-utils';
import styles from '../monitoring.module.css';
import layout from '@/components/common/layout/list-detail.module.css';

const PAGE_SIZE = 10;
/** Radix Select 不允许空字符串选项值，「全部」用哨兵值再映射回 ''。 / Sentinel for "all" (Radix rejects ''). */
const ALL_VALUE = '__all__';
const DELIVERY_EVENTS = ['firing', 'resolved', 'test', 'reminder'];
const DELIVERY_STATUSES = [
  'sent',
  'failed',
  'sending',
  'retrying',
  'pending',
  'canceled',
  'exhausted',
];
// 行内文案兜底范围比筛选项多一个历史别名 success。Row labels also cover the legacy 'success' alias.
const DELIVERY_STATUS_LABELS = [...DELIVERY_STATUSES, 'success'];

export type NotifyView = 'channels' | 'deliveries';

export function DeliveriesPanel({
  view,
  onViewChange,
  channels,
}: {
  view: NotifyView;
  onViewChange: (v: NotifyView) => void;
  channels: NotificationChannel[];
}) {
  const t = useTranslations('monitoring');
  const tc = useTranslations('common');
  const [items, setItems] = useState<NotificationDelivery[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [channelId, setChannelId] = useState('');
  const [status, setStatus] = useState('');
  const [eventType, setEventType] = useState('');

  const load = useCallback(async () => {
    const params: NotificationDeliveryFilterParams = {
      page,
      page_size: PAGE_SIZE,
    };
    if (channelId) {
      params.channel_id = Number(channelId);
    }
    if (status) {
      params.status = status as NotificationDeliveryStatus;
    }
    if (eventType) {
      params.event_type = eventType as NotificationDeliveryEventType;
    }
    const data = await services.monitoring.listNotificationDeliveries(params);
    const list = data.deliveries ?? [];
    const count = data.total ?? 0;
    // 页码越过新总数（筛选后条数变少或记录被清理）时回第一页重新拉取。
    // Drop back to page 1 when the cursor outran a shrunken result set.
    if (page > 1 && list.length === 0 && count > 0) {
      setPage(1);
      return;
    }
    setItems(list);
    setTotal(count);
  }, [page, channelId, status, eventType]);

  const refresh = useCallback(() => {
    setLoading(true);
    setError(null);
    load()
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }, [load]);

  useEffect(refresh, [refresh]);

  const channelOptions = useMemo(
    () => [
      {value: ALL_VALUE, label: t('scopeAll')},
      ...channels.map((c) => ({value: String(c.id), label: c.name})),
    ],
    [channels, t],
  );
  const statusOptions = useMemo(
    () => [
      {value: ALL_VALUE, label: t('scopeAll')},
      ...DELIVERY_STATUSES.map((s) => ({value: s, label: t(`delivery.${s}`)})),
    ],
    [t],
  );
  const eventOptions = useMemo(
    () => [
      {value: ALL_VALUE, label: t('scopeAll')},
      ...DELIVERY_EVENTS.map((e) => ({value: e, label: t(`deliveryEvent.${e}`)})),
    ],
    [t],
  );

  const hasFilters = Boolean(channelId || status || eventType);
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <>
      <div className={layout.toolbar}>
        <SegmentedControl
          label={t('notifyViewLabel')}
          layoutGroup='notify-view'
          value={view}
          onValueChange={(v) => onViewChange(v as NotifyView)}
          options={[
            {value: 'channels', label: t('channels')},
            {value: 'deliveries', label: t('deliveries')},
          ]}
        />
        <div className={cn(layout.filterCtrl, styles.filterW)}>
          <Select
            label={t('channels')}
            value={channelId || ALL_VALUE}
            onValueChange={(v) => {
              setChannelId(v === ALL_VALUE ? '' : v);
              setPage(1);
            }}
            displayValue={!channelId ? t('channels') : undefined}
            options={channelOptions}
          />
        </div>
        <div className={cn(layout.filterCtrl, styles.filterW)}>
          <Select
            label={t('deliveryStatusLabel')}
            value={status || ALL_VALUE}
            onValueChange={(v) => {
              setStatus(v === ALL_VALUE ? '' : v);
              setPage(1);
            }}
            displayValue={!status ? t('deliveryStatusLabel') : undefined}
            options={statusOptions}
          />
        </div>
        <div className={cn(layout.filterCtrl, styles.filterW)}>
          <Select
            label={t('deliveryEventLabel')}
            value={eventType || ALL_VALUE}
            onValueChange={(v) => {
              setEventType(v === ALL_VALUE ? '' : v);
              setPage(1);
            }}
            displayValue={!eventType ? t('deliveryEventLabel') : undefined}
            options={eventOptions}
          />
        </div>
      </div>

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='deliveries'>
          <TableCard
            loading={loading}
            hasData={items.length > 0}
            skeletonColumns={8}
            skeletonRows={8}
            error={error}
            onRetry={refresh}
            pagination={{
              currentPage: page,
              totalPages,
              pageSize: PAGE_SIZE,
              totalItems: total,
              onPageChange: setPage,
            }}
            empty={
              <EmptyState
                title={t('deliveriesEmpty')}
                description={
                  hasFilters ? t('emptyFilteredDesc') : t('deliveriesEmptyDesc')
                }
                action={
                  hasFilters ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={() => {
                        setChannelId('');
                        setStatus('');
                        setEventType('');
                        setPage(1);
                      }}
                    >
                      {t('clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <div className={layout.rowHead} aria-hidden>
              <span>{t('deliveryColAlert')}</span>
              <span>{t('channels')}</span>
              <span>{t('deliveryEventLabel')}</span>
              <span>{t('deliveryStatusLabel')}</span>
              <span>{t('deliveryColAttempts')}</span>
              <span>{t('deliveryColResponse')}</span>
              <span>{t('deliveryColTime')}</span>
              <span>{t('deliveryColError')}</span>
            </div>
            <div className={layout.tableScroll}>
              {items.map((d) => (
                <div key={d.id} className={cn(layout.row, styles.rowStatic)}>
                  <div className={layout.rowName}>
                    <TruncatedText
                      text={d.alert_name || d.alert_id}
                      className={layout.name}
                    />
                    {d.cluster_name ? (
                      <TruncatedText
                        text={d.cluster_name}
                        className={layout.sub}
                      />
                    ) : null}
                  </div>
                  <TruncatedText
                    text={d.channel_name}
                    fallback='—'
                    className={layout.sub}
                  />
                  <NeutralChip>
                    {DELIVERY_EVENTS.includes(d.event_type)
                      ? t(`deliveryEvent.${d.event_type}`)
                      : d.event_type}
                  </NeutralChip>
                  <DeliveryChip
                    status={d.status}
                    label={
                      DELIVERY_STATUS_LABELS.includes(d.status)
                        ? t(`delivery.${d.status}`)
                        : d.status
                    }
                  />
                  <span className={cn(layout.sub, styles.num, styles.nowrap)}>
                    {d.attempt_count}
                  </span>
                  <span className={cn(layout.sub, styles.num, styles.nowrap)}>
                    {d.response_status_code ?? '—'}
                  </span>
                  <span className={cn(layout.sub, styles.num, styles.nowrap)}>
                    {formatAgo(minutesAgo(d.sent_at || d.created_at), t)}
                  </span>
                  {d.last_error ? (
                    <Popover>
                      <PopoverTrigger asChild>
                        <button
                          type='button'
                          className={cn(
                            layout.sub,
                            styles.truncate,
                            styles.errCell,
                          )}
                        >
                          {d.last_error}
                        </button>
                      </PopoverTrigger>
                      <PopoverContent
                        align='end'
                        className={cn(
                          'w-[min(440px,calc(100vw-32px))] rounded-[var(--radius-panel)] p-0',
                          styles.errPop,
                        )}
                      >
                        <div className={styles.errPopHead}>
                          <span>{t('deliveryColError')}</span>
                          <CopyButton
                            variant='plain'
                            value={d.last_error}
                            label={tc('copy')}
                          />
                        </div>
                        <pre className={styles.errPopText}>{d.last_error}</pre>
                      </PopoverContent>
                    </Popover>
                  ) : (
                    <span className={layout.sub}>—</span>
                  )}
                </div>
              ))}
            </div>
          </TableCard>
        </div>
      </div>
    </>
  );
}
