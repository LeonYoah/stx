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
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Button} from '@/components/arc/button/button';
import {
  listContainerImages,
  resolveImageSeatunnelVersion,
  type ContainerImage,
} from '@/lib/services/installer/image.service';

export interface K8sImagePickerValue {
  image: string;
  seatunnelVersion: string;
  pullPolicy: string;
  pullSecrets: string;
}

interface K8sImagePickerProps {
  value: string;
  onChange: (value: K8sImagePickerValue) => void;
  id?: string;
}

const CUSTOM_KEY = '__custom__';

// K8sImagePicker 通用容器镜像选择器：注册镜像 / 官方预设 / 自定义输入三段，
// 版本识别走 resolveImageSeatunnelVersion 三级逻辑。
// K8sImagePicker is the shared container image picker: registered images,
// official presets, or custom input, with three-tier version resolution.
export function K8sImagePicker({value, onChange, id}: K8sImagePickerProps) {
  const t = useTranslations('k8s.imagePicker');
  const [items, setItems] = useState<ContainerImage[]>([]);
  const [presets, setPresets] = useState<ContainerImage[]>([]);
  const [customMode, setCustomMode] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // 镜像清单加载失败必须显式可重试，禁止静默吞错留下永久空清单。
  // Image list load failures surface a retry action; never swallow errors silently.
  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    listContainerImages()
      .then((result) => {
        setItems(result.items || []);
        setPresets(result.official_presets || []);
      })
      .catch((err) => {
        setLoadError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const options = useMemo(() => {
    const all: {key: string; label: string; image: ContainerImage}[] = [];
    for (const item of items) {
      all.push({
        key: `reg-${item.id}`,
        label: item.is_default
          ? `${item.name}（${item.image}）· ${t('defaultBadge')}`
          : `${item.name}（${item.image}）`,
        image: item,
      });
    }
    for (const preset of presets) {
      // 注册表已覆盖同名镜像时不重复展示官方预设。
      // Skip presets already covered by a registered entry.
      if (items.some((item) => item.image === preset.image)) {continue;}
      all.push({key: `preset-${preset.image}`, label: preset.image, image: preset});
    }
    return all;
  }, [items, presets, t]);

  const selectedKey = useMemo(() => {
    const hit = options.find((option) => option.image.image === value);
    return hit ? hit.key : '';
  }, [options, value]);

  const emit = (imageRef: string, source?: ContainerImage) => {
    onChange({
      image: imageRef,
      seatunnelVersion: resolveImageSeatunnelVersion(imageRef, items),
      pullPolicy: source?.image_pull_policy || 'IfNotPresent',
      pullSecrets: source?.image_pull_secrets || '',
    });
  };

  const recognized = resolveImageSeatunnelVersion(value, items);

  return (
    <div className='space-y-2' data-testid='k8s-image-picker'>
      {loading ? (
        <Skeleton lines={1} className='h-9' label={t('placeholder')} />
      ) : loadError ? (
        <div className='flex items-center gap-2'>
          <p className='min-w-0 flex-1 truncate text-xs text-destructive'>{loadError}</p>
          <Button type='button' variant='secondary' size='sm' onClick={load}>
            {t('retry')}
          </Button>
        </div>
      ) : !customMode ? (
        <Select
          aria-label={t('placeholder')}
          value={selectedKey}
          placeholder={t('placeholder')}
          options={[
            ...options.map((option) => ({value: option.key, label: option.label})),
            {value: CUSTOM_KEY, label: t('customImage')},
          ]}
          onValueChange={(key) => {
            if (key === CUSTOM_KEY) {
              setCustomMode(true);
              return;
            }
            const option = options.find((entry) => entry.key === key);
            if (option) {emit(option.image.image, option.image);}
          }}
          data-testid='k8s-image-picker-select'
        />
      ) : (
        <div className='flex items-center gap-2'>
          <Input
            id={id}
            label={t('customImage')}
            data-testid='k8s-image-picker-input'
            value={value}
            placeholder='apache/seatunnel:2.3.13'
            onChange={(event) => emit(event.target.value)}
          />
          <button
            type='button'
            className='shrink-0 text-xs text-primary underline-offset-2 hover:underline'
            onClick={() => setCustomMode(false)}
          >
            {t('backToList')}
          </button>
        </div>
      )}
      {value ? (
        <p className='text-xs text-muted-foreground' data-testid='k8s-image-picker-version'>
          {recognized
            ? t('versionRecognized', {version: recognized})
            : t('versionUnrecognized')}
        </p>
      ) : null}
    </div>
  );
}
