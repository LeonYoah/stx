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

/**
 * Upload Package Dialog Component
 * 上传安装包对话框组件
 */

'use client';

import {useState} from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Progress} from '@/components/ui/progress';
import {FileUpload} from '@/components/ui/file-upload';
import {AlertCircle} from 'lucide-react';
import {useTranslations} from 'next-intl';

interface UploadPackageDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUpload: (
    file: File,
    version: string,
    sourceFile?: File,
    onProgress?: (percent: number) => void,
  ) => Promise<void>;
  existingLocalVersions: string[];
}

export function UploadPackageDialog({
  open,
  onOpenChange,
  onUpload,
  existingLocalVersions,
}: UploadPackageDialogProps) {
  const t = useTranslations();
  const [file, setFile] = useState<File | null>(null);
  const [sourceFile, setSourceFile] = useState<File | null>(null);
  const [version, setVersion] = useState('');
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);

  // 从文件名提取版本 / Extract version from filename
  const extractVersion = (filename: string): string => {
    // Format: apache-seatunnel-{version}-bin.tar.gz
    const match = filename.match(/apache-seatunnel-(.+)-bin\.tar\.gz/);
    return match ? match[1] : '';
  };

  // 处理运行包选择 / Handle runtime package selection
  const handleFileSelect = (selectedFile: File | null) => {
    setFile(selectedFile);
    setError(null);
    if (!selectedFile) {
      return;
    }
    const extractedVersion = extractVersion(selectedFile.name);
    if (extractedVersion) {
      setVersion(extractedVersion);
    }
  };

  // 验证并上传 / Validate and upload
  const handleUpload = async () => {
    if (!file || !version) {
      setError(t('installer.pleaseSelectFileAndVersion'));
      return;
    }

    // 验证文件扩展名 / Validate file extension
    if (!file.name.endsWith('.tar.gz')) {
      setError(t('installer.invalidFileFormat'));
      return;
    }

    try {
      setUploading(true);
      setProgress(0);
      setError(null);

      await onUpload(file, version, sourceFile || undefined, (percent) => {
        setProgress(percent);
      });

      setProgress(100);

      // 重置并关闭 / Reset and close
      setTimeout(() => {
        setFile(null);
        setSourceFile(null);
        setVersion('');
        setProgress(0);
        onOpenChange(false);
      }, 500);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('installer.uploadFailed'));
    } finally {
      setUploading(false);
    }
  };

  // 对话框关闭时重置状态 / Reset state when dialog closes
  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setFile(null);
      setSourceFile(null);
      setVersion('');
      setProgress(0);
      setError(null);
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-[500px]' aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{t('installer.uploadPackage')}</DialogTitle>
        </DialogHeader>

        <div className='space-y-4 py-4'>
          {/* 运行包拖放区 / Runtime package dropzone */}
          <FileUpload
            variant='dropzone'
            accept='.tar.gz'
            value={file}
            uploading={uploading}
            emptyLabel={t('installer.dragDropOrClick')}
            emptyHint={t('installer.supportedFormat')}
            data-testid='upload-package-bin'
            onFileChange={handleFileSelect}
          />

          {/* 可选源码包：运行包上传成功后再补传，失败不会撤销运行包。 / Optional source archive: uploaded after runtime; failure does not remove runtime. */}
          <div className='space-y-2'>
            <Label htmlFor='source-file'>
              {t('installer.sourcePackageOptional')}
            </Label>
            <FileUpload
              id='source-file'
              variant='input'
              accept='.tar.gz'
              value={sourceFile}
              uploading={uploading}
              showSelectedFile={false}
              data-testid='upload-package-source'
              onFileChange={setSourceFile}
            />
            <p className='text-xs text-muted-foreground'>
              {/* 文件名已在控件内展示，这里只保留说明 / The control already shows the file name; keep only the hint */}
              {t('installer.sourcePackageHint')}
            </p>
          </div>

          {/* 版本输入 / Version input */}
          <div className='space-y-2'>
            <Label htmlFor='version'>{t('installer.version')}</Label>
            <Input
              id='version'
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder={
                existingLocalVersions[0]
                  ? `e.g., ${existingLocalVersions[0]}`
                  : 'e.g., 2.x.y'
              }
              disabled={uploading}
            />
            <p className='text-xs text-muted-foreground'>
              {t('installer.versionHint')}
            </p>
          </div>

          {/* 进度条 / Progress bar */}
          {uploading && (
            <div className='space-y-2'>
              <Progress value={progress} />
              <p className='text-sm text-center text-muted-foreground'>
                {t('installer.uploading')} {progress}%
              </p>
            </div>
          )}

          {/* 表单级错误（版本缺失 / 格式 / 上传失败） / Form-level errors (version, format, upload) */}
          {error && (
            <div className='flex items-center gap-2 text-destructive text-sm'>
              <AlertCircle className='h-4 w-4' />
              {error}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={uploading}
          >
            {t('common.cancel')}
          </Button>
          <Button
            onClick={handleUpload}
            disabled={!file || !version || uploading}
          >
            {uploading ? t('installer.uploading') : t('installer.upload')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
