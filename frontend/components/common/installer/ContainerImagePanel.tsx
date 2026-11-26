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
import {ChevronDown, ChevronRight, Plus} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Button} from '@/components/arc/button/button';
import {Input} from '@/components/arc/input/input';
import {Badge} from '@/components/arc/badge/badge';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {ConfirmMorph} from '@/components/arc/confirm-morph/confirm-morph';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {
  listContainerImages,
  createContainerImage,
  updateContainerImage,
  deleteContainerImage,
  type ContainerImage,
} from '@/lib/services/installer/image.service';

interface ImageFormState {
  name: string;
  image: string;
  seatunnel_version: string;
  image_pull_policy: string;
  image_pull_secrets: string;
  is_default: boolean;
  description: string;
  registry_host: string;
  registry_username: string;
  registry_password: string;
}

const EMPTY_FORM: ImageFormState = {
  name: '',
  image: '',
  seatunnel_version: '',
  image_pull_policy: 'IfNotPresent',
  image_pull_secrets: '',
  is_default: false,
  description: '',
  registry_host: '',
  registry_username: '',
  registry_password: '',
};

const PULL_POLICIES = ['IfNotPresent', 'Always', 'Never'] as const;

// deriveVersionFromImage 从镜像 tag 推导 SeaTunnel 版本：apache/seatunnel:2.3.13 → 2.3.13；
// digest 引用（@sha256:）无法推导，返回空串交用户手填。
// deriveVersionFromImage derives the SeaTunnel version from the image tag:
// apache/seatunnel:2.3.13 → 2.3.13; digest references (@sha256:) cannot be derived
// and yield an empty string for manual input.
function deriveVersionFromImage(image: string): string {
  const trimmed = image.trim();
  if (trimmed.includes('@')) {
    return '';
  }
  const slash = trimmed.lastIndexOf('/');
  const colon = trimmed.lastIndexOf(':');
  if (colon <= slash) {
    return '';
  }
  return trimmed.slice(colon + 1).trim();
}

// ContainerImagePanel 安装包管理「容器镜像」Tab：注册镜像 CRUD + 官方预设快捷注册。
// ContainerImagePanel is the packages-page container image tab: registered image
// CRUD plus one-click registration from official presets.
export function ContainerImagePanel() {
  const t = useTranslations('k8s.images');
  const [items, setItems] = useState<ContainerImage[]>([]);
  const [presets, setPresets] = useState<ContainerImage[]>([]);
  const [loading, setLoading] = useState(true);
  const [presetsOpen, setPresetsOpen] = useState(false);
  const [editing, setEditing] = useState<ContainerImage | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<ImageFormState>(EMPTY_FORM);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const result = await listContainerImages();
      setItems(result.items || []);
      setPresets(result.official_presets || []);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const patch = useCallback((partial: Partial<ImageFormState>) => {
    setForm((prev) => ({...prev, ...partial}));
  }, []);

  const openCreate = useCallback((prefill?: Partial<ImageFormState>) => {
    setEditing(null);
    setForm({...EMPTY_FORM, ...prefill});
    setDialogOpen(true);
  }, []);

  const openEdit = useCallback((item: ContainerImage) => {
    setEditing(item);
    setForm({
      name: item.name,
      image: item.image,
      seatunnel_version: item.seatunnel_version || '',
      image_pull_policy: item.image_pull_policy || 'IfNotPresent',
      image_pull_secrets: item.image_pull_secrets || '',
      is_default: item.is_default,
      description: item.description || '',
      registry_host: item.registry_host || '',
      registry_username: item.registry_username || '',
      // 密码永不回显：留空保存表示保持既有凭据。
      // The password never round-trips: saving with an empty field keeps the
      // existing credential.
      registry_password: '',
    });
    setDialogOpen(true);
  }, []);

  // 点击预设 badge 一键预填注册表单。
  // Clicking a preset badge prefills the registration form.
  const openPreset = useCallback(
    (preset: ContainerImage) => {
      openCreate({
        name: preset.name,
        image: preset.image,
        seatunnel_version: preset.seatunnel_version || deriveVersionFromImage(preset.image),
      });
    },
    [openCreate],
  );

  const handleImageBlur = useCallback(() => {
    // 版本留空时从 tag 自动推导；用户已手填则不覆盖。
    // Auto-derive the version from the tag only while the field is empty; never
    // overwrite manual input.
    if (!form.seatunnel_version.trim()) {
      const derived = deriveVersionFromImage(form.image);
      if (derived) {
        patch({seatunnel_version: derived});
      }
    }
    // registry_host 从镜像引用自动推导：首段含 .或:或是 localhost 即为 registry，
    // 剥离误带的 http(s):// 与尾斜杠；用户已手填则不覆盖。
    // The registry host auto-derives from the image reference: the first segment is a
    // registry when it contains a dot or colon or is localhost; accidental schemes and
    // trailing slashes are stripped. Manual input is never overwritten.
    if (!form.registry_host.trim()) {
      const cleaned = form.image.trim().replace(/^https?:\/\//, '').replace(/\/+$/, '');
      const firstSegment = cleaned.split('/')[0] || '';
      if (firstSegment && (/[.:]/.test(firstSegment) || firstSegment === 'localhost') && cleaned.includes('/')) {
        patch({registry_host: firstSegment});
      }
    }
    }
  }, [form.seatunnel_version, form.image, patch]);

  const save = async () => {
    setSaving(true);
    try {
      if (editing) {
        await updateContainerImage(editing.id, form);
      } else {
        await createContainerImage(form);
      }
      toast.success(t('saved'));
      setDialogOpen(false);
      void reload();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (item: ContainerImage) => {
    try {
      await deleteContainerImage(item.id);
      toast.success(t('deleted'));
      void reload();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  };

  const credentialComplete = useMemo(
    () =>
      form.registry_host.trim() !== '' &&
      form.registry_username.trim() !== '' &&
      (form.registry_password.trim() !== '' || editing?.has_registry_auth === true),
    [form.registry_host, form.registry_username, form.registry_password, editing],
  );
  const credentialPartial = useMemo(
    () =>
      (form.registry_host.trim() !== '' ||
        form.registry_username.trim() !== '' ||
        form.registry_password.trim() !== '') &&
      !credentialComplete,
    [form.registry_host, form.registry_username, form.registry_password, credentialComplete],
  );

  return (
    <div className='space-y-3 p-3' data-testid='container-image-panel'>
      <div className='flex items-center justify-between'>
        <p className='text-xs text-muted-foreground'>{t('description')}</p>
        <Button size='sm' onClick={() => openCreate()} data-testid='container-image-create'>
          <Plus className='size-3.5' aria-hidden />
          {t('create')}
        </Button>
      </div>

      <table className='w-full text-sm [&_tr]:border-b-[0.5px] [&_tr]:border-border/70'>
        <thead>
          <tr className='text-left text-xs text-muted-foreground'>
            <th className='py-2 font-medium'>{t('colName')}</th>
            <th className='font-medium'>{t('colImage')}</th>
            <th className='font-medium'>{t('colVersion')}</th>
            <th className='font-medium'>{t('colPullPolicy')}</th>
            <th className='w-32 font-medium'>{t('colActions')}</th>
          </tr>
        </thead>
        <tbody>
          {loading
            ? Array.from({length: 3}).map((_, index) => (
                <tr key={`skeleton-${index}`} className='last:border-0'>
                  <td className='py-2' colSpan={5}>
                    <Skeleton className='h-5 w-full' />
                  </td>
                </tr>
              ))
            : items.map((item) => (
                <tr key={item.id} className='last:border-0' data-testid={`container-image-row-${item.id}`}>
                  <td className='py-2'>
                    <span className='flex items-center gap-2'>
                      {item.name}
                      {item.is_default ? <Badge tone='neutral'>{t('defaultBadge')}</Badge> : null}
                      {item.has_registry_auth ? (
                        <Badge tone='neutral' outline className='text-[10px]'>
                          {t('privateBadge')}
                        </Badge>
                      ) : null}
                    </span>
                  </td>
                  <td className='font-mono text-xs'>{item.image}</td>
                  <td>{item.seatunnel_version || '-'}</td>
                  <td>{item.image_pull_policy}</td>
                  <td>
                    <div className='flex items-center gap-2'>
                      <button type='button' className='text-xs text-primary' onClick={() => openEdit(item)}>
                        {t('edit')}
                      </button>
                      <ConfirmMorph
                        label={t('delete')}
                        confirmLabel={t('deleteConfirm', {name: item.name})}
                        pendingLabel={t('delete')}
                        className='text-xs text-destructive'
                        data-testid={`container-image-delete-confirm-${item.id}`}
                        onConfirm={() => remove(item)}
                      />
                    </div>
                  </td>
                </tr>
              ))}
          {!loading && items.length === 0 ? (
            <tr>
              <td colSpan={5} className='py-8'>
                <div className='flex flex-col items-center gap-2 text-center'>
                  <p className='text-xs text-muted-foreground'>{t('empty')}</p>
                  <Button size='sm' variant='secondary' onClick={() => openCreate()}>
                    <Plus className='size-3.5' aria-hidden />
                    {t('emptyAction')}
                  </Button>
                </div>
              </td>
            </tr>
          ) : null}
        </tbody>
      </table>

      {presets.length > 0 ? (
        <div className='space-y-1 border-t pt-2'>
          <button
            type='button'
            className='flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground'
            onClick={() => setPresetsOpen((prev) => !prev)}
            data-testid='container-image-presets-toggle'
          >
            {presetsOpen ? <ChevronDown className='size-3' aria-hidden /> : <ChevronRight className='size-3' aria-hidden />}
            {t('officialPresets')}（{presets.length}）
          </button>
          {presetsOpen ? (
            <div className='flex flex-wrap gap-1.5 pt-1'>
              {presets.map((preset) => (
                <button
                  key={preset.image}
                  type='button'
                  title={t('presetRegisterHint')}
                  onClick={() => openPreset(preset)}
                  data-testid={`container-image-preset-${preset.seatunnel_version}`}
                >
                  <Badge tone='neutral' outline className='cursor-pointer font-mono text-[10px]'>
                    {preset.image}
                  </Badge>
                </button>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent
          title={editing ? t('editTitle') : t('createTitle')}
          className='arc-compact max-h-[85vh]! w-[min(94vw,640px)]! overflow-y-auto!'
        >
          <div className='space-y-3'>
            <div className='space-y-1.5'>
              <Input
                label={t('fieldName')}
                id='image-form-name'
                value={form.name}
                onChange={(e) => patch({name: e.target.value})}
                data-testid='image-form-name'
              />
            </div>
            <div className='space-y-1.5'>
              <Input
                label={t('fieldImage')}
                id='image-form-image'
                placeholder='apache/seatunnel:2.3.13'
                value={form.image}
                onChange={(e) => patch({image: e.target.value})}
                onBlur={handleImageBlur}
                data-testid='image-form-image'
              />
            </div>
            <div className='space-y-1.5'>
              <Input
                label={t('fieldVersion')}
                id='image-form-version'
                placeholder='2.3.13'
                value={form.seatunnel_version}
                onChange={(e) => patch({seatunnel_version: e.target.value})}
              />
              <p className='text-[11px] text-muted-foreground'>{t('fieldVersionHint')}</p>
            </div>
            <div className='space-y-1.5'>
              <SegmentedControl
                label={t('colPullPolicy')}
                value={form.image_pull_policy}
                onValueChange={(policy) => patch({image_pull_policy: policy})}
                options={PULL_POLICIES.map((policy) => ({value: policy, label: policy}))}
                data-testid='image-form-pull-policy'
              />
            </div>
            <div className='space-y-1.5'>
              <Input
                label={t('fieldPullSecrets')}
                id='image-form-pull-secrets'
                value={form.image_pull_secrets}
                onChange={(e) => patch({image_pull_secrets: e.target.value})}
              />
              <p className='text-[11px] text-muted-foreground'>{t('fieldPullSecretsHint')}</p>
            </div>
            <div className='space-y-1.5'>
              <Input
                label={t('fieldDescription')}
                id='image-form-description'
                value={form.description}
                onChange={(e) => patch({description: e.target.value})}
              />
            </div>

            <div className='space-y-2 border-t-[0.5px] border-border/80 pt-3'>
              <p className='text-xs font-medium'>{t('registryTitle')}</p>
              <p className='text-[11px] text-muted-foreground'>{t('registryHint')}</p>
              <div className='grid grid-cols-2 gap-2'>
                <div className='space-y-1.5'>
                  <Input
                    label={t('registryHost')}
                    id='image-form-registry-host'
                    placeholder='registry.example.com'
                    value={form.registry_host}
                    onChange={(e) => patch({registry_host: e.target.value})}
                    data-testid='image-form-registry-host'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Input
                    label={t('registryUsername')}
                    id='image-form-registry-username'
                    value={form.registry_username}
                    onChange={(e) => patch({registry_username: e.target.value})}
                  />
                </div>
              </div>
              <div className='space-y-1.5'>
                <Input
                  label={t('registryPassword')}
                  id='image-form-registry-password'
                  type='password'
                  placeholder={editing?.has_registry_auth ? t('registryPasswordKeep') : ''}
                  value={form.registry_password}
                  onChange={(e) => patch({registry_password: e.target.value})}
                  data-testid='image-form-registry-password'
                />
                {credentialPartial ? (
                  <p className='text-[11px] text-destructive'>{t('registryIncomplete')}</p>
                ) : null}
              </div>
            </div>

            <Checkbox label={t('fieldDefault')} checked={form.is_default} onCheckedChange={(checked) => patch({is_default: checked === true})} />
          </div>
          <div className='flex justify-end gap-2 pt-2'>
            <Button variant='secondary' onClick={() => setDialogOpen(false)}>
              {t('cancel')}
            </Button>
            <Button onClick={() => void save()} disabled={saving} data-testid='image-form-save'>
              {t('save')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
