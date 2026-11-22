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
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {SoftChip} from '@/components/common/layout';
import services from '@/lib/services';
import type {JavaRuntimeCandidate} from '@/lib/services/installer/types';
import {cn} from '@/lib/utils';

const CUSTOM_VALUE = '__custom__';

export interface JavaHomePickerValue {
  /** Path submitted to the API / 提交给 API 的具体 JDK 路径 */
  javaHome: string;
  /** Whether the current value is a custom typed path / 当前值是否为用户自填路径 */
  isCustom: boolean;
  /** True when populated automatically from probe/process rather than a manual user click / 是否由底层探测自动预选（非用户手动切换） */
  isAutoPrefill?: boolean;
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
  /** Hide the field label when the parent already names the control / 父级已有标题时隐藏字段标签 */
  hideLabel?: boolean;
  /** Deprecated: auto-detect is no longer offered as an option globally / 已废弃：全局不再将「自动探测」作为下拉选项 */
  hideAuto?: boolean;
  /**
   * 列表探测完成且当前无值时，是否自动预选首个探测到的具体 JDK 供用户确认（默认 true）。
   * When true (default), auto-select the first probed concrete JDK for user confirmation if value is empty.
   */
  autoSelectFirst?: boolean;
}

/**
 * JDK 路径选择器：底层自动探测主机可用 JDK 列表供用户确认与选择，不再提供模糊的「自动探测」选项，并始终保留自定义路径入口。
 * JDK path picker: probes available host JDKs internally for explicit user confirmation/selection (no ambiguous "auto" option) and always offers custom path entry.
 */
export function JavaHomePicker({
  hostId,
  value,
  onChange,
  preferredJavaHome,
  disabled,
  className,
  id,
  hideLabel,
  autoSelectFirst = true,
}: JavaHomePickerProps) {
  const t = useTranslations();
  const [candidates, setCandidates] = useState<JavaRuntimeCandidate[]>([]);
  const [loading, setLoading] = useState(false);
  const [customMode, setCustomMode] = useState(false);
  const [appliedInitial, setAppliedInitial] = useState(false);
  const [listLoaded, setListLoaded] = useState(false);
  const [loadError, setLoadError] = useState(false);

  useEffect(() => {
    setAppliedInitial(false);
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

  // 探测列表就绪后按优先级预填具体 JDK 路径（进程实测 JDK > 已有值 > 首个探测候选），让用户明确确认或切换。
  // Once probed, prefill a concrete JDK path in priority order (process JDK > existing value > first detected candidate) for explicit confirmation.
  useEffect(() => {
    if (appliedInitial || !listLoaded || loading) {
      return;
    }
    const preferred = (preferredJavaHome || '').trim();
    if (preferred) {
      const inList = candidates.some((c) => c.java_home === preferred);
      setCustomMode(!inList);
      if (value.trim() !== preferred) {
        onChange({javaHome: preferred, isCustom: !inList, isAutoPrefill: true});
      }
      setAppliedInitial(true);
      return;
    }
    if (value.trim()) {
      const inList = candidates.some((c) => c.java_home === value.trim());
      setCustomMode(!inList);
      setAppliedInitial(true);
      return;
    }
    if (autoSelectFirst && candidates.length > 0) {
      setCustomMode(false);
      onChange({
        javaHome: candidates[0].java_home,
        isCustom: false,
        isAutoPrefill: true,
      });
      setAppliedInitial(true);
      return;
    }
    if (candidates.length === 0) {
      // 探测列表为空时进入自定义填写，不阻塞（AC1b）/ Empty list → custom entry without blocking (AC1b)
      setCustomMode(true);
    }
    setAppliedInitial(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- apply initial selection once per host/list load
  }, [appliedInitial, listLoaded, loading, preferredJavaHome, candidates, autoSelectFirst]);

  useEffect(() => {
    if (!value || !listLoaded) {
      return;
    }
    // Sync custom vs detected after the list loads / 列表加载后同步自定义与探测选中状态
    const inList = candidates.some((c) => c.java_home === value);
    setCustomMode(!inList);
  }, [value, candidates, listLoaded]);

  const selectValue = useMemo(() => {
    if (customMode) {
      return CUSTOM_VALUE;
    }
    if (!value) {
      return '';
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
      : '';

  const selectOptions = useMemo(
    () => [
      ...candidates.map((c) => ({
        value: c.java_home,
        label: t('javaHome.optionDetected', {
          path: c.java_home,
          version: c.version || t('javaHome.versionUnknown'),
        }),
      })),
      {value: CUSTOM_VALUE, label: t('javaHome.optionCustom')},
    ],
    [candidates, t],
  );

  const renderBadge = () => {
    if (!badgeLabel) {
      return null;
    }
    if (customMode) {
      return <SoftChip tone='blue'>{badgeLabel}</SoftChip>;
    }
    return <SoftChip tone='emerald'>{badgeLabel}</SoftChip>;
  };

  return (
    <div className={cn('space-y-2', className)}>
      {hideLabel ? (
        badgeLabel || loading ? (
          <div className='flex items-center gap-2'>
            {renderBadge()}
            {loading && (
              <Loader2 className='h-3 w-3 animate-spin text-muted-foreground' />
            )}
          </div>
        ) : null
      ) : (
        <div className='flex items-center gap-2'>
          <label
            htmlFor={id}
            className='text-xs font-medium text-foreground'
          >
            {t('javaHome.label')}
          </label>
          <HelpHint label={t('javaHome.label')} content={t('javaHome.help')} />
          {renderBadge()}
          {loading && (
            <Loader2 className='h-3 w-3 animate-spin text-muted-foreground' />
          )}
        </div>
      )}
      <Select
        id={id}
        aria-label={t('javaHome.label')}
        className='font-mono text-xs'
        disabled={disabled || loading}
        value={selectValue}
        placeholder={t('javaHome.placeholder')}
        options={selectOptions}
        onValueChange={(next) => {
          if (next === CUSTOM_VALUE) {
            setCustomMode(true);
            onChange({javaHome: value || '', isCustom: true});
            return;
          }
          setCustomMode(false);
          onChange({javaHome: next, isCustom: false});
        }}
      />
      {customMode && (
        <Input
          label={t('javaHome.optionCustom')}
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
        <p
          className='text-[11px] text-amber-700 dark:text-amber-300'
          role='status'
        >
          {t('javaHome.loadFailed')}
        </p>
      )}
    </div>
  );
}

export default JavaHomePicker;
