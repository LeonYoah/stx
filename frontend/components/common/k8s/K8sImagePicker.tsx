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
  composeImageRef,
  extractVersionFromTag,
  listContainerRegistries,
  listRegistryTags,
  type ContainerRegistry,
} from '@/lib/services/installer/registry.service';

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

const DOCKER_HUB_HOST = 'docker.io';
const DOCKER_HUB_DEFAULT_REPO = 'apache/seatunnel';

// K8sImagePicker 三层镜像选择器：仓库 → 镜像名（catalog 探测 + 输入过滤）→ tag，
// 版本为显式字段（从 tag 预填可编辑，提取失败黄字告警），派生镜像地址只读展示。
// K8sImagePicker is the three-tier image picker: registry → repo (catalog probe
// with input filtering) → tag, an explicit version field (prefilled from the tag,
// editable, yellow warning when extraction fails) and a read-only derived reference.
export function K8sImagePicker({value, onChange, id}: K8sImagePickerProps) {
  const t = useTranslations('k8s.imagePicker');
  const [registries, setRegistries] = useState<ContainerRegistry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [registryId, setRegistryId] = useState('');
  const [repo, setRepo] = useState('');
  const [repoFilter, setRepoFilter] = useState('');
  const [tag, setTag] = useState('');
  const [tags, setTags] = useState<string[]>([]);
  const [tagsVerified, setTagsVerified] = useState(true);
  const [version, setVersion] = useState('');

  // 镜像清单加载失败必须显式可重试，禁止静默吞错留下永久空清单。
  // Registry list load failures surface a retry action; never swallow errors silently.
  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    listContainerRegistries()
      .then((items) => {
        setRegistries(items);
        // 默认选中 docker.io 系统行，用户开箱即可用。
        // Preselect the docker.io system row so the picker works out of the box.
        if (items.length > 0) {
          const sys = items.find((item) => item.is_system) || items[0];
          setRegistryId(String(sys.id));
        }
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

  const registry = useMemo(
    () => registries.find((item) => String(item.id) === registryId) || null,
    [registries, registryId],
  );

  // docker.io 预填 apache/seatunnel 但保留可编辑口子；私仓从 catalog 探测缓存过滤。
  // docker.io prefills apache/seatunnel yet stays editable; private registries
  // filter the probed catalog cache.
  useEffect(() => {
    if (!registry) {return;}
    setRepo(registry.host === DOCKER_HUB_HOST ? DOCKER_HUB_DEFAULT_REPO : '');
    setRepoFilter('');
    setTag('');
    setTags([]);
    setTagsVerified(true);
  }, [registry?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!registry || !repo.trim()) {
      setTags([]);
      return;
    }
    let cancelled = false;
    listRegistryTags(registry.id, repo.trim())
      .then((items) => {
        if (!cancelled) {
          setTags(items);
          setTagsVerified(true);
        }
      })
      .catch(() => {
        // 探测失败退化为手动输入并标「未验证」，不死锁用户。
        // Probe failure degrades to manual input flagged unverified; never deadlocks.
        if (!cancelled) {
          setTags([]);
          setTagsVerified(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [registry, repo]);

  const emit = useCallback(
    (nextRepo: string, nextTag: string, nextVersion: string) => {
      if (!registry) {return;}
      onChange({
        image: composeImageRef(registry.host, nextRepo, nextTag),
        seatunnelVersion: nextVersion,
        pullPolicy: 'IfNotPresent',
        pullSecrets: '',
      });
    },
    [registry, onChange],
  );

  const pickTag = (nextTag: string) => {
    setTag(nextTag);
    const extracted = extractVersionFromTag(nextTag);
    const nextVersion = extracted || version;
    setVersion(nextVersion);
    emit(repo, nextTag, nextVersion);
  };

  const catalogOptions = useMemo(() => {
    const repos = registry?.catalog_repos || [];
    const needle = repoFilter.trim().toLowerCase();
    const filtered = needle ? repos.filter((item) => item.toLowerCase().includes(needle)) : repos;
    return filtered.slice(0, 100);
  }, [registry, repoFilter]);

  const tagOptions = useMemo(() => tags.slice(0, 200), [tags]);
  const extracted = extractVersionFromTag(tag);
  const versionWarning = tag.trim() !== '' && extracted === '';

  // 已有镜像地址回填：仅在反向可解析时展示，避免双数据源冲突。
  // Existing references surface read-only; the tiers remain the source of truth.
  void value;

  if (loading) {
    return <Skeleton lines={3} className='h-9' label={t('registryLabel')} />;
  }
  if (loadError) {
    return (
      <div className='flex items-center gap-2'>
        <p className='min-w-0 flex-1 truncate text-xs text-destructive'>{loadError}</p>
        <Button type='button' variant='secondary' size='sm' onClick={load}>
          {t('retry')}
        </Button>
      </div>
    );
  }

  const derived = registry ? composeImageRef(registry.host, repo, tag) : '';

  return (
    <div className='space-y-2' data-testid='k8s-image-picker'>
      <Select
        aria-label={t('registryLabel')}
        value={registryId}
        placeholder={t('registryPlaceholder')}
        options={registries.map((item) => ({
          value: String(item.id),
          label: item.is_system ? `${item.host} · ${t('officialBadge')}` : item.host,
        }))}
        onValueChange={(next) => {
          setRegistryId(next);
          setVersion('');
        }}
        data-testid='k8s-image-picker-registry'
      />
      {registry && registry.host !== DOCKER_HUB_HOST && catalogOptions.length > 0 ? (
        <div className='space-y-1'>
          <Input
            id={id ? `${id}-filter` : undefined}
            label={t('repoFilterLabel')}
            value={repoFilter}
            placeholder={t('repoFilterPlaceholder')}
            onChange={(event) => setRepoFilter(event.target.value)}
          />
          <Select
            aria-label={t('repoLabel')}
            value={repo}
            placeholder={t('repoPlaceholder')}
            options={catalogOptions.map((item) => ({value: item, label: item}))}
            onValueChange={(next) => {
              setRepo(next);
              setTag('');
              emit(next, '', version);
            }}
            data-testid='k8s-image-picker-repo'
          />
        </div>
      ) : (
        <Input
          id={id}
          label={t('repoLabel')}
          value={repo}
          placeholder={registry?.host === DOCKER_HUB_HOST ? DOCKER_HUB_DEFAULT_REPO : 'seatunnel'}
          onChange={(event) => {
            setRepo(event.target.value);
            setTag('');
            emit(event.target.value, '', version);
          }}
          data-testid='k8s-image-picker-repo-input'
        />
      )}
      {tagOptions.length > 0 ? (
        <Select
          aria-label={t('tagLabel')}
          value={tag}
          placeholder={t('tagPlaceholder')}
          options={tagOptions.map((item) => ({value: item, label: item}))}
          onValueChange={pickTag}
          data-testid='k8s-image-picker-tag'
        />
      ) : (
        <div className='space-y-1'>
          <Input
            label={t('tagLabel')}
            value={tag}
            placeholder='3.1.1'
            onChange={(event) => pickTag(event.target.value)}
            data-testid='k8s-image-picker-tag-input'
          />
          {repo.trim() && !tagsVerified ? (
            <p className='text-xs text-amber-600 dark:text-amber-400'>{t('tagUnverified')}</p>
          ) : null}
        </div>
      )}
      <Input
        label={t('versionLabel')}
        value={version}
        placeholder='3.1.1'
        onChange={(event) => {
          setVersion(event.target.value);
          emit(repo, tag, event.target.value);
        }}
        data-testid='k8s-image-picker-version-input'
      />
      {versionWarning ? (
        <p className='text-xs text-amber-600 dark:text-amber-400' data-testid='k8s-image-picker-version-warning'>
          {t('versionUndetermined')}
        </p>
      ) : null}
      {derived ? (
        <p className='font-mono text-xs text-muted-foreground' data-testid='k8s-image-picker-derived'>
          {derived}
        </p>
      ) : null}
    </div>
  );
}
