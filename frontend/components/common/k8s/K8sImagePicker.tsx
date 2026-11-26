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
import {Input} from '@/components/ui/input';
import {Badge} from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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

  useEffect(() => {
    let cancelled = false;
    listContainerImages()
      .then((result) => {
        if (cancelled) return;
        setItems(result.items || []);
        setPresets(result.official_presets || []);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  const options = useMemo(() => {
    const all: {key: string; label: string; image: ContainerImage}[] = [];
    for (const item of items) {
      all.push({
        key: `reg-${item.id}`,
        label: `${item.name}（${item.image}）`,
        image: item,
      });
    }
    for (const preset of presets) {
      // 注册表已覆盖同名镜像时不重复展示官方预设。
      // Skip presets already covered by a registered entry.
      if (items.some((item) => item.image === preset.image)) continue;
      all.push({key: `preset-${preset.image}`, label: preset.image, image: preset});
    }
    return all;
  }, [items, presets]);

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
      {!customMode ? (
        <Select
          value={selectedKey}
          onValueChange={(key) => {
            if (key === CUSTOM_KEY) {
              setCustomMode(true);
              return;
            }
            const option = options.find((entry) => entry.key === key);
            if (option) emit(option.image.image, option.image);
          }}
        >
          <SelectTrigger id={id} data-testid='k8s-image-picker-select'>
            <SelectValue placeholder={t('placeholder')} />
          </SelectTrigger>
          <SelectContent>
            {options.map((option) => (
              <SelectItem key={option.key} value={option.key}>
                <span className='flex items-center gap-2'>
                  {option.label}
                  {option.image.is_default ? (
                    <Badge variant='secondary' className='text-[10px]'>
                      {t('defaultBadge')}
                    </Badge>
                  ) : null}
                </span>
              </SelectItem>
            ))}
            <SelectItem value={CUSTOM_KEY}>{t('customImage')}</SelectItem>
          </SelectContent>
        </Select>
      ) : (
        <div className='flex items-center gap-2'>
          <Input
            id={id}
            data-testid='k8s-image-picker-input'
            value={value}
            placeholder='apache/seatunnel:2.3.13'
            onChange={(event) => emit(event.target.value)}
          />
          <button
            type='button'
            className='text-xs text-primary underline-offset-2 hover:underline'
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
