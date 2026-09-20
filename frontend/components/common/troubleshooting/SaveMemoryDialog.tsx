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
import {
  AlertCircle,
  CheckCircle2,
  FileCode,
  Fingerprint,
  Lightbulb,
  Plus,
  Save,
  ShieldCheck,
  Tag,
  X,
} from 'lucide-react';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import services from '@/lib/services';
import type {
  TroubleshootingMemoryEntry,
  TroubleshootingTargetType,
} from '@/lib/services/troubleshooting';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {ScrollArea} from '@/components/ui/scroll-area';

export interface SaveMemoryDialogProps {
  /**
   * 弹窗是否可见
   * Whether dialog is open
   */
  open: boolean;

  /**
   * 弹窗开关控制函数
   * Open change callback
   */
  onOpenChange: (open: boolean) => void;

  /**
   * 预填充或待编辑的经验数据
   * Initial data pre-filled from error or alert context
   */
  initialData?: {
    id?: string;
    target_type?: TroubleshootingTargetType;
    fingerprint?: string;
    title?: string;
    error_summary?: string;
    root_cause?: string;
    solution?: string;
    preventive_tips?: string;
    cluster_id?: number | string;
    cluster_name?: string;
    tags?: string[];
    author?: string;
  } | null;

  /**
   * 保存成功回调
   * Callback fired after successful save
   */
  onSaved?: (entry: TroubleshootingMemoryEntry) => void;
}

// 常见快捷分类标签候选
// Common quick tag candidates for SeaTunnel troubleshooting
const QUICK_TAGS = [
  'mysql',
  'jdbc',
  'timeout',
  'oom',
  'heap',
  'slot',
  'checkpoint',
  'network',
  'kafka',
  'worker',
  'seatunnel-env',
];

/**
 * 沉淀排障解决方案与经验弹窗组件
 * Dialog to record verified solution & troubleshooting memory
 */
export function SaveMemoryDialog({
  open,
  onOpenChange,
  initialData,
  onSaved,
}: SaveMemoryDialogProps) {
  const [title, setTitle] = useState('');
  const [targetType, setTargetType] = useState<TroubleshootingTargetType>('error');
  const [fingerprint, setFingerprint] = useState('');
  const [errorSummary, setErrorSummary] = useState('');
  const [rootCause, setRootCause] = useState('');
  const [solution, setSolution] = useState('');
  const [preventiveTips, setPreventiveTips] = useState('');
  const [author, setAuthor] = useState('运维工程师');
  const [tags, setTags] = useState<string[]>([]);
  const [customTagInput, setCustomTagInput] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState<{title?: string; solution?: string}>({});

  // 初始化或当 initialData 变动时预填充表单
  // Sync form state when initialData changes or dialog opens
  useEffect(() => {
    if (open && initialData) {
      setTitle(
        initialData.title ||
          (initialData.fingerprint ? `${initialData.fingerprint} 排障方案` : ''),
      );
      setTargetType(initialData.target_type || 'error');
      setFingerprint(initialData.fingerprint || '');
      setErrorSummary(initialData.error_summary || '');
      setRootCause(initialData.root_cause || '');
      setSolution(initialData.solution || '');
      setPreventiveTips(initialData.preventive_tips || '');
      setAuthor(initialData.author || '运维工程师');
      setTags(initialData.tags || []);
      setErrors({});
    } else if (open) {
      setTitle('');
      setTargetType('error');
      setFingerprint('');
      setErrorSummary('');
      setRootCause('');
      setSolution('');
      setPreventiveTips('');
      setAuthor('运维工程师');
      setTags([]);
      setErrors({});
    }
  }, [initialData, open]);

  // 添加自定义标签
  // Add custom tag
  const handleAddTag = useCallback((tagToAdd: string) => {
    const trimmed = tagToAdd.trim().toLowerCase();
    if (!trimmed) {
      return;
    }
    setTags((prev) => (prev.includes(trimmed) ? prev : [...prev, trimmed]));
    setCustomTagInput('');
  }, []);

  // 移除标签
  // Remove tag
  const handleRemoveTag = useCallback((tagToRemove: string) => {
    setTags((prev) => prev.filter((t) => t !== tagToRemove));
  }, []);

  // 提交保存排障经验
  // Submit and persist troubleshooting memory
  const handleSubmit = useCallback(() => {
    const newErrors: {title?: string; solution?: string} = {};
    if (!title.trim()) {
      newErrors.title = '方案标题不能为空';
    }
    if (!solution.trim()) {
      newErrors.solution =
        '解决方案内容不能为空！请务必记录具体的排查修复动作（如改了什么参数、执行了什么命令）。';
    }

    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      toast.error(newErrors.solution || newErrors.title);
      return;
    }

    setSubmitting(true);
    try {
      const saved = services.troubleshooting.saveMemory({
        id: initialData?.id,
        target_type: targetType,
        fingerprint: fingerprint.trim() || title.trim(),
        title: title.trim(),
        error_summary: errorSummary.trim(),
        root_cause: rootCause.trim() || undefined,
        solution: solution.trim(),
        preventive_tips: preventiveTips.trim() || undefined,
        cluster_id: initialData?.cluster_id,
        cluster_name: initialData?.cluster_name,
        tags,
        author: author.trim() || '运维工程师',
      });

      toast.success(
        initialData?.id
          ? '排障经验已成功更新！'
          : '排障方案已入库！后续发生同类故障时将自动在此置顶回显。',
      );
      onSaved?.(saved);
      onOpenChange(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '保存失败，请重试';
      toast.error(msg);
    } finally {
      setSubmitting(false);
    }
  }, [
    author,
    errorSummary,
    fingerprint,
    initialData?.cluster_id,
    initialData?.cluster_name,
    initialData?.id,
    onOpenChange,
    onSaved,
    preventiveTips,
    rootCause,
    solution,
    tags,
    targetType,
    title,
  ]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl border border-border/80 dark:border-border/60 shadow-2xl p-0 flex flex-col max-h-[90vh] overflow-hidden bg-background'>
        {/* 弹窗头部 / Dialog Header */}
        <DialogHeader className='p-4 pb-3 border-b bg-muted/20'>
          <div className='flex items-center gap-2'>
            <div className='p-1 rounded-md bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 shrink-0'>
              <Lightbulb className='h-4 w-4' />
            </div>
            <DialogTitle className='text-base font-bold text-foreground'>
              {initialData?.id ? '编辑排障经验' : '记录排障解决方案与经验'}
            </DialogTitle>
            <Badge
              variant='outline'
              className='text-xs font-mono ml-auto border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10'
            >
              经验记忆库
            </Badge>
          </div>
          <DialogDescription className='text-xs text-muted-foreground mt-1'>
            针对已成功排查解决的故障现象沉淀方案。沉淀后，下次遇到相同指纹的错误或告警将自动置顶回显，赋能后续快速处置与 AI 知识复用。
          </DialogDescription>
        </DialogHeader>

        {/* 表单内容滚动区 / Form Scroll Area */}
        <ScrollArea className='flex-1 p-4'>
          <div className='space-y-4 text-xs pr-1'>
            {/* 关联故障指纹与分类 / Target Fingerprint Info */}
            <div className='rounded-md border p-2.5 bg-muted/30 space-y-1.5'>
              <div className='flex items-center justify-between text-[11px] text-muted-foreground'>
                <span className='flex items-center gap-1 font-medium text-foreground'>
                  <Fingerprint className='h-3.5 w-3.5 text-primary' />
                  匹配关联指纹（自动回显依据）
                </span>
                <Badge variant='secondary' className='text-[10px]'>
                  {targetType === 'error' ? '错误日志指纹' : '集群告警指标'}
                </Badge>
              </div>
              <div className='font-mono text-xs text-foreground break-all bg-background/80 p-1.5 rounded border'>
                {fingerprint || '未提取到指纹，将使用方案标题作为模糊匹配键'}
              </div>
            </div>

            {/* 方案标题 / Solution Title */}
            <div className='space-y-1.5'>
              <Label htmlFor='title' className='text-xs font-medium flex items-center gap-1'>
                <span>方案标题</span>
                <span className='text-destructive'>*</span>
              </Label>
              <Input
                id='title'
                value={title}
                onChange={(e) => {
                  setTitle(e.target.value);
                  if (errors.title) {
                    setErrors((prev) => ({...prev, title: undefined}));
                  }
                }}
                placeholder='例如：MySQL 连接超时中断 (Communications link failure) 恢复方案'
                className={cn('h-8 text-xs', errors.title && 'border-destructive focus-visible:ring-destructive')}
              />
              {errors.title && (
                <p className='text-[11px] text-destructive flex items-center gap-1'>
                  <AlertCircle className='h-3 w-3' />
                  {errors.title}
                </p>
              )}
            </div>

            {/* 故障现象摘要 / Error Summary */}
            <div className='space-y-1.5'>
              <Label htmlFor='summary' className='text-xs font-medium text-muted-foreground flex items-center gap-1'>
                <FileCode className='h-3.5 w-3.5' />
                <span>故障现象 / 关键错误日志摘要</span>
              </Label>
              <Textarea
                id='summary'
                value={errorSummary}
                onChange={(e) => setErrorSummary(e.target.value)}
                placeholder='可粘贴简要报错日志或异常栈信息...'
                rows={2}
                className='text-xs font-mono resize-none leading-relaxed'
              />
            </div>

            {/* 核心必填：解决方案与排障动作 / Core Solution & Action Steps */}
            <div className='space-y-1.5 rounded-lg border border-emerald-500/40 bg-emerald-500/5 dark:bg-emerald-950/20 p-3'>
              <div className='flex items-center justify-between'>
                <Label htmlFor='solution' className='text-xs font-semibold text-foreground flex items-center gap-1'>
                  <ShieldCheck className='h-4 w-4 text-emerald-600 dark:text-emerald-400' />
                  <span>解决方案与排障具体步骤</span>
                  <span className='text-destructive'>* (必填)</span>
                </Label>
                <span className='text-[11px] text-muted-foreground'>
                  务必具体说明修改了哪些配置参数、执行了什么命令
                </span>
              </div>
              <Textarea
                id='solution'
                value={solution}
                onChange={(e) => {
                  setSolution(e.target.value);
                  if (errors.solution) {
                    setErrors((prev) => ({...prev, solution: undefined}));
                  }
                }}
                placeholder={`请写下你迈过这道坎的具体操作，例如：\n1. 调大 MySQL wait_timeout 与 interactive_timeout 至 28800 秒；\n2. 在作业 JDBC URL 中追加参数 autoReconnect=true&connectTimeout=30000；\n3. 重启 Worker 进程恢复作业运行。`}
                rows={4}
                className={cn(
                  'text-xs leading-relaxed bg-background/90',
                  errors.solution && 'border-destructive focus-visible:ring-destructive',
                )}
              />
              {errors.solution && (
                <p className='text-[11px] text-destructive flex items-center gap-1 font-medium'>
                  <AlertCircle className='h-3.5 w-3.5 shrink-0' />
                  {errors.solution}
                </p>
              )}
            </div>

            {/* 根本原因分析（选填） / Root Cause Analysis (Optional) */}
            <div className='space-y-1.5'>
              <Label htmlFor='root_cause' className='text-xs font-medium text-muted-foreground'>
                根本原因剖析（选填）
              </Label>
              <Textarea
                id='root_cause'
                value={rootCause}
                onChange={(e) => setRootCause(e.target.value)}
                placeholder='例如：数据库连接池空闲时间超过了 MySQL 服务端主动保活上限导致连接句柄失效...'
                rows={2}
                className='text-xs resize-none'
              />
            </div>

            {/* 防范建议（选填） / Preventive Tips (Optional) */}
            <div className='space-y-1.5'>
              <Label htmlFor='preventive_tips' className='text-xs font-medium text-muted-foreground'>
                防范与优化建议（选填）
              </Label>
              <Input
                id='preventive_tips'
                value={preventiveTips}
                onChange={(e) => setPreventiveTips(e.target.value)}
                placeholder='例如：针对批量离线任务建议关闭外部持久化 IMAP 存储，避免额外 I/O 损耗...'
                className='h-8 text-xs'
              />
            </div>

            {/* 分类标签 / Tags */}
            <div className='space-y-1.5'>
              <Label className='text-xs font-medium text-muted-foreground flex items-center gap-1'>
                <Tag className='h-3 w-3' />
                <span>分类检索标签</span>
              </Label>
              <div className='flex items-center gap-1.5 flex-wrap'>
                {tags.map((tag) => (
                  <span
                    key={tag}
                    className='inline-flex items-center gap-1 px-2 py-0.5 rounded bg-muted border text-[11px] font-mono'
                  >
                    #{tag}
                    <button
                      type='button'
                      onClick={() => handleRemoveTag(tag)}
                      className='text-muted-foreground hover:text-foreground'
                    >
                      <X className='h-2.5 w-2.5' />
                    </button>
                  </span>
                ))}
              </div>
              <div className='flex items-center gap-1.5 pt-1'>
                <Input
                  value={customTagInput}
                  onChange={(e) => setCustomTagInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      handleAddTag(customTagInput);
                    }
                  }}
                  placeholder='输入新标签按回车...'
                  className='h-7 text-xs w-36'
                />
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => handleAddTag(customTagInput)}
                  className='h-7 px-2 text-xs'
                >
                  <Plus className='h-3 w-3 mr-1' />
                  添加
                </Button>
                <div className='text-[10px] text-muted-foreground ml-1 flex items-center gap-1 flex-wrap'>
                  <span>快捷建议:</span>
                  {QUICK_TAGS.slice(0, 5).map((qTag) => (
                    <button
                      key={qTag}
                      type='button'
                      onClick={() => handleAddTag(qTag)}
                      className='hover:text-primary underline cursor-pointer'
                    >
                      {qTag}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* 记录人 / Author */}
            <div className='space-y-1.5'>
              <Label htmlFor='author' className='text-xs font-medium text-muted-foreground'>
                记录人 / 署名
              </Label>
              <Input
                id='author'
                value={author}
                onChange={(e) => setAuthor(e.target.value)}
                placeholder='例如：SRE 运维组 / 张三'
                className='h-8 text-xs max-w-xs'
              />
            </div>
          </div>
        </ScrollArea>

        {/* 弹窗底部操作 / Dialog Footer */}
        <DialogFooter className='p-3 border-t bg-muted/20 flex items-center justify-between sm:justify-between'>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => onOpenChange(false)}
            className='h-8 text-xs'
          >
            取消
          </Button>
          <Button
            type='button'
            size='sm'
            onClick={handleSubmit}
            disabled={submitting}
            className='h-8 text-xs bg-emerald-600 hover:bg-emerald-700 text-white gap-1.5 shadow-xs'
          >
            <Save className='h-3.5 w-3.5' />
            保存到经验记忆库
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
