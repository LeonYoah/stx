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

import {useState, useEffect, useCallback, useRef, type ReactNode} from 'react';
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
import {Button} from '@/components/arc/button/button';
import {Combobox} from '@/components/arc/combobox/combobox';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input as ArcInput} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {UnsavedChangesDialog} from '@/components/common/layout/UnsavedChangesDialog';
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
            size='sm'
            iconOnly
            aria-label={isExpanded ? '收起' : '放大编辑'}
            onClick={() => onExpand(isExpanded ? null : fieldKey)}
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
  const tGuard = useTranslations('monitoring');
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
  const [confirmDiscardOpen, setConfirmDiscardOpen] = useState(false);
  // 打开时快照一次表单，脏表单关闭前走未保存拦截。
  // Snapshot the filled form on open; closing a dirty form asks first.
  const filledSnapshotRef = useRef('');

  const formSnapshot = useCallback(
    () =>
      JSON.stringify({
        title,
        targetType,
        fingerprint,
        errorSummary,
        rootCause,
        solution,
        preventiveTips,
        author,
        tags,
      }),
    [
      author,
      errorSummary,
      fingerprint,
      preventiveTips,
      rootCause,
      solution,
      tags,
      targetType,
      title,
    ],
  );

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
      const next = {
        title:
          initialData.title ||
          (initialData.fingerprint
            ? `${initialData.fingerprint} ${t('defaultTitleSuffix')}`
            : ''),
        targetType: initialData.target_type || 'error',
        fingerprint: initialData.fingerprint || '',
        errorSummary: initialData.error_summary || '',
        rootCause: initialData.root_cause || '',
        solution: initialData.solution || '',
        preventiveTips: initialData.preventive_tips || '',
        author: initialData.author || defaultAuthorName,
        tags: initialData.tags || [],
      };
      setTitle(next.title);
      setTargetType(next.targetType);
      setFingerprint(next.fingerprint);
      setErrorSummary(next.errorSummary);
      setRootCause(next.rootCause);
      setSolution(next.solution);
      setPreventiveTips(next.preventiveTips);
      setAuthor(next.author);
      setTags(next.tags);
      setErrors({});
      setExpandedField(null);
      filledSnapshotRef.current = JSON.stringify(next);
    } else if (open) {
      const next = {
        title: '',
        targetType: 'error',
        fingerprint: '',
        errorSummary: '',
        rootCause: '',
        solution: '',
        preventiveTips: '',
        author: defaultAuthorName,
        tags: [] as string[],
      };
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
      filledSnapshotRef.current = JSON.stringify(next);
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
      newErrors.solution = t('solutionRequired');
    }

    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      // 校验失败只留内联错误并把首个报错字段滚入视野聚焦，不再叠加全局 toast。
      // On invalid fields keep the inline errors and focus the first one; no global toast.
      if (newErrors.title && expandedField) {
        setExpandedField(null);
      }
      const firstId = newErrors.title
        ? 'memory-field-title'
        : expandedField === 'solution'
          ? 'memory-expand-editor'
          : 'memory-field-solution';
      requestAnimationFrame(() => {
        const el = document.getElementById(firstId);
        el?.scrollIntoView({block: 'center', behavior: 'smooth'});
        el?.focus({preventScroll: true});
      });
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
    expandedField,
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

  // 关闭请求：脏表单先确认，干净直接关。
  // Close request: confirm first when the form is dirty; clean closes pass through.
  const requestClose = useCallback(() => {
    if (formSnapshot() !== filledSnapshotRef.current) {
      setConfirmDiscardOpen(true);
      return;
    }
    onOpenChange(false);
  }, [formSnapshot, onOpenChange]);

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (next) {
            onOpenChange(true);
            return;
          }
          requestClose();
        }}
      >
        <DialogContent
          title={
            <span className='flex items-center gap-2'>
              <span className='flex size-6 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground'>
                <Lightbulb className='size-3.5' />
              </span>
              <span className='min-w-0 truncate'>
                {initialData?.id ? t('dialogEditTitle') : t('dialogCreateTitle')}
              </span>
              <Badge
                variant='outline'
                className='h-5 px-1.5 text-[10px] font-mono text-muted-foreground'
              >
                {t('title')}
              </Badge>
              <span className='ml-auto shrink-0'>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  iconOnly
                  aria-label={isMaximized ? '还原窗口' : '放大窗口'}
                  onClick={() => setIsMaximized((prev) => !prev)}
                >
                  {isMaximized ? (
                    <Minimize2 className='size-3.5' />
                  ) : (
                    <Maximize2 className='size-3.5' />
                  )}
                </Button>
              </span>
            </span>
          }
          className={cn(
            'arc-compact transition-[width] duration-200',
            isMaximized
              ? 'w-[min(96vw,1400px)]!'
              : 'w-[min(95vw,768px)]!',
          )}
          bodyClassName={cn(
            'flex! min-h-0! flex-col! overflow-hidden! p-0! transition-[height] duration-200',
            isMaximized
              ? 'h-[min(92dvh,1000px)]!'
              : 'h-[min(85dvh,720px)]!',
          )}
        >
          {/* 表单主体：默认滚动；单个字段放大时用同层覆盖编辑 */}
          {/* Form body: scroll by default; one field can cover this pane for focused editing */}
          <div className='relative min-h-0 flex-1 overflow-hidden'>
            <div className='h-full overflow-y-auto px-5 py-4'>
              <div className='space-y-4 text-xs pr-1'>
                {/* 区域 1：关联故障指纹与类型 */}
                {/* Section 1: Fault fingerprint context and target category */}
                <div className='flex items-center justify-between gap-3 px-3 py-2 rounded-lg bg-muted/40 border border-border/60'>
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
                  {initialData?.target_type ? (
                    <Badge
                      variant='secondary'
                      className='text-[10px] font-medium shrink-0 h-5 px-1.5'
                    >
                      {targetType === 'error' ? t('errorType') : t('alertType')}
                    </Badge>
                  ) : (
                    <SegmentedControl
                      value={targetType}
                      onValueChange={(value) =>
                        setTargetType(value as TroubleshootingTargetType)
                      }
                      label={t('targetTypeLabel')}
                      options={[
                        {value: 'error', label: t('errorType')},
                        {value: 'alert', label: t('alertType')},
                      ]}
                      className='shrink-0'
                    />
                  )}
                </div>

                {/* 区域 2：方案标题 */}
                {/* Section 2: Solution Title */}
                <ArcInput
                  id='memory-field-title'
                  label='方案标题'
                  value={title}
                  onChange={(e) => {
                    setTitle(e.target.value);
                    if (errors.title) {
                      setErrors((prev) => ({...prev, title: undefined}));
                    }
                  }}
                  placeholder='例如：MySQL 连接超时中断 (Communications link failure) 恢复方案'
                  error={errors.title}
                />

                {/* 区域 3：核心必填解决方案（高信噪比主视区） */}
                {/* Section 3: Core mandatory solution and execution steps */}
                <div className='rounded-xl border border-border/60 bg-muted/10 p-3'>
                  <MemoryTextField
                    id='memory-field-solution'
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
                        <ShieldCheck className='size-3.5 text-primary' />
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
                        className='text-[11px] text-primary hover:underline cursor-pointer flex items-center gap-0.5'
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
                    id='memory-field-root-cause'
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
                    id='memory-field-preventive-tips'
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
                  id='memory-field-error-summary'
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
                        variant='secondary'
                        size='sm'
                        onClick={() => handleAddTag(customTagInput)}
                      >
                        <Plus className='size-3' />
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
                    <ArcInput
                      id='memory-field-author'
                      label='记录人 / 署名'
                      value={author}
                      onChange={(e) => setAuthor(e.target.value)}
                      placeholder='例如：SRE 运维组 / 工程师'
                    />
                    {userOptions.length > 0 && (
                      <Combobox
                        aria-label={t('pickMember')}
                        value=''
                        options={userOptions.map((u) => ({
                          value: u.value,
                          label: u.label,
                          keywords: [u.label, u.value],
                        }))}
                        onValueChange={(value) => setAuthor(value)}
                        placeholder={t('memberSearchPlaceholder')}
                        emptyMessage={t('memberEmpty')}
                      />
                    )}
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
                        className='text-[11px] text-primary hover:underline cursor-pointer flex items-center gap-0.5'
                      >
                        <Plus className='size-3' />
                        插入步骤模板
                      </button>
                    ) : null}
                    <Button
                      type='button'
                      variant='secondary'
                      size='sm'
                      onClick={() => setExpandedField(null)}
                    >
                      <Minimize2 className='size-3.5' />
                      收起
                    </Button>
                  </div>
                </div>
                <Textarea
                  id='memory-expand-editor'
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
          <div className='px-5 py-3 border-t border-border/60 bg-muted/20 flex items-center justify-between shrink-0'>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={requestClose}
            >
              取消
            </Button>
            <Button
              type='button'
              size='sm'
              variant='primary'
              onClick={handleSubmit}
              loading={submitting}
              disabled={submitting}
            >
              <Save className='size-3.5' />
              {t('saveToLibrary')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <UnsavedChangesDialog
        open={confirmDiscardOpen}
        onOpenChange={setConfirmDiscardOpen}
        title={tGuard('unsavedChangesTitle')}
        description={tGuard('unsavedChangesDescription')}
        keepLabel={tGuard('keepEditing')}
        discardLabel={tGuard('discardChanges')}
        onKeepEditing={() => setConfirmDiscardOpen(false)}
        onDiscard={() => {
          setConfirmDiscardOpen(false);
          onOpenChange(false);
        }}
      />
    </>
  );
}
