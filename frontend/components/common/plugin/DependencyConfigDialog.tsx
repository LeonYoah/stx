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

import {useState, useEffect, useCallback} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Badge} from '@/components/ui/badge';
import {toast} from 'sonner';
import {FileCode, Package, Plus, RefreshCw, Trash2, Upload} from 'lucide-react';
import {PluginService} from '@/lib/services/plugin';
import type {PluginDependencyConfig} from '@/lib/services/plugin';

interface PluginDependencyConfigSectionProps {
  pluginName: string;
  seatunnelVersion?: string;
  compact?: boolean;
  onChanged?: () => void;
}

interface DependencyConfigDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pluginName: string;
  seatunnelVersion?: string;
}

function parseMavenXml(
  xml: string,
): {groupId: string; artifactId: string; version: string} | null {
  try {
    const groupIdMatch = xml.match(/<groupId>\s*([^<]+)\s*<\/groupId>/);
    const artifactIdMatch = xml.match(
      /<artifactId>\s*([^<]+)\s*<\/artifactId>/,
    );
    const versionMatch = xml.match(/<version>\s*([^<]+)\s*<\/version>/);

    if (!groupIdMatch || !artifactIdMatch) {
      return null;
    }

    return {
      groupId: groupIdMatch[1].trim(),
      artifactId: artifactIdMatch[1].trim(),
      version: versionMatch ? versionMatch[1].trim() : '',
    };
  } catch {
    return null;
  }
}

function inferJarCoordinates(fileName: string): {
  artifactId: string;
  version: string;
} {
  const normalized = fileName.replace(/\.jar$/i, '').trim();
  const match = normalized.match(/^(.*)-([0-9][A-Za-z0-9._+-]*)$/);
  if (!match) {
    return {artifactId: normalized, version: ''};
  }
  return {
    artifactId: match[1].trim(),
    version: match[2].trim(),
  };
}

function getSourceBadgeVariant(sourceType: string) {
  switch (sourceType) {
    case 'upload':
      return 'default';
    case 'maven':
      return 'secondary';
    default:
      return 'outline';
  }
}

function getDependencyFileName(dep: PluginDependencyConfig): string {
  if (dep.original_file_name?.trim()) {
    return dep.original_file_name.trim();
  }
  const artifact = dep.artifact_id?.trim() || 'dependency';
  const depVersion = dep.version?.trim();
  return depVersion ? `${artifact}-${depVersion}.jar` : `${artifact}.jar`;
}

function buildCustomDependencyActionId(
  prefix: string,
  artifactId: string,
): string {
  return `${prefix}-${artifactId.replace(/[^a-z0-9]+/gi, '-').replace(/^-+|-+$/g, '')}`.toLowerCase();
}

export function PluginDependencyConfigSection({
  pluginName,
  seatunnelVersion,
  compact = false,
  onChanged,
}: PluginDependencyConfigSectionProps) {
  const t = useTranslations();
  const [dependencies, setDependencies] = useState<PluginDependencyConfig[]>(
    [],
  );
  const [loading, setLoading] = useState(false);
  const [showAddForm, setShowAddForm] = useState(false);
  const [groupId, setGroupId] = useState('');
  const [artifactId, setArtifactId] = useState('');
  const [version, setVersion] = useState('');
  const [adding, setAdding] = useState(false);
  const [showXmlParse, setShowXmlParse] = useState(false);
  const [mavenXml, setMavenXml] = useState('');
  const [showUploadForm, setShowUploadForm] = useState(false);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadGroupId, setUploadGroupId] = useState('');
  const [uploadArtifactId, setUploadArtifactId] = useState('');
  const [uploadVersion, setUploadVersion] = useState('');
  const [uploading, setUploading] = useState(false);

  const loadDependencies = useCallback(async () => {
    if (!pluginName) {
      setDependencies([]);
      return;
    }
    setLoading(true);
    try {
      const deps = await PluginService.listDependencies(
        pluginName,
        seatunnelVersion,
      );
      setDependencies(deps || []);
    } catch (err) {
      console.error('Failed to load dependencies:', err);
      setDependencies([]);
    } finally {
      setLoading(false);
    }
  }, [pluginName, seatunnelVersion]);

  useEffect(() => {
    if (!pluginName) {
      return;
    }
    void loadDependencies();
  }, [loadDependencies, pluginName]);

  const resetForms = () => {
    setGroupId('');
    setArtifactId('');
    setVersion('');
    setShowAddForm(false);
    setShowXmlParse(false);
    setMavenXml('');
    setShowUploadForm(false);
    setUploadFile(null);
    setUploadGroupId('');
    setUploadArtifactId('');
    setUploadVersion('');
  };

  const afterMutation = async () => {
    await loadDependencies();
    onChanged?.();
  };

  const handleAddDependency = async () => {
    if (!groupId.trim() || !artifactId.trim() || !version.trim()) {
      toast.error(t('plugin.allFieldsRequired'));
      return;
    }

    setAdding(true);
    try {
      await PluginService.addDependency(pluginName, {
        group_id: groupId.trim(),
        artifact_id: artifactId.trim(),
        version: version.trim(),
        seatunnel_version: seatunnelVersion,
      });
      toast.success(t('plugin.addDependencySuccess'));
      resetForms();
      await afterMutation();
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : t('plugin.addDependencyFailed');
      toast.error(errorMsg);
    } finally {
      setAdding(false);
    }
  };

  const handleUploadDependency = async () => {
    if (!uploadFile) {
      toast.error(t('plugin.uploadJarRequired'));
      return;
    }

    setUploading(true);
    try {
      await PluginService.uploadDependency(pluginName, {
        file: uploadFile,
        seatunnel_version: seatunnelVersion,
        group_id: uploadGroupId.trim() || undefined,
        artifact_id: uploadArtifactId.trim() || undefined,
        version: uploadVersion.trim() || undefined,
      });
      toast.success(t('plugin.uploadDependencySuccess'));
      resetForms();
      await afterMutation();
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : t('plugin.uploadDependencyFailed');
      toast.error(errorMsg);
    } finally {
      setUploading(false);
    }
  };

  const handleDeleteDependency = async (dep: PluginDependencyConfig) => {
    try {
      await PluginService.deleteDependency(pluginName, dep.id);
      toast.success(t('plugin.deleteDependencySuccess'));
      await afterMutation();
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : t('plugin.deleteDependencyFailed');
      toast.error(errorMsg);
    }
  };

  const handleParseMavenXml = () => {
    if (!mavenXml.trim()) {
      toast.error(t('plugin.mavenXmlEmpty'));
      return;
    }

    const parsed = parseMavenXml(mavenXml);
    if (!parsed) {
      toast.error(t('plugin.mavenXmlParseError'));
      return;
    }

    setGroupId(parsed.groupId);
    setArtifactId(parsed.artifactId);
    setVersion(parsed.version);
    setShowXmlParse(false);
    setShowAddForm(true);
    setShowUploadForm(false);
    setMavenXml('');
    toast.success(t('plugin.mavenXmlParseSuccess'));
  };

  const handleSelectFile = (file: File | null) => {
    setUploadFile(file);
    if (!file) {
      setUploadArtifactId('');
      setUploadVersion('');
      return;
    }
    const inferred = inferJarCoordinates(file.name);
    setUploadArtifactId((prev) => prev || inferred.artifactId);
    setUploadVersion((prev) => prev || inferred.version);
  };

  const containerClassName = compact
    ? 'space-y-4 rounded-xl border bg-muted/20 p-4'
    : 'space-y-4';

  return (
    <div
      className={containerClassName}
      data-testid='plugin-custom-dependencies'
    >
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='min-w-0 flex-1 mr-2'>
          <h4 className='font-medium'>{t('plugin.configDependency')}</h4>
          <p className='mt-1 text-xs text-muted-foreground'>
            {t('plugin.configuredDependenciesDesc')}
          </p>
        </div>
        <div className='flex items-center gap-2 shrink-0 flex-wrap sm:flex-nowrap'>
          <Button
            variant='outline'
            size='sm'
            className='whitespace-nowrap shrink-0'
            onClick={() => {
              setShowXmlParse((prev) => !prev);
              setShowAddForm(false);
              setShowUploadForm(false);
            }}
          >
            <FileCode className='mr-2 h-4 w-4' />
            {t('plugin.parseMavenXml')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='whitespace-nowrap shrink-0'
            onClick={() => {
              setShowAddForm((prev) => !prev);
              setShowXmlParse(false);
              setShowUploadForm(false);
            }}
          >
            <Plus className='mr-2 h-4 w-4' />
            {t('plugin.addDependency')}
          </Button>
          <Button
            size='sm'
            data-testid='plugin-upload-dependency-trigger'
            className='whitespace-nowrap shrink-0'
            onClick={() => {
              setShowUploadForm((prev) => !prev);
              setShowAddForm(false);
              setShowXmlParse(false);
            }}
          >
            <Upload className='mr-2 h-4 w-4' />
            {t('plugin.uploadJar')}
          </Button>
        </div>
      </div>

      {showXmlParse && (
        <div className='space-y-4 rounded-lg border bg-background p-4'>
          <div>
            <h5 className='text-sm font-medium'>{t('plugin.parseMavenXml')}</h5>
            <p className='mt-1 text-xs text-muted-foreground'>
              {t('plugin.parseMavenXmlDesc')}
            </p>
          </div>
          <Textarea
            value={mavenXml}
            onChange={(e) => setMavenXml(e.target.value)}
            placeholder={`<dependency>\n  <groupId>com.mysql</groupId>\n  <artifactId>mysql-connector-j</artifactId>\n  <version>8.0.33</version>\n</dependency>`}
            className='min-h-[120px] font-mono text-sm'
          />
          <div className='flex justify-end gap-2'>
            <Button
              variant='outline'
              className='whitespace-nowrap shrink-0'
              onClick={() => {
                setShowXmlParse(false);
                setMavenXml('');
              }}
            >
              {t('common.cancel')}
            </Button>
            <Button onClick={handleParseMavenXml} className='whitespace-nowrap shrink-0'>
              {t('plugin.parseAndFill')}
            </Button>
          </div>
        </div>
      )}

      {showAddForm && (
        <div className='space-y-4 rounded-lg border bg-background p-4'>
          <div>
            <h5 className='text-sm font-medium'>
              {t('plugin.addDependencyDesc')}
            </h5>
            <p className='mt-1 text-xs text-muted-foreground'>
              {t('plugin.dependenciesDesc')}
            </p>
          </div>
          <div className='grid gap-4 md:grid-cols-3'>
            <div className='space-y-2'>
              <Label htmlFor='groupId'>{t('plugin.groupId')} *</Label>
              <Input
                id='groupId'
                value={groupId}
                onChange={(e) => setGroupId(e.target.value)}
                placeholder={t('plugin.groupIdPlaceholder')}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='artifactId'>{t('plugin.artifactId')} *</Label>
              <Input
                id='artifactId'
                value={artifactId}
                onChange={(e) => setArtifactId(e.target.value)}
                placeholder={t('plugin.artifactIdPlaceholder')}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='version'>{t('plugin.version')} *</Label>
              <Input
                id='version'
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                placeholder={t('plugin.versionPlaceholderRequired')}
              />
            </div>
          </div>
          <div className='flex justify-end gap-2'>
            <Button
              variant='outline'
              className='whitespace-nowrap shrink-0'
              onClick={resetForms}
              disabled={adding}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleAddDependency}
              disabled={adding}
              className='whitespace-nowrap shrink-0'
            >
              {adding ? (
                <RefreshCw className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <Plus className='mr-2 h-4 w-4' />
              )}
              {t('plugin.addDependency')}
            </Button>
          </div>
        </div>
      )}

      {showUploadForm && (
        <div
          className='space-y-4 rounded-lg border bg-background p-4'
          data-testid='plugin-upload-dependency-form'
        >
          <div>
            <h5 className='text-sm font-medium'>{t('plugin.uploadJarDesc')}</h5>
            <p className='mt-1 text-xs text-muted-foreground'>
              {t('plugin.uploadJarHint')}
            </p>
          </div>
          <div className='grid gap-4 md:grid-cols-3'>
            <div className='space-y-2 md:col-span-3'>
              <Label htmlFor='uploadJar'>{t('plugin.uploadJarFile')} *</Label>
              <Input
                id='uploadJar'
                type='file'
                accept='.jar'
                data-testid='plugin-upload-jar-input'
                onChange={(e) => handleSelectFile(e.target.files?.[0] || null)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='uploadGroupId'>{t('plugin.groupId')}</Label>
              <Input
                id='uploadGroupId'
                value={uploadGroupId}
                onChange={(e) => setUploadGroupId(e.target.value)}
                placeholder={t('plugin.uploadGroupIdPlaceholder')}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='uploadArtifactId'>{t('plugin.artifactId')}</Label>
              <Input
                id='uploadArtifactId'
                value={uploadArtifactId}
                onChange={(e) => setUploadArtifactId(e.target.value)}
                placeholder={t('plugin.uploadArtifactIdPlaceholder')}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='uploadVersion'>{t('plugin.version')}</Label>
              <Input
                id='uploadVersion'
                value={uploadVersion}
                onChange={(e) => setUploadVersion(e.target.value)}
                placeholder={t('plugin.uploadVersionPlaceholder')}
              />
            </div>
          </div>
          {uploadFile && (
            <div className='rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground'>
              {t('plugin.selectedFile')}: {uploadFile.name}
            </div>
          )}
          <div className='flex justify-end gap-2'>
            <Button
              variant='outline'
              className='whitespace-nowrap shrink-0'
              onClick={resetForms}
              disabled={uploading}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleUploadDependency}
              disabled={uploading}
              className='whitespace-nowrap shrink-0'
            >
              {uploading ? (
                <RefreshCw className='mr-2 h-4 w-4 animate-spin' />
              ) : (
                <Upload className='mr-2 h-4 w-4' />
              )}
              {t('plugin.uploadJar')}
            </Button>
          </div>
        </div>
      )}

      {loading ? (
        <div className='flex items-center justify-center py-8'>
          <RefreshCw className='h-6 w-6 animate-spin text-muted-foreground' />
        </div>
      ) : dependencies.length === 0 ? (
        <div className='rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground'>
          <Package className='mx-auto mb-3 h-10 w-10 opacity-50' />
          <p>{t('plugin.noDependencies')}</p>
        </div>
      ) : (
        <div className='overflow-x-auto rounded-lg border bg-background'>
          <Table className='min-w-[980px]'>
            <TableHeader>
              <TableRow>
                <TableHead>{t('plugin.source')}</TableHead>
                <TableHead>{t('plugin.groupId')}</TableHead>
                <TableHead>{t('plugin.artifactId')}</TableHead>
                <TableHead>{t('plugin.version')}</TableHead>
                <TableHead>{t('plugin.targetDir')}</TableHead>
                <TableHead>{t('plugin.fileName')}</TableHead>
                <TableHead className='w-[80px]'>
                  {t('common.actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {dependencies.map((dep) => (
                <TableRow
                  key={dep.id}
                  data-testid={buildCustomDependencyActionId(
                    'plugin-custom-dependency',
                    dep.artifact_id,
                  )}
                >
                  <TableCell>
                    <Badge variant={getSourceBadgeVariant(dep.source_type)}>
                      {dep.source_type === 'upload'
                        ? t('plugin.sourceUpload')
                        : t('plugin.sourceMaven')}
                    </Badge>
                  </TableCell>
                  <TableCell className='font-mono text-xs whitespace-nowrap'>
                    {dep.group_id}
                  </TableCell>
                  <TableCell className='font-mono text-xs whitespace-nowrap'>
                    {dep.artifact_id}
                  </TableCell>
                  <TableCell className='font-mono text-xs whitespace-nowrap'>
                    {dep.version}
                  </TableCell>
                  <TableCell className='font-mono text-xs break-all'>
                    {dep.target_dir}
                  </TableCell>
                  <TableCell className='text-xs break-all'>
                    {getDependencyFileName(dep)}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant='ghost'
                      size='sm'
                      aria-label={`${t('common.delete')} ${dep.artifact_id}`}
                      data-testid={buildCustomDependencyActionId(
                        'plugin-delete-custom-dependency',
                        dep.artifact_id,
                      )}
                      className='text-destructive hover:text-destructive'
                      onClick={() => handleDeleteDependency(dep)}
                    >
                      <Trash2 className='h-4 w-4' />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}

export function DependencyConfigDialog({
  open,
  onOpenChange,
  pluginName,
  seatunnelVersion,
}: DependencyConfigDialogProps) {
  const t = useTranslations();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-4xl'>
        <DialogHeader>
          <DialogTitle>
            {t('plugin.configDependency')} - {pluginName}
          </DialogTitle>
          <DialogDescription>
            {t('plugin.configuredDependenciesDesc')}
          </DialogDescription>
        </DialogHeader>
        <PluginDependencyConfigSection
          pluginName={pluginName}
          seatunnelVersion={seatunnelVersion}
        />
      </DialogContent>
    </Dialog>
  );
}
