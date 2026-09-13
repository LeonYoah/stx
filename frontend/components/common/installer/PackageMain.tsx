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
 * Package Management Main Component
 * 安装包管理主组件
 */

'use client';

import { useState } from 'react';
import { usePackages } from '@/hooks/use-installer';
import { PackageTable } from './PackageTable';
import { UploadPackageDialog } from './UploadPackageDialog';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Upload, RefreshCw, Package, Cloud, HardDrive } from 'lucide-react';
import { useTranslations } from 'next-intl';
import {WorkspaceHeader} from '@/components/common/layout';

export function PackageMain() {
  const t = useTranslations();
  const { packages, loading, error, refresh, uploadPackage, deletePackage, startDownload, downloads, refreshVersions, refreshingVersions } = usePackages();
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<'online' | 'local'>('online');

  const handleUpload = async (
    file: File,
    version: string,
    onProgress?: (percent: number) => void,
  ) => {
    await uploadPackage(file, version, onProgress);
    setUploadDialogOpen(false);
  };

  const handleDelete = async (version: string) => {
    if (confirm(t('installer.confirmDeletePackage', { version }))) {
      await deletePackage(version);
    }
  };

  const handleDownload = async (version: string, mirror: 'aliyun' | 'apache' | 'huaweicloud') => {
    try {
      await startDownload(version, mirror);
    } catch (err) {
      // Error is handled by the hook / 错误由 hook 处理
      console.error('Download failed:', err);
    }
  };

  return (
    <div className="w-full space-y-6">
      {/* Header / 头部 */}
      <WorkspaceHeader
        icon={<Package />}
        title={t('installer.packageManagement')}
        subtitle={t('installer.packageManagementDesc')}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={refresh} disabled={loading}>
              <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
              {t('common.refresh')}
            </Button>
            <Button size="sm" onClick={() => setUploadDialogOpen(true)}>
              <Upload className="h-4 w-4 mr-2" />
              {t('installer.uploadPackage')}
            </Button>
          </>
        }
      />

      {/* Error display / 错误显示 */}
      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6">
            <p className="text-destructive">{error}</p>
          </CardContent>
        </Card>
      )}

      {/* Stats cards / 统计卡片 */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t('installer.availableVersions')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {packages?.versions.length || 0}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t('installer.localPackages')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {packages?.local_packages.length || 0}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t('installer.recommendedVersion')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-primary">
              {packages?.recommended_version || '-'}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Package tabs / 安装包标签页 */}
      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'online' | 'local')}>
        <TabsList>
          <TabsTrigger value="online" className="flex items-center gap-2">
            <Cloud className="h-4 w-4" />
            {t('installer.onlineVersions')}
          </TabsTrigger>
          <TabsTrigger value="local" className="flex items-center gap-2">
            <HardDrive className="h-4 w-4" />
            {t('installer.localPackages')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="online" className="mt-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle>{t('installer.onlineVersions')}</CardTitle>
                <CardDescription>
                  {t('installer.onlineVersionsDesc')}
                </CardDescription>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={refreshVersions}
                disabled={refreshingVersions}
              >
                <RefreshCw className={`h-4 w-4 mr-2 ${refreshingVersions ? 'animate-spin' : ''}`} />
                {t('installer.refreshVersions')}
              </Button>
            </CardHeader>
            <CardContent>
              <PackageTable
                type="online"
                versions={packages?.versions || []}
                localPackages={packages?.local_packages || []}
                recommendedVersion={packages?.recommended_version}
                loading={loading}
                onDownload={handleDownload}
                downloads={downloads}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="local" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>{t('installer.localPackages')}</CardTitle>
              <CardDescription>
                {t('installer.localPackagesDesc')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <PackageTable
                type="local"
                localPackages={packages?.local_packages || []}
                loading={loading}
                onDelete={handleDelete}
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Upload dialog / 上传对话框 */}
      <UploadPackageDialog
        open={uploadDialogOpen}
        onOpenChange={setUploadDialogOpen}
        onUpload={handleUpload}
        existingLocalVersions={(packages?.local_packages || []).map((pkg) => pkg.version)}
      />
    </div>
  );
}
