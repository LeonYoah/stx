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
 * 下载在线安装包对话框
 * Download online package dialog
 */

'use client';

import {useEffect, useState} from 'react';
import {AlertCircle, Download, FileCode2, Loader2} from 'lucide-react';
import {useTranslations} from 'next-intl';
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
import {Label} from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type {MirrorSource} from '@/lib/services/installer/types';

interface DownloadPackageDialogProps {
  open: boolean;
  version: string | null;
  onOpenChange: (open: boolean) => void;
  onDownload: (
    version: string,
    mirror: MirrorSource,
    withSource: boolean,
    idempotencyKey: string,
  ) => Promise<void>;
}

// 为一次下载意图生成稳定键，同一弹窗内重试时复用。
// Create a stable key for one download intent and reuse it for retries within the same dialog.
function createDownloadIdempotencyKey(version: string): string {
  const safeVersion = version.replace(/[^0-9A-Za-z_-]/g, '_');
  const randomPart = Math.random().toString(36).slice(2, 10);
  return `web-download-${safeVersion}-${Date.now()}-${randomPart}`;
}

export function DownloadPackageDialog({
  open,
  version,
  onOpenChange,
  onDownload,
}: DownloadPackageDialogProps) {
  const t = useTranslations();
  const [mirror, setMirror] = useState<MirrorSource>('aliyun');
  const [withSource, setWithSource] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [idempotencyKey, setIdempotencyKey] = useState('');

  // 每次打开时恢复推荐选项，避免上一次操作影响新的下载。
  // Restore recommended options on each open so a previous action does not affect a new download.
  useEffect(() => {
    if (open) {
      setMirror('aliyun');
      setWithSource(true);
      setError(null);
      setIdempotencyKey(createDownloadIdempotencyKey(version || 'unknown'));
    }
  }, [open, version]);

  // 提交下载并仅在服务端成功接收任务后关闭窗口。
  // Submit the download and close only after the server accepts the task.
  const handleDownload = async () => {
    if (!version) {
      return;
    }

    try {
      setSubmitting(true);
      setError(null);
      await onDownload(version, mirror, withSource, idempotencyKey);
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('installer.downloadFailed'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => !submitting && onOpenChange(nextOpen)}>
      <DialogContent className="border border-border/80 shadow-2xl sm:max-w-md dark:border-border/60">
        <DialogHeader>
          <DialogTitle>{t('installer.downloadPackageTitle')}</DialogTitle>
          <DialogDescription>{t('installer.downloadPackageDesc')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="rounded-md border bg-muted/30 px-3 py-2.5">
            <span className="text-xs text-muted-foreground">{t('installer.downloadVersion')}</span>
            <p className="mt-0.5 font-mono text-sm font-medium">SeaTunnel {version}</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="download-mirror">{t('installer.mirrorSource')}</Label>
            <Select
              value={mirror}
              onValueChange={(value) => setMirror(value as MirrorSource)}
              disabled={submitting}
            >
              <SelectTrigger id="download-mirror" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="aliyun">{t('installer.mirrors.aliyun')}</SelectItem>
                <SelectItem value="huaweicloud">{t('installer.mirrors.huaweicloud')}</SelectItem>
                <SelectItem value="apache">{t('installer.mirrors.apache')}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="rounded-md border px-3 py-3">
            <div className="flex items-start gap-3">
              <Checkbox
                id="include-source"
                checked={withSource}
                disabled={submitting}
                onCheckedChange={(checked) => setWithSource(checked === true)}
                className="mt-0.5"
              />
              <div className="space-y-1">
                <Label htmlFor="include-source" className="flex cursor-pointer items-center gap-1.5">
                  <FileCode2 className="h-4 w-4" />
                  {t('installer.includeSource')}
                </Label>
                <p className="text-xs leading-5 text-muted-foreground">
                  {t('installer.includeSourceHint')}
                </p>
              </div>
            </div>
          </div>

          <p className="text-xs text-muted-foreground">
            {t('installer.sourceMirrorAvailable')}
          </p>

          {error && (
            <div className="flex items-start gap-2 rounded-md bg-destructive/10 p-2.5 text-sm text-destructive">
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleDownload} disabled={!version || submitting}>
            {submitting ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Download className="mr-2 h-4 w-4" />
            )}
            {submitting ? t('installer.startingDownload') : t('installer.startDownload')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
