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

/**
 * 离线导入 / 导出：统一入口。
 * - 导入：自动识别「离线资产包」或「单个 SeaTunnel bin」
 * - 导出：从本地仓打包安装包（± 插件）为 tar.gz
 * Offline import / export in one dialog.
 * - Import: auto-detect asset bundle vs single SeaTunnel bin archive
 * - Export: pack local package (± plugins) into tar.gz
 */

import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from '@/lib/toast';
import {
  ChevronDown,
  ChevronRight,
  Download,
  Loader2,
  PackageOpen,
} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {FileUpload} from '@/components/ui/file-upload';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {
  createOfflineBundle,
  downloadOfflineBundle,
  importOfflineBundle,
  listOfflineBundles,
} from '@/lib/services/installer';
import type {OfflineBundleInfo, PackageInfo} from '@/lib/services/installer';
import {
  extractSeatunnelVersionFromPackageFileName,
  isRecommendedSeatunnelVersionFormat,
} from '@/lib/services/installer/package-version';

type OfflineBundleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  localPackages: PackageInfo[];
  /** 打开完整「单包上传」表单（含源码包）/ Open full single-package upload form (incl. source) */
  onOpenSinglePackageUpload?: () => void;
  /** 单包上传到本地仓 / Upload a single bin archive into local cache */
  onUploadSinglePackage?: (file: File, version: string) => Promise<void>;
  onImported?: () => void;
};

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = size;
  let idx = 0;
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024;
    idx += 1;
  }
  return `${value.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`;
}

/** 是否为 STX 离线资产包文件名 / Whether filename looks like an STX offline asset bundle */
function isOfflineAssetBundleName(fileName: string): boolean {
  return /stx-seatunnel-offline-.*\.tar\.gz$/i.test(fileName.trim());
}

export function OfflineBundleDialog({
  open,
  onOpenChange,
  localPackages,
  onOpenSinglePackageUpload,
  onUploadSinglePackage,
  onImported,
}: OfflineBundleDialogProps) {
  const t = useTranslations('installer');
  const commonT = useTranslations('common');
  const [activeTab, setActiveTab] = useState<'import' | 'export'>('import');
  const [version, setVersion] = useState('');
  const [includePlugins, setIncludePlugins] = useState(true);
  const [includeSource, setIncludeSource] = useState(false);
  const [creating, setCreating] = useState(false);
  const [importing, setImporting] = useState(false);
  const [downloading, setDownloading] = useState<string | null>(null);
  const [bundles, setBundles] = useState<OfflineBundleInfo[]>([]);
  const [loadingList, setLoadingList] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importVersion, setImportVersion] = useState('');
  const [importError, setImportError] = useState<string | null>(null);
  // null = 跟随是否已有本地安装包；用户点过之后以用户选择为准 / null follows local-package availability until the user toggles it
  const [prereqOpen, setPrereqOpen] = useState<boolean | null>(null);
  const prereqExpanded = prereqOpen ?? localPackages.length === 0;

  // 每次打开都回到默认展开规则（父组件不卸载本弹窗）/ Reset to the default rule on every open (the parent keeps this dialog mounted)
  useEffect(() => {
    if (open) {
      setPrereqOpen(null);
    }
  }, [open]);

  const refreshList = useCallback(async () => {
    setLoadingList(true);
    try {
      setBundles(await listOfflineBundles());
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('offlineBundleListFailed'),
      );
    } finally {
      setLoadingList(false);
    }
  }, [t]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const first = localPackages[0]?.version || '';
    setVersion((prev) => prev || first);
    setImportFile(null);
    setImportVersion('');
    setImportError(null);
    // 有本地包时默认导出；否则默认导入（无网首装更常见）。
    // Prefer export when local packages exist; otherwise import (typical offline first install).
    setActiveTab(localPackages.length > 0 ? 'export' : 'import');
    void refreshList();
  }, [open, localPackages, refreshList]);

  const handleCreate = async () => {
    if (!version) {
      toast.error(t('offlineBundleSelectVersion'));
      return;
    }
    setCreating(true);
    try {
      const info = await createOfflineBundle({
        version,
        include_plugins: includePlugins,
        include_source: includeSource,
      });
      toast.success(t('offlineBundleCreated', {name: info.file_name}));
      await refreshList();
      await downloadOfflineBundle(info.file_name);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('offlineBundleCreateFailed'),
      );
    } finally {
      setCreating(false);
    }
  };

  const handleDownload = async (bundle: OfflineBundleInfo) => {
    setDownloading(bundle.file_name);
    try {
      await downloadOfflineBundle(bundle.file_name);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t('offlineBundleDownloadFailed'),
      );
    } finally {
      setDownloading(null);
    }
  };

  const handleImportFileSelect = (selectedFile: File | null) => {
    setImportError(null);
    if (!selectedFile) {
      setImportFile(null);
      setImportVersion('');
      return;
    }
    if (!selectedFile.name.toLowerCase().endsWith('.tar.gz')) {
      setImportFile(null);
      setImportVersion('');
      setImportError(t('invalidFileFormat'));
      return;
    }
    setImportFile(selectedFile);
    if (isOfflineAssetBundleName(selectedFile.name)) {
      setImportVersion('');
      return;
    }
    setImportVersion(
      extractSeatunnelVersionFromPackageFileName(selectedFile.name),
    );
  };

  const handleImportSelectedFile = async () => {
    if (!importFile) {
      setImportError(t('offlineImportChooseFileFirst'));
      return;
    }
    const name = importFile.name;
    if (!name.toLowerCase().endsWith('.tar.gz')) {
      setImportError(t('invalidFileFormat'));
      return;
    }

    if (isOfflineAssetBundleName(name)) {
      setImporting(true);
      setImportError(null);
      try {
        const info = await importOfflineBundle(importFile);
        toast.success(
          t('offlineBundleImported', {version: info.seatunnel_version}),
        );
        setImportFile(null);
        setImportVersion('');
        await refreshList();
        onImported?.();
      } catch (err) {
        setImportError(
          err instanceof Error ? err.message : t('offlineBundleImportFailed'),
        );
      } finally {
        setImporting(false);
      }
      return;
    }

    const packageVersion = importVersion.trim();
    if (!packageVersion) {
      setImportError(t('offlineImportVersionRequired'));
      return;
    }
    if (!onUploadSinglePackage) {
      setImportError(t('offlineImportSingleFailed'));
      return;
    }

    setImporting(true);
    setImportError(null);
    try {
      await onUploadSinglePackage(importFile, packageVersion);
      toast.success(t('offlineImportSingleDone', {version: packageVersion}));
      setImportFile(null);
      setImportVersion('');
      onImported?.();
    } catch (err) {
      setImportError(
        err instanceof Error ? err.message : t('offlineImportSingleFailed'),
      );
    } finally {
      setImporting(false);
    }
  };

  const selectedImportIsBundle = importFile
    ? isOfflineAssetBundleName(importFile.name)
    : false;
  const trimmedImportVersion = importVersion.trim();
  const showImportVersionWarning =
    !selectedImportIsBundle &&
    trimmedImportVersion.length > 0 &&
    !isRecommendedSeatunnelVersionFormat(trimmedImportVersion);
  const canImport =
    importFile != null &&
    !importing &&
    (selectedImportIsBundle || trimmedImportVersion.length > 0);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='flex max-h-[88vh] w-[95vw] max-w-2xl flex-col overflow-hidden border border-border/80 shadow-2xl sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <PackageOpen className='h-5 w-5 text-primary' />
            {t('offlineAssetsTitle')}
          </DialogTitle>
          <DialogDescription>{t('offlineAssetsDesc')}</DialogDescription>
        </DialogHeader>

        <Tabs
          value={activeTab}
          onValueChange={(v) => setActiveTab(v as 'import' | 'export')}
          className='min-h-0 flex-1 flex flex-col overflow-hidden'
        >
          <TabsList className='grid h-9 w-full shrink-0 grid-cols-2'>
            <TabsTrigger
              value='import'
              className='text-xs'
              data-testid='offline-assets-tab-import'
            >
              {t('offlineAssetsTabImport')}
            </TabsTrigger>
            <TabsTrigger
              value='export'
              className='text-xs'
              data-testid='offline-assets-tab-export'
            >
              {t('offlineAssetsTabExport')}
            </TabsTrigger>
          </TabsList>

          <div className='min-h-0 flex-1 overflow-y-auto py-3 pr-1'>
            <TabsContent value='import' className='mt-0 space-y-4'>
              <ul className='space-y-1.5 rounded-lg border bg-muted/20 px-3.5 py-3 text-xs text-muted-foreground'>
                <li>{t('offlineImportHintBundle')}</li>
                <li>{t('offlineImportHintSingle')}</li>
              </ul>
              <FileUpload
                id='offline-assets-import-input'
                variant='input'
                accept='.tar.gz,application/gzip'
                value={importFile}
                uploading={importing}
                resetOnSelect
                showSelectedFile={false}
                emptyLabel={t('offlineImportChooseFile')}
                error={importError}
                data-testid='offline-assets-import'
                onFileChange={handleImportFileSelect}
              />
              {importFile ? (
                <div className='rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground'>
                  {selectedImportIsBundle
                    ? t('offlineImportKindBundle')
                    : t('offlineImportKindPackage')}
                </div>
              ) : null}
              {importFile && !selectedImportIsBundle ? (
                <div className='space-y-1.5'>
                  <Label htmlFor='offline-import-version' className='text-xs'>
                    {t('version')}
                    <span className='ml-0.5 text-destructive'>*</span>
                  </Label>
                  <Input
                    id='offline-import-version'
                    value={importVersion}
                    disabled={importing}
                    placeholder='3.1.0'
                    data-testid='offline-import-version'
                    onChange={(event) => {
                      setImportVersion(event.target.value);
                      setImportError(null);
                    }}
                  />
                  <p className='text-xs text-muted-foreground'>
                    {t('offlineImportVersionHint')}
                  </p>
                  {showImportVersionWarning ? (
                    <p
                      className='text-xs text-amber-600 dark:text-amber-400'
                      data-testid='offline-import-version-warning'
                    >
                      {t('offlineImportVersionFormatWarning')}
                    </p>
                  ) : null}
                </div>
              ) : null}
              <Button
                className='w-full'
                disabled={!canImport}
                data-testid='offline-import-submit'
                onClick={() => void handleImportSelectedFile()}
              >
                {importing ? (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                ) : null}
                {t('offlineImportStart')}
              </Button>
              {onOpenSinglePackageUpload ? (
                <Button
                  variant='link'
                  size='sm'
                  className='h-auto px-0 text-xs'
                  disabled={importing}
                  onClick={() => {
                    onOpenChange(false);
                    onOpenSinglePackageUpload();
                  }}
                >
                  {t('offlineImportAdvancedUpload')}
                </Button>
              ) : null}
            </TabsContent>

            <TabsContent value='export' className='mt-0 space-y-4'>
              {/* 打包前置步骤默认折叠，没有本地安装包时自动展开 / Prerequisites collapse by default and expand when no local package exists */}
              <div className='rounded-lg border'>
                <button
                  type='button'
                  aria-expanded={prereqExpanded}
                  aria-controls='offline-bundle-prereq'
                  data-testid='offline-bundle-prereq-toggle'
                  onClick={() => setPrereqOpen(!prereqExpanded)}
                  className='flex w-full items-center gap-1.5 rounded-lg px-3.5 py-2.5 text-left text-xs font-medium text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
                >
                  {prereqExpanded ? (
                    <ChevronDown className='h-3.5 w-3.5 shrink-0' />
                  ) : (
                    <ChevronRight className='h-3.5 w-3.5 shrink-0' />
                  )}
                  {t('offlineBundlePrereqTitle')}
                </button>
                {/* 始终渲染，保证 aria-controls 指向存在的元素 / Always rendered so aria-controls points at a real element */}
                <ol
                  id='offline-bundle-prereq'
                  hidden={!prereqExpanded}
                  className='list-decimal space-y-1.5 px-3.5 pb-3 pl-8 text-xs leading-relaxed text-muted-foreground'
                >
                  <li>{t('offlineBundlePrereqStep1')}</li>
                  <li>{t('offlineBundlePrereqStep2')}</li>
                  <li>{t('offlineBundlePrereqStep3')}</li>
                </ol>
              </div>

              <div className='space-y-3 rounded-lg border bg-muted/20 p-3.5'>
                <div className='space-y-1.5'>
                  <Label className='text-xs'>{t('version')}</Label>
                  <Select value={version} onValueChange={setVersion}>
                    <SelectTrigger
                      className='h-9'
                      data-testid='offline-bundle-export-version'
                    >
                      <SelectValue
                        placeholder={t('offlineBundleSelectVersion')}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {localPackages.map((pkg) => (
                        <SelectItem key={pkg.version} value={pkg.version}>
                          v{pkg.version}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {localPackages.length === 0 ? (
                    <p className='text-xs text-amber-600 dark:text-amber-400'>
                      {t('offlineBundleNeedLocalPackage')}
                    </p>
                  ) : null}
                </div>

                <label className='flex items-center gap-2 text-sm'>
                  <Checkbox
                    data-testid='offline-bundle-include-plugins'
                    checked={includePlugins}
                    onCheckedChange={(v) => setIncludePlugins(v === true)}
                  />
                  <span>{t('offlineBundleIncludePlugins')}</span>
                </label>
                <label className='flex items-center gap-2 text-sm'>
                  <Checkbox
                    data-testid='offline-bundle-include-source'
                    checked={includeSource}
                    onCheckedChange={(v) => setIncludeSource(v === true)}
                  />
                  <span>{t('offlineBundleIncludeSource')}</span>
                </label>

                <Button
                  className='w-full'
                  disabled={!version || creating || localPackages.length === 0}
                  data-testid='offline-bundle-create-submit'
                  onClick={() => void handleCreate()}
                >
                  {creating ? (
                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                  ) : (
                    <Download className='mr-2 h-4 w-4' />
                  )}
                  {t('offlineBundlePackAndDownload')}
                </Button>
              </div>

              <div className='space-y-2'>
                <div className='flex items-center justify-between gap-2'>
                  <h4 className='text-sm font-medium'>
                    {t('offlineBundleExisting')}
                  </h4>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='h-7 text-xs'
                    disabled={loadingList}
                    onClick={() => void refreshList()}
                  >
                    {commonT('refresh')}
                  </Button>
                </div>
                {bundles.length === 0 ? (
                  <p className='text-xs text-muted-foreground'>
                    {t('offlineBundleEmpty')}
                  </p>
                ) : (
                  <ul className='space-y-2'>
                    {bundles.map((bundle) => (
                      <li
                        key={bundle.file_name}
                        className='flex items-center justify-between gap-2 rounded-md border px-3 py-2 text-xs'
                      >
                        <div className='min-w-0'>
                          <div className='truncate font-mono font-medium'>
                            {bundle.file_name}
                          </div>
                          <div className='text-muted-foreground'>
                            v{bundle.seatunnel_version} ·{' '}
                            {formatBytes(bundle.file_size)}
                            {bundle.include_plugins
                              ? ` · ${t('offlineBundlePluginCount', {
                                  count: bundle.plugin_count,
                                })}`
                              : ''}
                          </div>
                        </div>
                        <Button
                          variant='outline'
                          size='sm'
                          className='h-7 shrink-0'
                          disabled={downloading === bundle.file_name}
                          onClick={() => void handleDownload(bundle)}
                        >
                          {downloading === bundle.file_name ? (
                            <Loader2 className='h-3.5 w-3.5 animate-spin' />
                          ) : (
                            <Download className='h-3.5 w-3.5' />
                          )}
                        </Button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </TabsContent>
          </div>
        </Tabs>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {commonT('close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
