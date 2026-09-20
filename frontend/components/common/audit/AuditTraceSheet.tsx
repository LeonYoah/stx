'use client';

/**
 * 同一次控制层操作下发到 Agent 的命令链路。
 * Agent commands issued by one control-plane operation.
 */

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import services from '@/lib/services';
import {AuditLogInfo, CommandLogInfo, CommandParameters} from '@/lib/services/audit/types';

interface AuditTraceSheetProps {
  log: AuditLogInfo | null;
  onOpenChange: (open: boolean) => void;
}

/** 只挑安装目录、角色等关键字段做一行摘要，不展开整份参数。
 * Keep a one-line hint from known fields instead of dumping the full parameter map.
 */
function commandHint(parameters: CommandParameters | null): string {
  if (!parameters) {
    return '';
  }
  const keys = ['install_dir', 'role', 'process_name', 'node_id', 'path'];
  return keys
    .map((key) => parameters[key])
    .filter((value): value is string => typeof value === 'string' && value.trim() !== '')
    .join(' · ');
}

export function AuditTraceSheet({log, onOpenChange}: AuditTraceSheetProps) {
  const t = useTranslations();
  const [commands, setCommands] = useState<CommandLogInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!log?.request_id) {
      setCommands([]);
      setError('');
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError('');
    services.audit
      .getCommandLogsSafe({current: 1, size: 50, request_id: log.request_id})
      .then((result) => {
        if (cancelled) {
          return;
        }
        if (!result.success) {
          setCommands([]);
          setError(result.error || t('audit.traceFailed'));
          return;
        }
        // 列表接口按时间倒序，链路展示改成执行顺序。
        // The list API is newest-first; reverse it so the sheet reads in execution order.
        const items = result.data?.commands || [];
        setCommands([...items].reverse());
      })
      .catch(() => {
        if (!cancelled) {
          setCommands([]);
          setError(t('audit.traceFailed'));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [log?.request_id, t]);

  return (
    <Sheet open={log != null} onOpenChange={onOpenChange}>
      <SheetContent className='flex w-[480px] flex-col sm:max-w-[480px]'>
        <SheetHeader>
          <SheetTitle>{t('audit.traceTitle')}</SheetTitle>
          <SheetDescription>
            {log ? getTraceHeadline(log, t) : ''}
          </SheetDescription>
        </SheetHeader>
        <div className='mt-4 min-h-0 flex-1 space-y-3 overflow-y-auto pr-1'>
          {loading ? <p className='text-sm text-muted-foreground'>{t('common.loading')}</p> : null}
          {!loading && error ? <p className='text-sm text-destructive'>{error}</p> : null}
          {!loading && !error && commands.length === 0 ? (
            <p className='text-sm text-muted-foreground'>{t('audit.traceEmpty')}</p>
          ) : null}
          {commands.map((command, index) => {
            const hint = commandHint(command.parameters);
            return (
              <div key={command.command_id || command.id} className='rounded-md border px-3 py-2'>
                <div className='flex items-center justify-between gap-2'>
                  <span className='font-mono text-sm'>{index + 1}. {command.command_type}</span>
                  <Badge variant={command.status === 'failed' ? 'destructive' : 'outline'}>
                    {t(`audit.statuses.${command.status}`)}
                  </Badge>
                </div>
                <p className='mt-1 truncate text-xs text-muted-foreground'>{command.agent_id}</p>
                {hint ? <p className='mt-1 break-all text-xs text-muted-foreground'>{hint}</p> : null}
                {command.display_command ? (
                  <div className='mt-2 rounded-md border bg-muted/30 px-2.5 py-2'>
                    <p className='text-[11px] text-muted-foreground'>{t('audit.actualCommand')}</p>
                    <code
                      className='mt-1 block overflow-x-auto whitespace-nowrap font-mono text-xs'
                      title={command.display_command}
                    >
                      {command.display_command}
                    </code>
                  </div>
                ) : null}
                {command.error ? <p className='mt-1 text-xs text-destructive'>{command.error}</p> : null}
              </div>
            );
          })}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function getTraceHeadline(log: AuditLogInfo, t: (key: string) => string): string {
  const actionKey = `audit.actions.${(log.action || '').replace(/\./g, '_')}`;
  let action = log.action || '';
  try {
    const label = t(actionKey);
    if (label && label !== actionKey) {
      action = label;
    }
  } catch {
    // 缺文案时保留原始 action / Keep the raw action when the message is missing.
  }
  return [action, log.resource_name].filter(Boolean).join(' · ');
}
