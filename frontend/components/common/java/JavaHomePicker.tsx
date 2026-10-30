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
import {Loader2} from 'lucide-react';

import {HelpHint} from '@/components/common/HelpHint';
import {Badge} from '@/components/ui/badge';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import services from '@/lib/services';
import type {JavaRuntimeCandidate} from '@/lib/services/installer/types';
import {cn} from '@/lib/utils';

const CUSTOM_VALUE = '__custom__';
const AUTO_VALUE = '__auto__';

export interface JavaHomePickerValue {
  /** Path submitted to the API; empty means auto detection / 提交给 API 的路径，空表示自动探测 */
  javaHome: string;
  /** Whether the current value is a custom typed path / 当前值是否为用户自填路径 */
  isCustom: boolean;
}

interface JavaHomePickerProps {
  hostId?: number | null;
  value: string;
  onChange: (next: JavaHomePickerValue) => void;
  /** Prefill from a running process JDK (AC1f); applied once when list loads / 纳管时预选进程 JDK（AC1f） */
  preferredJavaHome?: string;
  disabled?: boolean;
  className?: string;
  id?: string;
}

/**
 * JDK path picker: loads detectable JDKs for a host as a select list, always offers custom entry (AC1b).
 * JDK 路径选择器：加载主机可探测 JDK 作下拉，并始终提供自定义入口（AC1b）。
 */
export function JavaHomePicker({
  hostId,
  value,
  onChange,
  preferredJavaHome,
  disabled,
  className,
  id,
}: JavaHomePickerProps) {
  const t = useTranslations();
  const [candidates, setCandidates] = useState<JavaRuntimeCandidate[]>([]);
  const [loading, setLoading] = useState(false);
  const [customMode, setCustomMode] = useState(false);
  const [appliedPreferred, setAppliedPreferred] = useState(false);
  const [listLoaded, setListLoaded] = useState(false);
  const [loadError, setLoadError] = useState(false);

  useEffect(() => {
    setAppliedPreferred(false);
    setCustomMode(false);
    setCandidates([]);
    setListLoaded(false);
    setLoadError(false);
    if (!hostId) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    void services.installer
      .listJavaRuntimes(hostId)
      .then((list) => {
        if (!cancelled) {
          setCandidates(list || []);
          setLoadError(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setCandidates([]);
          setLoadError(true);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
          setListLoaded(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [hostId]);

  useEffect(() => {
    if (appliedPreferred || loading) {
      return;
    }
    const preferred = (preferredJavaHome || '').trim();
    if (!preferred) {
      setAppliedPreferred(true);
      return;
    }
    const inList = candidates.some((c) => c.java_home === preferred);
    setCustomMode(!inList);
    onChange({javaHome: preferred, isCustom: !inList});
    setAppliedPreferred(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- apply preferred once per host/list load
  }, [appliedPreferred, loading, preferredJavaHome, candidates]);

  useEffect(() => {
    if (!value) {
      return;
    }
    // Sync custom vs detected after the list loads / 列表加载后同步自定义与探测选中状态
    const inList = candidates.some((c) => c.java_home === value);
    setCustomMode(!inList);
  }, [value, candidates]);

  // 探测列表为空时进入自定义填写，不阻塞（AC1b）/ Empty list → custom entry without blocking (AC1b)
  useEffect(() => {
    if (!listLoaded || !hostId) {
      return;
    }
    if (candidates.length === 0 && !value) {
      setCustomMode(true);
    }
  }, [listLoaded, hostId, candidates.length, value]);

  const selectValue = useMemo(() => {
    if (customMode) {
      return CUSTOM_VALUE;
    }
    if (!value) {
      return AUTO_VALUE;
    }
    if (candidates.some((c) => c.java_home === value)) {
      return value;
    }
    return CUSTOM_VALUE;
  }, [customMode, value, candidates]);

  const badgeLabel = customMode
    ? t('javaHome.badgeCustom')
    : value
      ? t('javaHome.badgeDetected')
      : t('javaHome.badgeAuto');

  return (
    <div className={cn('space-y-2', className)}>
      <div className='flex items-center gap-2'>
        <Label htmlFor={id} className='text-xs'>
          {t('javaHome.label')}
        </Label>
        <HelpHint label={t('javaHome.label')} content={t('javaHome.help')} />
        <Badge variant='outline' className='h-5 text-[10px]'>
          {badgeLabel}
        </Badge>
        {loading && <Loader2 className='h-3 w-3 animate-spin text-muted-foreground' />}
      </div>
      <Select
        disabled={disabled || loading}
        value={selectValue}
        onValueChange={(next) => {
          if (next === CUSTOM_VALUE) {
            setCustomMode(true);
            onChange({javaHome: value || '', isCustom: true});
            return;
          }
          setCustomMode(false);
          if (next === AUTO_VALUE) {
            onChange({javaHome: '', isCustom: false});
            return;
          }
          onChange({javaHome: next, isCustom: false});
        }}
      >
        <SelectTrigger id={id} className='font-mono text-xs'>
          <SelectValue placeholder={t('javaHome.placeholder')} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={AUTO_VALUE}>{t('javaHome.optionAuto')}</SelectItem>
          {candidates.map((c) => (
            <SelectItem key={c.java_home} value={c.java_home}>
              {t('javaHome.optionDetected', {
                path: c.java_home,
                version: c.version || t('javaHome.versionUnknown'),
              })}
            </SelectItem>
          ))}
          <SelectItem value={CUSTOM_VALUE}>{t('javaHome.optionCustom')}</SelectItem>
        </SelectContent>
      </Select>
      {customMode && (
        <Input
          className='font-mono text-xs'
          value={value}
          disabled={disabled}
          placeholder={t('javaHome.customPlaceholder')}
          onChange={(e) =>
            onChange({javaHome: e.target.value.trim(), isCustom: true})
          }
        />
      )}
      {loadError && (
        <p className='text-[11px] text-amber-700 dark:text-amber-300' role='status'>
          {t('javaHome.loadFailed')}
        </p>
      )}
    </div>
  );
}

export default JavaHomePicker;
