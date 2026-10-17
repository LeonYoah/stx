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

import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import {AlertTriangle, Loader2} from 'lucide-react';

import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {ScrollArea} from '@/components/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Switch} from '@/components/ui/switch';
import {cn} from '@/lib/utils';
import {SyncService} from '@/lib/services/sync/sync.service';
import type {
  DolphinProject,
  DolphinTaskStatus,
  DolphinWorkflowDetail,
  DolphinWorkflowSummary,
  SchedulerConnection,
} from '@/lib/services/sync/types';

// 任务侧边栏里的推送弹窗：新建工作流、复用已有工作流，或只更新已绑定节点。
// Sidebar dialog that creates, reuses, or updates a DolphinScheduler SHELL node.
export function DolphinPushDialog({
  open,
  onOpenChange,
  taskId,
  status,
  onPushed,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: number;
  status?: DolphinTaskStatus | null;
  onPushed?: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  const bound = Boolean(status?.binding);
  const [mode, setMode] = useState<'new' | 'reuse'>('new');
  const [connections, setConnections] = useState<SchedulerConnection[]>([]);
  const [connectionId, setConnectionId] = useState<string>('');
  const [projects, setProjects] = useState<DolphinProject[]>([]);
  const [projectCode, setProjectCode] = useState<string>('');
  const [workflows, setWorkflows] = useState<DolphinWorkflowSummary[]>([]);
  const [workflowCode, setWorkflowCode] = useState<string>('');
  const [workflowName, setWorkflowName] = useState('');
  const [workflow, setWorkflow] = useState<DolphinWorkflowDetail | null>(null);
  const [upstreams, setUpstreams] = useState<number[]>([]);
  const [downstreams, setDownstreams] = useState<number[]>([]);
  const [useBuiltinSchedule, setUseBuiltinSchedule] = useState(false);
  const [creating, setCreating] = useState(false);
  const [connectionsLoaded, setConnectionsLoaded] = useState(false);
  const [connName, setConnName] = useState('');
  const [connURL, setConnURL] = useState('');
  const [connToken, setConnToken] = useState('');
  const [connVersion, setConnVersion] = useState('');
  const [busy, setBusy] = useState(false);
  const [cycle, setCycle] = useState(false);
  const selectedConnection = connections.find(
    (item) => String(item.id) === connectionId,
  );

  useEffect(() => {
    if (!open) {
      setConnectionsLoaded(false);
      return;
    }
    setMode('new');
    setCycle(false);
    setConnectionsLoaded(false);
    void SyncService.listSchedulerConnections()
      .then((result) => {
        const items = result.items ?? [];
        setConnections(items);
        const initial =
          status?.binding?.connection_id ||
          status?.connection?.id ||
          items[0]?.id;
        if (initial) {
          setConnectionId(String(initial));
        } else {
          setConnectionId('');
        }
        setConnectionsLoaded(true);
      })
      .catch(() => {
        setConnections([]);
        setConnectionId('');
        setConnectionsLoaded(true);
      });
  }, [open, status?.binding?.connection_id, status?.connection?.id]);

  useEffect(() => {
    if (!connectionId) {
      setProjects([]);
      return;
    }
    void SyncService.listDolphinProjects(Number(connectionId))
      .then((items) => {
        setProjects(items ?? []);
        const preferred =
          status?.binding?.project_code ||
          items[0]?.code;
        if (preferred) {
          setProjectCode(String(preferred));
        }
      })
      .catch(() => setProjects([]));
  }, [connectionId, status?.binding?.project_code]);

  useEffect(() => {
    if (!connectionId || !projectCode || mode !== 'reuse') {
      setWorkflows([]);
      return;
    }
    void SyncService.listDolphinWorkflows(Number(connectionId), Number(projectCode))
      .then((items) => setWorkflows(items ?? []))
      .catch(() => setWorkflows([]));
  }, [connectionId, projectCode, mode]);

  useEffect(() => {
    if (!open) {
      return;
    }
    if (bound && status?.binding?.connection_id && status.binding.project_code && status.binding.workflow_code) {
      void SyncService.getDolphinWorkflow(
        status.binding.connection_id,
        status.binding.project_code,
        status.binding.workflow_code,
      )
        .then((item) => setWorkflow(item))
        .catch(() => setWorkflow(null));
      return;
    }
    if (!connectionId || !projectCode || !workflowCode || mode !== 'reuse') {
      setWorkflow(null);
      return;
    }
    void SyncService.getDolphinWorkflow(
      Number(connectionId),
      Number(projectCode),
      Number(workflowCode),
    )
      .then((item) => setWorkflow(item))
      .catch(() => setWorkflow(null));
  }, [
    bound,
    connectionId,
    mode,
    open,
    projectCode,
    status?.binding?.connection_id,
    status?.binding?.project_code,
    status?.binding?.workflow_code,
    workflowCode,
  ]);

  const tasks = workflow?.tasks ?? [];
  const relations = workflow?.relations ?? [];
  const boundRemoteScript = tasks.find(
    (item) => item.code === status?.binding?.task_code,
  )?.raw_script;
  // 用 Dolphin 现有边加上即将新增的上下游做前端成环预检，最终仍以后端为准。
  // Preview cycles from existing Dolphin edges plus selected extra edges; the server remains authoritative.
  const cyclePreview = useMemo(() => {
    if (mode !== 'reuse' || bound || !workflow) {
      return false;
    }
    const edges = new Map<number, number[]>();
    const add = (from: number, to: number) => {
      if (!to || !from) {
        return;
      }
      const next = edges.get(from) ?? [];
      next.push(to);
      edges.set(from, next);
    };
    for (const rel of relations) {
      add(rel.pre_task_code, rel.post_task_code);
    }
    for (const up of upstreams) {
      add(up, -1);
    }
    for (const down of downstreams) {
      add(-1, down);
    }
    const color = new Map<number, number>();
    const visit = (node: number): boolean => {
      color.set(node, 1);
      for (const next of edges.get(node) ?? []) {
        const state = color.get(next) ?? 0;
        if (state === 1 || (state === 0 && visit(next))) {
          return true;
        }
      }
      color.set(node, 2);
      return false;
    };
    for (const node of edges.keys()) {
      if ((color.get(node) ?? 0) === 0 && visit(node)) {
        return true;
      }
    }
    return false;
  }, [bound, downstreams, mode, relations, upstreams, workflow]);

  useEffect(() => {
    setCycle(cyclePreview);
  }, [cyclePreview]);

  const submit = async (dryRun: boolean) => {
    if (!connectionId) {
      toast.error(t('dolphinConnectionRequired'));
      return;
    }
    setBusy(true);
    try {
      const result = await SyncService.pushDolphin(taskId, {
        mode: bound ? 'update' : mode,
        connection_id: Number(connectionId),
        project_code: Number(projectCode || 0),
        workflow_name: workflowName,
        workflow_code: Number(workflowCode || 0),
        upstreams,
        downstreams,
        use_builtin_schedule: useBuiltinSchedule,
        dry_run: dryRun,
        expected_version: workflow?.version || undefined,
      });
      if (result.cycle) {
        setCycle(true);
        toast.error(t('dolphinCycleBlocked'));
        return;
      }
      if (dryRun) {
        toast.success(result.message || t('dolphinDryRunOk'));
        return;
      }
      toast.success(t('dolphinPushOk'));
      onPushed?.();
      onOpenChange(false);
    } catch (error) {
      const message = error instanceof Error ? error.message : t('dolphinPushFailed');
      // 后端成环返回 400，前端仍切到成环提示，避免只出现一条泛化失败 toast。
      // A cycle is a 400 from the server; keep the in-dialog cycle warning instead of a generic toast.
      if (/cycle/i.test(message)) {
        setCycle(true);
        toast.error(t('dolphinCycleBlocked'));
        return;
      }
      toast.error(message);
    } finally {
      setBusy(false);
    }
  };

  const createConnection = async () => {
    if (!connName.trim() || !connURL.trim()) {
      toast.error(t('dolphinConnectionRequired'));
      return;
    }
    if (!connToken.trim()) {
      toast.error(t('dolphinTokenMissing'));
      return;
    }
    setCreating(true);
    try {
      const created = await SyncService.createSchedulerConnection({
        name: connName.trim(),
        base_url: connURL.trim(),
        token: connToken.trim(),
        dolphin_version: connVersion.trim() || undefined,
      });
      setConnections((items) => [...items, created]);
      setConnectionId(String(created.id));
      setConnToken('');
      toast.success(t('dolphinConnectionSaved'));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('dolphinPushFailed'));
    } finally {
      setCreating(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className='flex max-h-[88vh] flex-col overflow-hidden border border-border/80 p-0 shadow-2xl sm:max-w-4xl dark:border-border/60'
      >
        <DialogHeader className='border-b border-border/60 px-6 py-4'>
          <DialogTitle>
            {bound ? t('dolphinUpdateNode') : t('dolphinPushTitle')}
          </DialogTitle>
        </DialogHeader>
        <ScrollArea className='min-h-0 flex-1 px-6 py-4'>
          <div className='space-y-4'>
            {!connectionsLoaded ? null : connections.length === 0 ? (
              <div
                className='space-y-3 rounded-lg border border-border/50 bg-muted/10 p-3'
                data-testid='dolphin-new-connection-form'
              >
                <p className='text-xs leading-relaxed text-muted-foreground'>
                  {t('dolphinNoConnectionHint')}
                </p>
                <ConnectionCreateFields
                  connName={connName}
                  connURL={connURL}
                  connToken={connToken}
                  connVersion={connVersion}
                  creating={creating}
                  onNameChange={setConnName}
                  onURLChange={setConnURL}
                  onTokenChange={setConnToken}
                  onVersionChange={setConnVersion}
                  onSave={() => void createConnection()}
                />
              </div>
            ) : (
              <>
                <div className='grid gap-3 sm:grid-cols-2'>
                  <div className='space-y-1.5 min-w-0'>
                    <Label>{t('dolphinConnection')}</Label>
                    <Select value={connectionId} onValueChange={setConnectionId}>
                      <SelectTrigger className='w-full'>
                        <SelectValue placeholder={t('dolphinSelectConnection')} />
                      </SelectTrigger>
                      <SelectContent>
                        {connections.map((item) => (
                          <SelectItem key={item.id} value={String(item.id)}>
                            {item.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {selectedConnection?.token_set ? (
                      <p className='text-[11px] text-muted-foreground'>
                        {t('dolphinConnectionTokenSet')}
                      </p>
                    ) : selectedConnection ? (
                      <p className='text-[11px] text-destructive'>
                        {t('dolphinConnectionTokenMissing')}
                      </p>
                    ) : null}
                  </div>
                  <div className='space-y-1.5 min-w-0'>
                    <Label>{t('dolphinProject')}</Label>
                    <Select value={projectCode} onValueChange={setProjectCode}>
                      <SelectTrigger className='w-full'>
                        <SelectValue placeholder={t('dolphinSelectProject')} />
                      </SelectTrigger>
                      <SelectContent>
                        {projects.map((item) => (
                          <SelectItem key={item.code} value={String(item.code)}>
                            {item.name} ({item.code})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <details
                  className='rounded-lg border border-border/50 bg-muted/10 p-3'
                  data-testid='dolphin-new-connection-details'
                >
                  <summary className='cursor-pointer text-xs font-medium'>
                    {t('dolphinNewConnection')}
                  </summary>
                  <div className='mt-3' data-testid='dolphin-new-connection-form'>
                    <ConnectionCreateFields
                      connName={connName}
                      connURL={connURL}
                      connToken={connToken}
                      connVersion={connVersion}
                      creating={creating}
                      onNameChange={setConnName}
                      onURLChange={setConnURL}
                      onTokenChange={setConnToken}
                      onVersionChange={setConnVersion}
                      onSave={() => void createConnection()}
                    />
                  </div>
                </details>
              </>
            )}

            {bound ? (
              <div className='space-y-3 rounded-lg border border-border/50 bg-muted/10 p-3 text-xs'>
                <div className='font-medium'>{t('dolphinBoundHint')}</div>
                {boundRemoteScript ? (
                  <div className='min-w-0'>
                    <div className='mb-1 text-[11px] text-muted-foreground'>
                      {t('dolphinRemoteScript')}
                    </div>
                    <pre className='max-h-32 overflow-auto whitespace-pre-wrap font-mono text-[11px]'>
                      {boundRemoteScript}
                    </pre>
                  </div>
                ) : null}
                <div className='min-w-0'>
                  <div className='mb-1 text-[11px] text-muted-foreground'>
                    {t('dolphinGeneratedScript')}
                  </div>
                  <pre className='max-h-40 overflow-auto whitespace-pre-wrap font-mono text-[11px]'>
                    {status?.script?.script}
                  </pre>
                </div>
              </div>
            ) : (
              <>
                <div className='flex flex-wrap gap-2'>
                  {(['new', 'reuse'] as const).map((item) => (
                    <button
                      key={item}
                      type='button'
                      onClick={() => setMode(item)}
                      className={cn(
                        'rounded-full border px-3 py-1 text-xs',
                        mode === item
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border/60 text-muted-foreground',
                      )}
                    >
                      {item === 'new' ? t('dolphinModeNew') : t('dolphinModeReuse')}
                    </button>
                  ))}
                </div>
                {mode === 'new' ? (
                  <div className='space-y-3'>
                    <div className='space-y-1.5'>
                      <Label>{t('dolphinWorkflowName')}</Label>
                      <Input
                        value={workflowName}
                        onChange={(event) => setWorkflowName(event.target.value)}
                      />
                    </div>
                    <label className='flex items-center gap-2 text-xs'>
                      <Switch
                        checked={useBuiltinSchedule}
                        onCheckedChange={setUseBuiltinSchedule}
                      />
                      {t('dolphinUseBuiltinSchedule')}
                    </label>
                  </div>
                ) : (
                  <div className='space-y-3'>
                    <div className='space-y-1.5'>
                      <Label>{t('dolphinWorkflow')}</Label>
                      <Select value={workflowCode} onValueChange={setWorkflowCode}>
                        <SelectTrigger className='w-full'>
                          <SelectValue placeholder={t('dolphinSelectWorkflow')} />
                        </SelectTrigger>
                        <SelectContent>
                          {workflows.map((item) => (
                            <SelectItem key={item.code} value={String(item.code)}>
                              {item.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className='grid gap-3 sm:grid-cols-2'>
                      <NodeChecks
                        title={t('dolphinUpstreams')}
                        tasks={tasks}
                        selected={upstreams}
                        onChange={setUpstreams}
                      />
                      <NodeChecks
                        title={t('dolphinDownstreams')}
                        tasks={tasks}
                        selected={downstreams}
                        onChange={setDownstreams}
                      />
                    </div>
                  </div>
                )}
              </>
            )}

            {cycle ? (
              <div className='flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive'>
                <AlertTriangle className='mt-0.5 size-3.5 shrink-0' />
                <span>{t('dolphinCycleBlocked')}</span>
              </div>
            ) : null}
          </div>
        </ScrollArea>
        <DialogFooter className='border-t border-border/60 bg-muted/20 px-6 py-3'>
          <Button type='button' variant='outline' onClick={() => onOpenChange(false)}>
            {t('cancel')}
          </Button>
          <Button
            type='button'
            variant='secondary'
            disabled={busy || cycle}
            onClick={() => void submit(true)}
          >
            {t('dolphinDryRun')}
          </Button>
          <Button type='button' disabled={busy || cycle} onClick={() => void submit(false)}>
            {busy ? <Loader2 className='size-3.5 animate-spin' /> : t('dolphinConfirmPush')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// 新建连接表单：Open API Token 必须带标签且可见，不能只靠 placeholder。
// New-connection form: the Open API token is a labeled required field, not a placeholder-only input.
function ConnectionCreateFields({
  connName,
  connURL,
  connToken,
  connVersion,
  creating,
  onNameChange,
  onURLChange,
  onTokenChange,
  onVersionChange,
  onSave,
}: {
  connName: string;
  connURL: string;
  connToken: string;
  connVersion: string;
  creating: boolean;
  onNameChange: (value: string) => void;
  onURLChange: (value: string) => void;
  onTokenChange: (value: string) => void;
  onVersionChange: (value: string) => void;
  onSave: () => void;
}) {
  const t = useTranslations('workbenchStudio');
  return (
    <div className='space-y-3'>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-1.5 min-w-0'>
          <Label htmlFor='dolphin-connection-name'>{t('dolphinConnectionName')}</Label>
          <Input
            id='dolphin-connection-name'
            value={connName}
            onChange={(event) => onNameChange(event.target.value)}
            placeholder={t('dolphinConnectionName')}
          />
        </div>
        <div className='space-y-1.5 min-w-0'>
          <Label htmlFor='dolphin-connection-url'>
            {t('dolphinBaseURL')}
            <span className='text-destructive'>*</span>
          </Label>
          <Input
            id='dolphin-connection-url'
            value={connURL}
            onChange={(event) => onURLChange(event.target.value)}
            placeholder='http://dolphin:12345/dolphinscheduler'
            autoComplete='off'
          />
        </div>
        <div className='space-y-1.5 min-w-0 sm:col-span-2'>
          <Label htmlFor='dolphin-open-api-token'>
            {t('dolphinOpenApiToken')}
            <span className='text-destructive'>*</span>
          </Label>
          <Input
            id='dolphin-open-api-token'
            data-testid='dolphin-open-api-token'
            type='password'
            value={connToken}
            onChange={(event) => onTokenChange(event.target.value)}
            placeholder={t('dolphinTokenPlaceholder')}
            autoComplete='new-password'
            required
            aria-required='true'
          />
          <p className='text-[11px] leading-relaxed text-muted-foreground'>
            {t('dolphinOpenApiTokenHint')}
          </p>
        </div>
        <div className='space-y-1.5 min-w-0 sm:col-span-2'>
          <Label htmlFor='dolphin-connection-version'>{t('dolphinVersion')}</Label>
          <Input
            id='dolphin-connection-version'
            value={connVersion}
            onChange={(event) => onVersionChange(event.target.value)}
            placeholder={t('dolphinVersionPlaceholder')}
          />
        </div>
      </div>
      <p className='text-[11px] leading-relaxed text-muted-foreground'>
        {t('dolphinVersionHint')}
      </p>
      <Button type='button' size='sm' disabled={creating} onClick={onSave}>
        {creating ? <Loader2 className='size-3.5 animate-spin' /> : t('dolphinSaveConnection')}
      </Button>
    </div>
  );
}

// 复用工作流时勾选上下游节点。
// Checkbox list for upstream/downstream nodes when reusing a workflow.
function NodeChecks({
  title,
  tasks,
  selected,
  onChange,
}: {
  title: string;
  tasks: {code: number; name: string}[];
  selected: number[];
  onChange: (values: number[]) => void;
}) {
  return (
    <div className='rounded-lg border border-border/50 p-3'>
      <div className='mb-2 text-xs font-medium'>{title}</div>
      <div className='max-h-40 space-y-1.5 overflow-auto'>
        {tasks.map((task) => {
          const checked = selected.includes(task.code);
          return (
            <label key={task.code} className='flex items-center gap-2 text-xs'>
              <Checkbox
                checked={checked}
                onCheckedChange={(value) => {
                  if (value) {
                    onChange([...selected, task.code]);
                    return;
                  }
                  onChange(selected.filter((code) => code !== task.code));
                }}
              />
              <span className='truncate font-mono'>
                {task.name} #{task.code}
              </span>
            </label>
          );
        })}
      </div>
    </div>
  );
}
