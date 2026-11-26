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
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/ui/checkbox';
import {Skeleton} from '@/components/ui/skeleton';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
  const [deletingId, setDeletingId] = useState<number | null>(null);
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
    setDeletingId(null);
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

      <table className='w-full text-sm'>
        <thead>
          <tr className='border-b text-left text-xs text-muted-foreground'>
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
                <tr key={`skeleton-${index}`} className='border-b last:border-0'>
                  <td className='py-2' colSpan={5}>
                    <Skeleton className='h-5 w-full' />
                  </td>
                </tr>
              ))
            : items.map((item) => (
                <tr key={item.id} className='border-b last:border-0' data-testid={`container-image-row-${item.id}`}>
                  <td className='py-2'>
                    <span className='flex items-center gap-2'>
                      {item.name}
                      {item.is_default ? <Badge variant='secondary'>{t('defaultBadge')}</Badge> : null}
                      {item.has_registry_auth ? (
                        <Badge variant='outline' className='text-[10px]'>
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
                      <Popover
                        open={deletingId === item.id}
                        onOpenChange={(open) => setDeletingId(open ? item.id : null)}
                      >
                        <PopoverTrigger asChild>
                          <button type='button' className='text-xs text-destructive'>
                            {t('delete')}
                          </button>
                        </PopoverTrigger>
                        <PopoverContent className='w-56 space-y-2 p-3' align='start'>
                          <p className='text-xs text-muted-foreground'>
                            {t('deleteConfirm', {name: item.name})}
                          </p>
                          <div className='flex justify-end gap-2'>
                            <Button size='sm' variant='ghost' onClick={() => setDeletingId(null)}>
                              {t('cancel')}
                            </Button>
                            <Button
                              size='sm'
                              variant='destructive'
                              data-testid={`container-image-delete-confirm-${item.id}`}
                              onClick={() => void remove(item)}
                            >
                              {t('delete')}
                            </Button>
                          </div>
                        </PopoverContent>
                      </Popover>
                    </div>
                  </td>
                </tr>
              ))}
          {!loading && items.length === 0 ? (
            <tr>
              <td colSpan={5} className='py-8'>
                <div className='flex flex-col items-center gap-2 text-center'>
                  <p className='text-xs text-muted-foreground'>{t('empty')}</p>
                  <Button size='sm' variant='outline' onClick={() => openCreate()}>
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
                  <Badge variant='outline' className='cursor-pointer font-mono text-[10px] hover:bg-accent'>
                    {preset.image}
                  </Badge>
                </button>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='max-h-[85vh] overflow-y-auto'>
          <DialogHeader>
            <DialogTitle>{editing ? t('editTitle') : t('createTitle')}</DialogTitle>
          </DialogHeader>
          <div className='space-y-3'>
            <div className='space-y-1.5'>
              <Label htmlFor='image-form-name'>{t('fieldName')}</Label>
              <Input
                id='image-form-name'
                value={form.name}
                onChange={(e) => patch({name: e.target.value})}
                data-testid='image-form-name'
              />
            </div>
            <div className='space-y-1.5'>
              <Label htmlFor='image-form-image'>{t('fieldImage')}</Label>
              <Input
                id='image-form-image'
                placeholder='apache/seatunnel:2.3.13'
                value={form.image}
                onChange={(e) => patch({image: e.target.value})}
                onBlur={handleImageBlur}
                data-testid='image-form-image'
              />
            </div>
            <div className='space-y-1.5'>
              <Label htmlFor='image-form-version'>{t('fieldVersion')}</Label>
              <Input
                id='image-form-version'
                placeholder='2.3.13'
                value={form.seatunnel_version}
                onChange={(e) => patch({seatunnel_version: e.target.value})}
              />
              <p className='text-[11px] text-muted-foreground'>{t('fieldVersionHint')}</p>
            </div>
            <div className='space-y-1.5'>
              <Label>{t('colPullPolicy')}</Label>
              <div className='flex gap-1' data-testid='image-form-pull-policy'>
                {PULL_POLICIES.map((policy) => (
                  <Button
                    key={policy}
                    type='button'
                    size='sm'
                    variant={form.image_pull_policy === policy ? 'default' : 'outline'}
                    onClick={() => patch({image_pull_policy: policy})}
                  >
                    {policy}
                  </Button>
                ))}
              </div>
            </div>
            <div className='space-y-1.5'>
              <Label htmlFor='image-form-pull-secrets'>{t('fieldPullSecrets')}</Label>
              <Input
                id='image-form-pull-secrets'
                value={form.image_pull_secrets}
                onChange={(e) => patch({image_pull_secrets: e.target.value})}
              />
              <p className='text-[11px] text-muted-foreground'>{t('fieldPullSecretsHint')}</p>
            </div>
            <div className='space-y-1.5'>
              <Label htmlFor='image-form-description'>{t('fieldDescription')}</Label>
              <Input
                id='image-form-description'
                value={form.description}
                onChange={(e) => patch({description: e.target.value})}
              />
            </div>

            <div className='space-y-2 rounded-md border border-dashed p-3'>
              <p className='text-xs font-medium'>{t('registryTitle')}</p>
              <p className='text-[11px] text-muted-foreground'>{t('registryHint')}</p>
              <div className='grid grid-cols-2 gap-2'>
                <div className='space-y-1.5'>
                  <Label htmlFor='image-form-registry-host'>{t('registryHost')}</Label>
                  <Input
                    id='image-form-registry-host'
                    placeholder='registry.example.com'
                    value={form.registry_host}
                    onChange={(e) => patch({registry_host: e.target.value})}
                    data-testid='image-form-registry-host'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label htmlFor='image-form-registry-username'>{t('registryUsername')}</Label>
                  <Input
                    id='image-form-registry-username'
                    value={form.registry_username}
                    onChange={(e) => patch({registry_username: e.target.value})}
                  />
                </div>
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='image-form-registry-password'>{t('registryPassword')}</Label>
                <Input
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

            <label className='flex items-center gap-2 text-sm'>
              <Checkbox checked={form.is_default} onCheckedChange={(checked) => patch({is_default: checked === true})} />
              {t('fieldDefault')}
            </label>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setDialogOpen(false)}>
              {t('cancel')}
            </Button>
            <Button onClick={() => void save()} disabled={saving} data-testid='image-form-save'>
              {t('save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
