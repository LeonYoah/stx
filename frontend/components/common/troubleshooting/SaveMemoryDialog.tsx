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
  Maximize2,
  Minimize2,
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
  const [isMaximized, setIsMaximized] = useState(false);
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
  const handleSubmit = useCallback(async () => {
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
      const saved = await services.troubleshooting.saveRemoteMemory({
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

  // 插入排障步骤模板
  // Insert step-by-step troubleshooting template
  const handleInsertTemplate = useCallback(() => {
    const template = `1. 排查定位：\n2. 处置操作（改动参数/执行命令）：\n3. 验证结果：服务恢复正常运行。`;
    setSolution((prev) => (prev.trim() ? `${prev}\n\n${template}` : template));
    setErrors((prev) => ({...prev, solution: undefined}));
  }, []);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          'border border-border/80 dark:border-border/60 shadow-2xl p-0 flex flex-col overflow-hidden bg-background rounded-xl transition-all duration-200',
          isMaximized
            ? 'w-[96vw] max-w-[96vw] h-[92vh] max-h-[92vh]'
            : 'w-[95vw] sm:max-w-3xl h-[85vh] max-h-[85vh]',
        )}
      >
        {/* 弹窗头部：明确预留 pr-20 物理安全避让区，杜绝与右上角 X 按钮及放大按钮重叠 */}
        {/* Dialog Header: explicit pr-20 safe area avoiding collision with top-right Close and Maximize buttons */}
        <DialogHeader className='px-5 py-3.5 border-b bg-muted/20 pr-20 text-left sm:text-left relative'>
          <div className='flex items-center gap-2'>
            <div className='flex size-7 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'>
              <Lightbulb className='size-3.5' />
            </div>
            <DialogTitle className='text-sm font-bold tracking-tight text-foreground'>
              {initialData?.id ? '编辑排障方案' : '记录排障解决方案与经验'}
            </DialogTitle>
            <Badge
              variant='outline'
              className='h-5 px-1.5 text-[10px] font-mono border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10'
            >
              经验库
            </Badge>
          </div>
          <DialogDescription className='text-xs text-muted-foreground mt-1'>
            沉淀已验证的故障处置步骤，后续同类错误将自动置顶回显与辅助排障。
          </DialogDescription>

          {/* 放大/还原切换按钮 / Maximize and restore toggle button */}
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-7 text-muted-foreground hover:text-foreground absolute right-11 top-3'
            onClick={() => setIsMaximized((prev) => !prev)}
            title={isMaximized ? '还原窗口' : '放大看'}
          >
            {isMaximized ? <Minimize2 className='size-3.5' /> : <Maximize2 className='size-3.5' />}
          </Button>
        </DialogHeader>

        {/* 表单内容滚动区 / Form Scroll Area */}
        <ScrollArea className='flex-1 px-5 py-4'>
          <div className='space-y-4 text-xs pr-1'>
            {/* 区域 1：关联故障指纹与类型 */}
            {/* Section 1: Fault fingerprint context and target category */}
            <div className='flex items-center justify-between gap-2 px-3 py-2 rounded-lg bg-muted/40 border border-border/60'>
              <div className='flex items-center gap-1.5 min-w-0 flex-1'>
                <Fingerprint className='size-3.5 text-primary shrink-0' />
                <span className='text-[11px] text-muted-foreground shrink-0'>关联指纹:</span>
                {initialData?.fingerprint ? (
                  <span className='font-mono text-xs font-medium text-foreground truncate'>
                    {fingerprint}
                  </span>
                ) : (
                  <Input
                    value={fingerprint}
                    onChange={(e) => setFingerprint(e.target.value)}
                    placeholder='输入异常类名或指纹（如 SlotNotEnoughException）'
                    className='h-6 text-xs font-mono bg-background/80 py-0 px-2 max-w-[280px]'
                  />
                )}
              </div>
              <Badge
                variant={targetType === 'error' ? 'secondary' : 'outline'}
                className={cn(
                  'text-[10px] font-medium shrink-0 h-5 px-1.5',
                  !initialData?.target_type && 'cursor-pointer hover:bg-muted/80',
                )}
                onClick={() => {
                  if (!initialData?.target_type) {
                    setTargetType((prev) => (prev === 'error' ? 'alert' : 'error'));
                  }
                }}
                title={!initialData?.target_type ? '点击切换错误/告警类型' : undefined}
              >
                {targetType === 'error' ? '错误日志' : '集群告警'}
              </Badge>
            </div>

            {/* 区域 2：方案标题 */}
            {/* Section 2: Solution Title */}
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
                  <AlertCircle className='size-3' />
                  {errors.title}
                </p>
              )}
            </div>

            {/* 区域 3：核心必填解决方案（高信噪比主视区） */}
            {/* Section 3: Core mandatory solution and execution steps */}
            <div className='space-y-1.5 rounded-xl border border-emerald-500/35 bg-emerald-500/[0.02] dark:bg-emerald-950/20 p-3'>
              <div className='flex items-center justify-between'>
                <Label htmlFor='solution' className='text-xs font-semibold text-foreground flex items-center gap-1.5'>
                  <ShieldCheck className='size-3.5 text-emerald-600 dark:text-emerald-400' />
                  <span>解决方案与排障具体步骤</span>
                  <span className='text-destructive'>* (必填)</span>
                </Label>
                <button
                  type='button'
                  onClick={handleInsertTemplate}
                  className='text-[11px] text-emerald-600 dark:text-emerald-400 hover:underline cursor-pointer flex items-center gap-0.5'
                >
                  <Plus className='size-3' />
                  插入步骤模板
                </button>
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
                placeholder={`请写下具体的处置操作与命令参数，例如：\n1. 在 seatunnel.yaml 或 seatunnel-env.sh 中调大 checkpoint 超时时长：checkpoint.timeout: 120000；\n2. 检查下游目标数据库写入负载，优化 Sink 端 batch.size 与写入并发，消除反压以加速 Barrier 对齐；\n3. 重启任务验证恢复状态。`}
                rows={isMaximized ? 8 : 4}
                className={cn(
                  'text-xs font-mono leading-relaxed bg-background/90 overflow-y-auto resize-none',
                  isMaximized ? 'h-64 max-h-80' : 'h-36 max-h-48',
                  errors.solution && 'border-destructive focus-visible:ring-destructive',
                )}
              />
              {errors.solution && (
                <p className='text-[11px] text-destructive flex items-center gap-1 font-medium'>
                  <AlertCircle className='size-3 shrink-0' />
                  {errors.solution}
                </p>
              )}
            </div>

            {/* 区域 4：根因剖析与防范建议（两列紧凑并排） */}
            {/* Section 4: Root cause and preventive tips in compact two-column grid */}
            <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
              <div className='space-y-1.5'>
                <Label htmlFor='root_cause' className='text-xs font-medium text-muted-foreground'>
                  根本原因剖析（选填）
                </Label>
                <Textarea
                  id='root_cause'
                  value={rootCause}
                  onChange={(e) => setRootCause(e.target.value)}
                  placeholder='例如：连接空闲时长超过了服务端上限导致被主动切断...'
                  rows={2}
                  className='text-xs resize-none'
                />
              </div>

              <div className='space-y-1.5'>
                <Label htmlFor='preventive_tips' className='text-xs font-medium text-muted-foreground'>
                  防范与优化建议（选填）
                </Label>
                <Textarea
                  id='preventive_tips'
                  value={preventiveTips}
                  onChange={(e) => setPreventiveTips(e.target.value)}
                  placeholder='例如：批处理任务建议关闭外部持久化存储，流任务开启心跳...'
                  rows={2}
                  className='text-xs resize-none'
                />
              </div>
            </div>

            {/* 区域 5：故障现象摘要（选填折叠卡片） */}
            {/* Section 5: Error summary text snippet */}
            <div className='space-y-1.5'>
              <Label htmlFor='summary' className='text-xs font-medium text-muted-foreground flex items-center gap-1'>
                <FileCode className='size-3' />
                <span>故障现象 / 关键错误日志摘要（选填）</span>
              </Label>
              <Textarea
                id='summary'
                value={errorSummary}
                onChange={(e) => setErrorSummary(e.target.value)}
                placeholder='可粘贴简要报错日志或异常栈片段...'
                rows={2}
                className='text-xs font-mono resize-none leading-relaxed'
              />
            </div>

            {/* 区域 6：分类标签与记录人（底栏紧凑两列） */}
            {/* Section 6: Categorical tags and author */}
            <div className='grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1 border-t border-border/40'>
              <div className='space-y-1.5'>
                <Label className='text-xs font-medium text-muted-foreground flex items-center gap-1'>
                  <Tag className='size-3' />
                  <span>分类检索标签</span>
                </Label>
                <div className='flex items-center gap-1 flex-wrap min-h-6'>
                  {tags.map((tag) => (
                    <span
                      key={tag}
                      className='inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-muted/80 border text-[10px] font-mono'
                    >
                      #{tag}
                      <button
                        type='button'
                        onClick={() => handleRemoveTag(tag)}
                        className='text-muted-foreground hover:text-foreground'
                      >
                        <X className='size-2.5' />
                      </button>
                    </span>
                  ))}
                </div>
                <div className='flex items-center gap-1.5 pt-0.5'>
                  <Input
                    value={customTagInput}
                    onChange={(e) => setCustomTagInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        handleAddTag(customTagInput);
                      }
                    }}
                    placeholder='输入标签回车...'
                    className='h-7 text-xs w-28'
                  />
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => handleAddTag(customTagInput)}
                    className='h-7 px-2 text-xs'
                  >
                    <Plus className='size-3 mr-0.5' />
                    添加
                  </Button>
                  <div className='text-[10px] text-muted-foreground flex items-center gap-1 flex-wrap'>
                    {QUICK_TAGS.slice(0, 4).map((qTag) => (
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

              <div className='space-y-1.5'>
                <Label htmlFor='author' className='text-xs font-medium text-muted-foreground'>
                  记录人 / 署名
                </Label>
                <Input
                  id='author'
                  value={author}
                  onChange={(e) => setAuthor(e.target.value)}
                  placeholder='例如：SRE 运维组 / 工程师'
                  className='h-7 text-xs'
                />
              </div>
            </div>
          </div>
        </ScrollArea>

        {/* 弹窗底部操作 / Dialog Footer */}
        <DialogFooter className='px-5 py-3 border-t bg-muted/20 flex items-center justify-between sm:justify-between'>
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
            className='h-8 text-xs bg-emerald-600 hover:bg-emerald-700 active:scale-[0.98] text-white gap-1.5 shadow-xs font-medium'
          >
            <Save className='size-3.5' />
            保存到排障经验库
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
