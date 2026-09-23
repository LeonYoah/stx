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

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import {RefreshCw} from 'lucide-react';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationDelivery,
  NotificationDeliveryEventType,
  NotificationDeliveryStatus,
} from '@/lib/services/monitoring';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {Pagination} from '@/components/ui/pagination';
import {Input} from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

type ClusterOption = {
  id: string;
  name: string;
};

function formatDateTime(value?: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

function toRFC3339(value: string): string | undefined {
  if (!value) {
    return undefined;
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return undefined;
  }
  return parsed.toISOString();
}

function resolveStatusVariant(
  status: string,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  const normalized = status.trim().toLowerCase();
  if (normalized === 'failed') {
    return 'destructive';
  }
  if (normalized === 'sent') {
    return 'secondary';
  }
  if (normalized === 'sending' || normalized === 'retrying') {
    return 'default';
  }
  return 'outline';
}

function resolveEventTypeVariant(
  eventType: string,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  const normalized = eventType.trim().toLowerCase();
  if (normalized === 'firing') {
    return 'destructive';
  }
  if (normalized === 'resolved') {
    return 'secondary';
  }
  return 'outline';
}

export function MonitoringNotificationHistoryPanel() {
  const t = useTranslations('monitoringCenter');

  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [clusterOptions, setClusterOptions] = useState<ClusterOption[]>([]);

  const [channelFilter, setChannelFilter] = useState<string>('all');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [eventTypeFilter, setEventTypeFilter] = useState<string>('all');
  const [clusterFilter, setClusterFilter] = useState<string>('all');
  const [startTimeFilter, setStartTimeFilter] = useState<string>('');
  const [endTimeFilter, setEndTimeFilter] = useState<string>('');
  const [page, setPage] = useState<number>(1);
  const [pageSize, setPageSize] = useState<string>('50');

  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [total, setTotal] = useState<number>(0);
  const [loading, setLoading] = useState<boolean>(true);

  const pageSizeNumber = useMemo(
    () => Number.parseInt(pageSize, 10) || 50,
    [pageSize],
  );

  const loadChannels = useCallback(async () => {
    const result = await services.monitoring.listNotificationChannelsSafe();
    if (!result.success || !result.data) {
      setChannels([]);
      return;
    }
    setChannels(result.data.channels || []);
  }, []);

  const loadClusters = useCallback(async () => {
    const healthResult = await services.monitoring.getClustersHealthSafe();
    if (healthResult.success && healthResult.data) {
      const options = (healthResult.data.clusters || [])
        .map((cluster) => ({
          id: String(cluster.cluster_id),
          name: cluster.cluster_name || `Cluster-${cluster.cluster_id}`,
        }))
        .sort((a, b) => Number.parseInt(a.id, 10) - Number.parseInt(b.id, 10));
      setClusterOptions(options);
      return;
    }

    try {
      const data = await services.cluster.getClusters({current: 1, size: 200});
      setClusterOptions(
        (data.clusters || []).map((cluster) => ({
          id: String(cluster.id),
          name: cluster.name,
        })),
      );
    } catch {
      setClusterOptions([]);
    }
  }, []);

  const loadDeliveries = useCallback(async () => {
    setLoading(true);
    try {
      const result = await services.monitoring.listNotificationDeliveriesSafe({
        channel_id:
          channelFilter === 'all'
            ? undefined
            : Number.parseInt(channelFilter, 10),
        status:
          statusFilter === 'all'
            ? undefined
            : (statusFilter as NotificationDeliveryStatus),
        event_type:
          eventTypeFilter === 'all'
            ? undefined
            : (eventTypeFilter as NotificationDeliveryEventType),
        cluster_id: clusterFilter === 'all' ? undefined : clusterFilter,
        start_time: toRFC3339(startTimeFilter),
        end_time: toRFC3339(endTimeFilter),
        page,
        page_size: pageSizeNumber,
      });

      if (!result.success || !result.data) {
        toast.error(result.error || t('history.loadError'));
        setDeliveries([]);
        setTotal(0);
        return;
      }

      setDeliveries(result.data.deliveries || []);
      setTotal(result.data.total || 0);
      if (result.data.page && result.data.page !== page) {
        setPage(result.data.page);
      }
    } finally {
      setLoading(false);
    }
  }, [
    channelFilter,
    statusFilter,
    eventTypeFilter,
    clusterFilter,
    startTimeFilter,
    endTimeFilter,
    page,
    pageSizeNumber,
    t,
  ]);

  useEffect(() => {
    loadChannels();
    loadClusters();
  }, [loadChannels, loadClusters]);

  useEffect(() => {
    loadDeliveries();
  }, [loadDeliveries]);

  const totalPages = useMemo(() => {
    if (total <= 0) {
      return 1;
    }
    return Math.max(1, Math.ceil(total / pageSizeNumber));
  }, [pageSizeNumber, total]);

  const resolveStatusLabel = useCallback(
    (status: string) => {
      const normalized = status.trim().toLowerCase();
      switch (normalized) {
        case 'pending':
          return t('history.statuses.pending');
        case 'sending':
          return t('history.statuses.sending');
        case 'sent':
          return t('history.statuses.sent');
        case 'failed':
          return t('history.statuses.failed');
        case 'retrying':
          return t('history.statuses.retrying');
        case 'canceled':
          return t('history.statuses.canceled');
        default:
          return status || '-';
      }
    },
    [t],
  );

  const resolveEventTypeLabel = useCallback(
    (eventType: string) => {
      const normalized = eventType.trim().toLowerCase();
      switch (normalized) {
        case 'firing':
          return t('history.eventTypes.firing');
        case 'resolved':
          return t('history.eventTypes.resolved');
        case 'test':
          return t('history.eventTypes.test');
        default:
          return eventType || '-';
      }
    },
    [t],
  );

  const renderLastError = (delivery: NotificationDelivery): string => {
    const statusCode = delivery.response_status_code
      ? `HTTP ${delivery.response_status_code}`
      : '';
    const errorText = delivery.last_error || '';
    return [statusCode, errorText].filter(Boolean).join(' · ') || '-';
  };

  return (
    <div className='flex-1 flex flex-col space-y-4'>
      <Card className='flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-270px)]'>
        <CardHeader>
          <CardTitle>{t('history.title')}</CardTitle>
          <div className='flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between'>
            <div className='flex flex-col gap-2 md:flex-row md:flex-wrap'>
              <div className='w-full md:w-56'>
                <Select
                  value={channelFilter}
                  onValueChange={(value) => {
                    setChannelFilter(value);
                    setPage(1);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('history.channelFilter')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('history.allChannels')}
                    </SelectItem>
                    {channels.map((channel) => (
                      <SelectItem key={channel.id} value={String(channel.id)}>
                        {channel.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className='w-full md:w-56'>
                <Select
                  value={statusFilter}
                  onValueChange={(value) => {
                    setStatusFilter(value);
                    setPage(1);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('history.statusFilter')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('history.allStatuses')}
                    </SelectItem>
                    <SelectItem value='pending'>
                      {t('history.statuses.pending')}
                    </SelectItem>
                    <SelectItem value='sending'>
                      {t('history.statuses.sending')}
                    </SelectItem>
                    <SelectItem value='sent'>
                      {t('history.statuses.sent')}
                    </SelectItem>
                    <SelectItem value='failed'>
                      {t('history.statuses.failed')}
                    </SelectItem>
                    <SelectItem value='retrying'>
                      {t('history.statuses.retrying')}
                    </SelectItem>
                    <SelectItem value='canceled'>
                      {t('history.statuses.canceled')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className='w-full md:w-56'>
                <Select
                  value={eventTypeFilter}
                  onValueChange={(value) => {
                    setEventTypeFilter(value);
                    setPage(1);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('history.eventTypeFilter')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('history.allEventTypes')}
                    </SelectItem>
                    <SelectItem value='firing'>
                      {t('history.eventTypes.firing')}
                    </SelectItem>
                    <SelectItem value='resolved'>
                      {t('history.eventTypes.resolved')}
                    </SelectItem>
                    <SelectItem value='test'>
                      {t('history.eventTypes.test')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className='w-full md:w-56'>
                <Select
                  value={clusterFilter}
                  onValueChange={(value) => {
                    setClusterFilter(value);
                    setPage(1);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('history.clusterFilter')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('history.allClusters')}
                    </SelectItem>
                    {clusterOptions.map((cluster) => (
                      <SelectItem key={cluster.id} value={cluster.id}>
                        {cluster.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className='w-full md:w-64'>
                <Input
                  type='datetime-local'
                  value={startTimeFilter}
                  onChange={(event) => {
                    setStartTimeFilter(event.target.value);
                    setPage(1);
                  }}
                />
              </div>

              <div className='w-full md:w-64'>
                <Input
                  type='datetime-local'
                  value={endTimeFilter}
                  onChange={(event) => {
                    setEndTimeFilter(event.target.value);
                    setPage(1);
                  }}
                />
              </div>
            </div>

            <div className='flex flex-wrap gap-2'>
              <Button
                variant='outline'
                onClick={() => {
                  setStartTimeFilter('');
                  setEndTimeFilter('');
                  setPage(1);
                }}
              >
                {t('history.clearTimeFilter')}
              </Button>
              <Button
                variant='outline'
                onClick={loadDeliveries}
                disabled={loading}
              >
                <RefreshCw className='mr-2 h-4 w-4' />
                {t('refresh')}
              </Button>
            </div>
          </div>
        </CardHeader>

        <CardContent className='flex-1 flex flex-col p-0'>
          <div className='overflow-x-auto flex-1'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('history.cluster')}</TableHead>
                  <TableHead>{t('history.alertName')}</TableHead>
                  <TableHead>{t('history.channel')}</TableHead>
                  <TableHead>{t('history.eventType')}</TableHead>
                  <TableHead>{t('history.status')}</TableHead>
                  <TableHead>{t('history.attempts')}</TableHead>
                  <TableHead>{t('history.lastError')}</TableHead>
                  <TableHead>{t('history.sentAt')}</TableHead>
                  <TableHead>{t('history.createdAt')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <TableCell
                      colSpan={9}
                      className='text-center text-muted-foreground'
                    >
                      {t('loading')}
                    </TableCell>
                  </TableRow>
                ) : !deliveries.length ? (
                  <TableRow>
                    <TableCell
                      colSpan={9}
                      className='text-center text-muted-foreground'
                    >
                      {t('history.empty')}
                    </TableCell>
                  </TableRow>
                ) : (
                  deliveries.map((delivery) => (
                    <TableRow key={delivery.id}>
                      <TableCell>
                        {delivery.cluster_name || delivery.cluster_id || '-'}
                      </TableCell>
                      <TableCell>
                        {delivery.alert_name || delivery.alert_id}
                      </TableCell>
                      <TableCell>
                        {delivery.channel_name || delivery.channel_id}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={resolveEventTypeVariant(delivery.event_type)}
                        >
                          {resolveEventTypeLabel(delivery.event_type)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant={resolveStatusVariant(delivery.status)}>
                          {resolveStatusLabel(delivery.status)}
                        </Badge>
                      </TableCell>
                      <TableCell>{delivery.attempt_count || 0}</TableCell>
                      <TableCell
                        className='max-w-[320px] truncate'
                        title={renderLastError(delivery)}
                      >
                        {renderLastError(delivery)}
                      </TableCell>
                      <TableCell>{formatDateTime(delivery.sent_at)}</TableCell>
                      <TableCell>{formatDateTime(delivery.created_at)}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          {/* 底部分页栏 / Table Footer Pagination */}
          <div className='border-t bg-muted/10 px-4 py-2.5 mt-auto'>
            <Pagination
              currentPage={page}
              totalPages={totalPages}
              pageSize={pageSizeNumber}
              totalItems={total}
              onPageChange={setPage}
              onPageSizeChange={(newSize) => {
                setPageSize(String(newSize));
                setPage(1);
              }}
              showPageSizeSelector={true}
              pageSizeOptions={[20, 50, 100, 200]}
            />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
