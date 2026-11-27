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

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Combobox} from '@/components/arc/combobox/combobox';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Button} from '@/components/arc/button/button';
import {
  composeImageRef,
  extractVersionFromTag,
  listContainerRegistries,
  listRegistryTags,
  parseImageRef,
  type ContainerRegistry,
} from '@/lib/services/installer/registry.service';

export interface K8sImagePickerValue {
  image: string;
  seatunnelVersion: string;
}

interface K8sImagePickerProps {
  value: string;
  version?: string;
  initialImage?: string;
  onChange: (value: K8sImagePickerValue) => void;
  id?: string;
  versionTestId?: string;
}

type PickerMode = 'registry' | 'custom';

const DOCKER_HUB_HOST = 'docker.io';
const DOCKER_HUB_DEFAULT_REPO = 'apache/seatunnel';
const TAG_DEBOUNCE_MS = 350;

// 解析仓库的默认镜像路径：优先使用仓库登记的 default_repo，docker.io 缺省回落 apache/seatunnel。
// Resolve a registry's default repository path: prefer configured default_repo, falling back to apache/seatunnel for docker.io.
function defaultRepoForRegistry(reg: ContainerRegistry | null | undefined): string {
  if (!reg) {
    return '';
  }
  const configured = reg.default_repo?.trim();
  if (configured) {
    return configured;
  }
  return reg.host === DOCKER_HUB_HOST ? DOCKER_HUB_DEFAULT_REPO : '';
}

// K8sImagePicker 统一镜像选择器：支持快速粘贴自动吸附、已登记仓库三层选择
// （仓库 → 自动预填默认镜像名 / 手填 → 350ms 防抖实时 tag 探测）与未登记自定义镜像（Route A），
// 并唯一托管引擎版本字段，不再覆写上层 pullPolicy/pullSecrets。
// K8sImagePicker unifies registry-guided selection (registry -> prefilled default_repo ->
// 350ms debounced live tag probing), quick-paste auto-snapping, custom unregistered image
// input (Route A), and the single engine-version field without overwriting pullPolicy or pullSecrets.
export function K8sImagePicker({
  value,
  version: versionProp,
  initialImage,
  onChange,
  id,
  versionTestId = 'k8s-version-input',
}: K8sImagePickerProps) {
  const t = useTranslations('k8s.imagePicker');
  const [registries, setRegistries] = useState<ContainerRegistry[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [mode, setMode] = useState<PickerMode>('registry');
  const [pasteInput, setPasteInput] = useState('');
  const [pasteNotice, setPasteNotice] = useState<string | null>(null);

  const [registryId, setRegistryId] = useState('');
  const [repo, setRepo] = useState('');
  const [tag, setTag] = useState('');
  const [manualTag, setManualTag] = useState(false);
  const [customImage, setCustomImage] = useState('');

  const [tags, setTags] = useState<string[]>([]);
  const [tagsLoading, setTagsLoading] = useState(false);
  const [tagsVerified, setTagsVerified] = useState(true);

  const [localVersion, setLocalVersion] = useState(versionProp ?? '');
  const versionTouchedRef = useRef(false);
  const hydratedRef = useRef(false);
  const skipNextDefaultResetRef = useRef(false);

  const currentVersion = versionProp !== undefined ? versionProp : localVersion;

  const updateVersionState = useCallback(
    (nextVersion: string) => {
      if (versionProp === undefined) {
        setLocalVersion(nextVersion);
      }
      return nextVersion;
    },
    [versionProp],
  );

  // 加载仓库清单；完成后按 value -> initialImage -> 默认 docker.io 顺序回填一次。
  // Load registry list; on completion hydrate once from value, initialImage, or docker.io.
  const load = useCallback(() => {
    setLoading(true);
    setLoadError(null);
    listContainerRegistries()
      .then((items) => {
        setRegistries(items);
        if (hydratedRef.current) {
          return;
        }
        hydratedRef.current = true;

        const trimmedValue = (value || '').trim();
        const trimmedInitial = (initialImage || '').trim();
        const findByHost = (host: string) =>
          items.find((item) => item.host.toLowerCase() === host.toLowerCase());

        if (trimmedValue) {
          const parsed = parseImageRef(trimmedValue);
          const matched = parsed ? findByHost(parsed.host) : undefined;
          if (parsed && matched) {
            skipNextDefaultResetRef.current = true;
            setMode('registry');
            setRegistryId(String(matched.id));
            setRepo(parsed.repo);
            setTag(parsed.tag);
            if (!versionTouchedRef.current && !versionProp) {
              updateVersionState(extractVersionFromTag(parsed.tag));
            }
          } else {
            setMode('custom');
            setCustomImage(trimmedValue);
            if (!versionTouchedRef.current && !versionProp && parsed) {
              updateVersionState(extractVersionFromTag(parsed.tag));
            }
          }
          return;
        }

        if (trimmedInitial) {
          const parsed = parseImageRef(trimmedInitial);
          const matched = parsed ? findByHost(parsed.host) : undefined;
          if (parsed && matched) {
            skipNextDefaultResetRef.current = true;
            setMode('registry');
            setRegistryId(String(matched.id));
            setRepo(parsed.repo);
            // 升级场景沿用原仓库与镜像名，清空 tag 让用户选择目标版本。
            // Upgrade mode keeps registry + repo and clears tag so user picks target version.
            setTag('');
          } else {
            setMode('custom');
            setCustomImage(trimmedInitial);
          }
          return;
        }

        if (items.length > 0) {
          const sys = items.find((item) => item.is_system) || items[0];
          setMode('registry');
          setRegistryId(String(sys.id));
          setRepo(defaultRepoForRegistry(sys));
        }
      })
      .catch((err) => {
        setLoadError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        setLoading(false);
      });
  }, [initialImage, updateVersionState, value, versionProp]);

  useEffect(() => {
    load();
  }, [load]);

  const registry = useMemo(
    () => registries.find((item) => String(item.id) === registryId) || null,
    [registries, registryId],
  );

  // 切换仓库时按该仓库配置的默认镜像名预填（若无则置空），并清空 tag 重新探测。
  // Switching registry prefills its configured default_repo (or empties it) and resets tags for probing.
  const handleSelectRegistry = (nextId: string) => {
    const nextReg = registries.find((item) => String(item.id) === nextId) || null;
    const nextRepo = defaultRepoForRegistry(nextReg);
    setRegistryId(nextId);
    setRepo(nextRepo);
    setTag('');
    setManualTag(false);
    setTags([]);
    setTagsVerified(true);
    setPasteNotice(null);
    if (!versionTouchedRef.current) {
      updateVersionState('');
    }
    onChange({
      image: '',
      seatunnelVersion: versionTouchedRef.current ? currentVersion : '',
    });
  };

  // 防抖探测 tag（350ms），避免输入镜像名时每敲一字请求一次远端仓库。
  // Debounce tag probing (350ms) so typing a repo name does not spam upstream registries.
  useEffect(() => {
    if (skipNextDefaultResetRef.current) {
      skipNextDefaultResetRef.current = false;
    }
    if (mode !== 'registry' || !registry || !repo.trim()) {
      setTags([]);
      setTagsLoading(false);
      return;
    }
    let cancelled = false;
    setTagsLoading(true);
    const timer = setTimeout(() => {
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
        })
        .finally(() => {
          if (!cancelled) {
            setTagsLoading(false);
          }
        });
    }, TAG_DEBOUNCE_MS);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [mode, registry, repo]);

  const emitRegistry = useCallback(
    (nextHost: string, nextRepo: string, nextTag: string, nextVersion: string) => {
      onChange({
        image: composeImageRef(nextHost, nextRepo, nextTag),
        seatunnelVersion: nextVersion,
      });
    },
    [onChange],
  );

  const pickTag = (nextTag: string) => {
    setTag(nextTag);
    setPasteNotice(null);
    const extracted = extractVersionFromTag(nextTag);
    const nextVersion = versionTouchedRef.current ? currentVersion : (extracted || '');
    if (!versionTouchedRef.current) {
      updateVersionState(nextVersion);
    }
    if (registry) {
      emitRegistry(registry.host, repo, nextTag, nextVersion);
    }
  };

  // 智能粘贴识别：命中已登记仓库自动切回三层模式并填值；未命中自动切到自定义模式（Route A）。
  // Smart paste snapping: matches a registered host into registry mode, or snaps to custom mode (Route A).
  const handleQuickPaste = (raw: string) => {
    setPasteInput(raw);
    const trimmed = raw.trim();
    if (!trimmed) {
      setPasteNotice(null);
      return;
    }
    const parsed = parseImageRef(trimmed);
    if (!parsed) {
      setPasteNotice(null);
      return;
    }
    const matched = registries.find((item) => item.host.toLowerCase() === parsed.host.toLowerCase());
    const extracted = extractVersionFromTag(parsed.tag);
    const nextVersion = versionTouchedRef.current ? currentVersion : (extracted || '');
    if (!versionTouchedRef.current) {
      updateVersionState(nextVersion);
    }

    if (matched) {
      setMode('registry');
      setRegistryId(String(matched.id));
      setRepo(parsed.repo);
      setTag(parsed.tag);
      setManualTag(false);
      setPasteNotice(t('pasteSnappedRegistry', {host: matched.host}));
      onChange({
        image: composeImageRef(matched.host, parsed.repo, parsed.tag),
        seatunnelVersion: nextVersion,
      });
    } else {
      setMode('custom');
      setCustomImage(trimmed);
      setPasteNotice(t('pasteSnappedCustom', {host: parsed.host}));
      onChange({
        image: trimmed,
        seatunnelVersion: nextVersion,
      });
    }
  };

  const handleModeChange = (nextModeValue: string) => {
    const nextMode = nextModeValue as PickerMode;
    if (nextMode === mode) {
      return;
    }
    setPasteNotice(null);
    if (nextMode === 'custom') {
      const derived = registry ? composeImageRef(registry.host, repo, tag) : '';
      const nextCustom = customImage || derived;
      setCustomImage(nextCustom);
      setMode('custom');
      onChange({
        image: nextCustom.trim(),
        seatunnelVersion: currentVersion,
      });
      return;
    }

    // 切回仓库模式时尝试从 customImage 吸附匹配到的已登记仓库。
    // Switching back to registry mode attempts to snap customImage onto a registered host.
    const parsed = parseImageRef(customImage);
    const matched = parsed
      ? registries.find((item) => item.host.toLowerCase() === parsed.host.toLowerCase())
      : undefined;
    if (parsed && matched) {
      setRegistryId(String(matched.id));
      setRepo(parsed.repo);
      setTag(parsed.tag);
      setMode('registry');
      emitRegistry(matched.host, parsed.repo, parsed.tag, currentVersion);
      return;
    }

    setMode('registry');
    emitRegistry(registry?.host || '', repo, tag, currentVersion);
  };

  const tagOptions = useMemo(
    () => tags.slice(0, 200).map((item) => ({value: item, label: item})),
    [tags],
  );
  const useTagCombobox = tagOptions.length > 0 && !manualTag;

  const derived =
    mode === 'custom'
      ? customImage.trim()
      : registry
        ? composeImageRef(registry.host, repo, tag)
        : '';

  const activeParsed = mode === 'custom' ? parseImageRef(customImage) : null;
  const activeTag = mode === 'custom' ? (activeParsed?.tag ?? '') : tag.trim();
  const extractedVersion = extractVersionFromTag(activeTag);
  const versionWarning = Boolean(activeTag) && extractedVersion === '' && currentVersion.trim() === '';

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

  return (
    <div className='space-y-3' data-testid='k8s-image-picker'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <SegmentedControl
          value={mode}
          onValueChange={handleModeChange}
          label={t('registryLabel')}
          options={[
            {value: 'registry', label: t('modeRegistry'), testId: 'k8s-image-picker-mode-registry'},
            {value: 'custom', label: t('modeCustom'), testId: 'k8s-image-picker-mode-custom'},
          ]}
        />
      </div>

      <div className='space-y-1'>
        <Input
          label={t('pasteLabel')}
          value={pasteInput}
          placeholder={t('pastePlaceholder')}
          onChange={(event) => handleQuickPaste(event.target.value)}
          data-testid='k8s-image-picker-paste'
        />
        {pasteNotice ? (
          <p className='text-xs text-muted-foreground' data-testid='k8s-image-picker-paste-notice'>
            {pasteNotice}
          </p>
        ) : null}
      </div>

      {mode === 'registry' ? (
        <div className='space-y-2.5'>
          <Select
            aria-label={t('registryLabel')}
            value={registryId}
            placeholder={t('registryPlaceholder')}
            options={registries.map((item) => ({
              value: String(item.id),
              label: item.is_system ? `${item.host} · ${t('officialBadge')}` : item.host,
            }))}
            onValueChange={handleSelectRegistry}
            data-testid='k8s-image-picker-registry'
          />

          <div className='space-y-1'>
            <Input
              id={id}
              label={t('repoLabel')}
              value={repo}
              placeholder={defaultRepoForRegistry(registry) || DOCKER_HUB_DEFAULT_REPO}
              onChange={(event) => {
                const nextRepo = event.target.value;
                setRepo(nextRepo);
                setTag('');
                setPasteNotice(null);
                if (registry) {
                  emitRegistry(registry.host, nextRepo, '', currentVersion);
                }
              }}
              data-testid='k8s-image-picker-repo-input'
            />
          </div>

          <div className='space-y-1'>
            {useTagCombobox ? (
              <Combobox
                label={t('tagLabel')}
                value={tag}
                placeholder={t('tagPlaceholder')}
                emptyMessage={t('tagEmpty')}
                options={tagOptions}
                onValueChange={pickTag}
                data-testid='k8s-image-picker-tag'
              />
            ) : (
              <Input
                label={t('tagLabel')}
                value={tag}
                placeholder='3.1.1'
                onChange={(event) => pickTag(event.target.value)}
                data-testid='k8s-image-picker-tag-input'
              />
            )}
            {tagsLoading ? (
              <p className='text-xs text-muted-foreground' data-testid='k8s-image-picker-tags-loading'>
                {t('tagsLoading')}
              </p>
            ) : null}
            {!tagsLoading && tagOptions.length > 0 ? (
              <button
                type='button'
                className='text-xs text-primary underline-offset-2 hover:underline'
                onClick={() => setManualTag((prev) => !prev)}
                data-testid='k8s-image-picker-tag-toggle'
              >
                {manualTag ? t('tagUseList') : t('tagManualFallback')}
              </button>
            ) : null}
            {!tagsLoading && repo.trim() && !tagsVerified ? (
              <p className='text-xs text-amber-600 dark:text-amber-400'>{t('tagUnverified')}</p>
            ) : null}
          </div>
        </div>
      ) : (
        <div className='space-y-1.5'>
          <Input
            id={id}
            label={t('customImageLabel')}
            value={customImage}
            placeholder={t('customImagePlaceholder')}
            onChange={(event) => {
              const nextRaw = event.target.value;
              setCustomImage(nextRaw);
              setPasteNotice(null);
              const parsed = parseImageRef(nextRaw);
              const extracted = parsed ? extractVersionFromTag(parsed.tag) : '';
              const nextVersion = versionTouchedRef.current ? currentVersion : (extracted || '');
              if (!versionTouchedRef.current) {
                updateVersionState(nextVersion);
              }
              onChange({
                image: nextRaw.trim(),
                seatunnelVersion: nextVersion,
              });
            }}
            data-testid='k8s-image-picker-custom-input'
          />
          <p className='text-xs text-muted-foreground'>{t('customImageHint')}</p>
        </div>
      )}

      <div className='space-y-1'>
        <Input
          label={t('versionLabel')}
          value={currentVersion}
          placeholder='3.1.1'
          onChange={(event) => {
            versionTouchedRef.current = true;
            const nextVersion = updateVersionState(event.target.value);
            onChange({
              image: derived,
              seatunnelVersion: nextVersion,
            });
          }}
          data-testid={versionTestId}
        />
        {versionWarning ? (
          <p className='text-xs text-amber-600 dark:text-amber-400' data-testid='k8s-image-picker-version-warning'>
            {t('versionUndetermined')}
          </p>
        ) : null}
      </div>

      {derived ? (
        <p className='font-mono text-xs text-muted-foreground' data-testid='k8s-image-picker-derived'>
          {derived}
        </p>
      ) : null}
    </div>
  );
}
