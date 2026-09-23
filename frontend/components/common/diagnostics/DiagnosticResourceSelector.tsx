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

import {useCallback, useEffect, useRef, useState} from 'react';
import {Loader2, ShieldAlert} from 'lucide-react';
import {useTranslations} from 'next-intl';
import services from '@/lib/services';
import {useAuth} from '@/hooks/use-auth';
import {cn} from '@/lib/utils';
import type {
  DiagnosticsResourceCode,
  DiagnosticsResourceDefinition,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/ui/checkbox';
import {localizeDiagnosticsText} from './text-utils';

type DiagnosticResourceSelectorProps = {
  selectedResources: DiagnosticsResourceCode[];
  onChange: (resources: DiagnosticsResourceCode[]) => void;
  disabled?: boolean;
  className?: string;
};

/**
 * 展示服务端登记的诊断资源，并从同一份目录读取默认选择和风险说明。
 * Render server-registered diagnostics resources with shared defaults and impact descriptions.
 */
export function DiagnosticResourceSelector({
  selectedResources,
  onChange,
  disabled = false,
  className,
}: DiagnosticResourceSelectorProps) {
  const t = useTranslations('diagnosticsCenter.resourceSelector');
  const {user} = useAuth();
  const [resources, setResources] = useState<DiagnosticsResourceDefinition[]>(
    [],
  );
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const defaultsAppliedRef = useRef(false);

  useEffect(() => {
    let active = true;
    const loadResources = async () => {
      setLoading(true);
      const result = await services.diagnostics.listResourcesSafe();
      if (!active) {
        return;
      }
      if (!result.success || !result.data) {
        setLoadError(result.error || t('loadError'));
        setResources([]);
      } else {
        setLoadError('');
        setResources(result.data.filter((resource) => resource.bundle_allowed));
      }
      setLoading(false);
    };
    void loadResources();
    return () => {
      active = false;
    };
  }, [t]);

  useEffect(() => {
    if (
      defaultsAppliedRef.current ||
      resources.length === 0 ||
      selectedResources.length > 0
    ) {
      return;
    }
    defaultsAppliedRef.current = true;
    onChange(
      resources
        .filter((resource) => resource.default_selected)
        .map((resource) => resource.code),
    );
  }, [onChange, resources, selectedResources.length]);

  const toggleResource = useCallback(
    (resource: DiagnosticsResourceDefinition, checked: boolean) => {
      if (disabled || (resource.admin_only && !user?.is_admin)) {
        return;
      }
      if (checked) {
        onChange([...new Set([...selectedResources, resource.code])]);
        return;
      }
      onChange(selectedResources.filter((code) => code !== resource.code));
    },
    [disabled, onChange, selectedResources, user?.is_admin],
  );

  if (loading) {
    return (
      <div className='flex min-h-24 items-center justify-center rounded-lg border bg-muted/10 text-xs text-muted-foreground'>
        <Loader2 className='mr-2 h-4 w-4 animate-spin' />
        {t('loading')}
      </div>
    );
  }

  if (loadError) {
    return (
      <div className='rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive'>
        {loadError}
      </div>
    );
  }

  return (
    <div className={cn('space-y-2', className)}>
      <div className='flex items-center justify-between gap-3 text-xs text-muted-foreground'>
        <span>{t('hint')}</span>
        <span className='whitespace-nowrap'>
          {t('selectedCount', {count: selectedResources.length})}
        </span>
      </div>
      <div className='grid gap-2 sm:grid-cols-2'>
        {resources.map((resource) => {
          const checked = selectedResources.includes(resource.code);
          const adminLocked = resource.admin_only && !user?.is_admin;
          return (
            <label
              key={resource.code}
              className={cn(
                'flex items-start gap-2.5 rounded-lg border p-3 transition-colors',
                checked && 'border-primary/50 bg-primary/5',
                (disabled || adminLocked) && 'cursor-not-allowed opacity-60',
                !disabled && !adminLocked && 'cursor-pointer hover:bg-muted/30',
              )}
            >
              <Checkbox
                checked={checked}
                disabled={disabled || adminLocked}
                onCheckedChange={(value) =>
                  toggleResource(resource, value === true)
                }
                className='mt-0.5'
              />
              <span className='min-w-0 flex-1 space-y-1'>
                <span className='flex flex-wrap items-center gap-1.5'>
                  <span className='text-xs font-medium text-foreground'>
                    {localizeDiagnosticsText(resource.title)}
                  </span>
                  <Badge variant='outline' className='h-5 px-1.5 text-[10px]'>
                    {resource.risk}
                  </Badge>
                  {resource.admin_only ? (
                    <Badge
                      variant='destructive'
                      className='h-5 px-1.5 text-[10px]'
                    >
                      <ShieldAlert className='mr-1 h-3 w-3' />
                      {t('adminOnly')}
                    </Badge>
                  ) : null}
                </span>
                <span className='block text-[11px] leading-4 text-muted-foreground'>
                  {localizeDiagnosticsText(resource.description)}
                </span>
                <span className='block text-[11px] leading-4 text-amber-700 dark:text-amber-300'>
                  {localizeDiagnosticsText(resource.impact)}
                </span>
              </span>
            </label>
          );
        })}
      </div>
    </div>
  );
}
