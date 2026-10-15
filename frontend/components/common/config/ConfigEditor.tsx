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
 * Config Editor Component
 * 配置编辑器组件
 */

'use client';

import { useState } from 'react';
import { useTranslations } from 'next-intl';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Edit, Save, X, History, Upload, Download } from 'lucide-react';
import type { ConfigInfo } from '@/lib/services/config';
import {formatDateTimeTz} from '@/lib/timezone';

interface ConfigEditorProps {
  config: ConfigInfo;
  content: string;
  isEditing: boolean;
  onEdit: () => void;
  onSave: (comment?: string) => void;
  onCancel: () => void;
  onChange: (content: string) => void;
  onViewVersions: () => void;
  onPromote?: () => void;
  onSyncFromTemplate?: () => void;
  showPromote?: boolean;
  showSync?: boolean;
}

/**
 * Config Editor Component
 * 配置编辑器组件
 */
export function ConfigEditor({
  config,
  content,
  isEditing,
  onEdit,
  onSave,
  onCancel,
  onChange,
  onViewVersions,
  onPromote,
  onSyncFromTemplate,
  showPromote = false,
  showSync = false,
}: ConfigEditorProps) {
  const t = useTranslations();
  const [showSaveDialog, setShowSaveDialog] = useState(false);
  const [comment, setComment] = useState('');

  const handleSave = () => {
    setShowSaveDialog(true);
  };

  const handleConfirmSave = () => {
    onSave(comment);
    setShowSaveDialog(false);
    setComment('');
  };

  return (
    <div className="space-y-4">
      {/* Toolbar / 工具栏 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">
            {t('config.version')}: v{config.version}
          </span>
          <span className="text-sm text-muted-foreground">|</span>
          <span className="text-sm text-muted-foreground">
            {t('config.updatedAt')}: {formatDateTimeTz(config.updated_at)}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {isEditing ? (
            <>
              <Button variant="outline" size="sm" onClick={onCancel}>
                <X className="h-4 w-4 mr-1" />
                {t('common.cancel')}
              </Button>
              <Button size="sm" onClick={handleSave}>
                <Save className="h-4 w-4 mr-1" />
                {t('common.save')}
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" size="sm" onClick={onViewVersions}>
                <History className="h-4 w-4 mr-1" />
                {t('config.history')}
              </Button>
              {showSync && onSyncFromTemplate && (
                <Button variant="outline" size="sm" onClick={onSyncFromTemplate}>
                  <Download className="h-4 w-4 mr-1" />
                  {t('config.syncFromTemplate')}
                </Button>
              )}
              {showPromote && onPromote && (
                <Button variant="outline" size="sm" onClick={onPromote}>
                  <Upload className="h-4 w-4 mr-1" />
                  {t('config.promoteToCluster')}
                </Button>
              )}
              <Button size="sm" onClick={onEdit}>
                <Edit className="h-4 w-4 mr-1" />
                {t('common.edit')}
              </Button>
            </>
          )}
        </div>
      </div>

      {/* Editor / 编辑器 */}
      <Textarea
        value={content}
        onChange={(e) => onChange(e.target.value)}
        readOnly={!isEditing}
        className={`font-mono text-sm min-h-[400px] ${
          isEditing ? 'bg-background' : 'bg-muted/50'
        }`}
        placeholder={t('config.contentPlaceholder')}
      />

      {/* Save dialog / 保存对话框 */}
      <Dialog open={showSaveDialog} onOpenChange={setShowSaveDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('config.saveConfig')}</DialogTitle>
            <DialogDescription>
              {t('config.saveConfigDesc')}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="comment">{t('config.comment')}</Label>
              <Input
                id="comment"
                value={comment}
                onChange={(e) => setComment(e.target.value)}
                placeholder={t('config.commentPlaceholder')}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSaveDialog(false)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={handleConfirmSave}>
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
