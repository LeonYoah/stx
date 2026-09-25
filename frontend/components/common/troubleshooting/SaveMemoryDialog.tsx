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

import {useState, useEffect, useCallback, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertCircle,
  FileCode,
  Fingerprint,
  Lightbulb,
  Maximize2,
  Minimize2,
  Plus,
  Save,
  ShieldCheck,
  Tag,
  User,
  X,
} from 'lucide-react';
import {toast} from 'sonner';
import {useAuth} from '@/hooks/use-auth';
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

/** 可单独放大编辑的大文本字段 / Expandable long-text fields */
type ExpandableFieldKey =
  | 'solution'
  | 'root_cause'
  | 'preventive_tips'
  | 'error_summary';

const FIELD_EXPAND_TITLES: Record<ExpandableFieldKey, string> = {
  solution: '解决方案与处置步骤',
  root_cause: '根本原因剖析',
  preventive_tips: '防范与优化建议',
  error_summary: '故障现象 / 关键错误日志摘要',
};

/**
 * 大文本区：默认固定高度内部滚动，标题旁可单独放大编辑，不跟弹窗全局放大绑定。
 * Long text field: fixed height with internal scroll by default; expand one field at a time without global dialog maximize.
 */
function MemoryTextField({
  id,
  label,
  value,
  onChange,
  placeholder,
  fieldKey,
  expandedField,
  onExpand,
  invalid,
  mono,
  compactHeightClass,
  actions,
}: {
  id: string;
  label: ReactNode;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  fieldKey: ExpandableFieldKey;
  expandedField: ExpandableFieldKey | null;
  onExpand: (key: ExpandableFieldKey | null) => void;
  invalid?: boolean;
  mono?: boolean;
  compactHeightClass: string;
  actions?: ReactNode;
}) {
  const isExpanded = expandedField === fieldKey;

  return (
    <div className='space-y-1.5'>
      <div className='flex items-center justify-between gap-2'>
        <Label htmlFor={id} className='text-xs font-medium flex items-center gap-1.5 min-w-0'>
          {label}
        </Label>
        <div className='flex items-center gap-1.5 shrink-0'>
          {actions}
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-6 text-muted-foreground hover:text-foreground'
            onClick={() => onExpand(isExpanded ? null : fieldKey)}
            title={isExpanded ? '收起' : '放大编辑'}
          >
            {isExpanded ? <Minimize2 className='size-3.5' /> : <Maximize2 className='size-3.5' />}
          </Button>
        </div>
      </div>
      {/* field-sizing-fixed 覆盖全局 Textarea 的 content 自适应，避免内容撑高布局 */}
      {/* field-sizing-fixed overrides the shared Textarea content sizing so long text scrolls instead of growing */}
      <Textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        style={{fieldSizing: 'fixed'}}
        className={cn(
          'field-sizing-fixed text-xs leading-relaxed overflow-y-auto resize-none',
          mono && 'font-mono',
          compactHeightClass,
          invalid && 'border-destructive focus-visible:ring-destructive',
        )}
      />
    </div>
  );
}

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
 * 经验库方案编辑弹窗
 * Dialog to record or edit a verified playbook entry
 */
export function SaveMemoryDialog({
  open,
  onOpenChange,
  initialData,
  onSaved,
}: SaveMemoryDialogProps) {
  const t = useTranslations('troubleshooting');
  const {user: currentUser} = useAuth();
  const defaultAuthorName =
    currentUser?.nickname?.trim() || currentUser?.username?.trim() || '运维工程师';

  const [title, setTitle] = useState('');
  const [targetType, setTargetType] = useState<TroubleshootingTargetType>('error');
  const [fingerprint, setFingerprint] = useState('');
  const [errorSummary, setErrorSummary] = useState('');
  const [rootCause, setRootCause] = useState('');
  const [solution, setSolution] = useState('');
  const [preventiveTips, setPreventiveTips] = useState('');
  const [author, setAuthor] = useState(defaultAuthorName);
  const [userOptions, setUserOptions] = useState<Array<{id: number | string; label: string; value: string}>>([]);
  const [tags, setTags] = useState<string[]>([]);
  const [customTagInput, setCustomTagInput] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [isMaximized, setIsMaximized] = useState(false);
  // 当前单独放大编辑的大文本字段；与弹窗全局放大互不绑定。
  // Which long-text field is expanded for focused editing; independent from dialog maximize.
  const [expandedField, setExpandedField] = useState<ExpandableFieldKey | null>(null);
  const [errors, setErrors] = useState<{title?: string; solution?: string}>({});

  // 弹窗打开时拉取有权限的候选用户列表（用于作者快捷下拉选择）
  useEffect(() => {
    if (!open) return;
    let isMounted = true;
    services.monitoring
      .listNotifiableUsers()
      .then((res) => {
        if (!isMounted || !res?.users) return;
        const opts = res.users.map((u) => {
          const displayName = u.nickname?.trim() || u.username;
          return {
            id: u.id,
            label: u.nickname?.trim() ? `${u.nickname} (${u.username})` : u.username,
            value: displayName,
          };
        });
        setUserOptions(opts);
      })
      .catch(() => {
        // 静默降级：如失败则仅使用当前用户与手动输入
      });
    return () => {
      isMounted = false;
    };
  }, [open]);

  // 初始化或当 initialData 变动时预填充表单
  // Sync form state when initialData changes or dialog opens
  useEffect(() => {
    if (open && initialData) {
      setTitle(
        initialData.title ||
          (initialData.fingerprint
            ? `${initialData.fingerprint} ${t('defaultTitleSuffix')}`
            : ''),
      );
      setTargetType(initialData.target_type || 'error');
      setFingerprint(initialData.fingerprint || '');
      setErrorSummary(initialData.error_summary || '');
      setRootCause(initialData.root_cause || '');
      setSolution(initialData.solution || '');
      setPreventiveTips(initialData.preventive_tips || '');
      setAuthor(initialData.author || defaultAuthorName);
      setTags(initialData.tags || []);
      setErrors({});
      setExpandedField(null);
    } else if (open) {
      setTitle('');
      setTargetType('error');
      setFingerprint('');
      setErrorSummary('');
      setRootCause('');
      setSolution('');
      setPreventiveTips('');
      setAuthor(defaultAuthorName);
      setTags([]);
      setErrors({});
      setExpandedField(null);
    } else {
      setExpandedField(null);
      setIsMaximized(false);
    }
  }, [defaultAuthorName, initialData, open, t]);

  const expandedValue =
    expandedField === 'solution'
      ? solution
      : expandedField === 'root_cause'
        ? rootCause
        : expandedField === 'preventive_tips'
          ? preventiveTips
          : expandedField === 'error_summary'
            ? errorSummary
            : '';

  const setExpandedValue = useCallback(
    (next: string) => {
      if (expandedField === 'solution') {
        setSolution(next);
        if (errors.solution) {
          setErrors((prev) => ({...prev, solution: undefined}));
        }
        return;
      }
      if (expandedField === 'root_cause') {
        setRootCause(next);
        return;
      }
      if (expandedField === 'preventive_tips') {
        setPreventiveTips(next);
        return;
      }
      if (expandedField === 'error_summary') {
        setErrorSummary(next);
      }
    },
    [errors.solution, expandedField],
  );

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
        initialData?.id ? t('updateSuccess') : t('saveSuccess'),
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
    t,
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
        showCloseButton={false}
        className={cn(
          // 必须保留 fixed 居中；勿加 relative，否则会覆盖 DialogContent 默认 fixed，弹窗落到视口外
          // Keep fixed centering; do not add relative or it overrides DialogContent fixed and drops the dialog off-screen
          'border border-border/80 dark:border-border/60 shadow-2xl p-0 flex flex-col overflow-hidden bg-background rounded-xl transition-[width,height,max-width,max-height] duration-200',
          isMaximized
            ? 'w-[96vw] max-w-[96vw] h-[92vh] max-h-[92vh]'
            : 'w-[95vw] sm:max-w-3xl h-[85vh] max-h-[85vh]',
        )}
      >
        {/* 弹窗头部：明确预留 pr-20 物理安全避让区，杜绝与右上角操作按钮重叠 */}
        {/* Dialog Header: explicit pr-20 safe area avoiding collision with top-right action buttons */}
        <DialogHeader className='px-5 py-3.5 border-b bg-muted/20 pr-20 text-left sm:text-left relative z-30'>
          <div className='flex items-center gap-2'>
            <div className='flex size-7 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'>
              <Lightbulb className='size-3.5' />
            </div>
            <DialogTitle className='text-sm font-bold tracking-tight text-foreground'>
              {initialData?.id ? t('dialogEditTitle') : t('dialogCreateTitle')}
            </DialogTitle>
            <Badge
              variant='outline'
              className='h-5 px-1.5 text-[10px] font-mono border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10'
            >
              {t('title')}
            </Badge>
          </div>
          <DialogDescription className='text-xs text-muted-foreground mt-1'>
            {t('dialogSubtitle')}
          </DialogDescription>

          {/* 右上角快捷操作区：放大/还原 + 关闭窗口 */}
          <div className='absolute right-3 top-3 flex items-center gap-1 z-40'>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='size-7 text-muted-foreground hover:text-foreground'
              onClick={() => setIsMaximized((prev) => !prev)}
              title={isMaximized ? '还原窗口' : '放大看'}
            >
              {isMaximized ? <Minimize2 className='size-3.5' /> : <Maximize2 className='size-3.5' />}
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='size-7 text-muted-foreground hover:text-foreground'
              onClick={() => onOpenChange(false)}
              title='关闭'
            >
              <X className='size-4' />
            </Button>
          </div>
        </DialogHeader>

        {/* 表单主体：默认滚动；单个字段放大时用同层覆盖编辑 */}
        {/* Form body: scroll by default; one field can cover this pane for focused editing */}
        <div className='relative min-h-0 flex-1 overflow-hidden'>
          <div className='h-full overflow-y-auto px-5 py-4'>
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
            <div className='rounded-xl border border-emerald-500/35 bg-emerald-500/[0.02] dark:bg-emerald-950/20 p-3'>
              <MemoryTextField
                id='solution'
                fieldKey='solution'
                expandedField={expandedField}
                onExpand={setExpandedField}
                value={solution}
                onChange={(next) => {
                  setSolution(next);
                  if (errors.solution) {
                    setErrors((prev) => ({...prev, solution: undefined}));
                  }
                }}
                invalid={Boolean(errors.solution)}
                mono
                compactHeightClass='h-36 max-h-36 bg-background/90'
                placeholder={`请写下具体的处置操作与命令参数，例如：\n1. 在 seatunnel.yaml 或 seatunnel-env.sh 中调大 checkpoint 超时时长：checkpoint.timeout: 120000；\n2. 检查下游目标数据库写入负载，优化 Sink 端 batch.size 与写入并发，消除反压以加速 Barrier 对齐；\n3. 重启任务验证恢复状态。`}
                label={
                  <>
                    <ShieldCheck className='size-3.5 text-emerald-600 dark:text-emerald-400' />
                    <span className='font-semibold text-foreground'>
                      {t('solutionStepsLabel')}
                    </span>
                    <span className='text-destructive'>* (必填)</span>
                  </>
                }
                actions={
                  <button
                    type='button'
                    onClick={handleInsertTemplate}
                    className='text-[11px] text-emerald-600 dark:text-emerald-400 hover:underline cursor-pointer flex items-center gap-0.5'
                  >
                    <Plus className='size-3' />
                    插入步骤模板
                  </button>
                }
              />
              {errors.solution && (
                <p className='mt-1.5 text-[11px] text-destructive flex items-center gap-1 font-medium'>
                  <AlertCircle className='size-3 shrink-0' />
                  {errors.solution}
                </p>
              )}
            </div>

            {/* 区域 4：根因剖析与防范建议（两列紧凑并排） */}
            {/* Section 4: Root cause and preventive tips in compact two-column grid */}
            <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
              <MemoryTextField
                id='root_cause'
                fieldKey='root_cause'
                expandedField={expandedField}
                onExpand={setExpandedField}
                value={rootCause}
                onChange={setRootCause}
                compactHeightClass='h-24 max-h-24'
                placeholder='例如：连接空闲时长超过了服务端上限导致被主动切断...'
                label={<span className='text-muted-foreground'>根本原因剖析（选填）</span>}
              />

              <MemoryTextField
                id='preventive_tips'
                fieldKey='preventive_tips'
                expandedField={expandedField}
                onExpand={setExpandedField}
                value={preventiveTips}
                onChange={setPreventiveTips}
                compactHeightClass='h-24 max-h-24'
                placeholder='例如：批处理任务建议关闭外部持久化存储，流任务开启心跳...'
                label={<span className='text-muted-foreground'>防范与优化建议（选填）</span>}
              />
            </div>

            {/* 区域 5：故障现象摘要（选填） */}
            {/* Section 5: Error summary text snippet */}
            <MemoryTextField
              id='summary'
              fieldKey='error_summary'
              expandedField={expandedField}
              onExpand={setExpandedField}
              value={errorSummary}
              onChange={setErrorSummary}
              mono
              compactHeightClass='h-24 max-h-24'
              placeholder='可粘贴简要报错日志或异常栈片段...'
              label={
                <>
                  <FileCode className='size-3 text-muted-foreground' />
                  <span className='text-muted-foreground'>故障现象 / 关键错误日志摘要（选填）</span>
                </>
              }
            />

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
                <div className='flex items-center justify-between gap-2'>
                  <Label htmlFor='author' className='text-xs font-medium text-muted-foreground flex items-center gap-1'>
                    <User className='size-3 text-muted-foreground' />
                    <span>记录人 / 署名</span>
                  </Label>
                  {userOptions.length > 0 && (
                    <span className='text-[10px] text-muted-foreground'>
                      支持下拉选择或直接修改
                    </span>
                  )}
                </div>
                <div className='flex items-center gap-2'>
                  <Input
                    id='author'
                    value={author}
                    onChange={(e) => setAuthor(e.target.value)}
                    placeholder='例如：SRE 运维组 / 工程师'
                    className='h-7 text-xs flex-1'
                  />
                  {userOptions.length > 0 && (
                    <select
                      value={userOptions.some((u) => u.value === author) ? author : ''}
                      onChange={(e) => {
                        if (e.target.value) {
                          setAuthor(e.target.value);
                        }
                      }}
                      className='h-7 text-xs rounded-md border border-input bg-background px-2 py-0 text-muted-foreground hover:text-foreground focus:outline-hidden focus:ring-1 focus:ring-ring shrink-0 max-w-[130px] cursor-pointer'
                      title='选择有权限的用户'
                    >
                      <option value='' disabled>
                        选择成员...
                      </option>
                      {userOptions.map((u) => (
                        <option key={u.id} value={u.value}>
                          {u.label}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>

          {/* 单个大文本字段的放大编辑层：盖住表单主体，不新开全局弹窗 */}
          {/* Focused editor overlay for one long-text field; covers the form body without a nested dialog */}
          {expandedField ? (
            <div className='absolute inset-0 z-20 flex flex-col bg-background px-5 py-3'>
              <div className='mb-2 flex items-center justify-between gap-2'>
                <div className='min-w-0'>
                  <p className='text-xs font-semibold text-foreground truncate'>
                    {FIELD_EXPAND_TITLES[expandedField]}
                  </p>
                  <p className='text-[11px] text-muted-foreground'>单独放大编辑，关闭后回到表单</p>
                </div>
                <div className='flex items-center gap-1.5 shrink-0'>
                  {expandedField === 'solution' ? (
                    <button
                      type='button'
                      onClick={handleInsertTemplate}
                      className='text-[11px] text-emerald-600 dark:text-emerald-400 hover:underline cursor-pointer flex items-center gap-0.5'
                    >
                      <Plus className='size-3' />
                      插入步骤模板
                    </button>
                  ) : null}
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    className='h-7 px-2 text-xs'
                    onClick={() => setExpandedField(null)}
                  >
                    <Minimize2 className='size-3.5 mr-1' />
                    收起
                  </Button>
                </div>
              </div>
              <Textarea
                value={expandedValue}
                onChange={(e) => setExpandedValue(e.target.value)}
                autoFocus
                style={{fieldSizing: 'fixed'}}
                className={cn(
                  'field-sizing-fixed min-h-0 flex-1 overflow-y-auto resize-none text-xs leading-relaxed',
                  (expandedField === 'solution' || expandedField === 'error_summary') && 'font-mono',
                  expandedField === 'solution' &&
                    errors.solution &&
                    'border-destructive focus-visible:ring-destructive',
                )}
              />
              {expandedField === 'solution' && errors.solution ? (
                <p className='mt-1.5 text-[11px] text-destructive flex items-center gap-1 font-medium'>
                  <AlertCircle className='size-3 shrink-0' />
                  {errors.solution}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>

        {/* 弹窗底部操作 / Dialog Footer */}
        <DialogFooter className='px-5 py-3 border-t bg-muted/20 flex items-center justify-between sm:justify-between shrink-0'>
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
            {t('saveToLibrary')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
