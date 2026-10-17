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

/**
 * ProcessEventList - 进程事件列表组件
 * Displays process lifecycle events for a cluster.
 * 显示集群的进程生命周期事件。
 * Requirements: 6.4
 */

'use client';

import { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
import { Pagination } from '@/components/ui/pagination';
import { Loader2, RefreshCw, Activity, AlertTriangle, XCircle, Play, Square } from 'lucide-react';
import { toast } from 'sonner';
import {
  ProcessEvent,
  ProcessEventType,
  ProcessEventFilter,
  listProcessEvents,
} from '@/lib/services/monitor';
import {formatDateTimeTz} from '@/lib/timezone';

interface ProcessEventListProps {
  clusterId: number;
  nodeId?: number;
}

// Event type badge config / 事件类型徽章配置
const eventTypeBadgeConfig: Record<
  ProcessEventType,
  { variant: 'default' | 'secondary' | 'destructive' | 'outline'; icon: React.ReactNode; label: string }
> = {
  started: { variant: 'default', icon: <Play className="h-3 w-3" />, label: 'monitor.events.started' },
  stopped: { variant: 'secondary', icon: <Square className="h-3 w-3" />, label: 'monitor.events.stopped' },
  crashed: { variant: 'destructive', icon: <XCircle className="h-3 w-3" />, label: 'monitor.events.crashed' },
  restarted: { variant: 'default', icon: <RefreshCw className="h-3 w-3" />, label: 'monitor.events.restarted' },
  restart_failed: { variant: 'destructive', icon: <AlertTriangle className="h-3 w-3" />, label: 'monitor.events.restartFailed' },
  restart_limit_reached: { variant: 'destructive', icon: <AlertTriangle className="h-3 w-3" />, label: 'monitor.events.restartLimitReached' },
  cluster_restart_requested: { variant: 'outline', icon: <RefreshCw className="h-3 w-3" />, label: 'monitor.events.clusterRestartRequested' },
  node_restart_requested: { variant: 'outline', icon: <RefreshCw className="h-3 w-3" />, label: 'monitor.events.nodeRestartRequested' },
  node_stop_requested: { variant: 'secondary', icon: <Square className="h-3 w-3" />, label: 'monitor.events.nodeStopRequested' },
  node_offline: { variant: 'destructive', icon: <AlertTriangle className="h-3 w-3" />, label: 'monitor.events.nodeOffline' },
  node_recovered: { variant: 'default', icon: <Activity className="h-3 w-3" />, label: 'monitor.events.nodeRecovered' },
};

export function ProcessEventList({ clusterId, nodeId }: ProcessEventListProps) {
  const t = useTranslations();
  const [events, setEvents] = useState<ProcessEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [eventTypeFilter, setEventTypeFilter] = useState<ProcessEventType | 'all'>('all');

  // Load events / 加载事件
  const loadEvents = useCallback(async () => {
    try {
      setLoading(true);
      const filter: ProcessEventFilter = {
        page,
        page_size: pageSize,
      };
      if (nodeId) {
        filter.node_id = nodeId;
      }
      if (eventTypeFilter !== 'all') {
        filter.event_type = eventTypeFilter;
      }

      const response = await listProcessEvents(clusterId, filter);
      setEvents(response.events || []);
      setTotal(response.total);
    } catch (error) {
      console.error('Failed to load process events:', error);
      toast.error(t('monitor.loadEventsError'));
    } finally {
      setLoading(false);
    }
  }, [clusterId, nodeId, page, pageSize, eventTypeFilter, t]);

  useEffect(() => {
    loadEvents();
  }, [loadEvents]);

  // Render event type badge / 渲染事件类型徽章
  const renderEventTypeBadge = (eventType: ProcessEventType) => {
    const config = eventTypeBadgeConfig[eventType];
    if (!config) {
      return <Badge variant="outline">{eventType}</Badge>;
    }
    return (
      <Badge variant={config.variant} className="flex items-center gap-1">
        {config.icon}
        {t(config.label)}
      </Badge>
    );
  };

  // Format time / 格式化时间
  const formatTime = (timeStr: string) => {
    return formatDateTimeTz(timeStr);
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Activity className="h-5 w-5" />
            <CardTitle>{t('monitor.eventsTitle')}</CardTitle>
          </div>
          <div className="flex items-center gap-2">
            <Select
              value={eventTypeFilter}
              onValueChange={(value) => {
                setEventTypeFilter(value as ProcessEventType | 'all');
                setPage(1);
              }}
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder={t('monitor.filterByType')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('monitor.allEvents')}</SelectItem>
                <SelectItem value="started">{t('monitor.events.started')}</SelectItem>
                <SelectItem value="stopped">{t('monitor.events.stopped')}</SelectItem>
                <SelectItem value="crashed">{t('monitor.events.crashed')}</SelectItem>
                <SelectItem value="restarted">{t('monitor.events.restarted')}</SelectItem>
                <SelectItem value="restart_failed">{t('monitor.events.restartFailed')}</SelectItem>
                <SelectItem value="restart_limit_reached">{t('monitor.events.restartLimitReached')}</SelectItem>
                <SelectItem value="cluster_restart_requested">{t('monitor.events.clusterRestartRequested')}</SelectItem>
                <SelectItem value="node_restart_requested">{t('monitor.events.nodeRestartRequested')}</SelectItem>
                <SelectItem value="node_stop_requested">{t('monitor.events.nodeStopRequested')}</SelectItem>
                <SelectItem value="node_offline">{t('monitor.events.nodeOffline')}</SelectItem>
                <SelectItem value="node_recovered">{t('monitor.events.nodeRecovered')}</SelectItem>
              </SelectContent>
            </Select>
            <Button variant="outline" size="icon" onClick={loadEvents} disabled={loading}>
              <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </div>
        <CardDescription>
          {t('monitor.eventsDescription', { total })}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {loading && events.length === 0 ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : events.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <Activity className="h-12 w-12 mb-2 opacity-50" />
            <p>{t('monitor.noEvents')}</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('monitor.eventType')}</TableHead>
                  <TableHead>{t('monitor.hostname')}</TableHead>
                  <TableHead>{t('monitor.ip')}</TableHead>
                  <TableHead>{t('monitor.processName')}</TableHead>
                  <TableHead>{t('monitor.pid')}</TableHead>
                  <TableHead>{t('monitor.role')}</TableHead>
                  <TableHead>{t('monitor.eventTime')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell>{renderEventTypeBadge(event.event_type)}</TableCell>
                    <TableCell className="text-sm">{event.hostname || '-'}</TableCell>
                    <TableCell className="font-mono text-sm">{event.ip || '-'}</TableCell>
                    <TableCell className="font-mono text-sm">{event.process_name || '-'}</TableCell>
                    <TableCell className="font-mono">{event.pid || '-'}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{event.role || '-'}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatTime(event.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            {/* Pagination / 分页 */}
            <div className="mt-4 pt-4 border-t">
              <Pagination
                currentPage={page}
                totalPages={Math.ceil(total / pageSize)}
                pageSize={pageSize}
                totalItems={total}
                onPageChange={setPage}
                showPageSizeSelector={false}
                showTotalItems={true}
              />
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
