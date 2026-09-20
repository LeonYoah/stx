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
 * Package Table Component
 * 安装包表格组件
 */

'use client';

import { useRef } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Progress } from '@/components/ui/progress';
import { Download, Trash2, MoreHorizontal, Star, Loader2, CheckCircle, XCircle, FileCode2, Upload } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useGSAP } from '@gsap/react';
import { TableLoadingBar, TableSkeletonRows } from '@/components/common/layout';
import { animateTableRows } from '@/lib/animations/gsap-motion';
import type { PackageInfo, DownloadTask } from '@/lib/services/installer/types';

interface PackageTableProps {
  type: 'online' | 'local';
  versions?: string[];
  localPackages?: PackageInfo[];
  recommendedVersion?: string;
  loading?: boolean;
  onDelete?: (version: string) => void;
  onDownloadRequest?: (version: string) => void;
  downloads?: DownloadTask[];
  onSourceUpload?: (version: string, file: File) => void;
  onSourceFetch?: (version: string) => void;
  onSourceDownload?: (version: string) => void;
}

// Format file size / 格式化文件大小
function formatFileSize(bytes: number): string {
  if (bytes === 0) {
    return '0 B';
  }
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Format date / 格式化日期
function formatDate(dateStr?: string): string {
  if (!dateStr) {
    return '-';
  }
  return new Date(dateStr).toLocaleString();
}

export function PackageTable({
  type,
  versions = [],
  localPackages = [],
  recommendedVersion,
  loading,
  onDelete,
  onDownloadRequest,
  downloads = [],
  onSourceUpload,
  onSourceFetch,
  onSourceDownload,
}: PackageTableProps) {
  const t = useTranslations();

  // Get download task for a version / 获取某版本的下载任务
  const getDownloadTask = (version: string): DownloadTask | undefined => {
    return downloads.find((d) => d.version === version);
  };

  // Check if version is already downloaded (exists in local packages)
  // 检查版本是否已下载（存在于本地安装包中）
  const isVersionDownloaded = (version: string): boolean => {
    return localPackages.some((pkg) => pkg.version === version);
  };

  // Format speed / 格式化速度
  const formatSpeed = (bytesPerSecond: number): string => {
    if (bytesPerSecond < 1024) {
      return `${bytesPerSecond} B/s`;
    }
    if (bytesPerSecond < 1024 * 1024) {
      return `${(bytesPerSecond / 1024).toFixed(1)} KB/s`;
    }
    return `${(bytesPerSecond / 1024 / 1024).toFixed(1)} MB/s`;
  };

  const tableContainerRef = useRef<HTMLDivElement>(null);

  // 安装包列表数据更新后执行平滑交错入场动效
  // Trigger staggered entrance animation when package data updates
  useGSAP(
    () => {
      const hasData = type === 'online' ? versions.length > 0 : localPackages.length > 0;
      if (hasData && !loading) {
        animateTableRows('.package-data-row');
      }
    },
    {dependencies: [type, versions, localPackages, loading], scope: tableContainerRef},
  );

  // Online versions table / 在线版本表格
  if (type === 'online') {
    return (
      <div ref={tableContainerRef} className="border rounded-lg relative overflow-hidden bg-card/40 shadow-xs flex-1 flex flex-col">
        <TableLoadingBar loading={Boolean(loading)} />
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('installer.version')}</TableHead>
              <TableHead>{t('installer.status')}</TableHead>
              <TableHead>{t('installer.sourcePackage')}</TableHead>
              <TableHead>{t('installer.downloadLinks')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && versions.length === 0 ? (
              <TableSkeletonRows columns={4} rows={5} />
            ) : versions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="text-center py-12 text-muted-foreground">
                  {t('installer.noVersionsAvailable')}
                </TableCell>
              </TableRow>
            ) : (
              versions.map((version) => (
                <TableRow
                  key={version}
                  className={`package-data-row transition-opacity duration-200 ${
                    loading ? 'opacity-60 pointer-events-none' : ''
                  }`}
                >
                  <TableCell className="font-medium">
                    <div className="flex items-center gap-2">
                      {version}
                      {version === recommendedVersion && (
                        <Badge variant="default" className="flex items-center gap-1">
                          <Star className="h-3 w-3" />
                          {t('installer.recommended')}
                        </Badge>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline">{t('installer.available')}</Badge>
                  </TableCell>
                  <TableCell>
                    {(() => {
                      const localPackage = localPackages.find((pkg) => pkg.version === version);
                      const task = getDownloadTask(version);

                      // 优先展示本地文件状态，其次展示当前下载任务的源码状态。
                      // Prefer local file state, then show source state from the active download task.
                      if (localPackage?.has_source || task?.source_status === 'completed') {
                        return (
                          <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">
                            <CheckCircle className="h-3 w-3 mr-1" />
                            {t('installer.sourceDownloaded')}
                          </Badge>
                        );
                      }

                      if (
                        task?.source_requested
                        && (task.source_status === 'pending' || task.source_status === 'downloading')
                      ) {
                        return (
                          <div className="flex items-center gap-2 min-w-[150px]">
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                            <div className="flex-1">
                              <Progress value={task.source_progress || 0} className="h-1.5" />
                              <span className="mt-1 block text-xs text-muted-foreground">
                                {t('installer.sourceDownloading')} {task.source_progress || 0}%
                              </span>
                            </div>
                          </div>
                        );
                      }

                      if (localPackage || task?.source_status === 'failed') {
                        return (
                          <Badge variant="secondary" title={task?.source_error}>
                            {task?.source_status === 'failed'
                              ? t('installer.sourceFailed')
                              : t('installer.sourceMissing')}
                          </Badge>
                        );
                      }

                      return (
                        <Badge variant="outline" className="text-muted-foreground">
                          <FileCode2 className="h-3 w-3 mr-1" />
                          {t('installer.sourceAvailable')}
                        </Badge>
                      );
                    })()}
                  </TableCell>
                  <TableCell>
                    {(() => {
                      const task = getDownloadTask(version);
                      const isDownloaded = isVersionDownloaded(version);

                      // Show downloaded status (check local files first)
                      // 显示已下载状态（优先检查本地文件）
                      if (isDownloaded) {
                        return (
                          <div className="flex items-center gap-2 text-green-600">
                            <CheckCircle className="h-4 w-4" />
                            <span className="text-sm">{t('installer.downloaded')}</span>
                          </div>
                        );
                      }

                      // Show download progress / 显示下载进度
                      if (task && (task.status === 'downloading' || task.status === 'pending')) {
                        return (
                          <div className="flex items-center gap-2 min-w-[200px]">
                            <Loader2 className="h-4 w-4 animate-spin" />
                            <div className="flex-1">
                              <Progress value={task.progress} className="h-2" />
                              <div className="text-xs text-muted-foreground mt-1">
                                {task.progress}% - {formatSpeed(task.speed)}
                              </div>
                            </div>
                          </div>
                        );
                      }

                      // Show completed status (just finished downloading)
                      // 显示完成状态（刚刚下载完成）
                      if (task?.status === 'completed') {
                        return (
                          <div className="flex items-center gap-2 text-green-600">
                            <CheckCircle className="h-4 w-4" />
                            <span className="text-sm">{t('installer.downloadSuccess')}</span>
                          </div>
                        );
                      }

                      // Show failed status / 显示失败状态
                      if (task?.status === 'failed') {
                        return (
                          <div className="flex items-center gap-2">
                            <div className="flex items-center gap-1 text-destructive">
                              <XCircle className="h-4 w-4" />
                              <span className="text-sm">{t('installer.downloadFailed')}</span>
                            </div>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => onDownloadRequest?.(version)}
                            >
                              <Download className="h-4 w-4 mr-2" />
                              {t('installer.retry')}
                            </Button>
                          </div>
                        );
                      }

                      // Show download button / 显示下载按钮
                      return (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => onDownloadRequest?.(version)}
                        >
                          <Download className="h-4 w-4 mr-2" />
                          {t('installer.downloadToServer')}
                        </Button>
                      );
                    })()}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    );
  }

  // Local packages table / 本地安装包表格
  return (
    <div ref={tableContainerRef} className="border rounded-lg relative overflow-hidden bg-card/40 shadow-xs flex-1 flex flex-col">
      <TableLoadingBar loading={Boolean(loading)} />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('installer.version')}</TableHead>
            <TableHead>{t('installer.fileName')}</TableHead>
            <TableHead>{t('installer.fileSize')}</TableHead>
            <TableHead>{t('installer.sourcePackage')}</TableHead>
            <TableHead>{t('installer.uploadedAt')}</TableHead>
            <TableHead className="w-[100px]">{t('common.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading && localPackages.length === 0 ? (
            <TableSkeletonRows columns={6} rows={5} />
          ) : localPackages.length === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="text-center py-12 text-muted-foreground">
                {t('installer.noLocalPackages')}
              </TableCell>
            </TableRow>
          ) : (
            localPackages.map((pkg) => (
              <TableRow
                key={pkg.version}
                className={`package-data-row transition-opacity duration-200 ${
                  loading ? 'opacity-60 pointer-events-none' : ''
                }`}
              >
                <TableCell className="font-medium">{pkg.version}</TableCell>
                <TableCell className="font-mono text-sm">{pkg.file_name}</TableCell>
                <TableCell>{formatFileSize(pkg.file_size)}</TableCell>
                <TableCell>
                  {pkg.has_source ? (
                    <div className="flex items-center gap-2 min-w-max">
                      <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">
                        <CheckCircle className="h-3 w-3 mr-1" />
                        {t('installer.sourceImported')}
                      </Badge>
                      <span className="text-xs text-muted-foreground">{formatFileSize(pkg.source_file_size || 0)}</span>
                    </div>
                  ) : pkg.source_status === 'failed' ? (
                    <Badge variant="outline" className="border-destructive/30 bg-destructive/10 text-destructive" title={pkg.source_error}>
                      <XCircle className="h-3 w-3 mr-1" />
                      {t('installer.sourceFailed')}
                    </Badge>
                  ) : (
                    <Badge variant="secondary">{t('installer.sourceMissing')}</Badge>
                  )}
                </TableCell>
                <TableCell>{formatDate(pkg.uploaded_at)}</TableCell>
                <TableCell>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon">
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {pkg.has_source ? (
                        <DropdownMenuItem onClick={() => onSourceDownload?.(pkg.version)}>
                          <Download className="h-4 w-4 mr-2" />
                          {t('installer.downloadSource')}
                        </DropdownMenuItem>
                      ) : (
                        <DropdownMenuItem onClick={() => onSourceFetch?.(pkg.version)}>
                          <FileCode2 className="h-4 w-4 mr-2" />
                          {t('installer.fetchSource')}
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem asChild>
                        <label className="cursor-pointer">
                          <Upload className="h-4 w-4 mr-2" />
                          {pkg.has_source ? t('installer.replaceSource') : t('installer.uploadSource')}
                          <input
                            type="file"
                            accept=".tar.gz"
                            className="hidden"
                            onChange={(event) => {
                              const selected = event.target.files?.[0];
                              if (selected) {
                                onSourceUpload?.(pkg.version, selected);
                              }
                              event.target.value = '';
                            }}
                          />
                        </label>
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        className="text-destructive"
                        onSelect={() => {
                          // 等下拉菜单关闭后再弹出确认框，避免与 AlertDialog 双重焦点陷阱
                          // Close the dropdown first, then open the confirm dialog to avoid nested focus traps
                          window.setTimeout(() => onDelete?.(pkg.version), 0);
                        }}
                      >
                        <Trash2 className="h-4 w-4 mr-2" />
                        {t('common.delete')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
