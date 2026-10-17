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
 * Config Version History Component
 * 配置版本历史组件
 */

'use client';

import { useState } from 'react';
import { useTranslations } from 'next-intl';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { History, RotateCcw, Eye } from 'lucide-react';
import type { ConfigInfo, ConfigVersionInfo } from '@/lib/services/config';
import {formatDateTimeTz} from '@/lib/timezone';

interface ConfigVersionHistoryProps {
  config: ConfigInfo;
  versions: ConfigVersionInfo[];
  onClose: () => void;
  onRollback: (version: number) => void;
}

/**
 * Config Version History Component
 * 配置版本历史组件
 */
export function ConfigVersionHistory({
  config,
  versions,
  onClose,
  onRollback,
}: ConfigVersionHistoryProps) {
  const t = useTranslations();
  const [selectedVersion, setSelectedVersion] = useState<ConfigVersionInfo | null>(null);
  const [showContent, setShowContent] = useState(false);
  const [showRollbackConfirm, setShowRollbackConfirm] = useState(false);
  const [versionToRollback, setVersionToRollback] = useState<number | null>(null);

  const handleViewContent = (version: ConfigVersionInfo) => {
    setSelectedVersion(version);
    setShowContent(true);
  };

  const handleRollbackClick = (version: number) => {
    setVersionToRollback(version);
    setShowRollbackConfirm(true);
  };

  const handleConfirmRollback = () => {
    if (versionToRollback !== null) {
      onRollback(versionToRollback);
    }
    setShowRollbackConfirm(false);
    setVersionToRollback(null);
  };

  return (
    <>
      <Dialog open={true} onOpenChange={onClose}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <History className="h-5 w-5" />
              {t('config.versionHistory')}
            </DialogTitle>
            <DialogDescription>
              {config.is_template
                ? t('config.templateVersionHistoryDesc')
                : t('config.nodeVersionHistoryDesc', {
                    node: config.host_name || `Node ${config.host_id}`,
                  })}
            </DialogDescription>
          </DialogHeader>

          <ScrollArea className="max-h-[400px]">
            <div className="space-y-2">
              {versions.length === 0 ? (
                <div className="text-center py-8 text-muted-foreground">
                  <History className="h-12 w-12 mx-auto mb-4 opacity-50" />
                  <p>{t('config.noVersions')}</p>
                </div>
              ) : (
                versions.map((version) => (
                  <div
                    key={version.id}
                    className="flex items-center justify-between p-3 border rounded-lg hover:bg-muted/50"
                  >
                    <div className="flex items-center gap-3">
                      <Badge
                        variant={version.version === config.version ? 'default' : 'outline'}
                      >
                        v{version.version}
                      </Badge>
                      <div>
                        <p className="text-sm font-medium">
                          {version.comment || t('config.noComment')}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {formatDateTimeTz(version.created_at)}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleViewContent(version)}
                      >
                        <Eye className="h-4 w-4 mr-1" />
                        {t('common.view')}
                      </Button>
                      {version.version !== config.version && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleRollbackClick(version.version)}
                        >
                          <RotateCcw className="h-4 w-4 mr-1" />
                          {t('config.rollback')}
                        </Button>
                      )}
                      {version.version === config.version && (
                        <Badge variant="secondary">{t('config.current')}</Badge>
                      )}
                    </div>
                  </div>
                ))
              )}
            </div>
          </ScrollArea>

          <DialogFooter>
            <Button variant="outline" onClick={onClose}>
              {t('common.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* View content dialog / 查看内容对话框 */}
      <Dialog open={showContent} onOpenChange={setShowContent}>
        <DialogContent className="max-w-3xl max-h-[80vh]">
          <DialogHeader>
            <DialogTitle>
              {t('config.versionContent', { version: selectedVersion?.version ?? '' })}
            </DialogTitle>
            <DialogDescription>
              {selectedVersion?.comment || t('config.noComment')}
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className="max-h-[500px]">
            <pre className="p-4 bg-muted rounded-lg text-sm font-mono whitespace-pre-wrap">
              {selectedVersion?.content}
            </pre>
          </ScrollArea>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowContent(false)}>
              {t('common.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Rollback confirm dialog / 回滚确认对话框 */}
      <AlertDialog open={showRollbackConfirm} onOpenChange={setShowRollbackConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('config.rollbackConfirmTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('config.rollbackConfirmDesc', { version: versionToRollback ?? '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmRollback}>
              {t('config.rollback')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
