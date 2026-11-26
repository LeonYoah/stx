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
import {toast} from '@/lib/toast';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/ui/checkbox';
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

// ContainerImagePanel 安装包管理「容器镜像」Tab：注册镜像 CRUD + 官方预设只读展示。
// ContainerImagePanel is the packages-page container image tab: registered image
// CRUD plus read-only official presets.
export function ContainerImagePanel() {
  const t = useTranslations('k8s.images');
  const [items, setItems] = useState<ContainerImage[]>([]);
  const [presets, setPresets] = useState<ContainerImage[]>([]);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<ContainerImage | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState({name: '', image: '', seatunnel_version: '', image_pull_policy: 'IfNotPresent', image_pull_secrets: '', is_default: false, description: ''});

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

  const openCreate = () => {
    setEditing(null);
    setForm({name: '', image: '', seatunnel_version: '', image_pull_policy: 'IfNotPresent', image_pull_secrets: '', is_default: false, description: ''});
    setDialogOpen(true);
  };

  const openEdit = (item: ContainerImage) => {
    setEditing(item);
    setForm({
      name: item.name,
      image: item.image,
      seatunnel_version: item.seatunnel_version || '',
      image_pull_policy: item.image_pull_policy || 'IfNotPresent',
      image_pull_secrets: item.image_pull_secrets || '',
      is_default: item.is_default,
      description: item.description || '',
    });
    setDialogOpen(true);
  };

  const save = async () => {
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

  return (
    <div className='space-y-3 p-3' data-testid='container-image-panel'>
      <div className='flex items-center justify-between'>
        <p className='text-xs text-muted-foreground'>{t('description')}</p>
        <Button size='sm' onClick={openCreate} data-testid='container-image-create'>
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
          {items.map((item) => (
            <tr key={item.id} className='border-b last:border-0' data-testid={`container-image-row-${item.id}`}>
              <td className='py-2'>
                <span className='flex items-center gap-2'>
                  {item.name}
                  {item.is_default ? <Badge variant='secondary'>{t('defaultBadge')}</Badge> : null}
                </span>
              </td>
              <td className='font-mono text-xs'>{item.image}</td>
              <td>{item.seatunnel_version || '-'}</td>
              <td>{item.image_pull_policy}</td>
              <td>
                <div className='flex gap-2'>
                  <button type='button' className='text-xs text-primary' onClick={() => openEdit(item)}>
                    {t('edit')}
                  </button>
                  <button type='button' className='text-xs text-destructive' onClick={() => void remove(item)}>
                    {t('delete')}
                  </button>
                </div>
              </td>
            </tr>
          ))}
          {items.length === 0 && !loading ? (
            <tr>
              <td colSpan={5} className='py-6 text-center text-xs text-muted-foreground'>
                {t('empty')}
              </td>
            </tr>
          ) : null}
        </tbody>
      </table>
      {presets.length > 0 ? (
        <div className='space-y-1 border-t pt-2'>
          <p className='text-xs font-medium text-muted-foreground'>{t('officialPresets')}</p>
          <div className='flex flex-wrap gap-1.5'>
            {presets.map((preset) => (
              <Badge key={preset.image} variant='outline' className='font-mono text-[10px]'>
                {preset.image}
              </Badge>
            ))}
          </div>
        </div>
      ) : null}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t('editTitle') : t('createTitle')}</DialogTitle>
          </DialogHeader>
          <div className='space-y-2'>
            <Input placeholder={t('fieldName')} value={form.name} onChange={(e) => setForm({...form, name: e.target.value})} data-testid='image-form-name' />
            <Input placeholder='apache/seatunnel:2.3.13' value={form.image} onChange={(e) => setForm({...form, image: e.target.value})} data-testid='image-form-image' />
            <Input placeholder={t('fieldVersion')} value={form.seatunnel_version} onChange={(e) => setForm({...form, seatunnel_version: e.target.value})} />
            <Input placeholder={t('fieldPullSecrets')} value={form.image_pull_secrets} onChange={(e) => setForm({...form, image_pull_secrets: e.target.value})} />
            <Input placeholder={t('fieldDescription')} value={form.description} onChange={(e) => setForm({...form, description: e.target.value})} />
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox checked={form.is_default} onCheckedChange={(checked) => setForm({...form, is_default: checked === true})} />
              {t('fieldDefault')}
            </label>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setDialogOpen(false)}>{t('cancel')}</Button>
            <Button onClick={() => void save()} data-testid='image-form-save'>{t('save')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
