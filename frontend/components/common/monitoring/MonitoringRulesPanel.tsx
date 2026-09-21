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

import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import services from '@/lib/services';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {AlertRule, AlertSeverity} from '@/lib/services/monitoring';
import {
  resolvePreferredClusterId,
  rememberPreferredClusterId,
  isSoleDefaultCluster,
} from '@/lib/cluster-preference';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Input} from '@/components/ui/input';
import {Switch} from '@/components/ui/switch';
import {Button} from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

type MonitoringRulesPanelProps = {
  embedded?: boolean;
};

export function MonitoringRulesPanel({
  embedded = false,
}: MonitoringRulesPanelProps) {
  const t = useTranslations('monitoringCenter');
  const tCluster = useTranslations('cluster');

  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [selectedClusterId, setSelectedClusterId] = useState<string>('');

  const [rules, setRules] = useState<AlertRule[]>([]);
  const [loading, setLoading] = useState<boolean>(false);
  const [savingRuleId, setSavingRuleId] = useState<number | null>(null);

  const loadClusters = useCallback(async () => {
    try {
      const data = await services.cluster.getClusters({current: 1, size: 200});
      const list = data.clusters || [];
      setClusters(list);
      if (!selectedClusterId && list.length > 0) {
        const preferred = resolvePreferredClusterId(list, {
          fallbackToFirst: true,
        });
        if (preferred != null) {
          setSelectedClusterId(String(preferred));
        }
      }
    } catch {
      setClusters([]);
    }
  }, [selectedClusterId]);

  const loadRules = useCallback(async () => {
    if (!selectedClusterId) {
      setRules([]);
      return;
    }

    setLoading(true);
    try {
      const result = await services.monitoring.getClusterRulesSafe(
        Number.parseInt(selectedClusterId, 10),
      );
      if (!result.success || !result.data) {
        toast.error(result.error || t('rules.loadError'));
        setRules([]);
        return;
      }
      setRules(result.data.rules || []);
    } finally {
      setLoading(false);
    }
  }, [selectedClusterId, t]);

  useEffect(() => {
    loadClusters();
  }, [loadClusters]);

  useEffect(() => {
    loadRules();
  }, [loadRules]);

  const patchRule = (ruleId: number, patch: Partial<AlertRule>) => {
    setRules((prev) =>
      prev.map((rule) => (rule.id === ruleId ? {...rule, ...patch} : rule)),
    );
  };

  const handleSaveRule = async (rule: AlertRule) => {
    if (!selectedClusterId) {
      return;
    }

    setSavingRuleId(rule.id);
    try {
      const result = await services.monitoring.updateClusterRuleSafe(
        Number.parseInt(selectedClusterId, 10),
        rule.id,
        {
          severity: rule.severity,
          enabled: rule.enabled,
          threshold: Number(rule.threshold),
          window_seconds: Number(rule.window_seconds),
        },
      );

      if (!result.success) {
        toast.error(result.error || t('rules.saveError'));
        return;
      }

      toast.success(t('rules.saveSuccess'));
    } finally {
      setSavingRuleId(null);
    }
  };

  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader className='flex flex-col gap-4 md:flex-row md:items-start md:justify-between'>
          <div className='space-y-1'>
            <CardTitle>
              {embedded
                ? t('policyCenter.legacyRulesCardTitle')
                : t('rules.title')}
            </CardTitle>
            {embedded ? (
              <p className='text-sm text-muted-foreground'>
                {t('policyCenter.legacyRulesCardSubtitle')}
              </p>
            ) : null}
          </div>
          <div className='w-full md:w-72'>
            <Select
              value={selectedClusterId}
              onValueChange={(value) => {
                setSelectedClusterId(value);
                rememberPreferredClusterId(value);
              }}
            >
              <SelectTrigger>
                <SelectValue placeholder={t('rules.clusterSelect')} />
              </SelectTrigger>
              <SelectContent>
                {clusters.map((cluster) => (
                  <SelectItem key={cluster.id} value={String(cluster.id)}>
                    {cluster.name}
                    {isSoleDefaultCluster(clusters, cluster.id)
                      ? ` · ${tCluster('defaultBadge')}`
                      : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardHeader>
        <CardContent>
          {!selectedClusterId ? (
            <div className='text-sm text-muted-foreground'>
              {t('rules.selectClusterHint')}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('rules.ruleName')}</TableHead>
                  <TableHead>{t('rules.description')}</TableHead>
                  <TableHead>{t('rules.severity')}</TableHead>
                  <TableHead>{t('rules.enabled')}</TableHead>
                  <TableHead>{t('rules.threshold')}</TableHead>
                  <TableHead>{t('rules.windowSeconds')}</TableHead>
                  <TableHead>{t('actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <TableCell
                      colSpan={7}
                      className='text-center text-muted-foreground'
                    >
                      {t('loading')}
                    </TableCell>
                  </TableRow>
                ) : !rules.length ? (
                  <TableRow>
                    <TableCell
                      colSpan={7}
                      className='text-center text-muted-foreground'
                    >
                      {t('rules.noRules')}
                    </TableCell>
                  </TableRow>
                ) : (
                  rules.map((rule) => (
                    <TableRow key={rule.id}>
                      <TableCell className='font-medium'>
                        {rule.rule_name}
                      </TableCell>
                      <TableCell className='max-w-[340px] break-all'>
                        {rule.description}
                      </TableCell>
                      <TableCell>
                        <Select
                          value={rule.severity}
                          onValueChange={(value) =>
                            patchRule(rule.id, {
                              severity: value as AlertSeverity,
                            })
                          }
                        >
                          <SelectTrigger className='w-[130px]'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value='warning'>
                              {t('alertSeverity.warning')}
                            </SelectItem>
                            <SelectItem value='critical'>
                              {t('alertSeverity.critical')}
                            </SelectItem>
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell>
                        <Switch
                          checked={rule.enabled}
                          onCheckedChange={(checked) =>
                            patchRule(rule.id, {enabled: checked})
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          type='number'
                          min={0}
                          value={rule.threshold}
                          onChange={(event) =>
                            patchRule(rule.id, {
                              threshold: Number(event.target.value),
                            })
                          }
                          className='w-[120px]'
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          type='number'
                          min={1}
                          value={rule.window_seconds}
                          onChange={(event) =>
                            patchRule(rule.id, {
                              window_seconds: Number(event.target.value),
                            })
                          }
                          className='w-[120px]'
                        />
                      </TableCell>
                      <TableCell>
                        <Button
                          size='sm'
                          onClick={() => handleSaveRule(rule)}
                          disabled={savingRuleId === rule.id}
                        >
                          {t('rules.save')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
