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

import { useState, useCallback } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Progress } from '@/components/ui/progress';
import { Upload, File, X, AlertCircle } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { cn } from '@/lib/utils';

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
  const [dragActive, setDragActive] = useState(false);

  // Extract version from filename / 从文件名提取版本
  const extractVersion = (filename: string): string => {
    // Format: apache-seatunnel-{version}-bin.tar.gz
    const match = filename.match(/apache-seatunnel-(.+)-bin\.tar\.gz/);
    return match ? match[1] : '';
  };

  // Handle file selection / 处理文件选择
  const handleFileSelect = (selectedFile: File) => {
    setFile(selectedFile);
    setError(null);

    // Auto-extract version from filename / 自动从文件名提取版本
    const extractedVersion = extractVersion(selectedFile.name);
    if (extractedVersion) {
      setVersion(extractedVersion);
    }
  };

  // Handle drag events / 处理拖拽事件
  const handleDrag = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  }, []);

  // Handle drop / 处理放置
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);

    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      handleFileSelect(e.dataTransfer.files[0]);
    }
  };

  // Handle file input change / 处理文件输入变化
  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      handleFileSelect(e.target.files[0]);
    }
  };

  // Validate and upload / 验证并上传
  const handleUpload = async () => {
    if (!file || !version) {
      setError(t('installer.pleaseSelectFileAndVersion'));
      return;
    }

    // Validate file extension / 验证文件扩展名
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

      // Reset and close / 重置并关闭
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

  // Reset state when dialog closes / 对话框关闭时重置状态
  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setFile(null);
      setSourceFile(null);
      setVersion('');
      setProgress(0);
      setError(null);
    }
    onOpenChange(open);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>{t('installer.uploadPackage')}</DialogTitle>
          <DialogDescription>
            {t('installer.uploadPackageDesc')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Drop zone / 拖放区域 */}
          <div
            className={cn(
              'relative border-2 border-dashed rounded-lg p-8 text-center transition-colors',
              dragActive ? 'border-primary bg-primary/5' : 'border-muted-foreground/25',
              file ? 'bg-muted/50' : ''
            )}
            onDragEnter={handleDrag}
            onDragLeave={handleDrag}
            onDragOver={handleDrag}
            onDrop={handleDrop}
          >
            {file ? (
              <div className="flex items-center justify-center gap-3">
                <File className="h-8 w-8 text-primary" />
                <div className="text-left">
                  <p className="font-medium">{file.name}</p>
                  <p className="text-sm text-muted-foreground">
                    {(file.size / 1024 / 1024).toFixed(2)} MB
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => setFile(null)}
                  disabled={uploading}
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            ) : (
              <div className="space-y-2">
                <Upload className="h-10 w-10 mx-auto text-muted-foreground" />
                <p className="text-muted-foreground">
                  {t('installer.dragDropOrClick')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t('installer.supportedFormat')}
                </p>
                <input
                  type="file"
                  accept=".tar.gz"
                  onChange={handleFileInputChange}
                  className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
                  disabled={uploading}
                />
              </div>
            )}
          </div>

          {/* 可选源码包：运行包上传成功后再补传，失败不会撤销运行包。 / Optional source archive: uploaded after runtime; failure does not remove runtime. */}
          <div className="space-y-2">
            <Label htmlFor="source-file">{t('installer.sourcePackageOptional')}</Label>
            <Input
              id="source-file"
              type="file"
              accept=".tar.gz"
              disabled={uploading}
              onChange={(event) => setSourceFile(event.target.files?.[0] || null)}
            />
            <p className="text-xs text-muted-foreground">
              {sourceFile ? sourceFile.name : t('installer.sourcePackageHint')}
            </p>
          </div>

          {/* Version input / 版本输入 */}
          <div className="space-y-2">
            <Label htmlFor="version">{t('installer.version')}</Label>
            <Input
              id="version"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder={existingLocalVersions[0] ? `e.g., ${existingLocalVersions[0]}` : 'e.g., 2.x.y'}
              disabled={uploading}
            />
            <p className="text-xs text-muted-foreground">
              {t('installer.versionHint')}
            </p>
          </div>

          {/* Progress bar / 进度条 */}
          {uploading && (
            <div className="space-y-2">
              <Progress value={progress} />
              <p className="text-sm text-center text-muted-foreground">
                {t('installer.uploading')} {progress}%
              </p>
            </div>
          )}

          {/* Error message / 错误消息 */}
          {error && (
            <div className="flex items-center gap-2 text-destructive text-sm">
              <AlertCircle className="h-4 w-4" />
              {error}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={uploading}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleUpload} disabled={!file || !version || uploading}>
            {uploading ? t('installer.uploading') : t('installer.upload')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
